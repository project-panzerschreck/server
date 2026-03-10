package tracker

import (
	"encoding/json"
	"log"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// the number of seconds after which an RPC server is removed from the list
const expiryDuration = 60 * time.Second

// the number of seconds to wait between announces
const interval = expiryDuration / 2

type Tracker struct {
	sync.RWMutex
	RpcServers map[string]clientInfo
}

type clientInfo struct {
	RpcServerInfo
	expiryTimer *time.Timer
}

type RpcServerInfo struct {
	Ip          string    `json:"ip"`
	Port        int       `json:"port"`
	LastSeen    time.Time `json:"last_seen"`
	MaxSize     int64     `json:"max_size"`
	Battery     float64   `json:"battery"`
	Temperature float64   `json:"temperature"`
}

func NewTracker() *Tracker {
	return &Tracker{
		RpcServers: make(map[string]clientInfo),
	}
}

func (t *Tracker) AddRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/announce", t.Announce)
	mux.HandleFunc("/servers", t.ListServers)
}

func (t *Tracker) Announce(w http.ResponseWriter, r *http.Request) {
	log.Printf("Announce request from %s", r.Host)
	type response struct {
		Interval int `json:"interval"`
	}

	t.Lock()
	defer t.Unlock()

	port := r.URL.Query().Get("port")
	if port == "" {
		http.Error(w, "missing port", http.StatusBadRequest)
		return
	}

	portNum, err := strconv.Atoi(port)
	if err != nil {
		http.Error(w, "invalid port", http.StatusBadRequest)
		return
	}

	ip := r.URL.Query().Get("ip")
	if ip == "" {
		// fill with the ip from r.RemoteAddr
		ip = strings.SplitN(r.RemoteAddr, ":", 2)[0]
	}

	var maxSize int64 = -1
	if maxSizeStr := r.URL.Query().Get("max_size"); maxSizeStr != "" {
		maxSize, _ = strconv.ParseInt(maxSizeStr, 10, 64)
	}

	var battery float64 = math.NaN()
	if batteryStr := r.URL.Query().Get("battery"); batteryStr != "" {
		battery, _ = strconv.ParseFloat(batteryStr, 64)
	}

	var temperature float64 = math.NaN()
	if temperatureStr := r.URL.Query().Get("temperature"); temperatureStr != "" {
		temperature, _ = strconv.ParseFloat(temperatureStr, 64)
	}

	// todo: validate ip

	clientId := ip + ":" + port

	// avoid duplicate timers
	if existingTimer := t.RpcServers[clientId].expiryTimer; existingTimer != nil {
		existingTimer.Stop()
		log.Printf("Reannounce from %s", clientId)
	} else {
		log.Printf("New announce from %s", clientId)
	}

	announceTime := time.Now()

	t.RpcServers[clientId] = clientInfo{
		RpcServerInfo: RpcServerInfo{
			LastSeen:    announceTime,
			Ip:          ip,
			Port:        portNum,
			MaxSize:     maxSize,
			Battery:     battery,
			Temperature: temperature,
		},
		expiryTimer: time.AfterFunc(expiryDuration, func() {
			t.Lock()
			defer t.Unlock()

			// there's a possible race condition if the client announces just as the timer expires,
			// preventing the timer from being stopped. To prevent that, we verify that the last seen time
			// has not been changed.
			if t.RpcServers[clientId].LastSeen.Equal(announceTime) {
				delete(t.RpcServers, clientId)
				log.Printf("Removed %s from tracker", clientId)
			}
		}),
	}

	// respond
	w.Header().Add("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(response{
		Interval: int(interval.Seconds()),
	})

	if err != nil {
		log.Printf("Failed to respond to announce: %v", err)
	}
}

func (t *Tracker) ListServers(w http.ResponseWriter, r *http.Request) {
	type response struct {
		Servers []RpcServerInfo `json:"servers"`
	}

	servers := t.GetServers()

	w.Header().Add("Content-Type", "application/json")

	err := json.NewEncoder(w).Encode(response{
		Servers: servers,
	})

	if err != nil {
		log.Printf("Failed to respond to list servers: %v", err)
	}
}

func (t *Tracker) GetServers() []RpcServerInfo {
	t.RLock()
	defer t.RUnlock()

	servers := make([]RpcServerInfo, 0, len(t.RpcServers))
	for _, server := range t.RpcServers {
		servers = append(servers, server.RpcServerInfo)
	}

	sort.Slice(servers, func(i, j int) bool {
		if servers[i].Ip < servers[j].Ip {
			return true
		}
		if servers[i].Ip > servers[j].Ip {
			return false
		}
		return servers[i].Port < servers[j].Port
	})

	return servers
}

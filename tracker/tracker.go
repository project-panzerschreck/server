package tracker

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// the number of seconds after which an RPC server is removed from the list
const expiryDuration = 1 * time.Hour

// the number of seconds to wait between announces
const interval = expiryDuration / 2

type Tracker struct {
	sync.RWMutex
	RpcServers map[string]*clientInfo
}

type ClientDetails struct {
	Model       string
	MaxSize     int64
	Battery     float64
	Temperature float64
}

type clientInfo struct {
	lastSeen    time.Time
	expiryTimer *time.Timer
	model       string
	maxSize     int64
	battery     float64
	temperature float64
	reachable   bool // indicates if the node passed a health check
}

func NewTracker() *Tracker {
	t := &Tracker{
		RpcServers: make(map[string]*clientInfo),
	}
	go t.healthCheckLoop()
	return t
}

func (t *Tracker) healthCheckLoop() {
	for {
		t.RLock()
		var serversToCheck []string
		for id := range t.RpcServers {
			serversToCheck = append(serversToCheck, id)
		}
		t.RUnlock()

		for _, id := range serversToCheck {
			// Quick net.Dial to see if it's reachable. Only check the port listed.
			conn, err := net.DialTimeout("tcp", id, 5*time.Second)
			reachable := err == nil
			if err == nil {
				conn.Close()
			}

			t.Lock()
			if info, exists := t.RpcServers[id]; exists {
				if info.reachable != reachable {
					log.Printf("Tracker node %s reachability changed: %v", id, reachable)
				}
				info.reachable = reachable
			}
			t.Unlock()
		}

		time.Sleep(30 * time.Second)
	}
}

func (t *Tracker) AddRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/announce", t.Announce)
	mux.HandleFunc("/servers", t.ListServers)
}

func (t *Tracker) Announce(w http.ResponseWriter, r *http.Request) {
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

	ip := r.URL.Query().Get("ip")
	if ip == "" {
		// fill with the ip from r.RemoteAddr
		ip = strings.SplitN(r.RemoteAddr, ":", 2)[0]
	}

	// todo validate that the IP and port are valid

	clientId := ip + ":" + port

	model := r.URL.Query().Get("model")
	maxSizeStr := r.URL.Query().Get("max_size")
	batteryStr := r.URL.Query().Get("battery")
	tempStr := r.URL.Query().Get("temperature")

	var maxSize int64
	if maxSizeStr != "" {
		maxSize, _ = strconv.ParseInt(maxSizeStr, 10, 64)
	}

	var battery float64
	if batteryStr != "" {
		battery, _ = strconv.ParseFloat(batteryStr, 64)
	}

	var temperature float64
	if tempStr != "" {
		temperature, _ = strconv.ParseFloat(tempStr, 64)
	}

	// Evict any existing entry for this IP (possibly on a different port) so each
	// physical device occupies exactly one slot in the map.
	alreadyLogged := false
	for existingId, existingInfo := range t.RpcServers {
		if strings.SplitN(existingId, ":", 2)[0] == ip {
			if existingInfo.expiryTimer != nil {
				existingInfo.expiryTimer.Stop()
			}
			delete(t.RpcServers, existingId)
			if existingId == clientId {
				log.Printf("Reannounce from %s", clientId)
			} else {
				log.Printf("Device %s moved port: %s -> %s", ip, existingId, clientId)
			}
			alreadyLogged = true
			break
		}
	}
	if !alreadyLogged {
		log.Printf("New announce from %s", clientId)
	}

	announceTime := time.Now()

	// Trust the announce immediately — devices are reachable when they announce.
	// The health-check loop can mark them unreachable if probes fail later.
	info := &clientInfo{
		lastSeen:    announceTime,
		model:       model,
		maxSize:     maxSize,
		battery:     battery,
		temperature: temperature,
		reachable:   true,
	}

	info.expiryTimer = time.AfterFunc(expiryDuration, func() {
		t.Lock()
		defer t.Unlock()

		// there's a possible race condition if the client announces just as the timer expires,
		// preventing the timer from being stopped. To prevent that, we verify that the last seen time
		// has not been changed.
		if t.RpcServers[clientId] != nil && t.RpcServers[clientId].lastSeen.Equal(announceTime) {
			delete(t.RpcServers, clientId)
			log.Printf("Removed %s from tracker due to expiry", clientId)
		}
	})

	t.RpcServers[clientId] = info

	// respond
	err := json.NewEncoder(w).Encode(response{
		Interval: int(interval.Seconds()),
	})

	if err != nil {
		log.Printf("Failed to respond to announce: %v", err)
	}
}

func (t *Tracker) ListServers(w http.ResponseWriter, r *http.Request) {
	type response struct {
		Servers []string `json:"servers"`
	}

	servers := t.GetServers()

	err := json.NewEncoder(w).Encode(response{
		Servers: servers,
	})

	if err != nil {
		log.Printf("Failed to respond to list servers: %v", err)
	}
}

func (t *Tracker) GetServers() []string {
	t.RLock()
	defer t.RUnlock()

	servers := make([]string, 0, len(t.RpcServers))
	for server, info := range t.RpcServers {
		if info.reachable {
			servers = append(servers, server)
		}
	}

	slices.Sort(servers)

	return servers
}

func (t *Tracker) GetServerDetails() map[string]ClientDetails {
	t.RLock()
	defer t.RUnlock()

	details := make(map[string]ClientDetails, len(t.RpcServers))
	for id, info := range t.RpcServers {
		// Even if not reachable currently, we can still show their details if they haven't expired
		details[id] = ClientDetails{
			Model:       info.model,
			MaxSize:     info.maxSize,
			Battery:     info.battery,
			Temperature: info.temperature,
		}
	}
	return details
}

package tracker

import (
	"encoding/json"
	"log"
	"net/http"
	"slices"
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
	lastSeen    time.Time
	expiryTimer *time.Timer
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

	log.Printf("Announce from %s", r.RemoteAddr)

	// avoid duplicate timers
	if existingTimer := t.RpcServers[clientId].expiryTimer; existingTimer != nil {
		existingTimer.Stop()
	}

	announceTime := time.Now()

	t.RpcServers[clientId] = clientInfo{
		lastSeen: announceTime,
		expiryTimer: time.AfterFunc(expiryDuration, func() {
			t.Lock()
			defer t.Unlock()

			// there's a possible race condition if the client announces just as the timer expires,
			// preventing the timer from being stopped. To prevent that, we verify that the last seen time
			// has not been changed.
			if t.RpcServers[clientId].lastSeen.Equal(announceTime) {
				delete(t.RpcServers, clientId)
				log.Printf("Removed %s from tracker", clientId)
			}
		}),
	}

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

	t.RLock()
	defer t.RUnlock()

	servers := make([]string, 0, len(t.RpcServers))
	for server := range t.RpcServers {
		servers = append(servers, server)
	}

	slices.Sort(servers)

	err := json.NewEncoder(w).Encode(response{
		Servers: servers,
	})

	if err != nil {
		log.Printf("Failed to respond to list servers: %v", err)
	}
}

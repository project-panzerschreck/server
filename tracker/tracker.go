package tracker

import (
	"encoding/json"
	"log"
	"net/http"
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
}

func (t *Tracker) Announce(w http.ResponseWriter, r *http.Request) {
	type response struct {
		Interval int `json:"interval"`
	}

	t.Lock()
	defer t.Unlock()

	// avoid duplicate timers
	if existingTImer := t.RpcServers[r.RemoteAddr].expiryTimer; existingTImer != nil {
		existingTImer.Stop()
	}

	announceTime := time.Now()

	t.RpcServers[r.RemoteAddr] = clientInfo{
		lastSeen: announceTime,
		expiryTimer: time.AfterFunc(expiryDuration, func() {
			t.Lock()
			defer t.Unlock()

			// there's a possible race condition if the client announces just as the timer expires,
			// preventing the timer from being stopped. To prevent that, we verify that the last seen time
			// has not been changed.
			if t.RpcServers[r.RemoteAddr].lastSeen.Equal(announceTime) {
				delete(t.RpcServers, r.RemoteAddr)
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

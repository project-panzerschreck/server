package tracker

import (
	"database/sql"
	"encoding/json"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wk-y/rama-swap/database"
)

// the number of seconds after which an RPC server is removed from the list
const expiryDuration = time.Second * 30

// the number of seconds to wait between announces
const interval = time.Second * 10

type Tracker struct {
	db *sql.DB
}

type clientInfo struct {
	RpcServerInfo
	expiryTimer *time.Timer
}

type RpcServerInfo struct {
	Ip            string    `json:"ip"`
	Port          int       `json:"port"`
	LastSeen      time.Time `json:"last_seen"`
	HardwareModel string    `json:"hardware_model"` // the hardware's model name
	MaxSize       int64     `json:"max_size"`
	Battery       float64   `json:"battery"`
	Temperature   float64   `json:"temperature"`
}

func NewTracker() *Tracker {
	return &Tracker{
		db: database.GetDB(),
	}
}

func (t *Tracker) AddRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/announce", t.Announce)
	mux.HandleFunc("/servers", t.ListServers)
}

func (t *Tracker) Announce(w http.ResponseWriter, r *http.Request) {
	log.Printf("Announce request from %s: %v", r.Host, r.URL)
	type response struct {
		Interval int `json:"interval"`
	}

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

	hardwareModel := r.URL.Query().Get("model")

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

	_, err = t.db.Exec(`INSERT OR REPLACE INTO nodes
(ip, port, last_seen, hardware_model, max_size, battery, temperature)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		ip,
		portNum,
		time.Now(),
		hardwareModel,
		maxSize,
		battery,
		temperature,
	)
	if err != nil {
		log.Printf("Failed to update node: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
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
	now := time.Now()
	expiryCutoff := now.Add(-expiryDuration)

	rows, err := t.db.Query(`SELECT ip, port, last_seen, hardware_model, max_size, ifnull(battery, 'NaN'), ifnull(temperature, 'NaN') FROM nodes WHERE last_seen > ?`, expiryCutoff)
	if err != nil {
		log.Printf("Failed to query nodes: %v", err)
		return nil
	}
	defer rows.Close()

	var servers []RpcServerInfo
	for rows.Next() {
		var server RpcServerInfo
		if err := rows.Scan(&server.Ip, &server.Port, &server.LastSeen, &server.HardwareModel, &server.MaxSize, &server.Battery, &server.Temperature); err != nil {
			log.Printf("Failed to scan node: %v", err)
			continue
		}
		servers = append(servers, server)
	}

	return servers
}

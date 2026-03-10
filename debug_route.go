package main

import (
	"net/http"
	"time"

	"github.com/wk-y/rama-swap/tracker"
)

func addDebugRoute(mux *http.ServeMux, tracker *tracker.Tracker) {
	mux.HandleFunc("/{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Refresh", "5")
		clients := tracker.GetServers()
		t := time.Now()
		debugPage(clients, t).Render(r.Context(), w)
	})
}

package main

import (
	"net/http"

	"github.com/wk-y/rama-swap/tracker"
)

func addDebugRoute(mux *http.ServeMux, tracker *tracker.Tracker) {
	mux.HandleFunc("/{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		clients := tracker.GetServers()
		debugPage(clients).Render(r.Context(), w)
	})
}

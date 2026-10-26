package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"val2harness/internal/config"
	"val2harness/internal/daemon"
)

func registerDaemonRoutes(mux *http.ServeMux, cfg *config.Config) {
	mux.HandleFunc("/api/local/daemon/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		pid, err := daemon.StartDaemon(cfg.ListenAddr)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"status": "ok", "pid": pid})
	})

	mux.HandleFunc("/api/local/daemon/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		err := daemon.StopDaemon(cfg.ListenAddr)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	})

	mux.HandleFunc("/api/local/port", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Addr string `json:"addr"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Addr) == "" {
			http.Error(w, "Invalid address", http.StatusBadRequest)
			return
		}
		cfg.ListenAddr = strings.TrimSpace(req.Addr)
		_ = cfg.Save()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"status": "ok", "addr": cfg.ListenAddr})
	})
}

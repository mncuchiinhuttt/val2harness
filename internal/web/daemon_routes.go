package web

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"

	"val2harness/internal/config"
	"val2harness/internal/configfile"
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
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
			http.Error(w, "Invalid address", http.StatusBadRequest)
			return
		}
		addr := strings.TrimSpace(req.Addr)
		host, portText, err := net.SplitHostPort(addr)
		port, portErr := strconv.Atoi(portText)
		if err != nil || portErr != nil || port < 1 || port > 65535 || (host != "" && host != "localhost" && net.ParseIP(host) == nil) {
			http.Error(w, "Use an IP address or localhost and a port from 1 to 65535", http.StatusBadRequest)
			return
		}
		path, err := config.GetConfigPath()
		if err == nil {
			err = configfile.Update(path, func(data []byte) ([]byte, error) {
				values, err := configfile.JSONObject(data)
				if err != nil {
					return nil, err
				}
				values["listen_addr"] = addr
				return json.MarshalIndent(values, "", "  ")
			})
		}
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]any{"error": "Could not save the listen address. The running gateway is unchanged."})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"status": "ok", "addr": addr, "requires_restart": addr != cfg.ListenAddr})
	})
}

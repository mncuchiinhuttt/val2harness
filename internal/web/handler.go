package web

import (
	"embed"
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"val2harness/internal/config"
	"val2harness/internal/daemon"
	"val2harness/internal/exporter"
	"val2harness/internal/omp"
	valSync "val2harness/internal/sync"
	"val2harness/internal/valclient"
)

//go:embed static/*
var staticFS embed.FS

func RegisterWebRoutes(mux *http.ServeMux, cfg *config.Config, client *valclient.Client, onTokenUpdate func(string)) {
	subFS, err := fs.Sub(staticFS, "static")
	if err == nil {
		mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(subFS))))
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		data, err := staticFS.ReadFile("static/index.html")
		if err != nil {
			http.Error(w, "index.html not found", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})

	mux.HandleFunc("/api/local/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		token := client.GetToken()
		var exp int64 = 0
		account := ""
		if token != "" {
			parts := strings.Split(token, ".")
			if len(parts) >= 2 {
				if decoded, err := base64.RawURLEncoding.DecodeString(parts[1]); err == nil {
					var payload map[string]any
					if json.Unmarshal(decoded, &payload) == nil {
						if expFloat, ok := payload["exp"].(float64); ok {
							exp = int64(expFloat)
						}
						if sub, ok := payload["sub"].(string); ok {
							account = sub
						}
					}
				}
			}
		}

		isExpired := exp > 0 && time.Now().Unix() > exp
		running, pid := daemon.GetStatus(cfg.ListenAddr)
		json.NewEncoder(w).Encode(map[string]any{
			"token_configured": token != "" && !isExpired,
			"token_exp":        exp,
			"is_expired":       isExpired,
			"account":          account,
			"upstream":         cfg.UpstreamURL,
			"listen":           cfg.ListenAddr,
			"daemon_running":   running,
			"daemon_pid":       pid,
		})
	})

	mux.HandleFunc("/api/local/login", func(w http.ResponseWriter, r *http.Request) {
		go func() {
			var cmd *exec.Cmd
			switch runtime.GOOS {
			case "darwin":
				cmd = exec.Command("open", cfg.UpstreamURL)
			case "windows":
				cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", cfg.UpstreamURL)
			default:
				cmd = exec.Command("xdg-open", cfg.UpstreamURL)
			}
			_ = cmd.Start()
		}()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"status": "ok", "url": cfg.UpstreamURL})
	})

	mux.HandleFunc("/api/local/logout", func(w http.ResponseWriter, r *http.Request) {
		cfg.Token = ""
		_ = cfg.Save()
		client.SetToken("")
		if onTokenUpdate != nil {
			onTokenUpdate("")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	})

	mux.HandleFunc("/api/local/token", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
			http.Error(w, "Invalid token", http.StatusBadRequest)
			return
		}

		cfg.Token = strings.TrimSpace(req.Token)
		_ = cfg.Save()
		client.SetToken(cfg.Token)
		if onTokenUpdate != nil {
			onTokenUpdate(cfg.Token)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	})

	mux.HandleFunc("/api/local/sync-omp", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		proxyBaseURL := "http://" + cfg.ListenAddr
		count, path, err := omp.SyncModels(r.Context(), client, proxyBaseURL, cfg.APIKey)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"count":  count,
			"path":   path,
		})
	})
	mux.HandleFunc("/api/local/sync-all", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		proxyBaseURL := "http://" + cfg.ListenAddr
		results := valSync.SyncAll(r.Context(), client, proxyBaseURL, cfg.APIKey)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status":  "ok",
			"results": results,
		})
	})

	mux.HandleFunc("/api/local/env", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(valSync.GenerateEnvSnippet(cfg.ListenAddr)))
	})


	mux.HandleFunc("/api/local/export", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		count, outDir, err := exporter.ExportAllChats(r.Context(), client, "exports")
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"count":  count,
			"dir":    outDir,
		})
	})
}

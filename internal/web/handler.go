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
	"sync"
	"time"

	"val2harness/internal/config"
	"val2harness/internal/daemon"
	"val2harness/internal/valclient"
)

//go:embed static/*
var staticFS embed.FS

func RegisterWebRoutes(mux *http.ServeMux, cfg *config.Config, client *valclient.Client, onTokenUpdate func(string)) {
	var tokenUpdateMu sync.Mutex
	persistToken := func(token string) error {
		tokenUpdateMu.Lock()
		defer tokenUpdateMu.Unlock()
		next := *cfg
		next.Token = token
		if err := next.Save(); err != nil {
			return err
		}
		if onTokenUpdate != nil {
			onTokenUpdate(token)
		} else {
			client.SetToken(token)
		}
		return nil
	}
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
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", 405)
			return
		}
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
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", 405)
			return
		}
		if err := persistToken(""); err != nil {
			http.Error(w, "Could not save account settings", 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	})

	tokenHandler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536)).Decode(&req); err != nil || strings.TrimSpace(req.Token) == "" {
			http.Error(w, "Invalid token", http.StatusBadRequest)
			return
		}

		if err := persistToken(strings.TrimSpace(req.Token)); err != nil {
			http.Error(w, "Could not save account settings", 500)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	}
	mux.HandleFunc("/api/local/token", tokenHandler)
	mux.HandleFunc("/api/local/import-token", tokenHandler)

	registerHarnessRoutes(mux, cfg, client)
}

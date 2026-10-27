package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"val2harness/internal/config"
	"val2harness/internal/exporter"
	valSync "val2harness/internal/sync"
	"val2harness/internal/valclient"
)

func registerHarnessRoutes(mux *http.ServeMux, cfg *config.Config, client *valclient.Client) {
	mux.HandleFunc("/api/local/harnesses", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		list := []map[string]any{
			{"id": "omp", "name": "OMP (~/.omp/models.yml)", "has_backup": valSync.HasBackup("omp")},
			{"id": "pi", "name": "Pi Harness (~/.pi/models.json)", "has_backup": valSync.HasBackup("pi")},
			{"id": "opencode", "name": "OpenCode (~/.config/opencode/opencode.json)", "has_backup": valSync.HasBackup("opencode")},
			{"id": "continue", "name": "Continue (~/.continue/config.yaml)", "has_backup": valSync.HasBackup("continue")},
		}
		json.NewEncoder(w).Encode(list)
	})

	mux.HandleFunc("/api/local/sync", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Harness string `json:"harness"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		proxyBaseURL := config.LocalBaseURL(cfg.ListenAddr)
		var count int
		var path string
		var err error
		switch strings.ToLower(req.Harness) {
		case "omp":
			count, path, err = valSync.SyncOMP(r.Context(), client, proxyBaseURL, cfg.APIKey)
		case "pi":
			count, path, err = valSync.SyncPi(r.Context(), client, proxyBaseURL, cfg.APIKey)
		case "opencode":
			count, path, err = valSync.SyncOpenCode(r.Context(), client, proxyBaseURL, cfg.APIKey)
		case "continue":
			count, path, err = valSync.SyncContinue(r.Context(), client, proxyBaseURL, cfg.APIKey)
		default:
			http.Error(w, "Invalid harness", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"status": "ok", "count": count, "path": path})
	})

	mux.HandleFunc("/api/local/restore", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Harness string `json:"harness"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		path, err := valSync.RestoreHarness(req.Harness)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"status": "ok", "path": path})
	})

	mux.HandleFunc("/api/local/env", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(valSync.GenerateEnvSnippet(cfg.ListenAddr, cfg.APIKey)))
	})

	mux.HandleFunc("/api/local/export", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		outDir, err := exporter.NewRunDirectory("exports")
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(500)
			json.NewEncoder(w).Encode(map[string]any{"status": "error", "count": 0, "dir": "exports", "error": "Could not create a private export folder: " + err.Error()})
			return
		}
		count, outDir, err := exporter.ExportAllChats(r.Context(), client, outDir)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			status := "error"
			if count > 0 {
				status = "partial"
			}
			json.NewEncoder(w).Encode(map[string]any{"status": status, "count": count, "dir": outDir, "error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"status": "ok", "count": count, "dir": outDir})
	})
}

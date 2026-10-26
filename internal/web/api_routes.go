package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"val2harness/internal/analytics"
	"val2harness/internal/config"
	"val2harness/internal/exporter"
	valSync "val2harness/internal/sync"
	"val2harness/internal/valclient"
	"val2harness/internal/version"
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
		proxyBaseURL := "http://" + cfg.ListenAddr
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
		json.NewEncoder(w).Encode(map[string]any{"status": "ok", "count": count, "dir": outDir})
	})
	mux.HandleFunc("/api/local/analytics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		summary := analytics.GetTracker().GetSummary()
		json.NewEncoder(w).Encode(summary)
	})
	mux.HandleFunc("/api/local/analytics/live", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}
		tracker := analytics.GetTracker()
		ch := tracker.Subscribe()
		defer tracker.Unsubscribe(ch)

		initBytes, _ := json.Marshal(tracker.GetSummary())
		fmt.Fprintf(w, "data: %s\n\n", string(initBytes))
		flusher.Flush()

		for {
			select {
			case <-r.Context().Done():
				return
			case summary, ok := <-ch:
				if !ok {
					return
				}
				data, _ := json.Marshal(summary)
				fmt.Fprintf(w, "data: %s\n\n", string(data))
				flusher.Flush()
			}
		}
	})
	mux.HandleFunc("/api/local/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		info, err := version.CheckUpdate(r.Context())
		if err != nil {
			json.NewEncoder(w).Encode(map[string]any{"current_version": version.CurrentVersion, "has_update": false, "error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(info)
	})

	mux.HandleFunc("/api/local/update", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		v, err := version.SelfUpdate(r.Context())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"status": "ok", "version": v})
	})
}

package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"val2harness/internal/analytics"
)

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":{"message":"Method not allowed"}}`, http.StatusMethodNotAllowed)
		return
	}

	if s.client.GetToken() == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": "RMIT Val token is not configured. Please login or set token.",
				"type":    "authentication_error",
			},
		})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 15<<20)
	start := time.Now()
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":{"message":"Bad Request"}}`, http.StatusBadRequest)
		return
	}

	var reqPreview struct {
		Model string `json:"model"`
	}
	_ = json.Unmarshal(bodyBytes, &reqPreview)
	if reqPreview.Model == "" {
		reqPreview.Model = "anthropic-claude-opus-4-7"
	}

	upstreamResp, err := s.client.ProxyChat(r.Context(), bytes.NewReader(bodyBytes))
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": "Upstream error: " + err.Error(),
				"type":    "upstream_error",
			},
		})
		return
	}
	defer upstreamResp.Body.Close()

	contentType := upstreamResp.Header.Get("Content-Type")
	isStreaming := strings.Contains(contentType, "text/event-stream")

	// If upstream returned JSON (error or non-streaming)
	if !isStreaming {
		respBody, readErr := io.ReadAll(upstreamResp.Body)
		if readErr == nil {
			var errCheck struct {
				Error *struct {
					Message string `json:"message"`
					Code    string `json:"code"`
				} `json:"error"`
			}
			if json.Unmarshal(respBody, &errCheck) == nil && errCheck.Error != nil {
				w.Header().Set("Content-Type", "application/json")
				statusCode := http.StatusTooManyRequests
				if errCheck.Error.Code != "budget_exceeded" {
					statusCode = http.StatusBadRequest
				}
				w.WriteHeader(statusCode)
				w.Write(respBody)
				return
			}
			w.Header().Set("Content-Type", contentType)
			w.WriteHeader(upstreamResp.StatusCode)
			w.Write(respBody)
			return
		}
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(upstreamResp.StatusCode)
	flusher, canFlush := w.(http.Flusher)
	buf := make([]byte, 4096)
	for {
		n, readErr := upstreamResp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				return
			}
			if canFlush {
				flusher.Flush()
			}
		}
		if readErr != nil {
			break
		}
	}

	latency := time.Since(start).Milliseconds()
	analytics.GetTracker().Record(reqPreview.Model, "openai", len(bodyBytes)/4, 40, latency, upstreamResp.StatusCode)
}

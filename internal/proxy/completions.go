package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
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

	upstreamResp, err := s.client.ProxyChat(r.Context(), r.Body)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
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

	// Copy upstream response headers
	contentType := upstreamResp.Header.Get("Content-Type")
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	if cc := upstreamResp.Header.Get("Cache-Control"); cc != "" {
		w.Header().Set("Cache-Control", cc)
	}

	isStreaming := strings.Contains(contentType, "text/event-stream")
	if isStreaming {
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
	}

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
			if readErr != io.EOF {
				// Client or upstream closed connection
			}
			break
		}
	}
}

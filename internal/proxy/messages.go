package proxy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"val2harness/internal/analytics"
)

type anthropicMsg struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type anthropicReq struct {
	Model       string         `json:"model"`
	Messages    []anthropicMsg `json:"messages"`
	System      any            `json:"system,omitempty"`
	MaxTokens   int            `json:"max_tokens,omitempty"`
	Stream      bool           `json:"stream,omitempty"`
	Temperature *float64       `json:"temperature,omitempty"`
}

func mapAnthropicModel(m string) string {
	low := strings.ToLower(m)
	switch {
	case strings.Contains(low, "opus-4-7") || strings.Contains(low, "opus-4.7"):
		return "anthropic-claude-opus-4-7"
	case strings.Contains(low, "opus-5") || strings.Contains(low, "opus"):
		return "anthropic-claude-opus-5"
	case strings.Contains(low, "sonnet-5"):
		return "anthropic-claude-sonnet-5"
	case strings.Contains(low, "sonnet"):
		return "anthropic-claude-sonnet-4-6"
	case strings.Contains(low, "haiku"):
		return "anthropic-claude-haiku-4-5"
	default:
		return "anthropic-claude-opus-4-7"
	}
}

func extractText(val any) string {
	switch v := val.(type) {
	case string:
		return v
	case []any:
		var sb strings.Builder
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if m["type"] == "text" {
					if t, ok := m["text"].(string); ok {
						sb.WriteString(t)
					}
				}
			}
		}
		return sb.String()
	default:
		return ""
	}
}

func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":{"type":"invalid_request_error","message":"Method not allowed"}}`, http.StatusMethodNotAllowed)
		return
	}
	start := time.Now()
	var req anthropicReq
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil || json.Unmarshal(bodyBytes, &req) != nil {
		http.Error(w, `{"error":{"type":"invalid_request_error","message":"Invalid JSON payload"}}`, http.StatusBadRequest)
		return
	}

	upstreamModel := mapAnthropicModel(req.Model)
	var openAIMessages []map[string]any

	if sysText := extractText(req.System); sysText != "" {
		openAIMessages = append(openAIMessages, map[string]any{
			"role":    "system",
			"content": sysText,
		})
	}

	for _, msg := range req.Messages {
		openAIMessages = append(openAIMessages, map[string]any{
			"role":    msg.Role,
			"content": extractText(msg.Content),
		})
	}

	valReq := map[string]any{
		"model":    upstreamModel,
		"messages": openAIMessages,
		"stream":   req.Stream,
	}
	if req.Temperature != nil {
		valReq["temperature"] = *req.Temperature
	}
	valJSON, _ := json.Marshal(valReq)

	upstreamResp, err := s.client.ProxyChat(r.Context(), bytes.NewReader(valJSON))
	if err != nil {
		http.Error(w, `{"error":{"type":"api_error","message":"Upstream error"}}`, http.StatusBadGateway)
		return
	}
	defer upstreamResp.Body.Close()

	if !req.Stream {
		var oaiResp struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		_ = json.NewDecoder(upstreamResp.Body).Decode(&oaiResp)
		text := ""
		if len(oaiResp.Choices) > 0 {
			text = oaiResp.Choices[0].Message.Content
		}

		w.Header().Set("Content-Type", "application/json")
		outTok := len(text) / 4
		analytics.GetTracker().Record(upstreamModel, "anthropic", len(bodyBytes)/4, outTok, time.Since(start).Milliseconds(), upstreamResp.StatusCode)
		json.NewEncoder(w).Encode(map[string]any{
			"id": fmt.Sprintf("msg_%d", time.Now().UnixNano()), "type": "message", "role": "assistant",
			"model": req.Model, "content": []map[string]any{{"type": "text", "text": text}},
			"stop_reason": "end_turn", "usage": map[string]int{"input_tokens": len(bodyBytes) / 4, "output_tokens": outTok},
		})
		return
	}

	// Streaming SSE translation
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, _ := w.(http.Flusher)

	msgID := fmt.Sprintf("msg_%d", time.Now().UnixNano())
	fmt.Fprintf(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"%s\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"%s\",\"usage\":{\"input_tokens\":0,\"output_tokens\":0}}}\n\n", msgID, req.Model)
	fmt.Fprintf(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
	if flusher != nil {
		flusher.Flush()
	}

	scanner := bufio.NewScanner(upstreamResp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(trimmed, "data: ")
		if payload == "[DONE]" {
			break
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(payload), &chunk) == nil && len(chunk.Choices) > 0 {
			delta := chunk.Choices[0].Delta.Content
			if delta != "" {
				deltaJSON, _ := json.Marshal(map[string]any{
					"type":  "content_block_delta",
					"index": 0,
					"delta": map[string]string{"type": "text_delta", "text": delta},
				})
				fmt.Fprintf(w, "event: content_block_delta\ndata: %s\n\n", string(deltaJSON))
				if flusher != nil {
					flusher.Flush()
				}
			}
		}
	}

	fmt.Fprintf(w, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
	fmt.Fprintf(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\",\"stop_sequence\":null},\"usage\":{\"output_tokens\":10}}\n\n")
	fmt.Fprintf(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	if flusher != nil {
		flusher.Flush()
	}
	analytics.GetTracker().Record(upstreamModel, "anthropic", len(bodyBytes)/4, 60, time.Since(start).Milliseconds(), upstreamResp.StatusCode)
}

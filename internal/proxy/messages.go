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
	Model       string          `json:"model"`
	Messages    []anthropicMsg  `json:"messages"`
	System      any             `json:"system,omitempty"`
	Tools       []anthropicTool `json:"tools,omitempty"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
	Temperature *float64        `json:"temperature,omitempty"`
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
	openAIMessages := convertMessagesToOpenAI(req.System, req.Messages)

	valReq := map[string]any{
		"model":    upstreamModel,
		"messages": openAIMessages,
		"stream":   req.Stream,
	}
	if len(req.Tools) > 0 {
		valReq["tools"] = convertToolsToOpenAI(req.Tools)
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

	if upstreamResp.StatusCode != http.StatusOK {
		errBytes, _ := io.ReadAll(upstreamResp.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(upstreamResp.StatusCode)
		w.Write(formatAnthropicError(upstreamResp.StatusCode, string(errBytes)))
		return
	}

	if !req.Stream {
		var oaiResp struct {
			Choices []struct {
				Message struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		}
		_ = json.NewDecoder(upstreamResp.Body).Decode(&oaiResp)
		var contentBlocks []map[string]any
		stopReason := "end_turn"

		if len(oaiResp.Choices) > 0 {
			ch := oaiResp.Choices[0]
			if ch.Message.Content != "" {
				contentBlocks = append(contentBlocks, map[string]any{"type": "text", "text": ch.Message.Content})
			}
			for _, tc := range ch.Message.ToolCalls {
				var argsObj any
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &argsObj)
				contentBlocks = append(contentBlocks, map[string]any{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Function.Name,
					"input": argsObj,
				})
			}
			if len(ch.Message.ToolCalls) > 0 {
				stopReason = "tool_use"
			}
		}

		outTok := 50
		analytics.GetTracker().Record(upstreamModel, "anthropic", len(bodyBytes)/4, outTok, time.Since(start).Milliseconds(), upstreamResp.StatusCode)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":          fmt.Sprintf("msg_%d", time.Now().UnixNano()),
			"type":        "message",
			"role":        "assistant",
			"model":       req.Model,
			"content":     contentBlocks,
			"stop_reason": stopReason,
			"usage":       map[string]int{"input_tokens": len(bodyBytes) / 4, "output_tokens": outTok},
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

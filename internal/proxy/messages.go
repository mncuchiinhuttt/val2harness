package proxy

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"val2harness/internal/analytics"
	"val2harness/internal/valclient"
)

type anthropicMsg struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}
type anthropicReq struct {
	Model         string               `json:"model"`
	Messages      []anthropicMsg       `json:"messages"`
	System        any                  `json:"system,omitempty"`
	MaxTokens     int                  `json:"max_tokens"`
	Stream        bool                 `json:"stream,omitempty"`
	Temperature   *float64             `json:"temperature,omitempty"`
	TopP          *float64             `json:"top_p,omitempty"`
	TopK          *int                 `json:"top_k,omitempty"`
	Tools         []anthropicTool      `json:"tools,omitempty"`
	ToolChoice    *anthropicToolChoice `json:"tool_choice,omitempty"`
	StopSequences []string             `json:"stop_sequences,omitempty"`
	Metadata      map[string]any       `json:"metadata,omitempty"`
	Thinking      map[string]any       `json:"thinking,omitempty"`
	CacheControl  map[string]any       `json:"cache_control,omitempty"`
	OutputConfig  map[string]any       `json:"output_config,omitempty"`
	Compatibility []string             `json:"-"`
	toolNames     map[string]string
}

func mapAnthropicModel(model string) string {
	aliases := map[string]string{
		"claude-opus-4-7":   "anthropic-claude-opus-4-7",
		"claude-opus-4.7":   "anthropic-claude-opus-4-7",
		"claude-opus-5":     "anthropic-claude-opus-5",
		"claude-sonnet-4-6": "anthropic-claude-sonnet-4-6",
		"claude-sonnet-4.6": "anthropic-claude-sonnet-4-6",
		"claude-sonnet-5":   "anthropic-claude-sonnet-5",
		"claude-haiku-4-5":  "anthropic-claude-haiku-4-5",
		"claude-haiku-4.5":  "anthropic-claude-haiku-4-5",
	}
	if id, ok := aliases[model]; ok {
		return id
	}
	return model
}

func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		messagesError(w, 405, "invalid_request_error", "Method not allowed")
		return
	}
	if s.client.GetToken() == "" {
		messagesError(w, 401, "authentication_error", "RMIT Val token is not configured. Sign in or set your Val token.")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxMessageBytes))
	if err != nil {
		messagesError(w, 413, "request_too_large", "Messages request could not be read within the 8 MiB limit")
		return
	}
	req, valJSON, err := convertMessagesRequest(body)
	if err != nil {
		messagesError(w, 400, "invalid_request_error", err.Error())
		return
	}
	if len(req.Compatibility) > 0 {
		w.Header().Set("X-Val2harness-Compatibility", strings.Join(req.Compatibility, ", "))
	}
	upstreamResp, err := s.client.ProxyChat(analytics.WithProtocol(r.Context(), "anthropic"), bytes.NewReader(valJSON))
	if err != nil {
		if errors.Is(err, valclient.ErrChatRequestTooLarge) {
			messagesError(w, 413, "request_too_large", err.Error())
			return
		}
		messagesError(w, 502, "api_error", "Could not reach Val")
		return
	}
	defer upstreamResp.Body.Close()
	if upstreamResp.StatusCode < 200 || upstreamResp.StatusCode >= 300 {
		status := upstreamResp.StatusCode
		if status < 400 {
			status = 502
		}
		if retry := upstreamResp.Header.Get("Retry-After"); retry != "" {
			w.Header().Set("Retry-After", retry)
		}
		messagesError(w, status, upstreamErrorType(status), fmt.Sprintf("Val returned HTTP %d. Check your account or retry as appropriate.", upstreamResp.StatusCode))
		return
	}
	if !req.Stream {
		writeMessagesReply(w, upstreamResp, req)
		return
	}
	s.streamMessages(w, r, upstreamResp, req)
}

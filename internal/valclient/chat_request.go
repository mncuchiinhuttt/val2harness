package valclient

import (
	"encoding/json"
	"errors"
	"io"
)

// MaxChatRequestBytes bounds both buffered input and the adapted Val payload.
// Anthropic Messages also enforces its smaller, protocol-specific input limit.
const MaxChatRequestBytes = 32 << 20

var ErrChatRequestTooLarge = errors.New("chat request exceeds the 32 MiB limit, including client-tool registration")

func prepareChatRequest(body io.Reader) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	data, err := io.ReadAll(io.LimitReader(body, MaxChatRequestBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxChatRequestBytes {
		return nil, ErrChatRequestTooLarge
	}
	var request map[string]json.RawMessage
	if json.Unmarshal(data, &request) != nil || request == nil {
		return data, nil // Leave upstream validation and unknown protocols intact.
	}
	if _, explicit := request["tool_servers"]; explicit {
		return data, nil
	}
	var tools []struct {
		Type     string          `json:"type"`
		Function json.RawMessage `json:"function"`
	}
	if json.Unmarshal(request["tools"], &tools) != nil || len(tools) == 0 {
		return data, nil
	}
	functions := make([]json.RawMessage, 0, len(tools))
	for _, tool := range tools {
		var function map[string]json.RawMessage
		var name string
		if tool.Type != "function" || json.Unmarshal(tool.Function, &function) != nil || function == nil || json.Unmarshal(function["name"], &name) != nil || name == "" {
			return data, nil // Do not partially adapt custom or malformed tools.
		}
		functions = append(functions, tool.Function)
	}
	// Val's native pipeline resolves caller-owned schemas separately from
	// OpenAI tools. No endpoint, credentials or execution state is added.
	registry, err := json.Marshal([]map[string]any{{"specs": functions}})
	if err != nil {
		return nil, err
	}
	request["tool_servers"] = registry
	adapted, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	if len(adapted) > MaxChatRequestBytes {
		return nil, ErrChatRequestTooLarge
	}
	return adapted, nil
}

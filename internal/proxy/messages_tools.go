package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
)

const maxToolArgumentBytes = 1 << 20

var errUndeclaredUpstreamTool = errors.New("upstream called a tool not declared in this request")

func upstreamToolErrorMessage(err error) string {
	if errors.Is(err, errUndeclaredUpstreamTool) {
		return "Val returned tool calls not declared by this client. Check upstream tool injection or select a model that preserves client tools."
	}
	return "Val returned invalid tool calls or content"
}

type openAIToolCall struct {
	Index    *int   `json:"index,omitempty"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function"`
}

func toolInput(arguments string) (map[string]any, error) {
	if len(arguments) > maxToolArgumentBytes {
		return nil, fmt.Errorf("tool arguments exceed 1 MiB")
	}
	var input map[string]any
	if err := decodeOneJSON([]byte(arguments), &input); err != nil || input == nil {
		return nil, fmt.Errorf("tool arguments must be one JSON object")
	}
	return input, nil
}
func responseToolCalls(raw json.RawMessage, req anthropicReq) ([]any, error) {
	if !hasJSONValue(raw) {
		return nil, nil
	}
	var calls []openAIToolCall
	if err := json.Unmarshal(raw, &calls); err != nil || len(calls) > 128 {
		return nil, fmt.Errorf("invalid upstream tool calls")
	}
	blocks := make([]any, 0, len(calls))
	ids := map[string]bool{}
	for _, call := range calls {
		if !toolIDPattern.MatchString(call.ID) || ids[call.ID] || call.Type != "function" || !toolNamePattern.MatchString(call.Function.Name) {
			return nil, fmt.Errorf("invalid tool identity")
		}
		ids[call.ID] = true
		if _, ok := req.toolNames[call.Function.Name]; !ok {
			return nil, errUndeclaredUpstreamTool
		}
		input, err := toolInput(call.Function.Arguments)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, map[string]any{"type": "tool_use", "id": call.ID, "name": req.originalToolName(call.Function.Name), "input": input})
	}
	return blocks, nil
}

func validateToolChoice(req anthropicReq, calls []any, stop string) error {
	choice := req.ToolChoice
	if choice == nil {
		return nil
	}
	if choice.DisableParallel != nil && *choice.DisableParallel && len(calls) > 1 {
		return fmt.Errorf("upstream violated single-tool selection")
	}
	switch choice.Type {
	case "none":
		if len(calls) > 0 {
			return fmt.Errorf("upstream called tools when tool_choice is none")
		}
	case "tool":
		for _, raw := range calls {
			if raw.(map[string]any)["name"] != choice.Name {
				return fmt.Errorf("upstream selected a different tool")
			}
		}
		fallthrough
	case "any":
		if len(calls) == 0 && stop != "max_tokens" && stop != "refusal" {
			return fmt.Errorf("upstream omitted the required tool call")
		}
	}
	return nil
}

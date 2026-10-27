package proxy

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

type anthropicTool struct {
	Name                string         `json:"name"`
	Description         string         `json:"description,omitempty"`
	InputSchema         map[string]any `json:"input_schema"`
	Type                string         `json:"type,omitempty"`
	CacheControl        map[string]any `json:"cache_control,omitempty"`
	Strict              *bool          `json:"strict,omitempty"`
	DeferLoading        bool           `json:"defer_loading,omitempty"`
	EagerInputStreaming bool           `json:"eager_input_streaming,omitempty"`
}
type anthropicToolChoice struct {
	Type            string `json:"type"`
	Name            string `json:"name,omitempty"`
	DisableParallel *bool  `json:"disable_parallel_tool_use,omitempty"`
}

var toolNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)
var toolIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,256}$`)

func upstreamToolName(name string) string {
	if len(name) <= 64 {
		return name
	}
	return fmt.Sprintf("v2h_%x", sha256.Sum256([]byte(name)))[:64]
}
func (r *anthropicReq) originalToolName(name string) string {
	if original, ok := r.toolNames[name]; ok {
		return original
	}
	return name
}
func (r *anthropicReq) warn(hint string) {
	for _, old := range r.Compatibility {
		if old == hint {
			return
		}
	}
	r.Compatibility = append(r.Compatibility, hint)
}

func (r *anthropicReq) cacheHint(value any) error {
	if value == nil {
		return nil
	}
	hint, ok := value.(map[string]any)
	if ok && hint == nil {
		return nil
	}
	if !ok || hint["type"] != "ephemeral" {
		return fmt.Errorf("cache_control must be an ephemeral hint")
	}
	for key, v := range hint {
		if key == "type" {
			continue
		}
		if key != "ttl" || (v != "5m" && v != "1h") {
			return fmt.Errorf("unsupported cache_control option")
		}
	}
	r.warn("prompt-cache-hints-not-forwarded")
	return nil
}

func convertMessagesRequest(data []byte) (anthropicReq, []byte, error) {
	var req anthropicReq
	var fields map[string]json.RawMessage
	if err := decodeOneJSON(data, &fields); err != nil || fields == nil {
		return req, nil, fmt.Errorf("invalid JSON object")
	}
	allowed := map[string]bool{"model": true, "messages": true, "system": true, "max_tokens": true, "stream": true, "temperature": true, "top_p": true, "top_k": true, "tools": true, "tool_choice": true, "stop_sequences": true, "metadata": true, "thinking": true, "cache_control": true, "output_config": true}
	for key := range fields {
		if !allowed[key] {
			return req, nil, fmt.Errorf("unsupported Messages option %q", key)
		}
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	d.DisallowUnknownFields()
	if err := d.Decode(&req); err != nil {
		return req, nil, fmt.Errorf("invalid Messages field types")
	}
	if strings.TrimSpace(req.Model) == "" || req.MaxTokens <= 0 || len(req.Messages) == 0 {
		return req, nil, fmt.Errorf("model, nonempty messages, and a positive max_tokens are required")
	}
	if req.Temperature != nil && (*req.Temperature < 0 || *req.Temperature > 1) {
		return req, nil, fmt.Errorf("temperature must be between 0 and 1")
	}
	if req.TopP != nil && (*req.TopP < 0 || *req.TopP > 1) {
		return req, nil, fmt.Errorf("top_p must be between 0 and 1")
	}
	if req.TopK != nil && *req.TopK < 0 {
		return req, nil, fmt.Errorf("top_k must be nonnegative")
	}
	if len(req.StopSequences) > 4 {
		return req, nil, fmt.Errorf("the OpenAI upstream supports at most four stop sequences")
	}
	for _, stop := range req.StopSequences {
		if stop == "" {
			return req, nil, fmt.Errorf("stop sequences cannot be empty")
		}
	}
	if len(req.Thinking) > 0 {
		if len(req.Thinking) != 1 || req.Thinking["type"] != "disabled" {
			return req, nil, fmt.Errorf("extended/adaptive thinking requires native upstream support; use thinking.type=disabled for the OpenAI adapter")
		}
	}
	if req.Metadata != nil {
		for key, value := range req.Metadata {
			if key != "user_id" {
				return req, nil, fmt.Errorf("unsupported client metadata field")
			}
			if _, ok := value.(string); !ok {
				return req, nil, fmt.Errorf("metadata.user_id must be a string")
			}
		}
		req.warn("client-metadata-not-forwarded")
	}
	if err := req.cacheHint(req.CacheControl); err != nil {
		return req, nil, err
	}
	upstream := map[string]any{"model": mapAnthropicModel(req.Model), "max_tokens": req.MaxTokens, "stream": req.Stream}
	for key, value := range req.OutputConfig {
		if value == nil {
			continue
		}
		switch key {
		case "effort":
			effort, ok := value.(string)
			if !ok {
				return req, nil, fmt.Errorf("effort must be a string")
			}
			switch effort {
			case "low", "medium", "high", "xhigh", "max":
			default:
				return req, nil, fmt.Errorf("unsupported effort")
			}
			// Anthropic effort controls the whole response, including when
			// thinking is disabled. OpenAI reasoning_effort can enable unsigned
			// thinking that this adapter cannot represent. Disclose the hint
			// instead of changing the client's thinking mode.
			req.warn("response-effort-hint-not-forwarded")
		case "format":
			format, ok := value.(map[string]any)
			if !ok || format["type"] != "json_schema" {
				return req, nil, fmt.Errorf("output format must be json_schema")
			}
			if err := fieldsOnly(format, "type", "schema"); err != nil {
				return req, nil, err
			}
			schema, ok := format["schema"].(map[string]any)
			if !ok {
				return req, nil, fmt.Errorf("JSON output requires an object schema")
			}
			upstream["response_format"] = map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "val2harness_response", "schema": schema, "strict": true}}
		default:
			return req, nil, fmt.Errorf("unsupported output_config option")
		}
	}
	if req.Temperature != nil {
		upstream["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		upstream["top_p"] = *req.TopP
	}
	if req.TopK != nil {
		upstream["top_k"] = *req.TopK
	}
	if len(req.StopSequences) > 0 {
		upstream["stop"] = req.StopSequences
		req.warn("matched-stop-sequence-metadata-upstream-dependent")
	}
	if req.Stream {
		upstream["stream_options"] = map[string]bool{"include_usage": true}
	}
	if err := req.convertTools(upstream); err != nil {
		return req, nil, err
	}
	messages, err := req.convertHistory()
	if err != nil {
		return req, nil, err
	}
	upstream["messages"] = messages
	body, err := json.Marshal(upstream)
	return req, body, err
}

func (r *anthropicReq) convertTools(out map[string]any) error {
	if len(r.Tools) > 128 {
		return fmt.Errorf("at most 128 tools are supported")
	}
	r.toolNames = map[string]string{}
	known := map[string]bool{}
	tools := []any{}
	for _, tool := range r.Tools {
		if !toolNamePattern.MatchString(tool.Name) || known[tool.Name] {
			return fmt.Errorf("tool names must be unique and use letters, numbers, underscores or hyphens")
		}
		known[tool.Name] = true
		if tool.InputSchema == nil || tool.InputSchema["type"] != "object" || (tool.Type != "" && tool.Type != "custom") || tool.DeferLoading {
			return fmt.Errorf("only explicit client tools with object input_schema are supported; server/deferred tools require native services")
		}
		if err := r.cacheHint(tool.CacheControl); err != nil {
			return err
		}
		name := upstreamToolName(tool.Name)
		if old, exists := r.toolNames[name]; exists && old != tool.Name {
			return fmt.Errorf("tool names collide after upstream mapping")
		}
		r.toolNames[name] = tool.Name
		function := map[string]any{"name": name, "description": tool.Description, "parameters": tool.InputSchema}
		if tool.Strict != nil {
			function["strict"] = *tool.Strict
		}
		if tool.EagerInputStreaming {
			r.warn("eager-input-streaming-hint-not-forwarded")
		}
		tools = append(tools, map[string]any{"type": "function", "function": function})
	}
	if len(tools) > 0 {
		out["tools"] = tools
	}
	if choice := r.ToolChoice; choice != nil {
		if choice.Type != "tool" && choice.Name != "" {
			return fmt.Errorf("tool_choice.name is only valid for a specific tool")
		}
		switch choice.Type {
		case "auto", "none":
			out["tool_choice"] = choice.Type
		case "any":
			if len(tools) == 0 {
				return fmt.Errorf("tool_choice any requires tools")
			}
			out["tool_choice"] = "required"
		case "tool":
			if !known[choice.Name] {
				return fmt.Errorf("tool_choice names an unavailable tool")
			}
			out["tool_choice"] = map[string]any{"type": "function", "function": map[string]string{"name": upstreamToolName(choice.Name)}}
		default:
			return fmt.Errorf("invalid tool_choice")
		}
		if choice.DisableParallel != nil {
			out["parallel_tool_calls"] = !*choice.DisableParallel
		}
	}
	return nil
}

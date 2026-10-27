package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"time"
)

const maxMessageBytes = 8 << 20

func decodeOneJSON(data []byte, out any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if err := d.Decode(out); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return fmt.Errorf("expected one JSON document")
	}
	return nil
}

func messagesError(w http.ResponseWriter, status int, kind, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"type": "error", "error": map[string]string{"type": kind, "message": message}})
}

func upstreamErrorType(status int) string {
	switch status {
	case 400, 422:
		return "invalid_request_error"
	case 401:
		return "authentication_error"
	case 403:
		return "permission_error"
	case 404:
		return "not_found_error"
	case 413:
		return "request_too_large"
	case 429:
		return "rate_limit_error"
	case 529, 503:
		return "overloaded_error"
	default:
		return "api_error"
	}
}

type completionUsage struct {
	Prompt        *int `json:"prompt_tokens"`
	Completion    *int `json:"completion_tokens"`
	PromptDetails *struct {
		Cached  *int `json:"cached_tokens"`
		Written *int `json:"cache_write_tokens"`
	} `json:"prompt_tokens_details"`
}

func (u *completionUsage) complete() bool {
	if u == nil || u.Prompt == nil || u.Completion == nil || *u.Prompt < 0 || *u.Completion < 0 {
		return false
	}
	remaining := *u.Prompt
	if u.PromptDetails != nil {
		for _, value := range []*int{u.PromptDetails.Cached, u.PromptDetails.Written} {
			if value != nil {
				if *value < 0 || *value > remaining {
					return false
				}
				remaining -= *value
			}
		}
	}
	return true
}

func (u *completionUsage) merge(other *completionUsage) {
	if other == nil {
		return
	}
	if other.Prompt != nil {
		u.Prompt = other.Prompt
	}
	if other.Completion != nil {
		u.Completion = other.Completion
	}
	if other.PromptDetails != nil {
		if u.PromptDetails == nil {
			u.PromptDetails = other.PromptDetails
		} else {
			if other.PromptDetails.Cached != nil {
				u.PromptDetails.Cached = other.PromptDetails.Cached
			}
			if other.PromptDetails.Written != nil {
				u.PromptDetails.Written = other.PromptDetails.Written
			}
		}
	}
}

func (u *completionUsage) translated() map[string]int {
	if u == nil {
		return nil
	}
	values := map[string]int{}
	if u.Prompt != nil && *u.Prompt >= 0 {
		values["input_tokens"] = *u.Prompt
		if u.PromptDetails != nil {
			if cached := u.PromptDetails.Cached; cached != nil {
				values["cache_read_input_tokens"] = *cached
				values["input_tokens"] -= *cached
			}
			if written := u.PromptDetails.Written; written != nil {
				values["cache_creation_input_tokens"] = *written
				values["input_tokens"] -= *written
			}
		}
	}
	if u.Completion != nil && *u.Completion >= 0 {
		values["output_tokens"] = *u.Completion
	}
	if len(values) == 0 {
		return nil
	}
	return values
}
func stopReason(reason string) (string, error) {
	switch reason {
	case "stop":
		return "end_turn", nil
	case "length":
		return "max_tokens", nil
	case "tool_calls":
		return "tool_use", nil
	case "content_filter":
		return "refusal", nil
	default:
		return "", fmt.Errorf("unsupported or missing upstream finish reason")
	}
}

type completionReply struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content      *string         `json:"content"`
			ToolCalls    json.RawMessage `json:"tool_calls"`
			FunctionCall json.RawMessage `json:"function_call"`
			Reasoning    json.RawMessage `json:"reasoning_content"`
			Refusal      json.RawMessage `json:"refusal"`
		} `json:"message"`
		Finish string `json:"finish_reason"`
	} `json:"choices"`
	Usage *completionUsage `json:"usage"`
	Error json.RawMessage  `json:"error"`
}

func hasJSONValue(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return true
	}
	value := compact.String()
	return value != "null" && value != "[]" && value != `""`
}

func writeMessagesReply(w http.ResponseWriter, resp *http.Response, req anthropicReq) {
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxMessageBytes+1))
	var upstream completionReply
	if err != nil || len(data) > maxMessageBytes || decodeOneJSON(data, &upstream) != nil || len(upstream.Choices) != 1 || hasJSONValue(upstream.Error) {
		messagesError(w, 502, "api_error", "Val returned an invalid or incomplete completion")
		return
	}
	choice := upstream.Choices[0]
	if !upstream.Usage.complete() {
		w.Header().Set("X-Val2harness-Usage", "unavailable")
		messagesError(w, 502, "api_error", "Val did not provide complete measured usage required by the Messages API")
		return
	}
	stop, err := stopReason(choice.Finish)
	if err != nil || hasJSONValue(choice.Message.FunctionCall) || hasJSONValue(choice.Message.Reasoning) {
		messagesError(w, 502, "api_error", "Val returned content this Messages adapter cannot translate")
		return
	}
	content := []any{}
	if choice.Message.Content != nil && *choice.Message.Content != "" {
		content = append(content, map[string]any{"type": "text", "text": *choice.Message.Content})
	}
	if hasJSONValue(choice.Message.Refusal) {
		var refusal string
		if json.Unmarshal(choice.Message.Refusal, &refusal) != nil {
			messagesError(w, 502, "api_error", "Invalid upstream refusal")
			return
		}
		content = append(content, map[string]any{"type": "text", "text": refusal})
		stop = "refusal"
	}
	calls, err := responseToolCalls(choice.Message.ToolCalls, req)
	if err != nil {
		messagesError(w, 502, "api_error", upstreamToolErrorMessage(err))
		return
	}
	if validateToolChoice(req, calls, stop) != nil || (len(calls) == 0 && stop == "tool_use") || (len(calls) > 0 && stop != "tool_use" && stop != "max_tokens") || (choice.Message.Content == nil && len(calls) == 0 && len(content) == 0 && stop != "refusal") {
		messagesError(w, 502, "api_error", "Val returned invalid tool calls or content")
		return
	}
	content = append(content, calls...)
	if len(content) == 0 && stop != "refusal" {
		content = append(content, map[string]any{"type": "text", "text": ""})
	}
	model := upstream.Model
	if model == "" {
		model = mapAnthropicModel(req.Model)
	}
	id := upstream.ID
	if id == "" {
		id = fmt.Sprintf("msg_%d", time.Now().UnixNano())
	}
	result := map[string]any{"id": id, "type": "message", "role": "assistant", "model": model, "content": content, "stop_reason": stop, "stop_sequence": nil}
	if usage := upstream.Usage.translated(); usage != nil {
		result["usage"] = usage
	} else {
		w.Header().Set("X-Val2harness-Usage", "unavailable")
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

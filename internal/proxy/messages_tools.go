package proxy

import (
	"encoding/json"
	"strings"
)

type anthropicTool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	InputSchema any    `json:"input_schema"`
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
			if m, ok := item.(map[string]any); ok && m["type"] == "text" {
				if t, ok := m["text"].(string); ok {
					sb.WriteString(t)
				}
			}
		}
		return sb.String()
	default:
		return ""
	}
}

func convertToolsToOpenAI(tools []anthropicTool) []map[string]any {
	var oaiTools []map[string]any
	for _, t := range tools {
		oaiTools = append(oaiTools, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  t.InputSchema,
			},
		})
	}
	return oaiTools
}

func convertMessagesToOpenAI(system any, messages []anthropicMsg) []map[string]any {
	var openAIMessages []map[string]any

	if sysText := extractText(system); sysText != "" {
		openAIMessages = append(openAIMessages, map[string]any{
			"role":    "system",
			"content": sysText,
		})
	}

	for _, msg := range messages {
		blocks, ok := msg.Content.([]any)
		if !ok {
			openAIMessages = append(openAIMessages, map[string]any{
				"role":    msg.Role,
				"content": extractText(msg.Content),
			})
			continue
		}

		var textParts []string
		for _, b := range blocks {
			m, isMap := b.(map[string]any)
			if !isMap {
				continue
			}
			blockType, _ := m["type"].(string)
			switch blockType {
			case "text":
				if t, ok := m["text"].(string); ok {
					textParts = append(textParts, t)
				}
			case "tool_result":
				toolUseID, _ := m["tool_use_id"].(string)
				content := extractText(m["content"])
				openAIMessages = append(openAIMessages, map[string]any{
					"role":         "tool",
					"tool_call_id": toolUseID,
					"content":      content,
				})
			case "tool_use":
				callID, _ := m["id"].(string)
				name, _ := m["name"].(string)
				inputData, _ := json.Marshal(m["input"])
				openAIMessages = append(openAIMessages, map[string]any{
					"role": "assistant",
					"tool_calls": []map[string]any{
						{
							"id":   callID,
							"type": "function",
							"function": map[string]any{
								"name":      name,
								"arguments": string(inputData),
							},
						},
					},
				})
			}
		}

		if len(textParts) > 0 {
			openAIMessages = append(openAIMessages, map[string]any{
				"role":    msg.Role,
				"content": strings.Join(textParts, "\n"),
			})
		}
	}

	return openAIMessages
}

func formatAnthropicError(status int, message string) []byte {
	errType := "api_error"
	if status == 400 || status == 422 {
		errType = "invalid_request_error"
	} else if status == 401 || status == 403 {
		errType = "authentication_error"
	} else if status == 429 {
		errType = "rate_limit_error"
	}
	res, _ := json.Marshal(map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    errType,
			"message": message,
		},
	})
	return res
}

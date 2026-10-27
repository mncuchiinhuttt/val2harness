package proxy

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func contentBlocks(value any) ([]map[string]any, error) {
	if text, ok := value.(string); ok {
		return []map[string]any{{"type": "text", "text": text}}, nil
	}
	list, ok := value.([]any)
	if !ok || len(list) == 0 {
		return nil, fmt.Errorf("content must be a string or nonempty block array")
	}
	blocks := make([]map[string]any, 0, len(list))
	for _, item := range list {
		block, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("content blocks must be objects")
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}
func fieldsOnly(block map[string]any, names ...string) error {
	allowed := map[string]bool{}
	for _, name := range names {
		allowed[name] = true
	}
	for name := range block {
		if !allowed[name] {
			return fmt.Errorf("unsupported content option %q", name)
		}
	}
	return nil
}
func (r *anthropicReq) imagePart(block map[string]any) (map[string]any, error) {
	if err := fieldsOnly(block, "type", "source", "cache_control"); err != nil {
		return nil, err
	}
	if err := r.cacheHint(block["cache_control"]); err != nil {
		return nil, err
	}
	source, ok := block["source"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("image source must be an object")
	}
	var imageURL string
	switch source["type"] {
	case "base64":
		if err := fieldsOnly(source, "type", "data", "media_type"); err != nil {
			return nil, err
		}
		mime, _ := source["media_type"].(string)
		switch mime {
		case "image/png", "image/jpeg", "image/gif", "image/webp":
		default:
			return nil, fmt.Errorf("unsupported image media_type")
		}
		encoded, ok := source["data"].(string)
		if !ok {
			return nil, fmt.Errorf("image data must be base64")
		}
		decoded, err := base64.StdEncoding.Strict().DecodeString(encoded)
		if err != nil || len(decoded) == 0 || len(decoded) > 5<<20 || http.DetectContentType(decoded) != mime {
			return nil, fmt.Errorf("image data is invalid, exceeds 5 MiB, or does not match its MIME type")
		}
		imageURL = "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(decoded)
	case "url":
		if err := fieldsOnly(source, "type", "url"); err != nil {
			return nil, err
		}
		raw, ok := source["url"].(string)
		u, err := url.Parse(raw)
		if !ok || err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
			return nil, fmt.Errorf("image URL must be HTTP(S) without embedded credentials")
		}
		imageURL = raw
	default:
		return nil, fmt.Errorf("image files require the upstream Files API; use base64 or HTTP(S) URL images")
	}
	return map[string]any{"type": "image_url", "image_url": map[string]string{"url": imageURL}}, nil
}

func (r *anthropicReq) userPart(block map[string]any) (map[string]any, error) {
	switch block["type"] {
	case "text":
		if err := fieldsOnly(block, "type", "text", "cache_control"); err != nil {
			return nil, err
		}
		if err := r.cacheHint(block["cache_control"]); err != nil {
			return nil, err
		}
		text, ok := block["text"].(string)
		if !ok {
			return nil, fmt.Errorf("text blocks require text")
		}
		return map[string]any{"type": "text", "text": text}, nil
	case "image":
		return r.imagePart(block)
	default:
		return nil, fmt.Errorf("unsupported user content block; expected text, image, or a paired tool_result")
	}
}

func (r *anthropicReq) convertHistory() ([]map[string]any, error) {
	out := []map[string]any{}
	pending := map[string]bool{}
	if r.System != nil {
		blocks, err := contentBlocks(r.System)
		if err != nil {
			return nil, err
		}
		var text strings.Builder
		for _, block := range blocks {
			if block["type"] != "text" {
				return nil, fmt.Errorf("system content must be text")
			}
			part, err := r.userPart(block)
			if err != nil {
				return nil, err
			}
			text.WriteString(part["text"].(string))
		}
		out = append(out, map[string]any{"role": "system", "content": text.String()})
	}
	for _, message := range r.Messages {
		blocks, err := contentBlocks(message.Content)
		if err != nil {
			return nil, err
		}
		switch message.Role {
		case "assistant":
			if len(pending) > 0 {
				return nil, fmt.Errorf("assistant tool calls require their user tool results before another assistant turn")
			}
			var text strings.Builder
			calls := []any{}
			for _, block := range blocks {
				if block["type"] == "text" {
					part, err := r.userPart(block)
					if err != nil {
						return nil, err
					}
					text.WriteString(part["text"].(string))
					continue
				}
				if block["type"] != "tool_use" {
					return nil, fmt.Errorf("unsupported assistant content block")
				}
				if err := fieldsOnly(block, "type", "id", "name", "input", "cache_control"); err != nil {
					return nil, err
				}
				if err := r.cacheHint(block["cache_control"]); err != nil {
					return nil, err
				}
				id, _ := block["id"].(string)
				name, _ := block["name"].(string)
				input, ok := block["input"].(map[string]any)
				if !toolIDPattern.MatchString(id) || pending[id] || !toolNamePattern.MatchString(name) || !ok || input == nil {
					return nil, fmt.Errorf("tool_use requires a unique id, valid name and object input")
				}
				pending[id] = true
				args, err := json.Marshal(input)
				if err != nil {
					return nil, err
				}
				calls = append(calls, map[string]any{"id": id, "type": "function", "function": map[string]string{"name": upstreamToolName(name), "arguments": string(args)}})
			}
			m := map[string]any{"role": "assistant", "content": text.String()}
			if len(calls) > 0 {
				m["tool_calls"] = calls
			}
			out = append(out, m)
		case "user":
			parts := []any{}
			hadOrdinary := false
			for _, block := range blocks {
				if block["type"] != "tool_result" {
					part, err := r.userPart(block)
					if err != nil {
						return nil, err
					}
					parts = append(parts, part)
					hadOrdinary = true
					continue
				}
				if hadOrdinary {
					return nil, fmt.Errorf("tool results must precede ordinary user content")
				}
				if err := fieldsOnly(block, "type", "tool_use_id", "content", "is_error", "cache_control"); err != nil {
					return nil, err
				}
				if err := r.cacheHint(block["cache_control"]); err != nil {
					return nil, err
				}
				id, _ := block["tool_use_id"].(string)
				if !pending[id] {
					return nil, fmt.Errorf("tool_result has no matching pending tool_use")
				}
				delete(pending, id)
				isError := false
				if value, exists := block["is_error"]; exists {
					var valid bool
					isError, valid = value.(bool)
					if !valid {
						return nil, fmt.Errorf("tool_result.is_error must be boolean")
					}
				}
				value, exists := block["content"]
				if !exists {
					value = ""
				}
				resultBlocks, err := contentBlocks(value)
				if list, ok := value.([]any); ok && len(list) == 0 {
					resultBlocks = nil
					err = nil
				}
				if err != nil {
					return nil, err
				}
				var text strings.Builder
				for _, result := range resultBlocks {
					part, err := r.userPart(result)
					if err != nil {
						return nil, err
					}
					if part["type"] == "text" {
						text.WriteString(part["text"].(string))
					} else {
						parts = append(parts, map[string]any{"type": "text", "text": "Image returned by tool " + id + ":"}, part)
					}
				}
				resultText := text.String()
				if isError {
					resultText = "Tool execution failed:\n" + resultText
				}
				out = append(out, map[string]any{"role": "tool", "tool_call_id": id, "content": resultText})
			}
			if len(pending) > 0 {
				return nil, fmt.Errorf("all parallel tool calls must have a result in the following user turn")
			}
			if len(parts) > 0 {
				if len(parts) == 1 && parts[0].(map[string]any)["type"] == "text" {
					out = append(out, map[string]any{"role": "user", "content": parts[0].(map[string]any)["text"]})
				} else {
					out = append(out, map[string]any{"role": "user", "content": parts})
				}
			}
		default:
			return nil, fmt.Errorf("message role must be user or assistant")
		}
	}
	if len(pending) > 0 {
		return nil, fmt.Errorf("unresolved tool_use history requires tool results")
	}
	return out, nil
}

package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
	"val2harness/internal/sse"
)

type completionChunk struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Content      *string         `json:"content"`
			ToolCalls    json.RawMessage `json:"tool_calls"`
			FunctionCall json.RawMessage `json:"function_call"`
			Reasoning    json.RawMessage `json:"reasoning_content"`
			Refusal      *string         `json:"refusal"`
		} `json:"delta"`
		Finish string `json:"finish_reason"`
	} `json:"choices"`
	Usage *completionUsage `json:"usage"`
	Error json.RawMessage  `json:"error"`
}

type streamTool struct {
	call      openAIToolCall
	arguments strings.Builder
}
type streamBlock struct {
	kind string
	text strings.Builder
	tool *streamTool
}

// Val commonly sends usage only at stream end. Buffer bounded output so the
// official SDK receives a valid message_start with measured counts, rather than
// invented zeros or an absent usage object that crashes its accumulator.
func (s *Server) streamMessages(w http.ResponseWriter, r *http.Request, resp *http.Response, req anthropicReq) {
	limited := &io.LimitedReader{R: resp.Body, N: (64 << 20) + 1}
	decoder := sse.NewDecoder(limited, 0)
	model := mapAnthropicModel(req.Model)
	modelSeen := ""
	id := ""
	stop := ""
	refused := false
	counts := &completionUsage{}
	tools := map[int]*streamTool{}
	blocks := []*streamBlock{}
	contentBytes := 0
	events := 0
	fail := func(message string) {
		if r.Context().Err() == nil {
			messagesError(w, 502, "api_error", message)
		}
	}
	appendText := func(text string) {
		if text == "" {
			return
		}
		if len(blocks) == 0 || blocks[len(blocks)-1].kind != "text" {
			blocks = append(blocks, &streamBlock{kind: "text"})
		}
		blocks[len(blocks)-1].text.WriteString(text)
	}
	for {
		event, err := decoder.Next()
		if r.Context().Err() != nil {
			return
		}
		if err == io.EOF {
			if limited.N <= 0 {
				fail("Val stream exceeded the 64 MiB transport limit")
				return
			}
			if stop == "" {
				fail("Val stream ended without a completion reason")
				return
			}
			break
		}
		if err != nil {
			fail("Val stream was interrupted or exceeded the event limit")
			return
		}
		events++
		if events > 200000 {
			fail("Val stream exceeded the event count limit")
			return
		}
		if event.Name == "error" {
			fail("Val reported a streaming error")
			return
		}
		if event.Name != "" && event.Name != "message" {
			continue
		}
		if event.Data == "[DONE]" {
			if stop == "" {
				fail("Val stream ended without a completion reason")
				return
			}
			break
		}
		var chunk completionChunk
		if decodeOneJSON([]byte(event.Data), &chunk) != nil || hasJSONValue(chunk.Error) || len(chunk.Choices) > 1 || (len(chunk.Choices) == 0 && chunk.Usage == nil) {
			fail("Val returned an invalid streaming completion")
			return
		}
		if chunk.Model != "" {
			if modelSeen != "" && chunk.Model != modelSeen {
				fail("Val changed model identity during the stream")
				return
			}
			modelSeen = chunk.Model
			model = chunk.Model
		}
		if chunk.ID != "" {
			if id != "" && id != chunk.ID {
				fail("Val changed response identity during the stream")
				return
			}
			id = chunk.ID
		}
		counts.merge(chunk.Usage)
		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]
		if hasJSONValue(choice.Delta.FunctionCall) || hasJSONValue(choice.Delta.Reasoning) {
			fail("Val returned unsupported legacy function or reasoning content")
			return
		}
		// Val emits an empty assistant envelope after finish, before usage.
		// It adds no content; substantive post-finish deltas remain invalid.
		newText := choice.Delta.Content != nil && *choice.Delta.Content != ""
		if stop != "" && (newText || hasJSONValue(choice.Delta.ToolCalls) || choice.Delta.Refusal != nil || choice.Finish != "") {
			fail("Val continued a completed stream")
			return
		}
		if choice.Delta.Content != nil {
			appendText(*choice.Delta.Content)
			contentBytes += len(*choice.Delta.Content)
		}
		if choice.Delta.Refusal != nil {
			appendText(*choice.Delta.Refusal)
			contentBytes += len(*choice.Delta.Refusal)
			refused = true
		}
		if hasJSONValue(choice.Delta.ToolCalls) {
			var deltas []openAIToolCall
			if json.Unmarshal(choice.Delta.ToolCalls, &deltas) != nil || len(deltas) > 128 {
				fail("Val returned invalid tool fragments")
				return
			}
			for _, delta := range deltas {
				if delta.Index == nil || *delta.Index < 0 || *delta.Index >= 128 {
					fail("Val returned an invalid tool index")
					return
				}
				tool, exists := tools[*delta.Index]
				if !exists {
					tool = &streamTool{}
					tools[*delta.Index] = tool
					blocks = append(blocks, &streamBlock{kind: "tool_use", tool: tool})
				}
				if delta.ID != "" {
					if tool.call.ID != "" && tool.call.ID != delta.ID {
						fail("Val changed a tool-call identity")
						return
					}
					tool.call.ID = delta.ID
				}
				if delta.Type != "" {
					if delta.Type != "function" {
						fail("Unsupported upstream tool type")
						return
					}
					tool.call.Type = delta.Type
				}
				name := delta.Function.Name
				if name != "" {
					tool.call.Function.Name += name
				}
				if len(tool.call.Function.Name) > 128 || len(tool.call.ID) > 256 || tool.arguments.Len()+len(delta.Function.Arguments) > maxToolArgumentBytes {
					fail("Val tool identity or arguments exceeded the size limit")
					return
				}
				tool.arguments.WriteString(delta.Function.Arguments)
				contentBytes += len(delta.Function.Arguments)
			}
		}
		if contentBytes > 16<<20 {
			fail("Val response exceeded the 16 MiB content limit")
			return
		}
		if choice.Finish != "" {
			stop, err = stopReason(choice.Finish)
			if err != nil {
				fail("Val returned an unsupported completion reason")
				return
			}
		}
	}
	if len(tools) > 0 && (stop != "tool_use" && stop != "max_tokens" || refused) {
		fail("Val tool calls conflict with the completion reason")
		return
	}
	if len(tools) == 0 && stop == "tool_use" {
		fail("Val completed a tool turn without tool calls")
		return
	}
	if refused {
		stop = "refusal"
	}
	if !counts.complete() {
		w.Header().Set("X-Val2harness-Usage", "unavailable")
		fail("Val did not provide complete measured usage required for an Anthropic stream; use OpenAI pass-through or enable upstream usage")
		return
	}
	usage := counts.translated()
	// Validate every call before delivering any executable tool request.
	usedIDs := map[string]bool{}
	allCalls := []any{}
	for _, block := range blocks {
		if block.kind != "tool_use" {
			continue
		}
		call := block.tool.call
		call.Function.Arguments = block.tool.arguments.String()
		if usedIDs[call.ID] {
			fail("Val reused a tool-call id")
			return
		}
		usedIDs[call.ID] = true
		raw, _ := json.Marshal([]openAIToolCall{call})
		translated, err := responseToolCalls(raw, req)
		if err != nil {
			fail(upstreamToolErrorMessage(err))
			return
		}
		allCalls = append(allCalls, translated...)
	}
	if validateToolChoice(req, allCalls, stop) != nil {
		fail("Val violated the requested tool choice")
		return
	}
	if len(blocks) == 0 && stop != "refusal" {
		blocks = append(blocks, &streamBlock{kind: "text"})
	}
	if id == "" {
		id = fmt.Sprintf("msg_%d", time.Now().UnixNano())
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("X-Val2harness-Usage", "measured-buffered")
	send := func(kind string, event map[string]any) bool {
		if r.Context().Err() != nil {
			return false
		}
		event["type"] = kind
		data, err := json.Marshal(event)
		if err != nil {
			return false
		}
		if _, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, data); err != nil {
			return false
		}
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		return true
	}
	if !send("message_start", map[string]any{"message": map[string]any{"id": id, "type": "message", "role": "assistant", "model": model, "content": []any{}, "stop_reason": nil, "stop_sequence": nil, "usage": usage}}) {
		return
	}
	for index, block := range blocks {
		var content map[string]any
		if block.kind == "text" {
			content = map[string]any{"type": "text", "text": ""}
		} else {
			content = map[string]any{"type": "tool_use", "id": block.tool.call.ID, "name": req.originalToolName(block.tool.call.Function.Name), "input": map[string]any{}}
		}
		if !send("content_block_start", map[string]any{"index": index, "content_block": content}) {
			return
		}
		text := block.text.String()
		deltaType, key := "text_delta", "text"
		if block.kind == "tool_use" {
			text = block.tool.arguments.String()
			deltaType, key = "input_json_delta", "partial_json"
		}
		for len(text) > 0 {
			end := min(len(text), 32768)
			for end < len(text) && !utf8.RuneStart(text[end]) {
				end--
			}
			if !send("content_block_delta", map[string]any{"index": index, "delta": map[string]any{"type": deltaType, key: text[:end]}}) {
				return
			}
			text = text[end:]
		}
		if !send("content_block_stop", map[string]any{"index": index}) {
			return
		}
	}
	if !send("message_delta", map[string]any{"delta": map[string]any{"stop_reason": stop, "stop_sequence": nil}, "usage": usage}) {
		return
	}
	send("message_stop", map[string]any{})
}

package valclient

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"val2harness/internal/analytics"
	"val2harness/internal/sse"
)

// Observation never retains the whole stream. JSON lookahead and individual
// SSE events are bounded independently; oversized data still passes through.
const maxObservedBytes = 1 << 20

type observedUsage struct {
	Prompt     *int `json:"prompt_tokens"`
	Completion *int `json:"completion_tokens"`
	Total      *int `json:"total_tokens"`
}

func (u observedUsage) apply(o *analytics.Observation) {
	if u.Prompt == nil || u.Completion == nil || *u.Prompt < 0 || *u.Completion < 0 || *u.Prompt > int(^uint(0)>>1)-*u.Completion {
		return
	}
	total := *u.Prompt + *u.Completion
	if int64(total) > analytics.MaxExactTokens || (u.Total != nil && *u.Total != total) {
		return
	}
	o.PromptTokens = *u.Prompt
	o.CompletionTokens = *u.Completion
	o.TotalTokens = total
	o.UsageKnown = true
}

// Usage snapshots are cumulative. Merge fields, never add snapshots, and keep
// any invalid or decreasing snapshot unmeasured even if a later one looks valid.
func (u *observedUsage) merge(next observedUsage) bool {
	for _, pair := range [][2]*int{{u.Prompt, next.Prompt}, {u.Completion, next.Completion}, {u.Total, next.Total}} {
		if pair[1] != nil && (*pair[1] < 0 || int64(*pair[1]) > analytics.MaxExactTokens || (pair[0] != nil && *pair[1] < *pair[0])) {
			return false
		}
	}
	if next.Prompt != nil {
		u.Prompt = next.Prompt
	}
	if next.Completion != nil {
		u.Completion = next.Completion
	}
	if next.Total != nil {
		u.Total = next.Total
	} else if next.Prompt != nil || next.Completion != nil {
		u.Total = nil
	}
	if u.Prompt != nil && u.Completion != nil {
		o := analytics.Observation{}
		u.apply(&o)
		return o.UsageKnown
	}
	return true
}

type observedChoice struct {
	Index   int             `json:"index"`
	Message json.RawMessage `json:"message"`
	Delta   json.RawMessage `json:"delta"`
	Finish  *string         `json:"finish_reason"`
}

type observedEnvelope struct {
	Choices []observedChoice `json:"choices"`
	Usage   json.RawMessage  `json:"usage"`
	Error   json.RawMessage  `json:"error"`
}

func decodeObserved(data []byte) (observedEnvelope, bool) {
	var reply observedEnvelope
	if json.Unmarshal(data, &reply) != nil || len(bytes.TrimSpace(data)) == 0 || bytes.TrimSpace(data)[0] != '{' {
		return reply, false
	}
	return reply, true
}

func hasObservedError(raw json.RawMessage) bool {
	return len(raw) > 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func budgetError(raw json.RawMessage) bool {
	var e struct {
		Code string `json:"code"`
	}
	return json.Unmarshal(raw, &e) == nil && e.Code == "budget_exceeded"
}

func statusFromHTTP(code int) string {
	switch {
	case code == 401 || code == 403:
		return "authentication_error"
	case code == 429:
		return "rate_limited"
	case code >= 400:
		return "provider_error"
	default:
		return "unknown"
	}
}

func observeError(o *analytics.Observation, raw json.RawMessage) bool {
	if !hasObservedError(raw) {
		return false
	}
	if o.Status != "authentication_error" {
		if budgetError(raw) {
			o.Status = "exhausted"
		} else if o.Status == "unknown" || o.Status == "available" {
			o.Status = "provider_error"
		}
	}
	return true
}

func validFinish(finish *string) bool {
	if finish == nil {
		return false
	}
	switch *finish {
	case "stop", "length", "tool_calls", "function_call", "content_filter":
		return true
	}
	return false
}

func observedObject(raw json.RawMessage) bool {
	return len(bytes.TrimSpace(raw)) > 0 && bytes.TrimSpace(raw)[0] == '{'
}

func validObservedMessage(raw json.RawMessage, finish string) bool {
	var message struct {
		Role      string  `json:"role"`
		Content   *string `json:"content"`
		Refusal   *string `json:"refusal"`
		ToolCalls []struct {
			ID       string `json:"id"`
			Type     string `json:"type"`
			Function struct {
				Name      string  `json:"name"`
				Arguments *string `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	}
	if !observedObject(raw) || json.Unmarshal(raw, &message) != nil || (message.Role != "" && message.Role != "assistant") {
		return false
	}
	for _, call := range message.ToolCalls {
		if call.ID == "" || call.Type != "function" || call.Function.Name == "" || call.Function.Arguments == nil {
			return false
		}
	}
	if finish == "tool_calls" {
		return len(message.ToolCalls) > 0
	}
	return message.Content != nil || message.Refusal != nil || len(message.ToolCalls) > 0
}

// Validate shape without retaining or assembling text/tool arguments. Empty
// assistant envelopes are harmless; substantive content after finish is not.
func observedDelta(raw json.RawMessage) (valid, substantive, recognized bool) {
	var delta struct {
		Role      string  `json:"role"`
		Content   *string `json:"content"`
		Refusal   *string `json:"refusal"`
		ToolCalls []struct {
			Index    *int   `json:"index"`
			ID       string `json:"id"`
			Type     string `json:"type"`
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	}
	if !observedObject(raw) || json.Unmarshal(raw, &delta) != nil || (delta.Role != "" && delta.Role != "assistant") || len(delta.ToolCalls) > 128 {
		return false, false, false
	}
	for _, call := range delta.ToolCalls {
		if call.Index == nil || *call.Index < 0 || *call.Index >= 128 || (call.Type != "" && call.Type != "function") {
			return false, false, false
		}
	}
	return true, (delta.Content != nil && *delta.Content != "") || delta.Refusal != nil || len(delta.ToolCalls) > 0, delta.Role == "assistant" || delta.Content != nil || delta.Refusal != nil || len(delta.ToolCalls) > 0
}

func observeJSON(data []byte, o analytics.Observation) analytics.Observation {
	reply, ok := decodeObserved(data)
	if !ok || observeError(&o, reply.Error) || o.StatusCode != http.StatusOK || len(reply.Choices) == 0 {
		return o
	}
	for _, choice := range reply.Choices {
		if !validFinish(choice.Finish) || !validObservedMessage(choice.Message, *choice.Finish) {
			return o
		}
	}
	o.Status = "available"
	var usage observedUsage
	if json.Unmarshal(reply.Usage, &usage) == nil {
		usage.apply(&o)
	}
	return o
}

type observedBody struct {
	body        io.ReadCloser
	reader      io.Reader
	ctx         context.Context
	tracker     *analytics.Tracker
	generation  uint64
	started     time.Time
	observation analytics.Observation
	pipe        *io.PipeWriter
	parsed      chan struct{}
	terminal    bool
	once        sync.Once
}

func (c *Client) observeResponse(ctx context.Context, resp *http.Response, generation uint64, o analytics.Observation, started time.Time) {
	o.StatusCode = resp.StatusCode
	o.Status = statusFromHTTP(resp.StatusCode)
	body := &observedBody{body: resp.Body, reader: resp.Body, ctx: ctx, tracker: c.tracker, generation: generation, started: started, observation: o}
	if strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		reader, writer := io.Pipe()
		body.pipe = writer
		body.parsed = make(chan struct{})
		go func() {
			defer close(body.parsed)
			defer reader.Close()
			body.observation, body.terminal = observeSSE(reader, o)
		}()
	} else {
		prefix, err := io.ReadAll(io.LimitReader(resp.Body, maxObservedBytes+1))
		body.reader = io.MultiReader(bytes.NewReader(prefix), resp.Body)
		if err != nil {
			body.reader = io.MultiReader(bytes.NewReader(prefix), observationReadError{err})
		}
		if err == nil && len(prefix) <= maxObservedBytes {
			body.observation = observeJSON(prefix, o)
			if resp.StatusCode == http.StatusOK && body.observation.Status == "exhausted" {
				resp.StatusCode = http.StatusTooManyRequests
				resp.Status = "429 Too Many Requests"
				body.observation.StatusCode = http.StatusTooManyRequests
			}
		}
	}
	resp.Body = body
}

type observationReadError struct{ err error }

func (r observationReadError) Read([]byte) (int, error) { return 0, r.err }

func (b *observedBody) Read(p []byte) (int, error) {
	n, err := b.reader.Read(p)
	if n > 0 && b.pipe != nil {
		_, _ = b.pipe.Write(p[:n])
	}
	if err != nil {
		b.finish(err == io.EOF, false)
	}
	return n, err
}

func (b *observedBody) Close() error {
	err := b.body.Close()
	b.finish(false, err == nil)
	return err
}

func (b *observedBody) finish(complete, closed bool) {
	b.once.Do(func() {
		if b.pipe != nil {
			if (complete || closed) && b.ctx.Err() == nil {
				_ = b.pipe.Close()
			} else {
				_ = b.pipe.CloseWithError(io.ErrUnexpectedEOF)
			}
			<-b.parsed
		}
		complete = b.ctx.Err() == nil && (complete || (closed && b.terminal))
		o := b.observation
		if !complete {
			o.UsageKnown = false
			if o.Status == "available" {
				o.Status = "unknown"
			}
		}
		o.LatencyMs = time.Since(b.started).Milliseconds()
		b.tracker.Observe(b.generation, o)
	})
}

func observeSSE(reader io.Reader, o analytics.Observation) (analytics.Observation, bool) {
	decoder := sse.NewDecoder(reader, maxObservedBytes)
	type observedStreamChoice struct{ finished, recognized bool }
	choices := make(map[int]observedStreamChoice)
	var usage observedUsage
	valid, done, usageValid, eof := true, false, true, false
	for {
		event, err := decoder.Next()
		if err != nil {
			if err != io.EOF {
				valid = false
			} else {
				eof = true
			}
			break
		}
		if strings.TrimSpace(event.Data) == "[DONE]" {
			done = true
			break
		}
		reply, ok := decodeObserved([]byte(event.Data))
		if !ok {
			valid = false
			continue
		}
		if observeError(&o, reply.Error) {
			valid = false
			continue
		}
		for _, choice := range reply.Choices {
			_, exists := choices[choice.Index]
			if choice.Index < 0 || choice.Index >= 128 || (!exists && len(choices) >= 128) {
				valid = false
				continue
			}
			state := choices[choice.Index]
			deltaValid, substantive, recognized := observedDelta(choice.Delta)
			if !deltaValid || (state.finished && (substantive || choice.Finish != nil)) {
				valid = false
			}
			if choice.Finish != nil && !validFinish(choice.Finish) {
				valid = false
			}
			choices[choice.Index] = observedStreamChoice{finished: state.finished || validFinish(choice.Finish), recognized: state.recognized || recognized}
		}
		if len(reply.Usage) > 0 && !bytes.Equal(bytes.TrimSpace(reply.Usage), []byte("null")) {
			var next observedUsage
			if json.Unmarshal(reply.Usage, &next) != nil {
				usageValid = false
				continue
			}
			if !usage.merge(next) {
				usageValid = false
			}
		}
	}
	for _, state := range choices {
		if !state.finished || !state.recognized {
			valid = false
		}
	}
	if valid && (done || eof) && len(choices) > 0 && o.StatusCode == http.StatusOK && o.Status == "unknown" {
		o.Status = "available"
		if usageValid {
			usage.apply(&o)
		}
	}
	return o, valid && done && len(choices) > 0
}

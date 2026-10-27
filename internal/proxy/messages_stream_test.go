package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"val2harness/internal/valclient"
)

func streamingFixture(t *testing.T, body string) *Server {
	return messageFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, body)
	})
}
func textChunk(text string) string {
	b, _ := json.Marshal(map[string]any{"model": "synthetic-stream-model", "choices": []any{map[string]any{"delta": map[string]string{"content": text}}}})
	return string(b)
}

const finishChunk = `{"choices":[{"delta":{},"finish_reason":"length"}],"usage":{"prompt_tokens":123,"completion_tokens":7}}`

func TestMessagesSSEAcceptsFramingAndLargeEvents(t *testing.T) {
	for name, text := range map[string]string{"no-space": "valid text", "large": strings.Repeat("x", 70000), "cr": "CR text", "multiline": "multiline text"} {
		t.Run(name, func(t *testing.T) {
			body := "data:" + textChunk(text) + "\n\ndata:" + finishChunk + "\n\ndata:[DONE]\n\n"
			if name == "cr" {
				body = strings.ReplaceAll(body, "\n", "\r")
			}
			if name == "multiline" {
				body = "data:{\ndata:\"model\":\"synthetic-stream-model\",\ndata:\"choices\":[{\"delta\":{\"content\":\"multiline text\"}}]}\n\ndata:" + finishChunk + "\n\ndata:[DONE]\n\n"
			}
			w := messageRequest(t, streamingFixture(t, body), true, nil)
			var reconstructed strings.Builder
			for _, line := range strings.Split(w.Body.String(), "\n") {
				if strings.HasPrefix(line, "data:") {
					var event struct {
						Delta struct {
							Text string `json:"text"`
						} `json:"delta"`
					}
					json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event)
					reconstructed.WriteString(event.Delta.Text)
				}
			}
			if reconstructed.String() != text || !strings.Contains(w.Body.String(), "event: message_stop") || strings.Contains(w.Body.String(), "event: error") {
				t.Fatalf("valid SSE lost text or failed: status=%d bytes=%d", w.Code, w.Body.Len())
			}
			if !strings.Contains(w.Body.String(), `"input_tokens":123`) || !strings.Contains(w.Body.String(), `"output_tokens":7`) || !strings.Contains(w.Body.String(), `"stop_reason":"max_tokens"`) {
				t.Fatalf("stream accounting/completion changed: %.800s", w.Body.String())
			}
		})
	}
}

func TestMessagesSSEFailuresNeverReportNormalCompletion(t *testing.T) {
	for name, body := range map[string]string{
		"malformed":            "data:{\n\n",
		"malformed-after-text": "data:" + textChunk("partial") + "\n\ndata:{\n\n",
		"upstream-error":       "data:" + textChunk("partial") + "\n\nevent:error\ndata:{\"error\":{\"message\":\"synthetic-secret-detail\"}}\n\n",
		"truncated-event":      "data:" + textChunk("partial") + "\n\ndata:{\"choices\":",
		"no-finish":            "data:" + textChunk("partial") + "\n\ndata:[DONE]\n\n",
		"oversize":             "data:" + textChunk(strings.Repeat("x", (1<<20)+1)) + "\n\n",
		"tool-call":            "data:{\"choices\":[{\"delta\":{\"tool_calls\":[{\"id\":\"synthetic\"}]},\"finish_reason\":\"tool_calls\"}]}\n\n",
	} {
		t.Run(name, func(t *testing.T) {
			w := messageRequest(t, streamingFixture(t, body), true, nil)
			if strings.Contains(w.Body.String(), "event: message_stop") || (!strings.Contains(w.Body.String(), "event: error") && w.Code != 502) {
				t.Fatalf("broken stream reported success: %d %.600s", w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "synthetic-secret-detail") {
				t.Fatal("stream error echoed arbitrary upstream data")
			}
		})
	}
}

func TestMessagesSSEEncodesModel(t *testing.T) {
	model := "unusual\"model\nname"
	chunk, _ := json.Marshal(map[string]any{"model": model, "choices": []any{map[string]any{"delta": map[string]string{"content": "ok"}, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 1, "completion_tokens": 1}})
	w := messageRequest(t, streamingFixture(t, "data:"+string(chunk)+"\n\ndata:[DONE]\n\n"), true, func(p map[string]any) { p["model"] = model })
	for _, line := range strings.Split(w.Body.String(), "\n") {
		if strings.HasPrefix(line, "data:") {
			if !json.Valid([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:")))) {
				t.Fatalf("invalid event JSON: %s", line)
			}
		}
	}
	if !strings.Contains(w.Body.String(), `"model":"unusual\"model\nname"`) || !strings.Contains(w.Body.String(), "event: message_stop") {
		t.Fatalf("model identity not encoded: %s", w.Body.String())
	}
}

// A broken HTTP stream must not be converted to a complete Messages response.
func TestMessagesSSEPropagatesHTTPReadFailure(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Content-Length", "99999")
		fmt.Fprint(w, "data:"+textChunk("partial")+"\n\n")
	}))
	defer up.Close()
	s := messageFixture(t, func(w http.ResponseWriter, r *http.Request) {})
	// Use a real HTTP server advertising a longer body than it writes.
	s.client = valclient.NewClient(up.URL, "synthetic")
	w := messageRequest(t, s, true, nil)
	if strings.Contains(w.Body.String(), "event: message_stop") || (!strings.Contains(w.Body.String(), "event: error") && w.Code != 502) {
		t.Fatalf("read failure became complete: %s", w.Body.String())
	}
}

func TestMessagesSSEOptionalArrayAndRefusal(t *testing.T) {
	for _, tc := range []struct {
		name, delta string
		valid       bool
	}{
		{"empty array", `{"content":"Hello","tool_calls":[ ]}`, true},
		{"refusal", `{"refusal":"Synthetic refusal"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `data:{"choices":[{"delta":` + tc.delta + `,"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}` + "\n\ndata:[DONE]\n\n"
			w := messageRequest(t, streamingFixture(t, body), true, nil)
			completed := strings.Contains(w.Body.String(), "event: message_stop")
			if completed != tc.valid {
				t.Fatalf("valid=%v, completed=%v: %s", tc.valid, completed, w.Body.String())
			}
		})
	}
}

type cancelOnWrite struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
}

func (w *cancelOnWrite) Write(p []byte) (int, error) {
	n, err := w.ResponseRecorder.Write(p)
	w.cancel()
	return n, err
}

func TestMessagesSSECancellationReachesUpstream(t *testing.T) {
	upstreamCanceled := make(chan bool, 1)
	upstreamStarted := make(chan struct{})
	s := messageFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data:"+textChunk("partial")+"\n\n")
		w.(http.Flusher).Flush()
		close(upstreamStarted)
		select {
		case <-r.Context().Done():
			upstreamCanceled <- true
		case <-time.After(3 * time.Second):
			upstreamCanceled <- false
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { <-upstreamStarted; cancel() }()
	r := httptest.NewRequest("POST", "http://127.0.0.1:8765/v1/messages", strings.NewReader(`{"model":"synthetic","max_tokens":32,"messages":[{"role":"user","content":"Hello"}],"stream":true}`)).WithContext(ctx)
	w := &cancelOnWrite{ResponseRecorder: httptest.NewRecorder(), cancel: cancel}
	s.handleMessages(w, r)
	if !<-upstreamCanceled {
		t.Fatal("client cancellation left the upstream stream running")
	}
	if strings.Contains(w.Body.String(), "event: message_stop") {
		t.Fatal("canceled stream reported successful completion")
	}
}

package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestOpenAIClientRegistryAndResponsePassThrough(t *testing.T) {
	for _, route := range []string{"/v1/chat/completions", "/chat/completions"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%v", route, stream), func(t *testing.T) {
				reply := `{"id":"synthetic","choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_echo","type":"function","function":{"name":"compat_echo","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":9,"completion_tokens":3}}`
				if stream {
					reply = "data:" + reply + "\n\ndata:[DONE]\n\n"
				}
				s := messageFixture(t, func(w http.ResponseWriter, r *http.Request) {
					var request map[string]json.RawMessage
					json.NewDecoder(r.Body).Decode(&request)
					if request["tool_servers"] == nil {
						t.Error("OpenAI route omitted native caller-tool registration")
					}
					contentType := "application/json"
					if stream {
						contentType = "text/event-stream"
					}
					w.Header().Set("Content-Type", contentType)
					fmt.Fprint(w, reply)
				})
				input := fmt.Sprintf(`{"model":"synthetic","stream":%v,"messages":[{"role":"user","content":"Call compat_echo"}],"tools":[{"type":"function","function":{"name":"compat_echo","parameters":{"type":"object"}}}]}`, stream)
				w := authCall(s, "POST", route, input, map[string]string{"Authorization": "Bearer local-owner"})
				if w.Code != 200 || w.Body.String() != reply || (stream && !w.Flushed) {
					t.Fatalf("response changed: HTTP %d %s", w.Code, w.Body.String())
				}
			})
		}
	}
}

func TestOpenAIForwardsFailureAndRetryDelay(t *testing.T) {
	for _, status := range []int{401, 429, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			s := messageFixture(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "17")
				w.WriteHeader(status)
				fmt.Fprint(w, `{"error":{"message":"synthetic failure","type":"rate_limit_error"}}`)
			})
			w := authCall(s, "POST", "/v1/chat/completions", `{}`, map[string]string{"Authorization": "Bearer local-owner"})
			if w.Code != status || w.Header().Get("Retry-After") != "17" || !strings.Contains(w.Body.String(), "synthetic failure") {
				t.Fatalf("error or retry contract lost: HTTP %d, retry %q, %s", w.Code, w.Header().Get("Retry-After"), w.Body.String())
			}
		})
	}
}

func TestOpenAIRejectsOversizeBeforeUpstream(t *testing.T) {
	var calls atomic.Int32
	s := messageFixture(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1) })
	w := authCall(s, "POST", "/v1/chat/completions", strings.Repeat(" ", (32<<20)+1), map[string]string{"Authorization": "Bearer local-owner"})
	if w.Code != 413 || calls.Load() != 0 || !strings.Contains(w.Body.String(), "request_too_large") {
		t.Fatalf("oversize contract lost: HTTP %d calls %d %s", w.Code, calls.Load(), w.Body.String())
	}
}

func TestOpenAIPostRedirectDoesNotReplayCallerBody(t *testing.T) {
	for _, status := range []int{307, 308} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var calls atomic.Int32
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
			defer target.Close()
			const reply = `{"error":{"message":"synthetic redirect"}}`
			s := messageFixture(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", target.URL)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				fmt.Fprint(w, reply)
			})
			w := authCall(s, "POST", "/v1/chat/completions", `{"messages":[{"role":"user","content":"private synthetic prompt"}]}`, map[string]string{"Authorization": "Bearer local-owner"})
			if w.Code != status || calls.Load() != 0 || w.Body.String() != reply {
				t.Fatalf("POST redirect replayed: HTTP %d target_calls=%d", w.Code, calls.Load())
			}
		})
	}
}

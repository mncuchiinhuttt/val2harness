package proxy

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestQuotaObservationAcrossProtocols(t *testing.T) {
	for _, protocol := range []string{"openai", "anthropic"} {
		for _, mode := range []string{"json", "stream", "stream-empty-tail"} {
			t.Run(protocol+"/"+mode, func(t *testing.T) {
				reply, contentType := textResponse, "application/json"
				if mode != "json" {
					finish := `{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`
					tail := ""
					if mode == "stream-empty-tail" {
						tail = "data:{\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"\"}}]}\n\n"
					}
					reply = "data:" + textChunk("Hello") + "\n\ndata:" + finish + "\n\n" + tail + "data:{\"choices\":[],\"usage\":{\"prompt_tokens\":123,\"completion_tokens\":7}}\n\ndata:[DONE]\n\n"
					contentType = "text/event-stream"
				}
				s := messageFixture(t, func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", contentType)
					fmt.Fprint(w, reply)
				})
				if protocol == "anthropic" {
					w := messageRequest(t, s, mode != "json", nil)
					if w.Code != 200 {
						t.Fatalf("Messages failed: HTTP %d %s", w.Code, w.Body.String())
					}
				} else {
					w := authCall(s, "POST", "/v1/chat/completions", `{ "model":"synthetic", "messages":[{"role":"user","content":"Hello"}] }`, map[string]string{"Authorization": "Bearer local-owner"})
					if w.Code != 200 || w.Body.String() != reply {
						t.Fatal("raw completion contract changed")
					}
				}
				q := s.client.UsageTracker().SnapshotQuota()
				if q.Status != "available" || q.ObservedRequests != 1 || q.MeasuredRequests != 1 || q.PromptTokens != 123 || q.CompletionTokens != 7 || q.TotalTokens != 130 {
					t.Fatalf("completed %s %s was not measured exactly once: %+v", protocol, mode, q)
				}
				logs := s.client.UsageTracker().GetSummary().RecentLogs
				if len(logs) != 1 || logs[0].Protocol != protocol {
					t.Fatalf("protocol attribution: %+v", logs)
				}
			})
		}
	}
}

func TestBudgetErrorIs429ForBothProtocols(t *testing.T) {
	for _, protocol := range []string{"openai", "anthropic"} {
		t.Run(protocol, func(t *testing.T) {
			s := messageFixture(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "17")
				fmt.Fprint(w, `{"error":{"code":"budget_exceeded","message":"synthetic only"}}`)
			})
			if protocol == "anthropic" {
				w := messageRequest(t, s, false, nil)
				if w.Code != 429 || w.Header().Get("Retry-After") != "17" {
					t.Fatalf("Messages budget status: %d", w.Code)
				}
			} else {
				w := authCall(s, "POST", "/v1/chat/completions", `{}`, map[string]string{"Authorization": "Bearer local-owner"})
				if w.Code != 429 || w.Header().Get("Retry-After") != "17" || !strings.Contains(w.Body.String(), "budget_exceeded") {
					t.Fatalf("OpenAI budget status: %d", w.Code)
				}
			}
			if q := s.client.UsageTracker().SnapshotQuota(); q.Status != "exhausted" || q.MeasuredRequests != 0 {
				t.Fatalf("budget observation: %+v", q)
			}
		})
	}
}

func TestQuotaDoesNotCallAnEmptyMessageAvailable(t *testing.T) {
	s := messageFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":123,"completion_tokens":7}}`)
	})
	authCall(s, "POST", "/v1/chat/completions", `{}`, map[string]string{"Authorization": "Bearer local-owner"})
	if q := s.client.UsageTracker().SnapshotQuota(); q.Status == "available" || q.MeasuredRequests != 0 {
		t.Fatalf("malformed completion invented availability: %+v", q)
	}
}

package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"val2harness/internal/config"
	"val2harness/internal/valclient"
)

func messageFixture(t *testing.T, handler http.HandlerFunc) *Server {
	t.Helper()
	up := httptest.NewServer(handler)
	t.Cleanup(up.Close)
	cfg := config.DefaultConfig()
	cfg.APIKey = "local-owner"
	cfg.Token = "synthetic"
	cfg.UpstreamURL = up.URL
	return NewServer(cfg, valclient.NewClient(up.URL, cfg.Token))
}
func messageRequest(t *testing.T, s *Server, stream bool, change func(map[string]any)) *httptest.ResponseRecorder {
	t.Helper()
	payload := map[string]any{"model": "anthropic-claude-opus-4-7", "messages": []any{map[string]any{"role": "user", "content": "Hello"}}, "max_tokens": 32, "stream": stream}
	if change != nil {
		change(payload)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return authCall(s, "POST", "/v1/messages", string(body), map[string]string{"Authorization": "Bearer local-owner"})
}

const textResponse = `{"id":"synthetic-response","model":"upstream-actual","choices":[{"message":{"role":"assistant","content":"Hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":123,"completion_tokens":7}}`

func TestMessagesPreserveHTTPFailures(t *testing.T) {
	for _, status := range []int{401, 429, 503} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/stream=%v", status, stream), func(t *testing.T) {
				s := messageFixture(t, func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Retry-After", "17")
					w.WriteHeader(status)
					fmt.Fprint(w, `{"error":"synthetic-private-upstream-detail"}`)
				})
				w := messageRequest(t, s, stream, nil)
				if w.Code != status {
					t.Fatalf("upstream %d became %d: %s", status, w.Code, w.Body.String())
				}
				var result map[string]any
				if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result["type"] != "error" {
					t.Fatalf("not a structured Messages error: %s", w.Body.String())
				}
				if w.Header().Get("Retry-After") != "17" || strings.Contains(w.Body.String(), "synthetic-private") {
					t.Fatal("lost retry guidance or echoed arbitrary upstream data")
				}
			})
		}
	}
}

func TestMessagesRejectUnsupportedAndInvalidRequestsBeforeUpstream(t *testing.T) {
	changes := map[string]func(map[string]any){
		"tools": func(p map[string]any) { p["tools"] = []any{map[string]any{"name": "read_file"}} },
		"tool_result": func(p map[string]any) {
			p["messages"] = []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": "synthetic", "content": "value"}}}}
		},
		"file_image": func(p map[string]any) {
			p["messages"] = []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "image", "source": map[string]any{"type": "file", "file_id": "requires-files-service"}}}}}
		},
		"thinking":         func(p map[string]any) { p["thinking"] = map[string]any{"type": "enabled", "budget_tokens": 16} },
		"unknown_option":   func(p map[string]any) { p["container"] = "requires-server-service" },
		"missing_model":    func(p map[string]any) { delete(p, "model") },
		"missing_limit":    func(p map[string]any) { delete(p, "max_tokens") },
		"negative_limit":   func(p map[string]any) { p["max_tokens"] = -1 },
		"missing_messages": func(p map[string]any) { p["messages"] = []any{} },
		"wrong_role": func(p map[string]any) {
			p["messages"] = []any{map[string]any{"role": "system", "content": "hidden instruction"}}
		},
		"invalid_content": func(p map[string]any) { p["messages"] = []any{map[string]any{"role": "user", "content": 123}} },
		"unsupported_message_option": func(p map[string]any) {
			p["messages"] = []any{map[string]any{"role": "user", "content": "Hello", "name": "silently discarded"}}
		},
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			called := false
			s := messageFixture(t, func(w http.ResponseWriter, r *http.Request) { called = true; fmt.Fprint(w, textResponse) })
			w := messageRequest(t, s, false, change)
			if w.Code != 400 || called {
				t.Fatalf("unsupported/invalid input reached upstream or returned success: %d called=%v", w.Code, called)
			}
		})
	}
}

func TestMessagesPreserveLimitsModelUsageAndStopReason(t *testing.T) {
	for _, finish := range []string{"stop", "length"} {
		t.Run(finish, func(t *testing.T) {
			var captured map[string]any
			s := messageFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
					t.Error(err)
				}
				fmt.Fprint(w, strings.Replace(textResponse, `"finish_reason":"stop"`, `"finish_reason":"`+finish+`"`, 1))
			})
			requested := "custom-model-with-\"quote"
			w := messageRequest(t, s, false, func(p map[string]any) {
				p["model"] = requested
				p["temperature"] = 0.25
				p["system"] = []any{map[string]any{"type": "text", "text": "Rules"}}
			})
			if w.Code != 200 {
				t.Fatal(w.Body.String())
			}
			if captured["max_tokens"] != float64(32) || captured["model"] != requested || captured["temperature"] != 0.25 {
				t.Fatalf("request contract changed: %v", captured)
			}
			var result struct {
				Model string         `json:"model"`
				Stop  string         `json:"stop_reason"`
				Usage map[string]int `json:"usage"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			wantStop := "end_turn"
			if finish == "length" {
				wantStop = "max_tokens"
			}
			if result.Model != "upstream-actual" || result.Stop != wantStop || result.Usage["input_tokens"] != 123 || result.Usage["output_tokens"] != 7 {
				t.Fatalf("response accounting or identity fabricated: %s", w.Body.String())
			}
		})
	}
}

func TestMessagesRejectMalformedOrUnsupportedUpstreamResponses(t *testing.T) {
	for _, body := range []string{`{`, `{"choices":[]}`, `{"choices":[{"message":{"content":null},"finish_reason":"stop"}]}`, `{"choices":[{"message":{"content":"partial"},"finish_reason":null}]}`, `{"choices":[{"message":{"content":"","tool_calls":[{"id":"synthetic"}]},"finish_reason":"tool_calls"}]}`} {
		t.Run(body, func(t *testing.T) {
			s := messageFixture(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) })
			if w := messageRequest(t, s, false, nil); w.Code != 502 {
				t.Fatalf("invalid upstream became success: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestMessagesMissingUsageIsExplicitlyUnavailable(t *testing.T) {
	s := messageFixture(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"Hello"},"finish_reason":"stop"}]}`)
	})
	w := messageRequest(t, s, false, nil)
	var result map[string]any
	json.Unmarshal(w.Body.Bytes(), &result)
	if w.Code != 502 {
		t.Fatal(w.Body.String())
	}
	if _, exists := result["usage"]; exists || w.Header().Get("X-Val2harness-Usage") != "unavailable" {
		t.Fatalf("missing accounting represented as measured usage: %s", w.Body.String())
	}
}

func TestMessagesMissingValTokenDoesNotCallUpstream(t *testing.T) {
	called := false
	s := messageFixture(t, func(w http.ResponseWriter, r *http.Request) { called = true; fmt.Fprint(w, textResponse) })
	s.client.SetToken("")
	if w := messageRequest(t, s, false, nil); w.Code != 401 || called {
		t.Fatal("missing Val token reached upstream or returned success")
	}
}

func TestMessagesExactModelAliasesAndZeroUsage(t *testing.T) {
	for input, want := range map[string]string{"claude-sonnet-4-6": "anthropic-claude-sonnet-4-6", "my-opus-service": "my-opus-service", "unknown": "unknown"} {
		s := messageFixture(t, func(w http.ResponseWriter, r *http.Request) {
			var got map[string]any
			json.NewDecoder(r.Body).Decode(&got)
			if got["model"] != want {
				t.Errorf("model %s became %v", input, got["model"])
			}
			fmt.Fprint(w, `{"choices":[{"message":{"content":"Hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":0,"completion_tokens":0}}`)
		})
		w := messageRequest(t, s, false, func(p map[string]any) { p["model"] = input })
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"input_tokens":0`) || !strings.Contains(w.Body.String(), `"output_tokens":0`) {
			t.Fatal("genuine zero usage was not preserved")
		}
	}
}

func TestMessagesEmptyOptionalArraysIgnoreJSONWhitespace(t *testing.T) {
	s := messageFixture(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"Hello","tool_calls":[ ]},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	})
	if w := messageRequest(t, s, false, nil); w.Code != 200 {
		t.Fatalf("whitespace in empty optional data broke text: %s", w.Body.String())
	}
}
func TestMessagesRefusalIsNotDropped(t *testing.T) {
	s := messageFixture(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"","refusal":"Synthetic refusal"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	})
	if w := messageRequest(t, s, false, nil); w.Code != 200 || !strings.Contains(w.Body.String(), `"stop_reason":"refusal"`) || !strings.Contains(w.Body.String(), "Synthetic refusal") {
		t.Fatalf("refusal became empty successful text: %s", w.Body.String())
	}
}

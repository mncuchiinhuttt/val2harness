package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

const tinyPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a8ncAAAAASUVORK5CYII="

func toolDefinition(name string) map[string]any {
	return map[string]any{"name": name, "description": "Read a synthetic fixture", "input_schema": map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}}}
}
func toolReply() string {
	return `{"model":"synthetic-agent","choices":[{"message":{"content":"Checking files","tool_calls":[{"id":"call_a","type":"function","function":{"name":"Read","arguments":"{\"path\":\"fixture.txt\",\"precise\":9007199254740993}"}},{"id":"call_b","type":"function","function":{"name":"Inspect","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":30,"completion_tokens":12}}`
}

func TestAgentToolDefinitionsAndParallelCalls(t *testing.T) {
	var captured map[string]any
	s := messageFixture(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&captured)
		fmt.Fprint(w, toolReply())
	})
	w := messageRequest(t, s, false, func(p map[string]any) {
		p["tools"] = []any{toolDefinition("Read"), toolDefinition("Inspect")}
		p["tool_choice"] = map[string]any{"type": "any", "disable_parallel_tool_use": false}
		p["top_p"] = 0.9
	})
	if w.Code != 200 {
		t.Fatalf("agent tools rejected: %d %s", w.Code, w.Body.String())
	}
	tools := captured["tools"].([]any)
	function := tools[0].(map[string]any)["function"].(map[string]any)
	if function["name"] != "Read" || function["parameters"] == nil || captured["tool_choice"] != "required" || captured["parallel_tool_calls"] != true || captured["top_p"] != 0.9 {
		t.Fatalf("tool controls lost: %v", captured)
	}
	var result map[string]any
	json.Unmarshal(w.Body.Bytes(), &result)
	content := result["content"].([]any)
	if result["stop_reason"] != "tool_use" || len(content) != 3 || content[1].(map[string]any)["id"] != "call_a" || content[2].(map[string]any)["name"] != "Inspect" {
		t.Fatalf("parallel tool response corrupted: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "9007199254740993") {
		t.Fatal("tool argument integer precision lost")
	}
}

func TestAgentToolResultsImagesAndPortableOptions(t *testing.T) {
	var captured map[string]any
	s := messageFixture(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&captured)
		fmt.Fprint(w, textResponse)
	})
	w := messageRequest(t, s, false, func(p map[string]any) {
		p["tools"] = []any{toolDefinition("Read")}
		p["metadata"] = map[string]any{"user_id": "synthetic-user"}
		p["thinking"] = map[string]any{"type": "disabled"}
		p["stop_sequences"] = []string{"END"}
		p["system"] = []any{map[string]any{"type": "text", "text": "System rules", "cache_control": map[string]string{"type": "ephemeral"}}}
		p["messages"] = []any{
			map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "Look"}, map[string]any{"type": "image", "source": map[string]string{"type": "base64", "media_type": "image/png", "data": tinyPNG}}}},
			map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "tool_use", "id": "call_a", "name": "Read", "input": map[string]any{"path": "fixture.txt"}}}},
			map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": "call_a", "is_error": true, "content": []any{map[string]any{"type": "text", "text": "synthetic failure"}, map[string]any{"type": "image", "source": map[string]string{"type": "url", "url": "https://example.invalid/result.png"}}}}, map[string]any{"type": "text", "text": "Continue"}}},
		}
	})
	if w.Code != 200 {
		t.Fatalf("tool-result/image flow rejected: %s", w.Body.String())
	}
	encoded, _ := json.Marshal(captured)
	for _, expected := range []string{`"role":"tool"`, `"tool_call_id":"call_a"`, "data:image/png;base64," + tinyPNG, "https://example.invalid/result.png", "synthetic failure", "Continue", `"stop":["END"]`} {
		if !strings.Contains(string(encoded), expected) {
			t.Errorf("lost %s: %s", expected, string(encoded))
		}
	}
	if strings.Contains(string(encoded), "synthetic-user") {
		t.Fatal("unneeded client identity forwarded to Val")
	}
	if w.Header().Get("X-Val2harness-Compatibility") == "" {
		t.Fatal("ignored cache/metadata hints were not disclosed")
	}
}

func TestAgentRejectsInvalidHistoryAndUnsafeImageShapes(t *testing.T) {
	for name, content := range map[string]any{
		"orphan-result":  []any{map[string]any{"type": "tool_result", "tool_use_id": "missing", "content": "value"}},
		"invalid-base64": []any{map[string]any{"type": "image", "source": map[string]string{"type": "base64", "media_type": "image/png", "data": "not-base64"}}},
		"file-url":       []any{map[string]any{"type": "image", "source": map[string]string{"type": "url", "url": "file:///private.png"}}},
		"bad-mime":       []any{map[string]any{"type": "image", "source": map[string]string{"type": "base64", "media_type": "text/html", "data": tinyPNG}}},
	} {
		t.Run(name, func(t *testing.T) {
			called := false
			s := messageFixture(t, func(w http.ResponseWriter, r *http.Request) { called = true; fmt.Fprint(w, textResponse) })
			w := messageRequest(t, s, false, func(p map[string]any) { p["messages"] = []any{map[string]any{"role": "user", "content": content}} })
			if w.Code != 400 || called {
				t.Fatalf("invalid input reached upstream: %d %v", w.Code, called)
			}
		})
	}
}

func TestAgentStreamingParallelToolArguments(t *testing.T) {
	frames := []string{
		`{"model":"synthetic-agent","choices":[{"delta":{"role":"assistant","content":"Checking "}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"Read","arguments":"{\"path\":"}},{"index":1,"id":"call_b","type":"function","function":{"name":"Inspect","arguments":"{"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":1,"function":{"arguments":"}"}},{"index":0,"function":{"arguments":"\"fixture.txt\"}"}}]}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":30,"completion_tokens":12}}`,
		`[DONE]`,
	}
	body := ""
	for _, frame := range frames {
		body += "data:" + frame + "\n\n"
	}
	w := messageRequest(t, streamingFixture(t, body), true, func(p map[string]any) { p["tools"] = []any{toolDefinition("Read"), toolDefinition("Inspect")} })
	if w.Code != 200 || !strings.Contains(w.Body.String(), "event: message_stop") {
		t.Fatalf("streamed tool loop failed: %d %s", w.Code, w.Body.String())
	}
	for _, expected := range []string{`"type":"tool_use"`, `"type":"input_json_delta"`, `"id":"call_a"`, `"id":"call_b"`, `"stop_reason":"tool_use"`} {
		if !strings.Contains(w.Body.String(), expected) {
			t.Errorf("stream lost %s: %s", expected, w.Body.String())
		}
	}
}

func TestAgentOutputControls(t *testing.T) {
	var captured map[string]any
	s := messageFixture(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&captured)
		fmt.Fprint(w, textResponse)
	})
	w := messageRequest(t, s, false, func(p map[string]any) {
		p["output_config"] = map[string]any{"effort": "medium", "format": map[string]any{"type": "json_schema", "schema": map[string]any{"type": "object", "properties": map[string]any{}}}}
	})
	if w.Code != 200 || captured["reasoning_effort"] != nil || captured["response_format"] == nil || !strings.Contains(w.Header().Get("X-Val2harness-Compatibility"), "response-effort-hint-not-forwarded") {
		t.Fatalf("portable output controls were rejected/lost: %d %s", w.Code, w.Body.String())
	}
}

func TestAgentEffortDoesNotActivateUnsupportedThinking(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, explicitDisabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("stream=%v/disabled=%v", stream, explicitDisabled), func(t *testing.T) {
				s := messageFixture(t, func(w http.ResponseWriter, r *http.Request) {
					var upstream map[string]any
					json.NewDecoder(r.Body).Decode(&upstream)
					if upstream["reasoning_effort"] != nil || upstream["thinking"] != nil {
						// A reasoning provider may emit unsigned thinking when this
						// unrelated parameter is sent, which the adapter must reject.
						http.Error(w, "unexpected reasoning activation", 400)
						return
					}
					if stream {
						w.Header().Set("Content-Type", "text/event-stream")
						fmt.Fprint(w, "data:{\"model\":\"synthetic-agent\",\"choices\":[{\"delta\":{\"content\":\"Ready\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":1}}\n\ndata:[DONE]\n\n")
					} else {
						fmt.Fprint(w, textResponse)
					}
				})
				w := messageRequest(t, s, stream, func(p map[string]any) {
					p["output_config"] = map[string]any{"effort": "high"}
					if explicitDisabled {
						p["thinking"] = map[string]any{"type": "disabled"}
					}
				})
				if w.Code != 200 || !strings.Contains(w.Header().Get("X-Val2harness-Compatibility"), "response-effort-hint-not-forwarded") {
					t.Fatalf("effort activated reasoning or was silently dropped: %d %s", w.Code, w.Body.String())
				}
			})
		}
	}
}

func TestAgentNullableOutputAndHighEffort(t *testing.T) {
	for _, output := range []map[string]any{{"effort": nil, "format": nil}, {"effort": "xhigh"}} {
		s := messageFixture(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, textResponse) })
		if w := messageRequest(t, s, false, func(p map[string]any) { p["output_config"] = output }); w.Code != 200 {
			t.Fatalf("documented output option rejected: %s", w.Body.String())
		}
	}
}

func TestAgentToolArgumentsCannotChangeIdentityOrExecuteUndeclaredCalls(t *testing.T) {
	for name, deltas := range map[string][]string{
		"undeclared":   {`{"index":0,"id":"call_a","type":"function","function":{"name":"Hidden","arguments":"{}"}}`},
		"incomplete":   {`{"index":0,"id":"call_a","type":"function","function":{"name":"Read","arguments":"{"}}`},
		"scalar":       {`{"index":0,"id":"call_a","type":"function","function":{"name":"Read","arguments":"null"}}`},
		"changed-id":   {`{"index":0,"id":"call_a","type":"function","function":{"name":"Read","arguments":"{"}}`, `{"index":0,"id":"call_b","function":{"arguments":"}"}}`},
		"duplicate-id": {`{"index":0,"id":"call_a","type":"function","function":{"name":"Read","arguments":"{}"}}`, `{"index":1,"id":"call_a","type":"function","function":{"name":"Read","arguments":"{}"}}`},
	} {
		t.Run(name, func(t *testing.T) {
			body := ""
			for _, delta := range deltas {
				body += `data:{"choices":[{"delta":{"tool_calls":[` + delta + `]}}]}` + "\n\n"
			}
			body += `data:{"choices":[{"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}` + "\n\ndata:[DONE]\n\n"
			w := messageRequest(t, streamingFixture(t, body), true, func(p map[string]any) { p["tools"] = []any{toolDefinition("Read")} })
			if w.Code != 502 || strings.Contains(w.Body.String(), "event: message_stop") || strings.Contains(w.Body.String(), "event: content_block_start") {
				t.Fatalf("unsafe call delivered: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestAgentEqualToolNameFragmentsRemainDistinct(t *testing.T) {
	body := `data:{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"a","arguments":"{"}}]}}]}` + "\n\n" + `data:{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"a","arguments":"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}` + "\n\ndata:[DONE]\n\n"
	w := messageRequest(t, streamingFixture(t, body), true, func(p map[string]any) { p["tools"] = []any{toolDefinition("a"), toolDefinition("aa")} })
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"name":"aa"`) {
		t.Fatalf("name fragments changed tool identity: %s", w.Body.String())
	}
}

func TestAgentResponseEnforcesToolChoice(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, tc := range []struct {
			name   string
			choice map[string]any
			names  []string
		}{
			{"none", map[string]any{"type": "none"}, []string{"Read"}},
			{"specific", map[string]any{"type": "tool", "name": "Read"}, []string{"Inspect"}},
			{"single", map[string]any{"type": "auto", "disable_parallel_tool_use": true}, []string{"Read", "Inspect"}},
		} {
			t.Run(fmt.Sprintf("%s/stream=%v", tc.name, stream), func(t *testing.T) {
				calls := []any{}
				for index, name := range tc.names {
					calls = append(calls, map[string]any{"index": index, "id": fmt.Sprintf("call_%d", index), "type": "function", "function": map[string]string{"name": name, "arguments": "{}"}})
				}
				payload := map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": "", "tool_calls": calls}, "finish_reason": "tool_calls"}}, "usage": map[string]int{"prompt_tokens": 1, "completion_tokens": 1}}
				if stream {
					payload["choices"] = []any{map[string]any{"delta": map[string]any{"tool_calls": calls}, "finish_reason": "tool_calls"}}
				}
				encoded, _ := json.Marshal(payload)
				s := messageFixture(t, func(w http.ResponseWriter, r *http.Request) {
					if stream {
						w.Header().Set("Content-Type", "text/event-stream")
						fmt.Fprintf(w, "data:%s\n\ndata:[DONE]\n\n", encoded)
					} else {
						w.Write(encoded)
					}
				})
				w := messageRequest(t, s, stream, func(p map[string]any) {
					p["tools"] = []any{toolDefinition("Read"), toolDefinition("Inspect")}
					p["tool_choice"] = tc.choice
				})
				if w.Code != 502 || strings.Contains(w.Body.String(), "event: content_block_start") {
					t.Fatalf("tool-choice constraint ignored: %d %s", w.Code, w.Body.String())
				}
			})
		}
	}
}

func TestAgentEmptyFilteredCompletionIsARefusal(t *testing.T) {
	s := messageFixture(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":null},"finish_reason":"content_filter"}],"usage":{"prompt_tokens":1,"completion_tokens":0}}`)
	})
	w := messageRequest(t, s, false, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"stop_reason":"refusal"`) {
		t.Fatalf("valid refusal became transport failure: %s", w.Body.String())
	}
}

func TestAgentAllowsAnExplicitlyEmptyToolResult(t *testing.T) {
	s := messageFixture(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, textResponse) })
	w := messageRequest(t, s, false, func(p map[string]any) {
		p["messages"] = []any{
			map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "tool_use", "id": "call_empty", "name": "Read", "input": map[string]any{}}}},
			map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": "call_empty", "content": []any{}}}},
		}
	})
	if w.Code != 200 {
		t.Fatalf("empty completed result was rejected: %s", w.Body.String())
	}
}

func TestAgentTranslatesActualCachedUsage(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			s := messageFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, `data:{"choices":[{"delta":{"content":"Hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":7,"prompt_tokens_details":{"cached_tokens":80}}}`+"\n\ndata:[DONE]\n\n")
				} else {
					fmt.Fprint(w, `{"choices":[{"message":{"content":"Hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":7,"prompt_tokens_details":{"cached_tokens":80}}}`)
				}
			})
			w := messageRequest(t, s, stream, nil)
			if w.Code != 200 || !strings.Contains(w.Body.String(), `"input_tokens":20`) || !strings.Contains(w.Body.String(), `"cache_read_input_tokens":80`) {
				t.Fatalf("cached input accounting changed: %s", w.Body.String())
			}
		})
	}
}

func TestAgentReportsUpstreamToolInjectionWithoutDeliveringCalls(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			s := messageFixture(t, func(w http.ResponseWriter, r *http.Request) {
				call := `{"id":"call_server","index":0,"type":"function","function":{"name":"search_chats","arguments":"{}"}}`
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, `data:{"choices":[{"delta":{"tool_calls":[`+call+`]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`+"\n\ndata:[DONE]\n\n")
				} else {
					fmt.Fprint(w, `{"choices":[{"message":{"content":null,"tool_calls":[`+call+`]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
				}
			})
			w := messageRequest(t, s, stream, func(p map[string]any) { p["tools"] = []any{toolDefinition("Read")} })
			if w.Code != 502 || !strings.Contains(w.Body.String(), "not declared by this client") || strings.Contains(w.Body.String(), "event: content_block_start") || strings.Contains(w.Body.String(), "event: message_stop") {
				t.Fatalf("upstream tool injection was unclear or executable: HTTP %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestAgentNoToolsDoesNotImposeUnsupportedNoneChoice(t *testing.T) {
	var payload map[string]any
	s := messageFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		// Val's Sonnet backend rejected an automatically imposed none choice.
		// Preserve caller intent without adding that unsupported provider flag.
		if payload["tool_choice"] == "none" {
			http.Error(w, "unsupported tool choice", 400)
			return
		}
		fmt.Fprint(w, textResponse)
	})
	w := messageRequest(t, s, false, nil)
	if w.Code != 200 {
		t.Fatalf("no-tool turn received an unsupported automatic choice: HTTP %d %s", w.Code, w.Body.String())
	}
	if _, imposed := payload["tool_choice"]; imposed {
		t.Fatalf("unexpected automatic tool choice: %v", payload["tool_choice"])
	}
}

func TestAgentRegistersClientSchemasForValNativePipeline(t *testing.T) {
	var request map[string]any
	s := messageFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		// Val's native pipeline resolves its own registry. A normal OpenAI
		// tools array alone was replaced by built-ins in the live failure.
		if _, registered := request["tool_servers"]; !registered {
			fmt.Fprint(w, `{"choices":[{"message":{"content":null,"tool_calls":[{"id":"call_builtin","type":"function","function":{"name":"search_chats","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
			return
		}
		fmt.Fprint(w, toolReply())
	})
	w := messageRequest(t, s, false, func(p map[string]any) {
		p["tools"] = []any{toolDefinition("Read"), toolDefinition("Inspect")}
	})
	if w.Code != 200 {
		t.Fatalf("native registry lost caller tools: HTTP %d %s", w.Code, w.Body.String())
	}
	servers, ok := request["tool_servers"].([]any)
	if !ok || len(servers) != 1 {
		t.Fatalf("missing client specs: %v", request["tool_servers"])
	}
	server := servers[0].(map[string]any)
	if len(server) != 1 || server["specs"] == nil {
		t.Fatalf("client registration contained endpoint or execution configuration: %v", server)
	}
	specs := server["specs"].([]any)
	tools := request["tools"].([]any)
	if len(specs) != len(tools) {
		t.Fatalf("registry count differs: %v", server)
	}
	for i, raw := range tools {
		function := raw.(map[string]any)["function"]
		a, _ := json.Marshal(function)
		b, _ := json.Marshal(specs[i])
		if string(a) != string(b) {
			t.Fatalf("client schema changed in native registration: %s != %s", a, b)
		}
	}
	for _, field := range []string{"tool_ids", "chat_id", "session_id", "id"} {
		if _, exists := request[field]; exists {
			t.Fatalf("registration added server execution state: %s", field)
		}
	}
}

func TestAgentSSEAcceptsEmptyValTailBeforeMeasuredUsage(t *testing.T) {
	for _, tail := range []string{"", "unexpected extra text"} {
		t.Run(fmt.Sprintf("tail=%q", tail), func(t *testing.T) {
			body := `data:{"id":"synthetic","model":"synthetic-agent","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_tail","type":"function","function":{"name":"Read","arguments":"{}"}}]}}]}` + "\n\n" +
				`data:{"id":"synthetic","model":"synthetic-agent","choices":[{"delta":{},"finish_reason":"tool_calls"}]}` + "\n\n"
			encoded, _ := json.Marshal(map[string]any{"id": "synthetic", "model": "synthetic-agent", "choices": []any{map[string]any{"delta": map[string]string{"role": "assistant", "content": tail}}}})
			body += "data:" + string(encoded) + "\n\n" + `data:{"id":"synthetic","model":"synthetic-agent","choices":[{"delta":{}}],"usage":{"prompt_tokens":10,"completion_tokens":4}}` + "\n\ndata:[DONE]\n\n"
			w := messageRequest(t, streamingFixture(t, body), true, func(p map[string]any) { p["tools"] = []any{toolDefinition("Read")} })
			if tail == "" {
				if w.Code != 200 || !strings.Contains(w.Body.String(), `"name":"Read"`) || !strings.Contains(w.Body.String(), `"input_tokens":10`) || !strings.Contains(w.Body.String(), "event: message_stop") {
					t.Fatalf("empty Val envelope rejected or usage lost: HTTP %d %s", w.Code, w.Body.String())
				}
			} else if w.Code != 502 || strings.Contains(w.Body.String(), "event: content_block_start") {
				t.Fatalf("substantive post-finish data became executable: HTTP %d %s", w.Code, w.Body.String())
			}
		})
	}
}

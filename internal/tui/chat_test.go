package tui

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"val2harness/internal/config"
	"val2harness/internal/valclient"
)

func TestTerminalChatFramingAndFailedTurnHistory(t *testing.T) {
	for _, mode := range []string{"no-space", "http-failure", "truncated", "malformed", "done-without-finish", "empty-array", "refusal"} {
		t.Run(mode, func(t *testing.T) {
			var requests []map[string]any
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var p map[string]any
				if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
					t.Error(err)
				}
				requests = append(requests, p)
				if len(requests) == 1 && mode == "http-failure" {
					w.WriteHeader(429)
					fmt.Fprint(w, `{"error":"synthetic-private"}`)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if len(requests) == 1 && mode == "truncated" {
					fmt.Fprint(w, "data:{\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n")
					return
				}
				if len(requests) == 1 && mode == "malformed" {
					fmt.Fprint(w, "data:invalid\n\n")
					return
				}
				if len(requests) == 1 && mode == "done-without-finish" {
					fmt.Fprint(w, "data:{\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\ndata:[DONE]\n\n")
					return
				}
				if len(requests) == 1 && mode == "refusal" {
					fmt.Fprint(w, "data:{\"choices\":[{\"delta\":{\"refusal\":\"Synthetic refusal\"},\"finish_reason\":\"stop\"}]}\n\ndata:[DONE]\n\n")
					return
				}
				if mode == "empty-array" {
					fmt.Fprint(w, "data:{\"choices\":[{\"delta\":{\"content\":\"Synthetic reply\",\"tool_calls\":[ ]},\"finish_reason\":\"stop\"}]}\n\ndata:[DONE]\n\n")
					return
				}
				fmt.Fprint(w, "data:{\"choices\":[{\"delta\":{\"content\":\"Synthetic reply\"},\"finish_reason\":\"stop\"}]}\r\n\r\ndata:[DONE]\r\n\r\n")
			}))
			defer up.Close()
			input, err := os.CreateTemp(t.TempDir(), "input")
			if err != nil {
				t.Fatal(err)
			}
			defer input.Close()
			input.WriteString("first\nsecond\n/exit\n")
			input.Seek(0, 0)
			output, err := os.CreateTemp(t.TempDir(), "output")
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			oldIn, oldOut := os.Stdin, os.Stdout
			os.Stdin, os.Stdout = input, output
			t.Cleanup(func() { os.Stdin, os.Stdout = oldIn, oldOut })
			RunTerminalChat(config.DefaultConfig(), valclient.NewClient(up.URL, "synthetic"), "synthetic-model")
			os.Stdin, os.Stdout = oldIn, oldOut
			output.Seek(0, 0)
			printed, _ := io.ReadAll(output)
			if len(requests) != 2 {
				t.Fatalf("expected two chat calls, got %d", len(requests))
			}
			messages := requests[1]["messages"].([]any)
			if mode == "no-space" || mode == "empty-array" {
				if len(messages) != 3 || messages[1].(map[string]any)["content"] != "Synthetic reply" {
					t.Fatal("valid reply did not enter history")
				}
			} else {
				if len(messages) != 1 || messages[0].(map[string]any)["content"] != "second" {
					t.Fatalf("failed turn entered history: %v", messages)
				}
				if !strings.Contains(string(printed), "Error") {
					t.Fatal("failure did not show a user-visible error")
				}
			}
		})
	}
}

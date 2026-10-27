package proxy

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"val2harness/internal/config"
	"val2harness/internal/valclient"
)

// Opt-in external SDK/CLI fixture. It uses only synthetic account data and
// localhost servers, and stops on a stop file or bounded timeout.
func TestAgentInteropFixture(t *testing.T) {
	dir := os.Getenv("VAL2HARNESS_INTEROP_DIR")
	if dir == "" {
		t.Skip("opt-in SDK/CLI fixture")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	editMode := os.Getenv("VAL2HARNESS_INTEROP_EDIT") == "1"
	marker := "fixture-token-519"
	initial := marker + "\n"
	if editMode {
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			t.Fatal(err)
		}
		marker = "synthetic-edit-" + hex.EncodeToString(random[:])
		initial = "before: " + marker + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "fixture.txt"), []byte(initial), 0600); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	requests := []map[string]any{}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			http.Error(w, "invalid fake request", 400)
			return
		}
		mu.Lock()
		if editMode && len(requests) >= 3 {
			mu.Unlock()
			http.Error(w, "synthetic edit request budget exhausted", http.StatusBadRequest)
			return
		}
		requests = append(requests, request)
		record, _ := json.MarshalIndent(requests, "", "  ")
		os.WriteFile(filepath.Join(dir, "requests.json"), record, 0600)
		mu.Unlock()
		messages, _ := request["messages"].([]any)
		hasResult := false
		hasRead := false
		hasWrite := false
		hasEdit := false
		writeResult := false
		editResult := false
		for _, raw := range messages {
			m, _ := raw.(map[string]any)
			if m["role"] == "tool" && strings.Contains(fmt.Sprint(m["content"]), marker) {
				hasResult = true
			}
			if m["role"] == "tool" && m["tool_call_id"] == "call_write_3" {
				writeResult = true
			}
			if m["role"] == "tool" && m["tool_call_id"] == "call_edit_3" {
				editResult = true
			}
		}
		if tools, ok := request["tools"].([]any); ok {
			for _, raw := range tools {
				tool, _ := raw.(map[string]any)
				function, _ := tool["function"].(map[string]any)
				if function["name"] == "Read" {
					hasRead = true
				}
				if function["name"] == "Write" {
					hasWrite = true
				}
				if function["name"] == "Edit" {
					hasEdit = true
				}
			}
		}
		model, _ := request["model"].(string)
		content := "Synthetic connection works"
		finish := "stop"
		var calls []map[string]any
		if editMode && hasResult && hasEdit && !editResult {
			args, _ := json.Marshal(map[string]string{"file_path": filepath.Join(dir, "fixture.txt"), "old_string": "before:", "new_string": "after:"})
			calls = []map[string]any{{"id": "call_edit_3", "type": "function", "function": map[string]string{"name": "Edit", "arguments": string(args)}}}
			content = "Updating the synthetic fixture."
			finish = "tool_calls"
		} else if editMode && editResult {
			content = "after: " + marker
		} else if hasResult && hasWrite && !writeResult {
			args, _ := json.Marshal(map[string]string{"file_path": filepath.Join(dir, "result.txt"), "content": "fixture-token-519\n"})
			calls = []map[string]any{{"id": "call_write_3", "type": "function", "function": map[string]string{"name": "Write", "arguments": string(args)}}}
			content = "Saving the fixture result."
			finish = "tool_calls"
		} else if hasResult {
			content = "Agent loop verified: " + marker + "."
		} else if hasRead {
			args, _ := json.Marshal(map[string]string{"file_path": filepath.Join(dir, "fixture.txt")})
			calls = []map[string]any{{"id": "call_read_1", "type": "function", "function": map[string]string{"name": "Read", "arguments": string(args)}}}
			if strings.HasPrefix(model, "synthetic-sdk") {
				calls = append(calls, map[string]any{"id": "call_inspect_2", "type": "function", "function": map[string]string{"name": "Inspect", "arguments": "{}"}})
			}
			content = "Checking the fixture."
			finish = "tool_calls"
		}
		usage := map[string]int{"prompt_tokens": 30, "completion_tokens": 12}
		if request["stream"] == true {
			w.Header().Set("Content-Type", "text/event-stream")
			emit := func(v any) { b, _ := json.Marshal(v); fmt.Fprintf(w, "data:%s\n\n", b); w.(http.Flusher).Flush() }
			emit(map[string]any{"id": "synthetic-response", "model": model, "choices": []any{map[string]any{"delta": map[string]string{"role": "assistant", "content": content}}}})
			for index, call := range calls {
				function := call["function"].(map[string]string)
				args := function["arguments"]
				split := len(args) / 2
				emit(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{"index": index, "id": call["id"], "type": "function", "function": map[string]string{"name": function["name"], "arguments": args[:split]}}}}}}})
				emit(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{"index": index, "function": map[string]string{"arguments": args[split:]}}}}}}})
			}
			emit(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{}, "finish_reason": finish}}, "usage": usage})
			fmt.Fprint(w, "data:[DONE]\n\n")
			return
		}
		message := map[string]any{"role": "assistant", "content": content}
		if len(calls) > 0 {
			message["tool_calls"] = calls
		}
		json.NewEncoder(w).Encode(map[string]any{"id": "synthetic-response", "model": model, "choices": []any{map[string]any{"message": message, "finish_reason": finish}}, "usage": usage})
	}))
	defer up.Close()
	cfg := config.DefaultConfig()
	cfg.APIKey = "synthetic-interop-owner"
	cfg.Token = "synthetic-val"
	cfg.UpstreamURL = up.URL
	server := httptest.NewUnstartedServer(nil)
	cfg.ListenAddr = server.Listener.Addr().String()
	server.Config.Handler = NewServer(cfg, valclient.NewClient(up.URL, cfg.Token))
	server.Start()
	defer server.Close()
	metadata := map[string]string{"url": server.URL, "key": cfg.APIKey, "root": dir}
	if editMode {
		metadata["fixture_kind"] = "synthetic-edit-v1"
		metadata["fixture_marker"] = marker
	}
	data, _ := json.Marshal(metadata)
	if err := os.WriteFile(filepath.Join(dir, "gateway.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6000; i++ {
		if _, err := os.Stat(filepath.Join(dir, "stop")); err == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("interop fixture timed out")
}

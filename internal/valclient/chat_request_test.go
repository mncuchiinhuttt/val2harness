package valclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestProxyChatRegistersClientToolSchemas(t *testing.T) {
	input := `{"model":"synthetic","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://example.invalid/image.png"}}]}],"tools":[{"type":"function","function":{"name":"compat_echo","strict":true,"parameters":{"type":"object","properties":{"value":{"const":9007199254740993}}}}}],"tool_choice":"required","parallel_tool_calls":false,"future_option":{"precise":9007199254740993}}`
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat/completions" || r.Header.Get("Authorization") != "Bearer synthetic" {
			t.Error("upstream destination or credential changed")
		}
		data, _ := io.ReadAll(r.Body)
		var request map[string]json.RawMessage
		json.Unmarshal(data, &request)
		var servers []map[string][]json.RawMessage
		if err := json.Unmarshal(request["tool_servers"], &servers); err != nil || len(servers) != 1 || len(servers[0]) != 1 || len(servers[0]["specs"]) != 1 {
			t.Errorf("missing URL-free caller schema registration: %s", data)
			w.WriteHeader(400)
			return
		}
		var tools []map[string]json.RawMessage
		json.Unmarshal(request["tools"], &tools)
		if string(servers[0]["specs"][0]) != string(tools[0]["function"]) {
			t.Error("client registry changed the function schema")
		}
		var original map[string]json.RawMessage
		json.Unmarshal([]byte(input), &original)
		for key, value := range original {
			if string(request[key]) != string(value) {
				t.Errorf("caller field %s changed: %s", key, request[key])
			}
		}
		for _, key := range []string{"chat_id", "session_id", "tool_ids", "id"} {
			if _, exists := request[key]; exists {
				t.Errorf("unexpected execution context %s", key)
			}
		}
		fmt.Fprint(w, `{}`)
	}))
	defer up.Close()
	resp, err := NewClient(up.URL, "synthetic").ProxyChat(context.Background(), strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("native pipeline lost caller registry: %d", resp.StatusCode)
	}
}

type failedChatReader struct{ err error }

func (r failedChatReader) Read([]byte) (int, error) { return 0, r.err }

func TestProxyChatPreparationFailuresDoNotDispatch(t *testing.T) {
	var calls atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer up.Close()
	client := NewClient(up.URL, "synthetic")
	readFailure := errors.New("synthetic read failure")
	if _, err := client.ProxyChat(context.Background(), failedChatReader{readFailure}); !errors.Is(err, readFailure) {
		t.Fatalf("reader failure lost: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.ProxyChat(ctx, failedChatReader{readFailure}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled request was read or dispatched: %v", err)
	}
	// Input fits, but serializing the same large schema in both registries
	// exceeds the output limit. It must not become a partial upstream request.
	input := `{"tools":[{"type":"function","function":{"name":"echo","description":"` + strings.Repeat("x", 16<<20) + `"}}]}`
	if _, err := client.ProxyChat(context.Background(), strings.NewReader(input)); !errors.Is(err, ErrChatRequestTooLarge) {
		t.Fatalf("adapted request exceeded limit: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("preparation failure sent %d upstream requests", calls.Load())
	}
}

func TestProxyChatKeepsResponseIncrementalAndCancelable(t *testing.T) {
	first := "data:{\"choices\":[]}\n\n"
	release := make(chan struct{})
	canceled := make(chan struct{})
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, first)
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
		}
		close(canceled)
	}))
	defer up.Close()
	defer close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := NewClient(up.URL, "synthetic").ProxyChat(ctx, strings.NewReader(`{"stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data := make([]byte, len(first))
	if _, err := io.ReadFull(resp.Body, data); err != nil || string(data) != first {
		t.Fatalf("incremental response unavailable: %q %v", data, err)
	}
	cancel()
	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation did not reach upstream")
	}
}

func TestProxyChatRejectsOversizeBeforeUpstream(t *testing.T) {
	var calls atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		io.Copy(io.Discard, r.Body)
		fmt.Fprint(w, `{}`)
	}))
	defer up.Close()
	resp, err := NewClient(up.URL, "synthetic").ProxyChat(context.Background(), strings.NewReader(strings.Repeat(" ", (32<<20)+1)))
	if resp != nil {
		resp.Body.Close()
	}
	if err == nil || calls.Load() != 0 {
		t.Fatalf("oversize request reached upstream: error=%v calls=%d", err, calls.Load())
	}
}

func TestProxyChatDoesNotReplayBodyOnRedirect(t *testing.T) {
	for _, status := range []int{307, 308} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var redirected atomic.Int32
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				redirected.Add(1)
				fmt.Fprint(w, `{}`)
			}))
			defer target.Close()
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, target.URL, status)
			}))
			defer up.Close()
			resp, err := NewClient(up.URL, "synthetic").ProxyChat(context.Background(), strings.NewReader(`{"messages":[{"role":"user","content":"private synthetic prompt"}]}`))
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != status || redirected.Load() != 0 {
				t.Fatalf("private POST was replayed: status=%d target_calls=%d", resp.StatusCode, redirected.Load())
			}
		})
	}
}

func TestProxyChatPreservesUnadaptedBodies(t *testing.T) {
	for _, input := range []string{
		`{ "model": "synthetic", "messages": [], "precision": 9007199254740993 }`,
		`{"tools":[]}`, `{"tools":null}`, `{"tools":"invalid"}`, `not-json`, `{} {}`,
		`{"tools":[{"type":"custom","custom":{"name":"custom_tool"}}]}`,
		`{"tools":[{"type":"function","function":null}]}`,
		`{"tools":[{"type":"function","function":{"name":"echo"}}],"tool_servers":[]}`,
		`{"tools":[{"type":"function","function":{"name":"echo"}}],"tool_servers":null}`,
	} {
		t.Run(input, func(t *testing.T) {
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				data, _ := io.ReadAll(r.Body)
				if string(data) != input {
					t.Errorf("unadapted body changed: %s", data)
				}
				fmt.Fprint(w, `{}`)
			}))
			defer up.Close()
			resp, err := NewClient(up.URL, "synthetic").ProxyChat(context.Background(), strings.NewReader(input))
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
		})
	}
}

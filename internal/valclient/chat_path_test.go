package valclient

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetChatIDIsOneOpaquePathSegment(t *testing.T) {
	for _, id := range []string{"chat?admin=true", "chat#fragment", "chat%2Fother"} {
		t.Run(id, func(t *testing.T) {
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.RawQuery != "" || r.URL.Path != "/api/v1/chats/"+id {
					t.Errorf("ID changed request path/query: %s", r.URL.String())
				}
				fmt.Fprint(w, `{"metadata":{"precise":9007199254740993}}`)
			}))
			defer up.Close()
			data, err := NewClient(up.URL, "synthetic").GetChat(context.Background(), id)
			if err != nil {
				t.Fatal(err)
			}
			if fmt.Sprint(data["metadata"].(map[string]any)["precise"]) != "9007199254740993" {
				t.Fatalf("source number changed: %v", data)
			}
		})
	}
}

func TestGetChatRejectsPathChangingIDs(t *testing.T) {
	calls := 0
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, `{}`) }))
	defer up.Close()
	for _, id := range []string{"", ".", "..", "../other", `..\other`, "chat\nother", "chat\x00other"} {
		if _, err := NewClient(up.URL, "synthetic").GetChat(context.Background(), id); err == nil || !strings.Contains(err.Error(), "chat ID") {
			t.Errorf("id=%q err=%v", id, err)
		}
	}
	if calls != 0 {
		t.Fatalf("invalid IDs made %d upstream requests", calls)
	}
}

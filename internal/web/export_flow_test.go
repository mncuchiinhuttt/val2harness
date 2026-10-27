package web_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"val2harness/internal/config"
	"val2harness/internal/proxy"
	"val2harness/internal/valclient"
	"val2harness/internal/web"
)

func TestExportFailureReportsCompletedChatsAndLocation(t *testing.T) {
	t.Chdir(t.TempDir())
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/chats/":
			fmt.Fprint(w, `[{"id":"good","title":"Good"},{"id":"bad","title":"Bad"}]`)
		case "/api/v1/chats/good":
			fmt.Fprint(w, `{"messages":[{"role":"user","content":"Synthetic conversation"}]}`)
		default:
			http.Error(w, "synthetic failure", 500)
		}
	}))
	defer up.Close()
	cfg := config.DefaultConfig()
	cfg.APIKey = "local-owner"
	cfg.Token = "synthetic"
	cfg.UpstreamURL = up.URL
	server := httptest.NewUnstartedServer(nil)
	cfg.ListenAddr = server.Listener.Addr().String()
	client := valclient.NewClient(up.URL, cfg.Token)
	gateway := proxy.NewServer(cfg, client)
	web.RegisterWebRoutes(gateway.GetMux(), cfg, client, gateway.UpdateToken)
	server.Config.Handler = gateway
	server.Start()
	defer server.Close()
	r := call(t, "POST", server.URL+"/api/local/export", "", "local-owner", server.URL)
	var result struct {
		Status string `json:"status"`
		Count  int    `json:"count"`
		Dir    string `json:"dir"`
		Error  string `json:"error"`
	}
	if err := json.NewDecoder(r.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if r.StatusCode != 500 || result.Count != 1 || !strings.HasPrefix(result.Dir, filepath.Join("exports", "run-")) || result.Status != "partial" || result.Error == "" {
		t.Fatalf("partial result discarded: %+v HTTP %d", result, r.StatusCode)
	}
	second := call(t, "POST", server.URL+"/api/local/export", "", "local-owner", server.URL)
	var retry struct {
		Count int    `json:"count"`
		Dir   string `json:"dir"`
	}
	if err := json.NewDecoder(second.Body).Decode(&retry); err != nil {
		t.Fatal(err)
	}
	if retry.Count != 1 || retry.Dir == result.Dir {
		t.Fatalf("retry failed or reused destination: %+v", retry)
	}
}

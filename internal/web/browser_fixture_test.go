package web_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"val2harness/internal/config"
	"val2harness/internal/daemon"
	"val2harness/internal/proxy"
	"val2harness/internal/valclient"
	"val2harness/internal/web"
)

// Opt-in fixture for browser acceptance checks. It never contacts Val and exits
// after its stop file appears or a bounded timeout. All credentials are synthetic.
func TestBrowserFixture(t *testing.T) {
	output := os.Getenv("VAL2HARNESS_BROWSER_FIXTURE")
	if output == "" {
		t.Skip("opt-in browser fixture")
	}
	originalTransport := http.DefaultTransport
	external := new(atomic.Int32)
	http.DefaultTransport = localOnlyTransport{base: originalTransport, external: external}
	t.Cleanup(func() {
		http.DefaultTransport = originalTransport
		if n := external.Load(); n != 0 { t.Errorf("browser fixture attempted %d external HTTP requests", n) }
	})
	var modelRequests, chatRequests atomic.Int32
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".pi"), 0700)
	os.WriteFile(filepath.Join(home, ".pi", "models.json"), []byte(`{"providers":{"personal":{"apiKey":"synthetic-personal"}}}`), 0600)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/models":
			modelRequests.Add(1)
			fmt.Fprint(w, `{"data":[{"id":"anthropic-claude-opus-4-7","name":"Synthetic model"}]}`)
		case "/api/chat/completions":
			count := chatRequests.Add(1)
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Synthetic browser reply\"}}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
			if count%2 == 1 { fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":0,\"completion_tokens\":0,\"total_tokens\":0}}\n\n") }
			fmt.Fprint(w, "data: [DONE]\n\n")
		default:
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()
	cfg := config.DefaultConfig()
	cfg.APIKey = "browser-fixture-local-key"
	cfg.Token = "synthetic-val"
	cfg.UpstreamURL = upstream.URL
	server := httptest.NewUnstartedServer(nil)
	cfg.ListenAddr = server.Listener.Addr().String()
	client := valclient.NewClient(upstream.URL, cfg.Token)
	gateway := proxy.NewServer(cfg, client)
	web.RegisterWebRoutes(gateway.GetMux(), cfg, client, gateway.UpdateToken)
	gateway.GetMux().HandleFunc("/api/local/fixture-stats", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]int32{"model_requests": modelRequests.Load(), "chat_requests": chatRequests.Load()})
	})
	server.Config.Handler = gateway
	server.Start()
	defer server.Close()
	ticketURL, err := daemon.DashboardURL(cfg)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(map[string]string{"url": server.URL, "ticketURL": ticketURL, "key": cfg.APIKey})
	if err := os.WriteFile(output, data, 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 1800; i++ {
		if _, err := os.Stat(output + ".stop"); err == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("browser fixture timed out")
}

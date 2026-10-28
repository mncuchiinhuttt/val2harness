package web_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"val2harness/internal/config"
	"val2harness/internal/proxy"
	"val2harness/internal/valclient"
	"val2harness/internal/web"
)

type localOnlyTransport struct {
	base     http.RoundTripper
	external *atomic.Int32
}

func (tr localOnlyTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	host := r.URL.Hostname()
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		tr.external.Add(1)
		return nil, fmt.Errorf("external HTTP is disabled in passive quota tests")
	}
	return tr.base.RoundTrip(r)
}

func quotaFixture(t *testing.T) (string, *atomic.Int32, string) {
	t.Helper()
	original := http.DefaultTransport
	external := new(atomic.Int32)
	http.DefaultTransport = localOnlyTransport{base: original, external: external}
	t.Cleanup(func() {
		http.DefaultTransport = original
		if n := external.Load(); n != 0 {
			t.Errorf("management reads attempted %d external HTTP requests", n)
		}
	})
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	calls := new(atomic.Int32)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"synthetic"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`)
	}))
	t.Cleanup(up.Close)
	cfg := config.DefaultConfig()
	cfg.APIKey, cfg.Token, cfg.UpstreamURL = "local-owner", "synthetic-val", up.URL
	server := httptest.NewUnstartedServer(nil)
	cfg.ListenAddr = server.Listener.Addr().String()
	client := valclient.NewClient(up.URL, cfg.Token)
	gateway := proxy.NewServer(cfg, client)
	web.RegisterWebRoutes(gateway.GetMux(), cfg, client, gateway.UpdateToken)
	server.Config.Handler = gateway
	server.Start()
	t.Cleanup(server.Close)
	return server.URL, calls, home
}

func readQuota(t *testing.T, base string) map[string]any {
	t.Helper()
	response := call(t, "GET", base+"/api/local/quota", "", "local-owner", base)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("quota status %d", response.StatusCode)
	}
	var result map[string]any
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	return result
}

func TestPassiveQuotaReadsUnknownWithoutUpstreamOrHistory(t *testing.T) {
	base, calls, home := quotaFixture(t)
	history := filepath.Join(home, ".config", "val2harness", "usage.jsonl")
	if err := os.MkdirAll(filepath.Dir(history), 0700); err != nil {
		t.Fatal(err)
	}
	legacy := []byte("legacy user data must stay untouched\n")
	if err := os.WriteFile(history, legacy, 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		q := readQuota(t, base)
		if q["status"] != "unknown" || q["source"] != "gateway_session" || q["observed_requests"] != float64(0) {
			t.Fatalf("initial quota invented an observation: %#v", q)
		}
		for _, field := range []string{"remaining", "limit", "reset_at", "observed_at"} {
			value, exists := q[field]
			if !exists || value != nil {
				t.Fatalf("%s must be explicit null, got %#v", field, value)
			}
		}
	}
	for _, route := range []string{"status", "analytics"} {
		response := call(t, "GET", base+"/api/local/"+route, "", "local-owner", base)
		if response.StatusCode != 200 {
			t.Fatalf("%s: %d", route, response.StatusCode)
		}
		response.Body.Close()
	}
	if calls.Load() != 0 {
		t.Fatalf("status/refresh made %d upstream requests", calls.Load())
	}
	data, err := os.ReadFile(history)
	if err != nil || string(data) != string(legacy) {
		t.Fatal("reading status changed legacy history")
	}
}

func TestPassiveManagementRoutesEnforceOwnerOriginHostAndMethods(t *testing.T) {
	base, calls, _ := quotaFixture(t)
	for _, route := range []string{"quota", "analytics", "analytics/live", "version", "update", "harnesses", "env", "status"} {
		for _, key := range []string{"", "incorrect"} {
			response := call(t, "GET", base+"/api/local/"+route, "", key, base)
			if response.StatusCode != 401 {
				t.Errorf("%s with %q: %d", route, key, response.StatusCode)
			}
			response.Body.Close()
		}
		response := call(t, "GET", base+"/api/local/"+route, "", "local-owner", "https://untrusted.invalid")
		if response.StatusCode != 403 {
			t.Errorf("cross origin %s: %d", route, response.StatusCode)
		}
		response.Body.Close()
		method := "POST"
		if route == "update" {
			method = "GET"
		}
		response = call(t, method, base+"/api/local/"+route, "", "local-owner", base)
		if response.StatusCode != 405 {
			t.Errorf("%s %s: %d", method, route, response.StatusCode)
		}
		response.Body.Close()
		req, _ := http.NewRequest("GET", base+"/api/local/"+route, nil)
		req.Host = "untrusted.invalid"
		req.Header.Set("Authorization", "Bearer local-owner")
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != 403 {
			t.Errorf("untrusted host %s: %d", route, response.StatusCode)
		}
		response.Body.Close()
	}
	if calls.Load() != 0 {
		t.Fatalf("denied requests contacted upstream: %d", calls.Load())
	}
}

func TestPassiveAnalyticsClearResetsSameObservedSession(t *testing.T) {
	base, calls, home := quotaFixture(t)
	response := call(t, "POST", base+"/v1/chat/completions", `{"model":"synthetic","messages":[{"role":"user","content":"hello"}],"stream":false}`, "local-owner", base)
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("completion: %d", response.StatusCode)
	}
	q := readQuota(t, base)
	if q["observed_requests"] != float64(1) || q["total_tokens"] != float64(5) || q["status"] != "available" {
		t.Fatalf("quota does not share request observation: %#v", q)
	}
	response = call(t, "GET", base+"/api/local/analytics", "", "local-owner", base)
	var summary map[string]any
	if err := json.NewDecoder(response.Body).Decode(&summary); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if summary["total_requests"] != float64(1) || summary["total_tokens"] != float64(5) {
		t.Fatalf("analytics diverged: %#v", summary)
	}
	for _, origin := range []string{"https://untrusted.invalid", base} {
		response = call(t, "DELETE", base+"/api/local/analytics", "", "local-owner", origin)
		want := 200
		if origin != base {
			want = 403
		}
		if response.StatusCode != want {
			t.Fatalf("clear: %d, want %d", response.StatusCode, want)
		}
		response.Body.Close()
		if origin != base && readQuota(t, base)["observed_requests"] != float64(1) {
			t.Fatal("denied clear changed observations")
		}
	}
	q = readQuota(t, base)
	if q["status"] != "unknown" || q["observed_requests"] != float64(0) {
		t.Fatalf("clear did not reset: %#v", q)
	}
	if calls.Load() != 1 {
		t.Fatalf("reads/clear issued extra inference: %d", calls.Load())
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "val2harness", "usage.jsonl")); !os.IsNotExist(err) {
		t.Fatal("observations persisted a history file")
	}
}

func TestPassiveLiveAnalyticsStartsWithAuthenticatedMemorySnapshot(t *testing.T) {
	base, calls, _ := quotaFixture(t)
	response := call(t, "GET", base+"/api/local/analytics/live", "", "local-owner", base)
	if response.StatusCode != 200 || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("live status: %d", response.StatusCode)
	}
	line, err := bufio.NewReader(response.Body).ReadString('\n')
	response.Body.Close()
	if err != nil || !strings.Contains(line, `"total_requests":0`) {
		t.Fatalf("initial live snapshot: %q, %v", line, err)
	}
	if calls.Load() != 0 {
		t.Fatal("live snapshot contacted upstream")
	}
}

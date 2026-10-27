package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"val2harness/internal/config"
	"val2harness/internal/valclient"
)

func ticketFor(t *testing.T, s *Server, path string) string {
	t.Helper()
	w := authCall(s, "POST", path, "", map[string]string{"Authorization": "Bearer local-owner"})
	if w.Code != 200 {
		t.Fatalf("ticket request failed: %d", w.Code)
	}
	var v map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	return v["ticket"]
}
func TestDashboardTicketOneUseAndExpiringSession(t *testing.T) {
	s := authFixture(t, "local-owner")
	now := time.Now()
	s.auth.clock = func() time.Time { return now }
	ticket := ticketFor(t, s, "/api/local/session-ticket")
	body := fmt.Sprintf(`{"ticket":%q}`, ticket)
	w := authCall(s, "POST", "/api/local/session", body, nil)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var session map[string]string
	json.Unmarshal(w.Body.Bytes(), &session)
	if session["token"] == "" || session["token"] == "local-owner" {
		t.Fatal("browser did not receive a distinct session")
	}
	if authCall(s, "GET", "/v1/models", "", map[string]string{"Authorization": "Bearer " + session["token"]}).Code != 200 {
		t.Fatal("dashboard session unusable")
	}
	if authCall(s, "POST", "/api/local/session", body, nil).Code != 401 {
		t.Fatal("ticket replay accepted")
	}
	now = now.Add(9 * time.Hour)
	if authCall(s, "GET", "/v1/models", "", map[string]string{"Authorization": "Bearer " + session["token"]}).Code != 401 {
		t.Fatal("expired dashboard session accepted")
	}
	ticket = ticketFor(t, s, "/api/local/session-ticket")
	now = now.Add(6 * time.Minute)
	if authCall(s, "POST", "/api/local/session", fmt.Sprintf(`{"ticket":%q}`, ticket), nil).Code != 401 {
		t.Fatal("expired sign-in ticket accepted")
	}
}
func TestTicketConcurrentRedemptionHasOneWinner(t *testing.T) {
	s := authFixture(t, "local-owner")
	ticket := ticketFor(t, s, "/api/local/session-ticket")
	results := make(chan int, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- authCall(s, "POST", "/api/local/session", fmt.Sprintf(`{"ticket":%q}`, ticket), nil).Code
		}()
	}
	wg.Wait()
	close(results)
	winners := 0
	for status := range results {
		if status == 200 {
			winners++
		} else if status != 401 {
			t.Errorf("unexpected code %d", status)
		}
	}
	if winners != 1 {
		t.Fatalf("ticket used %d times", winners)
	}
}
func TestValImportTicketCannotAuthorizeSiblingRoutes(t *testing.T) {
	s := authFixture(t, "local-owner")
	s.GetMux().HandleFunc("/api/local/import-token", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	ticket := ticketFor(t, s, "/api/local/import-ticket")
	origin := s.upstreamOrigin()
	if authCall(s, "POST", "/api/local/session", fmt.Sprintf(`{"ticket":%q}`, ticket), nil).Code != 401 {
		t.Fatal("import ticket became dashboard session")
	}
	if authCall(s, "GET", "/v1/models", "", map[string]string{"Authorization": "Bearer " + ticket}).Code != 401 {
		t.Fatal("import ticket authorized inference")
	}
	for _, headers := range []map[string]string{{"Origin": "http://untrusted.example", "X-Val-Import": ticket}, {"X-Val-Import": ticket}} {
		if authCall(s, "POST", "/api/local/import-token", `{}`, headers).Code != 403 {
			t.Fatal("wrong/missing Origin permitted import")
		}
	}
	headers := map[string]string{"Origin": origin, "X-Val-Import": ticket}
	if authCall(s, "GET", "/api/local/import-token", "", headers).Code != 405 {
		t.Fatal("import accepted GET")
	}
	if authCall(s, "POST", "/api/local/import-token", `{}`, headers).Code != 204 {
		t.Fatal("intended Val import rejected")
	}
	if authCall(s, "POST", "/api/local/import-token", `{}`, headers).Code != 401 {
		t.Fatal("import ticket replay accepted")
	}
}

func authFixture(t *testing.T, key string) *Server {
	t.Helper()
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[{"id":"synthetic"}],"choices":[{"message":{"content":"ok"}}]}`)
	}))
	t.Cleanup(up.Close)
	cfg := config.DefaultConfig()
	cfg.APIKey = key
	cfg.Token = "synthetic-val"
	cfg.UpstreamURL = up.URL
	s := NewServer(cfg, valclient.NewClient(up.URL, cfg.Token))
	s.GetMux().HandleFunc("/api/local/protected", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	s.GetMux().HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, "public dashboard shell")
	})
	return s
}
func authCall(s *Server, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://127.0.0.1:8765"+path, strings.NewReader(body))
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}
func TestOwnerRequiredEvenWhenKeyMissing(t *testing.T) {
	for _, key := range []string{"", "local-owner"} {
		s := authFixture(t, key)
		for _, path := range []string{"/v1/models", "/models", "/v1/chat/completions", "/chat/completions", "/v1/messages", "/messages", "/api/local/protected"} {
			t.Run(key+path, func(t *testing.T) {
				if w := authCall(s, "POST", path, `{}`, nil); w.Code != 401 {
					t.Fatalf("unauthorized request got %d", w.Code)
				}
			})
		}
	}
}
func TestForeignOriginDeniedEvenWithOwnerKey(t *testing.T) {
	s := authFixture(t, "local-owner")
	for _, origin := range []string{"http://untrusted.example", "null", "http://127.0.0.1:9999"} {
		w := authCall(s, "POST", "/api/local/protected", `{}`, map[string]string{"Origin": origin, "Authorization": "Bearer local-owner", "Content-Type": "text/plain"})
		if w.Code != 403 || w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("foreign Origin %s accepted: %d", origin, w.Code)
		}
	}
}
func TestUnexpectedHostDenied(t *testing.T) {
	s := authFixture(t, "local-owner")
	r := httptest.NewRequest("POST", "http://rebound.example:8765/api/local/protected", nil)
	r.Header.Set("Authorization", "Bearer local-owner")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("unexpected Host accepted: %d", w.Code)
	}
}
func TestPublicShellAndMinimalHealthWithAuthenticationEnabled(t *testing.T) {
	s := authFixture(t, "local-owner")
	for _, path := range []string{"/", "/healthz"} {
		w := authCall(s, "GET", path, "", nil)
		if w.Code != 200 {
			t.Errorf("public %s blocked: %d", path, w.Code)
		}
		if strings.Contains(w.Body.String(), "local-owner") || strings.Contains(w.Body.String(), "synthetic-val") || strings.Contains(w.Body.String(), "upstream") {
			t.Error("public response exposes private state")
		}
	}
}
func TestNativeOwnerAndAnthropicKeyRemainUsable(t *testing.T) {
	s := authFixture(t, "local-owner")
	for _, headers := range []map[string]string{{"Authorization": "Bearer local-owner"}, {"x-api-key": "local-owner"}, {"Authorization": "Bearer local-owner", "Origin": "http://127.0.0.1:8765"}} {
		w := authCall(s, "GET", "/v1/models", "", headers)
		if w.Code != 200 {
			t.Errorf("authorized client rejected: %d", w.Code)
		}
	}
}

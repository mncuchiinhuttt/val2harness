package web_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"val2harness/internal/config"
	"val2harness/internal/proxy"
	"val2harness/internal/valclient"
	"val2harness/internal/web"
)

func webFixture(t *testing.T) (string, *valclient.Client, *config.Config, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{"data":[{"id":"synthetic"}]}`) }))
	t.Cleanup(up.Close)
	cfg := config.DefaultConfig()
	cfg.APIKey = "local-owner"
	cfg.Token = "synthetic-original"
	cfg.UpstreamURL = up.URL
	server := httptest.NewUnstartedServer(nil)
	cfg.ListenAddr = server.Listener.Addr().String()
	client := valclient.NewClient(up.URL, cfg.Token)
	gateway := proxy.NewServer(cfg, client)
	web.RegisterWebRoutes(gateway.GetMux(), cfg, client, gateway.UpdateToken)
	server.Config.Handler = gateway
	server.Start()
	t.Cleanup(server.Close)
	return server.URL, client, cfg, home
}
func call(t *testing.T, method, url, body, key, origin string) *http.Response {
	t.Helper()
	r, err := http.NewRequest(method, url, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	if key != "" {
		r.Header.Set("Authorization", "Bearer "+key)
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	r.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { response.Body.Close() })
	return response
}
func TestAuthenticatedLogoutRequiresPost(t *testing.T) {
	base, c, _, _ := webFixture(t)
	response := call(t, "GET", base+"/api/local/logout", "", "local-owner", base)
	if response.StatusCode != 405 || c.GetToken() != "synthetic-original" {
		t.Fatal("GET changed account state")
	}
}
func TestTokenPersistenceFailureLeavesMemoryUntouched(t *testing.T) {
	base, c, _, home := webFixture(t)
	p := filepath.Join(home, ".config", "val2harness", "config.json")
	os.MkdirAll(p, 0700)
	response := call(t, "POST", base+"/api/local/token", `{"token":"synthetic-replacement"}`, "local-owner", base)
	if response.StatusCode < 400 || c.GetToken() != "synthetic-original" {
		t.Fatal("failed token save reported success or changed memory")
	}
}
func TestScopedImportStoresOnlyIntendedSyntheticToken(t *testing.T) {
	base, c, cfg, _ := webFixture(t)
	response := call(t, "POST", base+"/api/local/import-ticket", "", "local-owner", base)
	var ticket map[string]string
	if err := json.NewDecoder(response.Body).Decode(&ticket); err != nil {
		t.Fatal(err)
	}
	r, _ := http.NewRequest("POST", base+"/api/local/import-token", bytes.NewBufferString(`{"token":"synthetic-imported"}`))
	r.Header.Set("Origin", cfg.UpstreamURL)
	r.Header.Set("X-Val-Import", ticket["ticket"])
	r.Header.Set("Content-Type", "application/json")
	imported, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer imported.Body.Close()
	if imported.StatusCode != 200 || c.GetToken() != "synthetic-imported" {
		t.Fatalf("intended import failed: %d", imported.StatusCode)
	}
}

func TestConcurrentTokenSavesAndStatusKeepDiskAndClientConsistent(t *testing.T) {
	base, client, _, home := webFixture(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			resp := call(t, "POST", base+"/api/local/token", fmt.Sprintf(`{"token":"synthetic-%d"}`, i), "local-owner", base)
			if resp.StatusCode != 200 {
				t.Errorf("save: %d", resp.StatusCode)
			}
			resp.Body.Close()
			resp = call(t, "GET", base+"/api/local/status", "", "local-owner", base)
			if resp.StatusCode != 200 {
				t.Errorf("status: %d", resp.StatusCode)
			}
			resp.Body.Close()
		}(i)
	}
	wg.Wait()
	data, err := os.ReadFile(filepath.Join(home, ".config", "val2harness", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var disk config.Config
	if err := json.Unmarshal(data, &disk); err != nil {
		t.Fatal(err)
	}
	if disk.Token != client.GetToken() {
		t.Fatal("successful concurrent saves left disk and request credentials different")
	}
}

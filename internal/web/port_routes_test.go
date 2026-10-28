package web_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPortSaveStagesOnlyAddressAndPreservesRunningHostAndLatestCredentials(t *testing.T) {
	base, client, cfg, home := webFixture(t)
	path := filepath.Join(home, ".config", "val2harness", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"token":"newer-disk-token","api_key":"newer-disk-key","upstream_url":"https://synthetic.invalid","listen_addr":"127.0.0.1:8765","custom":{"precise":9007199254740993}}`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	running := cfg.ListenAddr
	response := call(t, "POST", base+"/api/local/port", `{"addr":" 127.0.0.1:19998 "}`, "local-owner", base)
	var result map[string]any
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 || result["requires_restart"] != true || result["addr"] != "127.0.0.1:19998" {
		t.Fatalf("staged result: %d %#v", response.StatusCode, result)
	}
	if cfg.ListenAddr != running || client.GetToken() != "synthetic-original" {
		t.Fatal("port save changed the live listener/account")
	}
	response = call(t, "GET", base+"/api/local/status", "", "local-owner", base)
	if response.StatusCode != 200 {
		t.Fatalf("current Host guard changed before restart: %d", response.StatusCode)
	}
	response.Body.Close()
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal(updated, &values); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"token": `"newer-disk-token"`, "api_key": `"newer-disk-key"`, "upstream_url": `"https://synthetic.invalid"`, "listen_addr": `"127.0.0.1:19998"`} {
		if string(values[key]) != want {
			t.Errorf("port save overwrote %s: %s", key, values[key])
		}
	}
	var custom map[string]json.RawMessage
	if err := json.Unmarshal(values["custom"], &custom); err != nil || string(custom["precise"]) != "9007199254740993" {
		t.Fatal("port save changed unknown configuration")
	}
}

func TestPortSaveRejectsMalformedAddressWithoutMutation(t *testing.T) {
	base, _, cfg, home := webFixture(t)
	running := cfg.ListenAddr
	for _, addr := range []string{"", "8765", "http://127.0.0.1:8765", "127.0.0.1:0", "127.0.0.1:65536", "127.0.0.1:abc", "untrusted.invalid:8765", "127.0.0.1:8765/path"} {
		body, _ := json.Marshal(map[string]string{"addr": addr})
		response := call(t, "POST", base+"/api/local/port", string(body), "local-owner", base)
		if response.StatusCode != 400 {
			t.Errorf("invalid %q: %d", addr, response.StatusCode)
		}
		response.Body.Close()
		if cfg.ListenAddr != running {
			t.Fatal("invalid port changed live config")
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "val2harness", "config.json")); !os.IsNotExist(err) {
		t.Fatal("invalid port save wrote configuration")
	}
}

func TestPortSaveReportsPersistenceFailureWithoutChangingListener(t *testing.T) {
	base, _, cfg, home := webFixture(t)
	running := cfg.ListenAddr
	if err := os.MkdirAll(filepath.Join(home, ".config", "val2harness", "config.json"), 0700); err != nil {
		t.Fatal(err)
	}
	response := call(t, "POST", base+"/api/local/port", `{"addr":"127.0.0.1:19998"}`, "local-owner", base)
	if response.StatusCode != 500 {
		t.Fatalf("failed persistence reported %d", response.StatusCode)
	}
	response.Body.Close()
	if cfg.ListenAddr != running {
		t.Fatal("failed save changed listener/Host guard")
	}
}

func TestTokenSavePreservesStagedListenerAndOtherDiskSettings(t *testing.T) {
	base, client, cfg, home := webFixture(t)
	path := filepath.Join(home, ".config", "val2harness", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"token":"old","api_key":"disk-owner-key","upstream_url":"https://configured.invalid","listen_addr":"127.0.0.1:19998","custom":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	running := cfg.ListenAddr
	response := call(t, "POST", base+"/api/local/token", `{"token":"new-account-token"}`, "local-owner", base)
	if response.StatusCode != 200 {
		t.Fatalf("token save: %d", response.StatusCode)
	}
	response.Body.Close()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var values map[string]any
	if err := json.Unmarshal(data, &values); err != nil {
		t.Fatal(err)
	}
	if values["listen_addr"] != "127.0.0.1:19998" || values["api_key"] != "disk-owner-key" || values["upstream_url"] != "https://configured.invalid" || values["custom"] != true {
		t.Fatalf("token save overwrote unrelated settings: %#v", values)
	}
	if values["token"] != "new-account-token" || client.GetToken() != "new-account-token" || cfg.ListenAddr != running {
		t.Fatal("token update did not preserve live listener consistency")
	}
}

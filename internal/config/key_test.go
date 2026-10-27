package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func privateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	for _, k := range []string{"VAL_RMIT_TOKEN", "VAL_RMIT_UPSTREAM", "VAL_RMIT_ADDR"} {
		t.Setenv(k, "")
	}
	return home
}
func TestLoadCreatesStableOwnerKeyWithoutPersistingEnvironment(t *testing.T) {
	home := privateHome(t)
	t.Setenv("VAL_RMIT_TOKEN", "environment-only-synthetic")
	t.Setenv("VAL_RMIT_ADDR", "127.0.0.1:9292")
	first, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	second, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(first.APIKey) < 32 || first.APIKey != second.APIKey {
		t.Errorf("missing or unstable local key")
	}
	data, err := os.ReadFile(filepath.Join(home, ".config", "val2harness", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var disk map[string]any
	if err := json.Unmarshal(data, &disk); err != nil {
		t.Fatal(err)
	}
	if disk["token"] == "environment-only-synthetic" || disk["listen_addr"] == "127.0.0.1:9292" {
		t.Fatal("transient environment persisted")
	}
	if first.Token != "environment-only-synthetic" || first.ListenAddr != "127.0.0.1:9292" {
		t.Fatal("environment overrides lost")
	}
}
func TestConcurrentLoadsConvergeOnOwnerKey(t *testing.T) {
	privateHome(t)
	keys := make(chan string, 8)
	errs := make(chan error, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cfg, err := Load()
			if err != nil {
				errs <- err
				return
			}
			keys <- cfg.APIKey
		}()
	}
	wg.Wait()
	close(keys)
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	first := ""
	for key := range keys {
		if key == "" {
			t.Error("empty owner key")
		}
		if first != "" && key != first {
			t.Error("processes disagree on owner key")
		}
		first = key
	}
}
func TestLoadPreservesCustomKeyAndUnknownSettings(t *testing.T) {
	home := privateHome(t)
	path := filepath.Join(home, ".config", "val2harness", "config.json")
	os.MkdirAll(filepath.Dir(path), 0700)
	original := []byte(`{"api_key":"user-selected-local-key","token":"synthetic","future_setting":{"keep":true}}`)
	os.WriteFile(path, original, 0600)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey != "user-selected-local-key" {
		t.Fatal("custom key replaced")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(original) {
		t.Fatal("unchanged user settings rewritten")
	}
}

func TestSavePreservesUnknownSettingsAndFirstOriginal(t *testing.T) {
	home := privateHome(t)
	path := filepath.Join(home, ".config", "val2harness", "config.json")
	os.MkdirAll(filepath.Dir(path), 0700)
	original := []byte(`{"api_key":"user-selected-local-key","token":"synthetic","future_setting":{"keep":true}}`)
	os.WriteFile(path, original, 0600)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Token = "replacement-synthetic"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	var saved map[string]any
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved["future_setting"] == nil || saved["token"] != cfg.Token {
		t.Fatal("save discarded unrelated settings or lost token update")
	}
	backup, _ := os.ReadFile(path + ".bak")
	if string(backup) != string(original) {
		t.Fatal("save did not retain the first original")
	}
}

func TestLocalBaseURL(t *testing.T) {
	for addr, expected := range map[string]string{
		":9292": "http://127.0.0.1:9292", "0.0.0.0:9292": "http://127.0.0.1:9292",
		"[::]:9292": "http://[::1]:9292", "[::1]:9292": "http://[::1]:9292", "localhost:9292": "http://localhost:9292",
	} {
		if got := LocalBaseURL(addr); got != expected {
			t.Errorf("%s: %s != %s", addr, got, expected)
		}
	}
}

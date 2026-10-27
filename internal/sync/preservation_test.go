package sync

import (
	"bytes"
	"context"
	"fmt"
	"gopkg.in/yaml.v3"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"val2harness/internal/valclient"
)

type harnessCase struct {
	name, path, original, invalid string
	run                           func(context.Context, *valclient.Client, string, string) (int, string, error)
}

var harnessCases = []harnessCase{
	{"pi", ".pi/models.json", `{"providers":{"personal":{"apiKey":"synthetic-private"}},"keep":"original"}`, `{"providers":`, SyncPi},
	{"opencode", ".config/opencode/opencode.json", `{"provider":{"personal":{"options":{"apiKey":"synthetic-private"}}},"keep":"original"}`, `{"provider":`, SyncOpenCode},
	{"continue", ".continue/config.yaml", "name: Personal\nmodels: []\nkeep: original\n", "models: [", SyncContinue},
	{"omp", ".omp/models.yml", "providers:\n  personal:\n    apiKey: synthetic-private\nkeep: original\n", "providers: [", SyncOMP},
}

func harnessFixture(t *testing.T, h harnessCase, content string) (string, *valclient.Client) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	p := filepath.Join(home, filepath.FromSlash(h.path))
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/models" {
			t.Errorf("unexpected upstream route %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"data":[{"id":"anthropic-claude-opus-4-7","name":"Synthetic model"}]}`)
	}))
	t.Cleanup(upstream.Close)
	return p, valclient.NewClient(upstream.URL, "synthetic-val")
}

func TestSyncRejectsMalformedWithoutChangingOriginalOrBackup(t *testing.T) {
	for _, h := range harnessCases {
		t.Run(h.name, func(t *testing.T) {
			p, c := harnessFixture(t, h, h.invalid)
			baseline := []byte("prior-baseline")
			os.WriteFile(p+".bak", baseline, 0600)
			if _, _, err := h.run(context.Background(), c, "http://127.0.0.1:9292", "local-secret"); err == nil {
				t.Error("malformed settings accepted")
			}
			got, _ := os.ReadFile(p)
			backup, _ := os.ReadFile(p + ".bak")
			if !bytes.Equal(got, []byte(h.invalid)) {
				t.Error("original settings changed")
			}
			if !bytes.Equal(backup, baseline) {
				t.Error("existing baseline overwritten")
			}
		})
	}
}
func TestRepeatedSyncAndRestoreKeepsFirstBaseline(t *testing.T) {
	for _, h := range harnessCases {
		t.Run(h.name, func(t *testing.T) {
			p, c := harnessFixture(t, h, h.original)
			for i := 0; i < 2; i++ {
				if _, _, err := h.run(context.Background(), c, "http://127.0.0.1:8765", "local-secret"); err != nil {
					t.Fatal(err)
				}
			}
			backup, _ := os.ReadFile(p + ".bak")
			if !bytes.Equal(backup, []byte(h.original)) {
				t.Error("first original backup replaced")
			}
			if _, err := RestoreHarness(h.name); err != nil {
				t.Fatal(err)
			}
			got, _ := os.ReadFile(p)
			if !bytes.Equal(got, []byte(h.original)) {
				t.Error("restore did not recover original bytes")
			}
		})
	}
}
func TestFailedBackupLeavesOriginalUntouched(t *testing.T) {
	for _, h := range harnessCases {
		t.Run(h.name, func(t *testing.T) {
			p, c := harnessFixture(t, h, h.original)
			os.Mkdir(p+".bak", 0700)
			if _, _, err := h.run(context.Background(), c, "http://127.0.0.1:8765", "local-secret"); err == nil {
				t.Error("failed backup not reported")
			}
			got, _ := os.ReadFile(p)
			if !bytes.Equal(got, []byte(h.original)) {
				t.Error("original modified despite failed backup")
			}
		})
	}
}
func TestSyncRejectsWrongContainerShapes(t *testing.T) {
	for _, h := range harnessCases {
		for _, content := range []string{"null", "[]", map[string]string{"pi": `{"providers":"personal"}`, "opencode": `{"provider":"personal"}`, "continue": "models: personal\n", "omp": "providers: personal\n"}[h.name]} {
			t.Run(h.name+"/"+content, func(t *testing.T) {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("invalid settings caused panic: %v", r)
					}
				}()
				p, c := harnessFixture(t, h, content)
				if _, _, err := h.run(context.Background(), c, "http://127.0.0.1:8765", "local-secret"); err == nil {
					t.Error("wrong shape accepted")
				}
				got, _ := os.ReadFile(p)
				if !bytes.Equal(got, []byte(content)) {
					t.Error("wrong-shaped original modified")
				}
			})
		}
	}
}
func TestPrivateBackupPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix mode contract; Windows uses ACLs")
	}
	for _, h := range harnessCases {
		t.Run(h.name, func(t *testing.T) {
			p, c := harnessFixture(t, h, h.original)
			os.WriteFile(p+".bak", []byte(h.original), 0644)
			os.Chmod(p+".bak", 0644)
			if _, _, err := h.run(context.Background(), c, "http://127.0.0.1:8765", "local-secret"); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{p, p + ".bak"} {
				st, err := os.Stat(name)
				if err != nil {
					t.Fatal(err)
				}
				if st.Mode().Perm()&0077 != 0 {
					t.Errorf("%s exposes owner settings: %o", name, st.Mode().Perm())
				}
			}
		})
	}
}

func TestRestoreCaseAliasesStillRequireJSON(t *testing.T) {
	for _, h := range harnessCases {
		if h.name != "pi" && h.name != "opencode" {
			continue
		}
		t.Run(h.name, func(t *testing.T) {
			path, _ := harnessFixture(t, h, h.original)
			os.WriteFile(path+".bak", []byte("providers: personal\n"), 0600)
			alias := map[string]string{"pi": "PI", "opencode": "OpenCode"}[h.name]
			if _, err := RestoreHarness(alias); err == nil {
				t.Error("case alias accepted YAML as JSON")
			}
			data, _ := os.ReadFile(path)
			if string(data) != h.original {
				t.Error("invalid backup replaced original")
			}
		})
	}
}

func TestYAMLSyncPreservesAliasValuesWhenReplacingAnchors(t *testing.T) {
	for _, h := range harnessCases {
		if h.name != "continue" && h.name != "omp" {
			continue
		}
		t.Run(h.name, func(t *testing.T) {
			original := "models:\n  - name: RMIT Old\n    apiBase: http://127.0.0.1:8765/v1\n    apiKey: &shared synthetic-personal\n  - name: Personal\n    apiKey: *shared\n"
			if h.name == "omp" {
				original = "providers:\n  rmit-val:\n    apiKey: &shared synthetic-personal\n  personal:\n    apiKey: *shared\n"
			}
			path, client := harnessFixture(t, h, original)
			if _, _, err := h.run(context.Background(), client, "http://127.0.0.1:8765", "local-secret"); err != nil {
				t.Fatal(err)
			}
			data, _ := os.ReadFile(path)
			var result map[string]any
			if err := yaml.Unmarshal(data, &result); err != nil {
				t.Fatalf("sync published invalid YAML: %v", err)
			}
			var personal map[string]any
			if h.name == "continue" {
				personal = result["models"].([]any)[0].(map[string]any)
			} else {
				personal = result["providers"].(map[string]any)["personal"].(map[string]any)
			}
			if personal["apiKey"] != "synthetic-personal" {
				t.Fatal("personal alias value changed")
			}
		})
	}
}

func TestContinueAcceptsModelAliasMapping(t *testing.T) {
	h := harnessCases[2]
	original := "models:\n  - &personal\n    name: Personal\n    apiKey: synthetic-personal\n  - *personal\n"
	path, client := harnessFixture(t, h, original)
	if _, _, err := h.run(context.Background(), client, "http://127.0.0.1:8765", "local-secret"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	var result struct {
		Models []map[string]any `yaml:"models"`
	}
	if err := yaml.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Models) < 3 || result.Models[1]["apiKey"] != "synthetic-personal" {
		t.Fatal("personal alias model was lost")
	}
}

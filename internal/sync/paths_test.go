package sync

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"val2harness/internal/omp"
)

func TestOMPSyncUsesAgentFileWithoutMirroringLegacy(t *testing.T) {
	h := harnessCases[3]
	h.path = ".omp/agent/models.yml"
	path, client := harnessFixture(t, h, h.original)
	_, synced, err := SyncOMP(context.Background(), client, "http://127.0.0.1:9292", "local-secret")
	if err != nil {
		t.Fatal(err)
	}
	if synced != path {
		t.Fatalf("synced %s instead of existing agent file %s", synced, path)
	}
	legacy := filepath.Join(filepath.Dir(filepath.Dir(path)), "models.yml")
	if _, err := os.Lstat(legacy); !os.IsNotExist(err) {
		t.Fatalf("sync unexpectedly created legacy settings: %v", err)
	}
	if !HasBackup("omp") {
		t.Fatal("agent backup not detected")
	}
	if restored, err := RestoreHarness("OMP"); err != nil || restored != path {
		t.Fatalf("restore selected another file: %s %v", restored, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != h.original {
		t.Fatalf("agent original was not restored: %q %v", data, err)
	}
}

func TestOMPRefusesAmbiguousSettingsAndBackups(t *testing.T) {
	for _, suffix := range []string{"", ".bak"} {
		t.Run(suffix, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			paths := []string{
				filepath.Join(home, ".omp", "agent", "models.yml"),
				filepath.Join(home, ".omp", "models.yml"),
			}
			for _, path := range paths {
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path+suffix, []byte("keep: original\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := omp.GetOMPModelsPath(); err == nil {
				t.Fatal("ambiguous OMP paths accepted")
			}
			if _, err := RestoreHarness("omp"); err == nil {
				t.Fatal("restore accepted ambiguous OMP paths")
			}
			for _, path := range paths {
				data, err := os.ReadFile(path + suffix)
				if err != nil || string(data) != "keep: original\n" {
					t.Fatalf("ambiguous file changed: %s %q %v", path+suffix, data, err)
				}
				if suffix == ".bak" {
					if _, err := os.Lstat(path); !os.IsNotExist(err) {
						t.Fatalf("ambiguous restore created %s: %v", path, err)
					}
				}
			}
		})
	}
}

func TestOMPFindsLegacyBackupBeforeEmptyAgentDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := os.MkdirAll(filepath.Join(home, ".omp", "agent"), 0700); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(home, ".omp", "models.yml")
	if err := os.WriteFile(legacy+".bak", []byte("keep: original\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if path, err := RestoreHarness("omp"); err != nil || path != legacy {
		t.Fatalf("legacy backup was not selected: %s %v", path, err)
	}
	data, err := os.ReadFile(legacy)
	if err != nil || string(data) != "keep: original\n" {
		t.Fatalf("legacy original was not restored: %q %v", data, err)
	}
}

func TestOMPChoosesExistingAgentDirectoryForNewSettings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	agent := filepath.Join(home, ".omp", "agent", "models.yml")
	if err := os.MkdirAll(filepath.Dir(agent), 0700); err != nil {
		t.Fatal(err)
	}
	if path, err := omp.GetOMPModelsPath(); err != nil || path != agent {
		t.Fatalf("agent directory was not selected: %s %v", path, err)
	}
}

func TestOpenCodeUsesExistingFileAndMatchingRestorePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	defaultPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	legacy := filepath.Join(home, ".opencode", "opencode.json")
	os.MkdirAll(filepath.Dir(defaultPath), 0700)
	os.MkdirAll(filepath.Dir(legacy), 0700)
	os.WriteFile(legacy, []byte(`{"personal":true}`), 0600)
	path, err := openCodePath(home)
	if err != nil || path != legacy {
		t.Fatalf("existing legacy file was not selected: %s %v", path, err)
	}
	os.WriteFile(legacy+".bak", []byte(`{"original":true}`), 0600)
	if path, err = RestoreHarness("OpenCode"); err != nil || path != legacy {
		t.Fatalf("restore selected another file: %s %v", path, err)
	}
	os.WriteFile(defaultPath, []byte(`{}`), 0600)
	if _, err := openCodePath(home); err == nil {
		t.Fatal("ambiguous configuration paths accepted")
	}
}

func TestEnvironmentSnippetUsesActualKeyAndShellQuoting(t *testing.T) {
	snippet := GenerateEnvSnippet("[::]:9292", "synthetic'key$literal")
	if !strings.Contains(snippet, "http://[::1]:9292/v1") || !strings.Contains(snippet, `'synthetic'\''key$literal'`) {
		t.Fatalf("private connection settings are not safely quoted: %q", snippet)
	}
	if strings.Contains(snippet, "rmit-val-key") {
		t.Fatal("placeholder credential generated")
	}
}

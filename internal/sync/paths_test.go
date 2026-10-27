package sync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

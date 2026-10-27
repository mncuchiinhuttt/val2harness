package configfile

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func increment(path string) error {
	return Update(path, func(data []byte) ([]byte, error) {
		n := 0
		if data != nil {
			var err error
			n, err = strconv.Atoi(string(data))
			if err != nil {
				return nil, err
			}
		}
		return []byte(strconv.Itoa(n + 1)), nil
	})
}
func TestConcurrentProcessHelper(t *testing.T) {
	p := os.Getenv("VAL2HARNESS_TEST_SETTINGS")
	if p == "" {
		t.Skip("subprocess fixture")
	}
	for i := 0; i < 5; i++ {
		if err := increment(p); err != nil {
			t.Fatal(err)
		}
	}
}
func TestUpdatesAreSerializedAcrossProcesses(t *testing.T) {
	p := filepath.Join(t.TempDir(), "counter")
	os.WriteFile(p, []byte("0"), 0600)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	failures := make(chan string, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestConcurrentProcessHelper$")
			cmd.Env = append(os.Environ(), "VAL2HARNESS_TEST_SETTINGS="+p)
			if out, err := cmd.CombinedOutput(); err != nil {
				failures <- fmt.Sprintf("%v: %s", err, out)
			}
		}()
	}
	wg.Wait()
	close(failures)
	for failure := range failures {
		t.Error(failure)
	}
	data, _ := os.ReadFile(p)
	if string(data) != "20" {
		t.Errorf("lost concurrent updates: %s", data)
	}
	backup, _ := os.ReadFile(p + ".bak")
	if string(backup) != "0" {
		t.Errorf("lost first baseline: %s", backup)
	}
}
func TestValidationFailureDoesNotChangeFiles(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings")
	os.WriteFile(p, []byte("user-owned"), 0600)
	os.WriteFile(p+".bak", []byte("original"), 0600)
	if err := Update(p, func([]byte) ([]byte, error) { return nil, fmt.Errorf("invalid document") }); err == nil {
		t.Fatal("validation failure hidden")
	}
	current, _ := os.ReadFile(p)
	backup, _ := os.ReadFile(p + ".bak")
	if string(current) != "user-owned" || string(backup) != "original" {
		t.Fatal("validation changed user files")
	}
}
func TestNoopUpdateRepairsReadableSecretFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix mode semantics")
	}
	p := filepath.Join(t.TempDir(), "settings")
	for _, name := range []string{p, p + ".bak"} {
		os.WriteFile(name, []byte("synthetic-secret"), 0600)
		os.Chmod(name, 0644)
	}
	if err := Update(p, func(data []byte) ([]byte, error) { return data, nil }); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{p, p + ".bak"} {
		st, _ := os.Stat(name)
		if st.Mode().Perm()&0077 != 0 {
			t.Errorf("unchanged secret remains readable: %s mode %o", name, st.Mode().Perm())
		}
	}
}
func TestRestoreAndFreshFilesArePrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix mode semantics")
	}
	p := filepath.Join(t.TempDir(), "settings")
	if err := Write(p, []byte("new")); err != nil {
		t.Fatal(err)
	}
	if err := Update(p, func([]byte) ([]byte, error) { return []byte("changed"), nil }); err != nil {
		t.Fatal(err)
	}
	os.Remove(p)
	if err := Restore(p, func(data []byte) error {
		if string(data) != "new" {
			return fmt.Errorf("bad baseline")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{p, p + ".bak"} {
		st, _ := os.Stat(name)
		if st.Mode().Perm()&0077 != 0 {
			t.Error("secret-bearing output is not private")
		}
	}
}
func TestInvalidYAMLAndSymlinksAreNotRewritten(t *testing.T) {
	for _, body := range []string{"keep: first\n---\nkeep: second\n", "keep: first\nkeep: second\n", "null", "[]"} {
		if _, err := YAMLDocument([]byte(body)); err == nil {
			t.Errorf("ambiguous/wrong-shaped YAML accepted: %s", body)
		}
	}
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs separate Windows privileges")
	}
	root := t.TempDir()
	target := filepath.Join(root, "target")
	link := filepath.Join(root, "linked")
	os.WriteFile(target, []byte("keep"), 0600)
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := Update(link, func([]byte) ([]byte, error) { return []byte("replace"), nil }); err == nil {
		t.Fatal("linked config silently replaced")
	}
	data, _ := os.ReadFile(target)
	if strings.TrimSpace(string(data)) != "keep" {
		t.Fatal("symlink target modified")
	}
}

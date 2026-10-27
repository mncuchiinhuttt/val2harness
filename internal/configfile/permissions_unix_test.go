//go:build !windows

package configfile

import (
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"testing"
)

func TestSecretWritesWithPermissiveUmask(t *testing.T) {
	previous := unix.Umask(0)
	defer unix.Umask(previous)
	p := filepath.Join(t.TempDir(), "settings")
	if err := Write(p, []byte("original")); err != nil {
		t.Fatal(err)
	}
	if err := Update(p, func([]byte) ([]byte, error) { return []byte("updated"), nil }); err != nil {
		t.Fatal(err)
	}
	os.Remove(p)
	if err := Restore(p, func([]byte) error { return nil }); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{p, p + ".bak"} {
		st, err := os.Stat(name)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm() != 0600 {
			t.Errorf("permissive umask exposed file: %o", st.Mode().Perm())
		}
	}
}

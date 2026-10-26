// Package safeio provides secure file operations for configs holding credentials.
package safeio

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

// SecurePerm is the mode for files that may contain API keys or tokens.
const SecurePerm = 0600

// WriteSecure writes data atomically with owner-only permissions.
func WriteSecure(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, SecurePerm); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Chmod(path, SecurePerm)
}

// WritePristineBackup stores the user's original config exactly once.
// Later syncs never clobber it, so a restore always returns the true
// pre-val2harness state instead of a previously synced copy.
func WritePristineBackup(path string, data []byte) error {
	backup := path + ".bak"
	if _, err := os.Stat(backup); err == nil {
		return nil
	}
	return WriteSecure(backup, data)
}

// ReadExisting returns the file bytes plus whether the file was present.
// A present-but-empty file reports found=true with empty data so callers
// can treat it as a fresh document rather than a parse failure.
func ReadExisting(path string) (data []byte, found bool, err error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return raw, true, nil
}

// IsBlank reports whether the config body carries no meaningful content.
func IsBlank(data []byte) bool {
	return len(bytes.TrimSpace(data)) == 0
}

// ParseGuard converts a parse failure into a refusal to overwrite.
// Rewriting a config we could not understand would delete every provider
// the user configured by hand, so the sync aborts instead.
func ParseGuard(path string, parseErr error) error {
	return fmt.Errorf(
		"refusing to overwrite %s: existing file could not be parsed (%v); "+
			"fix or remove the file, then sync again", path, parseErr)
}

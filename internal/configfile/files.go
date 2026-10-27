// Package configfile preserves user-owned configuration during local updates.
package configfile

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Update validates/serializes through change before retaining the first original
// backup and atomically publishing the new private file. A nil input means absent.
func Update(path string, change func([]byte) ([]byte, error)) error {
	return locked(path, func() error {
		original, exists, err := readRegular(path)
		if err != nil {
			return err
		}
		data, err := change(original)
		if err != nil {
			return fmt.Errorf("cannot update %s: %w", path, err)
		}
		if exists && bytes.Equal(original, data) {
			if err := tightenPrivate(path); err != nil {
				return err
			}
			_, hasBackup, err := readRegular(path + ".bak")
			if err != nil {
				return err
			}
			if hasBackup {
				return tightenPrivate(path + ".bak")
			}
			return nil
		}
		if exists {
			if err := preserveBackup(path+".bak", original); err != nil {
				return fmt.Errorf("cannot preserve original %s: %w", path, err)
			}
		}
		return replacePrivate(path, data)
	})
}

// Write atomically publishes private bytes without replacing an original backup.
func Write(path string, data []byte) error {
	return locked(path, func() error { return replacePrivate(path, data) })
}

// Restore validates a first-original backup and publishes it without backing up
// the already-modified target over that original.
func Restore(path string, validate func([]byte) error) error {
	return locked(path, func() error {
		data, exists, err := readRegular(path + ".bak")
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("original backup not found: %s.bak", path)
		}
		if err := validate(data); err != nil {
			return fmt.Errorf("invalid original backup: %w", err)
		}
		if err := tightenPrivate(path + ".bak"); err != nil {
			return err
		}
		return replacePrivate(path, data)
	})
}

func readRegular(path string) ([]byte, bool, error) {
	st, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !st.Mode().IsRegular() {
		return nil, false, fmt.Errorf("%s is not a regular file; linked/special settings require manual handling", path)
	}
	data, err := os.ReadFile(path)
	return data, true, err
}
func preserveBackup(path string, original []byte) error {
	_, exists, err := readRegular(path)
	if err != nil {
		return err
	}
	if exists {
		return tightenPrivate(path)
	}
	return replacePrivate(path, original)
}

func tightenPrivate(path string) error {
	st, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", path)
	}
	return os.Chmod(path, st.Mode().Perm()&0600)
}
func replacePrivate(path string, data []byte) error {
	st, err := os.Lstat(path)
	if err == nil {
		if !st.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular file", path)
		}
		if st.Mode().Perm()&0200 == 0 {
			return fmt.Errorf("%s is read-only", path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".val2harness-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = replaceFile(name, path); err != nil {
		return fmt.Errorf("cannot replace %s: %w", path, err)
	}
	return nil
}
func locked(path string, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	lockPath := path + ".val2harness.lock"
	if st, err := os.Lstat(lockPath); err == nil && !st.Mode().IsRegular() {
		return fmt.Errorf("unsafe settings lock: %s", lockPath)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := lockFile(f); err != nil {
		return err
	}
	defer unlockFile(f)
	return fn()
}

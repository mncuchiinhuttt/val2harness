package safeio

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafeioBackupAndPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "config.json")

	originalData := []byte(`{"key":"original"}`)
	if err := WriteSecure(filePath, originalData); err != nil {
		t.Fatalf("WriteSecure failed: %v", err)
	}

	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("Expected 0600 permissions, got %o", info.Mode().Perm())
	}

	if err := WritePristineBackup(filePath, originalData); err != nil {
		t.Fatalf("WritePristineBackup failed: %v", err)
	}

	modifiedData := []byte(`{"key":"modified"}`)
	if err := WritePristineBackup(filePath, modifiedData); err != nil {
		t.Fatalf("Second WritePristineBackup failed: %v", err)
	}

	backupData, err := os.ReadFile(filePath + ".bak")
	if err != nil {
		t.Fatalf("Failed to read backup: %v", err)
	}
	if string(backupData) != string(originalData) {
		t.Errorf("Pristine backup was overwritten! Expected %s, got %s", string(originalData), string(backupData))
	}
}

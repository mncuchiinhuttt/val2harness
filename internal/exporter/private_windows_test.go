//go:build windows

package exporter

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func readableWindowsDirectory(t *testing.T) (string, string) {
	t.Helper()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + user.User.Sid.String() + ")(A;OICI;GR;;;BU)")
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		t.Fatal(err)
	}
	return dir, user.User.Sid.String()
}

func assertWindowsPrivate(t *testing.T, path, userSID string) {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	control, _, err := sd.Control()
	if err != nil {
		t.Fatal(err)
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		t.Error("export inherits its parent DACL; expected protected permissions")
	}
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil {
		t.Fatalf("missing private DACL: %v", err)
	}
	if dacl.AceCount != 1 {
		t.Errorf("export grants %d ACL entries; expected only current user", dacl.AceCount)
	}
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil {
			t.Fatal(err)
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceFlags&windows.INHERITED_ACE != 0 || sid.String() != userSID {
			t.Error("export DACL grants an inherited or other-principal ACE")
		}
	}
}

func TestWindowsExportsPrivateInsideUsersReadableDirectory(t *testing.T) {
	dir, userSID := readableWindowsDirectory(t)
	before, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	count, _, err := ExportAllChats(context.Background(), exportClient(t, []string{"synthetic-chat"}, nil), dir)
	if err != nil || count != 1 {
		t.Fatalf("count=%d error=%v", count, err)
	}
	for _, name := range exportFiles(t, dir) {
		assertWindowsPrivate(t, filepath.Join(dir, name), userSID)
	}
	after, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	if before.String() != after.String() {
		t.Fatal("export changed existing directory permissions")
	}
}

func TestWindowsExportRefusesExistingFileWithoutChangingACL(t *testing.T) {
	dir, _ := readableWindowsDirectory(t)
	path := filepath.Join(dir, "existing.json")
	if err := os.WriteFile(path, []byte("unrelated"), 0600); err != nil {
		t.Fatal(err)
	}
	before, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := writeNewPrivate(root, "existing.json", []byte("replacement")); err == nil {
		t.Fatal("existing output was replaced")
	}
	after, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	if before.String() != after.String() {
		t.Fatal("existing output ACL changed")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "unrelated" {
		t.Fatalf("existing content changed: %q error=%v", data, err)
	}
}

func TestWindowsNewRunDirectoryPrivateAtCreation(t *testing.T) {
	parent, userSID := readableWindowsDirectory(t)
	before, err := windows.GetNamedSecurityInfo(parent, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	first, err := NewRunDirectory(parent)
	if err != nil {
		t.Fatal(err)
	}
	assertWindowsPrivate(t, first, userSID)
	second, err := NewRunDirectory(parent)
	if err != nil {
		t.Fatal(err)
	}
	assertWindowsPrivate(t, second, userSID)
	if first == second {
		t.Fatal("snapshot directory reused")
	}
	after, err := windows.GetNamedSecurityInfo(parent, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	if before.String() != after.String() {
		t.Fatal("snapshot changed existing parent ACL")
	}
}

func TestWindowsPrivateCreatorRejectsNativePathAliases(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	for _, name := range []string{"", ".", "..", "../outside", `..\outside`, `C:\outside`, "NUL", "CON.json", "file.json:stream", "trailing.", "trailing ", "bad\x00name"} {
		if err := writeNewPrivate(root, name, []byte("synthetic")); err == nil {
			t.Errorf("native path accepted: %q", name)
		}
	}
	if files := exportFiles(t, dir); len(files) != 0 {
		t.Fatalf("invalid names created files: %v", files)
	}
}

func TestWindowsPrivateCreatorUsesPinnedDirectory(t *testing.T) {
	parent := t.TempDir()
	original, moved := filepath.Join(parent, "original"), filepath.Join(parent, "moved")
	if err := os.Mkdir(original, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(original)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := os.Rename(original, moved); err != nil {
		if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
			t.Fatal(err)
		}
		// Windows Root handles can deny directory renames altogether. That
		// also preserves the pinned-directory invariant against replacement.
		if err := writeNewPrivate(root, "pinned.json", []byte("synthetic")); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(original, "pinned.json"))
		if err != nil || string(data) != "synthetic" {
			t.Fatalf("lost pinned directory: data=%q error=%v", data, err)
		}
		return
	}
	if err := os.Mkdir(original, 0700); err != nil {
		t.Fatal(err)
	}
	if err := writeNewPrivate(root, "pinned.json", []byte("synthetic")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(moved, "pinned.json"))
	if err != nil || string(data) != "synthetic" {
		t.Fatalf("lost pinned directory: data=%q error=%v", data, err)
	}
	if files := exportFiles(t, original); len(files) != 0 {
		t.Fatalf("wrote through replaced absolute path: %v", files)
	}
}

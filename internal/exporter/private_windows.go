//go:build windows

package exporter

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func createNewPrivateFile(root *os.Root, name string) (*os.File, error) {
	return createPrivateWindows(root, name, false)
}

func createNewPrivateDirectory(root *os.Root, name string) error {
	f, err := createPrivateWindows(root, name, true)
	if err != nil {
		return err
	}
	return f.Close()
}

// NtCreateFile accepts the DACL at creation, without a public-file interval.
// RootDirectory is a pinned handle; FILE_CREATE refuses every existing object,
// including reparse points. Restrict names to one ordinary component because NT
// paths bypass the normal Win32 path/device normalization.
func createPrivateWindows(root *os.Root, name string, directory bool) (*os.File, error) {
	base, _, _ := strings.Cut(name, ".")
	if !filepath.IsLocal(name) || !filepath.IsLocal(base) || name == "." || name == ".." || strings.ContainsAny(name, `/\:<>"|?*`) || strings.TrimRight(name, ". ") != name || strings.IndexFunc(name, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
		return nil, &os.PathError{Op: "create private export", Path: name, Err: windows.ERROR_INVALID_NAME}
	}
	dir, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	var volumeFlags uint32
	if err := windows.GetVolumeInformationByHandle(windows.Handle(dir.Fd()), nil, 0, nil, nil, &volumeFlags, nil, 0); err != nil {
		return nil, fmt.Errorf("check export filesystem ACL support: %w", err)
	}
	if volumeFlags&windows.FILE_PERSISTENT_ACLS == 0 {
		return nil, fmt.Errorf("export filesystem does not support persistent private ACLs")
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, fmt.Errorf("identify export owner: %w", err)
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		return nil, fmt.Errorf("prepare private export ACL: %w", err)
	}
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return nil, err
	}
	oa := &windows.OBJECT_ATTRIBUTES{RootDirectory: windows.Handle(dir.Fd()), ObjectName: objectName, Attributes: windows.OBJ_CASE_INSENSITIVE, SecurityDescriptor: sd}
	oa.Length = uint32(unsafe.Sizeof(*oa))
	options := uint32(windows.FILE_NON_DIRECTORY_FILE)
	if directory {
		options = windows.FILE_DIRECTORY_FILE
	}
	var handle windows.Handle
	var status windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(&handle, windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE|windows.SYNCHRONIZE, oa, &status, nil, windows.FILE_ATTRIBUTE_NORMAL,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, windows.FILE_CREATE,
		options|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	runtime.KeepAlive(sd)
	runtime.KeepAlive(objectName)
	if err != nil {
		if nt, ok := err.(windows.NTStatus); ok {
			err = nt.Errno()
		}
		return nil, &os.PathError{Op: "create private export", Path: name, Err: err}
	}
	return os.NewFile(uintptr(handle), name), nil
}

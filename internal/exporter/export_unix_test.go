//go:build unix

package exporter

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestExportRefusesFIFOWithoutOpeningIt(t *testing.T) {
	client := exportClient(t, []string{"synthetic-chat"}, nil)
	baseline := t.TempDir()
	if _, _, err := ExportAllChats(context.Background(), client, baseline); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	var target string
	for _, name := range exportFiles(t, baseline) {
		if strings.HasSuffix(name, ".json") {
			target = filepath.Join(dir, name)
			break
		}
	}
	if err := unix.Mkfifo(target, 0600); err != nil {
		t.Skipf("FIFO fixture unavailable: %v", err)
	}
	// A regression that follows/opens the FIFO should fail assertions rather
	// than hang indefinitely waiting for a reader.
	reader, err := unix.Open(target, unix.O_RDONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(reader)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	count, _, err := ExportAllChats(ctx, client, dir)
	if count != 0 || err == nil {
		t.Fatalf("count=%d err=%v", count, err)
	}
	st, err := os.Lstat(target)
	if err != nil || st.Mode()&os.ModeNamedPipe == 0 {
		t.Fatalf("FIFO changed: info=%v err=%v", st, err)
	}
}

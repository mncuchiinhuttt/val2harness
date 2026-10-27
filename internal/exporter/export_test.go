package exporter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	"val2harness/internal/valclient"
)

func exportClient(t *testing.T, ids []string, detail func(http.ResponseWriter, *http.Request)) *valclient.Client {
	t.Helper()
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/chats/" {
			items := []map[string]string{}
			for _, id := range ids {
				items = append(items, map[string]string{"id": id, "title": "CON: ../../ same title"})
			}
			json.NewEncoder(w).Encode(items)
			return
		}
		if detail != nil {
			detail(w, r)
			return
		}
		fmt.Fprint(w, `{"chat":{"messages":[{"role":"user","content":"Synthetic question"},{"role":"assistant","content":"Synthetic reply"}]},"metadata":{"precise":9007199254740993}}`)
	}))
	t.Cleanup(up.Close)
	return valclient.NewClient(up.URL, "synthetic")
}

func exportFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names
}

func TestNewRunDirectoriesAreUniquePrivateAndUsable(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "snapshots")
	seen := map[string]bool{}
	for i := 0; i < 3; i++ {
		dir, err := NewRunDirectory(parent)
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Dir(dir) != parent || !strings.HasPrefix(filepath.Base(dir), "run-") || seen[dir] {
			t.Fatalf("unsafe or reused run directory: %q", dir)
		}
		seen[dir] = true
		st, err := os.Stat(dir)
		if err != nil || !st.IsDir() {
			t.Fatalf("snapshot directory missing: %v", err)
		}
		if runtime.GOOS != "windows" && st.Mode().Perm()&0077 != 0 {
			t.Fatalf("snapshot permissions %o", st.Mode().Perm())
		}
		count, _, err := ExportAllChats(context.Background(), exportClient(t, []string{"synthetic-chat"}, nil), dir)
		if err != nil || count != 1 {
			t.Fatalf("new snapshot cannot export: count=%d error=%v", count, err)
		}
	}
}

func TestExportShortAndCollidingIDsPreserveBothFormats(t *testing.T) {
	client := exportClient(t, []string{"x", "abcdefgh-one", "abcdefgh-two", "CON", "NUL", "COM1", "Case", "case"}, nil)
	dir := filepath.Join(t.TempDir(), "exports")
	count, gotDir, err := ExportAllChats(context.Background(), client, dir)
	if err != nil || count != 8 || gotDir != dir {
		t.Fatalf("count=%d dir=%q err=%v", count, gotDir, err)
	}
	files := exportFiles(t, dir)
	if len(files) != 16 {
		t.Fatalf("expected two distinct files per chat, got %v", files)
	}
	for _, name := range files {
		if strings.ContainsAny(name, `/\:`) || filepath.Base(name) != name {
			t.Fatalf("unsafe filename %q", name)
		}
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(name, ".md") && (!strings.Contains(string(body), "Synthetic question") || !strings.Contains(string(body), "Synthetic reply")) {
			t.Fatalf("Markdown lost messages: %s", body)
		}
		if strings.HasSuffix(name, ".json") && !strings.Contains(string(body), "9007199254740993") {
			t.Fatalf("JSON lost integer precision: %s", body)
		}
		if runtime.GOOS != "windows" {
			st, _ := os.Stat(filepath.Join(dir, name))
			if st.Mode().Perm()&0077 != 0 {
				t.Fatalf("export permissions %o", st.Mode().Perm())
			}
		}
	}
	other := filepath.Join(t.TempDir(), "exports")
	if _, _, err := ExportAllChats(context.Background(), client, other); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(files, exportFiles(t, other)) {
		t.Fatal("same IDs produced nondeterministic filenames")
	}
	if runtime.GOOS != "windows" {
		st, _ := os.Stat(dir)
		if st.Mode().Perm()&0077 != 0 {
			t.Fatalf("new export directory permissions %o", st.Mode().Perm())
		}
	}
}

func TestExportRejectsPathIDsWithoutFetchingThem(t *testing.T) {
	details := 0
	client := exportClient(t, []string{"../outside123", `..\outside123`, "/absolute", ".", "..", ""}, func(w http.ResponseWriter, r *http.Request) { details++; fmt.Fprint(w, `{}`) })
	parent := t.TempDir()
	dir := filepath.Join(parent, "exports")
	count, _, err := ExportAllChats(context.Background(), client, dir)
	if err == nil || count != 0 || details != 0 {
		t.Fatalf("count=%d details=%d err=%v", count, details, err)
	}
	if files := exportFiles(t, dir); len(files) != 0 {
		t.Fatalf("unsafe IDs created %v", files)
	}
	if entries := exportFiles(t, parent); !reflect.DeepEqual(entries, []string{"exports"}) {
		t.Fatalf("escaped output: %v", entries)
	}
}

func TestExportPartialSuccessReportsFetchFailure(t *testing.T) {
	client := exportClient(t, []string{"failure-one", "success-one", "failure-two"}, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "failure") {
			http.Error(w, "synthetic upstream failure", http.StatusBadGateway)
			return
		}
		fmt.Fprint(w, `{"messages":[{"role":"assistant","content":"saved"}]}`)
	})
	dir := t.TempDir()
	count, gotDir, err := ExportAllChats(context.Background(), client, dir)
	if count != 1 || gotDir != dir || err == nil {
		t.Fatalf("count=%d dir=%q err=%v", count, gotDir, err)
	}
	if len(exportFiles(t, dir)) != 2 {
		t.Fatal("partial success did not preserve completed chat")
	}
	if !strings.Contains(err.Error(), "failure-one") || !strings.Contains(err.Error(), "failure-two") {
		t.Fatalf("missing per-chat errors: %v", err)
	}
}

func TestExportNeverOverwritesExistingPaths(t *testing.T) {
	client := exportClient(t, []string{"first-chat", "second-chat"}, nil)
	baseline := t.TempDir()
	if _, _, err := ExportAllChats(context.Background(), client, baseline); err != nil {
		t.Fatal(err)
	}
	files := exportFiles(t, baseline)
	for _, kind := range []string{"file", "directory", "symlink", "hardlink"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			outside := filepath.Join(t.TempDir(), "unrelated")
			if err := os.WriteFile(outside, []byte("keep me"), 0600); err != nil {
				t.Fatal(err)
			}
			// Block one Markdown target, exercising a JSON-only incomplete chat.
			var target string
			for _, name := range files {
				if strings.HasSuffix(name, ".md") {
					target = filepath.Join(dir, name)
					break
				}
			}
			var err error
			switch kind {
			case "file":
				err = os.WriteFile(target, []byte("keep me"), 0600)
			case "directory":
				err = os.Mkdir(target, 0700)
			case "symlink":
				err = os.Symlink(outside, target)
			case "hardlink":
				err = os.Link(outside, target)
			}
			if err != nil {
				if kind == "symlink" || kind == "hardlink" {
					t.Skipf("link fixture unavailable: %v", err)
				}
				t.Fatal(err)
			}
			count, _, err := ExportAllChats(context.Background(), client, dir)
			if err == nil || count != 1 {
				t.Fatalf("count=%d err=%v", count, err)
			}
			bytes, _ := os.ReadFile(outside)
			if string(bytes) != "keep me" {
				t.Fatalf("unrelated target changed: %q", bytes)
			}
			if kind != "directory" {
				bytes, _ = os.ReadFile(target)
				if string(bytes) != "keep me" {
					t.Fatalf("existing file overwritten: %q", bytes)
				}
			}
		})
	}
}

func TestExportCancellationReturnsContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := exportClient(t, []string{"first-chat", "second-chat"}, func(w http.ResponseWriter, r *http.Request) { cancel(); <-r.Context().Done() })
	count, _, err := ExportAllChats(ctx, client, t.TempDir())
	if count != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("count=%d err=%v", count, err)
	}
}

func TestExportOutputDirectoryFailureIsReported(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	count, _, err := ExportAllChats(context.Background(), exportClient(t, nil, nil), filepath.Join(path, "exports"))
	if count != 0 || err == nil {
		t.Fatalf("count=%d err=%v", count, err)
	}
}

func TestRenderSupportedMessageShapesInStableHistoryOrder(t *testing.T) {
	for _, body := range []string{
		`{"chat":{"history":{"messages":{"z":{"id":"z","parentId":null,"role":"user","content":"first"},"a":{"id":"a","parentId":"z","role":"assistant","content":"second"}}}}}`,
		`{"messages":{"z":{"id":"z","parentId":null,"timestamp":1,"role":"user","content":"first"},"a":{"id":"a","parentId":"z","timestamp":2,"role":"assistant","content":"second"}}}`,
		`{"chat":{"messages":[{"role":"user","content":"first"},{"role":"assistant","content":[{"type":"text","text":"second"},{"type":"image_url","image_url":{"url":"synthetic"}}]}]}}`,
		`{"messages":[{"role":"user","content":"first"},{"role":"assistant","content":"second"}]}`,
	} {
		var data map[string]any
		if err := json.Unmarshal([]byte(body), &data); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 40; i++ {
			markdown := renderChatMarkdown("Synthetic", data)
			first, second := strings.Index(markdown, "first"), strings.Index(markdown, "second")
			if first < 0 || second < 0 || first >= second {
				t.Fatalf("lost or reordered messages: %s", markdown)
			}
			if strings.Contains(body, "image_url") && !strings.Contains(markdown, "Non-text or unsupported content block retained in the companion JSON") {
				t.Fatalf("unsupported block silently omitted: %s", markdown)
			}
		}
	}
}

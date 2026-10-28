package version

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type updateTransport func(*http.Request) (*http.Response, error)

func (f updateTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func fakeUpdateHTTP(t *testing.T, serve func(*http.Request) (int, []byte)) {
	t.Helper()
	old := http.DefaultTransport
	http.DefaultTransport = updateTransport(func(r *http.Request) (*http.Response, error) {
		code, body := serve(r)
		return &http.Response{StatusCode: code, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header), Request: r}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = old })
}

func archiveName() string {
	ext := ".tar.gz"
	if runtime.GOOS == "windows" {
		ext = ".zip"
	}
	return "val2harness-" + runtime.GOOS + "-" + runtime.GOARCH + ext
}

func releaseBody(t *testing.T, archiveURL, checksumURL string) []byte {
	t.Helper()
	assets := []map[string]string{{"name": archiveName(), "browser_download_url": archiveURL}}
	if checksumURL != "" {
		assets = append(assets, map[string]string{"name": "checksums.txt", "browser_download_url": checksumURL})
	}
	data, err := json.Marshal(map[string]any{"tag_name": "v9.0.0", "published_at": "2026-10-28T00:00:00Z", "assets": assets})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func testAssetURL(name string) string {
	return "https://github.com/mncuchiinhuttt/val2harness/releases/download/v9.0.0/" + name
}

func makeUpdateArchive(t *testing.T, zipFormat bool, size int64) []byte {
	t.Helper()
	var out bytes.Buffer
	if zipFormat {
		zw := zip.NewWriter(&out)
		file, err := zw.Create("val2harness.exe")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.CopyN(file, zeroReader{}, size); err != nil {
			t.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
	} else {
		gz := gzip.NewWriter(&out)
		tw := tar.NewWriter(gz)
		if err := tw.WriteHeader(&tar.Header{Name: "val2harness", Mode: 0755, Size: size}); err != nil {
			t.Fatal(err)
		}
		if _, err := io.CopyN(tw, zeroReader{}, size); err != nil {
			t.Fatal(err)
		}
		if err := tw.Close(); err != nil {
			t.Fatal(err)
		}
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return out.Bytes()
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func TestCheckUpdateRejectsOversizedAndTrailingMetadata(t *testing.T) {
	for _, body := range [][]byte{append(releaseBody(t, testAssetURL(archiveName()), ""), bytes.Repeat([]byte(" "), (1<<20)+1)...), append(releaseBody(t, testAssetURL(archiveName()), ""), []byte("{}")...)} {
		t.Run(fmt.Sprint(len(body)), func(t *testing.T) {
			fakeUpdateHTTP(t, func(*http.Request) (int, []byte) { return http.StatusOK, body })
			if _, err := CheckUpdate(context.Background()); err == nil {
				t.Fatal("unbounded or trailing release metadata accepted")
			}
		})
	}
}

func TestSelfUpdateRejectsNonOKDownloads(t *testing.T) {
	archive := makeUpdateArchive(t, runtime.GOOS == "windows", 8)
	for _, failedPath := range []string{"metadata", "checksums", "archive"} {
		t.Run(failedPath, func(t *testing.T) {
			path := executableFixture(t)
			fakeUpdateHTTP(t, func(r *http.Request) (int, []byte) {
				kind := "archive"
				body := archive
				if r.URL.Host == "api.github.com" {
					kind, body = "metadata", releaseBody(t, testAssetURL(archiveName()), testAssetURL("checksums.txt"))
				} else if r.URL.String() == testAssetURL("checksums.txt") {
					kind, body = "checksums", []byte(fmt.Sprintf("%x  %s\n", sha256.Sum256(archive), archiveName()))
				}
				if kind == failedPath {
					return http.StatusBadGateway, body
				}
				return http.StatusOK, body
			})
			if _, err := SelfUpdate(context.Background()); err == nil {
				t.Error("non-OK response accepted")
			}
			assertExecutablePreserved(t, path)
		})
	}
}

func TestSelfUpdateRollsBackFailedWindowsReplacement(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows replacement sequence")
	}
	path := executableFixture(t)
	archive := makeUpdateArchive(t, true, 8)
	fakeUpdateHTTP(t, func(r *http.Request) (int, []byte) {
		if r.URL.Host == "api.github.com" {
			return http.StatusOK, releaseBody(t, testAssetURL(archiveName()), testAssetURL("checksums.txt"))
		}
		if r.URL.String() == testAssetURL("checksums.txt") {
			return http.StatusOK, []byte(fmt.Sprintf("%x  %s\n", sha256.Sum256(archive), archiveName()))
		}
		return http.StatusOK, archive
	})
	oldRename := renameFile
	failed := false
	renameFile = func(from, to string) error {
		if to == path && !failed {
			failed = true
			return errors.New("synthetic install failure")
		}
		return os.Rename(from, to)
	}
	t.Cleanup(func() { renameFile = oldRename })
	if _, err := SelfUpdate(context.Background()); err == nil {
		t.Fatal("replacement failure not reported")
	}
	assertExecutablePreserved(t, path)
}

func TestCheckUpdateRejectsUnreviewedAssetURLs(t *testing.T) {
	for _, raw := range []string{
		"http://github.com/mncuchiinhuttt/val2harness/releases/download/v9.0.0/" + archiveName(),
		"https://github.com.evil.test/mncuchiinhuttt/val2harness/releases/download/v9.0.0/" + archiveName(),
		testAssetURL(archiveName()) + "?download=1",
		strings.Replace(testAssetURL(archiveName()), "v9.0.0/", "other-tag/", 1),
	} {
		t.Run(raw, func(t *testing.T) {
			fakeUpdateHTTP(t, func(*http.Request) (int, []byte) { return http.StatusOK, releaseBody(t, raw, "") })
			if _, err := CheckUpdate(context.Background()); err == nil {
				t.Fatal("unreviewed release asset URL accepted")
			}
		})
	}
}

func TestArchiveExtractionRejectsOversizedAndEmptyBinary(t *testing.T) {
	for _, zipFormat := range []bool{false, true} {
		for _, size := range []int64{0, (32 << 20) + 1} {
			t.Run(fmt.Sprintf("zip=%v,size=%d", zipFormat, size), func(t *testing.T) {
				data := makeUpdateArchive(t, zipFormat, size)
				var err error
				if zipFormat {
					_, err = extractFromZip(data)
				} else {
					_, err = extractFromTarGz(data)
				}
				if err == nil {
					t.Fatal("empty or oversized executable accepted")
				}
			})
		}
	}
}

func executableFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gateway.exe")
	for _, suffix := range []string{"", ".new", ".old"} {
		if err := os.WriteFile(path+suffix, []byte("original"+suffix), 0700); err != nil {
			t.Fatal(err)
		}
	}
	old := executablePath
	executablePath = func() (string, error) { return path, nil }
	t.Cleanup(func() { executablePath = old })
	return path
}

func assertExecutablePreserved(t *testing.T, path string) {
	t.Helper()
	for _, suffix := range []string{"", ".new", ".old"} {
		data, err := os.ReadFile(path + suffix)
		if err != nil || string(data) != "original"+suffix {
			t.Errorf("unrelated executable file changed: %s %q %v", suffix, data, err)
		}
	}
}

func TestSelfUpdateRefusesMissingMalformedAndMismatchedChecksums(t *testing.T) {
	archive := makeUpdateArchive(t, runtime.GOOS == "windows", 8)
	for _, checksum := range []string{"", "malformed", strings.Repeat("0", 64) + "  " + archiveName() + "\n", strings.Repeat("0", 64) + "  different-archive.zip\n", strings.Repeat(fmt.Sprintf("%x  %s\n", sha256.Sum256(archive), archiveName()), 2)} {
		t.Run(checksum, func(t *testing.T) {
			path := executableFixture(t)
			fakeUpdateHTTP(t, func(r *http.Request) (int, []byte) {
				if r.URL.Host == "api.github.com" {
					checksums := ""
					if checksum != "" {
						checksums = testAssetURL("checksums.txt")
					}
					return http.StatusOK, releaseBody(t, testAssetURL(archiveName()), checksums)
				}
				if r.URL.String() == testAssetURL("checksums.txt") {
					return http.StatusOK, []byte(checksum)
				}
				return http.StatusOK, archive
			})
			if _, err := SelfUpdate(context.Background()); err == nil {
				t.Error("update without a matching release checksum accepted")
			}
			assertExecutablePreserved(t, path)
		})
	}
}

func TestSelfUpdateVerifiesArchiveAndPreservesUnrelatedStagingNames(t *testing.T) {
	path := executableFixture(t)
	archive := makeUpdateArchive(t, runtime.GOOS == "windows", 8)
	fakeUpdateHTTP(t, func(r *http.Request) (int, []byte) {
		switch r.URL.String() {
		case "https://api.github.com/repos/mncuchiinhuttt/val2harness/releases/latest":
			return http.StatusOK, releaseBody(t, testAssetURL(archiveName()), testAssetURL("checksums.txt"))
		case testAssetURL("checksums.txt"):
			return http.StatusOK, []byte(fmt.Sprintf("%x  %s\n", sha256.Sum256(archive), archiveName()))
		case testAssetURL(archiveName()):
			return http.StatusOK, archive
		default:
			t.Fatalf("unexpected outbound URL: %s", r.URL)
			return 0, nil
		}
	})
	if got, err := SelfUpdate(context.Background()); err != nil || got != "v9.0.0" {
		t.Fatalf("verified update failed: %s %v", got, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, make([]byte, 8)) {
		t.Fatalf("verified executable was not installed: %q %v", data, err)
	}
	for _, suffix := range []string{".new", ".old"} {
		data, err := os.ReadFile(path + suffix)
		if err != nil || string(data) != "original"+suffix {
			t.Errorf("unrelated %s file overwritten: %q %v", suffix, data, err)
		}
	}
}

type countedBody struct {
	reader io.Reader
	read   int64
}

func (r *countedBody) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.read += int64(n)
	return n, err
}

func TestSelfUpdateStopsOversizedDownloads(t *testing.T) {
	for _, test := range []struct {
		kind  string
		limit int64
	}{{"metadata", 1 << 20}, {"checksums", 256 << 10}, {"archive", 64 << 20}} {
		t.Run(test.kind, func(t *testing.T) {
			path := executableFixture(t)
			size := test.limit + 4096
			body := &countedBody{reader: io.LimitReader(zeroReader{}, size)}
			hash := sha256.New()
			if _, err := io.CopyN(hash, zeroReader{}, size); err != nil {
				t.Fatal(err)
			}
			old := http.DefaultTransport
			http.DefaultTransport = updateTransport(func(r *http.Request) (*http.Response, error) {
				kind := "archive"
				var reader io.Reader
				if r.URL.Host == "api.github.com" {
					kind = "metadata"
					reader = bytes.NewReader(releaseBody(t, testAssetURL(archiveName()), testAssetURL("checksums.txt")))
				} else if r.URL.String() == testAssetURL("checksums.txt") {
					kind = "checksums"
					reader = strings.NewReader(fmt.Sprintf("%x  %s\n", hash.Sum(nil), archiveName()))
				}
				if kind == test.kind {
					reader = body
				}
				if reader == nil {
					t.Fatalf("unexpected update request: %s", r.URL)
				}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), ContentLength: -1, Body: io.NopCloser(reader), Request: r}, nil
			})
			t.Cleanup(func() { http.DefaultTransport = old })
			if _, err := SelfUpdate(context.Background()); err == nil {
				t.Error("oversized download accepted")
			}
			if body.read == 0 || body.read > test.limit+1 {
				t.Errorf("download consumed %d bytes; expected at most %d", body.read, test.limit+1)
			}
			assertExecutablePreserved(t, path)
		})
	}
}

func TestSelfUpdateRestrictsAssetRedirects(t *testing.T) {
	for _, location := range []string{
		"http://release-assets.githubusercontent.com/fixture",
		"https://evil.test/fixture",
		"https://release-assets.githubusercontent.com:444/fixture",
		"https://user@release-assets.githubusercontent.com/fixture",
	} {
		t.Run(location, func(t *testing.T) {
			path := executableFixture(t)
			old := http.DefaultTransport
			untrustedCalls := 0
			http.DefaultTransport = updateTransport(func(r *http.Request) (*http.Response, error) {
				code, header := http.StatusOK, make(http.Header)
				var body []byte
				if r.URL.Host == "api.github.com" {
					body = releaseBody(t, testAssetURL(archiveName()), testAssetURL("checksums.txt"))
				} else if r.URL.String() == testAssetURL("checksums.txt") {
					code = http.StatusFound
					header.Set("Location", location)
				} else {
					untrustedCalls++
				}
				return &http.Response{StatusCode: code, Header: header, Body: io.NopCloser(bytes.NewReader(body)), Request: r}, nil
			})
			t.Cleanup(func() { http.DefaultTransport = old })
			if _, err := SelfUpdate(context.Background()); err == nil {
				t.Error("unreviewed asset redirect accepted")
			}
			if untrustedCalls != 0 {
				t.Error("request followed an unreviewed redirect")
			}
			assertExecutablePreserved(t, path)
		})
	}
}

func TestSelfUpdateRetainsRecoveryCopyWhenWindowsRollbackFails(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows replacement sequence")
	}
	path := executableFixture(t)
	archive := makeUpdateArchive(t, true, 8)
	fakeUpdateHTTP(t, func(r *http.Request) (int, []byte) {
		if r.URL.Host == "api.github.com" {
			return http.StatusOK, releaseBody(t, testAssetURL(archiveName()), testAssetURL("checksums.txt"))
		}
		if r.URL.String() == testAssetURL("checksums.txt") {
			return http.StatusOK, []byte(fmt.Sprintf("%x  %s\n", sha256.Sum256(archive), archiveName()))
		}
		return http.StatusOK, archive
	})
	oldRename := renameFile
	renameFile = func(from, to string) error {
		if to == path {
			return errors.New("synthetic install and rollback failure")
		}
		return os.Rename(from, to)
	}
	t.Cleanup(func() { renameFile = oldRename })
	_, updateErr := SelfUpdate(context.Background())
	if updateErr == nil {
		t.Fatal("rollback failure was not reported")
	}
	backups, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".val2harness-previous-*"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("recoverable previous executable missing: %v %v", backups, err)
	}
	data, err := os.ReadFile(backups[0])
	if err != nil || string(data) != "original" || !strings.Contains(updateErr.Error(), backups[0]) {
		t.Fatalf("previous executable or recovery instructions lost: %q %v %v", data, err, updateErr)
	}
	for _, suffix := range []string{".new", ".old"} {
		data, err := os.ReadFile(path + suffix)
		if err != nil || string(data) != "original"+suffix {
			t.Errorf("unrelated %s was changed", suffix)
		}
	}
}

func TestCheckUpdateRefusesCancelledRequestWithoutHTTP(t *testing.T) {
	calls := 0
	fakeUpdateHTTP(t, func(*http.Request) (int, []byte) { calls++; return http.StatusOK, nil })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := CheckUpdate(ctx); err == nil {
		t.Error("cancelled check accepted")
	}
	if calls != 0 {
		t.Error("cancelled check made an HTTP request")
	}
}

func TestSelfUpdateAllowsReviewedReleaseStorageRedirects(t *testing.T) {
	for _, host := range []string{"release-assets.githubusercontent.com", "objects.githubusercontent.com"} {
		t.Run(host, func(t *testing.T) {
			path := executableFixture(t)
			archive := makeUpdateArchive(t, runtime.GOOS == "windows", 8)
			old := http.DefaultTransport
			http.DefaultTransport = updateTransport(func(r *http.Request) (*http.Response, error) {
				code, header := http.StatusOK, make(http.Header)
				var body []byte
				switch r.URL.String() {
				case "https://api.github.com/repos/mncuchiinhuttt/val2harness/releases/latest":
					body = releaseBody(t, testAssetURL(archiveName()), testAssetURL("checksums.txt"))
				case testAssetURL("checksums.txt"):
					code = http.StatusFound
					header.Set("Location", "https://"+host+"/checksums?signature=synthetic")
				case testAssetURL(archiveName()):
					code = http.StatusFound
					header.Set("Location", "https://"+host+"/archive?signature=synthetic")
				case "https://" + host + "/checksums?signature=synthetic":
					body = []byte(fmt.Sprintf("%x  %s\n", sha256.Sum256(archive), archiveName()))
				case "https://" + host + "/archive?signature=synthetic":
					body = archive
				default:
					t.Fatalf("unexpected update URL: %s", r.URL)
				}
				return &http.Response{StatusCode: code, Header: header, Body: io.NopCloser(bytes.NewReader(body)), Request: r}, nil
			})
			t.Cleanup(func() { http.DefaultTransport = old })
			if _, err := SelfUpdate(context.Background()); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(data, make([]byte, 8)) {
				t.Fatalf("redirected archive was not installed: %q %v", data, err)
			}
		})
	}
}

func TestSelfUpdateAtCurrentVersionDoesNotDownloadOrSelectExecutable(t *testing.T) {
	fakeUpdateHTTP(t, func(r *http.Request) (int, []byte) {
		if r.URL.String() != "https://api.github.com/repos/mncuchiinhuttt/val2harness/releases/latest" {
			t.Fatalf("current version downloaded an asset: %s", r.URL)
		}
		return http.StatusOK, []byte(`{"tag_name":"v1.0.0","assets":[]}`)
	})
	old := executablePath
	executablePath = func() (string, error) { t.Fatal("current version selected an executable"); return "", nil }
	t.Cleanup(func() { executablePath = old })
	if got, err := SelfUpdate(context.Background()); err != nil || got != CurrentVersion {
		t.Fatalf("unexpected current-version outcome: %s %v", got, err)
	}
}

package version

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const CurrentVersion = "v1.0.0"
const RepoName = "mncuchiinhuttt/val2harness"

const (
	maxMetadataBytes  = 1 << 20
	maxChecksumBytes  = 256 << 10
	maxArchiveBytes   = 64 << 20
	maxBinaryBytes    = 32 << 20
	maxExpandedBytes  = 64 << 20
	maxArchiveEntries = 1024
)

var executablePath = os.Executable
var renameFile = os.Rename

// Release metadata stays on the API endpoint. Asset redirects may reach only
// GitHub's HTTPS release storage hosts, and never carry credentials.
var updateHTTPClient = &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
	if len(via) == 0 || len(via) > 3 || via[0].URL.Host != "github.com" {
		return fmt.Errorf("unexpected update redirect")
	}
	u := req.URL
	if u.Scheme != "https" || u.User != nil || u.Fragment != "" || (u.Host != "release-assets.githubusercontent.com" && u.Host != "objects.githubusercontent.com") {
		return fmt.Errorf("untrusted update redirect")
	}
	return nil
}}

type UpdateInfo struct {
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	HasUpdate      bool   `json:"has_update"`
	ReleaseNotes   string `json:"release_notes"`
	DownloadURL    string `json:"download_url"`
	PublishedAt    string `json:"published_at"`
}

type ghRelease struct {
	TagName     string    `json:"tag_name"`
	Body        string    `json:"body"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func targetArchive() string {
	ext := ".tar.gz"
	if runtime.GOOS == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("val2harness-%s-%s%s", runtime.GOOS, runtime.GOARCH, ext)
}

func CheckUpdate(ctx context.Context) (*UpdateInfo, error) {
	info, _, err := checkUpdate(ctx)
	return info, err
}

func checkUpdate(ctx context.Context) (*UpdateInfo, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	data, err := downloadBytes(ctx, "https://api.github.com/repos/"+RepoName+"/releases/latest", maxMetadataBytes)
	if err != nil {
		return nil, "", err
	}
	var rel ghRelease
	if err := json.Unmarshal(data, &rel); err != nil {
		return nil, "", fmt.Errorf("invalid release metadata: %w", err)
	}
	tag := rel.TagName
	if len(tag) == 0 || len(tag) > 128 || tag == "." || tag == ".." || strings.IndexFunc(tag, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-')
	}) >= 0 {
		return nil, "", fmt.Errorf("invalid release tag")
	}
	urls := make(map[string]string)
	for _, asset := range rel.Assets {
		if asset.Name != targetArchive() && asset.Name != "checksums.txt" {
			continue
		}
		expected := "https://github.com/" + RepoName + "/releases/download/" + tag + "/" + asset.Name
		if asset.BrowserDownloadURL != expected {
			return nil, "", fmt.Errorf("untrusted release asset URL for %s", asset.Name)
		}
		if _, exists := urls[asset.Name]; exists {
			return nil, "", fmt.Errorf("duplicate release asset: %s", asset.Name)
		}
		urls[asset.Name] = expected
	}
	return &UpdateInfo{
		CurrentVersion: CurrentVersion,
		LatestVersion:  tag,
		HasUpdate:      tag != CurrentVersion,
		ReleaseNotes:   rel.Body,
		DownloadURL:    urls[targetArchive()],
		PublishedAt:    rel.PublishedAt.Format("2006-01-02"),
	}, urls["checksums.txt"], nil
}

func downloadBytes(ctx context.Context, rawURL string, limit int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "val2harness-updater")
	if req.URL.Host == "api.github.com" {
		req.Header.Set("Accept", "application/vnd.github.v3+json")
	}
	resp, err := updateHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("update download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update download returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > limit {
		return nil, fmt.Errorf("update download exceeds %d bytes", limit)
	}
	data, err := readBounded(resp.Body, limit)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return data, nil
}

func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("update data exceeds %d bytes", limit)
	}
	return data, nil
}

func checksumFor(data []byte, name string) ([]byte, error) {
	var result []byte
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 || len(fields[0]) != 64 {
			return nil, fmt.Errorf("malformed release checksums")
		}
		sum, err := hex.DecodeString(fields[0])
		if err != nil {
			return nil, fmt.Errorf("malformed release checksum: %w", err)
		}
		if strings.TrimPrefix(fields[1], "*") == name {
			if result != nil {
				return nil, fmt.Errorf("duplicate release checksum for %s", name)
			}
			result = sum
		}
	}
	if result == nil {
		return nil, fmt.Errorf("release checksum missing for %s", name)
	}
	return result, nil
}

func SelfUpdate(ctx context.Context) (string, error) {
	info, checksumURL, err := checkUpdate(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to check for update: %w", err)
	}
	if !info.HasUpdate {
		return info.CurrentVersion, nil
	}
	if info.DownloadURL == "" {
		return "", fmt.Errorf("no matching binary found for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	if checksumURL == "" {
		return "", fmt.Errorf("release has no checksums.txt; executable was not changed")
	}
	checksums, err := downloadBytes(ctx, checksumURL, maxChecksumBytes)
	if err != nil {
		return "", err
	}
	expected, err := checksumFor(checksums, targetArchive())
	if err != nil {
		return "", err
	}
	archive, err := downloadBytes(ctx, info.DownloadURL, maxArchiveBytes)
	if err != nil {
		return "", err
	}
	actual := sha256.Sum256(archive)
	if !bytes.Equal(actual[:], expected) {
		return "", fmt.Errorf("release archive SHA256 mismatch; executable was not changed")
	}
	var binary []byte
	if runtime.GOOS == "windows" {
		binary, err = extractFromZip(archive)
	} else {
		binary, err = extractFromTarGz(archive)
	}
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	execPath, err := executablePath()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return "", err
	}
	original, err := os.Lstat(execPath)
	if err != nil {
		return "", err
	}
	if !original.Mode().IsRegular() {
		return "", fmt.Errorf("executable is not a regular file")
	}
	staged, err := stageExecutable(execPath, binary, original.Mode().Perm())
	if err != nil {
		return "", err
	}
	defer os.Remove(staged)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := replaceExecutable(execPath, staged); err != nil {
		return "", err
	}
	return info.LatestVersion, nil
}

func stageExecutable(execPath string, binary []byte, mode os.FileMode) (string, error) {
	file, err := os.CreateTemp(filepath.Dir(execPath), ".val2harness-update-*")
	if err != nil {
		return "", err
	}
	name := file.Name()
	_, writeErr := file.Write(binary)
	if writeErr == nil {
		writeErr = file.Chmod(mode)
	}
	if writeErr == nil {
		writeErr = file.Sync()
	}
	err = errors.Join(writeErr, file.Close())
	if err != nil {
		_ = os.Remove(name)
		return "", fmt.Errorf("stage update: %w", err)
	}
	return name, nil
}

func replaceExecutable(execPath, staged string) error {
	if runtime.GOOS != "windows" {
		if err := renameFile(staged, execPath); err != nil {
			return fmt.Errorf("replace executable: %w", err)
		}
		return nil
	}
	backup, err := os.CreateTemp(filepath.Dir(execPath), ".val2harness-previous-*")
	if err != nil {
		return err
	}
	backupPath := backup.Name()
	if err := backup.Close(); err != nil {
		_ = os.Remove(backupPath)
		return err
	}
	if err := renameFile(execPath, backupPath); err != nil {
		_ = os.Remove(backupPath)
		return fmt.Errorf("retain previous executable: %w", err)
	}
	if err := renameFile(staged, execPath); err != nil {
		if rollbackErr := renameFile(backupPath, execPath); rollbackErr != nil {
			return fmt.Errorf("replace executable: %v; rollback failed: %v; previous executable retained at %s", err, rollbackErr, backupPath)
		}
		return fmt.Errorf("replace executable failed; previous executable restored: %w", err)
	}
	// Windows may keep the previous running image open. A retained, unique
	// previous copy is safe to remove after that process exits.
	_ = os.Remove(backupPath)
	return nil
}

func extractFromTarGz(data []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(io.LimitReader(gz, maxExpandedBytes+1))
	for entries := 0; entries < maxArchiveEntries; entries++ {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if path.Base(hdr.Name) != "val2harness" {
			continue
		}
		if hdr.Typeflag != tar.TypeReg || hdr.Size <= 0 || hdr.Size > maxBinaryBytes {
			return nil, fmt.Errorf("invalid executable in release archive")
		}
		return readBounded(tr, maxBinaryBytes)
	}
	return nil, fmt.Errorf("val2harness binary not found within archive limits")
}

func extractFromZip(data []byte) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	if len(zr.File) > maxArchiveEntries {
		return nil, fmt.Errorf("too many entries in release archive")
	}
	for _, file := range zr.File {
		if path.Base(file.Name) != "val2harness.exe" {
			continue
		}
		if !file.Mode().IsRegular() || file.UncompressedSize64 == 0 || file.UncompressedSize64 > maxBinaryBytes {
			return nil, fmt.Errorf("invalid executable in release archive")
		}
		rc, err := file.Open()
		if err != nil {
			return nil, err
		}
		binary, readErr := readBounded(rc, maxBinaryBytes)
		return binary, errors.Join(readErr, rc.Close())
	}
	return nil, fmt.Errorf("val2harness.exe not found in zip")
}

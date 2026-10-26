package version

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const CurrentVersion = "v1.0.0"
const RepoName = "mncuchiinhuttt/val2harness"

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

func CheckUpdate(ctx context.Context) (*UpdateInfo, error) {
	reqURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", RepoName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "val2harness-updater")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api status: %d", resp.StatusCode)
	}

	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}

	latest := strings.TrimSpace(rel.TagName)
	hasUpdate := latest != "" && latest != CurrentVersion

	ext := ".tar.gz"
	if runtime.GOOS == "windows" {
		ext = ".zip"
	}
	targetName := fmt.Sprintf("val2harness-%s-%s%s", runtime.GOOS, runtime.GOARCH, ext)

	downloadURL := ""
	for _, a := range rel.Assets {
		if a.Name == targetName {
			downloadURL = a.BrowserDownloadURL
			break
		}
	}

	return &UpdateInfo{
		CurrentVersion: CurrentVersion,
		LatestVersion:  latest,
		HasUpdate:      hasUpdate,
		ReleaseNotes:   rel.Body,
		DownloadURL:    downloadURL,
		PublishedAt:    rel.PublishedAt.Format("2006-01-02"),
	}, nil
}

func SelfUpdate(ctx context.Context) (string, error) {
	info, err := CheckUpdate(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to check for update: %w", err)
	}
	if !info.HasUpdate {
		return info.CurrentVersion, nil
	}
	if info.DownloadURL == "" {
		return "", fmt.Errorf("no matching binary found for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}
	if execPath, err = filepath.EvalSymlinks(execPath); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, info.DownloadURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	pkgData, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var binBytes []byte
	if runtime.GOOS == "windows" {
		binBytes, err = extractFromZip(pkgData)
	} else {
		binBytes, err = extractFromTarGz(pkgData)
	}
	if err != nil {
		return "", err
	}

	tempPath := execPath + ".new"
	if err := os.WriteFile(tempPath, binBytes, 0755); err != nil {
		return "", fmt.Errorf("failed to write %s: %w", tempPath, err)
	}

	if runtime.GOOS == "windows" {
		_ = os.Rename(execPath, execPath+".old")
	}

	if err := os.Rename(tempPath, execPath); err != nil {
		_ = os.Remove(tempPath)
		return "", fmt.Errorf("failed to replace binary: %w", err)
	}

	return info.LatestVersion, nil
}

func extractFromTarGz(data []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Name == "val2harness" || filepath.Base(hdr.Name) == "val2harness" {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("val2harness binary not found in archive")
}

func extractFromZip(data []byte) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if f.Name == "val2harness.exe" || filepath.Base(f.Name) == "val2harness.exe" {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("val2harness.exe not found in zip")
}

package sync

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"val2harness/internal/config"
	"val2harness/internal/configfile"
	"val2harness/internal/valclient"
)

type Result struct {
	Harness string `json:"harness"`
	Count   int    `json:"count"`
	Path    string `json:"path"`
	Error   error  `json:"error,omitempty"`
}

func SyncAll(ctx context.Context, client *valclient.Client, proxyBaseURL, proxyAPIKey string) []Result {
	var results []Result

	// 1. OMP
	cOMP, pOMP, errOMP := SyncOMP(ctx, client, proxyBaseURL, proxyAPIKey)
	results = append(results, Result{Harness: "OMP", Count: cOMP, Path: pOMP, Error: errOMP})

	// 2. Pi Harness
	cPi, pPi, errPi := SyncPi(ctx, client, proxyBaseURL, proxyAPIKey)
	results = append(results, Result{Harness: "Pi", Count: cPi, Path: pPi, Error: errPi})

	// 3. OpenCode
	cOC, pOC, errOC := SyncOpenCode(ctx, client, proxyBaseURL, proxyAPIKey)
	results = append(results, Result{Harness: "OpenCode", Count: cOC, Path: pOC, Error: errOC})

	// 4. Continue
	cCont, pCont, errCont := SyncContinue(ctx, client, proxyBaseURL, proxyAPIKey)
	results = append(results, Result{Harness: "Continue", Count: cCont, Path: pCont, Error: errCont})

	return results
}

func getHarnessPaths(harness string) (target, backup string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	switch strings.ToLower(harness) {
	case "omp":
		target = filepath.Join(home, ".omp", "models.yml")
	case "pi":
		target = filepath.Join(home, ".pi", "models.json")
	case "opencode":
		target, err = openCodePath(home)
		if err != nil {
			return "", "", err
		}
	case "continue":
		target = filepath.Join(home, ".continue", "config.yaml")
	default:
		return "", "", fmt.Errorf("unknown harness: %s", harness)
	}
	backup = target + ".bak"
	return target, backup, nil
}

func HasBackup(harness string) bool {
	_, backup, err := getHarnessPaths(harness)
	if err != nil {
		return false
	}
	_, err = os.Stat(backup)
	return err == nil
}

func RestoreHarness(harness string) (string, error) {
	harness = strings.ToLower(harness)
	target, _, err := getHarnessPaths(harness)
	if err != nil {
		return "", err
	}
	err = configfile.Restore(target, func(data []byte) error {
		if harness == "pi" || harness == "opencode" {
			_, err := configfile.JSONObject(data)
			return err
		}
		_, err := configfile.YAMLDocument(data)
		return err
	})
	if err != nil {
		return "", err
	}
	return target, nil
}

func openCodePath(home string) (string, error) {
	paths := []string{filepath.Join(home, ".config", "opencode", "opencode.json"), filepath.Join(home, ".opencode", "opencode.json")}
	var chosen string
	for _, p := range paths {
		if _, err := os.Lstat(p); err == nil {
			if chosen != "" {
				return "", fmt.Errorf("both OpenCode settings locations exist; select one before syncing")
			}
			chosen = p
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	if chosen != "" {
		return chosen, nil
	}
	for _, p := range paths {
		if _, err := os.Lstat(p + ".bak"); err == nil {
			if chosen != "" {
				return "", fmt.Errorf("multiple OpenCode backups found; restore one manually")
			}
			chosen = p
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	if chosen != "" {
		return chosen, nil
	}
	return paths[0], nil
}

func GenerateEnvSnippet(addr, key string) string {
	base := config.LocalBaseURL(addr)
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
	return "# Private local connection settings; share only with tools you trust.\n" +
		"export OPENAI_BASE_URL=" + quote(base+"/v1") + "\n" +
		"export OPENAI_API_KEY=" + quote(key) + "\n" +
		"export ANTHROPIC_BASE_URL=" + quote(base) + "\n" +
		"export ANTHROPIC_API_KEY=" + quote(key) + "\n"
}

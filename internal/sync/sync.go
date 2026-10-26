package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
		pAgent := filepath.Join(home, ".omp", "agent", "models.yml")
		pLegacy := filepath.Join(home, ".omp", "models.yml")
		if _, err := os.Stat(filepath.Dir(pAgent)); err == nil {
			target = pAgent
		} else {
			target = pLegacy
		}
	case "pi":
		target = filepath.Join(home, ".pi", "models.json")
	case "opencode":
		p1 := filepath.Join(home, ".config", "opencode", "opencode.json")
		p2 := filepath.Join(home, ".opencode", "opencode.json")
		if _, err := os.Stat(p1 + ".bak"); err == nil {
			target = p1
		} else if _, err := os.Stat(p2 + ".bak"); err == nil {
			target = p2
		} else {
			target = p1
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
	target, backup, err := getHarnessPaths(harness)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(backup)
	if err != nil {
		return "", fmt.Errorf("backup file not found: %s", backup)
	}
	if err := os.WriteFile(target, data, 0644); err != nil {
		return "", fmt.Errorf("failed to restore %s: %w", target, err)
	}
	if strings.ToLower(harness) == "omp" {
		h, _ := os.UserHomeDir()
		_ = os.WriteFile(filepath.Join(h, ".omp", "models.yml"), data, 0644)
	}
	return target, nil
}

func GenerateEnvSnippet(addr string) string {
	baseURL := strings.TrimRight(addr, "/")
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "http://" + baseURL
	}

	var sb strings.Builder
	sb.WriteString("# Environment variables for AI Agent Harnesses (Cursor, Claude Code, Aider, zcode)\n")
	sb.WriteString(fmt.Sprintf("export OPENAI_BASE_URL=\"%s/v1\"\n", baseURL))
	sb.WriteString("export OPENAI_API_KEY=\"rmit-val-key\"\n")
	sb.WriteString(fmt.Sprintf("export ANTHROPIC_BASE_URL=\"%s\"\n", baseURL))
	sb.WriteString("export ANTHROPIC_API_KEY=\"rmit-val-key\"\n")
	return sb.String()
}

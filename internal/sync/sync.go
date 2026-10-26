package sync

import (
	"context"
	"fmt"
	"strings"

	"val2harness/internal/valclient"
)

type Result struct {
	Harness string
	Count   int
	Path    string
	Error   error
}

func SyncAll(ctx context.Context, client *valclient.Client, proxyBaseURL, proxyAPIKey string) []Result {
	var results []Result

	// 1. OMP
	cOMP, pOMP, errOMP := SyncOMP(ctx, client, proxyBaseURL, proxyAPIKey)
	results = append(results, Result{Harness: "OMP (~/.omp/models.yml)", Count: cOMP, Path: pOMP, Error: errOMP})

	// 2. Pi Harness
	cPi, pPi, errPi := SyncPi(ctx, client, proxyBaseURL, proxyAPIKey)
	results = append(results, Result{Harness: "Pi Harness (~/.pi/models.json)", Count: cPi, Path: pPi, Error: errPi})

	// 3. OpenCode
	cOC, pOC, errOC := SyncOpenCode(ctx, client, proxyBaseURL, proxyAPIKey)
	results = append(results, Result{Harness: "OpenCode (opencode.json)", Count: cOC, Path: pOC, Error: errOC})

	// 4. Continue
	cCont, pCont, errCont := SyncContinue(ctx, client, proxyBaseURL, proxyAPIKey)
	results = append(results, Result{Harness: "Continue (~/.continue/config.yaml)", Count: cCont, Path: pCont, Error: errCont})

	return results
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

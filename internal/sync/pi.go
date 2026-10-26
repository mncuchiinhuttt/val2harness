package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"val2harness/internal/safeio"
	"val2harness/internal/valclient"
)

type piCost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
}

type piModel struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ContextWindow int    `json:"contextWindow"`
	MaxTokens     int    `json:"maxTokens"`
	Cost          piCost `json:"cost"`
}

type piProvider struct {
	BaseURL string    `json:"baseUrl"`
	APIKey  string    `json:"apiKey"`
	API     string    `json:"api"`
	Models  []piModel `json:"models"`
}

func SyncPi(ctx context.Context, client *valclient.Client, proxyBaseURL, proxyAPIKey string) (int, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return 0, "", err
	}
	piPath := filepath.Join(home, ".pi", "models.json")

	valModels, err := client.ListModels(ctx)
	if err != nil {
		return 0, "", fmt.Errorf("failed to fetch models from Val: %w", err)
	}

	rawMap := make(map[string]any)
	data, found, err := safeio.ReadExisting(piPath)
	if err != nil {
		return 0, "", err
	}
	if found && !safeio.IsBlank(data) {
		if err := json.Unmarshal(data, &rawMap); err != nil {
			return 0, "", safeio.ParseGuard(piPath, err)
		}
		if err := safeio.WritePristineBackup(piPath, data); err != nil {
			return 0, "", err
		}
	}

	providersMap, ok := rawMap["providers"].(map[string]any)
	if !ok {
		providersMap = make(map[string]any)
		rawMap["providers"] = providersMap
	}

	apiKey := proxyAPIKey
	if apiKey == "" {
		apiKey = "rmit-val-key"
	}
	baseURL := strings.TrimRight(proxyBaseURL, "/") + "/v1"

	var models []piModel
	for _, m := range valModels {
		maxTok := 128000
		if strings.Contains(m.ID, "haiku") || strings.Contains(m.ID, "mini") {
			maxTok = 65536
		}
		models = append(models, piModel{
			ID:            m.ID,
			Name:          m.Name,
			ContextWindow: 1000000,
			MaxTokens:     maxTok,
		})
	}

	providersMap["rmit-val"] = piProvider{
		BaseURL: baseURL,
		APIKey:  apiKey,
		API:     "openai-completions",
		Models:  models,
	}

	outBytes, err := json.MarshalIndent(rawMap, "", "  ")
	if err != nil {
		return 0, "", err
	}

	if err := safeio.WriteSecure(piPath, outBytes); err != nil {
		return 0, "", err
	}

	return len(models), piPath, nil
}

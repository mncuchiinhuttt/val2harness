package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"val2harness/internal/valclient"
)

type piModel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
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
	if data, err := os.ReadFile(piPath); err == nil {
		_ = os.WriteFile(piPath+".bak", data, 0644)
		_ = json.Unmarshal(data, &rawMap)
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
		models = append(models, piModel{
			ID:   m.ID,
			Name: m.Name,
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

	if err := os.MkdirAll(filepath.Dir(piPath), 0755); err != nil {
		return 0, "", err
	}
	if err := os.WriteFile(piPath, outBytes, 0644); err != nil {
		return 0, "", err
	}

	return len(models), piPath, nil
}

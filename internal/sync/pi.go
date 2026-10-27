package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"val2harness/internal/configfile"
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

	err = configfile.Update(piPath, func(data []byte) ([]byte, error) {
		rawMap, err := configfile.JSONObject(data)
		if err != nil {
			return nil, err
		}
		providersMap, err := configfile.ObjectField(rawMap, "providers")
		if err != nil {
			return nil, err
		}
		providersMap["rmit-val"] = piProvider{
			BaseURL: baseURL,
			APIKey:  apiKey,
			API:     "openai-completions",
			Models:  models,
		}

		return json.MarshalIndent(rawMap, "", "  ")
	})
	if err != nil {
		return 0, "", err
	}

	return len(models), piPath, nil
}

package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
	"val2harness/internal/valclient"
)

type continueModel struct {
	Name     string   `yaml:"name"`
	Provider string   `yaml:"provider"`
	Model    string   `yaml:"model"`
	APIKey   string   `yaml:"apiKey"`
	APIBase  string   `yaml:"apiBase"`
	Roles    []string `yaml:"roles,omitempty"`
}

func SyncContinue(ctx context.Context, client *valclient.Client, proxyBaseURL, proxyAPIKey string) (int, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return 0, "", err
	}
	continuePath := filepath.Join(home, ".continue", "config.yaml")

	valModels, err := client.ListModels(ctx)
	if err != nil {
		return 0, "", fmt.Errorf("failed to fetch models from Val: %w", err)
	}

	rawMap := make(map[string]any)
	if data, err := os.ReadFile(continuePath); err == nil {
		_ = os.WriteFile(continuePath+".bak", data, 0644)
		_ = yaml.Unmarshal(data, &rawMap)
	}

	apiKey := proxyAPIKey
	if apiKey == "" {
		apiKey = "rmit-val-key"
	}
	baseURL := strings.TrimRight(proxyBaseURL, "/") + "/v1"

	var existingModels []any
	if em, ok := rawMap["models"].([]any); ok {
		for _, m := range em {
			if mm, ok := m.(map[string]any); ok {
				if ab, _ := mm["apiBase"].(string); !strings.Contains(ab, "8765") && !strings.Contains(ab, "rmit-val") {
					existingModels = append(existingModels, mm)
				}
			}
		}
	}

	count := 0
	for _, m := range valModels {
		// Prioritize main flagship models for Continue UI
		isFlagship := strings.Contains(m.ID, "opus") ||
			strings.Contains(m.ID, "sonnet") ||
			strings.Contains(m.ID, "haiku") ||
			strings.Contains(m.ID, "gpt-5") ||
			strings.Contains(m.ID, "gpt-6") ||
			strings.Contains(m.ID, "llama-4")
		if !isFlagship {
			continue
		}

		cModel := continueModel{
			Name:     fmt.Sprintf("RMIT %s", m.Name),
			Provider: "openai",
			Model:    m.ID,
			APIKey:   apiKey,
			APIBase:  baseURL,
			Roles:    []string{"chat", "edit", "apply"},
		}
		existingModels = append(existingModels, cModel)
		count++
	}

	rawMap["models"] = existingModels

	outBytes, err := yaml.Marshal(rawMap)
	if err != nil {
		return 0, "", err
	}

	if err := os.MkdirAll(filepath.Dir(continuePath), 0755); err != nil {
		return 0, "", err
	}
	if err := os.WriteFile(continuePath, outBytes, 0644); err != nil {
		return 0, "", err
	}

	return count, continuePath, nil
}

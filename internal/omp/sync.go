package omp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
	"val2harness/internal/configfile"
	"val2harness/internal/valclient"
)

type OMPModel struct {
	ID            string `yaml:"id"`
	Name          string `yaml:"name"`
	ContextWindow int    `yaml:"contextWindow,omitempty"`
	MaxTokens     int    `yaml:"maxTokens,omitempty"`
}

type OMPProvider struct {
	BaseURL string     `yaml:"baseUrl"`
	APIKey  string     `yaml:"apiKey"`
	API     string     `yaml:"api"`
	Models  []OMPModel `yaml:"models"`
}

func GetOMPModelsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	paths := []string{
		filepath.Join(home, ".omp", "agent", "models.yml"),
		filepath.Join(home, ".omp", "models.yml"),
	}
	// Select one existing document, or its backup when only a restore point
	// remains. Never mirror one location over a different user's document.
	for _, suffix := range []string{"", ".bak"} {
		var chosen string
		for _, path := range paths {
			if _, err := os.Lstat(path + suffix); err == nil {
				if chosen != "" {
					return "", fmt.Errorf("multiple OMP settings locations exist; select one before syncing or restoring")
				}
				chosen = path
			} else if !errors.Is(err, os.ErrNotExist) {
				return "", err
			}
		}
		if chosen != "" {
			return chosen, nil
		}
	}
	if info, err := os.Stat(filepath.Dir(paths[0])); err == nil && info.IsDir() {
		return paths[0], nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return paths[1], nil
}

func SyncModels(ctx context.Context, client *valclient.Client, proxyBaseURL, proxyAPIKey string) (int, string, error) {
	models, err := client.ListModels(ctx)
	if err != nil {
		return 0, "", fmt.Errorf("failed to list models from Val: %w", err)
	}
	path, err := GetOMPModelsPath()
	if err != nil {
		return 0, "", err
	}
	if proxyAPIKey == "" {
		proxyAPIKey = "rmit-val-key"
	}
	provider := OMPProvider{BaseURL: strings.TrimRight(proxyBaseURL, "/") + "/v1", APIKey: proxyAPIKey, API: "openai-completions"}
	for _, m := range models {
		max := 65536
		if strings.Contains(m.ID, "opus") || strings.Contains(m.ID, "sonnet") {
			max = 128000
		}
		provider.Models = append(provider.Models, OMPModel{ID: m.ID, Name: m.Name, ContextWindow: 200000, MaxTokens: max})
	}
	err = configfile.Update(path, func(data []byte) ([]byte, error) {
		root, err := configfile.YAMLDocument(data)
		if err != nil {
			return nil, err
		}
		providers, err := configfile.YAMLField(root.Content[0], "providers", yaml.MappingNode)
		if err != nil {
			return nil, err
		}
		node := new(yaml.Node)
		if err := node.Encode(provider); err != nil {
			return nil, err
		}
		configfile.SetYAMLField(providers, "rmit-val", node)
		return configfile.MarshalYAML(root)
	})
	if err != nil {
		return 0, "", err
	}
	return len(models), path, nil
}

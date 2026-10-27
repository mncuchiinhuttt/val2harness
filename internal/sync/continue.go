package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
	"val2harness/internal/configfile"
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
	path := filepath.Join(home, ".continue", "config.yaml")
	models, err := client.ListModels(ctx)
	if err != nil {
		return 0, "", fmt.Errorf("failed to fetch models from Val: %w", err)
	}
	if proxyAPIKey == "" {
		proxyAPIKey = "rmit-val-key"
	}
	base := strings.TrimRight(proxyBaseURL, "/") + "/v1"
	count := 0
	err = configfile.Update(path, func(data []byte) ([]byte, error) {
		root, err := configfile.YAMLDocument(data)
		if err != nil {
			return nil, err
		}
		sequence, err := configfile.YAMLField(root.Content[0], "models", yaml.SequenceNode)
		if err != nil {
			return nil, err
		}
		kept := make([]*yaml.Node, 0, len(sequence.Content))
		for _, node := range sequence.Content {
			if node.Kind != yaml.MappingNode {
				return nil, fmt.Errorf("each model must be a mapping")
			}
			var m map[string]any
			if err := node.Decode(&m); err != nil {
				return nil, err
			}
			name, _ := m["name"].(string)
			apiBase, _ := m["apiBase"].(string)
			if strings.HasPrefix(name, "RMIT ") && apiBase == base {
				continue
			}
			kept = append(kept, node)
		}
		for _, m := range models {
			if !(strings.Contains(m.ID, "opus") || strings.Contains(m.ID, "sonnet") || strings.Contains(m.ID, "haiku") || strings.Contains(m.ID, "gpt-5") || strings.Contains(m.ID, "gpt-6") || strings.Contains(m.ID, "llama-4")) {
				continue
			}
			node := new(yaml.Node)
			err := node.Encode(continueModel{Name: "RMIT " + m.Name, Provider: "openai", Model: m.ID, APIKey: proxyAPIKey, APIBase: base, Roles: []string{"chat", "edit", "apply"}})
			if err != nil {
				return nil, err
			}
			kept = append(kept, node)
			count++
		}
		sequence.Content = kept
		return configfile.MarshalYAML(root)
	})
	if err != nil {
		return 0, "", err
	}
	return count, path, nil
}

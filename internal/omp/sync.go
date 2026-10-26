package omp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
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
	agentPath := filepath.Join(home, ".omp", "agent", "models.yml")
	if _, err := os.Stat(filepath.Dir(agentPath)); err == nil {
		return agentPath, nil
	}
	return filepath.Join(home, ".omp", "models.yml"), nil
}

func SyncModels(ctx context.Context, client *valclient.Client, proxyBaseURL, proxyAPIKey string) (int, string, error) {
	valModels, err := client.ListModels(ctx)
	if err != nil {
		return 0, "", fmt.Errorf("failed to list models from RMIT Val: %w", err)
	}

	ompPath, err := GetOMPModelsPath()
	if err != nil {
		return 0, "", err
	}

	var root yaml.Node
	fileData, err := os.ReadFile(ompPath)
	if err == nil {
		// Create backup
		_ = os.WriteFile(ompPath+".bak", fileData, 0644)
		if err := yaml.Unmarshal(fileData, &root); err != nil {
			return 0, "", fmt.Errorf("failed to parse existing models.yml: %w", err)
		}
	} else if os.IsNotExist(err) {
		// New document
		root = yaml.Node{
			Kind: yaml.DocumentNode,
			Content: []*yaml.Node{
				{
					Kind: yaml.MappingNode,
				},
			},
		}
	} else {
		return 0, "", err
	}

	if len(root.Content) == 0 {
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.MappingNode})
	}
	docMap := root.Content[0]

	// Find or create "providers" map
	var providersNode *yaml.Node
	for i := 0; i < len(docMap.Content); i += 2 {
		if docMap.Content[i].Value == "providers" {
			providersNode = docMap.Content[i+1]
			break
		}
	}
	if providersNode == nil {
		providersNode = &yaml.Node{Kind: yaml.MappingNode}
		docMap.Content = append(docMap.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "providers"},
			providersNode,
		)
	}

	// Prepare rmit-val provider
	apiKey := proxyAPIKey
	if apiKey == "" {
		apiKey = "rmit-val-key"
	}
	baseURL := strings.TrimRight(proxyBaseURL, "/") + "/v1"

	var ompModels []OMPModel
	for _, m := range valModels {
		ctxWindow := 1000000 // 1M context
		maxTok := 128000
		if strings.Contains(m.ID, "haiku") || strings.Contains(m.ID, "mini") {
			maxTok = 65536
		}
		ompModels = append(ompModels, OMPModel{
			ID:            m.ID,
			Name:          m.Name,
			ContextWindow: ctxWindow,
			MaxTokens:     maxTok,
		})
	}

	newProvider := OMPProvider{
		BaseURL: baseURL,
		APIKey:  apiKey,
		API:     "openai-completions",
		Models:  ompModels,
	}

	var providerNode yaml.Node
	pBytes, err := yaml.Marshal(newProvider)
	if err != nil {
		return 0, "", err
	}
	if err := yaml.Unmarshal(pBytes, &providerNode); err != nil {
		return 0, "", err
	}

	// Update or insert "rmit-val" key
	found := false
	for i := 0; i < len(providersNode.Content); i += 2 {
		if providersNode.Content[i].Value == "rmit-val" {
			providersNode.Content[i+1] = providerNode.Content[0]
			found = true
			break
		}
	}
	if !found {
		providersNode.Content = append(providersNode.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "rmit-val"},
			providerNode.Content[0],
		)
	}

	// Write back
	outData, err := yaml.Marshal(&root)
	if err != nil {
		return 0, "", fmt.Errorf("failed to serialize updated models.yml: %w", err)
	}

	home, _ := os.UserHomeDir()
	paths := []string{
		filepath.Join(home, ".omp", "agent", "models.yml"),
		filepath.Join(home, ".omp", "models.yml"),
	}
	for _, p := range paths {
		if err := os.MkdirAll(filepath.Dir(p), 0755); err == nil {
			_ = os.WriteFile(p, outData, 0644)
		}
	}

	return len(ompModels), ompPath, nil
}

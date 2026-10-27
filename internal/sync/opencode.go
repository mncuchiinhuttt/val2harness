package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"val2harness/internal/configfile"
	"val2harness/internal/valclient"
)

func SyncOpenCode(ctx context.Context, client *valclient.Client, proxyBaseURL, proxyAPIKey string) (int, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return 0, "", err
	}

	targetPath, err := openCodePath(home)
	if err != nil {
		return 0, "", err
	}

	valModels, err := client.ListModels(ctx)
	if err != nil {
		return 0, "", fmt.Errorf("failed to fetch models from Val: %w", err)
	}

	apiKey := proxyAPIKey
	if apiKey == "" {
		apiKey = "rmit-val-key"
	}
	baseURL := strings.TrimRight(proxyBaseURL, "/") + "/v1"

	modelsObj := make(map[string]any)
	for _, m := range valModels {
		modelsObj[m.ID] = map[string]any{
			"name": m.Name,
			"modalities": map[string]any{
				"input":  []string{"text", "image"},
				"output": []string{"text"},
			},
		}
	}

	err = configfile.Update(targetPath, func(data []byte) ([]byte, error) {
		rawMap, err := configfile.JSONObject(data)
		if err != nil {
			return nil, err
		}
		if _, ok := rawMap["$schema"]; !ok {
			rawMap["$schema"] = "https://opencode.ai/config.json"
		}
		providerMap, err := configfile.ObjectField(rawMap, "provider")
		if err != nil {
			return nil, err
		}
		providerMap["rmit-val"] = map[string]any{
			"npm": "@ai-sdk/openai-compatible",
			"options": map[string]any{
				"baseURL": baseURL,
				"apiKey":  apiKey,
			},
			"models": modelsObj,
		}

		return json.MarshalIndent(rawMap, "", "  ")
	})
	if err != nil {
		return 0, "", err
	}

	return len(modelsObj), targetPath, nil
}

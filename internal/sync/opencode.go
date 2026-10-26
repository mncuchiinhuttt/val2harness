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

func SyncOpenCode(ctx context.Context, client *valclient.Client, proxyBaseURL, proxyAPIKey string) (int, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return 0, "", err
	}

	paths := []string{
		filepath.Join(home, ".config", "opencode", "opencode.json"),
		filepath.Join(home, ".opencode", "opencode.json"),
	}

	var targetPath string
	for _, p := range paths {
		if _, err := os.Stat(filepath.Dir(p)); err == nil {
			targetPath = p
			break
		}
	}
	if targetPath == "" {
		targetPath = paths[0]
	}

	valModels, err := client.ListModels(ctx)
	if err != nil {
		return 0, "", fmt.Errorf("failed to fetch models from Val: %w", err)
	}

	rawMap := make(map[string]any)
	data, found, err := safeio.ReadExisting(targetPath)
	if err != nil {
		return 0, "", err
	}
	if found && !safeio.IsBlank(data) {
		if err := json.Unmarshal(data, &rawMap); err != nil {
			return 0, "", safeio.ParseGuard(targetPath, err)
		}
		if err := safeio.WritePristineBackup(targetPath, data); err != nil {
			return 0, "", err
		}
	}

	if _, ok := rawMap["$schema"]; !ok {
		rawMap["$schema"] = "https://opencode.ai/config.json"
	}

	providerMap, ok := rawMap["provider"].(map[string]any)
	if !ok {
		providerMap = make(map[string]any)
		rawMap["provider"] = providerMap
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

	providerMap["rmit-val"] = map[string]any{
		"npm": "@ai-sdk/openai-compatible",
		"options": map[string]any{
			"baseURL": baseURL,
			"apiKey":  apiKey,
		},
		"models": modelsObj,
	}

	outBytes, err := json.MarshalIndent(rawMap, "", "  ")
	if err != nil {
		return 0, "", err
	}

	if err := safeio.WriteSecure(targetPath, outBytes); err != nil {
		return 0, "", err
	}

	return len(modelsObj), targetPath, nil
}

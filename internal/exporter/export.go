package exporter

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"val2harness/internal/valclient"
)

var slugRe = regexp.MustCompile(`[^a-zA-Z0-9_\-]+`)

func sanitizeFilename(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, " ", "-")
	name = slugRe.ReplaceAllString(name, "")
	if len(name) > 60 {
		name = name[:60]
	}
	if name == "" {
		return "untitled"
	}
	return strings.ToLower(name)
}

func ExportAllChats(ctx context.Context, client *valclient.Client, outDir string) (int, string, error) {
	if outDir == "" {
		outDir = "exports"
	}
	if err := os.MkdirAll(outDir, 0700); err != nil {
		return 0, "", err
	}

	chats, err := client.ListChats(ctx)
	if err != nil {
		return 0, "", fmt.Errorf("failed to fetch chat list: %w", err)
	}

	if len(chats) == 0 {
		return 0, outDir, nil
	}

	count := 0
	for _, ch := range chats {
		chatDetails, err := client.GetChat(ctx, ch.ID)
		if err != nil {
			continue
		}

		slug := sanitizeFilename(ch.Title)
		idPrefix := ch.ID
		if len(idPrefix) > 8 {
			idPrefix = idPrefix[:8]
		}
		baseName := fmt.Sprintf("%s-%s", idPrefix, slug)

		// Save JSON
		jsonBytes, err := json.MarshalIndent(chatDetails, "", "  ")
		if err == nil {
			jsonPath := filepath.Join(outDir, baseName+".json")
			_ = os.WriteFile(jsonPath, jsonBytes, 0600)
		}

		// Save Markdown
		mdContent := renderChatMarkdown(ch.Title, chatDetails)
		mdPath := filepath.Join(outDir, baseName+".md")
		_ = os.WriteFile(mdPath, []byte(mdContent), 0600)
		count++
	}

	return count, outDir, nil
}

func renderChatMarkdown(title string, data map[string]any) string {
	var sb strings.Builder
	sb.WriteString("# " + title + "\n\n")

	chatMap, ok := data["chat"].(map[string]any)
	if !ok {
		return sb.String()
	}

	messagesMap, ok := chatMap["messages"].(map[string]any)
	if !ok {
		return sb.String()
	}

	historyMap, _ := chatMap["history"].(map[string]any)
	messagesList, _ := historyMap["messages"].(map[string]any)
	if messagesList == nil {
		messagesList = messagesMap
	}

	for _, v := range messagesList {
		msg, ok := v.(map[string]any)
		if !ok {
			continue
		}
		role, _ := msg["role"].(string)
		content, _ := msg["content"].(string)
		if role == "" && content == "" {
			continue
		}

		sb.WriteString(fmt.Sprintf("### %s\n\n%s\n\n---\n\n", strings.Title(role), content))
	}

	return sb.String()
}

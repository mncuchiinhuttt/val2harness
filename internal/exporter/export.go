package exporter

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"val2harness/internal/valclient"
)

var slugRe = regexp.MustCompile(`[^a-zA-Z0-9_\-]+`)

// NewRunDirectory creates a private, unique snapshot directory below parent.
// Existing parent directories and their permissions are left unchanged.
func NewRunDirectory(parent string) (string, error) {
	if parent == "" {
		parent = "exports"
	}
	if err := os.MkdirAll(parent, 0700); err != nil {
		return "", fmt.Errorf("prepare export parent: %w", err)
	}
	root, err := os.OpenRoot(parent)
	if err != nil {
		return "", err
	}
	defer root.Close()
	for attempt := 0; attempt < 10; attempt++ {
		name := "run-" + rand.Text()
		if err := createNewPrivateDirectory(root, name); err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return "", fmt.Errorf("create export snapshot: %w", err)
		}
		return filepath.Join(parent, name), nil
	}
	return "", fmt.Errorf("could not reserve a unique export snapshot directory")
}

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

// ExportAllChats counts a chat only when both its JSON and Markdown files have
// been written, synced, and closed. Other chats continue after a per-chat failure;
// a non-nil error may accompany completed files. Existing paths are never replaced.
func ExportAllChats(ctx context.Context, client *valclient.Client, outDir string) (int, string, error) {
	if outDir == "" {
		outDir = "exports"
	}
	if err := ctx.Err(); err != nil {
		return 0, outDir, err
	}
	if err := os.MkdirAll(outDir, 0700); err != nil {
		return 0, outDir, fmt.Errorf("prepare export directory: %w", err)
	}
	// Pin the selected directory. Every generated name is one component, and
	// exclusive creation refuses files, directories, hard links, and symlinks.
	root, err := os.OpenRoot(outDir)
	if err != nil {
		return 0, outDir, fmt.Errorf("open export directory: %w", err)
	}
	defer root.Close()

	chats, err := client.ListChats(ctx)
	if err != nil {
		return 0, outDir, fmt.Errorf("failed to fetch chat list: %w", err)
	}

	if len(chats) == 0 {
		return 0, outDir, nil
	}

	count := 0
	var failures []error
	for _, ch := range chats {
		if err := ctx.Err(); err != nil {
			failures = append(failures, err)
			break
		}
		chatDetails, err := client.GetChat(ctx, ch.ID)
		if err != nil {
			failures = append(failures, fmt.Errorf("chat %q: fetch details: %w", ch.ID, err))
			continue
		}

		slug := sanitizeFilename(ch.Title)
		// Hash the complete opaque ID: short IDs are valid, prefix collisions do
		// not lose conversations, and upstream text never becomes a path/device.
		baseName := fmt.Sprintf("chat-%x-%s", sha256.Sum256([]byte(ch.ID)), slug)

		jsonBytes, err := json.MarshalIndent(chatDetails, "", "  ")
		if err != nil {
			failures = append(failures, fmt.Errorf("chat %q: encode JSON: %w", ch.ID, err))
			continue
		}
		mdContent := renderChatMarkdown(ch.Title, chatDetails)
		var chatErrors []error
		for _, output := range []struct {
			suffix string
			data   []byte
		}{{".json", jsonBytes}, {".md", []byte(mdContent)}} {
			if err := ctx.Err(); err != nil {
				chatErrors = append(chatErrors, err)
				break
			}
			if err := writeNewPrivate(root, baseName+output.suffix, output.data); err != nil {
				chatErrors = append(chatErrors, fmt.Errorf("write %s: %w", output.suffix, err))
			}
		}
		if err := errors.Join(chatErrors...); err != nil {
			failures = append(failures, fmt.Errorf("chat %q incomplete (any saved files are retained): %w", ch.ID, err))
		} else {
			count++
		}
	}

	return count, outDir, errors.Join(failures...)
}

// configfile.Write intentionally replaces existing settings. Exports instead
// need O_EXCL, so cannot use that helper without risking unrelated user files.
func writeNewPrivate(root *os.Root, name string, data []byte) error {
	f, err := createNewPrivateFile(root, name)
	if err != nil {
		return err
	}
	n, writeErr := f.Write(data)
	if writeErr == nil && n != len(data) {
		writeErr = io.ErrShortWrite
	}
	var syncErr error
	if writeErr == nil {
		syncErr = f.Sync()
	}
	return errors.Join(writeErr, syncErr, f.Close())
}

func renderChatMarkdown(title string, data map[string]any) string {
	var sb strings.Builder
	sb.WriteString("# " + title + "\n\n")

	chatMap := data
	if wrapped, ok := data["chat"].(map[string]any); ok {
		chatMap = wrapped
	}
	var messages []map[string]any
	if list, ok := chatMap["messages"].([]any); ok && len(list) > 0 {
		for _, value := range list {
			if msg, ok := value.(map[string]any); ok {
				messages = append(messages, msg)
			} else {
				messages = append(messages, map[string]any{"content": value})
			}
		}
	} else {
		history, _ := chatMap["history"].(map[string]any)
		byID, _ := history["messages"].(map[string]any)
		if len(byID) == 0 {
			byID, _ = chatMap["messages"].(map[string]any)
		}
		messages = orderedHistory(byID)
	}

	if len(messages) == 0 {
		sb.WriteString("No supported messages were found. See the companion JSON for the original conversation data.\n\n")
	}
	for _, msg := range messages {
		role, _ := msg["role"].(string)
		if role == "" {
			role = "Message"
		}
		sb.WriteString("### " + role + "\n\n")
		switch content := msg["content"].(type) {
		case string:
			sb.WriteString(content + "\n\n")
		case []any:
			for _, block := range content {
				item, _ := block.(map[string]any)
				if item["type"] == "text" {
					if text, ok := item["text"].(string); ok {
						sb.WriteString(text + "\n\n")
						continue
					}
				}
				sb.WriteString("[Non-text or unsupported content block retained in the companion JSON.]\n\n")
			}
		default:
			sb.WriteString("[Content retained in the companion JSON.]\n\n")
		}
		sb.WriteString("---\n\n")
	}
	sb.WriteString("Original conversation data, including metadata, attachments, and alternate branches, is preserved in the companion JSON. History maps are rendered in deterministic parent-first order.\n")
	return sb.String()
}

// Parent links define order even when identifiers sort the other way around.
// Stable ID order breaks sibling ties; each node appears once even in cycles.
func orderedHistory(byID map[string]any) []map[string]any {
	keys := make([]string, 0, len(byID))
	for key := range byID {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	visited := make(map[string]bool, len(keys))
	var messages []map[string]any
	for _, key := range keys {
		var chain []string
		for key != "" && !visited[key] {
			value, exists := byID[key]
			if !exists {
				break
			}
			visited[key] = true
			chain = append(chain, key)
			msg, _ := value.(map[string]any)
			key, _ = msg["parentId"].(string)
		}
		for i := len(chain) - 1; i >= 0; i-- {
			if msg, ok := byID[chain[i]].(map[string]any); ok {
				messages = append(messages, msg)
			} else {
				messages = append(messages, map[string]any{"content": byID[chain[i]]})
			}
		}
	}
	return messages
}

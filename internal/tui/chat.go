package tui

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"val2harness/internal/config"
	"val2harness/internal/valclient"
)

func RunTerminalChat(cfg *config.Config, client *valclient.Client, initialModel string) {
	fmt.Println("\n=== Terminal Chat with RMIT Val ===")
	if initialModel == "" {
		initialModel = "anthropic-claude-opus-4-7"
	}

	model := initialModel
	fmt.Printf("Model: %s\n", model)
	fmt.Println("Type your message and press Enter. Type '/model <name>' to switch, or '/exit' to return.")
	fmt.Println("----------------------------------------------------------------")

	reader := bufio.NewReader(os.Stdin)
	var history []valclient.ChatMessage

	for {
		fmt.Printf("\nYou > ")
		text, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		if text == "/exit" || text == "/quit" {
			break
		}
		if strings.HasPrefix(text, "/model ") {
			model = strings.TrimSpace(strings.TrimPrefix(text, "/model "))
			fmt.Printf("[Switched to model: %s]\n", model)
			continue
		}

		history = append(history, valclient.ChatMessage{Role: "user", Content: text})

		reqBody := map[string]any{
			"model":    model,
			"messages": history,
			"stream":   true,
		}
		b, _ := json.Marshal(reqBody)

		fmt.Printf("\n%s > ", model)
		resp, err := client.ProxyChat(context.Background(), bytes.NewReader(b))
		if err != nil {
			fmt.Printf("\n[Error: %v]\n", err)
			continue
		}

		var fullReply strings.Builder
		respReader := bufio.NewReader(resp.Body)
		for {
			line, err := respReader.ReadString('\n')
			if len(line) > 0 {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "data: ") {
					payload := strings.TrimPrefix(trimmed, "data: ")
					if payload == "[DONE]" {
						break
					}
					var chunk struct {
						Choices []struct {
							Delta struct {
								Content string `json:"content"`
							} `json:"delta"`
						} `json:"choices"`
					}
					if json.Unmarshal([]byte(payload), &chunk) == nil {
						if len(chunk.Choices) > 0 {
							content := chunk.Choices[0].Delta.Content
							fmt.Print(content)
							fullReply.WriteString(content)
						}
					}
				}
			}
			if err != nil {
				if err != io.EOF {
					fmt.Printf("\n[Stream read error: %v]", err)
				}
				break
			}
		}
		_ = resp.Body.Close()
		fmt.Println()

		history = append(history, valclient.ChatMessage{Role: "assistant", Content: fullReply.String()})
	}
}

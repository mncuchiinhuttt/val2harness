package tui

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"val2harness/internal/config"
	"val2harness/internal/sse"
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

		turn := append(append([]valclient.ChatMessage(nil), history...), valclient.ChatMessage{Role: "user", Content: text})

		reqBody := map[string]any{
			"model":    model,
			"messages": turn,
			"stream":   true,
		}
		b, _ := json.Marshal(reqBody)

		fmt.Printf("\n%s > ", model)
		resp, err := client.ProxyChat(context.Background(), bytes.NewReader(b))
		if err != nil {
			fmt.Printf("\n[Error: %v]\n", err)
			continue
		}

		reply, err := readTerminalReply(resp, os.Stdout)
		_ = resp.Body.Close()
		fmt.Println()
		if err != nil {
			fmt.Printf("[Error: %v]\n", err)
			continue
		}
		history = append(turn, valclient.ChatMessage{Role: "assistant", Content: reply})
	}
}

// readTerminalReply returns only complete text turns; partial output is visible
// to the user but never becomes successful conversation history.
func readTerminalReply(resp *http.Response, output io.Writer) (string, error) {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("Val returned HTTP %d; check your account or retry", resp.StatusCode)
	}
	decoder := sse.NewDecoder(resp.Body, 0)
	var reply strings.Builder
	finished := false
	for {
		event, err := decoder.Next()
		if err != nil {
			return "", fmt.Errorf("Val stream ended before completion: %w", err)
		}
		if event.Name == "error" {
			return "", fmt.Errorf("Val reported a streaming error")
		}
		if event.Name != "" && event.Name != "message" {
			continue
		}
		if event.Data == "[DONE]" {
			if !finished {
				return "", fmt.Errorf("Val stream ended without a completion reason")
			}
			return reply.String(), nil
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content      *string         `json:"content"`
					ToolCalls    json.RawMessage `json:"tool_calls"`
					FunctionCall json.RawMessage `json:"function_call"`
					Reasoning    json.RawMessage `json:"reasoning_content"`
					Refusal      json.RawMessage `json:"refusal"`
				} `json:"delta"`
				Finish string `json:"finish_reason"`
			} `json:"choices"`
			Usage json.RawMessage `json:"usage"`
			Error json.RawMessage `json:"error"`
		}
		if json.Unmarshal([]byte(event.Data), &chunk) != nil || len(chunk.Error) > 0 && string(chunk.Error) != "null" {
			return "", fmt.Errorf("Val returned an invalid streaming completion")
		}
		if len(chunk.Choices) == 0 && len(chunk.Usage) > 0 {
			continue
		}
		if len(chunk.Choices) != 1 {
			return "", fmt.Errorf("Val returned no single text completion")
		}
		choice := chunk.Choices[0]
		if finished && (choice.Delta.Content != nil || choice.Finish != "") {
			return "", fmt.Errorf("Val continued a completed stream")
		}
		present := func(raw json.RawMessage) bool {
			if len(raw) == 0 {
				return false
			}
			var compact bytes.Buffer
			if err := json.Compact(&compact, raw); err != nil {
				return true
			}
			value := compact.String()
			return value != "null" && value != "[]" && value != `""`
		}
		if present(choice.Delta.ToolCalls) || present(choice.Delta.FunctionCall) || present(choice.Delta.Reasoning) || present(choice.Delta.Refusal) || choice.Finish != "" && choice.Finish != "stop" && choice.Finish != "length" {
			return "", fmt.Errorf("Val returned content this terminal text chat cannot display")
		}
		if choice.Finish != "" {
			finished = true
		}
		if choice.Delta.Content != nil {
			if _, err := io.WriteString(output, *choice.Delta.Content); err != nil {
				return "", err
			}
			reply.WriteString(*choice.Delta.Content)
		}
	}
}

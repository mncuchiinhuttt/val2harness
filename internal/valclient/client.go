package valclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"val2harness/internal/analytics"
)

type Client struct {
	baseURL    string
	token      string
	tokenMu    sync.RWMutex
	httpClient *http.Client
	tracker    *analytics.Tracker
}

func NewClient(baseURL, token string) *Client {
	baseURL = strings.TrimRight(baseURL, "/")
	return &Client{
		baseURL: baseURL,
		token:   token,
		tracker: analytics.NewTracker(),
		httpClient: &http.Client{
			Timeout: 0, // No global timeout for SSE streaming
		},
	}
}

func (c *Client) SetToken(token string) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	if c.token == token {
		return
	}
	c.token = token
	c.tracker.Reset()
}

func (c *Client) UsageTracker() *analytics.Tracker { return c.tracker }

func (c *Client) GetToken() string {
	c.tokenMu.RLock()
	defer c.tokenMu.RUnlock()
	return c.token
}

func (c *Client) ListModels(ctx context.Context) ([]ModelInfo, error) {
	reqURL := fmt.Sprintf("%s/api/models", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.GetToken())
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("upstream status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err == nil {
		if data, exists := envelope["data"]; exists && strings.TrimSpace(string(data)) != "null" {
			var models []ModelInfo
			if err := json.Unmarshal(data, &models); err == nil {
				return models, nil
			}
		}
	}

	var rawList []ModelInfo
	if err := json.Unmarshal(body, &rawList); err == nil && rawList != nil {
		return rawList, nil
	}

	return nil, fmt.Errorf("unexpected models response format")
}

func (c *Client) ProxyChat(ctx context.Context, body io.Reader) (*http.Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := prepareChatRequest(body)
	if err != nil {
		return nil, err
	}
	reqURL := fmt.Sprintf("%s/api/chat/completions", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	// Buffering must not enable automatic 307/308 POST replay to a new
	// destination. Raw OpenAI request bodies were not replayable before.
	req.GetBody = nil

	// Capture credentials and observation generation together. A token change
	// resets the tracker while holding this same lock, so old requests cannot
	// enter the new account's session history.
	c.tokenMu.RLock()
	token, generation := c.token, c.tracker.Begin()
	c.tokenMu.RUnlock()
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream, application/json")

	var metadata struct {
		Model string `json:"model"`
	}
	_ = json.Unmarshal(data, &metadata)
	observation := analytics.Observation{Model: analytics.ModelName(metadata.Model), Protocol: analytics.Protocol(ctx), Status: "unknown"}
	started := time.Now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctx.Err() == nil {
			observation.Status = "provider_error"
		}
		observation.LatencyMs = time.Since(started).Milliseconds()
		c.tracker.Observe(generation, observation)
		return resp, err
	}
	c.observeResponse(ctx, resp, generation, observation, started)
	return resp, nil
}

func (c *Client) ListChats(ctx context.Context) ([]ChatHistoryItem, error) {
	reqURL := fmt.Sprintf("%s/api/v1/chats/", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.GetToken())

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(b))
	}

	var items []ChatHistoryItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, err
	}
	return items, nil
}

func (c *Client) GetChat(ctx context.Context, id string) (map[string]any, error) {
	if id == "" || id == "." || id == ".." || strings.ContainsAny(id, "/\\") || strings.IndexFunc(id, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
		return nil, fmt.Errorf("invalid chat ID: expected one nonempty path segment")
	}
	reqURL := fmt.Sprintf("%s/api/v1/chats/%s", c.baseURL, url.PathEscape(id))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.GetToken())

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(b))
	}

	var chatData map[string]any
	decoder := json.NewDecoder(resp.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&chatData); err != nil {
		return nil, err
	}
	return chatData, nil
}

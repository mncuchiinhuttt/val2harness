package valclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type QuotaStatus struct {
	Status           string `json:"status"` // "active" or "exceeded"
	Message          string `json:"message"`
	MelbourneTime    string `json:"melbourne_time"`
	ResetAtMelbourne string `json:"reset_at_melbourne"`
	Countdown        string `json:"countdown"`
	SecondsUntil     int64  `json:"seconds_until"`
}

func (c *Client) CheckQuota(ctx context.Context) (*QuotaStatus, error) {
	// Probe with a minimal 1-token query to the lightest model
	probePayload := map[string]any{
		"model": "anthropic-claude-haiku-4-5",
		"messages": []map[string]any{
			{"role": "user", "content": "ping"},
		},
		"max_tokens": 1,
		"stream":     false,
	}
	b, _ := json.Marshal(probePayload)

	reqURL := fmt.Sprintf("%s/api/chat/completions", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.GetToken())
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	// Calculate Melbourne time & next 4AM reset
	loc, err := time.LoadLocation("Australia/Melbourne")
	if err != nil {
		// Fallback to UTC+11 (AEDT)
		loc = time.FixedZone("AEDT", 11*3600)
	}
	nowMel := time.Now().In(loc)
	resetToday := time.Date(nowMel.Year(), nowMel.Month(), nowMel.Day(), 4, 0, 0, 0, loc)
	target := resetToday
	if !nowMel.Before(target) {
		target = target.AddDate(0, 0, 1)
	}

	diff := target.Sub(nowMel)
	hours := int(diff.Hours())
	mins := int(diff.Minutes()) % 60
	countdownStr := fmt.Sprintf("%dh %02dm", hours, mins)

	// Inspect if budget exceeded
	var errCheck struct {
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	status := "active"
	msg := "Daily quota is active and ready."
	if json.Unmarshal(respBody, &errCheck) == nil && errCheck.Error != nil {
		if errCheck.Error.Code == "budget_exceeded" || strings.Contains(errCheck.Error.Message, "daily usage limit") {
			status = "exceeded"
			msg = "Daily usage limit reached. Resets at 4:00 AM Melbourne Time."
		}
	}

	return &QuotaStatus{
		Status:           status,
		Message:          msg,
		MelbourneTime:    nowMel.Format("15:04 MST"),
		ResetAtMelbourne: "04:00 AEDT",
		Countdown:        countdownStr,
		SecondsUntil:     int64(diff.Seconds()),
	}, nil
}

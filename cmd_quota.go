package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"val2harness/internal/analytics"
	"val2harness/internal/config"
)

// readQuota reads the running gateway cache. It never contacts the Val service.
func readQuota(ctx context.Context, cfg *config.Config) (*analytics.QuotaStatus, error) {
	base, err := url.Parse(config.LocalBaseURL(cfg.ListenAddr))
	if err != nil || base.User != nil || base.RawQuery != "" || base.Fragment != "" || base.Path != "" {
		return nil, errors.New("invalid local gateway address")
	}
	host := base.Hostname()
	if host != "localhost" && (net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback()) {
		return nil, errors.New("quota status requires a loopback gateway address")
	}
	if cfg.APIKey == "" {
		return nil, errors.New("local gateway owner key is not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String()+"/api/local/quota", nil)
	if err != nil {
		return nil, errors.New("invalid local gateway request")
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("local gateway unavailable; start it with val2harness start")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("local gateway returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 65537))
	if err != nil || len(data) > 65536 {
		return nil, errors.New("local quota response could not be read within its limit")
	}
	var q analytics.QuotaStatus
	if json.Unmarshal(data, &q) != nil || q.Source != "gateway_session" {
		return nil, errors.New("invalid local quota response; update the gateway")
	}
	switch q.Status {
	case "unknown", "available", "exhausted", "rate_limited", "authentication_error", "provider_error":
	default:
		return nil, errors.New("unrecognized local quota status")
	}
	return &q, nil
}

func runQuota(args []string) {
	if len(args) > 1 || (len(args) == 1 && args[0] != "--json") {
		fmt.Fprintln(os.Stderr, "Usage: val2harness quota [--json]")
		os.Exit(1)
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Could not load gateway configuration")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	q, err := readQuota(ctx, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Quota observation unavailable:", err)
		os.Exit(1)
	}
	if len(args) == 1 {
		json.NewEncoder(os.Stdout).Encode(q)
		return
	}
	fmt.Printf("Gateway session observations\n  Status: %s\n", q.Status)
	if q.ObservedAt != nil {
		fmt.Println("  Observed:", q.ObservedAt.Format(time.RFC3339))
	} else {
		fmt.Println("  Observed: No completed request yet")
	}
	fmt.Printf("  Requests: %d (%d measured, %d unmeasured)\n", q.ObservedRequests, q.MeasuredRequests, q.UnmeasuredRequests)
	if q.MeasuredRequests > 0 {
		fmt.Printf("  Measured tokens: %d input, %d output, %d total\n", q.PromptTokens, q.CompletionTokens, q.TotalTokens)
	} else {
		fmt.Println("  Measured tokens: Unknown")
	}
	fmt.Println("  Remaining account quota: Unknown\n  Quota reset time: Unknown")
	fmt.Println("Only requests through this running gateway are included; refresh sends no inference.")
}

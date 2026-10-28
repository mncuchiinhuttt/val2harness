// Package analytics holds bounded, in-memory observations of this gateway session.
package analytics

import (
	"context"
	"time"
)

type protocolKey struct{}

func WithProtocol(ctx context.Context, protocol string) context.Context {
	return context.WithValue(ctx, protocolKey{}, protocol)
}

func Protocol(ctx context.Context) string {
	if protocol, _ := ctx.Value(protocolKey{}).(string); protocol == "anthropic" {
		return protocol
	}
	return "openai"
}

// Observation contains metadata only, never request/response content or credentials.
type Observation struct {
	Model, Protocol, Status                     string
	StatusCode                                  int
	LatencyMs                                   int64
	UsageKnown                                  bool
	PromptTokens, CompletionTokens, TotalTokens int
}

type QuotaStatus struct {
	Status             string     `json:"status"`
	Message            string     `json:"message"`
	Source             string     `json:"source"`
	ObservedAt         *time.Time `json:"observed_at"`
	ObservedRequests   int        `json:"observed_requests"`
	MeasuredRequests   int        `json:"measured_requests"`
	UnmeasuredRequests int        `json:"unmeasured_requests"`
	PromptTokens       int        `json:"prompt_tokens"`
	CompletionTokens   int        `json:"completion_tokens"`
	TotalTokens        int        `json:"total_tokens"`
	Remaining          *int       `json:"remaining"`
	Limit              *int       `json:"limit"`
	ResetAt            *time.Time `json:"reset_at"`
}

type UsageRecord struct {
	Timestamp        int64  `json:"timestamp"`
	Date             string `json:"date"`
	Model            string `json:"model"`
	Protocol         string `json:"protocol"`
	UsageKnown       bool   `json:"usage_known"`
	PromptTokens     *int   `json:"prompt_tokens"`
	CompletionTokens *int   `json:"completion_tokens"`
	TotalTokens      *int   `json:"total_tokens"`
	LatencyMs        int64  `json:"latency_ms"`
	StatusCode       int    `json:"status_code"`
}

type ModelUsageStat struct {
	Model              string `json:"model"`
	Requests           int    `json:"requests"`
	MeasuredRequests   int    `json:"measured_requests"`
	UnmeasuredRequests int    `json:"unmeasured_requests"`
	PromptTokens       int    `json:"prompt_tokens"`
	CompletionTokens   int    `json:"completion_tokens"`
	TotalTokens        int    `json:"total_tokens"`
}

type AnalyticsSummary struct {
	Source             string           `json:"source"`
	TotalRequests      int              `json:"total_requests"`
	MeasuredRequests   int              `json:"measured_requests"`
	UnmeasuredRequests int              `json:"unmeasured_requests"`
	TotalTokens        int              `json:"total_tokens"`
	PromptTokens       int              `json:"prompt_tokens"`
	CompletionTokens   int              `json:"completion_tokens"`
	ActiveDays         int              `json:"active_days"`
	AvgLatencyMs       int64            `json:"avg_latency_ms"`
	ModelsBreakdown    []ModelUsageStat `json:"models_breakdown"`
	DailyRecords       map[string]int   `json:"daily_records"`
	DailyTokens        map[string]int   `json:"daily_tokens"`
	AvailableYears     []int            `json:"available_years"`
	RecentLogs         []UsageRecord    `json:"recent_logs"`
}

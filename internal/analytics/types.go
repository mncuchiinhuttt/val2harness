package analytics

type UsageRecord struct {
	Timestamp        int64  `json:"timestamp"`
	Date             string `json:"date"` // YYYY-MM-DD
	Model            string `json:"model"`
	Protocol         string `json:"protocol"` // "openai" or "anthropic"
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
	LatencyMs        int64  `json:"latency_ms"`
	StatusCode       int    `json:"status_code"`
}

type HeatmapCell struct {
	Date   string `json:"date"` // YYYY-MM-DD
	Count  int    `json:"count"`
	Tokens int    `json:"tokens"`
	Level  int    `json:"level"` // 0 to 4 (GitHub color scale)
}

type ModelUsageStat struct {
	Model            string `json:"model"`
	Requests         int    `json:"requests"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
}

type AnalyticsSummary struct {
	TotalRequests    int              `json:"total_requests"`
	TotalTokens      int              `json:"total_tokens"`
	PromptTokens     int              `json:"prompt_tokens"`
	CompletionTokens int              `json:"completion_tokens"`
	ActiveDays       int              `json:"active_days"`
	AvgLatencyMs     int64            `json:"avg_latency_ms"`
	ModelsBreakdown  []ModelUsageStat `json:"models_breakdown"`
	Heatmap          []HeatmapCell    `json:"heatmap"`
	RecentLogs       []UsageRecord    `json:"recent_logs"`
}

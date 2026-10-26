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
	DailyRecords     map[string]int   `json:"daily_records"`
	DailyTokens      map[string]int   `json:"daily_tokens"`
	AvailableYears   []int            `json:"available_years"`
	RecentLogs       []UsageRecord    `json:"recent_logs"`
}

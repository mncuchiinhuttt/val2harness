package analytics

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

func (t *Tracker) buildSummary() AnalyticsSummary {
	avgLat := int64(0)
	if t.quota.ObservedRequests > 0 {
		avgLat = t.totalLat / int64(t.quota.ObservedRequests)
	}

	mList := make([]ModelUsageStat, 0, len(t.modelsMap))
	for _, ms := range t.modelsMap {
		mList = append(mList, *ms)
	}
	sort.Slice(mList, func(i, j int) bool {
		if mList[i].Requests == mList[j].Requests {
			return mList[i].Model < mList[j].Model
		}
		return mList[i].Requests > mList[j].Requests
	})

	dailyRecords := make(map[string]int)
	dailyTokens := make(map[string]int)
	yearsMap := map[int]bool{time.Now().Year(): true}

	for dateStr, cell := range t.dailyMap {
		dailyRecords[dateStr] = cell.Count
		dailyTokens[dateStr] = cell.Tokens
		parts := strings.Split(dateStr, "-")
		if len(parts) > 0 {
			if y, err := strconv.Atoi(parts[0]); err == nil {
				yearsMap[y] = true
			}
		}
	}

	var availableYears []int
	for y := range yearsMap {
		availableYears = append(availableYears, y)
	}
	sort.Slice(availableYears, func(i, j int) bool {
		return availableYears[i] > availableYears[j]
	})

	recent := make([]UsageRecord, 0)
	for i := len(t.records) - 1; i >= 0; i-- {
		r := t.records[i]
		if r.PromptTokens != nil {
			prompt, completion, total := *r.PromptTokens, *r.CompletionTokens, *r.TotalTokens
			r.PromptTokens = &prompt
			r.CompletionTokens = &completion
			r.TotalTokens = &total
		}
		recent = append(recent, r)
	}

	return AnalyticsSummary{
		Source:             "gateway_session",
		TotalRequests:      t.quota.ObservedRequests,
		MeasuredRequests:   t.quota.MeasuredRequests,
		UnmeasuredRequests: t.quota.UnmeasuredRequests,
		TotalTokens:        t.quota.TotalTokens,
		PromptTokens:       t.quota.PromptTokens,
		CompletionTokens:   t.quota.CompletionTokens,
		ActiveDays:         len(t.dailyMap),
		AvgLatencyMs:       avgLat,
		ModelsBreakdown:    mList,
		DailyRecords:       dailyRecords,
		DailyTokens:        dailyTokens,
		AvailableYears:     availableYears,
		RecentLogs:         recent,
	}
}

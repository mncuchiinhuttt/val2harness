package analytics

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

func (t *Tracker) buildSummary() AnalyticsSummary {
	avgLat := int64(0)
	if t.totalReqs > 0 {
		avgLat = t.totalLat / int64(t.totalReqs)
	}

	var mList []ModelUsageStat
	for _, ms := range t.modelsMap {
		mList = append(mList, *ms)
	}
	sort.Slice(mList, func(i, j int) bool {
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
	for i := len(t.records) - 1; i >= 0 && len(recent) < 25; i-- {
		recent = append(recent, t.records[i])
	}

	return AnalyticsSummary{
		TotalRequests:    t.totalReqs,
		TotalTokens:      t.totalToks,
		PromptTokens:     t.promptToks,
		CompletionTokens: t.compToks,
		ActiveDays:       len(t.dailyMap),
		AvgLatencyMs:     avgLat,
		ModelsBreakdown:  mList,
		DailyRecords:     dailyRecords,
		DailyTokens:      dailyTokens,
		AvailableYears:   availableYears,
		RecentLogs:       recent,
	}
}

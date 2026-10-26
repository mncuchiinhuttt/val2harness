package analytics

import (
	"sort"
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

	now := time.Now()
	sunWeekday := int(now.Weekday())
	totalDays := 52*7 + (sunWeekday + 1)
	startDate := now.AddDate(0, 0, -(totalDays - 1))

	heatmap := make([]HeatmapCell, 0, 53*7)
	for i := range totalDays {
		day := startDate.AddDate(0, 0, i)
		dStr := day.Format("2006-01-02")
		cell := HeatmapCell{
			Date:      dStr,
			DayOfWeek: int(day.Weekday()),
			Month:     day.Format("Jan"),
		}
		if existing, ok := t.dailyMap[dStr]; ok {
			cell.Count = existing.Count
			cell.Tokens = existing.Tokens
			switch {
			case cell.Count >= 20:
				cell.Level = 4
			case cell.Count >= 10:
				cell.Level = 3
			case cell.Count >= 4:
				cell.Level = 2
			case cell.Count >= 1:
				cell.Level = 1
			}
		}
		heatmap = append(heatmap, cell)
	}

	rem := (7 - (int(now.Weekday()) + 1)) % 7
	for i := 1; i <= rem; i++ {
		futureDay := now.AddDate(0, 0, i)
		heatmap = append(heatmap, HeatmapCell{
			Date:      futureDay.Format("2006-01-02"),
			DayOfWeek: int(futureDay.Weekday()),
			Month:     futureDay.Format("Jan"),
			IsFuture:  true,
		})
	}

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
		Heatmap:          heatmap,
		RecentLogs:       recent,
	}
}

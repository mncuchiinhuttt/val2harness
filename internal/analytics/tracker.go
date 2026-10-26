package analytics

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"val2harness/internal/config"
)

type Tracker struct {
	mu         sync.RWMutex
	records    []UsageRecord
	dailyMap   map[string]*HeatmapCell
	modelsMap  map[string]*ModelUsageStat
	totalReqs  int
	totalToks  int
	promptToks int
	compToks   int
	totalLat   int64
	filePath   string
}

var globalTracker *Tracker
var once sync.Once

func GetTracker() *Tracker {
	once.Do(func() {
		globalTracker = newTracker()
	})
	return globalTracker
}

func newTracker() *Tracker {
	dir, _ := config.GetConfigDir()
	filePath := filepath.Join(dir, "usage.jsonl")

	t := &Tracker{
		records:   make([]UsageRecord, 0),
		dailyMap:  make(map[string]*HeatmapCell),
		modelsMap: make(map[string]*ModelUsageStat),
		filePath:  filePath,
	}
	t.load()
	return t
}

func (t *Tracker) load() {
	f, err := os.Open(t.filePath)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var r UsageRecord
		if err := json.Unmarshal(scanner.Bytes(), &r); err == nil {
			t.aggregate(r)
		}
	}
}

func (t *Tracker) aggregate(r UsageRecord) {
	t.records = append(t.records, r)
	t.totalReqs++
	t.totalToks += r.TotalTokens
	t.promptToks += r.PromptTokens
	t.compToks += r.CompletionTokens
	t.totalLat += r.LatencyMs

	// Daily heatmap cell
	cell, exists := t.dailyMap[r.Date]
	if !exists {
		cell = &HeatmapCell{Date: r.Date}
		t.dailyMap[r.Date] = cell
	}
	cell.Count++
	cell.Tokens += r.TotalTokens

	// Model stat
	ms, exists := t.modelsMap[r.Model]
	if !exists {
		ms = &ModelUsageStat{Model: r.Model}
		t.modelsMap[r.Model] = ms
	}
	ms.Requests++
	ms.PromptTokens += r.PromptTokens
	ms.CompletionTokens += r.CompletionTokens
	ms.TotalTokens += r.TotalTokens
}

func (t *Tracker) Record(model, protocol string, promptTokens, compTokens int, latencyMs int64, statusCode int) {
	now := time.Now()
	dateStr := now.Format("2006-01-02")
	total := promptTokens + compTokens
	if total == 0 {
		total = 10
	}

	rec := UsageRecord{
		Timestamp:        now.Unix(),
		Date:             dateStr,
		Model:            model,
		Protocol:         protocol,
		PromptTokens:     promptTokens,
		CompletionTokens: compTokens,
		TotalTokens:      total,
		LatencyMs:        latencyMs,
		StatusCode:       statusCode,
	}

	t.mu.Lock()
	t.aggregate(rec)
	t.mu.Unlock()

	// Persist asynchronously
	go func(r UsageRecord) {
		_ = os.MkdirAll(filepath.Dir(t.filePath), 0755)
		f, err := os.OpenFile(t.filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			defer f.Close()
			data, _ := json.Marshal(r)
			f.Write(append(data, '\n'))
		}
	}(rec)
}

func (t *Tracker) GetSummary() AnalyticsSummary {
	t.mu.RLock()
	defer t.mu.RUnlock()

	avgLat := int64(0)
	if t.totalReqs > 0 {
		avgLat = t.totalLat / int64(t.totalReqs)
	}

	// Models breakdown sorted by requests descending
	var mList []ModelUsageStat
	for _, ms := range t.modelsMap {
		mList = append(mList, *ms)
	}
	sort.Slice(mList, func(i, j int) bool {
		return mList[i].Requests > mList[j].Requests
	})

	// GitHub style 26-week / 182-day calendar heatmap
	now := time.Now()
	daysCount := 182
	heatmap := make([]HeatmapCell, 0, daysCount)
	for i := daysCount - 1; i >= 0; i-- {
		day := now.AddDate(0, 0, -i)
		dStr := day.Format("2006-01-02")
		cell := HeatmapCell{Date: dStr, Count: 0, Tokens: 0, Level: 0}
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

	// Recent logs (last 25)
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

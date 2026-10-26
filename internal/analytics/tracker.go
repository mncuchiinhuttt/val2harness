package analytics

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"val2harness/internal/config"
)

type dayBucket struct {
	Count  int
	Tokens int
}

type Tracker struct {
	mu          sync.RWMutex
	records     []UsageRecord
	dailyMap    map[string]*dayBucket
	modelsMap   map[string]*ModelUsageStat
	totalReqs   int
	totalToks   int
	promptToks  int
	compToks    int
	totalLat    int64
	filePath    string
	listenersMu sync.Mutex
	listeners   map[chan AnalyticsSummary]struct{}
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
		dailyMap:  make(map[string]*dayBucket),
		modelsMap: make(map[string]*ModelUsageStat),
		filePath:  filePath,
		listeners: make(map[chan AnalyticsSummary]struct{}),
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

	b, exists := t.dailyMap[r.Date]
	if !exists {
		b = &dayBucket{}
		t.dailyMap[r.Date] = b
	}
	b.Count++
	b.Tokens += r.TotalTokens

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
	summary := t.buildSummary()
	t.mu.Unlock()

	t.broadcast(summary)

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
	return t.buildSummary()
}

package analytics

import (
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

const maxRecords = 256
const maxModels = 256
const maxDays = 400

// MaxExactTokens is the largest integer that remains exact in dashboard JSON.
const MaxExactTokens int64 = 1<<53 - 1

type dayBucket struct{ Count, Tokens int }

type Tracker struct {
	mu         sync.RWMutex
	generation uint64
	records    []UsageRecord
	dailyMap   map[string]*dayBucket
	modelsMap  map[string]*ModelUsageStat
	quota      QuotaStatus
	totalLat   int64
	listeners  map[chan AnalyticsSummary]struct{}
}

func NewTracker() *Tracker {
	t := &Tracker{listeners: make(map[chan AnalyticsSummary]struct{})}
	t.clear()
	return t
}

func (t *Tracker) clear() {
	t.generation++
	t.records = nil
	t.dailyMap = make(map[string]*dayBucket)
	t.modelsMap = make(map[string]*ModelUsageStat)
	t.totalLat = 0
	t.quota = QuotaStatus{Status: "unknown", Message: statusMessage("unknown"), Source: "gateway_session"}
}

func (t *Tracker) Begin() uint64 { t.mu.RLock(); defer t.mu.RUnlock(); return t.generation }

func (t *Tracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.clear()
	t.broadcast()
}

func statusMessage(status string) string {
	switch status {
	case "available":
		return "A valid completion was observed. The next request may have a different result."
	case "exhausted":
		return "Val reported that its usage budget was exceeded. Remaining allowance and reset time are unknown."
	case "rate_limited":
		return "Val temporarily limited a request. This does not establish the remaining account budget."
	case "authentication_error":
		return "Val rejected the account credentials. Reconnect your account."
	case "provider_error":
		return "Val returned an error. Account allowance remains unknown."
	default:
		return "No valid completed response is known for the latest observation. Account allowance remains unknown."
	}
}

// ModelName bounds display metadata and copies it so a short model name cannot
// keep a large untrusted response/request allocation alive.
func ModelName(value string) string {
	if len(value) > 128 {
		value = value[:128]
		for !utf8.ValidString(value) && len(value) > 0 {
			value = value[:len(value)-1]
		}
	}
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
	if value == "" {
		return "unknown"
	}
	return strings.Clone(value)
}

func add(a, b int) int {
	max := int(^uint(0) >> 1)
	if b > max-a {
		return max
	}
	return a + b
}

func (t *Tracker) Observe(generation uint64, o Observation) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if generation != t.generation {
		return
	}
	now := time.Now()
	switch o.Status {
	case "unknown", "available", "exhausted", "rate_limited", "authentication_error", "provider_error":
	default:
		o.Status = "unknown"
	}
	if o.Protocol != "anthropic" {
		o.Protocol = "openai"
	}
	o.Model = ModelName(o.Model)
	max := int(^uint(0) >> 1)
	if o.PromptTokens < 0 || o.CompletionTokens < 0 || o.TotalTokens < 0 || o.PromptTokens > max-o.CompletionTokens || o.TotalTokens != o.PromptTokens+o.CompletionTokens {
		o.UsageKnown = false
	}
	// Preserve exact prior totals. An unsafe additional measurement is unknown;
	// saturating or rounding it would report token usage we did not measure.
	if int64(o.TotalTokens) > MaxExactTokens-int64(t.quota.TotalTokens) {
		o.UsageKnown = false
	}
	rec := UsageRecord{Timestamp: now.Unix(), Date: now.Format("2006-01-02"), Model: o.Model, Protocol: o.Protocol, UsageKnown: o.UsageKnown, LatencyMs: o.LatencyMs, StatusCode: o.StatusCode}
	t.quota.Status = o.Status
	t.quota.Message = statusMessage(o.Status)
	t.quota.ObservedAt = &now
	t.quota.ObservedRequests = add(t.quota.ObservedRequests, 1)
	if o.UsageKnown {
		rec.PromptTokens = &o.PromptTokens
		rec.CompletionTokens = &o.CompletionTokens
		rec.TotalTokens = &o.TotalTokens
		t.quota.MeasuredRequests = add(t.quota.MeasuredRequests, 1)
		t.quota.PromptTokens = add(t.quota.PromptTokens, o.PromptTokens)
		t.quota.CompletionTokens = add(t.quota.CompletionTokens, o.CompletionTokens)
		t.quota.TotalTokens = add(t.quota.TotalTokens, o.TotalTokens)
	} else {
		t.quota.UnmeasuredRequests = add(t.quota.UnmeasuredRequests, 1)
	}
	if len(t.records) == maxRecords {
		copy(t.records, t.records[1:])
		t.records[len(t.records)-1] = rec
	} else {
		t.records = append(t.records, rec)
	}
	if o.LatencyMs > 0 && t.totalLat <= int64(^uint64(0)>>1)-o.LatencyMs {
		t.totalLat += o.LatencyMs
	}
	if t.dailyMap[rec.Date] == nil {
		if len(t.dailyMap) >= maxDays {
			oldest := "9999"
			for day := range t.dailyMap {
				if day < oldest {
					oldest = day
				}
			}
			delete(t.dailyMap, oldest)
		}
		t.dailyMap[rec.Date] = &dayBucket{}
	}
	b := t.dailyMap[rec.Date]
	b.Count = add(b.Count, 1)
	if o.UsageKnown {
		b.Tokens = add(b.Tokens, o.TotalTokens)
	}
	model := o.Model
	if t.modelsMap[model] == nil && len(t.modelsMap) >= maxModels {
		model = "(other models)"
	}
	if t.modelsMap[model] == nil {
		t.modelsMap[model] = &ModelUsageStat{Model: model}
	}
	m := t.modelsMap[model]
	m.Requests = add(m.Requests, 1)
	if o.UsageKnown {
		m.MeasuredRequests = add(m.MeasuredRequests, 1)
		m.PromptTokens = add(m.PromptTokens, o.PromptTokens)
		m.CompletionTokens = add(m.CompletionTokens, o.CompletionTokens)
		m.TotalTokens = add(m.TotalTokens, o.TotalTokens)
	} else {
		m.UnmeasuredRequests = add(m.UnmeasuredRequests, 1)
	}
	t.broadcast()
}

func (t *Tracker) SnapshotQuota() QuotaStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()
	q := t.quota
	if q.ObservedAt != nil {
		observed := *q.ObservedAt
		q.ObservedAt = &observed
	}
	return q
}

func (t *Tracker) GetSummary() AnalyticsSummary {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.buildSummary()
}

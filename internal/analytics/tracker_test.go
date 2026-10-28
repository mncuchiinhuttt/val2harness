package analytics

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestTrackerUsesOnlyMeasuredUsageAndResetsGeneration(t *testing.T) {
	tracker := NewTracker()
	initial, _ := json.Marshal(tracker.SnapshotQuota())
	for _, field := range []string{`"status":"unknown"`, `"source":"gateway_session"`, `"remaining":null`, `"limit":null`, `"reset_at":null`, `"observed_at":null`} {
		if !strings.Contains(string(initial), field) {
			t.Errorf("initial quota missing %s: %s", field, initial)
		}
	}
	generation := tracker.Begin()
	tracker.Observe(generation, Observation{Model: "synthetic", Protocol: "openai", Status: "available", StatusCode: 200, UsageKnown: true, PromptTokens: 0, CompletionTokens: 0, TotalTokens: 0})
	tracker.Observe(generation, Observation{Model: "synthetic", Status: "available", StatusCode: 200})
	q := tracker.SnapshotQuota()
	if q.ObservedRequests != 2 || q.MeasuredRequests != 1 || q.UnmeasuredRequests != 1 || q.TotalTokens != 0 {
		t.Fatalf("invented or missing usage: %+v", q)
	}
	summary := tracker.GetSummary()
	if summary.RecentLogs[0].UsageKnown || summary.RecentLogs[0].TotalTokens != nil || !summary.RecentLogs[1].UsageKnown || *summary.RecentLogs[1].TotalTokens != 0 {
		t.Fatalf("unknown usage must remain nullable: %+v", summary.RecentLogs)
	}
	ch := tracker.Subscribe()
	defer tracker.Unsubscribe(ch)
	tracker.Reset()
	select {
	case update := <-ch:
		if update.TotalRequests != 0 {
			t.Fatal("reset event retained history")
		}
	case <-time.After(time.Second):
		t.Fatal("reset was not broadcast")
	}
	tracker.Observe(generation, Observation{Status: "available", UsageKnown: true, PromptTokens: 9, CompletionTokens: 3, TotalTokens: 12})
	if q := tracker.SnapshotQuota(); q.ObservedRequests != 0 || q.Status != "unknown" {
		t.Fatalf("stale response repopulated reset: %+v", q)
	}
}

func TestTrackerBoundsMetadataAndDetachesSnapshots(t *testing.T) {
	tracker := NewTracker()
	for i := 0; i < 2000; i++ {
		tracker.Observe(tracker.Begin(), Observation{Model: fmt.Sprintf("%d%s", i, strings.Repeat("x", 1024)), Protocol: "private-invalid", Status: "available", UsageKnown: true, PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3})
	}
	s := tracker.GetSummary()
	if len(s.ModelsBreakdown) > 257 || len(s.RecentLogs) > 256 || s.TotalRequests != 2000 || s.TotalTokens != 6000 {
		t.Fatalf("unbounded/lost aggregates: models=%d logs=%d total=%d tokens=%d", len(s.ModelsBreakdown), len(s.RecentLogs), s.TotalRequests, s.TotalTokens)
	}
	for _, rec := range s.RecentLogs {
		if len(rec.Model) > 128 || rec.Protocol != "openai" {
			t.Fatalf("unbounded metadata: %+v", rec)
		}
	}
	s.RecentLogs[0].Model = "changed"
	*s.RecentLogs[0].TotalTokens = 999
	for day := range s.DailyRecords {
		s.DailyRecords[day] = 999
	}
	s2 := tracker.GetSummary()
	if s2.RecentLogs[0].Model == "changed" || *s2.RecentLogs[0].TotalTokens != 3 {
		t.Fatal("snapshot mutated tracker")
	}
}

func TestTrackerConcurrentResetObservationAndSubscriptions(t *testing.T) {
	tracker := NewTracker()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 100; n++ {
				generation := tracker.Begin()
				ch := tracker.Subscribe()
				tracker.Observe(generation, Observation{Status: "unknown"})
				_ = tracker.GetSummary()
				_ = tracker.SnapshotQuota()
				tracker.Reset()
				tracker.Unsubscribe(ch)
				tracker.Unsubscribe(ch)
			}
		}()
	}
	wg.Wait()
}

func TestTrackerKeepsExactTokenTotalsWhenNewUsageWouldOverflow(t *testing.T) {
	large, err := strconv.Atoi("9007199254740991")
	if err != nil {
		t.Skip("requires 64-bit integers")
	}
	tracker := NewTracker()
	tracker.Observe(tracker.Begin(), Observation{Status: "available", UsageKnown: true, PromptTokens: large, TotalTokens: large})
	tracker.Observe(tracker.Begin(), Observation{Status: "available", UsageKnown: true, PromptTokens: 1, TotalTokens: 1})
	tracker.Observe(tracker.Begin(), Observation{Status: "available", UsageKnown: true, PromptTokens: large + 1, TotalTokens: large + 1})
	q := tracker.SnapshotQuota()
	if q.TotalTokens != large || q.PromptTokens != large || q.MeasuredRequests != 1 || q.UnmeasuredRequests != 2 {
		t.Fatalf("overflow invented aggregate measurement: %+v", q)
	}
}

func TestTrackerDoesNotRetainLargeMetadataBackingStorage(t *testing.T) {
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	tracker := NewTracker()
	for i := 0; i < 32; i++ {
		tracker.Observe(tracker.Begin(), Observation{Model: fmt.Sprintf("model-%d-%s", i, strings.Repeat("x", 1<<20)), Status: "unknown"})
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(tracker)
	if after.HeapAlloc > before.HeapAlloc+(16<<20) {
		t.Fatalf("bounded metadata retained large original strings: live heap grew by %d bytes", after.HeapAlloc-before.HeapAlloc)
	}
}

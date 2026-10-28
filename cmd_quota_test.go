package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"val2harness/internal/config"
)

func TestQuotaReadsOnlyAuthenticatedLocalSnapshot(t *testing.T) {
	var calls atomic.Int32
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "GET" || r.URL.Path != "/api/local/quota" || r.Header.Get("Authorization") != "Bearer local-owner" {
			t.Errorf("unexpected snapshot request: %s %s", r.Method, r.URL.Path)
		}
		fmt.Fprint(w, `{"status":"unknown","source":"gateway_session","observed_at":null,"remaining":null,"limit":null,"reset_at":null,"observed_requests":0,"measured_requests":0,"unmeasured_requests":0}`)
	}))
	defer local.Close()
	q, err := readQuota(context.Background(), &config.Config{ListenAddr: strings.TrimPrefix(local.URL, "http://"), APIKey: "local-owner", UpstreamURL: "https://val.invalid", Token: "must-not-send"})
	if err != nil || q.Status != "unknown" || q.Remaining != nil || q.ResetAt != nil || calls.Load() != 1 {
		t.Fatalf("snapshot = %+v, err = %v, local calls = %d", q, err, calls.Load())
	}
}

func TestQuotaRefusesRedirectAndNonlocalAddress(t *testing.T) {
	var leaked atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Add(1) }))
	defer destination.Close()
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer local.Close()
	for _, addr := range []string{strings.TrimPrefix(local.URL, "http://"), "example.invalid:8765"} {
		if _, err := readQuota(context.Background(), &config.Config{ListenAddr: addr, APIKey: "local-owner"}); err == nil {
			t.Fatalf("accepted redirect or nonlocal address: %s", addr)
		}
	}
	if leaked.Load() != 0 {
		t.Fatal("followed redirect with local owner credential")
	}
}

func TestQuotaRejectsInvalidSnapshotsAndHonorsCancellation(t *testing.T) {
	for _, body := range []string{`not-json`, `{}`, `{"status":"active","source":"gateway_session"}`, `{"status":"available","source":"somewhere_else"}`, strings.Repeat("x", 65537)} {
		t.Run(fmt.Sprint(len(body), body[:min(12, len(body))]), func(t *testing.T) {
			local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			defer local.Close()
			if _, err := readQuota(context.Background(), &config.Config{ListenAddr: strings.TrimPrefix(local.URL, "http://"), APIKey: "owner"}); err == nil {
				t.Fatal("accepted invalid snapshot")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := readQuota(ctx, &config.Config{ListenAddr: "127.0.0.1:1", APIKey: "owner"}); err == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestQuotaPrintsMeasuredUsageWithoutInventingReset(t *testing.T) {
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"status":"available","source":"gateway_session","observed_at":"2026-10-28T10:00:00Z","observed_requests":2,"measured_requests":1,"unmeasured_requests":1,"prompt_tokens":12,"completion_tokens":3,"total_tokens":15,"remaining":null,"limit":null,"reset_at":null}`)
	}))
	defer local.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	q, err := readQuota(ctx, &config.Config{ListenAddr: strings.TrimPrefix(local.URL, "http://"), APIKey: "owner"})
	if err != nil || q.PromptTokens != 12 || q.CompletionTokens != 3 || q.MeasuredRequests != 1 || q.UnmeasuredRequests != 1 || q.ResetAt != nil {
		t.Fatalf("snapshot=%+v err=%v", q, err)
	}
}

package valclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"val2harness/internal/analytics"
)

const completeJSON = `{"id":"synthetic","choices":[{"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":4,"total_tokens":9}}`
const streamEnd = "data:{\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata:[DONE]\n\n"

func TestPassiveJSONMeasurementAndStatus(t *testing.T) {
	for _, tc := range []struct {
		name, body, status     string
		httpStatus, normalized int
		measured               bool
		total                  int
	}{
		{"measured", completeJSON, "available", 200, 200, true, 9},
		{"zero", strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(completeJSON, `":5`, `":0`), `":4`, `":0`), `":9`, `":0`), "available", 200, 200, true, 0},
		{"missing_usage", `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`, "available", 200, 200, false, 0},
		{"arbitrary_object", `{}`, "unknown", 200, 200, false, 0},
		{"empty_message", `{"choices":[{"message":{},"finish_reason":"stop"}]}`, "unknown", 200, 200, false, 0},
		{"invalid_content", `{"choices":[{"message":{"content":5},"finish_reason":"stop"}]}`, "unknown", 200, 200, false, 0},
		{"tool_completion", `{"choices":[{"message":{"content":null,"tool_calls":[{"id":"synthetic","type":"function","function":{"name":"lookup","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":0,"completion_tokens":0}}`, "available", 200, 200, true, 0},
		{"invalid_json", `not-json`, "unknown", 200, 200, false, 0},
		{"extra_document", completeJSON + ` {}`, "unknown", 200, 200, false, 0},
		{"unfinished", `{"choices":[{"message":{"content":"ok"},"finish_reason":null}],"usage":{"prompt_tokens":1,"completion_tokens":2}}`, "unknown", 200, 200, false, 0},
		{"negative", strings.ReplaceAll(completeJSON, `"prompt_tokens":5`, `"prompt_tokens":-5`), "available", 200, 200, false, 0},
		{"inconsistent_total", strings.ReplaceAll(completeJSON, `"total_tokens":9`, `"total_tokens":10`), "available", 200, 200, false, 0},
		{"fractional", strings.ReplaceAll(completeJSON, `"prompt_tokens":5`, `"prompt_tokens":5.5`), "available", 200, 200, false, 0},
		{"missing_total", strings.ReplaceAll(completeJSON, `,"total_tokens":9`, ``), "available", 200, 200, true, 9},
		{"null", `null`, "unknown", 200, 200, false, 0},
		{"null_error", strings.ReplaceAll(completeJSON, `"id":`, `"error":null,"id":`), "available", 200, 200, true, 9},
		{"exact_budget", `{"error":{"code":"budget_exceeded","message":"synthetic"}}`, "exhausted", 200, 429, false, 0},
		{"budget_429", `{"error":{"code":"budget_exceeded"}}`, "exhausted", 429, 429, false, 0},
		{"budget_text_only", `{"error":{"code":"other","message":"daily usage limit"}}`, "provider_error", 200, 200, false, 0},
		{"unrelated_error", `{"error":{"code":"invalid_request"}}`, "provider_error", 400, 400, false, 0},
		{"unauthorized", `{"error":{"code":"invalid_token"}}`, "authentication_error", 401, 401, false, 0},
		{"forbidden", `no`, "authentication_error", 403, 403, false, 0},
		{"rate_limit", `{"error":{"code":"rate_limit_exceeded"}}`, "rate_limited", 429, 429, false, 0},
		{"provider", `bad gateway`, "provider_error", 502, 502, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "17")
				w.WriteHeader(tc.httpStatus)
				io.WriteString(w, tc.body)
			}))
			defer up.Close()
			c := NewClient(up.URL, "synthetic")
			resp, err := c.ProxyChat(analytics.WithProtocol(context.Background(), "anthropic"), strings.NewReader(`{"model":"synthetic","messages":[{"role":"user","content":"PRIVATE-PROMPT"}]}`))
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil || string(data) != tc.body || resp.StatusCode != tc.normalized || resp.Header.Get("Retry-After") != "17" {
				t.Fatalf("forwarded response changed: code=%d body=%q error=%v", resp.StatusCode, data, err)
			}
			q := c.UsageTracker().SnapshotQuota()
			if q.Status != tc.status || q.ObservedRequests != 1 || (q.MeasuredRequests == 1) != tc.measured || q.TotalTokens != tc.total {
				t.Fatalf("wrong observation: %+v", q)
			}
			s := c.UsageTracker().GetSummary()
			if s.RecentLogs[0].Protocol != "anthropic" {
				t.Fatal("protocol tag lost")
			}
			if calls.Load() != 1 {
				t.Fatalf("observer sent %d upstream requests", calls.Load())
			}
		})
	}
}

type observationTransport func(*http.Request) (*http.Response, error)

func (f observationTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type fragmentedReader struct{ io.Reader }

func (r fragmentedReader) Read(p []byte) (int, error) {
	if len(p) > 3 {
		p = p[:3]
	}
	return r.Reader.Read(p)
}

func fixtureClient(body io.ReadCloser, contentType string) *Client {
	c := NewClient("http://synthetic.invalid", "synthetic")
	c.httpClient.Transport = observationTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {contentType}}, Body: body, Request: r}, nil
	})
	return c
}

func TestPassiveSSEIncludesLateCumulativeUsageOnceAndPreservesBytes(t *testing.T) {
	for _, newline := range []string{"\n", "\r\n", "\r"} {
		t.Run(strings.ReplaceAll(newline, "\r", "CR"), func(t *testing.T) {
			body := ": keepalive\n\ndata:{\"choices\":[{\"index\":0,\"delta\":{\"content\":\"private-response\"},\"finish_reason\":null}],\ndata: \"usage\":{\"prompt_tokens\":5,\"completion_tokens\":2,\"total_tokens\":7}}\n\n" +
				"data:{\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
				"data:{\"choices\":[],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":4,\"total_tokens\":9}}\n\n" +
				"data:{\"choices\":[],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":4,\"total_tokens\":9}}\n\ndata:[DONE]\n\n"
			body = strings.ReplaceAll(body, "\n", newline)
			c := fixtureClient(io.NopCloser(fragmentedReader{strings.NewReader(body)}), "text/event-stream; charset=utf-8")
			resp, err := c.ProxyChat(context.Background(), strings.NewReader(`{"model":"synthetic","stream":true}`))
			if err != nil {
				t.Fatal(err)
			}
			got, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil || string(got) != body {
				t.Fatalf("SSE bytes changed: %v", err)
			}
			q := c.UsageTracker().SnapshotQuota()
			if q.Status != "available" || q.MeasuredRequests != 1 || q.ObservedRequests != 1 || q.PromptTokens != 5 || q.CompletionTokens != 4 || q.TotalTokens != 9 {
				t.Fatalf("late/cumulative usage miscounted: %+v", q)
			}
		})
	}
}

func TestPassiveSSERejectsIncompleteMalformedAndOversizedObservations(t *testing.T) {
	for _, body := range []string{
		"data:{\"choices\":[],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":4,\"total_tokens\":9}}\n\n",
		strings.TrimSuffix(streamEnd, "\n\n"),
		"data:garbage\n\n" + streamEnd,
		"data:{\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":0}}\n\ndata:[DONE]\n\n",
		"data:{\"choices\":[{\"index\":0,\"delta\":{\"content\":5}}]}\n\n" + streamEnd,
		"data:{\"choices\":[{\"index\":0,\"delta\":\"bad\",\"finish_reason\":\"stop\"}]}\n\ndata:[DONE]\n\n",
		strings.TrimSuffix(streamEnd, "data:[DONE]\n\n") + "data:{\"choices\":[{\"index\":0,\"delta\":{\"content\":\"late\"}}]}\n\ndata:[DONE]\n\n",
		"data:{\"choices\":[{\"index\":0,\"delta\":{\"content\":\"" + strings.Repeat("x", (1<<20)+1) + "\"}}]}\n\n" + streamEnd,
	} {
		c := fixtureClient(io.NopCloser(strings.NewReader(body)), "text/event-stream")
		resp, err := c.ProxyChat(context.Background(), strings.NewReader(`{"stream":true}`))
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || string(got) != body {
			t.Fatalf("invalid observation altered forwarding: %v", err)
		}
		q := c.UsageTracker().SnapshotQuota()
		if q.Status == "available" || q.MeasuredRequests != 0 || q.UnmeasuredRequests != 1 {
			t.Fatalf("partial stream became available/measured: %+v", q)
		}
	}
}

func TestPassiveOversizeJSONStillPassesThrough(t *testing.T) {
	body := `{"private":"` + strings.Repeat("x", (1<<20)+2) + `","error":{"code":"budget_exceeded"}}`
	c := fixtureClient(io.NopCloser(strings.NewReader(body)), "application/json")
	resp, err := c.ProxyChat(context.Background(), strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || string(got) != body || resp.StatusCode != 200 {
		t.Fatalf("large response altered: %v status=%d", err, resp.StatusCode)
	}
	q := c.UsageTracker().SnapshotQuota()
	if q.Status != "unknown" || q.MeasuredRequests != 0 || q.UnmeasuredRequests != 1 {
		t.Fatalf("oversize response treated as measured: %+v", q)
	}
}

func TestPassiveResetAndTokenChangeRejectInflightResponses(t *testing.T) {
	for _, reset := range []string{"clear", "token"} {
		t.Run(reset, func(t *testing.T) {
			c := fixtureClient(io.NopCloser(strings.NewReader(completeJSON)), "application/json")
			resp, err := c.ProxyChat(context.Background(), strings.NewReader(`{}`))
			if err != nil {
				t.Fatal(err)
			}
			if reset == "clear" {
				c.UsageTracker().Reset()
			} else {
				c.SetToken("synthetic-other")
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if q := c.UsageTracker().SnapshotQuota(); q.ObservedRequests != 0 || q.Status != "unknown" {
				t.Fatalf("old account/request contaminated new session: %+v", q)
			}
		})
	}
}

func TestPassiveCloseAndCancellationNeverRecordMeasuredSuccess(t *testing.T) {
	for _, kind := range []string{"close", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			body := "data:{\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"}}],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":4}}\n\n" + streamEnd
			c := fixtureClient(io.NopCloser(strings.NewReader(body)), "text/event-stream")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			resp, err := c.ProxyChat(ctx, strings.NewReader(`{"stream":true}`))
			if err != nil {
				t.Fatal(err)
			}
			if kind == "cancel" {
				cancel()
				io.Copy(io.Discard, resp.Body)
			}
			resp.Body.Close()
			resp.Body.Close()
			q := c.UsageTracker().SnapshotQuota()
			if q.ObservedRequests != 1 || q.UnmeasuredRequests != 1 || q.Status == "available" {
				t.Fatalf("partial/cancelled stream measured: %+v", q)
			}
		})
	}
}

func TestCachedCheckQuotaNeverMakesARequest(t *testing.T) {
	var calls atomic.Int32
	c := NewClient("http://synthetic.invalid", "synthetic")
	c.httpClient.Transport = observationTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("must not dispatch")
	})
	for i := 0; i < 3; i++ {
		q, err := c.CheckQuota(context.Background())
		if err != nil || q.Status != "unknown" {
			t.Fatalf("cached status: %+v %v", q, err)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("quota status sent %d requests", calls.Load())
	}
}

func TestPassiveSSEInvalidUsageCannotBeRepairedIntoMeasuredHistory(t *testing.T) {
	for _, badUsage := range []string{
		`{"prompt_tokens":-1,"completion_tokens":2,"total_tokens":1}`,
		`{"prompt_tokens":5,"completion_tokens":2,"total_tokens":6}`,
		`{"prompt_tokens":5,"completion_tokens":"2","total_tokens":7}`,
	} {
		body := "data:{\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"}}],\"usage\":" + badUsage + "}\n\n" +
			"data:{\"choices\":[],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":4,\"total_tokens\":9}}\n\n" + streamEnd
		c := fixtureClient(io.NopCloser(strings.NewReader(body)), "text/event-stream")
		resp, err := c.ProxyChat(context.Background(), strings.NewReader(`{"stream":true}`))
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if q := c.UsageTracker().SnapshotQuota(); q.MeasuredRequests != 0 || q.UnmeasuredRequests != 1 {
			t.Fatalf("invalid snapshot was repaired into fake history: %+v", q)
		}
	}
}

func TestPassiveSSERetainsExplicitBudgetErrors(t *testing.T) {
	body := "event:error\r\ndata:{\"error\":{\"code\":\"budget_exceeded\",\"message\":\"private detail\"}}\r\n\r\n"
	c := fixtureClient(io.NopCloser(fragmentedReader{strings.NewReader(body)}), "text/event-stream")
	resp, err := c.ProxyChat(context.Background(), strings.NewReader(`{"stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || string(got) != body || resp.StatusCode != 200 {
		t.Fatal("late streaming error changed public response")
	}
	if q := c.UsageTracker().SnapshotQuota(); q.Status != "exhausted" || q.UnmeasuredRequests != 1 {
		t.Fatalf("budget SSE error lost: %+v", q)
	}
}

type failingObservationBody struct {
	io.Reader
	err error
}

func (b failingObservationBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	if err == io.EOF {
		return n, b.err
	}
	return n, err
}
func (b failingObservationBody) Close() error { return nil }

func TestPassivePreservesBodyReadFailure(t *testing.T) {
	want := errors.New("synthetic read failure")
	for _, contentType := range []string{"application/json", "text/event-stream"} {
		body := completeJSON
		if contentType == "text/event-stream" {
			body = streamEnd
		}
		c := fixtureClient(failingObservationBody{strings.NewReader(body), want}, contentType)
		resp, err := c.ProxyChat(context.Background(), strings.NewReader(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if !errors.Is(err, want) || string(got) != body {
			t.Fatalf("read failure/bytes lost: %q %v", got, err)
		}
		if q := c.UsageTracker().SnapshotQuota(); q.MeasuredRequests != 0 || q.Status == "available" {
			t.Fatalf("failed read became measured success: %+v", q)
		}
	}
}

func TestPassiveClosingAfterTerminalSSEFrameRetainsMeasuredCompletion(t *testing.T) {
	body := "data:{\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"}}],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":4}}\n\n" + streamEnd
	c := fixtureClient(io.NopCloser(strings.NewReader(body)), "text/event-stream")
	resp, err := c.ProxyChat(context.Background(), strings.NewReader(`{"stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	// The Messages adapter's SSE decoder stops at DONE and closes without an
	// additional read to discover HTTP EOF. Terminal framing is sufficient.
	buf := make([]byte, len(body))
	if _, err := io.ReadFull(resp.Body, buf); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if q := c.UsageTracker().SnapshotQuota(); q.Status != "available" || q.MeasuredRequests != 1 || q.TotalTokens != 9 {
		t.Fatalf("terminal stream lost on ordinary close: %+v", q)
	}
}

func TestPassiveCleanSSEEOFAfterFinishIsCompleteButEarlyCloseIsNot(t *testing.T) {
	body := "data:{\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"}}],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":4}}\n\n" + strings.TrimSuffix(streamEnd, "data:[DONE]\n\n")
	for _, earlyClose := range []bool{false, true} {
		c := fixtureClient(io.NopCloser(strings.NewReader(body)), "text/event-stream")
		resp, err := c.ProxyChat(context.Background(), strings.NewReader(`{"stream":true}`))
		if err != nil {
			t.Fatal(err)
		}
		if earlyClose {
			_, err = io.ReadFull(resp.Body, make([]byte, len(body)))
		} else {
			_, err = io.ReadAll(resp.Body)
		}
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		q := c.UsageTracker().SnapshotQuota()
		if earlyClose {
			if q.MeasuredRequests != 0 || q.Status == "available" {
				t.Fatalf("early close guessed stream complete: %+v", q)
			}
		} else if q.MeasuredRequests != 1 || q.Status != "available" || q.TotalTokens != 9 {
			t.Fatalf("clean EOF lost completed observation: %+v", q)
		}
	}
}

func TestPassiveMeasuredZeroTokenEmptySSECompletion(t *testing.T) {
	body := "data:{\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"\"}}],\"usage\":{\"prompt_tokens\":0,\"completion_tokens\":0}}\n\n" + streamEnd
	c := fixtureClient(io.NopCloser(strings.NewReader(body)), "text/event-stream")
	resp, err := c.ProxyChat(context.Background(), strings.NewReader(`{"stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if q := c.UsageTracker().SnapshotQuota(); q.Status != "available" || q.MeasuredRequests != 1 || q.TotalTokens != 0 {
		t.Fatalf("valid measured zero confused with missing output/usage: %+v", q)
	}
}

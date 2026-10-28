# Passive Quota Implementation Plan

> **For agentic workers:** Use Superpowers implementation, TDD and review skills
> task by task. User authorized execution of the agreed four-step design.

**Goal:** Integrate newer main and deliver a passive, honest local quota/usage view.
**Architecture:** Shared bounded per-client observation feeds protected web and CLI
reads. Preserve verified protocol/auth/config behavior during integration.
**Tech Stack:** Go standard library, embedded HTML/CSS/JavaScript; no new dependency.
**Spec:** `docs/superpowers/specs/2026-10-28-passive-quota-design.md`.

## Global constraints

- Work only on `codex/passive-quota-checker`, based on published `bf9ad72`.
- No live inference, credentials in logs/artifacts, real settings writes or daemon restart.
- Keep user data local, no automatic persistence or synthetic quota estimates.
- Status and refresh send zero upstream requests; exact remaining/reset remain null.
- Preserve buffered Anthropic validation and incremental OpenAI forwarding.

## Review focus

Cancelled bodies, late SSE usage and cumulative duplicates must not fabricate
counts; account changes/reset must reject stale responses; large replies must
keep flowing while observation is bounded; new management routes must retain
owner/Host/Origin protection; merged update/analytics features must not restore
older unsafe protocol/config/process behavior.

### Task 1: Reconcile main

- [x] Fresh baseline `go test ./... -count=1` before integration.
- [x] Create branch from selected published Codex branch; carry research record.
- [ ] Merge `origin/main` at `d8e4760`, resolve conflicts with verified behavior.
- [ ] Run Go tests, Node/dashboard checks; record any deliberate integration choices.

### Task 2: Shared passive observation

Files: `internal/analytics/{types,tracker,summary,events,observer}.go` and tests;
`internal/valclient/client.go`, `chat_response.go` and tests; protocol tag in Messages.

Interface used by web/CLI:
`Client.UsageTracker() *analytics.Tracker`, `Tracker.SnapshotQuota() QuotaStatus`,
`Tracker.GetSummary() AnalyticsSummary`, `Tracker.Reset()`, Subscribe/Unsubscribe.
Quota JSON: status/message/source, observed_at, observed_requests,
measured_requests/unmeasured_requests, prompt_tokens/completion_tokens/total_tokens,
remaining/limit/reset_at (null when unknown). Source is `gateway_session`.
Analytics retains existing field names, adds measurement counts and per-record
`usage_known`; missing per-record counts serialize null.

- [ ] Write and watch failing tests for valid/invalid JSON, measured zero/missing
  usage, HTTP 200 budget errors, 401/403/429/5xx and malformed responses.
- [ ] Implement bounded observation in the shared client without extra requests.
- [ ] Prove fragmented streaming, late and duplicate usage, partial/cancelled
  bodies, byte preservation, oversize fallback, reset and token-change isolation.
- [ ] Run targeted tests and race checks for analytics/Val client.

### Task 3: Web and CLI

Files: `internal/web/api_routes.go`, static assets, tests; `cmd_quota.go`, main/help.

- [ ] Regression tests: GET quota/analytics causes zero fake-upstream calls;
  methods/auth/origins enforced; DELETE clears cache without touching user files.
- [ ] Route all views to `client.UsageTracker()`; preserve live analytics SSE.
- [ ] Replace probe/reset countdown with observed status, unknown remaining/reset,
  session scope, measured/unmeasured counts, refresh and explicit clear control.
- [ ] Preserve owner authentication on new live requests and tab navigation.
- [ ] CLI reads the authenticated local gateway only, with bounded body/time and
  no redirects; test offline/unconfigured states with isolated fixtures.
- [ ] Check JS syntax and Node dashboard tests; inspect desktop/mobile fixtures.

### Task 4: Review and delivery

- [ ] Go suite/vet/build, targeted race tests, appropriate Python/SDK regressions.
- [ ] Independent branch review and Codex Security assessment of immutable patch.
- [ ] Resolve important findings with failing/passing regression evidence.
- [ ] Refresh Graphify offline; update README/context and completion record.
- [ ] Commit coherent changes; report branch, checks and remaining coverage limits.

## Decisions and evidence ledger

- User's latest instruction supersedes the earlier request to stay on the
  existing branch: new branch is based on `codex/agent-compatibility-readme`.
- Design details expand the previously approved four steps; routine choices use
  existing conventions. Further repetitive approval gates are not introduced.
- Research commit `4b091c2` was carried forward as `c4395cd` (documentation only).
- Baseline Go suite passed all existing packages. No app source has changed yet.

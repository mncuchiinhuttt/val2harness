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
- [x] Merge `origin/main` at `d8e4760`, resolve conflicts with verified behavior.
- [x] Run Go tests, Node/dashboard checks; record any deliberate integration choices.

### Task 2: Shared passive observation

Files: `internal/analytics/{types,tracker,summary,events}.go` and tests;
`internal/valclient/client.go`, `chat_response.go` and tests; protocol tag in Messages.

Interface used by web/CLI:
`Client.UsageTracker() *analytics.Tracker`, `Tracker.SnapshotQuota() QuotaStatus`,
`Tracker.GetSummary() AnalyticsSummary`, `Tracker.Reset()`, Subscribe/Unsubscribe.
Quota JSON: status/message/source, observed_at, observed_requests,
measured_requests/unmeasured_requests, prompt_tokens/completion_tokens/total_tokens,
remaining/limit/reset_at (null when unknown). Source is `gateway_session`.
Analytics retains existing field names, adds measurement counts and per-record
`usage_known`; missing per-record counts serialize null.

- [x] Write and watch failing tests for valid/invalid JSON, measured zero/missing
  usage, HTTP 200 budget errors, 401/403/429/5xx and malformed responses.
- [x] Implement bounded observation in the shared client without extra requests.
- [x] Prove fragmented streaming, late and duplicate usage, partial/cancelled
  bodies, byte preservation, oversize fallback, reset and token-change isolation.
- [x] Run targeted tests and race checks for analytics/Val client.

### Task 3: Web and CLI

Files: `internal/web/api_routes.go`, static assets, tests; `cmd_quota.go`, main/help.

- [x] Regression tests: GET quota/analytics causes zero fake-upstream calls;
  methods/auth/origins enforced; DELETE clears cache without touching user files.
- [x] Route all views to `client.UsageTracker()`; preserve live analytics SSE.
- [x] Replace probe/reset countdown with observed status, unknown remaining/reset,
  session scope, measured/unmeasured counts, refresh and explicit clear control.
- [x] Preserve owner authentication on new live requests and tab navigation.
- [x] CLI reads the authenticated local gateway only, with bounded body/time and
  no redirects; test offline/unconfigured states with isolated fixtures.
- [x] Check JS syntax and Node dashboard tests; inspect desktop/mobile fixtures.

### Task 4: Review and delivery

- [x] Go suite/vet/build, targeted race tests, appropriate Python/SDK regressions.
- [x] Primary Codex Security assessment of immutable patch; JSON validator passed.
- [ ] Fresh independent branch review: reviewer failed immediately due account
  usage limit. Not completed; human review remains required before merging.
- [x] Resolve identified important issues with failing/passing regression evidence.
- [x] Refresh Graphify offline; update README/context and completion record.
- [x] Commit coherent implementation changes; record branch, checks and remaining
  coverage limits. Publication/final handoff follow this checkpoint.

## Decisions and evidence ledger

- User's latest instruction supersedes the earlier request to stay on the
  existing branch: new branch is based on `codex/agent-compatibility-readme`.
- Design details expand the previously approved four steps; routine choices use
  existing conventions. Further repetitive approval gates are not introduced.
- Research commit `4b091c2` was carried forward as `c4395cd` (documentation only).
- Baseline Go suite passed all existing packages before integration.
- Reconciliation retains verified daemon/export/config/protocol implementations;
  main's overlapping safety helpers are superseded. Its unused `safeio` package
  is omitted (no production callers; Windows permission test assumed Unix modes).
- OMP adds a single safely selected `agent/models.yml` location; ambiguous files
  are refused. Unsupported 1M context and invented zero-cost claims are omitted.
- New port configuration must preserve live immutable config and update only the
  saved address; subsequent token updates must preserve that staged address.
- The merged updater is bounded and checksum-verified before it can replace an
  executable. No real update/download was run during verification.
- One initial negative-method web test reached the public GitHub release metadata
  API before its guard was fixed. No Val call or executable download/update ran;
  tests now block non-loopback HTTP so this cannot silently recur.
- CLI tests: expected RED (`readQuota` missing), then GREEN for authenticated
  cache reads, no redirects/nonlocal credentials, bounds, cancellation and null
  unknown reset/remaining. Local `version` now prints without a network request.
- Source checkpoint `60907a5`: all 13 tested Go packages, vet, Windows build,
  official OpenAI/Anthropic offline SDK checks and desktop/mobile browser checks
  passed. Six selected Ubuntu packages passed natively; Darwin arm64 build-only.
- Windows race checks passed for analytics/client/proxy/web/version at `04a1d32`;
  Go files are unchanged at `60907a5`. The later change fixes update UI copy and
  adds its Node regression (observed RED, then GREEN).
- Immutable final patch SHA256:
  `a19ba7735fce133fb18a67d62b4b7c8488bdb440769c36a76226fc0fdb5e926e`.
  Codex Security recommends human review, with partial protection and moderate
  confidence. No completed final-tree native scan or independent review claimed.
- See `docs/PASSIVE_QUOTA_CHECK.md` for user-visible behavior, complete checks,
  privacy, recovery and remaining release acceptance.
- Offline Graphify refresh: 1,025 nodes, 3,415 edges, 66 communities; no external
  AI or query logging. Receiver edges and semantic documentation remain partial.

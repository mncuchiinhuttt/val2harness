# Passive quota status and integration

The user approved the four-step proposal in chat: reconcile newer main with the
verified fixes, observe normal requests, show honest quota status, and verify
locally. Latest branch instruction: create a new branch based on
`codex/agent-compatibility-readme`. Work is on `codex/passive-quota-checker` from
`bf9ad72`, with the REA research checkpoint carried forward. Main is never a
push or merge destination for this task.

## Result and approach

Keep the Go gateway, CLI, dashboard, verified protocol adapters and owner-auth
boundary. Integrate remote main `d8e4760` without replacing the tested fixes
with older implementations. Retain its useful analytics, heatmap, tab navigation,
version/update controls and harness improvements where compatible.

The selected first checker is passive observation. Authoritative account quota
metadata would be preferable, but the investigated self-account response did
not provide that contract. Automatic inference probes spend quota and cannot
measure remaining budget, so no probe is included. See REA_QUOTA_RESEARCH.md.

## Shared behavior

One concurrency-safe tracker per Val client observes responses for both API
protocols. It stores measured counters and bounded recent metadata in memory,
never prompts, responses, tool arguments, credentials or account identifiers.
It does not load or append main's legacy estimated usage history. Restarting,
changing the Val token or explicitly clearing observations starts fresh. Old
in-flight responses cannot repopulate a cleared account/session generation.

States: `unknown`, `available`, `exhausted`, `rate_limited`,
`authentication_error`, `provider_error`. Available means a valid completion was
observed at the displayed time, not a promise about the next request. Distinguish
HTTP authentication failures, transient rate limits, provider failures, malformed
responses and explicit budget errors. Never default arbitrary replies to success.

Track completed observed attempts, measured/unmeasured request counts and measured
prompt/completion/total tokens. Late SSE usage is included once; cumulative
snapshots are not summed. Partial/cancelled streams and missing/invalid usage
remain unmeasured. Observation is bounded and does not alter forwarded bytes or
break incremental OpenAI streaming. An exact HTTP 200 JSON `budget_exceeded`
envelope is normalized to HTTP 429 centrally; unrelated status/body/Retry-After
behavior remains intact. Unsupported or oversized observation data yields unknown
measurement while the original response still reaches the consumer.

GET `/api/local/quota` reads memory only. GET `/api/local/analytics` and its live
stream use the same tracker. DELETE `/api/local/analytics` clears observations
under existing owner/Host/Origin checks. Remaining amount, account limit and reset
timestamp are null until authoritative evidence exists. No 4 AM clock, percentage,
price weights, synthetic token estimates or inference on page/status load.

The dashboard labels gateway-session scope, observation time and missing usage;
provides refresh and clear controls; keeps the heatmap and measured analytics.
The `quota` CLI reads the running gateway's protected local route and makes no
Val requests. Disabled/unavailable gateway status is explicit. No persistent
history or additional service/dependency is introduced.

## Acceptance and operation

Fake upstreams cover JSON and fragmented SSE, late/duplicate usage, malformed
replies, budget-vs-rate errors, auth errors, cancellation, bounds, concurrent
reads/token changes/reset, owner/Origin denial and zero upstream calls from
status/refresh. Preserve existing Go and SDK/CLI regression behavior. Test CLI
with isolated configuration and a fake local gateway. Preview UI with fixtures.

Use Codex Security's supported local diff/patch workflow on immutable final
source, preserve native evidence and state any blocked preflight honestly.
No new live inference, no real harness/config writes and no daemon replacement.
Review before release; exact account-wide quota remains an explicit limitation.

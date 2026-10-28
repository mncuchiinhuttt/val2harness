# Sonnet tool/image diagnosis — 26 October 2026

Branch `codex/preserve-config-owner-auth`, base
`6999a37c298137e386567dfabee38d8c9caf83d0`; changes remain local and uncommitted.
This checkpoint supersedes the earlier blocked-tool status in
[LIVE_VAL_CHECK.md](LIVE_VAL_CHECK.md), while retaining its historical ledger.

The later [OpenAI/editing checkpoint](OPENAI_EDIT_CHECK.md) supersedes the remaining
raw OpenAI registration gap and adds live Edit execution evidence. This document
retains the earlier 26 October ledger and its explicitly scoped verification.

## Current result

Sonnet `anthropic-claude-sonnet-4-6` completed a live JSON client-tool/result
loop and a live streaming, image-assisted automatic tool/result loop through
the Go gateway and official Anthropic SDK **2.3.0**, with strict SDK response
validation. The image was a generated red PNG; its color was not disclosed in
the tool schema. The gateway returned the declared call, accepted the synthetic
tool result and delivered the final acknowledgement with measured usage.

After correcting effort translation, live Claude Code **2.1.310** completed a
scoped Read -> tool result -> final reply loop. It returned the random marker
stored only in a disposable fixture, using exactly two streaming Val requests
with no retry. Earlier failed CLI probes are retained below. This establishes
the selected Read flow, not every Claude Code tool or full agent compatibility.

Only temporary test binaries were built. The user's running workspace executable
and persistent account/harness configuration have not been replaced by this work.

## Causes and corrections

### Client schemas were missing from Val's native registry

Ordinary OpenAI `tools` definitions alone did not preserve the requested client
registry on the observed Val path: Sonnet returned built-in search/knowledge
tools instead. Supplying the same flat function definitions through Val's
client-schema channel restored the declared calls:

```json
{"tool_servers": [{"specs": ["same validated function objects as tools"]}]}
```

The example shows the wrapper shape; `specs` contains actual function objects,
not strings. `messages_request.go:convertTools` now builds both representations
from the same validated objects. The wrapper has only `specs`: no URL, endpoint,
headers, credentials, chat/session identity or server-execution configuration.
Schemas go to the same configured upstream twice, increasing payload size.
The consuming harness still owns tool execution. The gateway continues rejecting
undeclared names, incomplete arguments, invalid identities and choice violations.

The controlled live experiment establishes the working registration path.
Related first-party [Open WebUI v0.8.2 middleware](https://raw.githubusercontent.com/open-webui/open-webui/v0.8.2/backend/open_webui/utils/middleware.py)
shows native tool resolution collecting `tool_servers.specs` and replacing the
ordinary tools array. Its [mode selection](https://raw.githubusercontent.com/open-webui/open-webui/v0.8.2/backend/open_webui/main.py)
also lets model-native configuration win over a request's `default` mode.
[PR 21555](https://github.com/open-webui/open-webui/pull/21555.diff) changes caller-tool
handling. These explain a related implementation; they do not establish Val's
exact deployed source revision. Public sources were retrieved with Firecrawl on
26 October 2026; Val's read-only metadata reports version `26.9`.

An earlier candidate that automatically imposed `tools: []` and
`tool_choice: none` received live HTTP 400 and was fully reverted. Empty tool
lists retain their previous omission behavior; the regression prevents silently
reintroducing that unsupported override.

### An empty stream tail was mistaken for new content

Val emitted `finish_reason`, then an empty assistant role/content envelope, then
measured usage. The adapter treated a non-null empty string as continued content
and rejected an otherwise complete tool stream with HTTP 502.

`messages_stream.go` now permits an empty content tail. It still rejects
nonempty text, new tool fragments, refusal fields, repeated finish markers,
identity changes and unsupported reasoning. Complete usage and declared-tool
validation still precede executable output. The meaningful distinction is whether
the tail adds content, rather than whether a content field exists.

### Claude Code effort was translated to a different control

Claude Code's isolated requests included `output_config.effort: high`. The adapter
mapped it to upstream `reasoning_effort`, although the client disabled thinking.
Anthropic's [official effort documentation](https://platform.claude.com/docs/en/build-with-claude/effort)
describes effort as a whole-response control that also works without thinking;
it is not an instruction to enable thinking.

The adapter now validates supported effort values, omits the incompatible
translation, and discloses `response-effort-hint-not-forwarded` in
`X-Val2harness-Compatibility`. JSON-schema output formatting remains forwarded.
Extended/adaptive thinking and unsigned reasoning responses remain explicitly
unsupported; no signatures or thinking content are fabricated. A regression
covers JSON/SSE with thinking omitted and explicitly disabled.

The separately authorized final live check passed after this correction. Its
two upstream streams contained no reasoning fields and completed the declared
Read/result/final-marker flow, supporting the effort translation as the cause
of that observed CLI failure. This does not establish native signed thinking.

## Authorized live ledger

The user approved **12 additional inference attempts total across runs**, at most
128 requested output tokens each, without retries or account-setting changes.
All 12 were used; combined with the previous nine, **21 historical attempts**
have been made. HTTP denials count as attempts, not necessarily billed inference.
The user then separately authorized two final attempts; their results follow.

| Additional attempts | Observation |
| --- | --- |
| 1 | Automatic no-tools/none candidate: HTTP 400; candidate reverted. |
| 2 | Forced named tool without client-schema registration: HTTP 400. |
| 3–4 | Diagnostic URL-free schema registration: named tool/result loop passed. |
| 5–6 | Production Go registration, recorder mutation off: required tool/result loop passed. |
| 7–8 | Streaming call failed with HTTP 502; structural recording isolated the empty post-finish envelope. |
| 9–10 | Fixed streaming path: automatic image-assisted tool/result loop passed. Measured prompt/completion usage: 1,094/77, then 1,187/29. |
| 11–12 | Actual Claude Code scoped Read probe failed/timed out. Its own automatic retry consumed the second attempt; the recorder maintained the overall ceiling. Reasoning fields were present. |

The CLI retry was unintended and is recorded as a test-harness error. The helper
now sets `CLAUDE_CODE_MAX_RETRIES=0`, consistent with the
[official Agent SDK retry configuration](https://code.claude.com/docs/en/agent-sdk/typescript#handle-slow-or-stalled-api-responses).
The recorder additionally rejects duplicate canonical JSON payloads before
forwarding, counts failed upstream attempts, and returns a permanent local HTTP
400 on duplication or budget exhaustion. A different tool-result turn can still
proceed within budget. These controls were exercised against a fake upstream;
actual CLI failure acceptance confirmed one POST with no retry.

### Final two-attempt authorization

Both attempts passed through the compiled current gateway, with recorder schema
mutation off. Claude Code had only the path-scoped Read permission, an isolated
profile, `--bare`, no MCP servers, disabled thinking and zero retries. The first
Val stream requested Read (measured prompt/completion **1,365/89**); the second
processed its result and returned the exact random fixture marker (**1,491/28**).
CLI exit status was zero; the recorder counted exactly two inference requests.
No reasoning fields appeared in either stream. **23 historical attempts** have
now been used across four explicitly authorized batches; no further live
inference is authorized. Counts include denials and do not establish billing.

The real credential stayed inside local processes and was never printed or sent
to research tools. Inputs were fixed synthetic text, tiny schemas and generated
images. No repository files or real conversations were submitted, and no
account/model settings or real harness configuration were changed. These tests
cannot establish upstream logging or retention.

## Verification and security scope

Red/green regressions were observed for native client registration, empty stream
tails, the effort translation, and recorder duplicate/budget denial. The focused
effort tests passed after the latest correction, as did all **nine** Python
recorder tests. Fresh `go test ./... -count=1`, `go vet ./...` and
`go test -race ./internal/proxy ./internal/valclient -count=1` passed. The current
Windows binary built; the proxy test binary passed natively under Ubuntu/WSL.
Dashboard Node checks, Python syntax and JavaScript syntax checks passed.

Actual Claude Code fake-upstream Read acceptance also passed with two streaming
requests and no reasoning-effort field. A separate fake HTTP 502 check failed
promptly after exactly one POST and zero GETs, confirming no retry for that case.
The initial fresh Read/Write fixture exposed two helper bugs: `--bare` removes
Write from the CLI's tool inventory, and `Write(path)` rules never authorize file
modifications. The [official permission documentation](https://code.claude.com/docs/en/permissions#path-rules-that-never-match)
requires `Edit(path)` for that permission. The helper now omits `--bare` only for
the known synthetic localhost Go fixture and uses `Edit(./result.txt)`, with the
exposed tools still limited to Read and Write. Non-synthetic keys and non-loopback
URLs fail before CLI setup. Isolated home, empty setting sources, strict empty
MCP, zero retries and file permissions remain in force. Live Read keeps `--bare`.

The corrected fake-Val Read/Write check passed: **three requests/turns**, exact
expected file bytes, both tool results returned, no reasoning-effort fields and
zero permission denials. Its Go fixture stopped cleanly. This is fresh offline
acceptance, not a live Val Write check.

The completed native Codex Security Standard scan
`92903b28-d9ef-4127-82cc-9d92be491088` reviewed all **16** `internal/proxy` files
and reported **zero findings**. Its original generated report is retained locally;
the [portable summary](SECURITY_REVIEW.md#scoped-proxy-scan--26-october-2026)
records the scope. The sealed snapshot **predates these patches**;
subsequent independent static patch review is supplementary, not a completed
scan of the final tree or a repository-wide security guarantee.

Internal plugin usage measurements remain in the local reports and are not
presented as Val quota or billing figures.

## Remaining work

- Retain the live Read acceptance and corrected offline Read/Write acceptance;
  live editing and other Claude Code features need separate evidence.
- Graphify refresh and clustering completed offline: **703 nodes, 2,225 edges,
  34 communities**. Independent static review of registration, stream and
  effort/recorder changes found no actionable issue; it did not rerun the native
  scan against the final tree.
- The new schema registration currently applies to Anthropic Messages.
  OpenAI Chat Completions remains raw pass-through and does not add this channel;
  equivalent Sonnet tools through that API remain unverified.
- Standalone Sonnet requests without client tools, native signed thinking,
  broader harness/version coverage and release installation remain unverified
  or unsupported. Refresh the compatibility matrix as evidence changes.

Supplemental evidence saved through the Security plugin:
`artifacts/validation/2026-10-26-sonnet-correction.md`, SHA-256
`bd69eef4023c384fb6c42af539c4889d6dd703df5227fc2b168413f7e4db98eb`.
The later helper verification is retained separately as
`artifacts/validation/2026-10-26-sonnet-cli-helper.md`, SHA-256
`422c00d432c10b69727761961e44e14dc8f89c2bd110e381c0512940dc42dd5c`.

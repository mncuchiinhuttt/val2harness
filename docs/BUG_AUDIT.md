# Initial bug audit

Date: 26 October 2026. Application source baseline:
`6999a37c298137e386567dfabee38d8c9caf83d0` (`main`).

Historical baseline: this document retains the original diagnosis and sealed
security evidence. The subsequent configuration/authentication patch is recorded
in [CONFIG_AUTH_FIX.md](CONFIG_AUTH_FIX.md), and the runtime/protocol corrections
in [RUNTIME_FIX.md](RUNTIME_FIX.md). The latest
[AGENT_COMPATIBILITY.md](AGENT_COMPATIBILITY.md), [DAEMON_FIX.md](DAEMON_FIX.md)
and [EXPORT_FIX.md](EXPORT_FIX.md) record the tool/image, lifecycle and export
continuation, tested clients/platforms and the blocked native diff-scan launch.
Statements below about unchanged
source, absent repository tests, and untested Unix permissions describe the audit
snapshot, not that later working tree.

This is a diagnosis, not a set of fixes or a release certification. No application
source was changed. The application is open source; credentials, conversations,
settings, logs, and exports belong to the user. Testing used synthetic tokens,
fake upstreams, temporary homes, and a disposable source copy. No real Val account,
conversation history, or personal harness settings were accessed.

## Results and evidence

- **17 reproducible correctness/robustness issue groups**, detailed below.
- **2 medium-severity security findings** in the completed Codex Security report.
- All 25 Go files, embedded web assets, installers, and supporting first-party
  documentation received source review. An independent security baseline and a
  focused boundary review corroborated the security findings; an independent
  architecture review mapped actual resource consumers and startup paths.
- The original `go test ./...` and `go vet ./...` pass, but the original repository
  has no tests. `node --check internal/web/static/app.js` also passes.
- A diagnostic suite in the disposable copy ran **24 top-level Go tests: 21 failed
  and 3 passed**. These are deliberately failing contract checks against the
  unchanged program, not 21 distinct bugs. Several tests cover the same root cause.
- A separate `go test -race -run '^TestRaceTokenRefresh$'` failed with a confirmed
  data race in `Client.SetToken`/`GetToken`.
- A Node VM/DOM stub confirmed four unbound dashboard controls and propagation of
  a model identifier into `innerHTML`. It did not execute injected browser code.

Codex Security scan: `37217ea0-663e-4ce5-9540-f40ea7040e53`.

The generated report, threat model, coverage JSON, diagnostic sources and raw
execution logs are retained locally. Their original machine paths are omitted
from this public record. See the [portable security review summary](SECURITY_REVIEW.md)
for scope and outcomes. Review/redact diagnostics before any separate sharing;
future runs must not be assumed to contain only synthetic data.

## Security findings

The generated report is authoritative for severity, source snippets, attack
paths, counterevidence, and remediation. This engineering record summarizes it:

| ID | Finding | Evidence and limits |
| --- | --- | --- |
| SEC-01 | Default gateway allows reachable unrelated local callers to use owner account/management authority | `internal/proxy/server.go:48-69` permits every Origin and skips authentication for the default empty APIKey. Synthetic foreign-Origin token/model requests and GET logout succeeded. A secret configured key blocks actual requests; default loopback prevents ordinary direct remote access. Browser delivery is conditional on browser network protections. |
| SEC-02 | New harness backups can expose private provider credentials on Unix | All four adapters create whole-file `.bak` copies with mode `0644`; new config and recreated restore targets share the mode. Requires secret-bearing data, newly created files, traversable parents, and a permissive Unix umask. Source-validated; no Unix runtime or Windows ACL disclosure claim. |

## Reproduced correctness issues

P1 means fix before recommending real-world agent use because of data loss,
unreliable core behavior, or broken lifecycle/control. P2 means a concrete
correctness or usability issue. These engineering priorities are separate from
the security report's severity ratings. Every entry below has a direct diagnostic
reproduction; wider consequences are labeled when inferred from source.

### BUG-01 — P1: Sync overwrites malformed existing configuration

`internal/sync/pi.go:38-42`, `opencode.go:41-45`, and `continue.go:35-39` ignore
deserialization errors and continue serializing a new map over the original.
`TestMalformedHarnessConfigPreserved` supplied malformed JSON/YAML separately to
all three adapters: each returned no error and replaced the original bytes.
OMP's parser does return an error for malformed YAML, providing a useful working
comparison. **Acceptance:** failed parsing must leave the original untouched,
report an actionable error, and preserve a usable original backup.

### BUG-02 — P1: A second sync destroys the original recovery baseline

All four adapters overwrite the same `.bak` on every run. In
`TestRepeatedSyncRestorePreservesOriginal`, two Pi syncs followed by restore left
the injected `rmit-val` provider in place: the second sync backed up the already
modified file. **Acceptance:** define whether backups mean original baseline or
latest version, implement that consistently, and prove repeated sync/restore
preserves unrelated user settings and can undo the integration as advertised.

### BUG-03 — P1: Anthropic requests lose tool/content and token-limit information

`internal/proxy/messages.go:19-26,46-65,90-105` decodes a narrow request and flattens
content to text. `TestAnthropicPreservesRequestContract` confirmed `tools` and
`max_tokens` disappear; `tool_use` and `tool_result` become empty strings; an image
block is also omitted from the captured request. A text-chat response therefore
does not establish a working agent tool loop. **Acceptance:** implement the
supported contract or return explicit unsupported-feature errors; never silently
discard content or resource limits. Verify a full tool-use/result round trip.

### BUG-04 — P1: Anthropic adapter turns upstream errors into successful output

`internal/proxy/messages.go:107-137,141-191` does not check upstream HTTP status
and ignores response-decoding errors. `TestAnthropicPreservesUpstreamErrors`
received upstream `401` but local `200`, both for JSON and streaming responses;
the stream ended normally with no answer. OpenAI pass-through correctly preserved
an upstream `429` in the control test. **Acceptance:** preserve meaningful errors
and failure status before starting output; signal stream failures after output
starts rather than fabricating normal completion.

### BUG-05 — P1: Token refresh has a data race

`internal/valclient/client.go:30-35` writes and reads the same token without shared
synchronization. The server's update mutex does not protect these client reads.
`TestRaceTokenRefresh` produced a race detector report pointing to lines 31 and
35. HTTP client operations also read this field directly. **Acceptance:** one
consistent synchronization strategy covers every read/write, and concurrent
refresh plus request/status tests pass under the race detector.

### BUG-06 — P1: Enabling local authentication breaks supported client flows

The global gate in `internal/proxy/server.go:60-66` requires a Bearer header even
for dashboard HTML/assets and health checks. `TestCustomKeyKeepsDashboardUsable`
received `401` for `/`, `/static/app.js`, and `/healthz`. The daemon health client
and dashboard requests do not send that header; the shell generator uses a fixed
placeholder rather than the configured key. `TestAnthropicXAPIKeyAuthentication`
also showed that providing the correct key through `x-api-key` is rejected.
**Acceptance:** retain owner authentication while providing working browser,
health, shell, and supported harness credential flows. Do not solve this by simply
removing protection from management operations.

### BUG-07 — P2: Anthropic SSE parser loses valid text and reports normal completion

`internal/proxy/messages.go:154-190` requires the exact `data: ` prefix, uses the
default Scanner line limit, and does not check `scanner.Err`. The two
`TestAnthropicSSEParsesValidFrames` cases lost a no-space `data:` event and a
70,000-character event; both still emitted `message_stop`. **Acceptance:** parse
the supported SSE grammar, handle appropriately bounded large events, and expose
read/parse failures without false success. Terminal chat uses the same strict
prefix and needs corresponding coverage.

### BUG-08 — P2: Unknown requested models silently become a different model

`internal/proxy/messages.go:28-43` defaults to a fixed Opus ID.
`TestAnthropicUnknownModelNotSilentlyChanged` captured an unknown requested ID
being replaced while the response reported the requested ID. Broad substring
matching also risks unintended aliases. **Acceptance:** resolve explicit,
documented aliases against known availability or reject the request clearly;
report the model actually used.

### BUG-09 — P2: Anthropic usage numbers are invented

`internal/proxy/messages.go:136,190` supplies constant/estimated counts.
`TestAnthropicPreservesUsage` supplied upstream usage `123/7` and received `10/1`.
Streaming reports a fixed output count too. **Acceptance:** translate real usage
when available and represent unavailable accounting honestly, without fabricated
precision. Actual upstream usage support remains unverified.

### BUG-10 — P1: Windows liveness check labels a running process dead

`internal/daemon/daemon.go:33-38` relies on Unix-style signal zero.
`TestWindowsCurrentProcessRecognized` returned false for the running test process
on Windows. `GetStatus` then removes a supposedly stale PID file at lines 54-55;
source tracing shows this can lose control of a still-running daemon and cause
misleading status/restarts. No production daemon was launched to test that wider
sequence. **Acceptance:** use platform-appropriate process checks and test real
isolated start/status/stop, stale PID handling, and address/key combinations.

### BUG-11 — P2: Malformed app configuration crashes commands

`internal/config/config.go:58-62` returns a nil config on load failure, while
`commands.go` and TUI callers discard errors. `runServe` prints the error but
continues dereferencing config. `TestMalformedConfigCommandDoesNotPanic` reproduced
a nil-pointer panic through `runStatus`. **Acceptance:** commands terminate with
a useful configuration error or enter a deliberate recovery flow; do not silently
discard the user's file or panic.

### BUG-12 — P2: Token API reports success when persistence fails

`internal/web/handler.go:119-127` ignores `cfg.Save` errors and changes the in-memory
token anyway. `TestTokenSaveFailureReported` blocked the config file path with a
directory: the API still returned `200` and changed memory. Logout and TUI saves
have related unchecked errors. **Acceptance:** report persistence failure and
keep memory/disk state consistent under a documented recovery strategy.

### BUG-13 — P2: Continue sync duplicates or deletes models based on port text

`internal/sync/continue.go:47-54` recognizes existing entries using `8765` or
`rmit-val` substrings. `TestContinueCustomPortIdempotent` produced two copies after
two syncs at port 9292. `TestContinueUnrelatedProviderPreserved` lost an unrelated
service whose URL used port 8765. **Acceptance:** identify integration ownership
explicitly, retain other providers, and make repeated sync idempotent at arbitrary
supported addresses.

### BUG-14 — P2: Unvalidated export IDs panic or escape the export directory

`internal/exporter/export.go:55-66` slices the first eight ID bytes and joins them
into a filename without validating length or separators.
`TestExportShortIDDoesNotPanic` panicked on ID `x`.
`TestExportIDCannotEscapeDirectory` wrote `owned-audit.json` beside, rather than
inside, the test export folder for upstream ID `../owned12345`. All writes stayed
inside disposable test directories. **Acceptance:** validate ID shape/length and
prove output containment. Security exploitability additionally needs evidence of
a weaker-privileged actor's control over Val-issued IDs; this audit has none.

### BUG-15 — P2: Export counts failed writes as completed chats

`internal/exporter/export.go:61-70` ignores both write errors and increments the
count. `TestExportWriteFailureReported` made both output names directories:
neither file could be written, but export returned count 1 and no error.
**Acceptance:** return accurate per-chat outcomes and error details, with sensible
partial-success behavior and recovery. Do not describe an export as complete when
no output was saved.

### BUG-16 — P2: Four visible dashboard controls do nothing

`internal/web/static/index.html` presents manual Save Token, Update Port, Start
Daemon, and Stop Daemon buttons. The complete `app.js` has no matching listeners;
the Node DOM stub confirmed all four are unbound. There are no matching web
start/stop/port routes either. **Acceptance:** wire deliberate supported actions
with error/status handling or remove misleading controls until implemented. Test
the actual browser user journey, including daemon stop disconnecting its own UI.

### BUG-17 — P2: A valid empty model-list wrapper is rejected

`internal/valclient/client.go:63-73` accepts a wrapper only when `len(Data)>0`,
then tries parsing it as a bare array. `TestEmptyModelsWrapperAccepted` supplied
`{"data":[]}` and received `unexpected models response format`.
**Acceptance:** distinguish an empty authorized result from malformed data and
test both supported response shapes, including empty lists.

## Additional source observations and unresolved candidates

These are not counted among the 17 reproduction groups or promoted to extra
security findings:

- `app.js:190` and `showToast` at line 41 place model IDs/error descriptions into
  HTML. A synthetic model ID reached an HTML sink in the DOM stub. Browser code
  execution and weaker-privileged control of Val model IDs were not established.
  Prefer text rendering, but do not claim an observed attacker exploit.
- Export UI at `app.js:265-271` reports completion without checking `res.ok`;
  model counts/addresses and the copied environment snippet have hardcoded
  defaults in HTML. Port/key changes can leave displayed instructions stale.
- The playground displays a transcript but sends only the latest user message
  (`app.js:307`); it is not a conversational-context test.
- CLI `open` starts a daemon then enters the TUI (`commands.go:49-54`) without
  directly opening a browser. TUI Escape handling can block waiting for another
  byte (`internal/tui/menu.go:37-40`). Neither interaction was browser/TTY-tested.
- OpenCode sync selects a path by directory existence, while restore selects by
  backup existence; these can diverge. SyncAll unconditionally creates/writes all
  four harness locations rather than detecting installed tools.
- Daemon startup assumes the config directory exists before opening its log.
  PID state is shared across addresses, health does not establish process
  identity, and stop trusts the stored PID. Full lifecycle testing remains open.
- Login opens an upstream URL and relies on manual token transfer, not a local
  OAuth exchange. Token status decodes claims without verifying the account
  upstream; a configured token is not proof of a working session. CLI token
  writes do not update an already running daemon's in-memory token.
- Installers do not verify advertised checksums, but no attacker replacement
  path beyond the trusted HTTPS release source was established. README's Go
  prerequisite disagrees with `go.mod`. Model limits/counts and broad harness
  compatibility claims need current external documentation and live validation.

## Reproduce safely

Use a fresh disposable directory; copy the current Go module/source and embedded
assets into it, then copy the linked diagnostic Go file into that module root.
Never replace a real home/config directory or run sync/export against user data.
The tests isolate HOME/USERPROFILE internally and use only localhost fake
upstreams. They intentionally fail until their associated contracts are fixed.

```powershell
# In the disposable source copy, with the retained audit_runtime_test.go present:
go test -count=1 -v -timeout 45s .
go test -race -count=1 -run '^TestRaceTokenRefresh$' -timeout 45s .

# Run the retained UI diagnostic with this source copy as the current directory:
node '<path-to-retained-audit-ui.cjs>'
```

Environment: Windows amd64, Go 1.26.0, Node 22.23.2, Python 3.12.10, CGO enabled
with an available GCC toolchain. Actual audit commands used the already cached Go
1.26.0 executable directly with `GOPROXY=off`, `GOSUMDB=off`, and
`GOTOOLCHAIN=local` after an initial wrapper invocation refused disabled checksum
verification before testing. No dependency downloads were needed for the actual
diagnostic runs.

## Coverage and next action

This is a broad first pass over product source, not an exhaustive claim about the
entire folder, dependencies, release binaries, or production deployment. The
Codex Security coverage is explicitly partial for those exclusions. Its completion
warning says directory contents changed during the scan and results were saved
for the original snapshot; verify against current source before remediation.
Production source was kept unchanged, and the disposable copy is checked against
it before handoff.

Live Val access, real harness/version sessions, production browser behavior,
Unix permissions, Windows ACLs, upstream model/tool support, and full daemon
lifecycle were not exercised. No external feature research or fixes were started.

Recommended next increments: (1) preserve user settings and establish usable owner
authentication, (2) repair error propagation and the Anthropic tool/content
contract, (3) fix shared token state and Windows lifecycle, then (4) address the
remaining correctness issues. Select a coherent fix, retain a failing regression
case in the repository, and verify the corrected behavior before expanding scope.

## Later Sonnet compatibility correction — 26 October 2026

The initial audit above is historical. [SONNET_FIX.md](SONNET_FIX.md) records
three later validated correctness causes and their regression/live evidence:

| Cause | Impact and correction | Acceptance evidence |
| --- | --- | --- |
| Missing client schemas in Val's native tool registry | Ordinary caller tools were replaced by built-ins. Register the same validated flat functions in the URL-free client-spec channel; retain undeclared-tool rejection. | Official SDK Sonnet JSON and streaming tool/result loops passed live; streaming input included a generated red image. |
| Empty post-finish SSE envelope treated as new content | Complete tool streams failed with 502 before late usage arrived. Permit empty content only while retaining identity, substantive-tail and usage checks. | Failing regression became green; live automatic streaming image-assisted loop passed. |
| Anthropic effort mapped to upstream reasoning effort | Claude Code with thinking disabled triggered incompatible reasoning. Validate and disclose the unforwarded response-effort hint instead. | JSON/SSE regressions passed; actual Claude Code 2.1.310 scoped Read/result/final-marker loop passed live in two requests without retries. |

These are protocol correctness bugs, not demonstrated security vulnerabilities.
A completed native Standard scan reviewed the 16 proxy files before the patches
and found no reportable vulnerabilities; subsequent static patch review is
supplementary. Current local checks and remaining coverage, including the latest
corrected CLI test-helper gaps and remaining raw OpenAI registration gap, are in
the checkpoint. The helper now passes fresh offline Read/Write acceptance using
the documented Edit(path) permission; live Write remains unverified.

## OpenAI and Edit continuation — 27 October 2026

[OPENAI_EDIT_CHECK.md](OPENAI_EDIT_CHECK.md) supersedes the remaining raw OpenAI
registration gap and records the current verification and immutable review.

| Cause | Correction and evidence |
| --- | --- |
| Native client registration was Anthropic-only | Move it into shared `valclient.Client.ProxyChat`. Both OpenAI aliases and direct callers now register the same URL-free schemas. Official SDK 3.38.0 JSON/streaming loops passed offline; streaming image/tool/result loop passed live. |
| OpenAI proxy dropped Retry-After | Preserve the upstream retry delay alongside original status/body. Failing 401/429/503 regressions passed after the fix. |
| Request buffering made 307/308 POST bodies replayable | Security patch review identified the new data-flow risk before live calls. Unset GetBody; direct/raw-route tests verify no target hit and original status/body. Anthropic also now refuses this replay. No hostile Val behavior was demonstrated. |
| Test acceptance was stricter than the requested behavior | Image color casing and final reply formatting caused false negatives. Tests now normalize only the red color label and allow prose around the final random marker; exact file bytes, valid calls, completed turns and permission checks remain enforced. |

The first live Edit attempt hit the authorized 128-token cap and incomplete
arguments were correctly rejected. After explicit authorization for 512 tokens,
actual editing verified exact expected bytes, three turns and zero denials. The
v4 checker then failed only whole-reply formatting; the v5 correction passed
offline and was not rerun live. No user repository files were sent or edited.
Shared request buffering is now limited to 32 MiB before dispatch; this is an
explicit compatibility contract, not a claim about every provider's own limits.

## Passive quota integration — 28 October 2026

Source checkpoint `60907a5`; main integration `04a1d32` on
`codex/passive-quota-checker`. [PASSIVE_QUOTA_CHECK.md](PASSIVE_QUOTA_CHECK.md)
records verification and recovery. These entries are correctness/integration
findings; they are not new validated security vulnerabilities.

| Cause and impact | Evidence and correction |
| --- | --- |
| Main's quota probe treated arbitrary HTTP 200 as available and presumed a reset clock | REA research reproduced four false-availability cases. Replaced probing with shared bounded observation; only valid completed envelopes imply available, budget/auth/rate errors stay distinct, remaining/reset stay null. New JSON/SSE and both-protocol tests passed. |
| Estimated/missing usage and repeated stream snapshots could mislead counters | New observer tests exercise missing/zero, late/cumulative/decreasing usage, oversized replies, malformed envelopes and cancellation. Only exact completed measurements count; forwarding bytes are preserved. |
| Old in-flight requests could contaminate cleared/new-account observations | Token/generation snapshot plus generation rejection; reset/token concurrency tests and race checks passed. |
| Port and subsequent token saves used stale whole-config state | Narrow per-field persistence preserves staged port, saved owner key and unknown fields; immutable live listener and failure cases have isolated regressions. |
| Integrated updater was unbounded and lacked verified archive/replacement recovery | Added explicit HTTPS/redirect/status/size/checksum gates and unique staging/Windows recovery. Temp-only executable tests passed on Windows and selected Ubuntu paths; real releases remain untested. |
| Update UI reloaded the browser while the process still ran its old executable | Node check failed before `60907a5`, then passed after replacing reload with a restart instruction. No automatic daemon replacement was added. |

Codex Security's immutable patch assessment is summarized in SECURITY_REVIEW.md;
it remains a scoped primary review with human review required, not a completed
final-tree repository scan. No new Val inference was used in this increment.

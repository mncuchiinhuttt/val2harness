# Agent compatibility and reliability checkpoint

26 October 2026. Local changes on `codex/preserve-config-owner-auth`, based on
`6999a37c298137e386567dfabee38d8c9caf83d0`. These changes are uncommitted and are
not included in the published v1.0.0 packages. Superpowers guided debugging,
regression tests, implementation and review; Firecrawl supplied official protocol
references. The Codex Security review limitation is recorded below.

## Result and scope

The Anthropic adapter now supports client tool calls, tool results and images.
The official Python Anthropic SDK **2.3.0** passed strict response validation for
JSON and streaming parallel-tool loops. Actual Claude Code **2.1.310** executed
its Read and Write tools in an isolated directory and returned their results
through the gateway. Both used the real gateway with a synthetic localhost Val
server. They establish client interoperability, not live model capability.

The latest bounded live checks **passed Sonnet JSON tool/result loops and an
automatic streaming image-assisted tool/result loop** through the official SDK.
The fix registers caller schemas in Val's client registry and accepts its empty
stream tail before measured usage. After a further effort-translation fix,
**Claude Code 2.1.310 passed a live scoped Read/result/final-reply loop** using
exactly two requests with no retry. Fresh offline Read/Write acceptance also
passed after correcting the helper's minimal-mode tool inventory and file
permission syntax. The [27 October checkpoint](OPENAI_EDIT_CHECK.md) adds live
OpenAI SDK streaming image/tool/result acceptance and verified live Edit file
execution; its checker had a final-reply formatting failure, corrected offline.
All 32 historical attempts are used, with no remaining live authorization.
See [SONNET_FIX.md](SONNET_FIX.md) for causes, evidence, security scope and
remaining limits, and [LIVE_VAL_CHECK.md](LIVE_VAL_CHECK.md) for earlier
observations. No real conversations or harness configuration were read or changed.

## Protocol matrix

| Capability | Current behavior and evidence |
| --- | --- |
| OpenAI Chat Completions | `/v1/chat/completions` and `/chat/completions` register nonempty function-tool schemas in Val's URL-free client registry through the shared client. Caller fields, large numbers and explicit registries are preserved; responses/status remain streamed through. Official SDK 3.38.0 passed JSON/streaming loops against a fake upstream and a streaming image/tool/result loop live with Sonnet 4.6. See OPENAI_EDIT_CHECK.md for scope. |
| Anthropic Messages | `/v1/messages` and `/messages` translate JSON and SSE; required model/messages/positive max_tokens and field types are validated. |
| Client tools | Object input schemas, descriptions, strict hints, automatic/none/required/specific choice and parallel control translate to OpenAI functions. Names longer than 64 characters receive stable upstream aliases and are restored on return. |
| Tool history/results | Call IDs and object arguments are preserved, including large JSON numbers. Missing, orphaned or duplicate results are rejected. Error results remain visibly marked; empty successful results are accepted. |
| Tool responses | Messages delivers only declared, correctly named calls with complete JSON-object arguments and checks tool-choice/parallel restrictions. OpenAI response bytes remain pass-through; the consuming client owns response validation and execution permissions. Parallel fragmented streams are covered by tests. |
| Images | User base64 JPEG/PNG/GIF/WebP and HTTP(S) image URLs translate to OpenAI image content. Live base64 PNG vision passed with val-gpt-4o and in Sonnet's streaming image-assisted client-tool loop. Standalone Sonnet vision without tools and other formats remain unverified. Base64 is bounded and MIME-checked; URL credentials and unsupported sources are rejected. The gateway never fetches image URLs. |
| Images in tool results | Text remains in contiguous OpenAI tool messages; image blocks become explicitly associated user image content after those results. This preserves data but changes its wire representation; live model interpretation is unverified. |
| Portable controls | max_tokens, temperature, top_p, top_k and up to four stop sequences are forwarded. Valid `output_config.effort` is disclosed as an unforwarded hint, since mapping it to reasoning_effort can activate unsupported thinking. JSON-schema output maps to response_format. Models can reject unsupported controls. Exact matched stop-sequence metadata is unavailable. |
| Hints and metadata | Valid ephemeral cache hints and eager-input-streaming hints are accepted but stripped; `metadata.user_id` is validated and kept out of upstream requests. `X-Val2harness-Compatibility` lists these adaptations. No caching behavior is promised. |
| Usage | Complete, measured prompt/completion counts are required for Messages. Actual cache-read/write breakdowns, when supplied, are translated without double counting. Missing, negative or inconsistent usage produces 502 with an unavailable marker. Counts are never estimated. |
| Errors/refusals | Upstream HTTP failures and Retry-After remain failures. Arbitrary upstream bodies are not echoed. Undeclared upstream tools produce an actionable 502 and are never delivered. Refusals translate explicitly; invalid/truncated completions fail instead of becoming successful empty turns. |
| Model identity | Exact documented aliases map to Val IDs; unfamiliar IDs pass upstream. The actual response/submitted model is reported. Model discovery does not prove tool or image support. |
| Cancellation and bounds | Request cancellation propagates. Shared Val request input and adapted payload are limited to 32 MiB; overflow returns 413 before dispatch. Messages additionally limits requests/JSON replies to 8 MiB; decoded images to 5 MiB; SSE events and individual tool arguments to 1 MiB; stream transport/content to 64/16 MiB and 200,000 events. |
| Native provider services | Signed extended/adaptive thinking, file IDs, built-in server/deferred tools, containers and other unimplemented fields are rejected. Use thinking.type=disabled. Token-counting, Files, Batches and OpenAI Responses APIs are not implemented. |

Anthropic SSE output is **buffered until the upstream turn completes**, including
its measured usage and validated tool arguments. The SDK then receives valid
message_start, indexed content blocks, input_json_delta, message_delta and
message_stop events. Initial output is delayed; this is not incremental token
latency. OpenAI pass-through streaming remains incremental. Buffering was chosen
over fabricated initial usage or releasing an invalid executable tool call.

Usage is a translation of upstream measurements, not a billing ledger. Some Val
models may omit usage even when stream_options.include_usage is requested; those
models cannot currently satisfy the strict Messages contract.

## Connect the selected clients

Build this source, connect Val through the dashboard and run `val2harness models`.
Choose an ID actually returned by that account. Run `val2harness env` to obtain
the private local connection settings; do not publish its output.

Claude Code uses ANTHROPIC_BASE_URL without `/v1`, the gateway's local key as
ANTHROPIC_API_KEY, and a model available through Val. Disable extended thinking
for this adapter. For example, after loading the private environment settings:

```powershell
$env:MAX_THINKING_TOKENS = '0'
claude --model '<available Val model ID>' --settings '{"alwaysThinkingEnabled":false}'
```

Retain Claude Code's normal permission controls. The automated test allowed only
Read and Write in a disposable directory; it did not bypass all tool permissions.
Current Read/Write fixture mode uses `Edit(./result.txt)` to authorize the Write
tool and is restricted to the known fake localhost gateway. Live Read mode keeps
`--bare`, which omits Write from this CLI's tool inventory.
This test does not establish every Claude Code feature or future version.

For the Python SDK, set base_url to the gateway root and api_key to its local
key. Standard `messages.create` and `messages.stream` tool loops are supported
within the matrix above. The application has no Python or Claude Code runtime
dependency; `tests/requirements-interop.txt` pins optional verification tools.

One actual path is: Claude Code sends a Read tool schema -> messages_request.go
maps it to a function -> Val's tool-call arguments are checked -> messages_stream.go
emits tool_use -> Claude Code reads the permitted file -> messages_content.go
turns its tool_result into an OpenAI tool message -> Val returns the final answer.
The harness owns file/network/tool permissions; the gateway never executes tools.

## Lifecycle and exports

[DAEMON_FIX.md](DAEMON_FIX.md) records corrected Windows liveness and verified
child ownership. New daemons use a separate random capability for an authenticated
readiness handshake and graceful shutdown. Its authority covers only the literal
daemon-control endpoint, never inference or general management. Stored address
and identity permit stop after settings change. Unix refuses an unsafe numeric
PID termination fallback. Legacy/unresponsive daemons can require manual recovery.

[EXPORT_FIX.md](EXPORT_FIX.md) records hashed contained filenames, exclusive
private creation, complete JSON preservation, deterministic Markdown and honest
partial counts. Default CLI/TUI/dashboard exports create a fresh private
`exports/run-*` directory. An explicit CLI directory never overwrites existing
targets. Windows export objects receive a protected user-only ACL at creation;
Unix directories/files use 0700/0600. Partial files are retained for recovery.

## Verification actually performed

Final source checks on Windows amd64 with Go 1.26.0:

- `go test ./... -count=1`, `go test -race ./... -count=1`, `go vet ./...`: passed.
- Native Windows build and actual CLI start/status/stop in two temporary homes:
  passed. Loopback/generated-key and wildcard/configured-key cases verified owner
  denial, scoped control, duplicate prevention, changed-settings shutdown and PID
  cleanup. Only synthetic tokens and an unreachable loopback upstream were used.
- Linux amd64 test binaries for every tested internal package executed under
  Ubuntu/WSL: passed. A Linux build passed the same actual CLI lifecycle script.
- Darwin arm64 build: passed; macOS runtime behavior was not tested.
- `node --check internal/web/static/app.js`, `node tests/dashboard-auth.cjs`,
  Python AST parsing and `git diff --check`: passed. Node checks include partial
  export failures without a success toast; Go covers repeated API export runs.
- Graphify code update and clustering completed offline with query logs disabled:
  631 nodes, 2,090 edges and 26 communities. Its Go receiver-call coverage is
  incomplete; important relationships were checked in source and tests.
- Official SDK JSON and streaming tool/result/image-input checks and actual
  Claude Code Read/Write loop: passed on the final gateway source. Vision tests
  prove wire preservation through a fake server, not image interpretation.

Regression coverage includes malformed/undeclared/oversize calls, name fragments,
parallel calls, precision, forbidden choices, URL/source validation, missing usage,
cache accounting, refusal, canceled/truncated streams, stale/reused process IDs,
encoded control paths, credential-scope denials, export conflicts, partial results
and inherited Windows ACL exposure. Windows symlink creation was unavailable;
Unix symlink/FIFO/mode tests executed on Ubuntu. Forced disk-full/sync/close
failures and hostile concurrent filesystem mutation were not runtime-tested.

Reproduce client checks with the opt-in `TestAgentInteropFixture` and
`tests/anthropic-sdk-check.py` / `tests/claude-code-check.py`; pass them a fresh
temporary fixture directory. The fixture stops on a `stop` file or after ten
minutes. `tests/daemon-cli-check.py` accepts a compiled executable and supplies
its own temporary homes. No test requires real account data.

`tests/live-val-check.py --live --binary <built executable>` is the separately
authorized live path: one model listing and at most three synthetic requests
(tool, result, generated red image), no retries, files or conversation exports.
It reads an existing local token without printing or persisting it in the child
gateway. The new recorder reports redacted structure and enforces an explicit
request limit. The initial missing-token attempt made no requests; later live
failures, successful val-gpt-4o vision and exhausted quota authorization are
recorded in LIVE_VAL_CHECK.md. The request ceiling is explicit and per run;
repeated runs require sufficient remaining human-authorized quota.

## Review and security evidence

The latest [Sonnet checkpoint](SONNET_FIX.md) records a completed native Standard
scan for 16 proxy files, sealed before its patches, and subsequent independent
static patch reviews. Its generated report is the primary current scoped
security artifact. The diff-launch failure below remains historical; it was not
resolved by changing global Git or plugin settings.

Two independent focused source reviews identified protocol and lifecycle/export
regressions. Reproduced corrections include concatenating equal name fragments,
enforcing requested tool choice, accepting empty refusals/results, retiring reused
PID records, replacing Unix numeric termination with scoped graceful control,
private Windows export ACLs and fresh default export directories. Parent review
also reproduced and closed encoded-route and control-token-as-owner bypasses.
Final regression tests passed. Workers later exhausted their account allowance;
the parent completed integration and verification directly.

The requested native Codex Security diff launch **did not start** and produced
no scan ID. It returned: “Review changes requires a non-bare Git worktree with a
resolvable HEAD.” Direct Git checks establish a valid non-bare worktree and HEAD.
The plugin's target helper instead chooses the ancestor `C:\.git` as its
protected repository, then refuses the installed Git executable under
`C:\Program Files\Git` as inside that protected root. Its trustedGit result is
null. Neither the ancestor metadata nor plugin/global configuration was changed.

Focused reviews and tests are not a completed native diff scan or security
clearance. The original baseline scan `37217ea0-663e-4ce5-9540-f40ea7040e53`
remains sealed against its original snapshot. Supplemental evidence is stored
through the plugin's artifact tool at
`artifacts/fixes/2026-10-26-agent-compatibility.md` in this target's persistent
standalone collection (SHA-256
`9032791e8aa9b6d20e49c208b040d518f749470464458a70ae082547f6fc428c`); [BUG_AUDIT.md](BUG_AUDIT.md) retains the audit
history. Resolve the scan-launch environment and perform a fresh final scan before
claiming that review complete. Windows settings/PID privacy still depends on
their containing directory ACLs; export ACL guarantees do not extend to all data.

## Remaining work

Complete the prepared redacted live investigation within an additionally
authorized quota budget, then exercise Claude Code against the actual model
with the user's normal permissions once tools work. Check
macOS lifecycle behavior natively. Remaining initial-audit work includes installer
download integrity, the dashboard playground stream parser and three unwired
daemon/port controls. Non-programmer feature research remains a separate next
phase. No universal agent compatibility or release readiness is claimed.

## Official references

Retrieved through Firecrawl on 26 October 2026; wire behavior was also checked
against the installed clients listed above:

- [Anthropic vision inputs](https://platform.claude.com/docs/en/build-with-claude/vision)
  and [streaming events](https://platform.claude.com/docs/en/build-with-claude/streaming).
- [Anthropic Messages request types](https://github.com/anthropics/anthropic-sdk-python/blob/main/src/anthropic/types/message_create_params.py),
  [tool-result types](https://github.com/anthropics/anthropic-sdk-python/blob/main/src/anthropic/types/tool_result_block_param.py),
  [output configuration](https://github.com/anthropics/anthropic-sdk-python/blob/main/src/anthropic/types/output_config_param.py),
  [usage](https://github.com/anthropics/anthropic-sdk-python/blob/main/src/anthropic/types/usage.py)
  and [SDK stream accumulation](https://github.com/anthropics/anthropic-sdk-python/blob/main/src/anthropic/lib/streaming/_messages.py).
- [OpenAI Chat Completion parameters](https://github.com/openai/openai-python/blob/main/src/openai/types/chat/completion_create_params.py)
  and [usage](https://github.com/openai/openai-python/blob/main/src/openai/types/completion_usage.py).
- [Claude Code CLI reference](https://code.claude.com/docs/en/cli-reference).

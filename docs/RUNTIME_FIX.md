# Runtime and protocol debugging checkpoint

Historical checkpoint: [AGENT_COMPATIBILITY.md](AGENT_COMPATIBILITY.md) supersedes
the text-only limits below. Client tools/results and images are now implemented;
refusals translate explicitly, and strict Messages responses require complete
measured usage instead of omitting unavailable counts. The previous tests and
decisions below describe this earlier increment.

26 October 2026. Local uncommitted continuation on
`codex/preserve-config-owner-auth`, based on upstream
`6999a37c298137e386567dfabee38d8c9caf83d0`. The prior configuration/authentication
changes remain present. Plan: `superpowers/plans/2026-10-26-runtime-reliability.md`.

## Corrected behavior

| Original audit | Current behavior |
| --- | --- |
| BUG-03, lost content/options and token limits | Positive max_tokens is required and forwarded. Only the implemented text subset is accepted; tools/results, images, thinking, and other unimplemented options receive 400 before upstream work. This fixes silent loss; it does not implement a tool loop. |
| BUG-04, false success on upstream failure | Upstream 4xx/5xx status and Retry-After are retained with structured Messages errors. Arbitrary upstream error bodies are not echoed. Malformed/missing response content or completion reasons produce 502. |
| BUG-05, token race | Client.SetToken/GetToken share one lock; every upstream request reads through GetToken. Web saves serialize disk publication and the live update, and startup Config stays immutable. |
| BUG-07, Go stream parsing | A shared 1 MiB-per-event SSE decoder handles LF/CRLF/CR, optional colon space, comments, multiline data and large frames. Parse/read/size/truncation failures do not emit normal message_stop. Terminal chat uses it too. |
| BUG-08, silent model substitution | Only listed exact aliases are mapped. Other IDs pass upstream unchanged for validation. Responses report the upstream model when present, otherwise the actual submitted ID. This does not establish model availability. |
| BUG-09, fabricated accounting | Actual input/output counts are translated, including measured zero. Missing counts are omitted. Nonstream responses without counts carry X-Val2harness-Usage: unavailable; streams advertise upstream-only accounting. |
| BUG-17, empty model wrapper | Both data:[] and raw [] are accepted. Null, missing data, and wrong shapes remain errors. |

Terminal chat now displays upstream/stream failures and commits a user/assistant
turn to history only after a supported finish reason and completion marker.
Partial output may already be visible, but a failed turn is not sent back as a
successful conversation. Structured refusal output is explicitly rejected until
it can be translated, rather than becoming an empty success.

## One important flow and the design choice

The gateway, direct CLI commands, and terminal menu all share Val client methods.
Locking only Server.UpdateToken could never protect direct client reads. The
lock now lives beside the Client token; each outgoing request takes a consistent
snapshot before building its Authorization header. A web token change first
saves successfully under the configuration file lock, then updates the client,
with a local save mutex preserving that order across simultaneous requests.
In-flight requests may finish with their original snapshot; new requests use the
latest successfully published token. Startup Config is no longer a second live
token store. Direct CLI token saves still require a running daemon to reload or
restart; inter-process hot reload is not implemented by this patch.

For Messages, validation happens before spending an upstream request. Successful
HTTP responses are decoded and validated before JSON success is sent. SSE starts
only after a valid completion frame; subsequent failures emit an error event
instead of a normal ending. Events are JSON encoded, including unusual model
strings. Request/reply documents are bounded at 8 MiB; SSE events at 1 MiB.
Client cancellation closes the upstream request. OpenAI wire pass-through stays
available and is not reinterpreted by this adapter.

The chosen scope corrects silent corruption/failure first. A complete Anthropic
tool/image/thinking implementation requires separate design, documented upstream
support and real harness/version tests. Rejecting unsupported behavior preserves
the user's ability to diagnose it and avoids claiming an agent completed work
that the gateway silently discarded.

## Regression and review evidence

Tests demonstrated the original failures before fixes: a race detector report
from token refresh versus ListModels; rejected empty wrappers and accepted null;
upstream 401/429/503 changed to 200; missing token limits/model fidelity;
unsupported requests reaching upstream; malformed responses returning success;
invented usage; lost no-space/70KB/CR/multiline streams; normal endings after
truncation; and failed terminal turns contaminating history.

One fresh Superpowers code reviewer inspected the runtime changes, their callers,
and credential boundaries without editing, running code or accessing real data.
Three concrete cases were independently reproduced and corrected: terminal
[DONE] without a finish reason, structured refusal output becoming empty success,
and legal whitespace in an empty tool-call array causing false rejection. The
latter two received JSON, streaming and terminal sibling coverage. Further
controls cover genuine zero counts, exact versus near-match aliases, model JSON
escaping, CRLF split across reads, and cancellation reaching the fake upstream.

The prior Codex Security threat model supplied the credential/access boundaries:
keep Val credentials private, preserve the owner HTTP gate and avoid echoing
arbitrary upstream data. This was focused change review, not another full security
scan. The original sealed scan and its findings were not changed. No new security
finding or full repository security clearance is claimed.

Final verification passed on the reviewed and corrected candidate:

- Windows amd64 / Go 1.26.0: `go test ./... -count=1`,
  `go test -race ./... -count=1`, `go vet ./...`, and a full executable build.
  The race suite now includes the original class of credential-refresh race and
  simultaneous token-save/status/request behavior, rather than only unrelated
  passing tests.
- Native Ubuntu/WSL execution of cross-compiled Linux test binaries for SSE,
  proxy, Val client, terminal chat and web. All passed. Windows builds the test
  binaries; Linux executes their filesystem/network/runtime code.
- Darwin/arm64 build passed; no macOS runtime check was performed.
- JavaScript syntax and dashboard lock/ticket/credential contract tests passed;
  `git diff --check` and formatting of changed Go files were clean.

These tests establish synthetic local behavior. No live Val request, installed
harness/version session, new browser UI acceptance run, Windows daemon lifecycle
check, export check, or full dependency/security scan was performed in this
increment. The previous browser/config/auth acceptance evidence remains recorded
in `CONFIG_AUTH_FIX.md`.

## Protocol limits and external evidence

Public official documentation was retrieved with Firecrawl on 26 October 2026:

- [Claude streaming guide](https://platform.claude.com/docs/en/build-with-claude/streaming):
  event order, error events, and cumulative usage in message deltas. These informed
  the framing and error tests; official behavior is broader than this adapter.
- [WHATWG SSE parsing](https://html.spec.whatwg.org/multipage/server-sent-events.html#parsing-an-event-stream):
  line endings, comments, optional colon spacing, multiline data and blank-line
  dispatch. Pending incomplete data is discarded and reported locally as a
  truncated stream instead of success.

The Messages endpoint is a documented partial implementation. Omitting unavailable
usage is an explicit local choice, not a claim that official Anthropic schemas
permit missing accounting. Strict clients may reject missing or late usage.
No token estimates are substituted. Upstream usage/model metadata and acceptance
of stream_options.include_usage have not been checked against a live Val account.
The public API-reference retrieval did not expose enough schema content to prove
full compatibility. Tools/results, images, structured refusal/reasoning output,
stop_sequences and other unimplemented options remain unsupported. The gateway's
usage must not be treated as a complete billing record.

Remaining work: Windows daemon liveness/control (BUG-10), export ID/path/write
integrity (BUG-14/15), remaining dashboard controls (BUG-16), installer integrity,
model metadata and real agent interoperability. The dashboard's separate browser
playground still has a lightweight OpenAI stream parser; its multiline/truncation
behavior is a source-observed follow-up, not repaired by the Go SSE package.
Usability/feature research remains pending.

All runtime tests use synthetic credentials, localhost fake upstreams, and
temporary homes/files. No real token, conversation or harness setting was read or
changed. No dependency, service, telemetry, upload, commit, push or deployment was
introduced. Open source does not change ownership of user data.

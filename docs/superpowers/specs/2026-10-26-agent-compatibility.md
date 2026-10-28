# Agent tools, images, and reliability

User-selected targets: Claude Code and the official Anthropic SDK. Preserve the
Go gateway and local owner boundary. Keep user data user-owned and testing
hermetic. Existing uncommitted fixes remain intact.

## Design

Translate Anthropic client tools to OpenAI functions, preserving schemas, call IDs,
parallel calls and results. Validate all IDs/results and preserve error status.
Support user text plus base64/HTTP(S) image blocks. Convert tool-result images into
explicitly associated user image content after the required tool result messages;
the gateway never fetches those URLs or executes a tool. Harnesses own execution.
Do not copy Anthropic file IDs, built-in server tools or signed thinking into
incompatible fields; unsupported provider services must remain explicit.

Map response tool_calls to tool_use blocks and streamed call fragments to valid
indexed input_json_delta events. Require complete, valid JSON object arguments
before normal completion. Interleaved/parallel tool streams retain correlation;
bound aggregate arguments and event count. Preserve failure and cancellation.
Translate real usage. Since upstream usage can arrive late, buffer initial output
where required by SDK accumulation; unavailable accounting stays explicit rather
than invented. Do not promise full vendor API parity or availability of a model.

Accept and validate portable common controls (limits, temperature/top_p, stop
sequences, tool choice). Optional client metadata and cache hints may be accepted
only with documented compatibility behavior and response warnings when stripped;
never pretend upstream prompt caching or token counting. Extended thinking,
server tools, files and other remote services need verified upstream support.

The chosen approach extends the existing translation boundary. Rewriting the
gateway or inserting another online provider would introduce unnecessary data
transfer and dependencies. Native Anthropic passthrough cannot be assumed because
the known Val endpoint accepts OpenAI Chat Completions.

## Acceptance

Prove JSON and streaming tool loops, parallel calls, malformed fragments, image
translation, tool errors/results, owner denial, cancellation and genuine usage
using fake upstreams. Exercise official SDK create/stream accumulation and an
isolated Claude Code read-tool loop. Distinguish this from live Val/model/harness
compatibility, which cannot be established with synthetic upstreams alone.

In parallel, reproduce and repair Windows daemon liveness/identity/lifecycle and
private, contained, accurate exports. Keep source ownership disjoint and retain
tests and concise subsystem records. Run full Go/race/vet/build and native Linux
checks, then a Codex Security diff review of the frozen final source.

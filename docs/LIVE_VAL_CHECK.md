# Live Val validation — 26 October 2026

Branch `codex/preserve-config-owner-auth`, base
`6999a37c298137e386567dfabee38d8c9caf83d0`, local uncommitted source. This record
supersedes the earlier missing-token status in AGENT_COMPATIBILITY.md.

**Latest checkpoint:** [SONNET_FIX.md](SONNET_FIX.md) supersedes the blocked-tool
status below. A third, separately authorized batch of 12 attempts established
working Sonnet JSON and streaming image-assisted client-tool loops. Earlier
batches below are retained as historical evidence; current quota and Claude Code
verification are recorded in the latest checkpoint.

The user saved a token locally and authorized model discovery plus at most three
small synthetic inference requests. The credential was read in process, never
printed or sent to research services. No user conversations, repository files or
harness settings were submitted. No chats were deliberately created or exported;
upstream logging/retention cannot be established by these checks.

## Observed results

Model discovery succeeded and returned **53 models**. The selected model was
`anthropic-claude-sonnet-4-6`.

| Request | Observed result | What it establishes |
| --- | --- | --- |
| 1. Official SDK through isolated gateway: call synthetic compat_echo | Gateway HTTP 502; script stopped before tool-result or image calls. | The live Messages tool loop failed. The original script did not retain a safe upstream structural diagnostic, so the exact first failure is unknown. |
| 2. Direct Val request with the same synthetic client tool/schema and required choice | Val HTTP 200, one choice, finish_reason=tool_calls, two returned calls. Neither matched the combined expected name/arguments test. Usage: prompt=1092, completion=85, total=1177. | Credentials/model access and measured nonstream usage work. The returned calls do not satisfy the synthetic fixture. This diagnostic did not retain individual names/argument shapes, so it cannot distinguish an undeclared tool from wrong arguments. |
| 3. Generated 32×32 red image plus synthetic tool and parallel_tool_calls=false | Val HTTP 400 with a detail field. | This combined request was rejected. Image support, parallel control and tool-choice interaction were not isolated, and the detailed cause was not retained. Vision is still unverified. |

The **initial three-request budget was exhausted and live inference stopped**. Successful
model discovery and HTTP 200 are not evidence of a working agent loop. No
successful tool result turn, live vision result, streaming Val loop or live
Claude Code run was established by that first batch. The subsequently authorized
follow-up below establishes one live vision result.

## Authorized six-request follow-up

The user separately approved **six additional small inference request attempts,
total across runs**, without automatic retries or account-setting changes.

| Additional attempts | Result |
| --- | --- |
| 1–2: Sonnet tools and image separately, using recorder | Both reached Val HTTP 200 but produced undeclared server tools. Tool request returned `search_chats` and `query_knowledge_files`; image-only request returned `query_knowledge_files` with finish_reason=length. The gateway refused each with 502. |
| 3: documented direct provider completion route | `/openai/chat/completions` returned HTTP 403. This route was not usable with the available access; no alternate means of circumventing that denial was attempted. |
| 4–5: allowed completion route with request-scoped params.function_calling=default | The same unwanted server tools remained for both tool and image checks. This flag did not remedy the behavior, so no production mode change was made. |
| 6: image-only check with available model val-gpt-4o | **Passed.** Official Anthropic SDK -> current gateway -> Val returned end_turn and correctly identified the generated red image. Upstream measured prompt=1293, completion=11, total=1304. |

At that checkpoint, all **nine approved request attempts** had been used. This includes HTTP
400/403 denials; it is not a claim that nine model executions were billed. Live
calls stopped at the combined ceiling. Live streaming and a successful live
tool/result loop remain unverified. Claude Code's successful Read/Write loop is
still fake-upstream evidence only.

Read-only metadata reports Val version `26.9`. Sonnet's model configuration
advertises vision and usage, sets function_calling=native and enables built-in
tools. Other available models, including val-gpt-4o, explicitly disable built-in
tools. Metadata alone does not prove their client-tool support. The next useful
tool-loop candidate is a suitable model from that group; testing it needs a new
explicit quota budget.

## Diagnosis and uncertainty

The adapter checks complete object arguments, declared tool names and requested
choice before releasing executable calls. Its fake-upstream tests and actual
SDK/Claude Code fixture loops remain valid evidence of client-side translation;
they do not override these live failures.

Firecrawl retrieved a similar
[Open WebUI caller-tool replacement issue](https://github.com/open-webui/open-webui/issues/21557)
and the current [official server-side tool API documentation](https://docs.openwebui.com/reference/server-side-tool-calling).
The issue describes supplied client tools being replaced by server tools and was
closed after a development-branch fix. Current documentation says an explicit
tools key preserves caller tools and skips server tool resolution. **This is a
candidate explanation, not a validated cause in Val.** Val's deployed backend
implementation and the original combined image/control rejection reason have
not been established. The follow-up did establish exact unwanted names and the
selected model's built-in tool configuration; the request-mode experiment did
not fix it. Do not infer that the upstream issue's published fix is deployed in
Val or change account/server settings based on that hypothesis.

The gateway now gives an actionable fixed error for undeclared calls: Val
returned tool calls not declared by this client; check upstream tool injection
or select a model that preserves client tools. It still refuses to deliver those
calls. A failing JSON/SSE regression established the previous generic error;
both variants pass with the clearer diagnostic. No upstream route or mode was
changed in production and no tool execution permission was expanded.

## Prepared follow-up

The original live script did not report enough structure to separate failures.
`tests/live_probe_support.py` now supplies an authenticated loopback recorder:

- Only model discovery and chat completion paths are forwarded. The real Val
  credential stays in the recorder; the child gateway receives a disposable
  local capability instead.
- A locked inference counter enforces an explicit 1–10 request ceiling. Model
  discovery is limited to one request per run. No redirects, retries, chat
  history endpoints or arbitrary forwarding are permitted by the recorder.
- Output contains HTTP status, measured usage, finish reason, validated tool
  names, argument kinds/known keys and fixed error categories. It excludes call
  IDs, content text, arbitrary argument values and provider error text. The real
  credential is removed before response summarization.
- Tools and vision can be checked separately. The script identifies each stage,
  continues an independent vision check when budget permits, and explicitly
  reports the number of inference requests attempted.

`tests/live-val-check.py` accepts `--checks all|tools|vision` and
`--max-requests N`. Its default all path needs up to three requests: tool call,
tool result, then image. Per-run ceilings do not constitute authorization for
repeated runs; count all requests against the user's agreed total budget.

Offline verification of the recorder passed seven unit/integration tests covering
redaction, tool argument shapes, fixed error categories, authentication, route
restrictions, credential substitution, fixed route/mode choices and a one-request
ceiling. Route/mode forwarding preserves other payload fields. A separate
synthetic-upstream check ran the actual SDK and compiled gateway through the
updated script: tools plus vision passed with exactly three upstream requests;
a one-request run stopped at its ceiling with the expected incomplete status.
Neither verification used a real token or external inference.

Diagnostic switches `--completion-path` and `--function-calling` were limited to
fixed documented choices; they were used only in the recorder experiments above.
Both remain opt-in. Further live calls require a new explicit quota authorization.
The user token and existing account/model settings remain unchanged.

## Other verification limits

The only production Go change in this continuation is the clearer undeclared
tool diagnostic; protocol denial is retained. `go test ./... -count=1`,
`go vet ./...` and `go test -race ./internal/proxy -count=1` passed after the change.
The previous wider race/native Windows+Ubuntu acceptance evidence remains in
AGENT_COMPATIBILITY.md. Recorder tests, fake-upstream script acceptance, Python
syntax and diff formatting checks passed. The final proxy package test binary
also executed successfully under Ubuntu/WSL after this error change. Graphify
was updated offline: 674 nodes, 2,166 edges and 26 communities. The native
Codex Security diff-launch limitation remains unresolved; supplemental evidence
does not replace a completed scan. No universal compatibility or release claim.

Plugin-managed follow-up evidence:
`artifacts/validation/2026-10-26-live-val-followup.md` in this target's persistent
standalone collection, SHA-256
`72a6eeffc7851e41804f54ede4c544885b02e7d1d07e3f0026bf5012ef2cdc97`.

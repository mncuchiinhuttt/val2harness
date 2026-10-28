# REA installation and Val quota investigation

Checkpoint: 28 October 2026. Research branch: `codex/rea-quota-research`,
based on published commit `bf9ad72`. The inspected remote main is
`d8e4760e7b0730dba6c4a7c0bb4d57d531a328f3`. Its newer features have not been
merged into this branch. This checkpoint installs an analysis tool and records
research; it does not implement a quota checker or change the running gateway.

## Reusable local REA setup

Installed `rea-agents@6.8.0` outside the application, with the upstream
`reverse-engineer-anything` skill from tag `rea-agents-6.8.0` (skill revision 49).
The installed skill includes a local launcher reference. Codex can discover it
on a subsequent turn; CLI access was verified in this session.

In Windows PowerShell:

```powershell
$reaLauncher = Join-Path $env:USERPROFILE '.codex\tools\rea-6.8.0\rea.ps1'
& $reaLauncher --version
& $reaLauncher inspect-web-network-capture 'C:\absolute\synthetic.har' har --format json
```

The launcher uses Codex's existing bundled Node **24.22.1** and selects it for
child commands within that invocation. System Node **22.23.2** is below this
REA release's required range. System Node, persistent PATH, client settings,
other MCP servers and the application's dependencies were not changed.

The CLI lives under `~/.codex/tools/rea-6.8.0`; the skill lives under
`~/.codex/skills/reverse-engineer-anything`. npm installation disabled lifecycle
scripts and omitted optional dependencies. Frida and external native-analysis
engines were not configured or verified. No REA MCP server was registered;
the CLI and skill are the chosen integration. A plugin/MCP connection can be
added later when needed, with a supported runtime and a scoped configuration
plan. Do not install both integrations as duplicate servers.

REA is MIT licensed. Its installed package retains its license and third-party
notices. Native commercial engines have separate licenses; none is required for
the JavaScript and HAR inspection verified here. The local launcher depends on
Codex's bundled runtime path: inspect the replacement path and version if that
runtime moves. Update package, launcher and matching skill deliberately, rather
than fetching `@latest` during an investigation.

Installation sources, read on 28 October:

- [REA repository](https://github.com/morluto/rea)
- [Installation and supported runtimes](https://github.com/morluto/rea/blob/main/docs/installation.md)
- [Pinned skill](https://github.com/morluto/rea/blob/rea-agents-6.8.0/.agents/skills/reverse-engineer-anything/SKILL.md)
- [Published package](https://www.npmjs.com/package/rea-agents/v/6.8.0)
- [License](https://github.com/morluto/rea/blob/rea-agents-6.8.0/LICENSE)

## Evidence and coverage

The initial investigation used unauthenticated public-page and public-asset
reads. After the saved token's local expiry was found to be in the past, the
user refreshed it and authorized read-only account metadata inspection. One
`GET /api/v1/auths/` returned HTTP 200 with JSON. There were **zero new Val
inference requests**, no redirects and no retries. No token or conversation was
sent to Firecrawl. Returned credentials and personal field values were not
printed or saved; the local summary keeps field types and selected quota-field
candidates only.

Machine-local material is under ignored `.codex/rea/quota-20261028/`:

| Evidence | Result and limit |
| --- | --- |
| Graphify's existing code graph | Queried offline: 799 nodes. Located the shared Val client, measured usage adapter, streaming tests and retry-delay test. Receiver-call edges remain incomplete; source was checked directly. |
| Val public entry page and directly referenced scripts | Nine files, 77,831 bytes. REA parsed eight JavaScript files with no parse failures. Coverage is partial; inline scripts and unavailable dependencies are not proof of absent behavior. |
| Further explicitly referenced public JavaScript | 78 retained assets, 4,935,530 bytes, including the app/auth route modules. The discovery pass made at most 56 additional asset requests and skipped or failed three. Text inspection identified Val version declaration `26.9` and API bases, including `/api/v1/rmit`; it did not establish a quota endpoint. This larger set was not passed through the initial REA analysis. |
| Synthetic HAR | REA retained all three records: measured success, HTTP 200 with a budget-error body, and HTTP 429 with a rate-limit error. Fixture uses `val.invalid`; inspection does not fetch its URLs. This verifies local tooling, not current Val behavior. |
| Remote-main quota source copied into an isolated Go module | Four meaningful failure checks fail: HTTP 401, HTTP 429 without a budget code, HTTP 503 HTML, and malformed HTTP 200 are all reported `active`. Only fake loopback servers and a synthetic credential were used. |
| Authorized self-account metadata read | HTTP 200 JSON, with profile/authentication/permission fields and no quota/rate-limit headers. No amount, remaining budget, cost unit or reset contract was identified in this response. A workspace `usage` capability is a permission flag, not a quota value. This does not establish that another permitted quota endpoint does not exist. |

Retained REA parent Evidence IDs:

- Public entry artifacts:
  `ev_bd3e35033a56ab0462962ae9e1cead6239acb4a77a5c37f8811695cf363f1ec5`.
- Synthetic HAR:
  `ev_faf3528b9691ffbf865cb74cedd3f3826416f94e98e527faad3368fed983a6fe`.

Complete evidence bundles and asset manifests remain local and ignored. A real
HAR can contain tokens, conversation text and personal data. Retain only what
the user authorizes, redact locally before presenting evidence, and select quota
fields or field shapes instead of printing whole responses. Do not upload a
private capture or commit it with the open-source code.

Final checks: skill frontmatter validation, PowerShell launcher parsing and
actual CLI version passed; both REA operations completed with exit 0. Relative
documentation links and `git diff --check` passed. Graphify update and clustering
ran with external AI and query logging disabled: 818 nodes, 2,543 edges and 46
communities. This refreshed the code graph, not semantic document extraction.
The isolated quota failure tests intentionally remain red against the copied
remote-main implementation; a fix has not been implemented in this checkpoint.

The [official RMIT Val page](https://www.rmit.edu.au/students/support-services/study-support/val),
retrieved with Firecrawl on 28 October, contains privacy and usage information,
but the retrieved text does not specify a quota amount, measurement unit or
reset time. The inspected public assets likewise do not establish those facts.
This is a bounded search, not proof that no quota API exists.

## What the gateway can observe today

The main request flow is `handleChatCompletions` → `Client.ProxyChat` →
`POST /api/chat/completions`. Anthropic Messages requests translate through the
same client. The local owner credential and the upstream Val token serve
different boundaries; quota observation should not expose either one.

| Input/output | Established meaning | What it cannot establish |
| --- | --- | --- |
| `model`, messages, tool schemas/results and images | Inputs sent to the selected model, including agent context | Exact billed cost or an account's quota limit |
| `max_tokens` | Requested output cap | Total request tokens; the prompt can be much larger than the cap |
| JSON/SSE `usage.prompt_tokens`, `completion_tokens`, `total_tokens` | Measured usage for that request when the upstream supplies it; terminal SSE usage may arrive late | Remaining account budget without a known quota and accounting policy |
| HTTP status, JSON error code and `Retry-After` | Observed failure or retry guidance | That every 429 means exhausted daily quota, or that a retry delay is the daily reset time |
| Local token expiry | A credential's claimed expiry time | A quota reset time |

Previous measured tool-loop evidence is in [OPENAI_EDIT_CHECK.md](OPENAI_EDIT_CHECK.md)
and [SONNET_FIX.md](SONNET_FIX.md). No additional quota was spent to collect it
again. All 32 historical inference attempts remain used; there is no remaining
inference authorization.

## Problems in the newer main implementation

Inspect these files at remote commit `d8e4760`, not as files implemented on this
research branch: `internal/valclient/quota.go`, `internal/web/api_routes.go`,
`internal/web/static/app.js`, and `internal/proxy/completions.go`.

1. `loadStatus` calls `loadQuota(false)`, which fetches a GET management route.
   `CheckQuota` then sends a real inference request. Opening or refreshing status
   can therefore spend quota without pressing the probe button.
2. `CheckQuota` defaults to `active` without requiring an HTTP success and a
   valid completion. The four fake-upstream cases above reproduce false success;
   a bad model ID or missing permission can be misclassified for the same reason.
3. The probe caps output at one token. It still sends a prompt and cannot prove
   that its total quota cost is one token or that the selected model is available.
4. The 4 AM Melbourne reset is an assumption in code. The fixed UTC+11 fallback
   and `04:00 AEDT` label are also wrong for Melbourne's standard-time season.
   Neither source nor public documentation inspected here verifies Val's reset
   policy.
5. Error-body handling recognizes `budget_exceeded`, including a possible HTTP
   200 envelope. This is a source-supported candidate signature; this session
   has not observed an actual authenticated Val quota failure. Other errors
   must retain their own status and meaning.
6. Main's analytics records estimated input `len(bodyBytes)/4` and fixed output
   `40` on the shown OpenAI streaming path. Those values cannot support an exact
   quota meter. Use measured usage, and represent missing usage as unknown.

These are correctness and evidence findings. They do not imply a new completed
Codex Security scan or authorization to merge the newer main changes.

## Approaches to a checker

**Preferred, if supported:** read a source-identified, non-inference account
metadata endpoint for authoritative limit, remaining amount, unit and reset.
Combine that with passive observation of the user's ordinary gateway requests.
Expose each value's source, observation time and freshness. This can include
account-wide activity only if Val's response actually accounts for it.

**Fallback:** show measured usage through this gateway and observed availability
or exhaustion. Local usage excludes the Val website, other clients and missing
history; it must be labeled accordingly. A manual synthetic probe can establish
availability at a moment in time, but needs an explicit request allowance and
cannot reveal an exact remaining percentage by itself.

The preferred design preserves the Go client/proxy/dashboard architecture.
One shared quota interpretation should feed both API protocols, CLI and web
views. A UI-specific probe or guessed model-cost calculator would duplicate
logic and create misleading results.

Acceptance criteria for a selected implementation:

- Dashboard loads and read-only status calls send no inference requests.
- Distinguish unknown, available-as-observed, budget exhausted, rate limited,
  authentication failure and provider failure. Malformed replies never imply
  availability.
- Leave unknown amounts and reset timestamps absent; do not substitute zero,
  100%, 4 AM or token expiry. Apply real time-zone/DST rules to verified policy.
- Count complete measured usage once, including late SSE usage; do not invent
  missing counts or treat a cancelled/partial stream as a complete measurement.
- Keep account metadata private, separate it from conversation content, clear
  stale account state when credentials change, and let the user delete any
  retained local usage history.
- Preserve local owner authentication, method checks, Host/Origin protections,
  bounded reads, cancellation and retry guidance. Any inference probe uses an
  explicit action, a bounded allowance, and no automatic retries.
- Test these failure cases against fake upstreams before any newly authorized
  live verification. Review integration with newer main separately.

## Next evidence needed

The user refreshed the saved token through the normal flow, and the one
source-identified self-account read completed. It did not supply a quota
contract. Further metadata discovery should follow observed, permitted client
routes; do not enumerate guessed/admin routes or work around permission flags.
Normal request observation can establish availability, measured usage and
recognized exhaustion signals. A new synthetic inference request still needs
an explicit allowance. Exact quota amounts, cost weights, reset policy and
account-wide coverage remain unresolved. The remaining evidence work is to
identify a permitted account-quota source or choose the clearly labeled passive
fallback; no quota feature has been implemented yet.

# Development direction and workflow

Agreed direction recorded on 26 October 2026. This document defines future work;
it is not a completed audit, research report, or authorization for every proposed
feature. The source baseline and tool setup are in `PROJECT_CONTEXT.md`.

## Product intent

Make authorized RMIT Val access usable from a broad range of agents and
harnesses. Someone without programming experience should be able to install the
bridge, understand its status, connect a supported tool, and recover from common
failures. Developers must retain dependable programmatic APIs and CLI controls.

Keep the existing Go architecture as the starting point. Fix structural problems
or add capabilities when evidence calls for them. Favor shared gateway behavior
with UI/CLI adapters over duplicating logic or rebuilding the product around one
specific harness. Universal compatibility is a direction, not an established fact.

The project is open source and user data belongs to the user. Preserve the MIT
license and attribution, and keep credentials, conversations, settings, logs, and
exports under user control. Public source is not consent to collect or publish
data. Prefer local storage and redacted diagnostics; additional telemetry, data
uploads, or sharing must be explicit and optional. Be clear that a model request
sends the selected input to the user's configured Val service.

## Phase 1: Trace correctness and security bugs

Use Superpowers systematic debugging: establish a repeatable trigger, inspect
working comparisons and the failing flow, test a specific hypothesis, and identify
the root cause before proposing a fix. Use Graphify for relationship discovery
and targeted source reads for actual behavior and counterevidence.

Start with the existing candidate observations in `PROJECT_CONTEXT.md`; do not
present them as confirmed audit findings without further investigation. Inspect:

- OpenAI forwarding and Anthropic translation: parameter/content preservation,
  tools, streaming events, partial streams, error status, and reported usage.
- Credential lifecycle and access boundaries: token loading/updating, browser
  origins, local management routes, listener exposure, and diagnostic redaction.
- Configuration integrity: parsing, safe writes, backup/restore, custom ports,
  preservation of other providers, and errors from each harness adapter.
- Daemon lifecycle and shared state: startup/shutdown, health checks, process
  identity, concurrent requests/token changes, and Windows/Unix differences.
- Export, UI, and installation: malformed data, filesystem failures, recovery,
  misleading status, download integrity, and documentation/code discrepancies.

Use Codex Security for security faults. Select the relevant repository-scan or
finding workflow, honor its preflight and scope, and preserve its native evidence
and report artifacts. Validate exploitability against real controls and realistic
attacker access; distinguish a security issue from a crash, compatibility gap, or
hardening suggestion. Do not upload the repository or switch to a Cloud scan just
because a local capability is unavailable. State the limitation and use the
supported workflow. Security findings stay source-backed and calibrated.

Prefer fake upstream servers, temporary home/config directories, and synthetic
tokens. Tests must not export real conversations, alter real harness settings, or
exercise a live account unintentionally. Where runtime proof is unavailable,
record the exact static evidence and remaining proof gap.

Produce `docs/BUG_AUDIT.md` with scope, commit, environment, checks, coverage gaps,
and one entry per distinct root cause: trigger, expected/actual behavior, affected
code path, user impact, priority, confidence, reproduction or proof gap, and fix
acceptance criteria. Link Codex Security artifacts where applicable. Group
duplicate symptoms. Do not call the repository secure because a scan found none.

Phase 1 is sufficiently complete to inform research when its scope has been
reviewed and findings/coverage gaps are recorded. Resolving every finding is not
a prerequisite to researching improvements; fixes get their own verified work.

## Phase 2: Research usability and interoperability

Use Superpowers brainstorming to clarify material product decisions and compare
approaches. Use Firecrawl for current website evidence, prioritizing official
Val guidance where accessible, protocol documentation, supported harness docs,
and relevant installation/accessibility guidance. Record versions and retrieval
dates. Distinguish published claims, source observations, and tested behavior.
Do not bypass login or assume institutional permissions, quotas, or entitlements.

Investigate these journeys as candidates, not preselected features:

| Journey | Research questions |
| --- | --- |
| Install and launch | What minimizes manual steps across Windows/macOS/Linux? How are integrity, prerequisites, updates, and rollback handled? |
| Connect Val | Can authorized onboarding avoid terminal/token jargon? How can users understand expiration and reconnect without exposing credentials? |
| Connect an agent | Which tools and versions accept which protocols? Can setup detect installations, preview changes, back up, validate, and restore safely? |
| Use daily | Can users tell whether the bridge, account, upstream, and selected model are working? What errors offer a useful next action? |
| Get help and recover | Can diagnostics explain failures without leaking tokens or chats? What repair and uninstall flows are safe and understandable? |
| Advanced use | Can CLI/API users automate configuration and health checks without depending on the dashboard? |

Build a compatibility matrix around documented and observed requirements. Include
OpenAI Chat Completions, Anthropic Messages, and other APIs only where target
harnesses require them. Do not equate exposing an MCP server with implementing a
model-provider API. Track tools, multimodal content, streaming, errors,
cancellation, authentication, model discovery, and known upstream restrictions.
Support an honest unsupported/partial state rather than silent degradation.

Produce `docs/FEATURE_RESEARCH.md`: user problems, cited evidence, options,
recommended order, compatibility matrix, links to audit findings, architecture
impact, dependencies/licensing/privacy/cost, effort, and measurable acceptance
criteria. Separate bug fixes, compatibility work, usability improvements, and
speculative ideas. Rank core reliability and demonstrated needs above breadth.

## Phase 3: Deliver selected work

Follow the applicable Superpowers design, planning, implementation, review, and
verification skills. Scale the plan to the task. Keep routine authorized work
moving; raise consequential choices with concrete alternatives and evidence.
Use agents only when permitted and useful, with bounded scope and acceptance
evidence. Avoid multiple competing plans or duplicate mandatory workflows.

For behavioral fixes, establish a meaningful failing regression test, implement
the smallest coherent correction, and verify both the trigger and relevant
failure cases. For security fixes, use the appropriate Codex Security remediation
workflow. Evolve interfaces behind shared code; explain migrations and compatibility
changes before introducing them. Do not bundle unrelated feature work into fixes.

Verification options, selected according to the change:

```powershell
go test ./...
go vet ./...
go build -ldflags '-s -w' -o val2harness.exe .
node --check internal/web/static/app.js
# For concurrency changes, when a suitable CGO/C compiler is available:
go test -race ./...
```

Use UI checks for changed user journeys and fake-upstream integration tests for
protocol behavior. Run appropriate cross-platform checks for OS-specific changes.
Do not imply unrun platforms or live harnesses were tested. Define practical
release and recovery criteria as release approaches.

At meaningful checkpoints, update the relevant audit/research/design records and
`PROJECT_CONTEXT.md`, preserving scope, commit, verified state, unfinished work,
and next action. Refresh Graphify with external AI disabled. Explain outcomes in
plain language, including one concrete code flow and what the evidence proves.

## Current handoff

The first audit is retained in `BUG_AUDIT.md`, including local reproductions, a
sealed Codex Security report, and coverage limits. The user accepted configuration
preservation and usable owner authentication; that increment is implemented on
`codex/preserve-config-owner-auth`. See `CONFIG_AUTH_FIX.md` for the exact current
verification, independent review, migration steps, and unresolved issues. Tests
now live in the repository and use synthetic data and temporary settings.
The continuation fixes token synchronization, protocol failure reporting, bounded
SSE, model/usage fidelity, terminal history, and empty model lists; see
`RUNTIME_FIX.md`. The later `AGENT_COMPATIBILITY.md` supersedes its text-only
limits: client tool/results, images, strict measured usage and buffered SSE now
pass official SDK and isolated Claude Code Read/Write loops. `DAEMON_FIX.md` and
`EXPORT_FIX.md` record lifecycle and private export corrections. Windows/Ubuntu
tests and native CLI acceptance passed; macOS was build-only. The authorized live
check initially stopped without a configured token. After local token setup,
model discovery and usage succeeded but Sonnet injected undeclared search and
knowledge tools. A separately authorized six-request follow-up verified live
vision through val-gpt-4o. All nine approved attempts are used; live tool loops
remain unverified. See LIVE_VAL_CHECK.md. Redacted per-request diagnostics enforce
the request ceiling, and clearer undeclared-tool errors retain the denial.

The requested native Codex Security diff scan did not start because its helper
selects an ancestor Git root and cannot trust the installed Git executable.
Focused source review, regressions and supplemental evidence are recorded;
no completed final scan is claimed. Next: obtain further bounded live quota
authorization, investigate with the prepared recorder, resolve the scan
environment, and test macOS natively. Initial-audit
installer/playground/unwired-control issues and non-programmer feature research
remain separate work. All changes remain local and uncommitted.

Latest continuation: [SONNET_FIX.md](SONNET_FIX.md) supersedes the blocked-tool
status above. Sonnet JSON and streaming image-assisted SDK tool loops passed live
after correcting its client registry and empty stream tail. The current effort
hint handling preserves disabled thinking. The checkpoint records actual Claude
Code verification, quota accounting, fresh checks and remaining OpenAI coverage.
A scoped native Standard Security scan completed for 16 proxy files before these
fixes; independent static patch review supplements that sealed snapshot.

The [27 October OpenAI/editing continuation](OPENAI_EDIT_CHECK.md) moves client
schema registration into the shared Go Val client, verifies an official OpenAI
SDK streaming image/tool/result loop live and verifies actual Claude Code Edit
file execution. Codex Security caught a buffering-induced redirect replay
regression before live runs; it was corrected and covered by failing/passing
tests. Final-reply formatting is separated from exact file/content evidence.
All 32 historical live attempts are used; no remaining live authorization.

# Agent compatibility implementation plan

> **For agentic workers:** Superpowers TDD and systematic debugging; independent
> daemon/export domains use bounded parallel agents; parent implements protocol.

**Goal:** Verify Anthropic tool/image agent loops and finish lifecycle/export fixes.
**Architecture:** Existing local Go gateway with request, response and SSE helpers.
**Tech stack:** Standard library and existing Go dependencies; temporary official
SDK/CLI installations for interoperability checks only.
**Spec:** `../specs/2026-10-26-agent-compatibility.md`.

- [x] Daemon worker: red/green platform liveness, safe process ownership,
  start/status/stop/failure/duplicate controls; isolated native acceptance.
- [x] Export worker: red/green ID containment, collisions, private exclusive
  writes, precise partial outcomes, ordered Markdown, source-number preservation.
- [x] Parent: red/green request tool schemas/choice, paired tool history/results,
  image conversion, portable controls and documented hint handling.
- [x] Parent: red/green JSON tool responses/refusals, valid partial-JSON streams,
  parallel IDs/indices, exact final arguments, real usage, bounds and failures.
- [x] Parent: official Anthropic SDK create/stream/tool-result loops and an
  isolated Claude Code Read/Write loop against the fake upstream; record versions.
- [x] Integrate partial export reporting and agent changes; inspect every caller.
- [x] Focused independent protocol and lifecycle/export review; reproduce and
  correct confirmed regressions. Parent completed integration after worker quota
  exhaustion, including scoped-control denials and cache-count translation.
- [ ] Complete native Codex Security diff workflow. Launch attempted but no scan
  was created: ancestor C:\.git causes the helper to reject installed Git. See
  AGENT_COMPATIBILITY.md; original sealed scan is unchanged.
- [x] Full normal/race/vet/build, JavaScript, native Linux and formatting checks;
  refresh Graphify and durable context/compatibility matrix. No commit/push.

Initial live attempt: no local token, no upstream requests. After the user saved
it, three authorized inference requests exposed live tool/image failures despite
successful discovery and measured usage. See LIVE_VAL_CHECK.md. The budget is
exhausted. The user then authorized six additional attempts: Sonnet's undeclared
search/knowledge calls were identified, direct routing returned 403, a request
mode did not fix injection, and val-gpt-4o passed live vision. All nine attempts
are used. Recorder and clearer denial diagnostics are tested; no speculative
production route/mode workaround was applied. Further tool-loop testing needs a
new explicit quota budget.
SDK 2.3.0 and Claude Code 2.1.310 final-source fake-upstream loops passed.
Windows/Ubuntu native lifecycle passed; Darwin arm64 build only.

Review focus: orphan/duplicate tool IDs, integer precision, fragment restatements,
oversize arguments, interleaved text/calls, poisoned URLs, prompt cache claims,
missing usage versus measured zero, unfamiliar process IDs, partial exports.

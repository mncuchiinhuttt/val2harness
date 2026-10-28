# Sonnet tool investigation

Goal: explain and repair Sonnet tool/image behavior where the local gateway can
enforce the client's intent, without executing unexpected server tools or
modifying account/model settings. Existing local changes remain uncommitted.

## Evidence and approach

The completed scoped Codex Security scan
`92903b28-d9ef-4127-82cc-9d92be491088` reviewed all 16 proxy files offline and
reported no vulnerabilities. Its native generated report is retained separately;
it is a source snapshot, not live Val compatibility proof. Independent review
confirmed declared tools are forwarded and undeclared responses are rejected.

First-party Open WebUI v0.8.2 source establishes a precise related mechanism:
native mode wins when either model or request selects it; native built-in
resolution replaces caller tools. PR 21555 adds a caller-tools exemption.
Current source has since changed mode precedence. Val's deployed implementation
is not available, so these are version-specific mechanisms to test, not proof
that the exact public revision is deployed.

The initial explicit zero-tool/none candidate received live HTTP 400 and was
reverted. The selected correction registers the same validated client functions
in both ordinary `tools` and URL-free `tool_servers: [{specs: ...}]`. Controlled
live experiments restored Sonnet's declared client calls. An additional stream
fix permits Val's empty post-finish envelope while retaining substantive-tail,
identity, usage and tool validation. SDK JSON and image-assisted SSE tool/result
loops passed. See [SONNET_FIX.md](../../SONNET_FIX.md) for the ledger and limits.

The remaining Claude Code failure showed reasoning fields. Its effort hint was
incorrectly translated to upstream `reasoning_effort`; the latest local fix
validates/discloses the unsupported hint instead. A separately authorized final
two-request live Claude Code scoped Read/result/final-marker loop passed.
The CLI helper and recorder now prevent automatic retry
attempts from silently consuming the next live budget.

## Work and acceptance

- [x] Complete independent current-source security baseline and architecture
  review, preserve native checkpoints, seal report against unchanged snapshot.
- [x] Retrieve exact first-party native-mode precedence, replacement branch and
  fix; distinguish source facts from deployed-service hypotheses.
- [x] Reject/revert the failed automatic no-tools/none candidate.
- [x] Reproduce missing native client registration, implement the same validated
  schema channel and preserve undeclared-tool denial.
- [x] Reproduce empty post-finish envelope rejection and permit only empty tails
  before measured usage; verify the live streaming image/tool/result loop.
- [x] Obtain the additional 12-attempt authorization and enforce the total.
  All 12 are now used, 21 historical attempts including prior batches.
- [x] Review registration/tail patches independently; full Go tests, vet and
  proxy/client race checks passed at that checkpoint.
- [x] Add red/green effort-translation and recorder retry/budget regressions;
  focused Go and nine Python recorder tests passed.
- [x] Finish fresh full checks and focused review after the effort correction.
- [x] Complete actual-CLI fake-upstream Read acceptance and verify no automatic
  retries on a failing inference.
- [x] Obtain a new bounded two-request quota authorization before further live requests;
  verify the latest correction with Claude Code Read/tool-result acceptance.
- [x] Refresh compatibility/context records and offline Graphify. Universal
  compatibility and standalone Sonnet no-tool behavior remain unestablished.

Approved and exhausted additional ceiling: **12 request attempts total across all runs**,
with no retries, at most 128 requested output tokens per request, tiny fixed
tool schemas/markers and a generated red PNG. Stop when enough evidence exists;
reserve requests for validation rather than exhausting them automatically. The
last CLI probe retried once unintentionally; it still stayed within the recorder
ceiling. The helper now disables CLI retries, and the recorder denies duplicate
canonical payloads and exhausted budgets with permanent local HTTP 400.
The already-denied direct provider route is not retried. No chat IDs, remote tool
execution flags, repository files, real conversations or server-setting writes.

Final authorization: **two additional attempts**, now both used successfully by
the current compiled gateway and Claude Code 2.1.310. Total historical usage is
**23 attempts** across four approved batches; no remaining live authorization.
The fresh local Write fixture initially failed because bare mode omits Write
and Write(path) rules never match file permissions. With full tool mode restricted
to the known synthetic localhost fixture and Edit(./result.txt), actual CLI
Read/Write acceptance passed in three turns with no permission denials. This
does not establish live editing support.

Acceptance requires the SDK to receive the declared tool with the exact marker,
return its synthetic result and finish normally for a full tool-loop claim.
Image acceptance requires identifying the generated color. A 200 response alone
is insufficient. The recorder enforces each run's request ceiling; the parent
also tracks the authorized total across runs. Fake upstream proof and source
review remain separate from live proof.

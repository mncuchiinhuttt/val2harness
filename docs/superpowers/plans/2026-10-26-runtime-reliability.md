# Runtime reliability implementation plan

> **For agentic workers:** Use Superpowers executing-plans inline, test-driven
> development, and a fresh focused code review before final verification.

**Goal:** Correct the remaining audited credential races and silent protocol
failures without claiming unsupported agent capabilities.
**Architecture:** Keep the Go gateway and existing APIs. Synchronize credentials
at their shared owner, validate the implemented Messages subset, and share bounded
SSE framing between the adapter and terminal chat.
**Tech stack:** Existing Go module and standard library; no new dependencies.
**Spec:** BUG-03/04/05/07/08/09/17 in `docs/BUG_AUDIT.md` and the decisions below.

## Decisions and constraints

Continue the accepted debugging work on the current branch, preserving the prior
uncommitted configuration/authentication increment. Use synthetic upstreams and
temporary homes only. All user data remains user-owned. No publication, telemetry,
new service, or real account access. Ordinary correctness fixes do not require
reopening the sealed Codex Security scan or relabeling its findings.

Reject request features the text-only Anthropic adapter cannot translate, instead
of silently deleting tools, images, thinking, or controls. Forward max_tokens and
temperature. Use exact model aliases only, pass other model IDs through for
upstream validation, and report the actual upstream model when supplied. Preserve
HTTP failures and Retry-After, report malformed/truncated data as failures, and use
SSE error events once output starts. Encode events as JSON, never string-insert
model names. Translate real usage and omit unavailable usage explicitly (document
the partial protocol contract and possible strict-client incompatibility).

SSE parsing handles LF/CRLF/CR, comments, optional colon spacing, multiline data,
bounded events, read errors, and incomplete final events. Do not report success
without an upstream finish signal. Terminal chat must not append failed partial
responses to its conversation. OpenAI pass-through remains available unchanged.

## Task 1: Shared credential state and empty model lists

- [x] Add a race reproduction covering concurrent refresh, reads, and every
  upstream request method, plus wrapped/raw empty-model-list tests. Observe red.
- [x] Synchronize all token reads/writes in `internal/valclient/client.go` and
  remove redundant mutable token copies in live server/web paths. Accept an
  explicitly present empty `data` array; reject malformed wrappers/null.
- [x] Run focused normal/race tests and relevant web/proxy controls.

## Task 2: Honest Anthropic request/response behavior

- [x] Add fake-upstream regressions for unsupported inputs, max_tokens/model
  preservation, 401/429/500, malformed JSON, real/missing usage and stop reasons.
  Observe failures before implementation.
- [x] Separate request conversion and response conversion behind small helpers.
  Reject unsupported behavior before upstream work. Check HTTP status and decode
  failures before sending success. Keep bodies/errors bounded and avoid echoing
  credentials or arbitrary upstream error pages.
- [x] Verify successful text controls, missing-token denial, method aliases,
  exact aliases and unusual model strings/JSON escaping.

## Task 3: Bounded and truthful streams

- [x] Add parser/adapter regressions for no-space data, CR/LF, multiline frames,
  70KB events, malformed/oversize frames, disconnects, missing completion and
  upstream error events. Observe red.
- [x] Implement `internal/sse/` framing and streaming Messages translation;
  normal completion requires a real finish reason, failures emit error without
  message_stop, usage is real or absent. Respect cancellation/write errors.
- [x] Reuse framing in terminal chat with focused output/history tests; a failed
  turn does not enter history as a successful reply.

## Task 4: Review and handoff

- [x] Obtain one fresh focused review using the Superpowers review workflow;
  examine credential boundaries using Codex Security methods where relevant.
  Reproduce concrete findings and correct them in scope.
- [x] Run full Go tests/race/vet/build, JavaScript checks and native Linux package
  tests. Update the audit checkpoint, project context, protocol limitations and
  Graphify index. Leave all changes local and uncommitted.

Review focus: concurrent token-save/status/request operations; exact versus
near-match model IDs; zero versus unavailable usage; error before/after stream
headers; bounds and incomplete events; terminal history on a failed turn.

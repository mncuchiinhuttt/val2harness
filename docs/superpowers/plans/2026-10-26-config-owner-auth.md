# Configuration preservation and owner authentication implementation plan

> **For agentic workers:** Use Superpowers executing-plans task by task, with
> test-driven development and one fresh Codex Security bypass/regression review.

**Goal:** Protect user configuration and make authenticated local use practical.
**Architecture:** Shared safe file operations below all adapters; one HTTP owner
boundary with temporary dashboard/import tickets and normal API credentials.
**Tech stack:** Existing Go 1.26, standard library, existing x/sys and YAML modules.
**Spec:** `docs/superpowers/specs/2026-10-26-config-owner-auth-design.md`.

## Constraints and review focus

No new online service, telemetry, dependency, real credentials, or user-setting
mutation during tests. Preserve MIT attribution. Existing instruction/docs changes
remain intact. Include null/wrong-shaped/multi-document inputs, existing unsafe
backup permissions, concurrent updates, origin/Host aliases, and expired/replayed
credentials in the relevant task's tests. Do not fix unrelated audit findings.

## Task 1: Preserve configuration

- [x] Add adapter regressions in `internal/sync/preservation_test.go`; run them
  against unchanged writers and record expected data-loss/backup failures.
- [x] Add `internal/configfile/` for validated update, first backup, atomic private
  writes and restore with cross-process serialization. Windows/Unix helpers own
  platform locking/replacement. Add focused helper tests for failures/concurrency.
- [x] Route OMP/Pi/OpenCode/Continue writes and restore through that boundary;
  validate root/provider/model container types and preserve unrelated settings.
- [x] Run adapter/helper tests, Unix build/permission checks where supported,
  and `go test ./...`; record remaining platform limits.

## Task 2: Usable owner authentication

- [x] Add failing denial/control tests in `internal/proxy/auth_test.go` and config
  tests for persistent random keys without persisting environment secrets.
- [x] Implement shared local authorization/Origin/Host controls, one-use tickets,
  and expiring dashboard bearers in `internal/proxy/auth.go`. Public assets and
  minimal health are inert; inference/management aliases remain protected.
- [x] Wire startup key provisioning, CLI/TUI authenticated dashboard opening,
  browser unlock/sessionStorage, real-key environment settings and scoped Val
  import. Add flow tests and browser validation using isolated settings.
- [x] Challenge alternate inputs and legitimate flows, run full tests/race/vet,
  JS syntax and builds, then obtain the one independent candidate review.
- [x] Resolve concrete review findings, update audit/context/status and refresh
  Graphify. Leave changes reviewable and uncommitted; no push or publication.

Execution choice: inline implementation to avoid repeated implementer contexts;
the security skill supplies independent pre-patch investigation and final review.
These are implementation decisions within the user's accepted fix scope.

Completed locally on `codex/preserve-config-owner-auth`. Verification and review
outcomes, upgrade steps, and explicit limits: `docs/CONFIG_AUTH_FIX.md`. No commit,
push, deployment, or publication was performed.

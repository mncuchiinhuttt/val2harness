# Daemon lifecycle correction

Recorded 26 October 2026 on `codex/preserve-config-owner-auth`, based on upstream
`6999a37c298137e386567dfabee38d8c9caf83d0`, alongside the uncommitted configuration,
owner-authentication, and protocol work. This increment addresses BUG-10 and the
lifecycle failures reproduced while tracing it. No dependency was added.

## Root cause and observed baseline

`IsProcessAlive` used `os.Process.Signal(0)` on every platform. Go 1.26's
`src/os/exec_windows.go` returns `syscall.EWINDOWS` for signals other than Kill;
therefore even the current Windows process was reported dead. `GetStatus` then
deleted its PID record. The first regression run reproduced that failure and:

- An alive but unhealthy recorded process was replaced by another start.
- Stop reported success after killing a process identified only by a numeric PID.
- Starting with a new isolated home failed because the log directory was absent.
- Early child exit was reported only after the full health polling delay.
- Unclosed parent log handles prevented temporary-directory cleanup on Windows.

## Implemented behavior

Windows liveness opens a synchronization handle and performs a zero-duration
`WaitForSingleObject` check. Access denied is treated conservatively as alive.
Unix retains signal-zero checks with permission-denied handling and rejects
nonpositive PIDs; Linux additionally recognizes exited zombie processes.

New private PID records contain the PID, listen address, process identity and a
random per-daemon control capability. The child receives that capability through
a dedicated environment variable, removes it from its environment, and never
persists it in normal configuration.
Windows identity combines creation time (`GetProcessTimes`) and executable path
(`QueryFullProcessImageName`). Linux uses `/proc/<pid>/stat` start ticks and the
executable link; other Unix systems use native `ps` start time and command.
Stop first verifies an authenticated HTTP handshake matching the child's PID
and identity, requests graceful shutdown and waits for exit. It uses the recorded
address and capability even after normal settings change. Windows fallback
verifies identity and terminates through the same opened handle, preventing a
reused numeric PID from redirecting termination. Unix refuses numeric-PID
termination when authenticated graceful control is unavailable. A provably reused
PID record is retired under the lifecycle lock without touching the unrelated
process; unreadable identity fails conservatively. Stop refuses to terminate the
current management process.

The capability permits only GET/POST on the literal `/api/local/daemon-control`
leaf. GET confirms readiness and identity; POST requests shutdown. Host/Origin
checks still apply. Owner keys, browser sessions, cookies, query parameters and
x-api-key do not authorize this endpoint. Conversely, the capability cannot
authorize inference or other management endpoints. Encoded-route and
capability-as-owner regression tests cover both boundaries.

Start, stop, and stale status-record cleanup serialize through an OS-backed
lifecycle file lock. Start refuses
any recorded live process even if its HTTP check fails or settings changed. It
checks address availability, creates the private directory/log, starts the child
hidden on Windows (`HideWindow` and `CREATE_NO_WINDOW`), atomically persists its
identity, closes the parent's log copy, and reaps/releases its process with Wait.
PID persistence failure stops the child instead of leaving an unmanaged gateway.
Authenticated readiness is checked immediately, then against a deadline and child-exit events;
there is no unconditional startup/shutdown sleep. HTTP responses are closed on
all status paths, wildcard addresses use a connectable loopback address, and
health requests bypass environment HTTP proxies and do not follow redirects.

An early child exit removes its record and names the log. A live child that
fails readiness retains its PID and returns a recovery error. Stop confirms exit
before removing the record and propagates termination/removal errors. The stored
identity controls stop, so changes to the configured address/key do not prevent
recovery. Public HTTP health describes the gateway listener; it does not prove
owner authentication, account validity, or upstream/model availability.

## Migration and caller contract

Function signatures are unchanged. Legacy integer PID records still support
status and prevent duplicate starts while alive. They cannot prove ownership,
so stop asks the user to stop that original process manually. Dead/invalid
records are removed; the next start writes the identity record.

`GetStatus` returns `(false, livePID)` for a live process whose address, health,
or identity cannot be verified. Callers should distinguish that state from
`(false, 0)`. CLI and TUI now show “Needs attention” and preserve the live PID for
recovery instead of falsely reporting a stopped server.

## Verification actually performed

Native environment: Windows amd64, Go 1.26.0; existing MinGW compiler available.

- Observed the failing Windows regression tests before changing production code.
- `go test ./internal/daemon -count=1`: passed after the fixes.
- `go test -race ./internal/daemon -count=1`: passed.
- `go vet ./internal/daemon`: passed.
- A later race rerun exposed a test cleanup that ignored `FindProcess` failure
  after successful stop. Cleanup now calls verified `StopDaemon`; final normal
  and race reruns passed.
- The final integrated suite passed `go test ./...`, `go test -race ./...` and
  `go vet ./...`. Linux amd64 package tests executed under Ubuntu/WSL and passed;
  Darwin arm64 application build passed, without native macOS execution.
- Built the real gateway executable into a temporary directory and exercised
  native CLI `start`, `status`, duplicate `start`, and `stop` with separate
  temporary HOME/USERPROFILE directories. One case used a wildcard custom port
  and supplied synthetic key; another used loopback and a freshly provisioned
  key. Both reported running with the child PID. A valid owner key obtained a
  ticket (HTTP 200); an incorrect key received HTTP 401. Changing both settings
  while the child remained alive refused a duplicate start without replacing
  its PID. Stop then terminated that original process and removed its record.
  Stopped status and OS process absence were checked in both cases. The final
  `tests/daemon-cli-check.py` repeated these cases against the compiled Windows
  and Linux binaries, including scoped capability denials and changed-settings
  graceful shutdown; both platforms passed.

Automated subprocess tests use the test executable as a synthetic local HTTP
server; they never load a Val account. Coverage includes current/nonpositive
PIDs, malformed/dead PID cleanup, alive-but-unhealthy PID retention/refusal,
legacy stop refusal, mismatched process identity, isolated start/status/stop,
wrong address, occupied address, mkdir failure, PID persistence failure, child
exit, simultaneous starts, and Windows hidden-process launch attributes.

Native acceptance used synthetic tokens/keys and an unreachable loopback
upstream (`http://127.0.0.1:1`), with no inference/model requests. Real account,
harness settings, and home configuration were not used. See
[AGENT_COMPATIBILITY.md](AGENT_COMPATIBILITY.md) for the integrated checkpoint.

## Limits and next verification

macOS runtime behavior remains unverified. Non-Linux Unix identity has
second-resolution start times and platform `ps` availability/format assumptions.
New daemons use a random capability handshake to distinguish the child from an
unrelated listener. Legacy daemons, or unresponsive Unix daemons whose control
endpoint cannot be authenticated, require manual recovery; no unsafe numeric
signal fallback is attempted. Windows verification and fallback termination
share one handle. PID/settings privacy on Windows still depends on the containing
directory ACLs. No clean security audit or universal compatibility is claimed;
the native diff-scan launch blocker is recorded in AGENT_COMPATIBILITY.md.

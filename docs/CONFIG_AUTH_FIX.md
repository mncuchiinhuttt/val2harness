# Configuration preservation and owner access

This checkpoint predates the runtime continuation. See [RUNTIME_FIX.md](RUNTIME_FIX.md)
for later corrections to protocol handling and token races; it supersedes those
items in this checkpoint's remaining-work list.
The later [agent compatibility checkpoint](AGENT_COMPATIBILITY.md) also records
tool/image support, lifecycle and export fixes, and final verification limits.

26 October 2026. Working branch: `codex/preserve-config-owner-auth`, based on
`6999a37c298137e386567dfabee38d8c9caf83d0`. Changes are local and uncommitted.
Scope: the user's accepted configuration-integrity and usable owner-authentication
increment following `BUG_AUDIT.md`. Superpowers supplied the implementation,
debugging, and verification process; Codex Security supplied the independent
pre-patch investigation and one final bypass/regression review.

## Result and code flow

Configuration changes now share `internal/configfile/`: hold an OS file lock,
read a regular file, parse and validate it, compute the new document, retain the
first original `.bak`, and publish through a private temporary file and atomic
replacement. A failed parse or backup leaves the original content unchanged.
Existing backup contents are retained. On Unix, used settings/backups lose group
and other permissions; stricter owner permissions are preserved on unchanged
files. Locks also serialize separate CLI processes. Symlinks/special files are
refused; linked parent directories and hostile same-owner filesystem mutation are
outside this boundary. Power-loss durability of the directory entry is not tested.

Pi, OpenCode, OMP, and Continue use this path. Root/provider/model shapes, duplicate
YAML keys, and extra documents are checked. JSON unknown fields and numeric
precision are retained. YAML comments are retained where nodes survive; aliases
are expanded to values before replacing nodes so a personal setting cannot lose
an anchor. The final YAML is parsed again before writing. Continue only replaces
matching generated entries at the same exact gateway base URL; entries from an
old port remain for manual cleanup. OpenCode sync and restore share one path
resolver and refuse two simultaneous settings locations. Application settings
saves preserve unknown fields too. Older overwritten backups cannot be recovered.

`config.Load` provisions a stable random local key before applying environment
overrides, so merely creating the key does not persist environment Val credentials.
Failed configuration loads stop their CLI/TUI/server callers. HTTP requests then
pass `Server.ServeHTTP` through `auth.go` before reaching any model or management
handler. A trusted Host and permitted browser Origin are checked first; protected
routes require the local key or a browser bearer session. Both Authorization
Bearer and Anthropic `x-api-key` work. The public HTML/assets contain no account
state, and `/healthz` exposes only a minimal status.

`val2harness open` and the terminal menu authenticate to obtain a five-minute,
single-use dashboard ticket. JavaScript clears the fragment before exchanging it
for a separate eight-hour token stored in per-origin sessionStorage. Restarting
the process invalidates tickets/sessions. Manual key entry clears its input and
does not store the permanent key as a session. There are no ambient auth cookies.
The Val bookmarklet uses a different five-minute, single-use ticket accepted only
for POST token import from the configured upstream origin; it cannot authorize
inference, dashboard sign-in, or sibling management endpoints. Token saves report
errors and update memory only after disk success. The manual token button now works.

Environment output and generated harness settings use the actual local key, with
shell quoting and connectable loopback addresses for wildcard listeners. A local
key grants owner authority including settings and export; share it only with
trusted tools. No new service, dependency, telemetry, or upload was introduced.
User data remains user-owned; intended model calls still go to configured Val.

## Regression evidence and review

Before each primary change, targeted tests demonstrated the original failure:
malformed documents were overwritten, first backups were lost, default/foreign
origin requests reached protected handlers, key provisioning was absent, and a
locked dashboard requested owner data. Native Linux tests later demonstrated an
unchanged settings file retaining mode 0644; that path was corrected too.

The one fresh final reviewer reported three concrete issues: mixed-case JSON
restore selected the YAML validator; a retained YAML alias could reference a
removed anchor; and Continue rejected alias entries resolving to mappings. Each
was reproduced in isolated tests and corrected. The anchor test also exposed the
same sibling issue in OMP. An additional regression demonstrated unknown settings
being discarded on explicit Save; merging under the same lock corrected it.
The reviewer reported no concrete surviving gateway authentication bypass. That
is scoped review evidence, not a guarantee against every vulnerability.

The regression suite covers all four adapters, backup failures, invalid container
types, original restoration, ambiguous OpenCode paths, cross-process updates,
Unix private permissions, symlink refusal, stable key creation, unknown settings,
route aliases, Host/Origin denial, header authentication, ticket purpose/replay/
expiry/concurrent redemption, session expiry, and token-save failure behavior.

Final verification of the revised candidate passed:

- Windows amd64, Go 1.26.0: `go test ./...`, `go test -race ./...`,
  `go vet ./...`, and `go build` (binary written outside the repository).
- `node --check internal/web/static/app.js`, `node tests/dashboard-auth.cjs`,
  and `git diff --check`.
- Native Ubuntu/WSL execution of Linux-amd64 Go test binaries for configfile,
  sync, config, proxy, and web. Tests used Linux `/tmp` for settings and exercised
  mode 0600, permissive umask 000, existing readable backups, restore, symlink
  refusal, and cross-process serialization. They were cross-compiled on Windows;
  no Go installation or application deployment inside WSL was required.
- A Darwin/arm64 build succeeded; macOS was not executed.
- Headless Microsoft Edge, driven by Playwright against an opt-in Go fixture:
  ticket sign-in and URL cleanup, authenticated status/models/streaming,
  Pi sync/restore, private environment settings, locked reload, wrong-key denial,
  and manual unlock. See `internal/web/browser_fixture_test.go` and
  `tests/browser-auth-check.js`. The fake upstream and temporary harness settings
  exercised the real embedded interface; no live Val account/browser import,
  Microsoft sign-in, or installed harness was used. The browser fixture was stopped
  and its browser session closed after the check.

All fixtures used synthetic credentials and isolated homes. No real account data,
chat history, or existing harness configuration was read or changed. The final
security review was one independent read-only cycle, followed by focused red/green
regressions and the complete checks above.

## Upgrade and remaining work

Restart an older running gateway with the new build. Run `val2harness open` to
sign in; existing explicit keys stay valid, while an empty key is replaced once.
Resync existing harness configurations and regenerate shell settings with
`val2harness env`; the old placeholder key is no longer a default credential.
Do not paste that private output into public bug reports. Keep existing `.bak`
files until you have inspected their contents and made your own recovery copy.

This addresses BUG-01/02 and SEC-01/02 within the documented boundary and corrects
BUG-06's generated-key/header paths. It also addresses token-save error handling,
same-port Continue deduplication/preservation, and the manual token control.
The sealed scan and original audit remain unchanged as historical evidence.

Still unresolved: Anthropic tool/content/error handling, client token data race,
Windows daemon lifecycle, export correctness, upstream model metadata/availability,
three other unwired dashboard controls, real harness/version interoperability,
and installer integrity work described in the audit. Passing the new race suite
does not cover or fix the separately reproduced client-token race. Windows ACL
hardening and macOS runtime behavior have not been verified. Arbitrary reverse
proxy hostnames and cross-origin browser integrations need deliberate configuration
work; native API clients with trusted address/credentials remain supported.
Live bookmarklet import also depends on browser policy allowing the Val page to
reach the local gateway; manual token entry remains available.
Feature research has not started. This increment is not a release certification.

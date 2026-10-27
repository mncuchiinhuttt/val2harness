# Preserve user configuration and require owner authentication

Scope accepted by the user: prepare fixes for configuration preservation and
usable owner authentication. Existing audit: `docs/BUG_AUDIT.md`, BUG-01/02/06,
SEC-01/02. This is a local Go implementation, without new services/dependencies.

## Decisions

Preserve the first original `.bak`; later syncs never replace its contents.
Parse and validate the expected document/container shapes before backing up or
writing. Reject symlinks, special files, ambiguous YAML documents, and unsafe
backup states with a useful error instead of replacing them. Share owner-only,
atomic writes and serialized update/restore operations across every adapter.
Tighten existing readable backup files when used. Keep unrelated provider data.

Require a persisted random local connection key by default. Existing explicit
keys remain valid. A key is an owner credential, distinct from the Val token.
Public dashboard assets contain no account data; public health returns no secrets.
Privileged APIs require a key or expiring dashboard bearer session. Reject
unrelated browser origins and unexpected Hosts, including on route aliases.

The CLI/TUI obtains a short-lived, one-use dashboard ticket using the local key.
The dashboard exchanges that ticket for an in-memory session and keeps its bearer
in per-origin sessionStorage. Clear tickets from the URL immediately. Avoid
ambient cookies and permanent credentials in URLs/logs. Manual key entry remains
a fallback. Val bookmarklet import uses a separate one-use ticket accepted only
from the configured upstream origin and only for token import.

Chosen over: unauthenticated loopback (does not close SEC-01); key-only gating of
all HTML (breaks normal browser use); a new account/cloud identity service
(unnecessary cost and user-data transfer); cookies alone (ambient cross-port
localhost authority). All account data stays under the user's control.

## Compatibility and acceptance

Preserve CLI/API operation, bearer and Anthropic x-api-key clients, embedded assets,
custom ports, explicit existing keys, and intended Val token transfer. Update
generated harness and shell settings to use the real local key. Health and the
authenticated open flow must work together without reviving unrelated daemon bugs.
Do not persist environment-supplied Val credentials merely to create a local key.

Prove malformed/wrong-shape configs and failed backups do not change original
bytes; repeated sync/restore recovers the first baseline; writes/backup permissions
are private on Unix; all adapters share the enforcement. Prove default denial,
authorized requests, aliases, origins, Hosts, ticket replay/expiry, session expiry,
scoped token import, dashboard unlock, and real-key settings. Use synthetic data.
Report Windows/Unix/browser verification limits; preserve the sealed audit report.

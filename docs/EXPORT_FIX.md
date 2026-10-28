# Export integrity correction

Date: 26 October 2026. Branch: `codex/preserve-config-owner-auth`, based on
`6999a37c298137e386567dfabee38d8c9caf83d0`, with the earlier configuration,
authentication, and runtime corrections still uncommitted. This record covers
`internal/exporter/` and the focused `Client.GetChat` path/JSON-decoding changes.
It does not certify live Val compatibility or release readiness.

## Root causes and resulting behavior

BUG-14 used `id[:8]` directly in a filename. A one-character ID panicked;
separators could escape the chosen directory, and distinct IDs sharing a prefix
and title overwrote each other. Export names now use `chat-`, a full SHA-256 hash
of the ID, and a bounded ASCII title slug. Short IDs work; separators and Windows
device names never become filenames. The hash avoids prefix and case-folding
collisions without exposing raw IDs in filenames. It is a stable identifier,
not encryption: source IDs and all other returned data remain in the JSON.

`GetChat` also interpolated an ID into its URL. `?`, `#`, and percent escapes
could change the query or requested resource. It now rejects empty IDs,
dot-segments, slash/backslash, and ASCII control characters before making a
request, and escapes other opaque IDs as one URL path segment. The policy does
not assume Val IDs must be UUIDs. The JSON decoder uses `UseNumber`, preventing
large integers such as `9007199254740993` from being rounded through `float64`.

BUG-15 ignored detail-fetch and both write errors, and counted the chat anyway.
The public `ExportAllChats` signature is unchanged. It now returns:

- The number of chats whose **JSON and Markdown both** finished writing, syncing,
  and closing.
- The chosen output directory, including on failure; empty input means `exports`.
- An aggregated error naming each failed chat and operation. A nonzero completed
  count can accompany an error. Cancellation is discoverable with
  `errors.Is(err, context.Canceled)` and stops subsequent work.

Independent chats continue after individual failures. Successfully saved files
remain available, including a single format from an incomplete chat. Files may
be incomplete after a write/sync/close failure and are never counted as a
completed pair. Errors explicitly say that any saved files are retained.

## Filesystem and recovery choices

The exporter pins the selected directory using Go's `os.OpenRoot` and exclusively
creates single-component output names. Unix uses `Root.OpenFile` with `O_EXCL`;
Windows uses handle-relative `NtCreateFile` with `FILE_CREATE` and a protected
current-user-only DACL supplied at creation. Existing
files, hard links, directories, symlinks, and special files are refused; unrelated
content is never replaced. The explicitly selected output directory may itself
be a user-selected link. No nested output paths are derived from upstream data.

New run directories use mode `0700` and files `0600` on Unix. Windows run folders
and files receive their private ACL atomically, even beneath a broadly readable
parent; a filesystem without persistent ACL support is refused. Existing parent
directories/files keep their ACLs. Windows names reject ADS, device names and
path components; the pinned root handle avoids absolute-path creation races.
`configfile.Write` is intentionally unsuitable here: its atomic
replacement contract would replace unrelated existing regular files. The
exporter's exclusive creator therefore shares no overwrite/backup behavior with
settings writes and introduces no dependency.

Default CLI, terminal menu and dashboard exports now call `NewRunDirectory` and
use a fresh private `exports/run-*` directory for every attempt. An explicit
`val2harness export <directory>` selects that exact directory. Exporting again
into an explicit directory reports conflicts and preserves previous files;
choose a fresh directory for a retry. CLI/TUI show the completed count, output
path and any partial error. The dashboard checks HTTP status and displays partial
counts without a success toast.
The pair is not a multi-file transaction: interruption, disk exhaustion, or a
failed second write can leave one saved format or a partial file. Existing paths
are never automatically removed to make a retry succeed. File syncing does not
establish directory-entry durability across every filesystem or power loss.

## Markdown and source preservation

The old renderer required `chat.messages` to be a map even before examining
`chat.history.messages`, so ordinary arrays and history-only documents produced
only a title. Iterating maps also changed message order between runs.

The renderer accepts a root chat object or an object wrapped in `chat`, renders
nonempty `messages` arrays in their supplied order, and otherwise reads
`history.messages` or `messages` maps. Map histories render every node once in
deterministic parent-first order using `parentId`, with lexicographic IDs as
tie-breakers. Broken links and cycles terminate safely; they do not establish a
chronological ordering. All branches remain in the source JSON, including when a
current-conversation array supplies the Markdown view. The renderer does not
claim to reproduce Val's current-branch UI.

Text strings and text content blocks appear in Markdown. Other blocks and
unrecognized content explicitly refer to the companion JSON. An unknown schema
produces a visible explanatory note rather than a silent empty transcript.
JSON retains all returned fields and exact JSON numeric values; formatting/key
order can change. Attachments are not downloaded. These supported shapes are
synthetic regression fixtures; no live conversation schema was fetched.

One important flow is now: list item -> validate/escape its ID -> fetch its
complete JSON with precise numbers -> generate a hashed filename and ordered
Markdown -> exclusively write both private files -> increment the completed
count only if both writes succeed.

## Verification and limits

Superpowers systematic debugging and test-driven development established red
regressions before implementation: short-ID panic; false completion after fetch
or filesystem failures; overwritten regular/hard-linked files; swallowed
cancellation; lost transcript shapes; URL query/fragment injection; and rounded
large JSON numbers. All requests use loopback fake upstreams with synthetic data
and temporary output directories. No real Val service, user conversations,
harness settings, or credentials were used.

Executed on Windows with Go 1.26.0:

- `go test ./internal/exporter ./internal/valclient`: passed.
- `go test -race ./internal/exporter ./internal/valclient`: passed.
- `go vet ./internal/exporter ./internal/valclient`: passed.
- `go test ./...`: passed across the working tree at that checkpoint.

The fixtures cover short IDs, duplicate prefixes, case-distinct IDs, hostile
separators, Windows device names, deterministic names/order, both saved formats,
large integer preservation, multiple fetch failures with partial success,
existing files/directories/hard links, unusable output directories, and canceled
requests. The symlink fixture is present but **skipped on this Windows host**
because link creation requires an unavailable privilege. Unix permission and FIFO
fixtures are present but are not Windows runtime evidence. The exporter test
binary also cross-compiled successfully for Linux/amd64, including the FIFO
fixture. In the final integrated run, all Linux exporter tests executed under
Ubuntu/WSL, including symlink, FIFO and mode checks, and passed. Windows tests
reproduced inherited ACL exposure before the fix, then verified protected
user-only ACLs, unchanged existing ACLs and private fresh run directories. The
complete repository normal/race/vet suites passed. Go API tests verify partial
counts and repeated fresh exports; Node checks verify honest error toasts.
Disk-full, forced sync/close failures,
hostile concurrent filesystem mutation, and live Val response variants were not
runtime-tested.

Caller integration is implemented. See
[AGENT_COMPATIBILITY.md](AGENT_COMPATIBILITY.md) for integrated evidence,
Graphify context and the native security-review launch limitation.

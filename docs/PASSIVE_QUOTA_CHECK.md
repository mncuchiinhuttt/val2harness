# Passive quota and main integration

Verified on 28 October 2026 on `codex/passive-quota-checker`.
Source checkpoint: `60907a5b6466796c20dc35a58b116f341928ec4b`.
The branch starts at published `bf9ad72` and incorporates main `d8e4760` through
merge `04a1d32`. Later checkpoint documentation does not change this source.

## What users can do

Build this development branch, then start/open the gateway normally. The dashboard
shows the last observed Val result, its time, and measured gateway-session usage.
Refresh reads memory; **Clear observations** resets that session. The CLI reads
the same protected local cache:

```powershell
.\val2harness.exe quota
.\val2harness.exe quota --json
```

No background inference is used to check quota. Remaining account allowance,
account limit and reset time are explicitly unknown. A successful observed
response does not promise the next request will succeed. Website/other-client
usage is outside this gateway's observations. Missing usage is distinct from
measured zero; malformed, cancelled and partial responses stay unmeasured.

Observations are bounded and memory-only: 256 recent records, 256 named models
plus an overflow group, 400 dates, 128 listeners and bounded display metadata.
They contain no prompts, replies, tool arguments, credentials or account IDs.
Restart, token change or explicit clear starts fresh. Legacy `usage.jsonl` files
are neither read nor deleted. No user daemon or installed executable was replaced
during this work; build/restart deliberately when adopting this branch.

## One flow through the code

Both completion handlers call `valclient.Client.ProxyChat`. It sends the ordinary
request once, then wraps the response with bounded JSON/SSE observation. Completed
metadata reaches that client's `analytics.Tracker`; quota, analytics and CLI views
read snapshots. Token and observation generation are captured together, so an old
request cannot refill history after a token change or clear.

Explicit `budget_exceeded` JSON errors returned as HTTP 200 become HTTP 429 for
both protocols, preserving the error body and Retry-After. Generic rate limiting
and credential rejection remain separate states. OpenAI bytes otherwise pass
through; Anthropic keeps its existing validated, buffered turn conversion.

Passive observation was selected over automatic probes because probes spend
quota and do not establish remaining budget. The authorized account-metadata
research found no authoritative remaining/reset contract; see
[REA_QUOTA_RESEARCH.md](REA_QUOTA_RESEARCH.md).

## Integration decisions and regressions

- Retained verified protocol, owner authentication, daemon identity/control,
  private exports and configuration preservation instead of older main variants.
- Kept main's heatmap, analytics, navigation and explicit update controls. Live
  analytics uses owner-authenticated fetch streaming. Status no longer treats
  token presence as verified connectivity or invents a 24-hour expiry.
- OMP selects one existing `models.yml` location, including `agent/`, and rejects
  ambiguity. Unverified 1M-context/zero-price claims were not adopted. Main's
  unused duplicate `safeio` helper was omitted.
- Port/token writes update only their own persisted field. Staging a port then
  saving a token preserves the staged address and disk owner key; the running
  listener remains immutable until restart. Tests reproduced the stale-write
  problem before correction.
- The merged updater requires bounded HTTPS metadata/assets, restricted derived
  redirects and a matching SHA256 before unique staging/replacement. Windows
  failures restore or retain a recovery copy. Tests use temporary executables.
  These are integrity checks within GitHub's trust, not independent signing.
- Review reproduced a misleading update-success/page-reload message. Its Node
  regression failed before `60907a5`, then passed: the dashboard now explains
  that installation needs a gateway restart; active requests are not restarted.

## Verification evidence

Final source was archived to a disposable directory with synthetic credentials,
temporary settings, cached offline Go dependencies and nonlocal HTTP blocked.

| Check | Result and scope |
| --- | --- |
| `go test ./... -count=1 -timeout=120s` | Passed all 13 tested packages; OMP has no package-local tests |
| `go vet ./...`, Windows amd64 build | Passed; archived build uses `-buildvcs=false` because of the host's ancestor Git root |
| Windows race checks | Analytics, Val client, proxy, web and version passed at `04a1d32`; Go files unchanged at final source checkpoint |
| Native Ubuntu / WSL | Analytics, Val client, proxy, web, version and sync passed |
| Darwin arm64 | Full application crossbuild passed; no native macOS execution |
| OpenAI SDK 3.38.0 | Offline JSON and streaming image/tool/result loops passed; four fake requests |
| Anthropic SDK 2.3.0 | Offline JSON/streaming parallel tools, results, images and measured usage passed |
| JavaScript | Syntax/dashboard checks passed, including unknown/zero usage, safe model rendering and explicit restart instruction |
| Browser at 1440/390 px | Authenticated live analytics, heatmap, refresh/clear and measured/missing usage passed; zero incidental inference or external requests |

During earlier test development, one negative-method check reached public GitHub
release metadata before its method guard was fixed. No Val call, binary download
or update occurred; subsequent management/browser fixtures block nonlocal HTTP.
Final fixtures were stopped. No new live Val requests or real harness writes
were made. All 32 historical inference attempts remain exhausted.

## Security review and release limits

Codex Security's supported standalone `assess-patch-risk` workflow assessed the
immutable 39-file range `bf9ad72..60907a5`. Patch SHA256:

```text
a19ba7735fce133fb18a67d62b4b7c8488bdb440769c36a76226fc0fdb5e926e
```

Its JSON passed the plugin validator. Recommendation: **human review required
before merging**; impact high, likelihood moderate, protection partial,
recoverability managed, confidence moderate. No additional unresolved
source-visible vulnerability was established in this scoped primary review.

The independent reviewer failed immediately due to account usage limits. The
previous native Diff preflight's ancestor Git-root/trust problem was unchanged;
no completed final-tree native scan or Cloud upload is claimed. Managed local
artifacts retain the patch, Markdown/JSON assessment and execution logs; portable
summary: [SECURITY_REVIEW.md](SECURITY_REVIEW.md). Historical sealed reports remain
bound to their original snapshots.

Not release-verified: real release assets/redirects or replacing a running
installed executable, native macOS, new live Claude Code/Val runs, exhaustive
models/harnesses and sustained load. Updater refusals fail before replacement
when metadata/checksums are absent or invalid. Remaining/reset information still
needs an authoritative provider contract, not a local estimate.

## Operation and recovery

Refreshing requires a running gateway and its owner key/session; unavailable or
locked gateways produce an explicit error. Clearing observations affects only
memory. A staged port takes effect after restart; subsequent token saves preserve
it. Explicit update checks contact GitHub, and installing an update requires a
gateway restart to use the new executable. Preserve settings/backups when
rebuilding or reinstalling a prior version; failed Windows rollback reports the
retained recovery-image path. No usage-history migration is required.

Next useful action: human review of this feature branch, then controlled release
and native macOS acceptance. Broader usability research and remaining initial
audit issues stay separate work.

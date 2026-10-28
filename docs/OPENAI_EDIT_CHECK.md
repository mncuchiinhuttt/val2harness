# OpenAI and editing checkpoint — 27 October 2026

Local uncommitted work on `codex/preserve-config-owner-auth`, atop
`6999a37c298137e386567dfabee38d8c9caf83d0`. This extends [SONNET_FIX.md](SONNET_FIX.md).
The user's running executable and persistent account/harness settings are unchanged.

**Result:** the official OpenAI SDK streaming image/tool/result loop passed live.
Claude Code live Edit execution also verified exact on-disk bytes, three turns,
zero permission denials and the returned random marker. Its v4 checker then
failed an unnecessarily strict final-reply formatting assertion; v5 removes only
that assertion and passes offline tests. V5 was not rerun live.

## Implemented and locally verified

- `valclient.Client.ProxyChat` now registers client function schemas for both
  protocol routes and direct callers. Anthropic-only duplication was removed.
  The same function objects appear in `tools` and URL-free `tool_servers.specs`;
  no new endpoint, credentials, chat/session state or tool execution is added.
- Caller options, image data, tool results, strict hints and large JSON numbers
  survive adaptation. Explicit `tool_servers` is retained, including empty/null.
  No-tool, malformed and custom-tool bodies retain upstream validation behavior.
  Custom/legacy tools are not claimed to gain native registry compatibility.
- Buffering input/adapted requests has a **32 MiB** limit. Overflow returns 413
  before upstream dispatch; Messages keeps its separate 8 MiB input limit.
  Response streaming remains incremental. Cancellation reaches upstream, and
  failed preparation never dispatches a partial request.
- Buffering initially made POST bodies replayable on 307/308 redirects. Codex
  Security identified this regression; failing direct-client/raw-route tests
  reproduced it. `GetBody` is now unset so private POST content is not replayed.
  Raw OpenAI returns the original redirect status/body; Messages reports an
  unsupported upstream redirect as failure. This also deliberately tightens the
  previous Anthropic redirect behavior. Existing 301/302/303 behavior is unchanged.
- OpenAI upstream statuses/bodies remain unchanged and `Retry-After` is now
  preserved. Tests exposed the previously missing header before the correction.
- Official OpenAI Python SDK **3.38.0** passed JSON and streaming image-assisted
  tool/result loops against the compiled gateway and fake Val, including strict
  SDK response validation, fragmented names/arguments and measured usage.
- Actual Claude Code **2.1.310** passed isolated Read/Edit/final-response
  acceptance in three requests: exact file bytes, both tool results and zero
  permission denials. Bare Read and fake Read/Write still passed.

## Live verification

The prior 23 attempts were exhausted. This continuation used two separately
approved batches, without automatic retries, repository files, real conversations
or account-setting changes:

| Batch | Attempts and result |
| --- | --- |
| A: up to five, output cap 128 | Four used. Two OpenAI first-turn probes returned valid calls and measured usage (1,074/55 each); a case/whitespace-only red label failed the overly exact test. Claude Read succeeded (2,514/99); its Edit response hit the 128-token cap (2,680/128) and incomplete arguments were rejected. One unused attempt was explicitly retired. |
| B: replacement five | All five used. OpenAI SDK **3.38.0** streamed the generated red-image tool call and result acknowledgement successfully in two requests, capped at 128: measured prompt/completion **1,088/73**, then **1,177/7**. Claude Code **2.1.310** completed Read, Edit and final reply in three requests capped at 512. Read usage was **2,520/99**, final reply **2,970/28**; the recorder's 40-frame diagnostic limit omitted the Edit turn's usage summary, although the gateway validated complete measured usage. |

Total historical inference attempts: **32**. No live authorization remains.
The retired unused request is not carried forward. Attempts include denials and
do not establish billing; CLI cost fields are not Val billing measurements.

The image test now accepts case/whitespace variation of red, while rejecting
other colors and extra argument keys. Non-image synthetic markers remain exact.
Its prompt explicitly asks for the same final acknowledgement the test requires.

The Edit fixture is always a parent-created random synthetic line in temporary
`fixture.txt`. CLI mode exposes Read and Edit, uses `Edit(./fixture.txt)` with
`dontAsk`, an isolated home, no setting sources/MCP and zero retries. Metadata,
loopback URL and exact initial bytes are checked before CLI startup; they are
setup controls, while CLI permissions govern actual file modification. Exact
post-edit bytes, three tool turns and final marker are required for acceptance;
surrounding prose/Markdown in the reply does not invalidate an exact file edit.
Read/Write mode remains restricted to the known fake localhost fixture.

The real Val credential remains in the recorder. The temporary gateway and CLI
receive disposable local capabilities only. Duplicates and exhausted budgets
are denied locally; failed remote requests still count against the total.

## Verification and review

Fresh `go test ./... -count=1`, `go vet ./...` and proxy/client race tests passed.
The Windows executable built, and both proxy/client test binaries passed natively
under Ubuntu/WSL. Official SDK JSON/streaming acceptance passed offline in four
requests; current Python discovery passed 26 tests with three opt-in CLI tests
skipped. A separate current helper suite passed **16/16**, including those three
actual installed-CLI Read/Edit/Write cases and 13 guard/evidence/preparation checks.
Python syntax, JavaScript syntax and diff formatting passed.

Codex Security reviewed immutable incremental patches. It identified the POST
redirect regression before live execution; v2 removed replay, and later versions
corrected test diagnostics and acceptance. Final v5 patch SHA-256:
`49ba40c9f8ae09c52349f7a3401ac0b92b00b3d2335e8ac0b2e2213e2ca06800`.
The Codex Security patch-risk report and its schema-validated JSON are retained
locally; the [portable review summary](SECURITY_REVIEW.md#incremental-openai--edit-assessment--27-october-2026)
records their recommendation for human review before merging. Impact if wrong is high, likelihood
moderate, protection partial, recovery easy and confidence moderate. No unresolved
source-visible defect was found in the final 15-file incremental runtime/test
patch; unrelated dirty work is outside that assessment. Final patch and identity
are retained beside the report as `2026-10-27-openai-edit-v5.patch` and
`2026-10-27-openai-edit-v5-identity.json`.
This is not a new completed native security scan. The earlier sealed 16-file
scan remains bound to its pre-patch snapshot.

Graphify refresh and clustering completed offline with query logging disabled:
**799 nodes, 2,513 edges and 40 communities**. Its receiver-call coverage remains
incomplete; important flows were verified in source and tests. The live-tested
Windows binary was built in temporary storage, SHA-256
`88e62e353387c93f034964cb056d39b1071bd5b76917328e696321140f62610b`.

Firecrawl retrieved the [official Chat Completions reference](https://developers.openai.com/api/reference/resources/chat/)
on 27 October 2026. The function-tool, tool-result and streaming usage contracts
informed these checks. The [Claude Code permission reference](https://code.claude.com/docs/en/permissions#path-rules-that-never-match)
documents `Edit(path)` permissions; `Write(path)` never matches file rules.

## Limits and next action

These checks do not establish universal
harness compatibility, every model's controls, native signed thinking, arbitrary
agent permissions or production deployment. OpenAI Responses, Files and Batches
remain unimplemented. Installer/playground/dashboard audit items and subsequent
non-programmer feature research remain separate work.

# Security review summary

This is a portable engineering summary for the public repository. It is not a
replacement for the original Codex Security reports, a new scan, or a security
certification. Original reports and diagnostic artifacts are retained locally;
machine-local paths and raw diagnostic logs are not published here.

## Initial audit — 26 October 2026

Baseline: `6999a37c298137e386567dfabee38d8c9caf83d0`.
Native scan: `37217ea0-663e-4ce5-9540-f40ea7040e53`.

The completed audit reported two medium-severity findings, summarized in
[BUG_AUDIT.md](BUG_AUDIT.md): default owner access/origin handling and unescaped
model metadata in browser rendering. The audit also recorded 17 correctness
groups. Subsequent authentication and rendering fixes have regression tests;
the initial report stays bound to its original snapshot.

## Scoped proxy scan — 26 October 2026

Native Standard scan: `92903b28-d9ef-4127-82cc-9d92be491088`.
The scan reviewed 16 proxy files and reported zero findings. This snapshot
predates the later Sonnet and shared OpenAI corrections. Subsequent static patch
reviews supplement it; they do not turn it into a final-tree or repository-wide scan.

## Incremental OpenAI / Edit assessment — 27 October 2026

Codex Security's read-only patch-risk workflow reviewed an immutable 15-file
runtime/test increment. Exact patch SHA-256:

```text
49ba40c9f8ae09c52349f7a3401ac0b92b00b3d2335e8ac0b2e2213e2ca06800
```

It identified a buffering-induced 307/308 POST replay regression before the live
checks. The correction disables body replay and is covered by direct-client and
raw-route tests. No unresolved source-visible defect remained in the final
increment. The assessment recommended **human review before merging**, with high
impact if wrong, moderate likelihood, partial regression protection, easy
recovery, and moderate confidence. The machine-readable assessment passed its
schema validator. This recommendation is advisory and does not authorize a merge.

Coverage excludes unrelated working-tree changes, provider internals, aggregate
load behavior, complete platform rollout, and an independently executed malicious
CLI path-denial case. The README refresh is documentation work after that patch
assessment. See [OPENAI_EDIT_CHECK.md](OPENAI_EDIT_CHECK.md) for the exact local/live
evidence and the final-reply formatting distinction.

## Reproducing and sharing evidence

Use the repository's synthetic tests and isolated temporary directories. Never
publish Val tokens, local owner keys, conversations, account files or raw logs.
Detailed native reports, threat models, coverage files, PoC inputs and execution
logs remain local unless separately reviewed and intentionally shared. Public
source availability does not change ownership of user data.

# README and branch publication

User authorization: create a new branch and push the existing verified work;
rewrite the README with an attractive GitHub presentation. Do not push to main.

- [x] Create `codex/agent-compatibility-readme` from the verified local baseline.
- [x] Inspect current GitHub README examples with Firecrawl and release metadata.
- [x] Add original SVG artwork, concise setup, client settings and tested matrix.
- [x] Preserve data ownership, protocol limits and the v1.0.0/source distinction.
- [x] Replace personal machine-path links with a portable security review summary.
- [x] Exclude local tools, secrets, generated graphs, binaries and runtime output.
- [x] Run Go tests/vet, Node checks, Python tests, link checks and visual previews.
- [x] Commit source/tests separately from engineering docs and README presentation.
- [x] Push only the new branch and verify its remote head.

The remote main branch has nine newer commits through `d8e4760`, covering
analytics, quota and update features. They are not mixed into this tested branch;
integration remains a separate review. No main ref is updated by this task.

Published on 27 October 2026 after repository write access was granted. The
noninteractive dry run and feature-branch push succeeded; `git ls-remote`
verified the published source and README head as
`31ed74a1cd2efa483e7c69cca17305aae4fd03b2`. The branch tracks
`origin/codex/agent-compatibility-readme`. Integration with the newer main commits
remains a separate review.

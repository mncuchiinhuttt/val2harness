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
- [ ] Push only the new branch and verify its remote head.

The remote main branch has nine newer commits through `d8e4760`, covering
analytics, quota and update features. They are not mixed into this tested branch;
integration remains a separate review. No main ref is updated by this task.

The initial push preflight returned HTTP 403 for the currently selected Git
account. Publication requires an account with write access to the repository.

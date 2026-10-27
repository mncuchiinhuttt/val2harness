# README presentation and publication

The user requested a new branch, publication of the verified fixes and a more
attractive GitHub README. The implementation choice is a concise product page:
an original local SVG illustration, a small set of factual badges, a three-step
source setup flow, client connection instructions and a tested compatibility
table. Expandable sections hold download and protocol details. User data
ownership and the development/release distinction remain visible.

An exhaustive reference page would bury setup; a screenshot-heavy landing page
would require synthetic UI imagery or new live account captures. The chosen
layout uses existing source facts and an original diagram instead.

Firecrawl retrieved these public README examples on 27 October 2026:

- [Gum](https://github.com/charmbracelet/gum): approachable opening and progressive examples.
- [uv](https://github.com/astral-sh/uv): concise highlights and a clear installation path.
- [LiteLLM](https://github.com/BerriAI/litellm): protocol orientation and integration navigation.

Only structural ideas informed this rewrite; text and artwork are original.
No third-party brand artwork or screenshots are copied. No benchmark, CI badge,
universal compatibility or model availability is invented.

Publication is restricted to `codex/agent-compatibility-readme`. Its base retains
the verified work from `6999a37`; the nine newer `origin/main` commits add analytics,
quota and update features that require separate integration. Main is not merged,
reset or pushed. Local tools, generated files, credentials and machine-local
diagnostic paths are excluded. The optional Graphify installation is not vendored.

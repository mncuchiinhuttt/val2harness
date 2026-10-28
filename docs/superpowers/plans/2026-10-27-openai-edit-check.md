# OpenAI tools and synthetic editing verification

> Use Superpowers systematic debugging, TDD and verification for each increment.

**Goal:** verify OpenAI Chat Completions client-tool interoperability and actual
Claude Code editing without exposing user files or altering account settings.

**Architecture:** register URL-free caller schemas once in the shared Go Val
client, preserving caller fields and explicit registries. Keep Anthropic response
validation and OpenAI response streaming/status behavior. Prepare an opt-in
three-turn Edit fixture with isolated CLI settings and a bounded recorder.

**Spec:** existing `AGENT_COMPATIBILITY.md` and `SONNET_FIX.md`; this corrects the
known missing OpenAI registration path rather than promising every OpenAI API.

- [x] Reproduce missing shared client registration and preserve precision,
  caller options, explicit registry and unadapted request behavior in tests.
- [x] Bound shared request buffering to 32 MiB with explicit 413 on overflow;
  preserve response streaming, cancellation, HTTP failures and Retry-After.
- [x] Move registration out of Anthropic-only conversion into the shared client;
  verify both protocol routes and direct callers with fake upstreams.
- [x] Add official OpenAI SDK JSON/streaming tool/result checks, including errors,
  images on the wire and measured streaming usage. Pin optional test version.
- [x] Prepare and run actual Claude Code Read/Edit/final-marker acceptance with
  a fixed disposable fixture and negative setup guards; retain existing modes.
- [x] Review an immutable patch with Codex Security; record limits and recovery.
- [x] Obtain new explicit bounded quota consent before live verification. Prior
  23 attempts are exhausted. No retries or user repository/conversation inputs.
- [x] Run appropriate checks, refresh Graphify offline and update current records.

Completed checkpoint: [OPENAI_EDIT_CHECK.md](../../OPENAI_EDIT_CHECK.md). Live
OpenAI stream/image/tool/result passed. Live Claude Edit verified exact file
execution, three turns, no permission denials and marker return; its whole-reply
format assertion failed, was corrected with red/green tests and not rerun live.
The final helper's separate actual-CLI suite passed 16/16 offline. Security
review caught and corrected 307/308 replay before live probes. Total historical
requests now 32; no live authorization remains. One unused earlier attempt was
retired before the final five were separately approved.

The 32 MiB limit is a new shared request contract; output is additionally bounded
while registering schemas. No persistent configuration/schema migration or user
daemon replacement is included. OpenAI Responses, Files and native provider
services remain separate unsupported APIs. All changes stay local unless asked.

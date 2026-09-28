# Decision log

[Contributor docs](README.md)

Non-obvious architectural choices. Append-only.

## 001: Go over Phel/PHP

**Date:** 2026-05-03

**Context:** initial stack pick. Considered Phel (Lisp on PHP) for dev velocity and showcase value.

**Decision:** Go.

**Why:** target users are AI tooling devs across stacks (JS, Python, Rust, Go). A PHP runtime cannot be assumed. Go ships a single static binary via Homebrew, `curl | sh`, or `go install`: zero runtime deps, trivial cross-compile (`GOOS GOARCH`). The Phel showcase did not justify the reach loss. Tradeoff: lose Lisp dev velocity, gain ecosystem-agnostic distribution.

## 002: MD + YAML frontmatter for source format

**Date:** 2026-05-03

**Context:** authoring format for agents/skills/rules. Considered pure YAML, pure MD, JSON.

**Decision:** MD + YAML frontmatter.

**Why:** matches Claude Code's native format, so migration is copy-paste. Markdown body fits prompts. YAML frontmatter is widely understood (Jekyll, Hugo, MDX). Hooks use pure YAML: they have no body, only fields.

## 003: Capability degradation over least-common-denominator

**Date:** 2026-05-03

**Context:** Codex/Gemini/Cursor lack hooks. Skills exist only on Claude. Should the spec omit unsupported features?

**Decision:** support the superset. Adapters skip unsupported kinds with a warning.

**Why:** restricting to LCD punishes Claude users for other tools' limits. Skipping with a warning makes the gap visible without breaking sync. Future tools may close gaps (e.g. Cursor adding hooks).

## 004: Adapter packages do not import each other

**Date:** 2026-05-03

**Context:** could share more code between adapters (e.g. agent merging logic).

**Decision:** adapters share only via `internal/adapters/internal/emit`. No cross-adapter imports.

**Why:** target quirks must not leak across adapters. Adding an adapter must not require touching existing ones. Premature abstraction across adapters has burned similar projects (compile-target frameworks, transpiler ecosystems).

## 005: Jev triage for target audits, optional and measured

**Date:** 2026-09-28

**Context:** a daily `/target-audit` sends Sonnet auditors to read every changed vendor page. Most changes are harmless, and reading them all spends the maintainer's Claude or Codex usage window. TypeSafe's Jev returns typed judgments (Choice, Noul) for a fraction of a cent. It needs an API key that not every machine or contributor has.

**Decision:** `scripts/jev-triage.sh` asks Jev whether each changed page contradicts any claim we make about its target, and whether it changes a project config surface. Jev leads order the reading. A target whose changed pages Jev fully answers with no lead, a surface score under 0.1, and no `mentions:` label skips the auditor, and one such target per run is audited anyway. With no key, or on any API failure, the script writes lexical leads, and every changed page is read as before.

**Why:** measured, not assumed. On 33 labeled past findings Jev flagged every confirmed drift in every run, where lexical matching alone flagged 12 of 18. A full audit of 25 targets with 19 changed pages cost 13 requests, 136,688 input tokens, and $0.0057, checked against the TypeSafe dashboard. Keeping the script optional preserves the audit for anyone without a key. Tradeoff: a wrong `clear` skips a read, so the clearing cut is far stricter than the lead cut, and the random spot check keeps testing it. Details and numbers: [Jev triage](jev-triage.md).

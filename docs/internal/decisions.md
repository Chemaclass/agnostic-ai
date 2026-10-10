# Decision log

[Contributor docs](README.md)

Design choices and their reasons. Add new decisions without removing earlier ones.

## 001: Go over Phel/PHP

**Date:** 2026-05-03

**Context:** initial language choice. Considered Phel (Lisp on PHP) for faster development and to demonstrate the language.

**Decision:** Go.

**Why:** target users build coding tools across languages (JS, Python, Rust, Go). A PHP runtime cannot be assumed. Go ships a single static binary via Homebrew, `curl | sh`, or `go install`: no separate runtime dependencies, easy builds for other operating systems and processors (`GOOS GOARCH`). Demonstrating Phel did not justify limiting who could run the tool. Tradeoff: give up faster Lisp development to distribute the tool across language communities.

## 002: MD + YAML frontmatter for source format

**Date:** 2026-05-03

**Context:** file format for writing agents, skills, and rules. Considered YAML alone, Markdown alone, and JSON.

**Decision:** MD + YAML frontmatter.

**Why:** matches Claude Code's native format, so migration is copy-paste. Markdown body fits prompts. YAML header fields (frontmatter) are widely understood (Jekyll, Hugo, MDX). Hooks use pure YAML: they have no body, only fields.

## 003: Capability degradation over least-common-denominator

**Date:** 2026-05-03

**Context:** Codex/Gemini/Cursor lack hooks. Skills exist only on Claude. Should the spec omit unsupported features?

**Decision:** support features even when only some tools can use them. Adapters skip unsupported kinds with a warning.

**Why:** restricting specs to features every tool supports would limit Claude users because of other tools' limits. Skipping with a warning makes the gap visible without breaking sync. Future tools may close gaps (e.g. Cursor adding hooks).

## 004: Adapter packages do not import each other

**Date:** 2026-05-03

**Context:** could share more code between adapters (e.g. agent merging logic).

**Decision:** adapters share only via `internal/adapters/internal/emit`. No cross-adapter imports.

**Why:** one tool's special behavior must not affect another adapter. Adding an adapter must not require touching existing ones. Sharing adapter-specific behavior too early has caused problems in similar projects that generate output for different platforms or languages.

## 005: Jev triage for target audits, optional and measured

**Date:** 2026-09-28

**Context:** a daily `/target-audit` sends Sonnet auditors to read every changed vendor page. Most changes are harmless, and reading them all spends the maintainer's Claude or Codex usage window. TypeSafe's Jev returns structured answers (Choice, Noul) for a fraction of a cent. It needs an API key that not every machine or contributor has.

**Decision:** `scripts/jev-triage.sh` asks Jev whether each changed page contradicts any claim we make about its target, and whether it changes a project configuration feature. Jev's possible problems determine the reading order. A target whose changed pages Jev fully answers with no possible problem, a configuration-feature score under 0.1, and no `mentions:` label skips the auditor, and one such target per run is audited anyway. With no key, or on any API failure, the script writes possible problems based on matching words, and every changed page is read as before.

**Why:** measured, not assumed. On 33 labeled past findings Jev flagged every confirmed difference from vendor documentation in every run, where word matching alone flagged 12 of 18. A full audit of 25 targets with 19 changed pages cost 13 requests, 136,688 input tokens, and $0.0057, checked against the TypeSafe dashboard. Keeping the script optional preserves the audit for anyone without a key. Tradeoff: a wrong `clear` skips a read, so the threshold for skipping a page is far stricter than the threshold for requesting a review, and the random spot check keeps testing it. Details and numbers: [Jev triage](jev-triage.md).

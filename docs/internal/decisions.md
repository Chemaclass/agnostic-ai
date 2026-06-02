# Decision log

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

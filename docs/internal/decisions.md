# Decision log

Non-obvious architectural choices. Append-only.

## 001: Go over Phel/PHP

**Date:** 2026-05-03

**Context:** initial stack pick. Considered Phel (Lisp on PHP) for dev velocity and as a Phel showcase.

**Decision:** Go.

**Why:** target users are AI tooling devs across stacks (JS, Python, Rust, Go), so a PHP runtime cannot be assumed. Go ships a single static binary via Homebrew, `curl | sh`, or `go install` with zero runtime deps and trivial cross-compile (`GOOS GOARCH`). Phel showcase value did not justify the reach loss. Tradeoff: lose Lisp dev velocity, gain ecosystem-agnostic distribution.

## 002: MD + YAML frontmatter for source format

**Date:** 2026-05-03

**Context:** how to author agents/skills/rules. Considered pure YAML, pure MD, JSON.

**Decision:** MD + YAML frontmatter.

**Why:** matches Claude Code's native format, so migration is copy-paste. Markdown body is the natural medium for prompts. YAML frontmatter is widely understood (Jekyll, Hugo, MDX). Hooks use pure YAML because they have no body, only fields.

## 003: Capability degradation over least-common-denominator

**Date:** 2026-05-03

**Context:** Codex/Gemini/Cursor lack hooks. Skills only exist on Claude. Should the spec omit unsupported features?

**Decision:** support the superset; adapters skip unsupported kinds with a warning.

**Why:** restricting to LCD would punish Claude users for other tools' limitations. Skipping with a warning makes the gap visible without breaking the sync. Future tools may close gaps (e.g. Cursor adding hooks).

## 004: Adapter packages do not import each other

**Date:** 2026-05-03

**Context:** could share more code between adapters (e.g. agent merging logic).

**Decision:** adapters share only via `internal/adapters/internal/emit`. No cross-adapter imports.

**Why:** each target's quirks should not leak into another's code. Adding a new adapter must not require touching existing ones. Premature abstraction across adapters has burned similar projects (compile-target frameworks, transpiler ecosystems).

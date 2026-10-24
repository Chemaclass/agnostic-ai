---
name: go-style
description: Baseline Go conventions for agnostic-ai.
globs: "**/*.go"
alwaysApply: false
---

- `gofmt` clean. Use `goimports` for import grouping.
- No external deps unless they earn their place. Prefer stdlib.
- One concern per PR. Open separate PRs for refactors.

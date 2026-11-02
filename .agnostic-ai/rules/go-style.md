---
name: go-style
description: Baseline Go conventions for agnostic-ai.
globs: "**/*.go"
alwaysApply: false
---

- `gofmt` clean. Use `goimports` for import grouping.
- Prefer stdlib; add a dependency only when it earns its place.
- One concern per PR; refactors get their own.

---
name: go-style
description: Go coding conventions for agnostic-ai.
globs: "**/*.go"
alwaysApply: true
---

- `gofmt` clean. Use `goimports` for import grouping.
- No external deps unless they earn their place. Prefer stdlib.
- Errors include enough context to find the file. Wrap with `fmt.Errorf("%s: %w", path, err)`.
- Test names describe behavior, not implementation. `TestEmit_WritesAgentFile`, not `TestEmit1`.
- One concern per PR. Open separate PRs for refactors.
- Adapters are stateless. Construct via `New()`.
- Adapter packages do not import each other. Share via `internal/adapters/internal/emit`.
- Use `t.TempDir()` and restore cwd in cleanup for tests that write files.

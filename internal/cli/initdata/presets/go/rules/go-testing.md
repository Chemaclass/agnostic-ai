---
name: go-testing
description: Go test conventions for this project.
globs: "**/*_test.go"
alwaysApply: true
---

- Test names describe behavior, not implementation: `TestEmit_WritesAgentFile`, not `TestEmit1`.
- Use `t.TempDir()` for filesystem fixtures and restore working directory in cleanup when tests `chdir`.
- Mark helper functions with `t.Helper()` so failures point at the call site.
- Prefer table-driven tests when more than two cases share the same setup; one-off cases stay flat.
- Use `t.Run(name, ...)` so subtests are addressable with `-run name/case`.
- No global state. No `init()` for test setup. No network calls without a `// requires network` comment and a build tag.

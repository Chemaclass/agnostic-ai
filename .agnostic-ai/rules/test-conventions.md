---
name: test-conventions
description: Go test conventions for agnostic-ai.
globs: "**/*_test.go"
alwaysApply: true
---

- Tests that write files use `t.TempDir()` and `testutil.Chdir(t, dir)` so they leave no traces and parallel runs do not collide.
- Test names describe behavior. `TestEmit_WritesRulesFile`, not `TestEmit1`.
- Use table-driven tests only when the table earns its keep (three or more cases with the same shape). One-off tests stay flat.
- Use `mock()` style helpers from `internal/testutil` when available. No external mocking libraries.
- Assertions: `t.Errorf` for soft failures, `t.Fatalf` only when continuing would crash or hide the real failure.
- Each adapter test reads the file back from disk to confirm content, not just that no error was returned.
- Integration tests live under `tests/integration` and shell out to the built binary; unit tests stay inside their package.

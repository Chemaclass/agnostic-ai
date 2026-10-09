---
name: test-conventions
description: Go test conventions for agnostic-ai.
globs: "**/*_test.go"
alwaysApply: false
---

- Tests that write files use `t.TempDir()` and `testutil.Chdir(t, dir)` so they leave no traces and parallel runs do not collide.
- Test names describe behavior. `TestEmit_WritesRulesFile`, not `TestEmit1`.
- Use table-driven tests only when it pays off (three or more cases with the same shape). A one-off test stays a plain test function.
- Use `mock()` style helpers from `internal/testutil` when available. No external mocking libraries.
- Assertions: `t.Errorf` when the test can keep going, `t.Fatalf` only when continuing would crash or hide the real failure.
- Each adapter test reads the file back from disk to confirm content, not just that no error was returned.
- Integration tests live under `tests/integration` and run the built binary; unit tests stay inside their package.

---
name: test-conventions
description: Go test conventions for agnostic-ai.
globs: "**/*_test.go"
alwaysApply: false
---

- Use `t.TempDir()` for files. Use `testutil.Chdir(t, dir)` only when the test needs a working directory; that test must not use `t.Parallel()`.
- Test names describe behavior. `TestEmit_WritesRulesFile`, not `TestEmit1`.
- Table-driven tests only for three or more cases of the same shape.
- Reuse helpers from `internal/testutil`. Do not add an external mocking library.
- `t.Errorf` by default; `t.Fatalf` only when continuing would crash or hide the failure.
- Adapter tests read the emitted file back from disk.
- Integration tests live under `tests/integration` and run the built binary; unit tests stay inside their package.

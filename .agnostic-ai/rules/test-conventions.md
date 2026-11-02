---
name: test-conventions
description: Go test conventions for agnostic-ai.
globs: "**/*_test.go"
alwaysApply: false
---

- Use `t.TempDir()` for files. Use `testutil.Chdir(t, dir)` only when the test needs a working directory; that test must not use `t.Parallel()`.
- Name tests by behavior: `TestEmit_WritesRulesFile`.
- Table-driven tests only for three or more cases of the same shape.
- Reuse `internal/testutil`; add no mocking library.
- `t.Errorf` ; `t.Fatalf` only when continuing would crash or hide the failure.
- Adapter tests read the emitted file back from disk.
- Integration tests live in `tests/integration` and run the built binary; unit tests stay in their package.

---
name: error-wrapping
description: Wrap errors with enough context to find the failing file or operation.
globs: "**/*.go"
alwaysApply: false
---

- File I/O: `fmt.Errorf("%s: %w", path, err)`.
- Decode failures: `fmt.Errorf("parse %s: %w", path, err)`.
- Adapter emit: `fmt.Errorf("<target>: %w", err)` for target-scoped failures.
- Wrap underlying errors with `%w` to preserve `errors.Is` / `errors.As`.
- Do not double-wrap. If the inner call already prefixes the path, wrap with the operation only.
- Never return a bare `errors.New("...")` from a function that also touches the filesystem.

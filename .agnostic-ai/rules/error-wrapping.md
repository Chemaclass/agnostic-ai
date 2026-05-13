---
name: error-wrapping
description: Wrap errors with enough context to find the failing file or operation.
globs: "**/*.go"
alwaysApply: true
---

Every returned error must carry enough context for the user to act without reading a stack trace.

- File I/O: `fmt.Errorf("%s: %w", path, err)`. The path leads so grep on the output finds the source.
- Decode failures: `fmt.Errorf("parse %s: %w", path, err)`.
- Adapter emit: `fmt.Errorf("<target>: %w", err)` when the failure is target-scoped and not file-scoped.
- Always wrap with `%w` so callers can `errors.Is` / `errors.As` against sentinel errors.
- Do not double-wrap. If the inner call already prefixes the path, wrap with the operation only.
- Never return a bare `errors.New("...")` from a function that also touches the filesystem.

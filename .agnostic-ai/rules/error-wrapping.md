---
name: error-wrapping
description: Wrap errors with enough context to find the failing file or operation.
globs: "**/*.go"
alwaysApply: false
---

- File I/O: `fmt.Errorf("%s: %w", path, err)`.
- Decode failures: `fmt.Errorf("parse %s: %w", path, err)`.
- Adapter emit: `fmt.Errorf("<target>: %w", err)`.
- Wrap with `%w` to keep `errors.Is` / `errors.As`. Do not double-wrap: if the inner call already prefixes the path, add the operation only.
- Never return a bare `errors.New("...")` from a function that touches the filesystem.

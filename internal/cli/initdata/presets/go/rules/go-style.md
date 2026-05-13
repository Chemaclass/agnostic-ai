---
name: go-style
description: Idiomatic Go style and formatting.
globs: "**/*.go"
alwaysApply: true
---

- `gofmt` clean. Run `goimports` for grouped imports.
- Prefer the standard library. New external dependencies need a justification in the PR.
- Errors carry enough context to find the file: `fmt.Errorf("%s: %w", path, err)`.
- Lower-case package names, no underscores, no plurals.
- Exported identifiers carry doc comments that begin with the identifier name.
- Receivers are short (1-2 letters), consistent across methods on the same type.

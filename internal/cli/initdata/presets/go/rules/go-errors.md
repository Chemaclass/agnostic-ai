---
name: go-errors
description: Error handling discipline for Go code.
globs: "**/*.go"
alwaysApply: true
---

- Wrap returned errors with context that names the operation: `fmt.Errorf("read %s: %w", path, err)`.
- Sentinel errors are exported as `ErrXxx`. Compare with `errors.Is`.
- Custom error types satisfy the `error` interface and are checked with `errors.As`.
- Do not log and return the same error; pick one. Logging hides the call site.
- `panic` is for programmer error only (impossible state). User-facing failures return errors.

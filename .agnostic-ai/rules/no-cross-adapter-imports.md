---
name: no-cross-adapter-imports
description: Adapter packages must not import each other.
globs: "internal/adapters/**/*.go"
alwaysApply: false
---

Target adapter packages under `internal/adapters/<target>/` must not import one another.

Shared output helpers belong in `internal/adapters/internal/emit/`, not the registry. Other shared helpers belong in internal packages and must not import target adapters.

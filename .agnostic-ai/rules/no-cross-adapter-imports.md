---
name: no-cross-adapter-imports
description: Adapter packages must not import each other.
globs: "internal/adapters/**/*.go"
alwaysApply: false
---

Packages under `internal/adapters/<target>/` must not import one another. Shared output helpers go in `internal/adapters/internal/emit/`, other shared helpers in internal packages that import no target adapter.

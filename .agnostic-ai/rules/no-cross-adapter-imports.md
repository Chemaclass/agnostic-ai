---
name: no-cross-adapter-imports
description: Adapter packages must not import each other.
globs: "internal/adapters/**/*.go"
alwaysApply: false
---

Target adapter packages under `internal/adapters/<target>/` must not import one another.

Reuse shared helpers under `internal/adapters/internal/` and other internal packages where appropriate. Put shared output-writing logic in `internal/adapters/internal/emit/`, not in the adapter registry. A helper must not depend on a target adapter.

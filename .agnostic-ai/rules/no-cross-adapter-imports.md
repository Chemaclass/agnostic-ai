---
name: no-cross-adapter-imports
description: Adapter packages must not import each other.
globs: "internal/adapters/**/*.go"
alwaysApply: true
---

Adapter packages live under `internal/adapters/<target>/` and must stay independent.

Allowed imports inside an adapter:

- `internal/adapters/internal/emit` (shared write helpers)
- `internal/config`
- `internal/spec`
- Standard library
- `gopkg.in/yaml.v3`

Forbidden:

- Importing one adapter package from another (`internal/adapters/<a>` importing `internal/adapters/<b>`).
- Adding shared business logic across adapters in the registry file.

If you need to share code between two adapters, lift it into `internal/adapters/internal/emit` first.

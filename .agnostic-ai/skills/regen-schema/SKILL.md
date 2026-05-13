---
name: regen-schema
description: Regenerate docs/schemas/config.schema.json from the Config struct. Use after editing internal/config/config.go.
---

# regen-schema

The JSON Schema published at `docs/schemas/config.schema.json` is reflected from the `config.Config` Go struct.

## When to run

Any of:

- Added, removed, or renamed a field on `config.Config`, `config.Output`, `config.Sources`, or `config.Gitignore`.
- Changed a struct tag (`yaml:` or `json:`) on those types.
- Bumped the schema `version`.

## Steps

1. `go run ./cmd/schemagen` from the repo root.
2. `git diff docs/schemas/config.schema.json` to confirm the change is what you intended.
3. Commit the regenerated schema alongside the struct change. CI's `Schema drift` job fails if they land separately.

Never hand-edit the schema file. The next regen overwrites it.

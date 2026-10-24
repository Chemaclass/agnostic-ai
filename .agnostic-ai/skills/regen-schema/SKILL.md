---
name: regen-schema
description: Regenerate docs/schemas/config.schema.json after changes to config fields, schema tags, nested types, or cmd/schemagen mappings.
---

# regen-schema

The JSON Schema published at `docs/schemas/config.schema.json` is generated from the `config.Config` Go struct.

## When to run

Any of:

- Changed a field on `config.Config` or a type it contains, including nested types in other `internal/config/` files.
- Changed a serialized name or schema constraint (`yaml:`, `json:`, `jsonschema:`, or `jsonschema_description:`).
- Changed reflection or custom type mappings in `cmd/schemagen/main.go`.

Changing only the numeric config `version` does not itself change the reflected schema. Regenerate when its schema representation changes.

## Steps

1. `go run ./cmd/schemagen` from the repo root.
2. `git diff docs/schemas/config.schema.json` to confirm the change is what you intended.
3. Include any schema diff with the source change. CI's `Schema drift` job regenerates it and fails on a diff. A source-only change that leaves the schema identical needs no artificial schema edit.

Never hand-edit the schema file. The next regen overwrites it.

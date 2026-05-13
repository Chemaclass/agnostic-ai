---
name: docs-sync
description: User-visible changes must update docs, schema, and CHANGELOG in the same PR.
globs: "**/*"
alwaysApply: true
---

When a change is visible to the user, update the matching artifacts in the same PR so they never drift:

- New or changed flag, target, or output field: update `docs/user/targets.md` and `docs/user/configuration.md`.
- New or changed spec field: update `docs/user/spec-format.md`.
- Any change to `internal/config/config.go` struct tags: run `go run ./cmd/schemagen` to regenerate `docs/schemas/config.schema.json`. CI fails if the schema is stale.
- New command or visible behavior: update `README.md` capability or quickstart section.
- Any user-visible change: add an entry under `## [Unreleased]` in `CHANGELOG.md`. Group as `Added`, `Changed`, `Fixed`, or `Removed`.

A pure refactor or test-only change skips all of the above.

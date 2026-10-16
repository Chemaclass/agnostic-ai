---
name: docs-sync
description: User-visible changes must update docs, schema, and CHANGELOG in the same PR.
globs: "**/*"
alwaysApply: true
---

For a user-visible change, read and follow `docs/internal/contributing.md#documentation-checklist` before editing. Complete its applicable updates in the same PR.

Pure refactors and test-only changes with no user-visible effect skip these updates.

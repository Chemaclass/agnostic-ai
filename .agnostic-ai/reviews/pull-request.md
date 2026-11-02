---
name: pull-request
description: Check a diff for broken behavior and project contracts that compilation cannot prove.
---

Review changed behavior against callers, tests and `.agnostic-ai/rules/`. Report verified defects only.

- Target adapter packages must not import one another. Shared emission belongs in `internal/adapters/internal/`.
- Changed emitted bytes need golden coverage in `tests/integration/fixtures/golden/` and the adapter's `testdata/`; byte-preserving refactors need none.
- Capability claims must match actual emission, CLI parity, and site capability data.
- New behavior needs a test that fails without the change.
- Returned errors retain the failing path or operation and preserve wrapped causes.
- User-visible changes update docs and `CHANGELOG.md` Unreleased per `docs/internal/contributing.md#documentation-checklist`.
- Config field or schema-tag changes regenerate `docs/schemas/config.schema.json`.

Report each finding as `path:line problem -> suggested fix`, highest impact first. Name validation limits.

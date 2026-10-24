---
name: pull-request
description: Check a diff for broken behavior and project contracts that compilation cannot prove.
---

Review changed behavior against its callers, tests, and `.agnostic-ai/rules/`. Report verified defects, not speculation or style preferences.

- Target adapter packages must not import one another. Shared emission belongs in `internal/adapters/internal/`.
- Changed emitted bytes need matching golden coverage in `tests/integration/fixtures/golden/` and the affected adapter's `testdata/`. Byte-preserving refactors need no golden diff.
- Capability claims must match actual emission, CLI parity, and site capability data.
- New behavior needs a test that fails without the change.
- Returned errors retain the failing path or operation and preserve wrapped causes.
- User-visible changes update the relevant docs or README and `CHANGELOG.md` under Unreleased, following `docs/internal/contributing.md#documentation-checklist`.
- Config field or schema-tag changes regenerate `docs/schemas/config.schema.json`; include any resulting diff.

Report each finding as `path:line problem -> suggested fix`, highest impact first. Name validation limits. Formatting and static-analysis results belong to their existing checks.

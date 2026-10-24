---
name: capability-fixtures
description: Move the capability matrix, parity list, and golden fixtures together when a target's supported kinds or outputs change.
globs: ["internal/adapters/**/*.go", "internal/cli/native_capabilities.go", "docs/site/data/capabilities.toml"]
alwaysApply: false
---

When supported kinds change, update `caps.Supports`, `targetsSupportingKind` in `internal/cli/native_capabilities.go`, and the target's row in `docs/site/data/capabilities.toml` together. Add the kind to its `kitSinkBundle`, parity test, and integration fixture.

When emitted bytes change, refresh the affected saved output:

- Adapter kit-sink golden file: `UPDATE_GOLDEN=1 go test ./internal/adapters/<target>/ -run TestKitSink_GoldenSnapshot`.
- Integration golden files: `UPDATE_GOLDEN=1 go test ./tests/integration/ -run 'TestGolden|TestSiteDocs_Compare|TestSiteDocs_LandingExplorer'`. Compare and landing data come from `tests/integration/fixtures/`.

Review every regenerated diff. Each change must follow from the affected target or shared behavior; discard unrelated fixture churn.

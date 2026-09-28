---
name: capability-fixtures
description: Move the capability matrix, parity list, and golden fixtures together when a target's supported kinds or outputs change.
globs: "{internal/adapters/**/*.go,internal/cli/native_capabilities.go,docs/site/data/capabilities.toml}"
alwaysApply: false
---

A change to what a target writes moves these together in one PR:

- `caps.Supports` in the adapter, `targetsSupportingKind` in `internal/cli/native_capabilities.go`, and the target's row in `docs/site/data/capabilities.toml`.
- The adapter's kit-sink golden: `UPDATE_GOLDEN=1 go test ./internal/adapters/<target>/ -run TestKitSink_GoldenSnapshot`, after adding the kind to its `kitSinkBundle` and parity test.
- Integration goldens: `UPDATE_GOLDEN=1 go test ./tests/integration/ -run 'TestGolden|TestSiteDocs_Compare|TestSiteDocs_LandingExplorer'`. The compare and landing data come from fixtures under `tests/integration/fixtures/`; give a new kind a spec there so the page shows it.

Review each regenerated diff: it should touch only the changed target.

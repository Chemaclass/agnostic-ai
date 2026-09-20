---
name: pull-request
description: What a review bot should check on an agnostic-ai diff.
---

Review the diff against the invariants this project cannot express as a compiler
check. The rules under `.agnostic-ai/rules/` say how to write the code; this page
says what to catch in a change.

- **Adapter independence.** No import between `internal/adapters/<a>/` and
  `internal/adapters/<b>/`. Shared behaviour belongs in
  `internal/adapters/internal/`. A cross-adapter import is the one change that
  makes every other target's output depend on an unrelated vendor.
- **Emitted output is a function of the specs.** A change to an adapter that
  alters emitted bytes must come with regenerated golden fixtures under
  `tests/integration/fixtures/golden/` and `internal/adapters/*/testdata/`. A
  diff that changes an emitter and touches no fixture is either dead code or an
  untested behaviour change.
- **Capability claims match the adapter.** A target listed as supporting a spec
  kind must actually write a file for it. Declaring support without an emitter
  produces a silent no-op, which is the failure mode this tool exists to prevent.
- **A new behaviour has a test that fails without it.** Not a test that merely
  passes afterwards.
- **Errors carry the path or operation.** `fmt.Errorf("read %s: %w", path, err)`,
  not a bare `err`.
- **User-visible change, user-visible record.** Anything a user can observe
  updates `docs/site/content/docs/` or `README.md`, and lands under
  `## [Unreleased]` in `CHANGELOG.md`.
- **Config struct changes regenerate the schema.** Editing
  `internal/config/config.go` without a matching
  `docs/schemas/config.schema.json` fails CI.

Report each finding as `path:line  problem -> suggested fix`. Lead with the
highest-impact one. Say nothing about formatting; `gofmt` and the linter already
gate it.

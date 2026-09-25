---
name: run-sync-check
description: Run agnostic-ai sync --check, interpret the diff, and pick the right next step. Use after editing specs or adapters.
---

# run-sync-check

Checks local generated files against the current specs and adapters. Most generated files are ignored by Git in this repository; `.openhands/setup.sh` is tracked for bootstrap.

## Steps

1. Run `go run ./cmd/agnostic-ai sync --check`. Exit 0 means the local generated files match. On drift, inspect the unified diff printed by the command; `git diff` cannot show ignored output.
2. Run `go run ./cmd/agnostic-ai sync` for intended changes, then repeat `go run ./cmd/agnostic-ai sync --check`.
3. Check `git status --short` for source and tracked output changes. Never edit generated files by hand to silence drift.

## CI

CI runs spec lint. It cannot compare ignored output in a fresh checkout, so `sync --check` is a local check, not a CI gate.

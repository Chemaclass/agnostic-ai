---
name: run-sync-check
description: Run agnostic-ai sync --check, interpret the diff, and pick the right next step. Use after editing specs or adapters.
---

# run-sync-check

Verifies that emitted target files match what the current specs would produce.

## Steps

1. Build the binary if changed: `make build`.
2. Run `./agnostic-ai sync --check`. Exit 0 means the working tree matches the bundle.
3. If exit is non-zero, the tool prints a unified diff per drifting file.
4. Decide:
   - Source edited, emitted file lags -> run `agnostic-ai sync` to refresh.
   - Adapter edited, emitted files would change -> intentional. Run `agnostic-ai sync` and review the diff in `git diff`.
   - Neither edited but check still drifts -> capture mode skipped reading existing files. Re-run with `agnostic-ai sync` and inspect.
5. Never edit emitted files by hand to silence the check. The next sync rewrites them.

## CI

`sync --check` is the gate. A drift exits 1 and fails the workflow. The fix is always to re-sync locally and commit, never to edit the emitted file.

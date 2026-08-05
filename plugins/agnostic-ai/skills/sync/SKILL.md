---
name: sync
description: Regenerate every AI tool's config from the agnostic-ai specs, or check for drift without writing. Use after editing anything under .agnostic-ai/, when a generated file looks stale, or when CI reports a drift failure.
---

# sync

Emits one set of specs to every configured target in its native format.

## Steps

1. Run `agnostic-ai sync`. It writes only the files whose content changed, so `git status` after a no-op run is empty.
2. Read the summary. Each line names a target and the paths it wrote.
3. Show the user the diff: `git diff --stat` (or `git status --short` when generated files are gitignored).

## Checking instead of writing

`agnostic-ai sync --check` compares planned output against what is on disk and writes nothing. Exit 0 means no drift; non-zero prints a diff per stale file. That is the CI gate:

```yaml
- uses: chemaclass/agnostic-ai-action@v1
  with: { command: check }
```

When `--check` fails, decide by what changed:

- A spec was edited and the generated file lags: run `agnostic-ai sync` and commit both.
- The CLI was upgraded and output shifted: run `sync`, review the diff, and commit it as a regeneration with no semantic change.
- Someone hand-edited a generated file: their edit is about to be lost. Move the change into the matching spec under `.agnostic-ai/`, then `sync`.

## Notes

- `agnostic-ai sync --watch` re-emits on every save, touching only affected targets. Useful while authoring several specs.
- `agnostic-ai sync --backup` keeps a `.bak` beside each overwritten file; `agnostic-ai revert` restores them. Without `--backup`, `revert` deletes generated files.
- `agnostic-ai status` reports spec counts, configured targets, the last sync time, and whether output is stale. `agnostic-ai doctor` is the stricter pre-merge check.
- A warning about an unsupported kind means the target has no surface for it (for example hooks on a target with no hook system). Silence it per project with `on-unsupported: silent`.
- Never resolve drift by editing generated files. The spec is the source of truth.

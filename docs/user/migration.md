# Import existing tool configuration

[User docs](README.md) · [Import reference](cli-reference.md#import)

Bring an existing project's instructions into `.agnostic-ai/`, review the imported content, then generate configuration for your selected tools.

## Scaffold and import

Start from a clean Git working tree so the imported and generated changes are easy to review. From the project root:

```bash
agnostic-ai init --from claude
```

Choose the tools you want to generate for. Replace `claude` with your source tool, or use `--from all` to detect existing configuration. For a project already initialized with agnostic-ai, run `agnostic-ai import claude` instead.

Import writes source specs without syncing native output or changing your target selection. Re-running import overwrites matching source filenames.

## Review before syncing

Inspect `.agnostic-ai/AGNOSTIC_AI.md` and the imported spec folders. Compare them with the original configuration.

When importing multiple tools, the last imported top-level instructions replace the shared instructions body. Merge any unique content from other tools into `.agnostic-ai/AGNOSTIC_AI.md` before syncing. See [import behavior by source](cli-reference.md#import).

Keep helper scripts referenced by hooks or settings in Git. Files inside a skill directory round-trip with the skill. Selected native helpers, including `.claude/statusline.sh`, are captured under `.agnostic-ai/overlays/`. Arbitrary scripts outside those supported locations must be preserved separately.

Commit the reviewed source specs, `agnostic-ai.yaml`, and `.gitignore` before generating output if you want a separate migration checkpoint. The instructions source lives inside `.agnostic-ai/`, not at the repository root.

## Generate and inspect

```bash
agnostic-ai sync --dry-run
agnostic-ai sync --backup
agnostic-ai sync --check
```

Inspect the diff and native files. The first sync can add headers, normalize formatting, and replace entry-point content with the shared instructions. Review content as well as formatting.

Choose your [Git strategy](getting-started.md#commit-or-ignore-generated-outputs). If you commit generated files, keep this initial regeneration in a separate commit from the import. Gitignore rules do not untrack files already in Git; use `git rm --cached <path>` for each generated file you decide to stop tracking.

## Back up and restore

`sync --backup` creates `.bak` files before overwriting existing outputs. To undo the generated output:

```bash
agnostic-ai revert
```

Revert restores backups where present and leaves files without backups in place. `revert --force` also deletes unbacked generated files. It does not undo edits to source specs. Repeated backup syncs replace earlier backups, so use Git for lasting checkpoints.

After accepting the migration, `agnostic-ai cleanup` removes sync-created backups. See [revert](cli-reference.md#revert) and [cleanup](cli-reference.md#cleanup) for filters and previews.

+++
title = "Import existing tool configuration"
description = "Import an existing AI tool setup into agnostic-ai and keep its behavior."
weight = 30

[extra]
group = "Start"
+++

# Import existing tool configuration


Move an existing project's instructions into `.agnostic-ai/`, review them, then generate configuration for your selected tools.

## Scaffold and import

Start from a clean Git working tree, at the project root, so the new files are easy to review:

```bash
agnostic-ai init --from claude
```

Pick the tools to generate for. Replace `claude` with your source tool, or use `--from all` to detect existing configuration. If the project already uses agnostic-ai, run `agnostic-ai import claude` instead.

Import writes source specs only. It does not sync native output or change your target selection. Re-running it overwrites matching source filenames. For a skill, agent, or command the body and every frontmatter key the tool expresses come from the native file, and keys that tool has no field for stay on the spec, so delete such a key in the spec rather than in the native file. Rule frontmatter is rebuilt from the native file alone, because a rule widened to a catch-all `globs` has to come back unscoped.

## Review before syncing

Compare `.agnostic-ai/AGNOSTIC_AI.md` and the imported spec folders against the original configuration.

When importing multiple tools, the last imported top-level instructions replace the shared instructions body. Merge any unique content from other tools into `.agnostic-ai/AGNOSTIC_AI.md` before syncing. See [import behavior by source](@/docs/cli-reference.md#import).

Keep the helper scripts that hooks or settings reference in Git. Files inside a skill directory round-trip with the skill, including imports from Zed, Warp, and Antigravity. Continue imports YAML and JSONC MCP files and preserves their connection options; Claude imports command, HTTP, MCP-tool, and prompt hook handlers. Selected native helpers, including `.claude/statusline.sh`, are captured under `.agnostic-ai/overlays/`. Preserve scripts outside those locations yourself.

For a separate migration checkpoint, commit the reviewed source specs, `agnostic-ai.yaml`, and `.gitignore` before generating output. The instructions source lives inside `.agnostic-ai/`, not at the repository root.

## Keep directory-specific instructions

`import codex` and `import gemini` retain discovered nested instruction directories as `scope`. `import claude` preserves subdirectories within `.claude/rules/`, but does not import every nested `CLAUDE.md`. For other layouts, create a rule with `new rule <name> --scope <directory>` and copy the instructions into it.

Review and commit the imported source. Then move conflicting hand-authored originals out of their native filenames before syncing:

```bash
mv services/payments/AGENTS.md services/payments/AGENTS.md.before-agnostic
agnostic-ai sync --dry-run
```

Keep the saved file until you have checked the generated output. The same applies to conflicting `AGENTS.override.md`, `WARP.md`, `CLAUDE.md`, or `.goosehints` aliases. `sync --backup` does not bypass ownership checks.

Upgrading older generated scopes can move output paths or skip unsupported targets. Run a full `sync`, then `sync --check`, to drop obsolete managed files. See [scoped context](@/docs/scoped-context.md) for compatibility.

## Generate and inspect

```bash
agnostic-ai sync --dry-run
agnostic-ai sync --backup
agnostic-ai sync --check
```

Inspect the diff and the native files. The first sync can add headers, normalize formatting, and replace entry-point content with the shared instructions, so check the content and not only the formatting.

Choose your [Git strategy](@/docs/getting-started.md#commit-or-ignore-generated-outputs). If you commit generated files, keep this initial regeneration in a separate commit from the import. Gitignore rules do not untrack files already in Git. Run `git rm --cached <path>` for each generated file you stop tracking.

## Back up and restore

`sync --backup` creates `.bak` files before overwriting existing outputs. To undo the generated output:

```bash
agnostic-ai revert
```

Revert restores backups where they exist and leaves other files in place. `revert --force` also deletes unbacked generated files. It does not undo edits to source specs. Repeated backup syncs replace earlier backups, so use Git for lasting checkpoints.

Once the migration looks right, `agnostic-ai cleanup` removes the backups sync created. See [revert](@/docs/cli-reference.md#revert) and [cleanup](@/docs/cli-reference.md#cleanup) for filters and previews.

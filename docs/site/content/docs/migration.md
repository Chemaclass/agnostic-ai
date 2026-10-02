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

Import writes source specs only. It does not sync native output or change your target selection. Specs from [`.agnostic-ai/local/`](@/docs/local-overrides.md#import) stay out of the shared source. Re-running it after editing a native file overwrites the matching source file, as long as that spec is unchanged since the last sync wrote it for that tool. A spec edited since then, or never synced to that tool, stops the import before it writes anything; pass `--overwrite` to replace it. For a skill or agent, the body and every frontmatter key the tool writes come from the native file, while keys that tool has no field for stay on the spec. Deleting a key the tool does write removes it from the spec too. Rule frontmatter is rebuilt from the native file alone, because a rule widened to a catch-all `globs` has to come back unscoped.

## Scope and pattern unions

**Breaking change:** `scope` plus `globs` or `paths` now means the union. A rule with `scope: src/a` and `globs: tests/a/**` reaches both directories. Earlier versions intersected them and skipped a rule when its patterns were outside the scope.

Remove `scope` when a rule should apply only to its file patterns, and spell those patterns relative to the project root. Keep the source file outside any folder under `rules/` that names a project directory. Remove `globs: "**/*"` from a rule that should apply only to its scope.

Directory-document targets such as Codex require whole subdirectory patterns, such as `tests/a/**`. An external file filter such as `tests/a/**/*.go`, or a root selector such as `CHANGELOG.md` or `**/*`, warns and skips the rule; `on-unsupported: error` fails sync. File-filter targets can preserve those patterns. Preview with `agnostic-ai render` before syncing. See [scoped context](@/docs/scoped-context.md#narrow-a-rule-to-certain-files).

## Review before syncing

Preview the import before it writes anything:

```bash
agnostic-ai import claude codex --dry-run --diff
```

The preview lists each spec the import would create or change, with a diff, and names the tools that propose different content for the same file. The last tool in the list wins. Reorder the arguments or merge the content by hand if the winner is wrong.

After importing, compare `.agnostic-ai/AGNOSTIC_AI.md` and the imported spec folders against the original configuration.

When importing multiple tools, the last imported top-level instructions replace the shared instructions body. Merge any unique content from other tools into `.agnostic-ai/AGNOSTIC_AI.md` before syncing. `--from all` and `import all` do this for a hand-written root `AGENTS.md`: the sections the shared body lacks are appended below it, and the output names each one. When that `AGENTS.md` is the only config found, it seeds `.agnostic-ai/AGNOSTIC_AI.md` instead. See [import behavior by source](@/docs/cli-reference/start.md#import).

Keep the helper scripts that hooks or settings reference in Git. Files inside a skill directory round-trip with the skill, including imports from Zed, Warp, and Antigravity. Continue imports YAML and JSONC MCP files and preserves their connection options; Claude imports command, HTTP, MCP-tool, and prompt hook handlers. Selected native helpers, including `.claude/statusline.sh`, are captured under `.agnostic-ai/overlays/`. Preserve scripts outside those locations yourself.

For a separate migration checkpoint, commit the reviewed source specs, `agnostic-ai.yaml`, and `.gitignore` before generating output. The instructions source lives inside `.agnostic-ai/`, not at the repository root.

## Keep directory-specific instructions

`import codex` and `import gemini` retain discovered nested instruction directories as `scope`. `import claude` preserves subdirectories within `.claude/rules/`, but does not import every nested `CLAUDE.md`. For other layouts, create a rule with `new rule <name> --scope <directory>` and copy the instructions into it.

Review and commit the imported source, then run `sync`. It replaces a hand-authored original whose text the imported specs hold. A file with a line no spec holds, such as an edit made after the import, stops the run, and the error quotes that line. Move the line into its spec, or set the original aside before syncing:

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

Choose your [Git strategy](@/docs/getting-started.md#commit-or-ignore-generated-outputs). If you commit generated files, keep this initial regeneration in a separate commit from the import. Gitignore rules do not untrack files already in Git: `sync` and `doctor` name each generated path that is still tracked, with the exact `git rm --cached` command; `sync --untrack` runs it (the working tree copy stays). After committing that deletion, other clones lose these files on their next pull. Run `agnostic-ai install-hook --post-checkout` in those clones before pulling to install the checkout and merge hooks, or run `agnostic-ai sync` after pulling. The command prints this reminder, including under `--quiet` and on stderr with `--json`.

## Check packaging after an upgrade

Version 0.75 moved Codex skills from `.codex/skills/` to `.agents/skills/`. An ignore such as `.codex/**` no longer covers these skills. Check `.npmignore`, `.vscodeignore`, and `.dockerignore` when upgrading, and add `.agents/skills/**` where those files should be excluded.

Run `agnostic-ai doctor` after sync. It names generated paths an existing root packaging ignore file does not cover. Then inspect the package with `npm pack --dry-run` or `vsce ls`, or check the Docker build context. Output overrides and future path changes need the same review.

## Back up and restore

`sync --backup` creates `.bak` files before overwriting existing outputs. To undo the generated output:

```bash
agnostic-ai revert
```

Revert restores backups where they exist and leaves other files in place. `revert --force` also deletes unbacked generated files. It does not undo edits to source specs. Repeated backup syncs replace earlier backups, so use Git for lasting checkpoints.

Once the migration looks right, `agnostic-ai cleanup` removes the backups sync created. See [revert](@/docs/cli-reference/maintain.md#revert) and [cleanup](@/docs/cli-reference/maintain.md#cleanup) for filters and previews.

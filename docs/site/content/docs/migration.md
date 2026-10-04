+++
title = "Import existing tool configuration"
description = "Import an existing AI tool setup into agnostic-ai and keep its behavior."
weight = 30

[extra]
group = "Start"
+++

# Import existing tool configuration

Move a project's existing AI tool instructions into `.agnostic-ai/`, review them, then generate output for the tools you pick.

## 1. Import

From the project root, with a clean Git working tree:

```bash
agnostic-ai init --from claude   # or --from all to detect every tool
```

Already using agnostic-ai? Run `agnostic-ai import claude` instead.

Import writes source specs only. It does not sync or change your targets. Re-running it after a native edit overwrites the spec with the same filename, as long as that spec has not changed since the last sync or import of that tool. Otherwise it stops before writing any spec; pass `--overwrite` to replace it. [Local specs](@/docs/local-overrides.md#import) stay out of the shared source. See [import](@/docs/cli-reference/start.md#import) for how it merges frontmatter and multiple sources.

After an import from several tools, a tool whose content was replaced by a later source needs `--overwrite` to replace that source's spec. A sync that skips edited or unmanaged outputs does not count as approval to replace changed source specs.

## 2. Review

Preview before writing:

```bash
agnostic-ai import claude codex --dry-run --diff
```

It shows each spec it would create or change and flags files that two tools disagree on. The last tool listed wins, so reorder the tools or merge by hand.

Then check:

- `.agnostic-ai/AGNOSTIC_AI.md` against your original instructions. With several tools, the last one's instructions win, except a hand-written root `AGENTS.md`, whose missing sections `--from all` appends.
- Helper scripts that hooks or settings call. Skill folders and some native helpers (such as `.claude/statusline.sh`, under `.agnostic-ai/overlays/`) come along. Keep any other script in Git yourself.
- MCP `env` and `headers` values. Import writes each one as a `${NAME}` reference and prints the variable to set, so a token never reaches a committed spec. Plain settings such as `NODE_ENV` become variables too: export them, or put a plain setting back by hand. Sync leaves a reference out of a tool that cannot read it, such as Gemini headers or Amp `env`, with a note. Continue MCP files are copied as they are. See [environment references](@/docs/spec-format/mcps.md#environment-references).

Optional: commit the reviewed specs, `agnostic-ai.yaml`, and `.gitignore` as a checkpoint before generating output.

## Keep directory-specific instructions

`import codex` and `import gemini` keep nested instruction directories as `scope`. `import claude` keeps subfolders of `.claude/rules/`, but not nested `CLAUDE.md` files. For those, run `agnostic-ai new rule <name> --scope <directory>` and copy the text in.

## 3. Generate

```bash
agnostic-ai sync --dry-run
agnostic-ai sync --backup
agnostic-ai sync --check
```

Sync replaces a hand-written file only when the specs hold all its text. Otherwise it stops and quotes the missing line. Move that line into a spec, or set the original aside:

```bash
mv services/payments/AGENTS.md services/payments/AGENTS.md.before-agnostic
```

Keep the saved copy until you have checked the output. `--backup` does not bypass this check.

Review the diff for content, not only formatting: the first sync adds headers and replaces entry points with the shared instructions.

## 4. Commit or ignore outputs

Pick a [Git strategy](@/docs/getting-started.md#commit-or-ignore-generated-outputs). If you commit outputs, put this first regeneration in its own commit.

Ignoring files Git already tracks needs one more step. `sync` and `doctor` list them; `sync --untrack` runs `git rm --cached` (your copy stays). Other clones lose those files on their next pull, so have teammates run `agnostic-ai install-hook --post-checkout` first, or `agnostic-ai sync` after pulling.

## Back up and restore

`sync --backup` saves a `.bak` before overwriting an output. To undo generated output:

```bash
agnostic-ai revert
```

It restores backups and leaves other files. `revert --force` also deletes generated files with no backup. It never undoes spec edits, and each backup sync replaces the last backup, so use Git for lasting checkpoints. When done, `agnostic-ai cleanup` removes the backups. See [revert](@/docs/cli-reference/maintain.md#revert) and [cleanup](@/docs/cli-reference/maintain.md#cleanup).

## Upgrading from older versions

After an upgrade, run `agnostic-ai migrate --dry-run` to see which old spec forms it can rewrite for you, then `agnostic-ai migrate` to apply them. See [migrate](@/docs/cli-reference/maintain.md#migrate). The changes below are the ones it cannot make, because they change what sync writes.

### Scope and pattern unions

**Breaking change:** `scope` plus `globs` or `paths` now applies to both, not their overlap. Remove `scope` when a rule should match only its patterns, and remove `globs: "**/*"` when it should match only its scope. Preview with `agnostic-ai render`. See [scoped context](@/docs/scoped-context.md#narrow-a-rule-to-certain-files).

Older generated scopes can move or skip output on upgrade. Run `sync`, then `sync --check`, to drop obsolete files.

### Codex skills moved in 0.75

Codex skills moved from `.codex/skills/` to `.agents/skills/`. Add `.agents/skills/**` to `.npmignore`, `.vscodeignore`, or `.dockerignore` where you exclude them. `agnostic-ai doctor` names generated paths a packaging ignore file misses; confirm with `npm pack --dry-run`, `vsce ls`, or your Docker context.

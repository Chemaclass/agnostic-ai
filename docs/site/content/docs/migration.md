+++
title = "Import existing tool configuration"
description = "Import an existing AI tool setup into agnostic-ai and keep its behavior."
weight = 30

[extra]
nav_title = "Import existing setup"
group = "Start"
+++

# Import existing tool configuration

Import copies your tool setup into editable sources in `.agnostic-ai/`. [Install agnostic-ai](@/docs/installation.md), then work from the project root with a clean Git tree so you can review and undo.

## Import the existing setup {#1-import}

For a project not yet using agnostic-ai:

```bash
agnostic-ai init --from claude
```

`--from all` detects every tool; select only the ones the project uses. In a project that already uses agnostic-ai, preview first:

```bash
agnostic-ai import claude --dry-run --diff
agnostic-ai import claude
```

Import writes source specs only. Generated files and selected targets stay unchanged.

## Review what was imported {#review-what-was-imported}

- Compare `.agnostic-ai/AGNOSTIC_AI.md` and the imported specs with the originals. With several tools, [review the import order](#2-review) first.
- With `--from all`, a hand-written root `AGENTS.md` contributes missing sections.
- Check scripts called by hooks and settings, and MCP connection variables. Skill folders and some tool scripts are imported; other referenced scripts must stay in your project.
- See [MCP import](@/docs/spec-format/mcp-import.md) for credential references and connection differences.

## Generate and check {#3-generate}

```bash
agnostic-ai sync --dry-run --backup
agnostic-ai sync --backup
agnostic-ai sync --check
```

The first sync adds generated headers and replaces root instruction files with the shared instructions. Read the native files before your next session.

## Commit or ignore outputs {#4-commit-or-ignore-outputs}

Choose a [Git strategy](@/docs/getting-started.md#commit-or-ignore-generated-outputs). If you commit outputs, give the first sync its own commit.

To ignore files Git already tracks, run `sync --untrack` (`git rm --cached`; your copy stays). Other clones lose those files on the next pull, so teammates run `agnostic-ai sync` after pulling, or install [checkout hooks](@/docs/git-hooks.md#regenerate-on-checkout) first.

From now on, edit `.agnostic-ai/` and sync ([daily use](@/docs/getting-started.md#daily-use)).

## Review imports from several tools {#2-review}

Preview several tools with `agnostic-ai import claude codex --dry-run --diff`. Imports can merge shared instructions and keep conflicting versions; resolve differences against the originals before you sync.

<details>
<summary>Re-import after editing a native tool file</summary>

Re-running import with no edits leaves every spec as it is. After you edit a tool's own file, import brings the edit back into the spec, or stops when that would lose content. See [keep existing specs](@/docs/cli-reference/import.md#keep-existing-specs) and [bring tool edits back](@/docs/cli-reference/import.md#bring-tool-edits-back). [Local specs](@/docs/local-overrides.md#import) stay out of the shared source.

</details>

<details>
<summary>Keep directory-specific and alternate instruction files</summary>

## Keep directory-specific instructions

`import codex` and `import gemini` keep nested instruction directories as `scope`. `import claude` imports `.claude/rules/` when that directory exists. Otherwise, it imports nested `CLAUDE.md` files as directory-scoped rules. If `.claude/rules/` exists alongside nested `CLAUDE.md` files, create a rule with `agnostic-ai new rule <name> --dir <directory>` and copy each nested file’s text into it.

For directory-specific instruction files, sync replaces a hand-written file only when the specs contain all its text. Otherwise it stops and quotes the missing line. Move that line into a spec, or set the original aside:

```bash
mv services/payments/AGENTS.md services/payments/AGENTS.md.before-agnostic
```

Keep the saved copy until you have checked the output. `--backup` does not bypass this directory-specific check. Root instruction files have a separate [backup and overwrite rule](@/docs/cli-reference/sync.md#sync).

</details>

<details>
<summary>Restore generated files from backups</summary>

## Back up and restore

`sync --backup` saves a `.bak` copy before overwriting a file. To undo generated files:

```bash
agnostic-ai revert
```

It restores the backups but never undoes spec edits; use Git for a lasting copy. `agnostic-ai cleanup` removes the backups. See [revert](@/docs/cli-reference/maintain.md#revert) and [cleanup](@/docs/cli-reference/maintain.md#cleanup).

</details>

<details>
<summary>Upgrade an older agnostic-ai setup</summary>

## Upgrading from older versions

After an upgrade, `agnostic-ai migrate --dry-run` shows which old spec forms it can rewrite, and `agnostic-ai migrate` applies them. See [migrate](@/docs/cli-reference/upgrades.md#migrate). The changes below need more than `migrate`.

### Scope flags and layer names

**Breaking change:** layer names changed in `list`, `status`, `explain --layers`, the web interface and adapter input. `project-user` is now `local`, `global` is `user`, and `global-local` is `user-local`. Update scripts that match the old names.

These flags are deprecated. The old spellings still work and print a warning:

| Old | New |
|---|---|
| `--global` | `--scope user` |
| `new rule --scope <dir>` | `new rule --dir <dir>` |
| `doctor --scope` | `doctor --checks` |

`new rule --scope <dir>` still reads the value as a folder only when it has a slash or names an existing folder, and is not `project`, `local`, `user` or `global`. Use `--dir` to be sure.

See [Scopes](@/docs/scopes.md#older-names).

### Unmarked MCP literals

**Breaking change:** `lint` and `sync` fail on an MCP `env` or `headers` value that is neither a `${NAME}` reference nor marked `!literal` (LINT035). Run `agnostic-ai migrate --only secrets`, then fix the values it leaves to you. See [secrets and plain settings](@/docs/spec-format/mcps.md#plain-settings).

### Scope and pattern unions

**Breaking change:** `scope` plus `globs` or `paths` now applies to both, not their overlap. Remove `scope` when a rule should match only its patterns, and remove `globs: "**/*"` when it should match only its scope. Preview with `agnostic-ai render`. See [scoped context](@/docs/scoped-context.md#narrow-a-rule-to-certain-files).

An upgrade can move or skip files that older versions wrote for scoped rules. Run `sync`, then `sync --check`, to remove them.

### Codex skills moved in 0.75

Codex skills moved from `.codex/skills/` to `.agents/skills/`. Add `.agents/skills/**` to `.npmignore`, `.vscodeignore`, or `.dockerignore` where you exclude them. `agnostic-ai doctor` names generated paths that a packaging ignore file misses; confirm with `npm pack --dry-run`, `vsce ls`, or your Docker context.

</details>

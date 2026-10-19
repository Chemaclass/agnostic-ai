+++
title = "Import existing tool configuration"
description = "Import an existing AI tool setup into agnostic-ai and keep its behavior."
weight = 30

[extra]
nav_title = "Import existing setup"
group = "Start"
+++

# Import existing tool configuration

Keep your current instructions while moving their editable sources into `.agnostic-ai/`. Start from the project root with a clean Git working tree so you can review and undo the import. [Install agnostic-ai](@/docs/installation.md) first.

## Import the existing setup {#1-import}

For a project not yet using agnostic-ai:

```bash
agnostic-ai init --from claude
```

Use `--from all` to detect every tool. Select only the coding tools the project uses. Already configured? Preview an import before writing:

```bash
agnostic-ai import claude --dry-run --diff
agnostic-ai import claude
```

Import writes source specs. It does not sync. The separate `import` command does not change selected targets.

## Review what was imported {#review-what-was-imported}

Compare `.agnostic-ai/AGNOSTIC_AI.md` and imported specs with the original files. With several tools, [review their import order](#2-review) before syncing. A hand-written root `AGENTS.md` contributes missing sections with `--from all`.

Check scripts called by hooks and settings, and inspect MCP connection variables before committing. Skill folders and some tool scripts are imported; other referenced scripts must remain available in your project. [MCP import details](@/docs/spec-format/mcp-import.md) explain credential references and connection differences.

## Generate and check {#3-generate}

```bash
agnostic-ai sync --dry-run
agnostic-ai sync --backup
agnostic-ai sync --check
```

Review the diff for content. The first sync adds generated headers and replaces root instruction files with the shared instructions. Read the resulting native files before starting a fresh coding-tool session.

## Commit or ignore outputs {#4-commit-or-ignore-outputs}

Choose a [Git strategy](@/docs/getting-started.md#commit-or-ignore-generated-outputs). If you commit outputs, keep the first sync in its own commit.

Ignoring files Git already tracks needs an explicit step: `sync --untrack` runs `git rm --cached`, keeping your working copy. Other clones lose those files on the next pull. Teammates must run `agnostic-ai sync` after pulling, or install [checkout hooks](@/docs/git-hooks.md#regenerate-on-checkout) first.

From now on, edit `.agnostic-ai/` sources and sync. Use [Daily use](@/docs/getting-started.md#daily-use) for the normal workflow.

## Review imports from several tools {#2-review}

Preview several tools at once with `agnostic-ai import claude codex --dry-run --diff`. The last tool listed wins for a shared spec, so reorder the tools or merge the sources by hand before generating files.

<details>
<summary>Re-import after editing a native tool file</summary>

Re-running import with no edits leaves every spec as it is. After you edit a tool's own file, import brings the edit back into the spec, or stops when that would lose content. See [keep existing specs](@/docs/cli-reference/start.md#keep-existing-specs) and [bring tool edits back](@/docs/cli-reference/start.md#bring-tool-edits-back). [Local specs](@/docs/local-overrides.md#import) stay out of the shared source.

</details>

<details>
<summary>Keep directory-specific and alternate instruction files</summary>

## Keep directory-specific instructions

`import codex` and `import gemini` keep nested instruction directories as `scope`. `import claude` imports `.claude/rules/` when that directory exists. Otherwise, it imports nested `CLAUDE.md` files as directory-scoped rules. If `.claude/rules/` exists alongside nested `CLAUDE.md` files, create a rule with `agnostic-ai new rule <name> --scope <directory>` and copy each nested file’s text into it.


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

It restores the backups. It never undoes spec edits; use Git for a lasting copy. When done, `agnostic-ai cleanup` removes the backups. See [revert](@/docs/cli-reference/maintain.md#revert) and [cleanup](@/docs/cli-reference/maintain.md#cleanup).

</details>

<details>
<summary>Upgrade an older agnostic-ai setup</summary>

## Upgrading from older versions

After an upgrade, run `agnostic-ai migrate --dry-run` to see which old spec forms it can rewrite for you, then `agnostic-ai migrate` to apply them. See [migrate](@/docs/cli-reference/upgrades.md#migrate). The changes below need more than `migrate`.

### Unmarked MCP literals

**Breaking change:** `lint` and `sync` fail on an MCP `env` or `headers` value that is neither a `${NAME}` reference nor marked `!literal` (LINT035). Run `agnostic-ai migrate --only secrets`, then fix the values it leaves to you. See [secrets and plain settings](@/docs/spec-format/mcps.md#plain-settings).

### Scope and pattern unions

**Breaking change:** `scope` plus `globs` or `paths` now applies to both, not their overlap. Remove `scope` when a rule should match only its patterns, and remove `globs: "**/*"` when it should match only its scope. Preview with `agnostic-ai render`. See [scoped context](@/docs/scoped-context.md#narrow-a-rule-to-certain-files).

An upgrade can move or skip files that older versions wrote for scoped rules. Run `sync`, then `sync --check`, to remove the old files.

### Codex skills moved in 0.75

Codex skills moved from `.codex/skills/` to `.agents/skills/`. Add `.agents/skills/**` to `.npmignore`, `.vscodeignore`, or `.dockerignore` where you exclude them. `agnostic-ai doctor` names generated paths that a packaging ignore file misses; confirm with `npm pack --dry-run`, `vsce ls`, or your Docker context.

</details>

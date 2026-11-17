+++
title = "Git hooks"
description = "Check for out-of-date generated files at commit time, and regenerate them after checkout or pull."
weight = 65

[extra]
group = "Workflows"
+++

# Git hooks

Git hooks check or regenerate files on commit and checkout. ([Coding-agent hooks](@/docs/spec-format/hooks.md) are different: they run inside a coding tool.) Pick a recipe by whether your team [commits or ignores outputs](@/docs/getting-started.md#commit-or-ignore-generated-outputs):

- Committed outputs: [check the staged files](#check-staged-files) at commit time, so you never commit output you forgot to stage.
- Ignored outputs: run `sync` once in each checkout, then [regenerate on checkout and pull](#regenerate-on-checkout) so coding agents find their files. In CI, use the [ignored-output recipe](@/docs/ci.md#ignored-outputs).

## Install the project hooks

```bash
agnostic-ai install-hook
agnostic-ai install-hook --post-checkout
```

The first installs the commit check. The second regenerates files after checkout and pull. Install once per checkout. With a hook manager, use its recipe instead of replacing its hooks.

<details>
<summary>Use pre-commit, lefthook, or husky instead</summary>

## pre-commit (Python)

[pre-commit](https://pre-commit.com) works in projects that use several languages. Add `.pre-commit-config.yaml`:

```yaml
repos:
  - repo: local
    hooks:
      - id: agnostic-ai-check
        name: agnostic-ai sync --check
        entry: agnostic-ai sync --check
        language: system
        pass_filenames: false
        files: '^(\.agnostic-ai/|agnostic-ai\.yaml$|agnostic\.config\.yaml$)'
```

Then install it:

```bash
pre-commit install
```

`files:` runs the hook only when a spec changes. `pass_filenames: false` checks the whole project, like CI.

## lefthook

[lefthook](https://lefthook.dev) is a single Go binary. This repo uses it (see [`lefthook.yml`](https://github.com/Chemaclass/agnostic-ai/blob/main/lefthook.yml)). Add to `lefthook.yml`:

```yaml
pre-commit:
  commands:
    agnostic-ai-check:
      glob: "{.agnostic-ai/**,agnostic-ai.yaml}"
      run: agnostic-ai sync --check
```

Then install it:

```bash
lefthook install
```

`glob:` runs the hook only when a spec or the config is staged.

## husky + lint-staged

In Node projects, use [husky](https://typicode.github.io/husky) with [lint-staged](https://github.com/lint-staged/lint-staged). `package.json`:

```json
{
  "scripts": {
    "prepare": "husky"
  },
  "lint-staged": {
    "{.agnostic-ai/**,agnostic-ai.yaml}": "agnostic-ai sync --check --against index --"
  }
}
```

`.husky/pre-commit`:

```sh
npx lint-staged
```

Then install it:

```bash
npm install
```

The trailing `--` keeps the file paths lint-staged adds from being read as options. `sync --check` checks the whole project, not single files.

</details>

## Check the staged files {#check-staged-files}

A plain `sync --check` reads the working tree, so it passes when `sync` rewrote `AGENTS.md` and you did not stage it. `--against index` compares the staged specs with the staged outputs instead:

```bash
agnostic-ai sync --check --against index
```

A staged spec change without its regenerated output fails. The error names the file and tells you to run `agnostic-ai sync` and `git add` the output (for `--against HEAD`, commit it). The hook from `install-hook` runs this form; run `install-hook` again to update a hook an older version wrote.

With `gitignore.enabled: true`, the hook only checks that the staged specs render, since `--against index` compares tracked outputs only. A plain `sync --check` compares all.

<details>
<summary>Limits of --against</summary>

- Each commit copies the tracked files to a temporary folder. In a large repo, run it only when specs are staged (the lint-staged recipe).
- `--against HEAD` checks the last commit; see the [CI recipe](@/docs/ci.md#committed-outputs).
- What it compares and skips: [`--against` details](@/docs/cli-reference/sync.md#against-details).

</details>

A hook manager that runs only when matching files are staged needs a glob covering every input, including a README that a review pulls in with `@path`. `agnostic-ai explain --inputs` prints them:

```bash
agnostic-ai explain --inputs
# .agnostic-ai/**
# .gitignore
# agnostic-ai.local.yaml
# agnostic-ai.yaml
# apps/web/README.md
```

Paste the list into the glob, or drop the glob. Rerun after adding an `@path` line.

## Regenerate on checkout and pull {#regenerate-on-checkout}

With ignored outputs (`gitignore.enabled: true`), a fresh clone or worktree has no tool files until `sync` runs, so an automated session can start without its config.

`git checkout`, `git clone`, and `git worktree add` run `post-checkout`. A pull that merges runs `post-merge`, which also restores generated files untracked by the incoming commit.

`agnostic-ai install-hook --post-checkout` writes both hooks. They prefer the local binary, run `agnostic-ai project` from the worktree root (keeps manual edits, installs nothing), skip a checkout without `agnostic-ai.yaml`, and print a recovery command when the binary is missing. Rerun it to update older hooks and add pull coverage.

To write them yourself, use [`sync --keep-edits`](@/docs/cli-reference/sync.md#sync): it writes every other output and leaves files you edited since the last sync, printing `~ kept <path>`. To keep an edit, move it into `.agnostic-ai/` and sync.

lefthook (`lefthook.yml`):

```yaml
post-checkout:
  commands:
    sync:
      run: agnostic-ai sync --keep-edits
```

Plain git (`.git/hooks/post-checkout`, `chmod +x`):

```sh
#!/bin/sh
agnostic-ai sync --keep-edits
```

To run only on branch and worktree switches, check the third argument (`0` for a file checkout):

```sh
#!/bin/sh
[ "$3" = "1" ] || exit 0   # 1 = branch checkout, 0 = file checkout
agnostic-ai sync --keep-edits
```

The same command works in `post-merge` (no `$3` check) and in `postinstall`. A pull with rebase needs `post-rewrite` (see [Node monorepos](#node-monorepos)).

`agnostic-ai` must be on `PATH` wherever the repo is checked out; document the install in `CONTRIBUTING.md`. If some contributors lack the CLI, commit the outputs, or in a Node project install the CLI with the dependencies ([Node monorepos](#node-monorepos)).

<details>
<summary>Pin the CLI and restore files in Node workspaces and worktrees</summary>

## Node monorepos {#node-monorepos}

In a pnpm workspace with ignored outputs, `pnpm install` creates every tool file, and git hooks regenerate them on each branch switch. npm and Yarn workspaces work the same way.

### Pin the CLI

Add the npm package to the workspace root as an exact dev dependency:

```bash
pnpm add -D -E -w agnostic-ai   # pnpm workspace
npm install -D -E agnostic-ai   # npm; or yarn add -D -E, bun add -D -E
```

The lockfile covers every OS and CPU, so one pin works on macOS, Linux, and Windows. Also set `requires: ">=X.Y.Z"` in `agnostic-ai.yaml`, so an older installed version stops with [AAI-005](@/docs/errors.md#aai-005-installed-version-outside-requires) before rewriting files. If you commit generated files, use an exact `requires: "X.Y.Z"` instead, since a newer release can write different output. [Requires](@/docs/configuration.md#requires) also accepts ranges.

### Sync on install

Root `package.json`:

```json
{
  "scripts": {
    "postinstall": "agnostic-ai project"
  }
}
```

Package scripts find the pinned binary in `node_modules/.bin`. Cloud agents and IDE worktree setups (Cursor Cloud, Codex environments, `.cursor/worktrees.json`) run `pnpm install` but no git hook, so this step is how they get their tool files.

### Sync on checkout, pull, and rebase

`lefthook.yml`:

```yaml
post-checkout:
  commands:
    agnostic-ai-sync:
      run: node_modules/.bin/agnostic-ai project
post-merge:
  commands:
    agnostic-ai-sync:
      run: node_modules/.bin/agnostic-ai project
post-rewrite:
  commands:
    agnostic-ai-sync:
      run: node_modules/.bin/agnostic-ai project
```

`post-merge` runs after `git pull`, and `post-rewrite` after a rebase or `git commit --amend`. Git hooks do not put `node_modules/.bin` on `PATH`, so these commands name the binary by path. If it is missing, run the package install, or a global `agnostic-ai project --bootstrap` with a version that supports the command. `--quiet` hides the summary but still reports `~ kept <path>`.

### First pull

When a branch changes the pinned version, `agnostic-ai project --check` reports the installed and required versions without installing anything, and `agnostic-ai project --bootstrap` installs from the lockfile once, then syncs with `--keep-edits --quiet`. Use these instead of comparing manifest text in shell. A hook manager can call `agnostic-ai project --check --against index` for a read-only pre-commit gate. See [`project`](@/docs/cli-reference/project.md#project) for version checks, supported package managers, and the releases that lack the command.

Generated hooks first check that the selected binary supports `project`. When no `agnostic-ai` is on `PATH`, they name the install to run, for example `bun install --frozen-lockfile`. Built-in memory hooks look upward for `node_modules/.bin/agnostic-ai` and stop at the nearest config, so a project nested inside another Git checkout uses its own package or the one on `PATH`.

### New worktrees

A new linked worktree has no `node_modules`, so the pinned binary is missing and a hook runner installed from npm, such as lefthook, may not start. The worktree gets its tool files once `pnpm install` runs in it. Put the install in an environment spec, `.agnostic-ai/environments/dev.yaml`:

```yaml
name: dev
setup: agnostic-ai project --bootstrap
```

This needs a global agnostic-ai that supports `project`, installed before worktree setup. Codex and Cursor run `setup` in a new worktree from their own files. For Claude Code, sync writes hooks that run it once in each new worktree. Delete any hand-written `bootstrap.yaml` hook from an earlier version of this recipe, because Claude Code runs matching hooks in parallel. See [Claude Code worktree setup](@/docs/spec-format/environments.md#claude-code-worktree-setup).

Claude Code copies the gitignored files that `.worktreeinclude` lists into each new worktree. When `claude` is a target, sync keeps its block there (see [gitignore](@/docs/sync-settings.md#gitignore)), so the ignored `.claude/settings.json` and the setup script reach the worktree. Otherwise, commit both so a new worktree has them before any sync runs. In `agnostic-ai.yaml`:

```yaml
gitignore:
  enabled: true
  commit: [claude:environments]
```

Commit the files after `sync` writes them. A fresh install writes the same content, so `git status` stays clean.

### Workspace settings

`pnpm-workspace.yaml`:

```yaml
allowBuilds:
  lefthook: true
minimumReleaseAge: 1440
minimumReleaseAgeExclude:
  - agnostic-ai
  - '@agnostic-ai/*'
```

- `allowBuilds` lets lefthook's install script add the git hooks. pnpm 11 fails the install when a dependency's build script is not approved. pnpm 10 calls the setting `onlyBuiltDependencies`.
- `minimumReleaseAge` needs both exclusions, because the binary ships in separate `@agnostic-ai/<os>-<arch>` packages and the cooldown holds back each one on its own.

### Formatters

Put ignore patterns in a fenced code block. Prettier escapes `*` in plain Markdown text (`/res/*` becomes `/res/\*`) but leaves code blocks alone. See [ignore specs](@/docs/spec-format/ignore.md).

Keep formatters away from committed generated files, or each sync undoes their edits. Prettier already skips files that `.gitignore` lists. `.prettierignore`:

```text
.claude/settings.json
```

</details>

To skip a hook for one commit, use `git commit --no-verify`. Save it for emergencies.

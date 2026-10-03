+++
title = "Git hooks"
description = "Run sync checks around commits, checkouts, and pulls with pre-commit, lefthook, or husky."
weight = 60

[extra]
group = "Workflows"
+++

# Git hooks

Catch spec drift at commit time, before CI. Each recipe runs `agnostic-ai sync --check` when a spec or `agnostic-ai.yaml` is staged. It blocks the commit if a generated file is out of date.

It is the same check as the [CI gate](@/docs/ci.md). A local run compares the whole working tree, not only staged files.

If outputs are ignored, run `sync` when you set up each checkout, before you enable a drift hook. In CI, use the [ignored-output recipe](@/docs/ci.md#ignored-outputs).

## Why a pre-commit hook

- You see drift when you create it, not in the next CI run.
- Every contributor gets the same gate.
- It is opt-in per checkout, so a fresh clone works without setup.

## pre-commit (Python)

[pre-commit](https://pre-commit.com) is common in polyglot repos. Add `.pre-commit-config.yaml`:

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

Install once per checkout:

```bash
pre-commit install
```

`files:` limits the hook to spec changes. `pass_filenames: false` checks the whole project, like CI, instead of passing each staged path.

## lefthook

[lefthook](https://lefthook.dev) is a single Go binary. This repo uses it (see [`lefthook.yml`](https://github.com/Chemaclass/agnostic-ai/blob/main/lefthook.yml)).

Add to `lefthook.yml`:

```yaml
pre-commit:
  commands:
    agnostic-ai-check:
      glob: "{.agnostic-ai/**,agnostic-ai.yaml}"
      run: agnostic-ai sync --check
```

Install once per checkout:

```bash
lefthook install
```

`glob:` skips the hook unless a spec or the config is staged.

## husky + lint-staged

In Node projects, use [husky](https://typicode.github.io/husky) with [lint-staged](https://github.com/lint-staged/lint-staged).

`package.json`:

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

Install once per checkout:

```bash
npm install
```

The trailing `--` absorbs the staged paths lint-staged appends. `sync --check` reads the project root, not single files.

## Check the staged files {#check-staged-files}

A plain `sync --check` reads the working tree. If `sync` rewrote `AGENTS.md` and you did not stage it, the check passes and you commit a stale file. Add `--against index` to render the staged specs and compare them with the staged outputs:

```bash
agnostic-ai sync --check --against index
```

A staged spec change without its regenerated output fails. The error names the file and says to run `agnostic-ai sync`, then stage the output with `git add` (or, for `--against HEAD`, commit it). The hook from `agnostic-ai install-hook` runs this form. Run `install-hook` again to update a hook an older version wrote.

{% <details summary="Limits of --against"> %}
- `--against` needs agnostic-ai 0.74.0 or later. A `--shared` hook blocks commits for a teammate on an older release until they upgrade.
- The check copies the tracked files to a temporary folder on each commit. In a large repository, use the lint-staged recipe above so it runs only when specs are staged.
- `--against HEAD` checks the last commit instead. Use it in a CI job that runs after `postinstall` has rewritten the working tree.
- Only outputs Git tracks are compared, so an output that `gitignore` leaves out never fails.
- A tracked output whose spec is gone fails until you delete it, because the check also renders the last commit's specs.
- Specs come from the same state, so an untracked `agnostic-ai.local.yaml` or `.agnostic-ai/local/` is not read.
{% </details> %}

If your hook manager runs a command only when matching files are staged, the glob must list every input, including a README that a review inlines with `@path`. `agnostic-ai explain --inputs` prints them, one per line:

```bash
agnostic-ai explain --inputs
# .agnostic-ai/**
# .gitignore
# agnostic-ai.local.yaml
# agnostic-ai.yaml
# apps/web/README.md
```

Paste the list into the glob, or drop the glob and check every commit. Rerun it after you add an `@path` line.

## Regenerate on checkout and pull {#regenerate-on-checkout}

Commit hooks do not help when generated outputs are gitignored (`gitignore.enabled: true`). A fresh clone or a new `git worktree` has no `CLAUDE.md`, rules, or hooks until someone runs `sync`. Automated worktree creation cannot run it, so an AI session there finds no config.

A `post-checkout` hook fixes this. `git checkout`, `git clone`, and `git worktree add` all fire it. A pull that merges fires `post-merge` instead. That hook restores files Git removes when an incoming commit untracks generated outputs.

`agnostic-ai install-hook --post-checkout` writes both hooks. Both run `agnostic-ai sync -q` from the worktree root and exit 0 when the binary or `agnostic-ai.yaml` is missing. Only `post-checkout` checks the branch-checkout flag below. In a clone with the older checkout-only hook, run the command again to add pull coverage. These hooks run a plain `sync`, which overwrites a hand edit to a generated file such as `AGENTS.md`. Git carries an uncommitted edit across a checkout, so to keep such edits, use the recipes below.

The recipes use `sync --keep-edits`. With `--keep-edits`, sync writes every other output and leaves each file edited since the last sync in place, naming it as `~ kept <path>`. A file with no ledger entry, as in a new linked worktree, counts as edited when it differs from `HEAD`. Move the edit into `.agnostic-ai/`, then run `agnostic-ai sync`.

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

`post-checkout` receives three arguments, and a file checkout passes `0` as the third. Guard on it to run only on branch and worktree switches:

```sh
#!/bin/sh
[ "$3" = "1" ] || exit 0   # 1 = branch checkout, 0 = file checkout
agnostic-ai sync --keep-edits
```

The same command works in `post-merge` and in a `postinstall` script. Leave out the `$3` guard in `post-merge`, which gets no checkout flag. A pull with rebase needs a `post-rewrite` hook (see [Node monorepos](#node-monorepos)).

`agnostic-ai` must be on `PATH` wherever the repo is checked out. If some contributors lack the CLI, commit the generated outputs instead of ignoring them. A Node project can install the CLI with its dependencies instead (see [Node monorepos](#node-monorepos)).

## Node monorepos {#node-monorepos}

In a pnpm workspace with ignored outputs, `pnpm install` creates every tool file, and git hooks regenerate them on each branch switch. npm and Yarn workspaces work the same way with their own commands.

### Pin the CLI

Add the npm package to the workspace root as an exact dev dependency:

```bash
pnpm add -D -E -w agnostic-ai
```

Everyone then runs the same release. Committed generated files can differ between releases, so also set `requires: "X.Y.Z"` in `agnostic-ai.yaml`. A newer global install or a stale `node_modules` then stops with [AAI-005](@/docs/errors.md#aai-005-installed-version-outside-requires) before it rewrites those files. [`requires`](@/docs/configuration.md#requires) accepts ranges too.

### Sync on install

Root `package.json`:

```json
{
  "scripts": {
    "postinstall": "agnostic-ai sync --keep-edits --quiet"
  }
}
```

Package scripts find the pinned binary in `node_modules/.bin`. Cloud agents and IDE worktree setups (Cursor Cloud, Codex environments, `.cursor/worktrees.json`) run `pnpm install` but no git hook. This step gives them their tool files.

### Sync on checkout, pull, and rebase

`lefthook.yml`:

```yaml
post-checkout:
  commands:
    agnostic-ai-sync:
      run: '[ ! -x node_modules/.bin/agnostic-ai ] || node_modules/.bin/agnostic-ai sync --keep-edits --quiet'
post-merge:
  commands:
    agnostic-ai-sync:
      run: '[ ! -x node_modules/.bin/agnostic-ai ] || node_modules/.bin/agnostic-ai sync --keep-edits --quiet'
post-rewrite:
  commands:
    agnostic-ai-sync:
      run: '[ ! -x node_modules/.bin/agnostic-ai ] || node_modules/.bin/agnostic-ai sync --keep-edits --quiet'
```

`post-merge` runs after `git pull`, and `post-rewrite` after a rebase or `git commit --amend`. Git hooks do not put `node_modules/.bin` on `PATH`, so the command names the binary by path. The guard skips a checkout that has not installed dependencies yet.

`--quiet` hides the routine summary. A `~ kept <path>` line still prints on stderr for each file the hook left alone.

### First pull

A developer on a commit from before agnostic-ai has no checkout hooks yet. The pull that adds agnostic-ai removes the outputs that commit tracked, and no hook regenerates them. Lefthook reads `lefthook.yml` from the working tree, so a pre-commit step in the new config runs on their next commit. It installs dependencies when the CLI in `node_modules` is not the pinned one. The install runs the `postinstall` sync and adds the checkout hooks:

```yaml
pre-commit:
  commands:
    agnostic-ai-install:
      run: |
        want=$(node -p "require('./package.json').devDependencies['agnostic-ai']")
        have=$(node_modules/.bin/agnostic-ai --version 2>/dev/null | awk '{print $3}')
        [ "$have" = "$want" ] || pnpm install --frozen-lockfile --prefer-offline
```

When the pin is installed, the step costs one `--version` call.

### New worktrees

A new linked worktree has no `node_modules`. A hook runner installed from npm, such as lefthook, may not start there, and the pinned binary is missing either way. The worktree gets its tool files once `pnpm install` runs in it.

Put the install in an environment spec, `.agnostic-ai/environments/dev.yaml`:

```yaml
name: dev
setup: pnpm install --frozen-lockfile
```

Codex and Cursor run `setup` in a new worktree from their own files. For Claude Code, sync writes hooks that run it once in each new worktree (a `--worktree` or Desktop session, a subagent worktree, or a worktree Claude enters during a session). Delete any hand-written `bootstrap.yaml` hook from an earlier version of this recipe: Claude Code runs matching hooks in parallel. See [Claude Code worktree setup](@/docs/spec-format/environments.md#claude-code-worktree-setup).

Claude Code copies the gitignored files that `.worktreeinclude` lists into each new worktree. When `claude` is a target, sync keeps its managed block there (see [gitignore](@/docs/configuration.md#gitignore)), so the ignored `.claude/settings.json` and the setup script reach the worktree. Otherwise, commit both, so a new worktree has them before any sync runs. In `agnostic-ai.yaml`:

```yaml
gitignore:
  enabled: true
  commit: [claude:environments]
```

Commit the files after `sync` writes them. A fresh install writes the same bytes, so `git status` stays clean.

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
- `minimumReleaseAge` needs both exclusions. The binary ships in separate `@agnostic-ai/<os>-<arch>` packages, and each fails the cooldown on its own.

### Formatters

Put ignore patterns in a fenced code block. Prettier escapes `*` in plain Markdown text (`/res/*` becomes `/res/\*`) but leaves code blocks alone. See [ignore specs](@/docs/spec-format/ignore.md).

Keep formatters away from committed generated files, or each sync undoes their edits. Prettier already skips files `.gitignore` lists. `.prettierignore`:

```text
.claude/settings.json
```

## Tips

- The hook needs `agnostic-ai` on `PATH`. Document the install in `CONTRIBUTING.md` so a new contributor does not hit `command not found` on their first commit.
- To recover from drift, run `agnostic-ai sync` and stage the regenerated outputs with the spec change.
- With `gitignore.enabled: true`, the `install-hook` hook checks that the staged specs render, because `--against index` compares only outputs Git tracks. A plain `sync --check` still compares every output.
- Skip a hook for one commit with `git commit --no-verify`. Save it for emergencies.

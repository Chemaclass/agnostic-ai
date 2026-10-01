+++
title = "Git hooks"
description = "Run sync checks around commits, checkouts, and pulls with pre-commit, lefthook, or husky."
weight = 60

[extra]
group = "Workflows"
+++

# Git hooks


Catch spec drift at commit time, before CI. Each recipe runs `agnostic-ai sync --check` whenever a spec or `agnostic-ai.yaml` is staged, blocking the commit if any generated file is out of date.

The same `sync --check` powers the [CI gate](@/docs/ci.md), and a local run compares your whole working tree, not only staged files.

For ignored outputs, run `sync` when bootstrapping each checkout before enabling a drift hook. In CI, use the [ignored-output recipe](@/docs/ci.md#ignored-outputs).

## Why a pre-commit hook

- Drift surfaces when you create it, not 30 seconds into the next CI run.
- Same gate on every contributor and machine.
- Opt-in per checkout, so a fresh clone still works without setup.

## pre-commit (Python)

[pre-commit](https://pre-commit.com) is the common choice in polyglot repos. Add `.pre-commit-config.yaml`:

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

`files:` scopes the hook to spec changes, so unrelated commits skip it. `pass_filenames: false` runs the binary on the whole project, like CI, instead of passing each staged path.

## lefthook

[lefthook](https://lefthook.dev) is a single Go binary, no runtime dependency. This repo uses it: see [`lefthook.yml`](https://github.com/Chemaclass/agnostic-ai/blob/main/lefthook.yml).

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

`glob:` keeps the hook silent unless a spec or the config is staged.

## husky + lint-staged

In Node projects, [husky](https://typicode.github.io/husky) plus [lint-staged](https://github.com/lint-staged/lint-staged) is the usual pairing.

`package.json`:

```json
{
  "scripts": {
    "prepare": "husky"
  },
  "lint-staged": {
    "{.agnostic-ai/**,agnostic-ai.yaml}": "agnostic-ai sync --check --"
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

The trailing `--` swallows the staged paths lint-staged appends, because `sync --check` reads the project root, not individual files.

## Check the staged files {#check-staged-files}

A plain `sync --check` reads the working tree. When `sync` already rewrote `AGENTS.md` but you did not stage it, the check passes and the commit goes out stale. Add `--against index` to render the staged specs and compare them with the staged outputs:

```bash
agnostic-ai sync --check --against index
```

A staged spec change without its regenerated output fails, names the file, and says to run `agnostic-ai sync`, then stage the output with `git add` (or, for `--against HEAD`, commit it). The hook `agnostic-ai install-hook` writes runs this form; running `install-hook` again updates a hook an older version wrote. `--against` needs agnostic-ai 0.74.0 or later, so a `--shared` hook blocks commits for a teammate on an older release until they upgrade. The check copies the tracked files to a temporary folder on each commit; in a large repository, the lint-staged recipe above runs it only when specs are staged. `--against HEAD` does the same for the last commit, for a CI job that runs after `postinstall` has rewritten the working tree. Only outputs Git tracks are compared, so an output that `gitignore` leaves out never fails. A tracked output whose spec is gone fails until you delete it, since the check also renders the last commit's specs. Specs come from the same state, so an untracked `agnostic-ai.local.yaml` or `.agnostic-ai/local/` is not read.

A hook manager that runs a command only when matching files are staged needs every input in its glob, including a README a review inlines with `@path`. `agnostic-ai explain --inputs` prints them, one per line:

```bash
agnostic-ai explain --inputs
# .agnostic-ai/**
# .gitignore
# agnostic-ai.local.yaml
# agnostic-ai.yaml
# apps/web/README.md
```

Paste the list into the glob, or leave the glob out and run the check on every commit. Rerun it after adding an `@path` line.

## Regenerate on checkout and pull {#regenerate-on-checkout}

Commit hooks do not help when generated outputs are gitignored (`gitignore.enabled: true`): a fresh clone or a new `git worktree` starts with no `CLAUDE.md`, rules, or hooks until someone runs `sync`. A contributor can run `sync` by hand, automated worktree creation cannot, so an AI session opened there finds no config.

A `post-checkout` hook closes the gap. `git checkout`, `git clone`, and `git worktree add` all fire it, so outputs regenerate themselves. A pull that merges runs `post-merge` instead: it restores files Git removes when an incoming commit untracks generated outputs.

`agnostic-ai install-hook --post-checkout` writes both `post-checkout` and `post-merge`. Both run `agnostic-ai sync -q` from the worktree root and exit 0 when the binary or `agnostic-ai.yaml` is not found. Only `post-checkout` checks the branch-checkout flag below. Run the install command again in a clone that already has the older checkout-only hook to add pull coverage. The hooks run a plain `sync`, so a hand edit to a generated file is overwritten; use the recipes below if you need `--keep-edits`.

The recipes use `sync --keep-edits`. Git carries an uncommitted edit across a checkout, and a plain `sync` would overwrite a hand edit to a generated file such as `AGENTS.md`. With `--keep-edits`, sync writes every other output, leaves each file edited since the last sync in place, and names it as `~ kept <path>`. A file with no ledger entry, as in a new linked worktree, counts as edited when it differs from `HEAD`. Move the edit into `.agnostic-ai/`, then run `agnostic-ai sync`.

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

`post-checkout` receives three arguments, and a file checkout passes `0` as the third. Guard on it to limit the hook to branch and worktree switches:

```sh
#!/bin/sh
[ "$3" = "1" ] || exit 0   # 1 = branch checkout, 0 = file checkout
agnostic-ai sync --keep-edits
```

The same sync command works in `post-merge` and in a `postinstall` script. Leave the `$3` checkout guard out of `post-merge`, which receives no checkout flag. A pull with rebase needs a `post-rewrite` hook instead; see [Node monorepos](#node-monorepos).

This needs `agnostic-ai` on `PATH` in every environment that checks out the repo. If some contributors lack the CLI, commit the generated outputs instead of gitignoring them. A Node project can install the CLI with its dependencies instead: see [Node monorepos](#node-monorepos).

## Node monorepos {#node-monorepos}

In a pnpm workspace with ignored outputs, `pnpm install` can create every tool file, and git hooks can regenerate them on each branch switch. npm and Yarn workspaces work the same way with their own commands.

### Pin the CLI

Add the npm package to the workspace root as an exact dev dependency:

```bash
pnpm add -D -E -w agnostic-ai
```

Everyone then runs the same release. Generated files you commit can differ between releases, so also set `requires: "X.Y.Z"` in `agnostic-ai.yaml`. A newer global install or a stale `node_modules` then stops with [AAI-005](@/docs/errors.md#aai-005-installed-version-outside-requires) before it rewrites those files. [`requires`](@/docs/configuration.md#requires) accepts ranges too.

### Sync on install

Root `package.json`:

```json
{
  "scripts": {
    "postinstall": "agnostic-ai sync --keep-edits --quiet"
  }
}
```

Package scripts find the pinned binary in `node_modules/.bin`. Cloud agents and IDE worktree setups, such as Cursor Cloud, Codex environments, and `.cursor/worktrees.json`, run `pnpm install` but no git hook, so this step gives them their tool files.

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

`--quiet` hides the routine summary, but a `~ kept <path>` line still prints, on stderr, so a hook running these recipes still reports a file it left alone.

### First pull

A developer on a commit from before agnostic-ai has no checkout hooks yet. The pull that adds it removes the outputs that commit tracked, and no hook regenerates them. Lefthook reads `lefthook.yml` from the working tree, so a pre-commit step in the new config runs on that developer's next commit. It installs when the CLI in `node_modules` is not the pinned one, and the install runs the `postinstall` sync and adds the checkout hooks:

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

A new linked worktree has no `node_modules`. A hook runner installed from npm, such as lefthook, may not start there, and the pinned binary is missing either way. The worktree gets its tool files once something runs `pnpm install` in it.

Put the install in an environment spec, `.agnostic-ai/environments/dev.yaml`:

```yaml
name: dev
setup: pnpm install --frozen-lockfile
```

Codex and Cursor run `setup` in a new worktree from their own files. For Claude Code, sync writes hooks that run it once in each new worktree: a `--worktree` or Desktop session, a subagent worktree, or a worktree Claude enters during a session. Delete a hand-written `bootstrap.yaml` hook from an earlier version of this recipe, since Claude Code runs matching hooks in parallel. See [Claude Code worktree setup](@/docs/spec-format/environments.md#claude-code-worktree-setup).

Claude Code copies the gitignored files `.worktreeinclude` lists into each new worktree, and sync keeps its managed block there when `claude` is a target (see [gitignore](@/docs/configuration.md#gitignore)). The ignored `.claude/settings.json` and the setup script then reach the worktree. Without that, commit both, so a new worktree has them before any sync runs. In `agnostic-ai.yaml`:

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

- `allowBuilds` lets lefthook's install script add the git hooks. pnpm 11 fails the install when a dependency's build script is not approved. pnpm 10 names the setting `onlyBuiltDependencies`.
- `minimumReleaseAge` needs both exclusions. The binary ships in separate `@agnostic-ai/<os>-<arch>` packages, and each one fails the cooldown on its own.

### Formatters

Keep ignore patterns in a fenced code block. Prettier escapes `*` in plain Markdown text, so `/res/*` becomes `/res/\*`, but it leaves code blocks alone. See [ignore specs](@/docs/spec-format/ignore.md).

Keep formatters away from the generated files you commit, or each sync undoes their edits. Prettier already skips the files `.gitignore` lists. `.prettierignore`:

```text
.claude/settings.json
```

## Tips

- The hook needs `agnostic-ai` on `PATH`. Document the install in `CONTRIBUTING.md` so a new contributor does not hit `command not found` on their first commit.
- To recover from drift, run `agnostic-ai sync` and stage the regenerated outputs with the spec change.
- Set `gitignore.enabled: true` in `agnostic-ai.yaml` to keep generated outputs out of git. The `install-hook` hook then checks that the staged specs render, since `--against index` compares only outputs Git tracks; a plain `sync --check` still compares every output.
- Skip a hook for one commit with `git commit --no-verify`. Keep that for emergencies.

+++
title = "Git hooks"
description = "Check for out-of-date generated files at commit time, and regenerate them after checkout or pull."
weight = 60

[extra]
group = "Workflows"
+++

# Git hooks

Catch out-of-date generated files at commit time, before CI. Each recipe runs `agnostic-ai sync --check` when a spec or `agnostic-ai.yaml` is staged. It blocks the commit if a generated file is out of date.

It is the same check as the [CI gate](@/docs/ci.md). It compares the whole working tree, not only staged files.

If outputs are ignored, run `sync` once in each checkout before you enable the hook. In CI, use the [ignored-output recipe](@/docs/ci.md#ignored-outputs).

## Why a pre-commit hook

- You see the problem when you cause it, not in the next CI run.
- Every contributor gets the same check.
- Enable it for each checkout. A fresh clone works without this setup.

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

`files:` runs the hook only when a spec changes. `pass_filenames: false` checks the whole project, like CI.

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

`glob:` runs the hook only when a spec or the config is staged.

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

The trailing `--` swallows the file paths lint-staged adds. `sync --check` checks the whole project, not single files.

## Check the staged files {#check-staged-files}

A plain `sync --check` reads the working tree. If `sync` rewrote `AGENTS.md` and you did not stage it, the check passes and you commit a stale file. Add `--against index` to compare the staged specs with the staged outputs:

```bash
agnostic-ai sync --check --against index
```

A staged spec change without its regenerated output fails. The error names the file and tells you to run `agnostic-ai sync` and stage the output with `git add` (or, for `--against HEAD`, commit it). The hook from `agnostic-ai install-hook` runs this form. Run `install-hook` again to update a hook an older version wrote.

{% <details summary="Limits of --against"> %}
- `--against` needs agnostic-ai 0.74.0 or later. A `--shared` hook blocks commits for a teammate on an older release until they upgrade.
- The check copies the tracked files to a temporary folder on each commit. In a large repository, use the lint-staged recipe above so it runs only when specs are staged.
- `--against HEAD` checks the last commit instead. Use it in a CI job that runs after `postinstall` has rewritten the working tree.
- Only outputs Git tracks are compared, so an ignored output never fails.
- A tracked output whose spec is gone fails until you delete it.
- An untracked `agnostic-ai.local.yaml` or `.agnostic-ai/local/` is not read.
{% </details> %}

If your hook manager runs a command only when matching files are staged, the glob must list every input, including a README that a review pulls in with `@path`. `agnostic-ai explain --inputs` prints them, one per line:

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

A `post-checkout` hook fixes this. `git checkout`, `git clone`, and `git worktree add` all run it. A pull that merges runs `post-merge` instead, which restores generated files that an incoming commit untracks.

`agnostic-ai install-hook --post-checkout` writes both hooks. Both prefer the local binary and run `agnostic-ai project` from the worktree root, preserving manual edits without installing dependencies. They skip a checkout without `agnostic-ai.yaml`; a missing binary produces a recovery command. In a clone with older generated hooks, run the command again to update their checks and add pull coverage.

The recipes use `sync --keep-edits`. It writes every other output and leaves each file you edited since the last sync alone, printing `~ kept <path>`. In a new linked worktree, a file counts as edited when it differs from `HEAD`. Move the edit into `.agnostic-ai/`, then run `agnostic-ai sync`.

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

`post-checkout` receives three arguments, and a file checkout passes `0` as the third. Check it to run only on branch and worktree switches:

```sh
#!/bin/sh
[ "$3" = "1" ] || exit 0   # 1 = branch checkout, 0 = file checkout
agnostic-ai sync --keep-edits
```

The same command works in `post-merge` and in a `postinstall` script. Leave out the `$3` check in `post-merge`, which gets no checkout flag. A pull with rebase needs a `post-rewrite` hook (see [Node monorepos](#node-monorepos)).

`agnostic-ai` must be on `PATH` wherever the repo is checked out. If some contributors lack the CLI, commit the generated outputs instead of ignoring them. A Node project can install the CLI with its dependencies instead (see [Node monorepos](#node-monorepos)).

## Node monorepos {#node-monorepos}

In a pnpm workspace with ignored outputs, `pnpm install` creates every tool file, and git hooks regenerate them on each branch switch. npm and Yarn workspaces work the same way.

### Pin the CLI

Add the npm package to the workspace root as an exact dev dependency:

```bash
pnpm add -D -E -w agnostic-ai
```

Everyone then runs the same release. Generated files can differ between releases, so also set `requires: "X.Y.Z"` in `agnostic-ai.yaml`. A newer global install or an out-of-date `node_modules` then stops with [AAI-005](@/docs/errors.md#aai-005-installed-version-outside-requires) before it rewrites those files. [`requires`](@/docs/configuration.md#requires) accepts ranges too.

### Sync on install

Root `package.json`:

```json
{
  "scripts": {
    "postinstall": "agnostic-ai project"
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

`post-merge` runs after `git pull`, and `post-rewrite` after a rebase or `git commit --amend`. Git hooks do not put `node_modules/.bin` on `PATH`, so these commands name the installed binary by path. A missing helper requires an explicit package install or a global `agnostic-ai project --bootstrap` using a version that supports the command.

`--quiet` hides the routine summary. A `~ kept <path>` line still prints for each file the hook left alone.

### First pull

When a branch changes the pinned version, check mode reports the installed and required versions without installing anything. Use the supported helper instead of comparing manifest text in shell:

```bash
agnostic-ai project --check
agnostic-ai project --bootstrap
```

Bootstrap uses one locked npm or pnpm install only when the local binary is missing or does not satisfy an exact stable `package.json` pin or `requires`. Conflicting exact declarations stop before installation and must be updated explicitly. Normal package scripts run; the helper prevents a recursive postinstall call from starting a second install. It then syncs with `--keep-edits --quiet`. Repeating bootstrap with the correct binary does not install again.

Only stable exact npm versions are checked against the installed version. Ranges, tags, aliases, prereleases, and file dependencies are not interpreted as npm version contracts; set `requires` to enforce the accepted releases. `--against index` and `--against HEAD` read both the config and package declarations from that Git view while using the installed binary in the working checkout.

Generated hooks first check that the selected binary supports `project`. Released v0.82.0 does not support it. An unsupported selected package produces an upgrade instruction; update its declared version and `requires` explicitly before using the new lifecycle. Built-in memory hooks look for the nearest ancestor config before selecting a binary, including projects nested inside another Git checkout.

A hook manager can call `agnostic-ai project --check --against index` for a read-only pre-commit gate. Use `agnostic-ai project` after checkout or merge to regenerate without installing. The helper prefers the local package even when an older global binary is on `PATH`; call the local helper or use a global version supporting this command. A missing helper must first be installed through your package manager. The command does not rewrite hook-manager configuration or widen an exact requirement.

### New worktrees

A new linked worktree has no `node_modules`. A hook runner installed from npm, such as lefthook, may not start there, and the pinned binary is missing either way. The worktree gets its tool files once `pnpm install` runs in it.

Put the install in an environment spec, `.agnostic-ai/environments/dev.yaml`:

```yaml
name: dev
setup: agnostic-ai project --bootstrap
```

This recipe needs an agnostic-ai version supporting `project` installed globally before worktree setup. Codex and Cursor run `setup` in a new worktree from their own files. For Claude Code, sync writes hooks that run it once in each new worktree. Delete any hand-written `bootstrap.yaml` hook from an earlier version of this recipe, because Claude Code runs matching hooks in parallel. See [Claude Code worktree setup](@/docs/spec-format/environments.md#claude-code-worktree-setup).

Claude Code copies the gitignored files that `.worktreeinclude` lists into each new worktree. When `claude` is a target, sync keeps its block there (see [gitignore](@/docs/configuration.md#gitignore)), so the ignored `.claude/settings.json` and the setup script reach the worktree. Otherwise, commit both, so a new worktree has them before any sync runs. In `agnostic-ai.yaml`:

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
- `minimumReleaseAge` needs both exclusions. The binary ships in separate `@agnostic-ai/<os>-<arch>` packages, and each one is held back by the cooldown on its own.

### Formatters

Put ignore patterns in a fenced code block. Prettier escapes `*` in plain Markdown text (`/res/*` becomes `/res/\*`) but leaves code blocks alone. See [ignore specs](@/docs/spec-format/ignore.md).

Keep formatters away from committed generated files, or each sync undoes their edits. Prettier already skips files that `.gitignore` lists. `.prettierignore`:

```text
.claude/settings.json
```

## Tips

- Project hooks prefer `node_modules/.bin/agnostic-ai`; a global fallback needs `agnostic-ai` on `PATH`. Document the install in `CONTRIBUTING.md` so a new contributor does not hit `command not found` on their first commit.
- To fix a failing check, run `agnostic-ai sync` and stage the regenerated outputs with the spec change.
- With `gitignore.enabled: true`, the `install-hook` hook only checks that the staged specs render, because `--against index` compares only outputs Git tracks. A plain `sync --check` still compares every output.
- Skip a hook for one commit with `git commit --no-verify`. Save it for emergencies.

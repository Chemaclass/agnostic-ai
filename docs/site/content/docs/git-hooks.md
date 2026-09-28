+++
title = "Git hooks"
description = "Run sync checks around commits and checkouts with pre-commit, lefthook, or husky."
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

## Regenerate on checkout

Commit hooks do not help when generated outputs are gitignored (`gitignore.enabled: true`): a fresh clone or a new `git worktree` starts with no `CLAUDE.md`, rules, or hooks until someone runs `sync`. A contributor can run `sync` by hand, automated worktree creation cannot, so an AI session opened there finds no config.

A `post-checkout` hook closes the gap. `git checkout`, `git clone`, and `git worktree add` all fire it, so outputs regenerate themselves.

lefthook (`lefthook.yml`):

```yaml
post-checkout:
  commands:
    sync:
      run: agnostic-ai sync
```

Plain git (`.git/hooks/post-checkout`, `chmod +x`):

```sh
#!/bin/sh
agnostic-ai sync
```

`post-checkout` receives three arguments, and a file checkout passes `0` as the third. Guard on it to limit the hook to branch and worktree switches:

```sh
#!/bin/sh
[ "$3" = "1" ] || exit 0   # 1 = branch checkout, 0 = file checkout
agnostic-ai sync
```

This needs `agnostic-ai` on `PATH` in every environment that checks out the repo. If some contributors lack the CLI, commit the generated outputs instead of gitignoring them. A Node project can install the CLI with its dependencies instead: see [Node monorepos](#node-monorepos).

## Node monorepos {#node-monorepos}

In a pnpm workspace with ignored outputs, `pnpm install` can create every tool file, and git hooks can regenerate them on each branch switch. npm and Yarn workspaces work the same way with their own commands.

### Pin the CLI

Add the npm package to the workspace root as an exact dev dependency:

```bash
pnpm add -D -E -w agnostic-ai
```

Everyone then runs the same release. Generated files you commit can differ between releases, and `requires: ">=X.Y.Z"` in `agnostic-ai.yaml` only stops older binaries.

### Sync on install

Root `package.json`:

```json
{
  "scripts": {
    "postinstall": "agnostic-ai sync --quiet"
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
      run: '[ ! -x node_modules/.bin/agnostic-ai ] || node_modules/.bin/agnostic-ai sync --quiet'
post-merge:
  commands:
    agnostic-ai-sync:
      run: '[ ! -x node_modules/.bin/agnostic-ai ] || node_modules/.bin/agnostic-ai sync --quiet'
post-rewrite:
  commands:
    agnostic-ai-sync:
      run: '[ ! -x node_modules/.bin/agnostic-ai ] || node_modules/.bin/agnostic-ai sync --quiet'
```

`post-merge` runs after `git pull`, and `post-rewrite` after a rebase or `git commit --amend`. Git hooks do not put `node_modules/.bin` on `PATH`, so the command names the binary by path. The guard skips a checkout that has not installed dependencies yet.

### New worktrees

A new linked worktree has no `node_modules`. A hook runner installed from npm, such as lefthook, may not start there, and the pinned binary is missing either way. The worktree gets its tool files once something runs `pnpm install` in it.

Claude Desktop skips `WorktreeCreate` and bootstraps a worktree only through the `SessionStart` hook in that worktree's own `.claude/settings.json`. Write that hook as a spec, `.agnostic-ai/hooks/bootstrap.yaml`:

```yaml
name: bootstrap
description: Install dependencies in a new worktree, which runs sync.
target: claude
event: SessionStart
matcher: startup
command: 'cd "$CLAUDE_PROJECT_DIR" && { test -d node_modules || pnpm install --frozen-lockfile; }'
```

Then keep `.claude/settings.json` committed, so a new worktree has it before any sync runs. In `agnostic-ai.yaml`:

```yaml
gitignore:
  enabled: true
  allow:
    - /.claude/settings.json
```

Commit the file after `sync` writes it. A fresh install writes the same bytes, so `git status` stays clean.

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

Keep ignore patterns in a fenced code block. Prettier escapes `*` in plain Markdown text, so `/res/*` becomes `/res/\*`, but it leaves code blocks alone. See [ignore specs](@/docs/spec-format.md#ignore).

Keep formatters away from the generated files you commit, or each sync undoes their edits. Prettier already skips the files `.gitignore` lists. `.prettierignore`:

```text
.claude/settings.json
```

## Tips

- The hook needs `agnostic-ai` on `PATH`. Document the install in `CONTRIBUTING.md` so a new contributor does not hit `command not found` on their first commit.
- To recover from drift, run `agnostic-ai sync` and stage the regenerated outputs with the spec change.
- Set `gitignore.enabled: true` in `agnostic-ai.yaml` to keep generated outputs out of git. The hook still catches drift because `sync --check` ignores `gitignore`.
- Skip a hook for one commit with `git commit --no-verify`. Keep that for emergencies.

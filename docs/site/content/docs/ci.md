+++
title = "CI"
description = "Fail CI when generated files no longer match your specs."
weight = 50

[extra]
group = "Workflows"
+++

# CI

Pick the check that matches your Git strategy. Run it from the project root, after you install the CLI.

## Committed outputs

Fail when the committed tool files no longer match the specs:

```bash
agnostic-ai sync --check
```

`--check` compares the files sync would write with the files on disk and writes nothing. A missing or changed file exits non-zero. To fix it, run `agnostic-ai sync` locally and commit the result.

Never run `sync` right before this check in CI. It would hide the problem you are testing for.

If an earlier step already rewrote the files, as a `postinstall` sync does, check against the commit instead:

```bash
agnostic-ai sync --check --against HEAD
```

It builds the files from the committed specs and compares them with the committed files. Ignored files are skipped.

`actions/checkout` fetches one commit by default. Set `fetch-depth: 2` so the check has the parent commit (in a pull request's merge commit, the base branch). Without it, a note says the comparison was skipped.

A committed file that no spec produces anymore also fails, so this command catches leftovers without `doctor`. That covers a file with the generated header where a tool writes, and any file the parent commit's specs produced, such as a JSON file with no header.

After a project moves its specs into `.agnostic-ai/`, a branch from before the move can still add a skill in the old place, such as `.cursor/skills/<name>/SKILL.md`. Only Cursor reads it there. The same check fails on it and names the fix: `agnostic-ai import cursor`, then `git rm --cached` the file.

## Ignored outputs

A fresh checkout has no generated files. Check the specs and confirm that sync works:

```bash
agnostic-ai validate
agnostic-ai sync
```

Add `agnostic-ai lint` to check spec quality. `sync --check` after `sync` adds nothing, because the files were just written.

In a Node workspace that pins the CLI and syncs on `postinstall`, `pnpm install --frozen-lockfile` already runs `sync`. See [Node monorepos](@/docs/git-hooks.md#node-monorepos).

This repository ignores generated tool files and runs `lint` in CI. See [contributor checks](https://github.com/Chemaclass/agnostic-ai/blob/main/docs/internal/contributing.md#choose-checks-for-your-change).

## Install the CLI in CI

In Node projects, install the CLI with the project's dependencies. Other projects use the install script.

### Node projects

Pin the npm package as a dev dependency and sync on install:

```bash
npm install -D -E agnostic-ai    # or pnpm add -D -E, yarn add -D -E, bun add -D -E
```

```json
{
  "scripts": {
    "postinstall": "agnostic-ai sync -q"
  }
}
```

The lockfile covers every OS and CPU, so one pin works on macOS, Linux, and Windows runners. For workspaces and git hooks, see [Node monorepos](@/docs/git-hooks.md#node-monorepos).

With ignored generated files, `npm ci` already runs `postinstall`, so the job only checks the specs:

```yaml
steps:
  - uses: actions/checkout@v4
  - uses: actions/setup-node@v4
    with:
      node-version: 22
  - run: npm ci
  - run: npx agnostic-ai lint --strict && npx agnostic-ai validate
```

With committed generated files, skip `postinstall` so it cannot rewrite the files before the check:

```yaml
steps:
  - uses: actions/checkout@v4
  - uses: actions/setup-node@v4
    with:
      node-version: 22
  - run: npm ci --ignore-scripts
  - run: npx agnostic-ai sync --check --diff
```

### Other projects

`scripts/install.sh` installs the release binary on Linux and macOS. Pin the version so local and CI behavior match:

```yaml
steps:
  - uses: actions/checkout@v4
  - name: Install agnostic-ai
    run: |
      export AGNOSTIC_AI_INSTALL_DIR="$HOME/.local/bin"
      curl -fsSL https://raw.githubusercontent.com/Chemaclass/agnostic-ai/main/scripts/install.sh | bash
      echo "$AGNOSTIC_AI_INSTALL_DIR" >> "$GITHUB_PATH"
    env:
      AGNOSTIC_AI_VERSION: vX.Y.Z
  - run: agnostic-ai sync --check --diff
```

`AGNOSTIC_AI_VERSION` takes a release tag. Without it, the script installs the latest release. It checks the archive against the release checksum. Set `AGNOSTIC_AI_VERIFY_ATTESTATION: 1` to also check the [where the archive was built](@/docs/verify-a-release.md#build-provenance). This needs the GitHub CLI, which GitHub-hosted runners include. On Windows runners, use `install.ps1`: see [Installation](@/docs/installation.md#pin-a-version-or-directory).

## Diagnose drift

Run `agnostic-ai sync --check --diff` to see the changes. The [CLI reference](@/docs/cli-reference/sync.md#reading-a-failing---check) explains the output and the failure types.

## Gate model and CLI changes

`sync --check` proves the generated files match your specs. It does not show whether a model or CLI still gives good results for your project.

Configure a project-owned verifier:

```yaml
verify:
  command: [./scripts/verify-harness]
```

Add the gate after installing the AI CLI it needs:

```yaml
- name: Verify Codex harness behavior
  run: agnostic-ai verify --target codex
```

`verify` first checks that generated files are current, then sends JSON to the script on stdin. The script runs the checks and scores them. Its stdout, stderr, and non-zero exit code reach CI unchanged. See the [`verify` command](@/docs/cli-reference/check.md#verify) for the JSON format.

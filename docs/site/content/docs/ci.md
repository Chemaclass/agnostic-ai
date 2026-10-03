+++
title = "CI"
description = "Catch drift between specs and generated output in CI."
weight = 50

[extra]
group = "Workflows"
+++

# CI

Pick the check that matches your Git strategy. Run it from the project root, after you install the CLI.

## Committed outputs

Fail when the checked-in tool files no longer match the specs:

```bash
agnostic-ai sync --check
```

`--check` compares the planned output with the files on disk and writes nothing. Missing or changed output exits non-zero. To fix drift, run `agnostic-ai sync` locally and commit the result.

Never run `sync` right before this check in CI. It erases the drift you are testing for.

If an earlier step already rewrote the files, as a `postinstall` sync does, compare the commit instead:

```bash
agnostic-ai sync --check --against HEAD
```

It renders the specs as committed and compares them with the committed outputs. Outputs that `gitignore` leaves out are skipped.

`actions/checkout` fetches one commit by default. Set `fetch-depth: 2` so the check has the parent commit (in a pull request's merge commit, the base branch). Without it, a note says the comparison was skipped.

A committed output that no spec produces anymore also fails, so this command catches leftovers without `doctor`. That covers a file with the generated header where a target writes, and any file the parent commit's specs rendered, such as a JSON file with no header.

After a project moves its specs into `.agnostic-ai/`, a branch from before the move can still add a skill in the old place, such as `.cursor/skills/<name>/SKILL.md`. Git keeps tracking it inside the ignored folder, where only Cursor reads it. The same check fails on it and names the fix: `agnostic-ai import cursor`, then `git rm --cached` the file.

## Ignored outputs

A fresh checkout has no generated files. Validate the source and confirm that generation succeeds:

```bash
agnostic-ai validate
agnostic-ai sync
```

Add `agnostic-ai lint` for source quality. A `sync --check` afterwards confirms the generated output is consistent. It says nothing about committed files.

In a Node workspace that pins the CLI and syncs on `postinstall`, `pnpm install --frozen-lockfile` already runs `sync`. See [Node monorepos](@/docs/git-hooks.md#node-monorepos).

This repository ignores generated tool files and runs spec lint in CI. See [contributor checks](https://github.com/Chemaclass/agnostic-ai/blob/main/docs/internal/contributing.md#choose-checks-for-your-change).

## Install the CLI in CI

Install the CLI with the project's dependencies. Other projects use the install script.

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

The lockfile carries the platform packages for every OS and CPU, so one pin works on macOS, Linux, and Windows runners. For workspaces and git hooks, see [Node monorepos](@/docs/git-hooks.md#node-monorepos).

With ignored outputs, `npm ci` already runs `postinstall`, so the job only checks the source:

```yaml
steps:
  - uses: actions/checkout@v4
  - uses: actions/setup-node@v4
    with:
      node-version: 22
  - run: npm ci
  - run: npx agnostic-ai lint --strict && npx agnostic-ai validate
```

With committed outputs, skip `postinstall` so it cannot rewrite the files before the check:

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

`AGNOSTIC_AI_VERSION` takes a release tag. Without it, the script installs the latest release. It checks the archive against the release checksum. Set `AGNOSTIC_AI_VERIFY_ATTESTATION: 1` to also check the [build provenance](@/docs/verify-a-release.md#build-provenance). This needs the GitHub CLI, which GitHub-hosted runners include. On Windows runners, use `install.ps1`: see [Installation](@/docs/installation.md#pin-a-version-or-directory).

## Diagnose drift

Run `agnostic-ai sync --check --diff` to see the changes. The [CLI reference](@/docs/cli-reference/sync.md#reading-a-failing---check) explains the output formats and failure categories.

## Gate model and CLI changes

`sync --check` proves the generated files match their specs. It does not show whether a model or CLI still gives good results for your project.

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

`verify` checks drift first, fingerprints the harness, and detects the CLI identity when it can. Then it sends versioned JSON to the script on stdin. The script runs the checks and scores them. Its stdout, stderr, and non-zero exit code reach CI unchanged. See the [`verify` command](@/docs/cli-reference/check.md#verify) for the JSON contract.

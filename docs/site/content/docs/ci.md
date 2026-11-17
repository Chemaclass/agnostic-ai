+++
title = "CI"
description = "Fail CI when generated files no longer match your specs."
weight = 60

[extra]
group = "Workflows"
+++

# CI

Fail CI when generated files drift from your specs. Pick the recipe for committed or ignored outputs, [install the CLI](#install-the-cli-in-ci), and run from the project root.

## Committed outputs

Fail when the committed tool files no longer match the specs:

```bash
agnostic-ai sync --check
```

`--check` writes nothing and exits non-zero if a file is missing or differs. Fix it by running `agnostic-ai sync` locally and committing. Never run `sync` before this check in CI; it hides the drift.

On Windows runners, CRLF checkouts show as `line endings only`. Add a `.gitattributes` rule such as `* text eol=lf`, or use `--against HEAD`.

<details>
<summary>Check committed files after another step rewrites the working tree</summary>

If an earlier step (such as a `postinstall` sync) rewrote the files, check against the commit:

```bash
agnostic-ai sync --check --against HEAD
```

- It builds files from the committed specs and compares them with the committed files. A committed file no spec produces also fails. See [what `--against` compares](@/docs/cli-reference/sync.md#against-details).
- `actions/checkout` fetches one commit by default. Set `fetch-depth: 2`, or a note says the comparison was skipped.

</details>

## Ignored outputs

A fresh checkout has no generated files. Check the specs and confirm sync works:

```bash
agnostic-ai validate
agnostic-ai sync
```

- Add `agnostic-ai lint` for spec quality. Skip `sync --check` here; the files were just written.
- A Node workspace that syncs on `postinstall` already syncs in `pnpm install --frozen-lockfile` (see [Node monorepos](@/docs/git-hooks.md#node-monorepos)).
- [Project readiness checks](@/docs/cli-reference/project.md#project-readiness) stop a cached environment from starting with missing or stale config. They check every configured tool without approving local hooks.

<details>
<summary>Install a pinned CLI in your CI job</summary>

## Install the CLI in CI

### Node projects

Pin the npm package as a dev dependency and sync on `postinstall`, as [Node monorepos](@/docs/git-hooks.md#node-monorepos) shows. With ignored generated files, `npm ci` already runs `postinstall`, so the job only checks the specs:

```yaml
steps:
  - uses: actions/checkout@v4
  - uses: actions/setup-node@v4
    with:
      node-version: 26
  - run: npm ci
  - run: npx agnostic-ai lint --strict && npx agnostic-ai validate
```

With committed generated files, skip `postinstall` so it cannot rewrite the files before the check:

```yaml
steps:
  - uses: actions/checkout@v4
  - uses: actions/setup-node@v4
    with:
      node-version: 26
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

`AGNOSTIC_AI_VERSION` takes a release tag; without it, the script installs the latest release. The script checks the archive against the release checksum, and checks [where the archive was built](@/docs/verify-a-release.md#build-provenance) when the GitHub CLI is logged in. GitHub-hosted runners include the GitHub CLI; set `{% raw %}GH_TOKEN: ${{ github.token }}{% endraw %}` to log it in, and `AGNOSTIC_AI_VERIFY_ATTESTATION: 1` to fail when the check cannot run. On Windows runners, use `install.ps1`: see [Installation](@/docs/installation.md#pin-a-version-or-directory).

</details>

## Diagnose drift

Run `agnostic-ai sync --check --diff` to see how files differ from your specs. The [CLI reference](@/docs/cli-reference/sync.md#reading-a-failing---check) explains the output.

<details>
<summary>Check the coding tool against tasks from your project</summary>

## Gate model and CLI changes

`sync --check` does not show whether a model or CLI still works well for your project. For that, configure a script that runs the AI tool against your tasks:

```yaml
verify:
  command: [./scripts/verify-harness]
```

Add the gate after installing the AI CLI it needs:

```yaml
- name: Verify Codex project behavior
  run: agnostic-ai verify --target codex
```

`verify` checks the generated files are current, then sends JSON to the script on stdin. The script runs the tasks and decides pass or fail; its stdout, stderr, and exit code reach CI unchanged. See [`verify`](@/docs/cli-reference/check.md#verify) for the JSON format.

</details>

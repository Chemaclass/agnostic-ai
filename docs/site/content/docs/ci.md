+++
title = "CI"
description = "Fail CI when generated files no longer match your specs."
weight = 60

[extra]
group = "Workflows"
+++

# CI

CI (continuous integration) checks on each change that the generated files match your specs. Pick the recipe for how your team stores them, committed in Git or ignored and generated during setup, and run it from the project root after [installing the CLI](#install-the-cli-in-ci).

## Committed outputs

Fail when the committed tool files no longer match the specs:

```bash
agnostic-ai sync --check
```

`--check` compares the files sync would write with the files on disk, writes nothing, and exits non-zero on a missing or changed file. To fix it, run `agnostic-ai sync` locally and commit the result. Never run `sync` right before this check in CI, because it hides the problem you are testing for.

<details>
<summary>Check committed files after another step rewrites the working tree</summary>

If an earlier step already rewrote the files, as a `postinstall` sync does, check against the commit instead:

```bash
agnostic-ai sync --check --against HEAD
```

It builds the files from the committed specs and compares them with the committed files. A committed file that no spec produces anymore also fails, which catches leftovers without `doctor`. See [what `--against` compares](@/docs/cli-reference/sync.md#against-details).

`actions/checkout` fetches one commit by default. Set `fetch-depth: 2` so the check has the parent commit (in a pull request's merge commit, the base branch). Without it, a note says the comparison was skipped.

</details>

## Ignored outputs

A fresh checkout has no generated files. Check the specs and confirm that sync works:

```bash
agnostic-ai validate
agnostic-ai sync
```

Add `agnostic-ai lint` to check spec quality. Do not add `sync --check` after `sync`: the files were just written. In a Node workspace that pins the CLI and syncs on `postinstall`, `pnpm install --frozen-lockfile` already runs `sync` (see [Node monorepos](@/docs/git-hooks.md#node-monorepos)).

For a cached environment that must stop before startup when project configuration is missing or stale, use the [project readiness checks](@/docs/cli-reference/project.md#project-readiness). They check every configured tool without approving local hooks.

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

`AGNOSTIC_AI_VERSION` takes a release tag; without it, the script installs the latest release. The script checks the archive against the release checksum, and checks [where the archive was built](@/docs/verify-a-release.md#build-provenance) when the GitHub CLI is logged in. GitHub-hosted runners include the GitHub CLI; set `{% raw %}GH_TOKEN: ${{ github.token }}{% endraw %}` to log it in, and `AGNOSTIC_AI_VERIFY_ATTESTATION: 1` to fail when the check cannot run. On Windows runners, use `install.ps1`: see [Installation](@/docs/installation.md#pin-a-version-or-directory).

</details>

## Diagnose drift

Drift means generated files differ from what your current specs produce. Run `agnostic-ai sync --check --diff` to see the changes. The [CLI reference](@/docs/cli-reference/sync.md#reading-a-failing---check) explains the output and the failure types.

<details>
<summary>Check the coding tool against tasks from your project</summary>

## Gate model and CLI changes

`sync --check` proves the generated files match your specs. It does not show whether a model or CLI still gives good results for your project.

To check that, configure a script that runs the AI tool against tasks from your project:

```yaml
verify:
  command: [./scripts/verify-harness]
```

Add the gate after installing the AI CLI it needs:

```yaml
- name: Verify Codex project behavior
  run: agnostic-ai verify --target codex
```

`verify` first checks that generated files are current, then sends JSON to the script on stdin. Your script runs the tasks and decides whether the results pass. Its stdout, stderr, and non-zero exit code reach CI unchanged. See the [`verify` command](@/docs/cli-reference/check.md#verify) for the JSON format.

</details>

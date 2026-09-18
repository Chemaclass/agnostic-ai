+++
title = "CI"
description = "Catch drift between specs and generated output in CI."
weight = 50

[extra]
group = "Workflows"
+++

# CI


Pick the check that matches your Git strategy. Run it from the project root, after installing the CLI.

## Committed outputs

Fail when the checked-in tool files no longer match the specs:

```bash
agnostic-ai sync --check
```

`--check` compares the planned output with the files on disk and writes nothing. Missing or changed output exits non-zero. To fix drift, run `agnostic-ai sync` locally and commit the result.

Never run `sync` right before this check in CI. It erases the drift you are testing for.

## Ignored outputs

A fresh checkout has no generated files. Validate the source and confirm that generation succeeds:

```bash
agnostic-ai validate
agnostic-ai sync
```

Add `agnostic-ai lint` for source quality. A `sync --check` afterwards confirms the generated output is consistent, but proves nothing about committed files.

This repository ignores generated tool files and runs spec lint in CI. See [contributor checks](https://github.com/Chemaclass/agnostic-ai/blob/main/docs/internal/contributing.md#choose-checks-for-your-change).

## GitHub Action

The [agnostic-ai action](https://github.com/chemaclass/agnostic-ai-action) installs the binary and runs a command. After your checkout step, use this for committed outputs:

```yaml
- uses: chemaclass/agnostic-ai-action@v1
  with:
    command: check
```

For ignored outputs, use `command: sync` instead. Set the action's `version` input to a released CLI version so local and CI behavior match. The action's README lists its current inputs and install behavior.

## Diagnose drift

Run `agnostic-ai sync --check --diff` to see the changes. The [CLI reference](@/docs/cli-reference.md#reading-a-failing---check) explains the output formats and failure categories.

## Gate model and CLI changes

`sync --check` proves the generated files match their specs. It proves nothing about whether a model or CLI still produces good results for your project.

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

agnostic-ai checks drift first, fingerprints the harness, detects the CLI identity when available, then sends versioned JSON to the script on stdin. The script owns execution and scoring. Its stdout, stderr, and non-zero exit code reach CI unchanged. See the [`verify` command](@/docs/cli-reference.md#verify) for the JSON contract.

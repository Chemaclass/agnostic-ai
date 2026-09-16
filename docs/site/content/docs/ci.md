+++
title = "CI"
description = "Detect generated-output drift and keep project configuration consistent in automation."
weight = 50

[extra]
group = "Workflows"
+++

# CI


Choose the check based on whether generated outputs are committed. Run it from the project root after installing the CLI.

## Committed outputs

Fail when the checked-in tool files no longer match the specs:

```bash
agnostic-ai sync --check
```

The command compares planned output with files on disk without writing. Missing or changed output produces a non-zero exit code. To fix drift, run `agnostic-ai sync` locally and commit the updated outputs.

Do not sync immediately before this check in CI: that would replace the evidence of drift.

## Ignored outputs

A fresh checkout has no generated files. Validate the source and confirm that generation succeeds:

```bash
agnostic-ai validate
agnostic-ai sync
```

Use `agnostic-ai lint` as an additional source-quality check. A subsequent `sync --check` can verify consistency of generated output, but it does not establish that any committed files were up to date.

This repository ignores generated tool files and runs spec lint in CI. See [contributor checks](https://github.com/Chemaclass/agnostic-ai/blob/main/docs/internal/contributing.md#choose-checks-for-your-change).

## GitHub Action

The [agnostic-ai action](https://github.com/chemaclass/agnostic-ai-action) installs the binary and runs a command. After your checkout step, use this for committed outputs:

```yaml
- uses: chemaclass/agnostic-ai-action@v1
  with:
    command: check
```

For ignored outputs, use `command: sync` instead. Set the action's `version` input to a released CLI version to keep local and CI behavior aligned. See the action's README for its current inputs and installation behavior.

## Diagnose drift

Use `agnostic-ai sync --check --diff` to inspect changes. The [CLI reference](@/docs/cli-reference.md#reading-a-failing---check) explains output formats and failure categories.

## Gate model and CLI changes

`sync --check` proves that generated files match their specs. It does not prove that a model or CLI still produces acceptable results for your project.

Configure a project-owned verifier:

```yaml
verify:
  command: [./scripts/verify-harness]
```

Then add the gate after installing the required AI CLI:

```yaml
- name: Verify Codex harness behavior
  run: agnostic-ai verify --target codex
```

agnostic-ai checks drift first, fingerprints the harness, detects the CLI identity when available, and sends versioned JSON to the script through stdin. The script owns execution and scoring. Its stdout, stderr, and non-zero exit code reach CI unchanged. See the [`verify` command](@/docs/cli-reference.md#verify) for the JSON contract.

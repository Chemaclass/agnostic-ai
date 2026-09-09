# CI

[User docs](README.md) · [Git strategy](getting-started.md#commit-or-ignore-generated-outputs)

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

This repository ignores generated tool files and runs spec lint in CI. See [contributor checks](../internal/contributing.md#choose-checks-for-your-change).

## GitHub Action

The [agnostic-ai action](https://github.com/chemaclass/agnostic-ai-action) installs the binary and runs a command. After your checkout step, use this for committed outputs:

```yaml
- uses: chemaclass/agnostic-ai-action@v1
  with:
    command: check
```

For ignored outputs, use `command: sync` instead. Set the action's `version` input to a released CLI version to keep local and CI behavior aligned. See the action's README for its current inputs and installation behavior.

## Diagnose drift

Use `agnostic-ai sync --check --diff` to inspect changes. The [CLI reference](cli-reference.md#reading-a-failing---check) explains output formats and failure categories.

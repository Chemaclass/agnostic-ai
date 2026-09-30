+++
title = "CLI reference"
description = "Look up every agnostic-ai command, option, and exit behavior."
weight = 140
sort_by = "weight"
template = "docs/hub.html"
page_template = "docs/page.html"

[extra]
group = "Reference"
hub = true
nav_after = "configuration"
+++

# CLI reference

```
agnostic-ai [command] [flags]
```

## Find a command

| Task | Commands |
|---|---|
| Set up a project | [init](@/docs/cli-reference/start.md#init), [import](@/docs/cli-reference/start.md#import), [new](@/docs/cli-reference/start.md#new) |
| Generate or preview output | [sync](@/docs/cli-reference/sync.md#sync), [render](@/docs/cli-reference/inspect.md#render) |
| Check source, output, and behavior | [validate](@/docs/cli-reference/check.md#validate), [lint](@/docs/cli-reference/check.md#lint), [doctor](@/docs/cli-reference/check.md#doctor), [status](@/docs/cli-reference/check.md#status), [verify](@/docs/cli-reference/check.md#verify) |
| Inspect routing | [list](@/docs/cli-reference/start.md#list), [explain](@/docs/cli-reference/inspect.md#explain), [compare](@/docs/cli-reference/inspect.md#compare), [graph](@/docs/cli-reference/inspect.md#graph), [why](@/docs/cli-reference/inspect.md#why) |
| Restore or remove generated files | [revert](@/docs/cli-reference/maintain.md#revert), [cleanup](@/docs/cli-reference/maintain.md#cleanup) |
| Share specs | [packs](@/docs/cli-reference/maintain.md#packs) |
| Write hook commands | [hook paths](@/docs/cli-reference/maintain.md#hook-paths) |
| Set up your environment | [completion](@/docs/cli-reference/maintain.md#completion), [upgrade or update](@/docs/cli-reference/maintain.md#upgrade), [install-hook](@/docs/cli-reference/maintain.md#install-hook), [lsp](@/docs/cli-reference/maintain.md#lsp) |

Walkthroughs: [Getting started](@/docs/getting-started.md), [Migration](@/docs/migration.md). Automation: [exit codes](#exit-codes), [CI guide](@/docs/ci.md).

## Global flags

| Flag | Description |
|------|-------------|
| `-h, --help` | Help for any command, same as `agnostic-ai help <command>`. |
| `--version` | Print version and exit |
| `-q, --quiet` | Errors only, plus the `~ kept` lines of `sync --keep-edits`, on stderr |
| `-v, --verbose` | Increase output verbosity (repeatable). Mutually exclusive with `--quiet`. |
| `--profile <file>` | Write a `runtime/pprof` CPU profile to `<file>` (or set `AGNOSTIC_AI_PROFILE`). Read it with `go tool pprof <file>`. |

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Any error (parse failure, IO error, missing config) |
| verifier exit code | `verify` returns the external verifier's non-zero code unchanged. |

## Environment variables

| Var | Default | Description |
|-----|---------|-------------|
| `AGNOSTIC_AI_HOME` | `~/.agnostic-ai` | Source root for `sync --global`, `list --global`, `lint --global`, and `validate --global`, including their `local/` override layer. Project sync does not load it. See [global configuration](@/docs/configuration.md#global-configuration). |

## Config precedence

Last wins:

1. Built-in defaults (see [configuration](@/docs/configuration.md))
2. `agnostic-ai.yaml`
3. CLI flags (e.g. `-t`)

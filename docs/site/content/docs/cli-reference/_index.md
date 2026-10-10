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
| Set up a project | [use](@/docs/cli-reference/start.md#use), [init](@/docs/cli-reference/start.md#init), [import](@/docs/cli-reference/start.md#import), [new](@/docs/cli-reference/start.md#new) |
| Generate or preview output | [sync](@/docs/cli-reference/sync.md#sync), [render](@/docs/cli-reference/inspect.md#render) |
| Check source, output, and behavior | [validate](@/docs/cli-reference/check.md#validate), [lint](@/docs/cli-reference/check.md#lint), [doctor](@/docs/cli-reference/check.md#doctor), [status](@/docs/cli-reference/check.md#status), [verify](@/docs/cli-reference/check.md#verify) |
| Inspect routing | [list](@/docs/cli-reference/start.md#list), [explain](@/docs/cli-reference/inspect.md#explain), [compare](@/docs/cli-reference/inspect.md#compare), [graph](@/docs/cli-reference/inspect.md#graph), [why](@/docs/cli-reference/inspect.md#why) |
| Restore or remove generated files | [revert](@/docs/cli-reference/maintain.md#revert), [cleanup](@/docs/cli-reference/maintain.md#cleanup) |
| Share specs | [packs](@/docs/cli-reference/maintain.md#packs) |
| Check and repair shared memory | [memory](@/docs/cli-reference/maintain.md#memory) |
| Write and test hook commands | [hook paths](@/docs/cli-reference/maintain.md#hook-paths), [hook run](@/docs/cli-reference/maintain.md#hook-run) |
| Set up your environment | [completion](@/docs/cli-reference/maintain.md#completion), [upgrade or update](@/docs/cli-reference/maintain.md#upgrade), [migrate](@/docs/cli-reference/maintain.md#migrate), [install-hook](@/docs/cli-reference/maintain.md#install-hook), [lsp](@/docs/cli-reference/maintain.md#lsp) |

Walkthroughs: [Getting started](@/docs/getting-started.md), [Migration](@/docs/migration.md). Automation: [exit codes](#exit-codes), [CI guide](@/docs/ci.md).

## Concurrent commands

`sync`, `import`, `use`, `init`, and `migrate` hold one project lock while they run. A second writer exits with the running command's name and process ID. Retry after that command finishes. `sync --watch` holds the lock until it exits.

The lock file is `.agnostic-ai/.command-lock`, which the managed `.gitignore` block ignores. It stays on disk after the command exits; leave it in place. The operating system releases the lock when the process exits or is killed. Writing commands refresh the runtime `.gitignore` entries even when generated outputs are committed.

`sync --check`, `sync --plan`, dry runs, `status`, and `list` do not take the lock. Global commands do not use the project lock. The lock works between processes on one machine, not across a network file system.

## Global flags

| Flag | Description |
|------|-------------|
| `-h, --help` | Help for any command, same as `agnostic-ai help <command>`. |
| `--version` | Print the version and exit |
| `-q, --quiet` | Errors only, plus the `~ kept` lines of `sync --keep-edits`, on stderr |
| `-v, --verbose` | Increase output verbosity (repeatable). Mutually exclusive with `--quiet`. |
| `--profile <file>` | Write a `runtime/pprof` CPU profile to `<file>` (or set `AGNOSTIC_AI_PROFILE`). Read it with `go tool pprof <file>`. |

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Any error (parse failure, I/O error, missing config) |
| verifier exit code | `verify` returns the external verifier's non-zero code unchanged. |

## Environment variables

| Var | Default | Description |
|-----|---------|-------------|
| `AGNOSTIC_AI_HOME` | `~/.agnostic-ai` | Source root for `sync --global`, `list --global`, `lint --global`, `validate --global`, and `migrate --global`, including their `local/` override layer. Project sync does not load it. See [global configuration](@/docs/configuration.md#global-configuration). |

## Which config wins {#config-precedence}

The last one wins:

1. Built-in defaults (see [configuration](@/docs/configuration.md))
2. `agnostic-ai.yaml`
3. CLI flags (e.g. `-t`)

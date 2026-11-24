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

Run project commands from the folder with `agnostic-ai.yaml`, unless the command supports subdirectories or `--scope user`. For the options in your installed release, run `agnostic-ai <command> --help`. Start with [`doctor`](@/docs/cli-reference/doctor.md#doctor) or see [Troubleshooting](@/docs/troubleshooting.md) to diagnose a problem.

## Find a command

For a new project, start here:

```bash
echo "claude,codex" | agnostic-ai init
agnostic-ai sync --plan
agnostic-ai sync
agnostic-ai doctor
```

| Task | Commands |
|---|---|
| Set up a project | [use](@/docs/cli-reference/start.md#use), [init](@/docs/cli-reference/start.md#init), [import](@/docs/cli-reference/import.md#import), [new](@/docs/cli-reference/start.md#new) |
| Generate or preview output | [sync](@/docs/cli-reference/sync.md#sync), [render](@/docs/cli-reference/inspect.md#render) |
| Diagnose problems, then check a specific area | [doctor](@/docs/cli-reference/doctor.md#doctor), [validate](@/docs/cli-reference/check.md#validate), [lint](@/docs/cli-reference/check.md#lint), [status](@/docs/cli-reference/check.md#status), [verify](@/docs/cli-reference/check.md#verify) |
| Find which specs each tool reads | [list](@/docs/cli-reference/start.md#list), [explain](@/docs/cli-reference/inspect.md#explain), [compare](@/docs/cli-reference/inspect.md#compare), [graph](@/docs/cli-reference/inspect.md#graph), [why](@/docs/cli-reference/inspect.md#why) |
| Inspect tool permissions and settings | [ui / view](@/docs/cli-reference/serve.md#view), [serve](@/docs/cli-reference/serve.md#serve) |
| Call the local JSON API | [local API](@/docs/cli-reference/serve-api.md), [conversations API](@/docs/cli-reference/conversations-api.md) |
| See the conversations that need you | [companion](@/docs/cli-reference/companion.md#companion), [companion list](@/docs/cli-reference/companion-list.md) |
| Start a linked conversation in a new tab | [conversations start](@/docs/cli-reference/conversations.md#conversations-start) |
| Let an agent message another session | [mailbox send](@/docs/cli-reference/mailbox.md#mailbox-send), [done](@/docs/cli-reference/mailbox.md#mailbox-done), [list and show](@/docs/cli-reference/mailbox.md#mailbox-list-and-show), [claim and release](@/docs/cli-reference/mailbox.md#mailbox-claim-and-release), [accept and decline](@/docs/cli-reference/mailbox.md#mailbox-accept-and-decline) |
| Preview the Git references for a code review | [mailbox refs](@/docs/cli-reference/mailbox.md#mailbox-refs) |
| Count what the mailbox recorded | [mailbox stats](@/docs/cli-reference/mailbox.md#mailbox-stats) |
| List each MCP server's tools for the overview | [mcp tools](@/docs/cli-reference/inspect.md#mcp-tools) |
| Restore or remove generated files | [revert](@/docs/cli-reference/maintain.md#revert), [cleanup](@/docs/cli-reference/maintain.md#cleanup) |
| Share specs | [packs](@/docs/cli-reference/maintain.md#packs) |
| Check and maintain shared memory | [memory commands](@/docs/cli-reference/memory.md#memory) |
| See or stop automatic answers to permission prompts | [approvals](@/docs/cli-reference/approvals.md#approvals) |
| Write and test hook commands | [hook paths](@/docs/cli-reference/maintain.md#hook-paths), [hook run](@/docs/cli-reference/hook-diagnostics.md#hook-run), [hook worktree-remove](@/docs/cli-reference/maintain.md#hook-worktree-remove) |
| Check or prepare the project CLI | [project](@/docs/cli-reference/project.md#project), [readiness checks](@/docs/cli-reference/project.md#project-readiness) |
| Set up your environment | [completion](@/docs/cli-reference/maintain.md#completion), [upgrade or update](@/docs/cli-reference/upgrades.md#upgrade), [migrate](@/docs/cli-reference/upgrades.md#migrate), [install-hook](@/docs/cli-reference/maintain.md#install-hook), [lsp](@/docs/cli-reference/maintain.md#lsp) |

Walkthroughs: [Getting started](@/docs/getting-started.md), [Migration](@/docs/migration.md). Automation: [exit codes](#exit-codes), [CI guide](@/docs/ci.md).

## Concurrent commands

Commands that write hold one project lock. A second writer exits with the running command's name and process ID; retry after it finishes.

<details>
<summary>Which commands lock, and the lock file</summary>

- `sync` (including `--watch`, until it exits), `import`, `use`, `init`, and `migrate` hold the lock while they run.
- `sync --check`, `sync --plan`, dry runs, `status`, `list`, and user-level commands take no lock.
- The lock works between processes on one machine, not across a network file system.
The lock file, `.agnostic-ai/.command-lock`, stays on disk. Do not delete it. The managed `.gitignore` block ignores it, and writing commands refresh those runtime entries even when generated outputs are committed. The operating system releases the lock when the process exits or is killed.

</details>

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
| 1 | Any error (invalid file format, file read or write failure, missing config) |
| verifier exit code | `verify` returns the external verifier's non-zero code unchanged. |

## Environment variables

| Var | Default | Description |
|-----|---------|-------------|
| `AGNOSTIC_AI_HOME` | `~/.agnostic-ai` | Source folder for the `--scope user` commands. See [user configuration](@/docs/configuration-reference.md#global-configuration). |

<details>
<summary>Which commands read AGNOSTIC_AI_HOME</summary>

`sync --scope user`, `list --scope user`, `lint --scope user`, `validate --scope user`, and `migrate --scope user` read it, including its `local/` override layer. Project sync does not load it.

</details>

## Which config wins {#config-precedence}

See [which value wins](@/docs/configuration.md#precedence) in the configuration reference.

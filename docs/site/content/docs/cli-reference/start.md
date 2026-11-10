+++
title = "Start a project"
description = "Commands that set up a project, create specs, and list what loads."
weight = 10

[extra]
group = "Reference"

[extra.moved]
import = "@/docs/cli-reference/import.md#import"
import-behavior = "@/docs/cli-reference/import.md#import-behavior"
keep-existing-specs = "@/docs/cli-reference/import.md#keep-existing-specs"
bring-tool-edits-back = "@/docs/cli-reference/import.md#bring-tool-edits-back"
files-and-source-order = "@/docs/cli-reference/import.md#files-and-source-order"
filename-prefix-reclassification = "@/docs/cli-reference/import.md#filename-prefix-reclassification"
import-reconcile = "@/docs/cli-reference/import.md#import-reconcile"
+++

# Start a project

## Import {#import .legacy-anchor}

## Import behavior {#import-behavior .legacy-anchor}

## Keep existing specs {#keep-existing-specs .legacy-anchor}

## Bring tool edits back {#bring-tool-edits-back .legacy-anchor}

## Files and source order {#files-and-source-order .legacy-anchor}

## Filename prefix reclassification {#filename-prefix-reclassification .legacy-anchor}

## Import reconcile {#import-reconcile .legacy-anchor}

`use <tool>` adds a tool and generates its files in one step; `init` and [`import`](@/docs/cli-reference/import.md#import) let you review existing instructions first. `new` creates a spec, and `list` shows what loads. Run these commands from the project root. In scripts or agent instructions, name the tools to skip interactive selection.

## use

Add an AI tool using the project's existing specs:

```bash
agnostic-ai use codex                  # switching to Codex this month
agnostic-ai use claude codex cursor    # a team on several tools
```

- With no `agnostic-ai.yaml`, it creates one enabling the detected tools plus the named ones, imports their instructions, skills, agents, hooks, and MCP servers into `.agnostic-ai/`, and asks about the `.gitignore` block, as `init` does.
- In an existing project, it first imports each named tool's own config, such as a hand-written `AGENTS.md`, then adds the tool to `targets`.
- If an import would replace a spec with content that tool never read, `use` writes no spec, lists each one, and names the `agnostic-ai import <tool> --overwrite` command.
- If an import fails, `use` leaves that tool out of `targets` and names the retry command.

It then syncs and shows what each added tool now reads:

```
✓ codex now reads, from .agnostic-ai/:
    instructions     AGENTS.md
    1 skill          .agents/skills/        review
    1 MCP server     .codex/config.toml     github
  edit .agnostic-ai/ and run agnostic-ai sync to change what every tool reads
```

A tool already in `targets` changes nothing. A run that stopped partway finishes on the next try. A mistyped name suggests the closest one. To remove a tool, edit `targets` by hand.

<details>
<summary>When use refuses or stops</summary>

- A tool whose instructions file is in `sync.unmanaged` stops `use`, because importing it would copy that file to every tool. Import it by hand.
- If `use` is interrupted before its imports finish, `sync` stops until `use` runs again.
- It refuses to start a project inside an existing project's directory.
- It refuses to add a tool when `agnostic-ai.local.yaml` sets `targets`, because that list wins. Add the tool there.

</details>

## init

Create `agnostic-ai.yaml`, the managed `.gitignore` block, and `.agnostic-ai/AGNOSTIC_AI.md` with a placeholder note when it does not exist. Errors if `agnostic-ai.yaml` exists. It creates only the source folders `--demo` or `--preset` fill; `new` and `import` create the rest.

```bash
echo "claude,codex" | agnostic-ai init
agnostic-ai sync
```

| Flag | Description |
|------|-------------|
| `[dir]` | Base directory for the source folders (`.` for the old root layout). Writes matching `sources:` paths; the default `.agnostic-ai/` writes none. |
| `--memory` | Enable shared memory. In Git with managed ignores, also write `memory.personal: repo` to a new `agnostic-ai.local.yaml`, never to an existing one. See [repo memory](@/docs/memory.md#one-store-per-repository) for Kilo Code. |
| `--demo` | Add example specs, one per source folder plus the `memory-curator` skill and two [spec guard hooks](@/docs/spec-format/hook-reference.md#spec-guard). The [`no-force-push` hook](@/docs/spec-format/hook-recipes.md#shared-hook-scripts) script goes to `.agnostic-ai/scripts/` whatever the base dir. Never overwrites files. |
| `--preset <name>` | Add starter specs for a stack: `go`, `ts-react`, `python`. Combines with `--demo` and `--all`. Never overwrites files. |
| `-a, --all` | Skip the target picker and enable every supported target. |
| `--gitignore[=on\|off]` | On by default: generated outputs go into a managed `.gitignore` block; `off` commits them. `true` and `false` work too, here and on `sync`. Write the value after `=`: `--gitignore off` stops with a hint, since `off` would read as the folder. |
| `--from <cli>` | Then import existing config from this [import source](@/docs/cli-reference/import.md#import), or `all`. |
| `--dry-run` | List the files it, and with `--from` the import, would write. |

Without `--all`, `init` takes targets from a terminal prompt or a comma-separated list on stdin. Unknown names error and write nothing. The prompt preselects the first nonempty set below. Press Enter to accept it:

1. Tools the project uses, by a marker such as `.claude/`, a root `CLAUDE.md`, `.codex/`, `.gemini/`, `.cursor/`, or `.github/copilot-instructions.md`. A root `AGENTS.md` is not a marker, since most tools read it.
2. Tools whose CLI is on `PATH`, as [`doctor install`](@/docs/cli-reference/doctor.md#doctor) reports.
3. `claude` and `codex`.

With no terminal and nothing piped (CI), or an empty piped line, `init` enables that default and names it. If a root `AGENTS.md` exists and `codex` is not in the set, it suggests enabling it.

In a terminal, when the project has config an importer reads, or a root `AGENTS.md`, plain `init` offers to import it, as `--from all` does. Without a terminal, the summary lists the `agnostic-ai import <tool>` commands instead. `--all` and `--dry-run` skip the question.

The [first-sync picker](@/docs/cli-reference/sync.md#first-sync-target-picker) ticks only the tools it detects.

## list

Print all loaded specs as `kind<tab>name<tab>layer`; built-ins add their name and the running version. With no specs, a hint goes to stderr and stdout stays empty.

| Flag | Description |
|------|-------------|
| `--global` | List the specs in effect from `$AGNOSTIC_AI_HOME` (default `~/.agnostic-ai`) and its `local/` overrides, including enabled built-ins. Works outside a project. |
| `--json` | Print an `entries` array of `kind`, `name`, `path`, and `layer`. A built-in has an empty `path` and a `builtin` object with `name` and `version`. An empty scope prints an empty array. |

## new

Create one spec in its configured `sources:` directory. Kinds: `agent`, `skill`, `rule`, `hook`, `mcp`, `command`, `settings`, `review`, `environment`, and `ignore`. Hooks, MCP, settings, and environments use YAML. Other kinds use Markdown.

```bash
agnostic-ai new rule no-console-log     # → <rules>/no-console-log.md
agnostic-ai new hook fmt-on-save        # → <hooks>/fmt-on-save.yaml
agnostic-ai new rule payments-context --scope services/payments
```

| Flag | Description |
|------|-------------|
| `--dry-run` | Preview the destination and exact content. |
| `--scope <dir>` | `new rule` only. Create a flat rule for a project-relative directory. See [scoped context](@/docs/scoped-context.md). |

It errors if the destination exists. Names are lowercase slugs (`[a-z0-9][a-z0-9-]*`). Replace the `TODO` description and body; `lint` reports the placeholder description as LINT031. Settings and environment templates hold only commented hints, and an ignore spec excludes no paths until you add patterns.

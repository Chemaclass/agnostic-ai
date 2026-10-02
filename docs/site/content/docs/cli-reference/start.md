+++
title = "Start a project"
description = "Commands that scaffold a project, import existing config, and create specs."
weight = 10

[extra]
group = "Reference"
+++

# Start a project

## use

Start using an AI tool with what the project already has, in one step:

```bash
agnostic-ai use codex                  # switching to Codex this month
agnostic-ai use claude codex cursor    # a team on several tools
```

- In a project with no `agnostic-ai.yaml`, it creates one that enables the tools it detects plus the ones you name. It imports their instructions, skills, agents, hooks, and MCP servers into `.agnostic-ai/`, and asks about the `.gitignore` block as `init` does.
- In an existing project, it adds each named tool to `targets` and first imports that tool's own config, such as a hand-written `AGENTS.md`, so the sync keeps it.

It then syncs and shows what each added tool now reads:

```
✓ codex now reads, from .agnostic-ai/:
    instructions     AGENTS.md
    1 skill          .agents/skills/        review
    1 MCP server     .codex/config.toml     github
  edit .agnostic-ai/ and run agnostic-ai sync to change what every tool reads
```

A tool already in `targets` changes nothing, and a run that stopped partway finishes on the next try. A mistyped name fails with the closest one. `use` refuses to start a project inside one an enclosing directory holds, and to add a tool when `agnostic-ai.local.yaml` sets `targets`, since that list wins; add it there instead. `use` only adds tools; remove one from `targets` by hand.

## init

Scaffold a project: `agnostic-ai.yaml` and the managed `.gitignore` block. Errors if `agnostic-ai.yaml` exists. It creates only the source folders that `--demo` or `--preset` seed; `new` and `import` create the rest on first use.

```bash
agnostic-ai init specs --demo     # base dir specs/, example specs to start from
echo "claude,codex" | agnostic-ai init
```

| Flag | Description |
|------|-------------|
| `[dir]` | Base directory for the source folders (`.` for the legacy root layout). Writes matching `sources:` paths; the default `.agnostic-ai/` writes none. |
| `--demo` | Seed example specs, one per source folder plus the `memory-curator` skill, so the first `sync` produces output. Never overwrites files. |
| `--preset <name>` | Seed starter specs for a stack: `go`, `ts-react`, `python`. Combines with `--demo` and `--all`. Never overwrites files. |
| `-a, --all` | Skip the target picker and enable every supported target. |
| `--gitignore[=on\|off]` | On by default: generated outputs go into a managed `.gitignore` block. `--gitignore=off` commits them instead. `true` and `false` work too, here and on `sync`. Give the value after `=`: `--gitignore off` stops with a hint, since `off` would read as the folder argument. |
| `--from <cli>` | After scaffolding, import existing config from this CLI (`claude`, `cursor`, `all`, and the other [import](#import) sources). |
| `--dry-run` | List what the scaffold would create without writing. With `--from`, also list every file the import would write. |

Without `--all`, `init` takes targets from a TTY prompt or a comma-separated list on piped stdin. With no terminal and nothing piped (CI), it enables the tools it detects, else the [default set](@/docs/configuration.md#targets), and prints one line naming them. When a root `AGENTS.md` exists and `codex` is not in that set, a second line suggests enabling it, since Codex owns that file. Unknown names error and write nothing.

The prompt and the [first-sync picker](@/docs/cli-reference/sync.md#first-sync-target-picker) pre-tick tools detected from a marker such as `.claude/`, a root `CLAUDE.md`, `.codex/`, `.gemini/`, `.cursor/`, or `.github/copilot-instructions.md`. A root `AGENTS.md` is not a marker: most tools read it.

## import

Translate an existing AI CLI configuration into agnostic specs, written into the `sources:` directories from `agnostic-ai.yaml`.

```bash
agnostic-ai import claude
agnostic-ai import claude codex   # in order; AGNOSTIC_AI.md comes from the last
agnostic-ai import all
agnostic-ai import claude codex --dry-run --diff   # review content and conflicts
```

`import all` imports every tool detected from its marker. A detected tool with no importer is skipped with a `skipping <tool>` line. An entry file that links outside the project is skipped with a `skipped <file>` note; naming the tool (`import claude`) follows the link. Output that sync wrote and nobody edited is skipped, so `import all` right after `sync` changes nothing. A root `AGENTS.md` is not a marker, so `import all` (and `init --from all`) reads it last: each `##` section of a hand-written `AGENTS.md` that `.agnostic-ai/AGNOSTIC_AI.md` does not hold yet is appended below the imported body, and a `merged <n> sections from AGENTS.md` line names them. When no tool is detected, a hand-written root `AGENTS.md` seeds `.agnostic-ai/AGNOSTIC_AI.md` and a `seeded from AGENTS.md` line says so; a generated one is left alone and `no importable AI CLI configs detected` is printed. See [Claude import](@/docs/targets/claude.md#import) for what `import claude` leaves in place.

`import --global` reads your user config (default model and effort, MCP servers) from the tools `sync --global` writes. It writes `settings/imported.yaml` and one `mcps/<name>.yaml` per server into `$AGNOSTIC_AI_HOME`. Name targets to narrow it. Anything a home spec already provides, `local/` included, is left out, and an existing spec file is never replaced. Two tools defining one server differently keep the first tool's server, with a warning. Servers that do not round-trip are skipped with a warning.

| Flag | Effect |
|---|---|
| `--dry-run` | List every file the import would write, once each, without file bodies. Runs in a temporary copy of the project and writes nothing to it. |
| `--diff` | With `--dry-run`, show each destination as `create`, `change`, or `unchanged`, the sources that wrote it, and a unified diff. Lists every destination two sources propose different content for, and the source a real import keeps (the last). A conflict is reported, not resolved: exit status stays 0. Requires `--dry-run`. |

- Writes only spec files under `sources:`. Run it after `init`; re-running overwrites by filename.
- Nested config search skips git-ignored directories, directories with their own `.git`, and `node_modules/`.
- A symlinked skill folder that links outside the project is skipped with a `skipped <path>` note.
- An existing skill or agent spec keeps frontmatter keys the source tool has nowhere to put (such as Cursor's `argument-hint`). Deleting a key the tool does write is read as deliberate and reaches the spec (removing `model` from a Qoder agent removes it from the spec). Rules are rebuilt from the native file.
- Each source mirrors its top-level instructions file to `.agnostic-ai/AGNOSTIC_AI.md`, so the last argument wins. A fenced `AGNOSTIC_AI.md` stays untouched when the entry point matches its rendered view; otherwise import overwrites it and warns.
- If another entry point holds different hand-written content (a distinct `AGENTS.md` next to `CLAUDE.md`), import warns that `sync` would overwrite it. Merge it into `.agnostic-ai/AGNOSTIC_AI.md` first. `import all` merges a root `AGENTS.md` itself instead of warning.
- `all` cannot combine with other sources.
- Valid sources: `claude`, `codex`, `cursor`, `aider`, `amp`, `warp`, `gemini`, `copilot`, `opencode`, `zed`, `antigravity`, `continue`, `cline`, `windsurf`, `junie`, `trae`, `kiro`, `crush`, `qoder`, `kilo`, `goose`, plus `all`. `factory`, `openhands`, `jules`, and `augment` are emit-only.

Each [target page](@/docs/targets/_index.md) lists what `import <target>` reads under its Import section.

### Filename prefix reclassification {#filename-prefix-reclassification}

`import cline`, `import windsurf`, `import trae`, `import qoder`, and `import continue` reclassify each file in their rules directory by name:

| Target | Rules directory |
|--------|-----------------|
| `cline` | `.clinerules/`, with `.cline/rules/` fallback |
| `windsurf` | `.devin/rules/`, with `.windsurf/rules/` fallback |
| `trae` | `.trae/rules/` |
| `qoder` | `.qoder/rules/` |
| `continue` | `.continue/rules/` |

| Source filename | Becomes |
|-----------------|---------|
| `agent-<name>.md` | `<agents>/<name>.md` |
| `skill-<name>.md` | `<skills>/<name>.md` |
| `<name>.md` | `<rules>/<name>.md` |
| `<scope>/<file>.md` | nested under the same `<scope>` in the destination |

Import strips the leading `# <heading>` block the adapter adds on emit and injects minimal `name:` frontmatter.

## list

Print all loaded specs as `kind<tab>name<tab>layer`. With no specs, the hint goes to stderr and stdout stays empty.

| Flag | Description |
|------|-------------|
| `--global` | List effective specs from `$AGNOSTIC_AI_HOME` (default `~/.agnostic-ai`) and its `local/` overrides. Layers are `global` and `global-local`. Works outside a project. |

## new

Scaffold one agent, skill, rule, hook, or mcp spec with kind-appropriate frontmatter in that kind's `sources:` directory.

```bash
agnostic-ai new rule no-console-log     # → <rules>/no-console-log.md
agnostic-ai new hook fmt-on-save        # → <hooks>/fmt-on-save.yaml
agnostic-ai new rule payments-context --scope services/payments
```

| Flag | Description |
|------|-------------|
| `--scope <dir>` | `new rule` only. Create a flat rule for a project-relative directory. See [scoped context](@/docs/scoped-context.md). |

Errors if the destination exists. Names must be lowercase slugs (`[a-z0-9][a-z0-9-]*`).


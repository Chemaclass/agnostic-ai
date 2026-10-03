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

- In a project with no `agnostic-ai.yaml`, it creates one that enables the tools it detects plus the ones you name. It imports their instructions, skills, agents, hooks, and MCP servers into `.agnostic-ai/`. It asks about the `.gitignore` block as `init` does.
- In an existing project, it first imports each named tool's own config, such as a hand-written `AGENTS.md`, so the sync keeps it. Then it adds the tool to `targets`.
- If an import would replace a spec with content that tool never read, `use` stops before writing any spec, lists each one, and names the `agnostic-ai import <tool> --overwrite` that replaces them.
- If an import fails, `use` leaves that tool out of `targets` and names the command to retry.

It then syncs and shows what each added tool now reads:

```
✓ codex now reads, from .agnostic-ai/:
    instructions     AGENTS.md
    1 skill          .agents/skills/        review
    1 MCP server     .codex/config.toml     github
  edit .agnostic-ai/ and run agnostic-ai sync to change what every tool reads
```

A tool already in `targets` changes nothing. A run that stopped partway finishes on the next try. A mistyped name fails with the closest one. `use` only adds tools. Remove one from `targets` by hand.

{% <details summary="When use refuses or stops"> %}
- A tool whose instructions file is in `sync.unmanaged` stops `use`, because importing it would copy that file to every tool. Import it by hand instead.
- If `use` is interrupted before its imports finish, `sync` stops until `use` runs again.
- `use` refuses to start a project inside one that an enclosing directory holds.
- `use` refuses to add a tool when `agnostic-ai.local.yaml` sets `targets`, because that list wins. Add the tool there instead.
{% </details> %}

## init

Scaffold a project: `agnostic-ai.yaml` and the managed `.gitignore` block. It errors if `agnostic-ai.yaml` exists. It creates only the source folders that `--demo` or `--preset` seed; `new` and `import` create the rest on first use.

```bash
agnostic-ai init specs --demo     # base dir specs/, example specs to start from
echo "claude,codex" | agnostic-ai init
```

| Flag | Description |
|------|-------------|
| `[dir]` | Base directory for the source folders (`.` for the legacy root layout). Writes matching `sources:` paths; the default `.agnostic-ai/` writes none. |
| `--demo` | Seed example specs, one per source folder plus the `memory-curator` skill, so the first `sync` produces output. The [`no-force-push` hook](@/docs/spec-format/hooks.md#shared-hook-scripts) script goes to `.agnostic-ai/scripts/` whatever the base dir. Never overwrites files. |
| `--preset <name>` | Seed starter specs for a stack: `go`, `ts-react`, `python`. Combines with `--demo` and `--all`. Never overwrites files. |
| `-a, --all` | Skip the target picker and enable every supported target. |
| `--gitignore[=on\|off]` | On by default: generated outputs go into a managed `.gitignore` block. `--gitignore=off` commits them instead. `true` and `false` work too, here and on `sync`. Give the value after `=`. `--gitignore off` stops with a hint, because `off` would read as the folder argument. |
| `--from <cli>` | After scaffolding, import existing config from this CLI (`claude`, `cursor`, `all`, and the other [import](#import) sources). |
| `--dry-run` | List what the scaffold would create without writing. With `--from`, also list every file the import would write. |

Without `--all`, `init` takes targets from a TTY prompt or from a comma-separated list on piped stdin. Unknown names error and write nothing.

The prompt starts with a default set ticked, so Enter accepts it. It shows `space to toggle, enter to confirm`. The default is the first of these that is not empty:

1. Tools the project already uses, detected from a marker such as `.claude/`, a root `CLAUDE.md`, `.codex/`, `.gemini/`, `.cursor/`, or `.github/copilot-instructions.md`. A root `AGENTS.md` is not a marker, because most tools read it.
2. Tools whose CLI is on `PATH`: the ones [`doctor install`](@/docs/cli-reference/check.md#doctor) reports.
3. `claude` and `codex`.

With no terminal and nothing piped (CI), or an empty piped line, `init` enables that same default and prints one line naming it. If a root `AGENTS.md` exists and `codex` is not in that set, a second line suggests enabling it, because Codex owns that file.

In a terminal, when the project already has config a tool's importer reads, or a root `AGENTS.md`, plain `init` asks whether to import it, as `--from all` does. Without a terminal it doesn't import. The summary lists the `agnostic-ai import <tool>` commands instead. `--all` and `--dry-run` skip the question.

The [first-sync picker](@/docs/cli-reference/sync.md#first-sync-target-picker) pre-ticks only detected tools.

## import

Translate an existing AI CLI configuration into agnostic specs. It writes them into the `sources:` directories from `agnostic-ai.yaml`. Absolute source paths keep their destination. `--dry-run` and `--dry-run --diff` preview those files in a temporary copy.

```bash
agnostic-ai import claude
agnostic-ai import claude codex   # in order; AGNOSTIC_AI.md comes from the last
agnostic-ai import all
agnostic-ai import claude codex --dry-run --diff   # review content and conflicts
```

`import all` imports every tool detected from its marker.

- A detected tool with no importer is skipped with a `skipping <tool>` line.
- An entry file that links outside the project is skipped with a `skipped <file>` note. Naming the tool (`import claude`) follows the link.
- Output that sync wrote and nobody edited is skipped, so `import all` right after `sync` changes nothing.

A root `AGENTS.md` is not a marker, so `import all` (and `init --from all`) reads it last. Each `##` section of a hand-written `AGENTS.md` that `.agnostic-ai/AGNOSTIC_AI.md` does not hold yet is appended below the imported body. A `merged <n> sections from AGENTS.md` line names them.

When no tool is detected, a hand-written root `AGENTS.md` seeds `.agnostic-ai/AGNOSTIC_AI.md`, and a `seeded from AGENTS.md` line says so. A generated one is left alone, and `no importable AI CLI configs detected` is printed.

See [Claude import](@/docs/targets/claude.md#import) for what `import claude` leaves in place.

`import --global` reads your user config (default model and effort, MCP servers) from the tools that `sync --global` writes. It writes `settings/imported.yaml` and one `mcps/<name>.yaml` per server into `$AGNOSTIC_AI_HOME`. Name targets to narrow it.

- Anything a home spec already provides, `local/` included, is left out. An existing spec file is never replaced.
- Two tools that define one server differently keep the first tool's server, with a warning.
- Servers that don't round-trip are skipped with a warning.
- A server with a literal [credential](@/docs/spec-format/mcps.md#what-import-writes) is left out with a warning that names the server and field, so no secret reaches the home. A plain setting such as `NODE_ENV: production` still imports.

| Flag | Effect |
|---|---|
| `--dry-run` | List every file the import would write, once each, without file bodies. Runs in a temporary copy of the project and writes nothing to it. |
| `--diff` | With `--dry-run`, show each destination as `create`, `change`, or `unchanged`, the sources that wrote it, and a unified diff. Lists every destination two sources propose different content for, and the source a real import keeps (the last). Disagreement between sources alone keeps exit status 0. Replacing a protected existing spec still requires `--overwrite`. Requires `--dry-run`. |
| `--overwrite` | Replace existing specs the import would change. Without it, see below. |

- Writes only spec files under `sources:`. Run it after `init`.
- An import that would replace an existing spec with content the tool never read stops with [AAI-203](@/docs/errors.md) and writes no spec. It lists each spec and the source that wanted it. Rename one to keep both, or pass `--overwrite`. `--dry-run` and `init --from` stop the same way.
- A spec the tool already reads may be replaced. That is how a native edit comes back. It covers a spec the last sync wrote for that tool, or one an earlier import of that tool wrote, while its bytes are unchanged since. A spec another tool's import wrote stops the import, and the message names both tools. `.agnostic-ai/AGNOSTIC_AI.md` never stops it, since import adds sections to it. In `import claude codex` a later source still replaces what an earlier one wrote.
- An identical spec keeps its content and modification time. After several sources import one file, only sources whose final content matches the file may re-import it without `--overwrite`.
- A sync that keeps edited or unmanaged outputs preserves earlier records for unchanged specs. It doesn't make changed specs safe to replace from that tool. An output not enabled, an omitted skill asset, or settings with no translated fields don't count as read by that tool.
- Destination file links are preserved. A dangling destination link fails with its path. A conflict or interrupt restores import writes through symlinks and hard links. Dry-run preserves those file relationships in its preview.
- Nested config search skips git-ignored directories, directories with their own `.git`, and `node_modules/`.
- A symlinked skill folder that links outside the project is skipped with a `skipped <path>` note.
- An existing skill or agent spec keeps frontmatter keys that the source tool has nowhere to put, such as Cursor's `argument-hint`. Deleting a key the tool does write counts as deliberate and reaches the spec. Removing `model` from a Qoder agent removes it from the spec. Rules are rebuilt from the native file.
- Each source mirrors its top-level instructions file to `.agnostic-ai/AGNOSTIC_AI.md`, so the last argument wins. A fenced `AGNOSTIC_AI.md` stays untouched when the entry point matches its rendered view. Otherwise import overwrites it and warns.
- If another entry point holds different hand-written content (a distinct `AGENTS.md` next to `CLAUDE.md`), import warns that `sync` would overwrite it. Merge that content into `.agnostic-ai/AGNOSTIC_AI.md` first. `import all` merges a root `AGENTS.md` itself instead of warning.
- `all` cannot combine with other sources.
- Valid sources: `claude`, `codex`, `cursor`, `aider`, `amp`, `warp`, `gemini`, `copilot`, `opencode`, `zed`, `antigravity`, `continue`, `cline`, `windsurf`, `junie`, `trae`, `kiro`, `crush`, `qoder`, `kilo`, `goose`, `factory`, `openhands`, `augment`, plus `all`. `jules` is emit-only.

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

Import strips the leading `# <heading>` block that the adapter adds on emit. It injects minimal `name:` frontmatter.

## list

Print all loaded specs as `kind<tab>name<tab>layer`. With no specs, a hint goes to stderr and stdout stays empty.

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

It errors if the destination exists. Names must be lowercase slugs (`[a-z0-9][a-z0-9-]*`).

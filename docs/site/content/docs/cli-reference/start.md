+++
title = "Start a project"
description = "Commands that set up a project, import existing config, and create specs."
weight = 10

[extra]
group = "Reference"
+++

# Start a project

`use <tool>` adds a tool and generates its files in one step; `init` and `import` let you review existing instructions first. `new` creates a spec, and `list` shows what loads. Run these from the project root, naming tools in agents and scripts to skip the interactive choice.

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

Create `agnostic-ai.yaml` and the managed `.gitignore` block. Errors if `agnostic-ai.yaml` exists. It creates only the source folders `--demo` or `--preset` fill; `new` and `import` create the rest.

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
| `--from <cli>` | Then import existing config from this [import source](#import), or `all`. |
| `--dry-run` | List the files it, and with `--from` the import, would write. |

Without `--all`, `init` takes targets from a terminal prompt or a comma-separated list on stdin. Unknown names error and write nothing. The prompt ticks a default set (Enter accepts it), the first of these that has any tools:

1. Tools the project uses, by a marker such as `.claude/`, a root `CLAUDE.md`, `.codex/`, `.gemini/`, `.cursor/`, or `.github/copilot-instructions.md`. A root `AGENTS.md` is not a marker, since most tools read it.
2. Tools whose CLI is on `PATH`, as [`doctor install`](@/docs/cli-reference/check.md#doctor) reports.
3. `claude` and `codex`.

With no terminal and nothing piped (CI), or an empty piped line, `init` enables that default and names it. If a root `AGENTS.md` exists and `codex` is not in the set, it suggests enabling it.

In a terminal, when the project has config an importer reads, or a root `AGENTS.md`, plain `init` offers to import it, as `--from all` does. Without a terminal, the summary lists the `agnostic-ai import <tool>` commands instead. `--all` and `--dry-run` skip the question.

The [first-sync picker](@/docs/cli-reference/sync.md#first-sync-target-picker) ticks only the tools it detects.

## import

Translate an existing AI CLI configuration into specs in the `sources:` directories, after `init`, writing nothing else. Preview diffs hide known [credential values](@/docs/cli-reference/sync.md#credential-previews) on both sides.

```bash
agnostic-ai import claude
agnostic-ai import claude codex   # in order; AGNOSTIC_AI.md comes from the last
agnostic-ai import all
agnostic-ai import claude codex --dry-run --diff   # review content and conflicts
```

| Flag | Effect |
|---|---|
| `--dry-run` | List every file the import would write, without contents, from a temporary copy of the project. |
| `--diff` | With `--dry-run`, show each destination as `create`, `change`, or `unchanged`, its sources, and a unified diff. Names each destination where sources disagree and the one a real import keeps (the last), still exiting 0. A protected spec still needs `--overwrite`. |
| `--overwrite` | Replace existing specs with what the tool's files hold. Keys the tool never shows and comments on kept keys stay; `::target` blocks for other tools are lost. |

### Import behavior

`import all` imports each tool it detects from config files and directories. A detected tool with no importer prints `skipping <tool>`. `all` cannot combine with other sources.

- An entry file or skill folder that links outside the project is skipped with a `skipped <path>` note. Naming the tool (`import claude`) follows an entry file link.
- Unedited sync output is skipped, so `import all` right after `sync` changes nothing.

`import all` (and `init --from all`) reads a root `AGENTS.md` last and adds each `##` section that `.agnostic-ai/AGNOSTIC_AI.md` lacks, reporting `merged <n> sections from AGENTS.md`. With no tool detected, a hand-written root `AGENTS.md` becomes `.agnostic-ai/AGNOSTIC_AI.md` (`seeded from AGENTS.md`); a generated one is left alone (`no importable AI CLI configs detected`).

`import --global [targets]` reads your user config from the tools `sync --global` writes into `$AGNOSTIC_AI_HOME`: default model and effort (`settings/imported.yaml`), Claude Code permission rules (`settings/permissions-claude.yaml`), MCP servers (`mcps/<name>.yaml`), and supported [Kiro hook files](@/docs/targets/kiro.md) (`hooks/<name>.yaml`).

- It leaves out anything a home spec provides, `local/` included, and never replaces a spec file, except that it appends new rules to `settings/permissions-claude.yaml` and removes none.
- Permission rules come only from `permissions.allow`, `ask`, and `deny` in `~/.claude/settings.json`, never managed or project settings. Rules sync recorded there are left out; rules with credentials or shell syntax it cannot inspect are left out with a warning.
- It warns and skips a server another tool defined differently (the first tool wins), one it cannot convert, and one with a literal [credential](@/docs/spec-format/mcps.md#what-import-writes), naming the server and field. A plain setting such as `NODE_ENV: production` still imports.

### Keep existing specs

- Replacing a spec with content the tool never read stops the import with [AAI-203](@/docs/errors.md#aai-203-import-would-replace-an-existing-spec), which lists the cases and fixes. A spec the tool already reads, unchanged since the last sync or that tool's import wrote it, may be replaced; that is how a native edit comes back.
- `.agnostic-ai/AGNOSTIC_AI.md` never stops it. In `import claude codex` a later source still replaces an earlier one.
- An identical spec keeps its content and modification time. After several sources import one file, only those whose content matches it may re-import without `--overwrite`.
- A sync that keeps edited or unmanaged outputs keeps earlier records for unchanged specs, but does not make changed specs safe to replace. A disabled output, a skill asset left out, or settings with no translated fields do not count as read by the tool.

### Bring tool edits back

- Re-importing tool files that still match the last sync keeps the spec exactly as written, including `::target` blocks, `workspaces`, comments, and overlays an older release wrote.
- An edit in a tool's file goes back into the spec, keeping frontmatter keys the tool never shows (such as `workspaces` for Claude) and comments on kept keys, with `--overwrite` too. A spec with `::target` blocks stops with AAI-203 instead.
- A skill or agent spec keeps keys the tool has no place for, such as Cursor's `argument-hint`. Deleting a key the tool does write reaches the spec: removing `model` from a Qoder agent removes it there. Rules are rebuilt from the native file.

### Files and source order

- An absolute source path keeps its own destination. Links at the destination are kept, also in dry-run; a dangling link fails with its path. A conflict or interrupt restores writes made through symlinks and hard links.
- The search for nested config skips git-ignored directories, directories with their own `.git`, and `node_modules/`.
- Each source copies its top-level instructions file to `.agnostic-ai/AGNOSTIC_AI.md`, so the last argument wins. A fenced `AGNOSTIC_AI.md` stays when the entry point matches what sync would write from it; otherwise import overwrites it and warns.
- If another entry point holds different hand-written content (an `AGENTS.md` next to `CLAUDE.md`), import warns that `sync` would overwrite it; merge it into `.agnostic-ai/AGNOSTIC_AI.md` first. `import all` merges a root `AGENTS.md` itself.
- Valid sources: `claude`, `codex`, `cursor`, `aider`, `amp`, `warp`, `gemini`, `copilot`, `opencode`, `zed`, `antigravity`, `continue`, `cline`, `windsurf`, `junie`, `trae`, `kiro`, `crush`, `qoder`, `kilo`, `goose`, `factory`, `openhands`, `augment`, `ellipsis`, plus `all`. `jules` supports sync but not import.

Each [target page](@/docs/targets/_index.md) lists what `import <target>` reads; see [Claude import](@/docs/targets/claude-reference.md#import) for what `import claude` leaves in place.

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

Import strips the leading `# <heading>` block the adapter adds and adds a `name:` field in frontmatter.

### import reconcile

Plan how to bring later edits from a tool's skill folder into the source specs after an import:

```bash
agnostic-ai import reconcile \
  --base before-migration \
  --migrated migration-commit \
  --upstream other-branch \
  --map .cursor/skills=.agnostic-ai/skills
```

| Flag | Commit or path |
|------|----------------|
| `--base` | Holds the original tool skill folder. An ancestor of `--upstream` and `HEAD`. |
| `--migrated` | Holds the first complete source specs, so import's own changes are the baseline. An ancestor of `HEAD`, which supplies the current specs. |
| `--upstream` | The branch with later edits to the tool skill folder. |
| `--map <native>=<source>` | A native skill directory and `sources.skills`, separate and relative to the project directory, even when nested in a larger repository. Repeat per native directory. |
| `--json` | Print the four resolved commit IDs and an `entries` array of `action`, `source`, and `destination`. |

Each row reports `add`, `remove`, `update`, or `conflict`, then the tool and source paths, assets and executable modes included. Matching final files get no row. A conflict is a concurrent source edit, deletion, or addition at an occupied destination; a proposed file clashing with a surviving file above or below it (planned removals can allow a file-to-directory swap); or a change to a destination several mapped sources own. Conflicts do not change the exit status.

It compares file contents stored in Git without merging text. Only direct skill folders with `SKILL.md` and regular files are supported; symlinks and special files stop planning, and every base file needs a source counterpart at `--migrated`. Commit changes in mapped trees first; other uncommitted changes are fine. It only reports a plan for you to apply: it copies no files, restores no skills, and leaves the index alone.

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

+++
title = "CLI reference"
description = "Look up every agnostic-ai command, option, and exit behavior."
weight = 140

[extra]
group = "Reference"
+++

# CLI reference

```
agnostic-ai [command] [flags]
```

## Find a command

| Task | Commands |
|---|---|
| Set up a project | [init](#init), [import](#import), [new](#new) |
| Generate or preview output | [sync](#sync), [render](#render) |
| Check source, output, and behavior | [validate](#validate), [lint](#lint), [doctor](#doctor), [status](#status), [verify](#verify) |
| Inspect routing | [list](#list), [explain](#explain), [compare](#compare), [graph](#graph), [why](#why) |
| Restore or remove generated files | [revert](#revert), [cleanup](#cleanup) |
| Share specs | [packs](#packs) |
| Set up your environment | [completion](#completion), [upgrade or update](#upgrade), [install-hook](#install-hook), [lsp](#lsp) |

Walkthroughs: [Getting started](@/docs/getting-started.md), [Migration](@/docs/migration.md). Automation: [exit codes](#exit-codes), [CI guide](@/docs/ci.md).

## Global flags

| Flag | Description |
|------|-------------|
| `-h, --help` | Help for any command, same as `agnostic-ai help <command>`. |
| `--version` | Print version and exit |
| `-q, --quiet` | Errors only, plus the `~ kept` lines of `sync --keep-edits`, on stderr |
| `-v, --verbose` | Increase output verbosity (repeatable). Mutually exclusive with `--quiet`. |
| `--profile <file>` | Write a `runtime/pprof` CPU profile to `<file>` (or set `AGNOSTIC_AI_PROFILE`). Read it with `go tool pprof <file>`. |

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
| `--gitignore` | On by default: generated outputs go into a managed `.gitignore` block. `--gitignore=false` commits them instead. |
| `--from <cli>` | After scaffolding, import existing config from this CLI (`claude`, `cursor`, `all`, and the other [import](#import) sources). |
| `--dry-run` | List what the scaffold would create without writing. With `--from`, also list every file the import would write. |

Without `--all`, `init` takes targets from a TTY prompt or a comma-separated list on piped stdin. With no terminal and nothing piped (CI), it enables the tools it detects, else the [default set](@/docs/configuration.md#targets), and prints one line naming them. Unknown names error and write nothing.

The prompt and the [first-sync picker](#first-sync-target-picker) pre-tick tools detected from a marker such as `.claude/`, a root `CLAUDE.md`, `.codex/`, `.gemini/`, `.cursor/`, or `.github/copilot-instructions.md`. A root `AGENTS.md` is not a marker: most tools read it.

## import

Translate an existing AI CLI configuration into agnostic specs, written into the `sources:` directories from `agnostic-ai.yaml`.

```bash
agnostic-ai import claude
agnostic-ai import claude codex   # in order; AGNOSTIC_AI.md comes from the last
agnostic-ai import all
agnostic-ai import claude codex --dry-run --diff   # review content and conflicts
```

`import all` imports every tool detected from its marker. A detected tool with no importer is skipped with a `skipping <tool>` line. An entry file that links outside the project is skipped with a `skipped <file>` note; naming the tool (`import claude`) follows the link. Output that sync wrote and nobody edited is skipped, so `import all` right after `sync` changes nothing. See [Claude import](@/docs/targets/claude.md#import) for what `import claude` leaves in place.

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
- If another entry point holds different hand-written content (a distinct `AGENTS.md` next to `CLAUDE.md`), import warns that `sync` would overwrite it. Merge it into `.agnostic-ai/AGNOSTIC_AI.md` first.
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

## validate

Load all specs, report parse errors, and print `loaded 12 entries. ok.` on success. With no specs, stdout still says `loaded 0 entries. ok.` and stderr suggests `init` or `import`.

| Check | Reports |
|-------|------------|
| Hook events | A hook spec's `event:` missing, or supported by no configured target (with the supported list). |
| Orphaned kinds | Hook or MCP specs no enabled target consumes, one line per kind naming targets that would. |
| Declared sources | An explicit `sources.<kind>` path in `agnostic-ai.yaml` with no directory. Warning only. |
| Entry-point fences | A `::target` / `::targets` name in `.agnostic-ai/AGNOSTIC_AI.md` that is not a built-in target or listed in `targets` (external adapter), or that reads no entry-point file (`cursor`, or any target with `outputs.<target>.rules-file`). |
| Global rules | With `--global`, a rule with scope, path, glob, or target conditions, which `sync --global` rejects. |
| Global settings | With `--global`, a settings `effort` a target cannot take, such as `max` for Claude. |

| Flag | Description |
|------|-------------|
| `--fix` | Rewrite source spec files to repair autofixable issues. |
| `--global` | Validate the specs in `$AGNOSTIC_AI_HOME` (default `~/.agnostic-ai`) and its `local/` overrides. Hook events are checked against the targets `sync --global` writes hooks for. A home config `targets` list narrows the checks. Works outside a project. |

Hook events accepted per target:

| Target | Events |
|--------|--------|
| Claude | `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `SessionStart`, `SessionEnd`, `Stop`, `SubagentStop`, `PreCompact`, `PostCompact`, `Notification` |
| Codex | `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `SessionStart`, `SessionEnd`, `Stop`, `PreCompact`, `PostCompact` |
| Gemini | `BeforeTool`, `AfterTool`, `BeforeAgent`, `AfterAgent`, `Notification`, `SessionStart`, `SessionEnd`, `PreCompress`, `BeforeModel`, `AfterModel`, `BeforeToolSelection` |
| Cursor | `beforeShellExecution`, `afterShellExecution`, `beforeMCPExecution`, `afterMCPExecution`, `beforeReadFile`, `afterFileEdit`, `beforeSubmitPrompt`, `preToolUse`, `postToolUse`, `postToolUseFailure`, `sessionStart`, `sessionEnd`, `subagentStart`, `subagentStop`, `preCompact`, `stop`, `afterAgentResponse`, `afterAgentThought`, `beforeTabFileRead`, `afterTabFileEdit`, `workspaceOpen` |
| Augment | `PreToolUse`, `PostToolUse`, `Stop`, `SessionStart`, `SessionEnd` |
| Qoder | `SessionStart`, `SessionEnd`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `PermissionRequest`, `PermissionDenied`, `Stop`, `StopFailure`, `SubagentStart`, `SubagentStop`, `PreCompact`, `PostCompact`, `Notification`, `InstructionsLoaded`, `ConfigChange`, `CwdChanged`, `FileChanged`, `WorktreeCreate`, `WorktreeRemove`, `Elicitation`, `ElicitationResult`, `TaskCreated`, `TaskCompleted`, `TeammateIdle`, `Setup` |

## lint

Semantic checks beyond the schema. Exits 1 on error findings. It flags empty specs, dead specs (kinds no enabled target supports), and hooks that set a matcher on an event that ignores it.

```bash
agnostic-ai lint --strict
```

| Flag | Description |
|------|-------------|
| `--strict` | Exit 1 on warnings too, for CI. |
| `--global` | Lint the specs in `$AGNOSTIC_AI_HOME` (default `~/.agnostic-ai`) and its `local/` overrides. Also reports LINT010 and LINT014. Budgets come from the home config's `lint` key. Works outside a project. |

| Code | Finding |
|------|---------|
| LINT003 | Two specs of one kind share a `name`; the loader keeps one body. Hooks sharing an event and matcher are fine. |
| LINT006 | Error. Frontmatter opens `---` and never closes, so the raw YAML is emitted as body. |
| LINT010 | Error, `--global` only. A rule with scope, path, glob, or target conditions, which `sync --global` rejects. |
| LINT014 | Error, `--global` only. A settings `effort` a target's user effort key cannot take, such as `max` for Claude or Copilot, which `sync --global` drops with a note. |
| LINT013 | Error. A rule's `globs` or `x-<target>.globs` is neither a string nor a list of strings. The rule loads in every session. `validate` reports it too. |
| LINT016 | Error. An environment spec's `dev-commands` entry has no `name:` or `command:`, repeats a name, is not a mapping, sets a key no target reads, or gives `cwd`, `url`, `auto-port`, `port`, or `env` the wrong type. `x-claude` overrides are checked too. |
| LINT008 | Error. A stdio MCP server lacks `command:`, or an `http`/`sse`/`ws` one lacks `url:`. `x-<target>` cannot set either reserved field. |

LINT007 warns on a frontmatter key one edit away from a key agnostic-ai reads (`glob:` for `globs:`), since the setting is lost. `sync` prints the same warning. Put target-native keys under `x-<target>:`. A key some targets read at the top level (Qoder's `glob:`, OpenCode's and Kilo's `mode:`) is flagged only when none of those targets is in `targets`. Settings and environment specs are not checked.

LINT015 warns when a spec body names another spec by a target-native path such as `.claude/skills/style/SKILL.md`. The finding names the `.agnostic-ai/` source path to use.

LINT009 warns on a permission that approves more than it says. An `allow` rule such as `Bash(git * main)` also matches a force push to main. Write the exact value, or keep `*` at the end (`Bash(go test:*)`). A `deny` rule with the same shape blocks nothing in Claude Code, so it is flagged too. `ask` rules are skipped.

LINT011 warns when a target loads more words at session start than [`lint.instructions-words`](@/docs/configuration.md#lint) allows (default 2000). The count covers the entry-point file as `sync` writes it, always-on rule files, every skill and agent description, and `@`-imported rule files. The adapter decides what is always on. Targets with the same numbers share one line:

```
LINT011 [warn] AGENTS.md: cline, windsurf, trae load 2396 words every session: AGENTS.md 1187 (AGNOSTIC_AI.md 136, rules 1032), always-on rule files 908, skill descriptions 195, agent descriptions 106; budget 2000 (lint.instructions-words).
```

A target with a published byte cap also warns past it: Codex stops reading `AGENTS.md` at 32 KiB (`project_doc_max_bytes`), Antigravity truncates rule files past 24,000 bytes.

LINT012 warns on a skill or agent description (or `x-<target>.description`) longer than [`lint.description-chars`](@/docs/configuration.md#lint) (default 1024). A skill description past 1024, the Agent Skills limit, warns even under a raised budget.

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

## explain

List every output file and section one spec contributes to, the reverse of the `<!-- source: ... -->` markers in merged documents. With `--file`, list the instructions configured for one source file instead.

```bash
agnostic-ai explain rules/conventional-commits.md --json
```

| Flag | Description |
|------|-------------|
| `--json` | Stable schema for editor extensions and scripts. |
| `--global` | Explain a spec in `$AGNOSTIC_AI_HOME` (default `~/.agnostic-ai`) or its `local/` layer. A relative path resolves against that root. |

Contributions are grouped by configured target, plus a "would emit if enabled" list for inactive adapters, tagged `(full file)` or `(section "<name>")`. With `--global`, an agent or skill is a whole file, a rule is a section of the instructions file, a hook is its event in the hooks file, an MCP server is its key in the user MCP file, and a settings spec lists each key it sets, tagged `(key "<key>")`.

```json
{"version": "1", "command": "explain", "spec": {"kind": "rule", "name": "...", "path": "..."},
 "contributions": [{"target": "...", "path": "...", "section": "...", "mode": "full|section|key"}],
 "would_emit_if_enabled": []}
```

### Explain a source file

Start from a project file instead of a spec. The report lists every instruction the target would read from the planned sync output, with its source, output path, selector, and reason. Cursor is the only supported target.

```bash
agnostic-ai explain --file services/payments/handler.go --target cursor
```

| Flag | Description |
|------|-------------|
| `--file <path>` | Project file to inspect. The file does not have to exist. Cannot be combined with a spec or error code argument. |
| `--target <name>` | Required with `--file`. Must be a configured target. Other targets fail with an unsupported-target error. |

| Status | Meaning |
|--------|---------|
| `always` | No file condition: `alwaysApply: true`, or the root `AGENTS.md`. |
| `match` | A `globs` pattern or a nested `AGENTS.md` directory covers the file. |
| `no-match` | A selector exists and misses the file. |
| `model-selected` | `alwaysApply: false` with a description and no globs. Cursor's agent decides. |
| `manual` | `alwaysApply: false` with neither. Loads only when `@`-mentioned. |
| `unknown` | Undocumented glob syntax (braces, classes, negation), or unreadable frontmatter. |
| `excluded` | Target selection (`target`, `targets`, `target-exclude`) leaves the target out. |
| `not-emitted` | The rule targets Cursor but sync writes nothing for it. |

A root `AGENTS.md` written for a peer target such as Codex reaches Cursor too. The report shows configured applicability, not the model's active context.

```json
{"version": "1", "command": "explain", "file": "...", "target": "cursor", "note": "...",
 "instructions": [{"status": "match", "source": "...", "output": "...", "selector": "...", "reason": "..."}]}
```

## compare

Compare how two built-in targets represent the project's agents and rule activation, before you switch or add a tool.

```bash
agnostic-ai compare claude cursor
```

| Flag | Description |
|------|-------------|
| `--json` | Stable schema for scripts. |

Coverage is agent fields plus rule `scope`, `paths`, `globs`, and `alwaysApply`. Other rules and spec kinds are left out. Each field gets one result per target:

| Result | Meaning |
|---|---|
| `preserved` | Written under the same key with the same values. |
| `translated` | Written under another key or file, with rewritten values, or only in part. |
| `unsupported` | The target has no home for the field or the kind. |
| `excluded` | The spec never reaches the target: a target filter, an opt-in output, or an inexpressible scope. |
| `unknown` | The emission gives no evidence either way. |

`preserved` describes the written file, not runtime behavior. `(differs)` marks a field with a different result per target. Each result names the output paths or reason, plus a `next:` step when known. Unknown targets, the same target twice, invalid specs or config, and external adapters fail the command.

```json
{"version": "1", "command": "compare", "targets": ["claude", "cursor"], "coverage": "...", "caveat": "...",
 "specs": [{"kind": "agent", "name": "...", "path": "...", "fields": [{"field": "tools", "differs": true,
   "results": [{"target": "cursor", "status": "unsupported", "reason": "...", "next": "..."}]}]}],
 "fields": 6, "differences": 3}
```

## render

Print what each target emits for one spec, without writing files.

```bash
agnostic-ai render rules/no-console-log.md --target claude,codex
```

| Flag | Description |
|------|-------------|
| `-t, --target <list>` | Targets to render, repeated or comma-separated. Default: all in `agnostic-ai.yaml`. |

Each file prints as `# target: <name>: <output path>` and its body. Targets that emit nothing for the kind print a note.

## sync

Emit per-target configs.

```bash
agnostic-ai sync --only claude,cursor
```

| Flag | Description |
|------|-------------|
| `-t, --target <list>` | Comma-separated targets (default: all in config) |
| `--only <list>` | Emit only these targets. Errors on unknown names. |
| `--except <list>` | Emit all configured targets except these. Errors on unknown names. Mutually exclusive with `--only`. |
| `--all` | Emit every configured target without the [first-sync picker](#first-sync-target-picker). |
| `--dry-run` | Print to stdout instead of writing files. Does not preview the orphan sweep. |
| `--plan` | Print per-target added and changed counts without writing. Exits 0. |
| `--check` | Exit non-zero if disk differs from emitted output. Writes nothing. |
| `--diff` | With `--check`, print a unified diff per drifted file (on-disk vs what sync would write). |
| `--format <human\|github>` | With `--check`: `human` (default) table or `github` Actions annotations. `--json` wins. |
| `--backup` | Copy each existing target file to `<path>.bak` before overwriting. Pair with `revert`. |
| `--keep-edits` | Keep each output edited since the last sync, write the rest, and name each kept file as `~ kept <path>` (on stderr under `--quiet`). Exits 0. For [git hooks](@/docs/git-hooks.md#regenerate-on-checkout). Not with `--check`, `--plan`, `--watch`, or `--global`. |
| `--untrack` | Run `git rm --cached` on generated paths that git tracks and ignores. The working copy stays. Not with `--check`, `--plan`, `--dry-run`, `--watch`, or `--global`. |
| `--gitignore <on\|off>` | Override `gitignore.enabled` for this run. |
| `--watch` | Stay running and re-emit on changes. Incompatible with `--check`. See [watch mode](#watch-mode). |
| `--watch-poll` | With `--watch`, force 200 ms polling, for network mounts or container volumes. |
| `--jobs <n>` | Targets emitted in parallel. `0` (default) is one worker per CPU; `1` is serial. See [parallel emission](#parallel-emission). |
| `--json` | Output as JSON, also with `--plan` or `--dry-run`. Not with `--watch` or `--diff`. See [JSON output](#json-output). |
| `--global` | Install user-level instructions, unconditional rules, hooks, and skills from `$AGNOSTIC_AI_HOME` (default `~/.agnostic-ai/`) into 22 tools' user config, plus native agents for 18 targets. |

`sync --global` details:

- Loads overrides from `local/` in the source root. Reads `targets` from an optional `agnostic-ai.yaml` there. See [global configuration](@/docs/configuration.md#global-configuration) and [global output](@/docs/target-behavior.md#global-output).
- Works outside a project; never loads project config or packs.
- Accepts `--target`, `--only`, `--except`, `--dry-run`, `--check`, `--check --diff`, `--backup`, `--plan`, and `--json`. `--plan` lists each file a sync would create, update, or delete, with key-level settings and MCP edits. `--json` adds a `keys` list per settings or MCP file.
- MCP specs install servers in the user MCP files of Augment, Claude, Codex, Cursor, Copilot, Gemini, OpenHands, and Qoder. Settings specs set `model` and `effort` for Claude, Codex, Copilot, Qoder, and Gemini. See [default model and effort](@/docs/configuration.md#global-default-model-and-effort).
- Rejects `--watch`, `--gitignore`, `--jobs`, and `--plan` with `--check` or `--dry-run`.

Paths listed under [`sync.unmanaged`](@/docs/configuration.md#syncunmanaged) are skipped and reported as `~ skip (unmanaged) <path>`.

**Summary.** `sync` ends with what it changed. With nothing to do it prints `✓ 25 targets up to date · 49ms`. After an edit it names the changed specs, then the files created (`+`), updated (`~`), and removed (`-`):

```
  ~ rule testing → 9 files in 9 targets
  + .cursor/rules/testing.mdc  .kiro/steering/testing.md  (+1 more)
  ~ AGENTS.md  CLAUDE.md  GEMINI.md  (+2 more)
  ! 1 file to commit: .openhands/setup.sh
✓ synced 25 targets · 3 created · 6 updated · 49ms
```

Each list shows three paths; `-v` lists all. `config` in the spec line means `agnostic-ai.yaml` changed. The `!` line lists changed files git tracks or does not ignore, and is absent outside a git repository.

**Tracked despite ignored.** A generated path git already tracks, typically committed before it moved into the managed `.gitignore` block, prints `! 1 file tracked despite being ignored: git rm --cached .claude/rules/tone.md`. `sync --untrack` runs that command for every such path and reports `~ untracked <path>`. `doctor` shows the same finding and never fails on it alone.

**Orphan sweep.** `sync` records every file it writes in `.agnostic-ai/.sync-state`. A full run deletes files it no longer emits and prunes empty directories, but only what it can prove it wrote (provenance header or recorded hash). A file edited since is kept as `~ kept orphan <path>` and counts as drift until you delete it or list it under `sync.unmanaged`.

Without `.sync-state` (a fresh checkout of a repo that commits generated files), `sync --check` and `doctor` scan git-tracked files instead. A tracked file is a leftover when it sits where a configured target writes and its first line carries the provenance header. A plain full `sync` removes none of them; it records them under target `unledgered` and lists each as `~ kept leftover <path>`. `--only` and `--except` record them without naming them. An empty `.sync-state` still counts as a ledger and turns this scan off. `doctor --fix` removes a leftover in a tool directory or root dotfile. Delete a scope document such as `services/api/AGENTS.md` by hand or add it to `sync.unmanaged`.

### First-sync target picker

On the first `sync` (no `.agnostic-ai/.sync-state` yet), if the config still lists every supported target, `sync` asks which to keep. The choice is saved to `agnostic-ai.yaml`.

| Context | Behavior |
|---------|----------|
| TTY | Multi-select prompt, same as `init`. |
| Piped stdin | Selects and saves without a prompt. |
| Non-TTY, nothing piped (CI) | Emits every configured target. |
| `--all`, `-t`, `--only`, or `--except` | Skips the picker for this run. |

`echo "claude,codex" | agnostic-ai sync` keeps two targets.

### Reading a failing `--check` {#reading-a-failing---check}

A drifting `--check` exits non-zero in every format. A file still holding what the last sync wrote is reported as out of date (the specs changed). A file whose bytes changed since is reported as edited locally (the next sync overwrites it). Without a record of the last sync, as in a fresh CI checkout, a changed file reads as out of date. Stderr names the fix, `agnostic-ai sync`, and points at `agnostic-ai doctor`.

- `--diff` prints changed lines. A missing file gets a one-line create summary; a large diff truncates with a count.
- `--format=github` emits `::error file=...,line=...::` annotations on the pull request.

Neither has a config-file key.

### Watch mode

`sync --watch` watches `agnostic-ai.yaml`, `agnostic-ai.local.yaml`, `.agnostic-ai/AGNOSTIC_AI.md`, every `sources.*` directory, `.agnostic-ai/local/`, and `.agnostic-ai/overlays/`, including ones that appear later. It uses fsnotify with a 50 ms debounce, polls every 200 ms where fsnotify fails, and exits on Ctrl+C. A spec change re-syncs only targets that emit that kind. Config and overlay edits, deletes, and renames re-sync everything.

### Parallel emission {#parallel-emission}

`sync` emits each target on its own worker; `--jobs <n>` bounds how many run at once. There is no config-file key. Output never depends on the value. Use `--jobs 1` only to debug or pin ordering.

### Profiling a slow sync

`--profile <file>` or `AGNOSTIC_AI_PROFILE=<file>` writes a CPU profile. `--verbose` adds wall time per target, as in `→ claude: 12 created, 3 updated, 0 unchanged in 42ms`. Under `--jobs > 1` these times overlap: read them as per-adapter cost, not a serial breakdown.

### JSON output

`sync --json`, `sync --plan --json`, `sync --dry-run --json`, and `sync --check --json` share one schema:

| Field | Description |
|-------|-------------|
| `version` | Schema version, currently `"1"`. Breaking changes bump it. |
| `command` | `"sync"`, `"sync --plan"`, `"sync --dry-run"`, or `"sync --check"`. |
| `writes` | Files written (`"create"`, `"update"`), orphans removed (`"delete"`), or, for `--check`, files needing attention (`"missing"`, `"stale"`, `"edited"`, `"orphan"`, `"leftover"`). `"stale"`: the specs changed. `"edited"`: the file changed since the last sync. `"leftover"`: no longer generated, removed by the next full sync (target `ledger`, or `unledgered` when no ledger proves sync wrote it). With `--untrack`, a path removed from the index is `"untracked"`, target `agnostic-ai`. |
| `skipped` | Files already matching (`"skip"`), user-owned (`"unmanaged"`), edited orphans kept (`"orphan"`), unledgered leftovers kept (`"leftover"` for `doctor --fix` to remove, `"orphan"` for a scope document), or, with `--keep-edits`, hand edits left in place (`"edited"`). A path git tracks and ignores, without `--untrack`, is `"tracked"`, target `agnostic-ai`. Empty for `--check`. |
| `errors` | Per-target errors with `target` and `message`. |

`writes` and `skipped` entries have `target`, `path`, `action` (strings), and `bytes` (number), for example `{"target": "claude", "path": "CLAUDE.md", "action": "create", "bytes": 1284}`.

`--plan --json` and `--dry-run --json` write nothing and exit 0. Each leftover they would remove is `"delete"` in `writes`; kept files are `"orphan"` or `"leftover"` in `skipped`. `--dry-run --json` also lists every unchanged output as `"skip"`. Count `writes` by `target` for the per-target numbers `--plan` prints.

## verify

Run a project-owned behavior check against each selected AI harness (omit `--target` for all configured targets). It first runs the target-scoped `sync --check`; a missing or stale generated file stops verification.

```bash
agnostic-ai verify --target codex
```

The command in [`verify.command`](@/docs/configuration.md#verify) runs once per target, without a shell, and receives this JSON document on stdin. Stdout and stderr pass through. A non-zero verifier exit stops the run and becomes the `agnostic-ai` exit code.

| Field | Meaning |
|------|---------|
| `version` | Contract version, currently `1`. |
| `target` | Target being verified, such as `"codex"`. |
| `configured_model` | Optional. Model from the rendered native config, not the model used at runtime. |
| `cli` | Optional. Detected CLI command, resolved path, and `--version` output. Omitted when the binary is missing. |
| `harness_fingerprint` | Stable SHA-256 digest of target-relevant specs and rendered files. It identifies the harness; it is not a score. |

The verifier owns datasets, judging, results, and baselines.

## doctor

Report missing (never synced), stale (out of date with the specs), edited (changed since the last sync), and orphaned (no longer generated, kept because edited) files. Read-only unless `--fix`. Exits non-zero on any drift.

| Flag | Description |
|------|-------------|
| `-t, --target <list>` | Comma-separated targets (default: all in config) |
| `--fix` | Write missing, stale, and edited files. Orphans stay for you to delete, so the exit stays non-zero while any remain. |
| `--backup` | With `--fix`, copy each existing file to `<path>.bak` before overwriting. |
| `--check-globs` | Flag rules whose `globs:` match no files. Off by default. |
| `--check-references` | Flag relative Markdown links in generated skills whose file is missing on disk. Off by default. |
| `--json` | Drift report as JSON, same schema as `sync --check --json`. With `--check-references`, adds a `references` list. |

`--check-references` reads each Markdown document a selected target writes for its skills. A link is valid when it resolves from the document's own directory or, inside the project, from the project root. Code spans, code blocks, URLs, absolute paths, and `#fragment`-only links are skipped; only the file of a `file#fragment` link is checked. [`doctor.check-references.ignore`](@/docs/configuration.md#doctorcheck-referencesignore) exempts destinations that can never resolve. It exits non-zero on any broken link. Findings group by source spec and link:

```
Skill references:
  ✗ .agnostic-ai/skills/deploy/SKILL.md:8 links to missing references/setup.md
      targets: claude, codex
```

Each `references` entry in the JSON has `target`, `source` (omitted when unknown), `path`, `line`, and `destination`.

Then doctor prints:

| Block | What it shows | Counts as drift |
|-------|---------------|-----------|
| **Tracked despite ignored** | A generated path git tracks and ignores, with the `git rm --cached` command. | No |
| **MCP** | Whether each stdio `command:` resolves on PATH, with install hints. `url:`-only servers are skipped. | No |
| **Script divergence** | Basenames under `.agnostic-ai/scripts/<tool>/` whose bodies differ across tools, with the suggested path `.agnostic-ai/scripts/<basename>`. | Yes, not auto-fixable |
| **Unmanaged config** | Markdown and TOML config files without a provenance marker, grouped by the `import` source that adopts each. | No |
| **User-owned** | [`sync.unmanaged`](@/docs/configuration.md#syncunmanaged) entries, left out of Unmanaged config. | Never |

Subcommands run one check: `doctor config` (validate `agnostic-ai.yaml`), `doctor install` (which AI CLIs are on PATH), `doctor mcp` (resolve each MCP server's command binary).

## status

Show project configuration and sync state. Exits 0 even on drift; use `sync --check` or `doctor` in CI.

```bash
agnostic-ai status [--json]
```

```
Project: my-project
Layers:  project (.agnostic-ai/)
Specs:   3 rules, 1 agent, 2 skills
Targets: claude, cursor, copilot
Last sync: 2026-05-07 14:22 (5 files changed)
Drift:   in sync
```

| JSON key | Type | Description |
|-----|------|-------------|
| `project` | string | Project directory base name. |
| `layers` | array | Active spec layers, each with `name` and `path`. |
| `specs` | object | Counts: `agents`, `skills`, `rules`, `hooks`, `mcps`. |
| `targets` | array | Targets in `agnostic-ai.yaml`. |
| `last_sync` | string or null | RFC 3339 time of the last successful `sync`, from `.agnostic-ai/.sync-state`. Falls back to the newest generated file mtime; `null` (`unknown` in text) with no files. |
| `files_changed_last_sync` | number or null | Files written by the last sync; `null` under the mtime fallback. |
| `drift_files` | number | Emitted files that differ from what `sync` would produce. |

## revert

Undo a `sync --backup`. For every emitted file and entry-point file (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, `CONVENTIONS.md`, `.agnostic-ai/AGNOSTIC_AI.md`), `revert` restores `<path>.bak` and removes the .bak. It also restores a nested `CLAUDE.md` that sync deleted for Claude's scoped rules. Files without a `.bak` stay unless you pass `--force`.

| Flag | Description |
|------|-------------|
| `-t`, `--only`, `--except` | Select targets, as in [`sync`](#sync). |
| `--dry-run` | Report intended actions without touching disk |
| `--force` | Also delete emitted files that lack a `.bak`, including generated entry-point files and user files sharing their paths. |
| `--json` | Same schema as `sync --json`. Actions: `"restore"` (`.bak` applied), `"remove"` (deleted), `"preserve"` (no `.bak`, no `--force`), `"skip"` (already absent). |

Paths under [`sync.unmanaged`](@/docs/configuration.md#syncunmanaged) are never restored or removed.

## cleanup

Remove the `<path>.bak` backups `sync --backup` wrote for emitted paths. Unrelated `.bak` files are never touched.

```bash
agnostic-ai cleanup
agnostic-ai cleanup --dry-run   # preview deletions
```

## graph

Render the spec → target → file dependency graph. Read-only. Full guide in [graph](@/docs/graph.md).

```bash
agnostic-ai graph --format mermaid --target claude
```

| Flag | Description |
|------|-------------|
| `--format` | `text` (default, aligned matrix), `mermaid`, `dot`, `json`. |
| `--target` | Restrict to one target. |
| `--spec` | Restrict to one spec name. |
| `--kind` | Restrict to one kind: agent, skill, rule, hook, mcp, command. |

## why

Show an emitted file's adapter, source spec(s), `outputs.<target>.*` keys, and last sync time. Full guide in [why](@/docs/trace.md).

```bash
agnostic-ai why .claude/rules/no-console-log.md --format json
```

| Flag | Description |
|------|-------------|
| `--format` | `text` (default) or `json`. |

## packs

Manage shareable spec packs. Packs load as a layer below the project, so a project spec overrides a pack entry with the same name. Full guide in [packs](@/docs/packs.md).

```bash
agnostic-ai packs add github.com/chemaclass/go-rules@v1.2.0
agnostic-ai packs add ./path/to/pack
agnostic-ai packs list
agnostic-ai packs update [name]
agnostic-ai packs remove go-rules
```

## install-hook

Install a pre-commit hook that runs `sync --check`, or, with `--post-checkout`, a hook that regenerates tool files after a checkout. See [git hooks](@/docs/git-hooks.md).

```bash
agnostic-ai install-hook            # writes .git/hooks/pre-commit (local)
agnostic-ai install-hook --shared   # writes .githooks/ and sets core.hooksPath
agnostic-ai install-hook --global   # gates commits to a global home kept in git

agnostic-ai install-hook --post-checkout            # writes .git/hooks/post-checkout (local)
agnostic-ai install-hook --post-checkout --shared   # writes .githooks/post-checkout
```

An existing hook keeps its content and the checks go at its end. A hook that already holds them stays as it is. A hook that would stop before reaching them (no `sh` or `bash` shebang, an `exec`, or an unindented `exit`) is left alone, and the command prints the lines to add by hand.

- `--shared` writes `.githooks/<hook>` at the root of the main working tree, from any linked worktree. It stops when `core.hooksPath` already points elsewhere.
- `--global` is for the global home, which must be the root of its own git repository. The hook runs `lint --global --strict`, `validate --global`, and `sync --global --check`; the commit fails when any fails. In a linked worktree it skips `sync --global --check`. It stops when run anywhere else, when `core.hooksPath` points elsewhere, or when the hook still runs the project `sync --check`. Not with `--shared` or `--post-checkout`.
- `--post-checkout` runs `agnostic-ai sync -q` from the worktree root on a branch or worktree checkout (never a single-file checkout), when the binary and `agnostic-ai.yaml` are found. The hooks directory is shared across linked worktrees, so one install covers `git worktree add` everywhere.

## completion

Generate a shell completion script.

```bash
agnostic-ai completion bash > ~/.local/share/bash-completion/completions/agnostic-ai  # or /etc/bash_completion.d/
agnostic-ai completion zsh > "${fpath[1]}/_agnostic-ai"
agnostic-ai completion fish > ~/.config/fish/completions/agnostic-ai.fish
agnostic-ai completion powershell | Out-String | Invoke-Expression
```

Restart your shell or `source` the file. Completing `--target` reads `agnostic-ai.yaml` in the current directory, or offers every target. See `agnostic-ai completion <shell> --help`.

## upgrade

Upgrade the running binary to the latest release with the method it was installed with. `update` is an alias.

```bash
agnostic-ai upgrade --version v0.56.1
```

| Flag | Description |
|------|-------------|
| `--check` | Print install details and exit without changing anything. With `--version`, adds a `Requested:` line and downloads nothing. |
| `--version <tag>` | Install one release, downgrades included. The leading `v` is optional. Standalone binaries only; package-manager installs are told to pin through their manager. |
| `--run` | Accepted for compatibility; upgrading is the default. |

| Binary location | Upgrade |
|-----------------|---------|
| `*/Cellar/*`, `*/Caskroom/*`, `/opt/homebrew/*`, `/home/linuxbrew/.linuxbrew/*` | `brew update && brew upgrade --cask Chemaclass/tap/agnostic-ai` |
| `$GOBIN` or `$GOPATH/bin` (defaults to `$HOME/go/bin`) | `go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@latest` |
| `*\scoop\apps\*`, `*\scoop\shims\*` | `scoop update agnostic-ai` |
| `*\Microsoft\WinGet\*` | `winget upgrade Chemaclass.agnostic-ai` |
| `*/node_modules/*` | `npm install -g agnostic-ai@latest` |
| Standalone binary on macOS or Linux | Download the release, verify checksum and version, replace the binary atomically. |
| Standalone binary on Windows | Use the [PowerShell install script](@/docs/installation.md); Windows cannot replace a running executable. |

Scoop, WinGet, and `node_modules` markers match case-insensitively. `upgrade` also lists any other `agnostic-ai` on `PATH` that shadows the resolved executable.

## lsp

Start the Language Server on stdin/stdout. Point your editor at `agnostic-ai lsp` for spec files (`.agnostic-ai/**/*.md`, `*.mdc`). It pushes lint diagnostics on open and save.

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

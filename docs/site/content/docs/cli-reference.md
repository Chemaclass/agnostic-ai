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
| `-q, --quiet` | Errors only |
| `-v, --verbose` | Increase output verbosity (repeatable). Mutually exclusive with `--quiet`. |
| `--profile <file>` | Write a `runtime/pprof` CPU profile to `<file>` (or set `AGNOSTIC_AI_PROFILE`). Off by default. Read it with `go tool pprof <file>`. |

## init

Scaffold a project: `agnostic-ai.yaml` plus empty `agents/`, `skills/`, `rules/`, `hooks/`, `mcps/` under `.agnostic-ai/` by default. Errors if `agnostic-ai.yaml` exists.

```bash
agnostic-ai init specs --demo     # base dir specs/, example specs to start from
echo "claude,codex" | agnostic-ai init
```

| Flag | Description |
|------|-------------|
| `[dir]` | Base directory for the source folders (`.` for the legacy root layout). `agnostic-ai.yaml` gets matching `sources:` paths. |
| `--demo` | Seed example specs, one per source folder plus the `memory-curator` skill, so the first `sync` produces output. Never overwrites files. |
| `--preset <name>` | Seed starter specs for a stack: `go`, `ts-react`, `python`. Combines with `--demo` and `--all`. Never overwrites files. |
| `-a, --all` | Skip the target picker and enable every supported target. |
| `--gitignore` | On by default: generated outputs go into a managed `.gitignore` block, without a prompt when non-interactive. `--gitignore=false` commits them instead. |

Without `--all`, `init` takes targets from a TTY prompt (↑/↓ to move, space to toggle, enter to confirm), a comma-separated list on piped stdin, or every target when stdin is closed. `targets:` records them in canonical order; unknown names error and write nothing. The prompt and the [first-sync picker](#first-sync-target-picker) pre-tick every tool detected from a marker such as `.claude/`, `.codex/`, `.gemini/`, `.cursor/`, or `.github/copilot-instructions.md`.

## import

Translate an existing AI CLI configuration into agnostic specs, written into the `sources:` directories from `agnostic-ai.yaml`.

```bash
agnostic-ai import claude
agnostic-ai import claude codex   # in order; AGNOSTIC_AI.md comes from the last
agnostic-ai import all
agnostic-ai import claude codex --dry-run --diff   # review content and conflicts
```

`import all` imports every tool detected from its marker directory. A detected tool with no importer is skipped with a `skipping <tool>` line and does not fail the run.

| Flag | Effect |
|---|---|
| `--dry-run` | List every file the import would write, once each, without file bodies. Runs the import in a temporary copy of the project (without `.git`), so the list matches a real import. Writes nothing to the project. |
| `--diff` | With `--dry-run`, show each destination as `create`, `change`, or `unchanged`, the sources that wrote it, and a unified diff per created or changed file. Lists every destination two sources propose different content for, and the source a real import keeps (the last). Requires `--dry-run`. |

`--diff` runs the real importers in a temporary copy of the project (everything except `.git`), so each later source reads what the earlier ones wrote and the preview shows the exact bytes a real import leaves. The project, its native files, config, ignore files, and sync state stay untouched. A symlink that points outside the project is copied by content, so the preview cannot write through it. A conflict is reported, not resolved: the exit status stays 0.

- Writes only spec files under `sources:`, never `targets:` or other config. Run it after `init`; re-running overwrites by filename.
- A skill or agent spec already on disk keeps the frontmatter keys the source tool has nowhere to put. Cursor writes no `argument-hint` on a skill, so importing a synced `.cursor/skills/<name>/SKILL.md` updates the body and the keys cursor does write, and leaves `argument-hint` alone. Deleting a key the tool does write is read as deliberate and reaches the spec, so removing `model` from a Qoder agent removes it from the spec. Rules are exempt entirely: their frontmatter is rebuilt from the native file, so a scope dropped there drops from the spec.
- Each source mirrors its top-level instructions file to `.agnostic-ai/AGNOSTIC_AI.md`, so the last argument wins. A fenced `AGNOSTIC_AI.md` stays untouched when the imported entry point matches its rendered view; otherwise import overwrites it and warns that the fences were replaced.
- When another entry point holds different hand-written content (a distinct `AGENTS.md` alongside `CLAUDE.md`), import warns that `sync` would overwrite it. Merge it into `.agnostic-ai/AGNOSTIC_AI.md` first.
- `all` auto-detects every CLI present in the project and cannot combine with other sources.
- Valid sources: `claude`, `codex`, `cursor`, `aider`, `amp`, `warp`, `gemini`, `copilot`, `opencode`, `zed`, `antigravity`, `continue`, `cline`, `windsurf`, `junie`, `trae`, `kiro`, `crush`, `qoder`, `kilo`, `goose`, plus `all`. The targets `factory`, `openhands`, `jules`, and `augment` are emit-only.

Each target page lists what `import <target>` reads: [Claude](@/docs/targets/claude.md#import), [Codex](@/docs/targets/codex.md#import), [Cursor](@/docs/targets/cursor.md#import), [Cline](@/docs/targets/cline.md#import), [Windsurf](@/docs/targets/windsurf.md#import), [Continue](@/docs/targets/continue.md#import), [Junie](@/docs/targets/junie.md#import), [Kiro](@/docs/targets/kiro.md#import), [Crush](@/docs/targets/crush.md#import), [Amp](@/docs/targets/amp.md#import), [Zed](@/docs/targets/zed.md#import), [Warp](@/docs/targets/warp.md#import), [Antigravity](@/docs/targets/antigravity.md#import), [Copilot](@/docs/targets/copilot.md#import), [Trae](@/docs/targets/trae.md#import), [Goose](@/docs/targets/goose.md#import), [OpenHands](@/docs/targets/openhands.md#import), [Factory](@/docs/targets/factory.md#import), and [Kilo](@/docs/targets/kilo.md).

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

Load all specs, report parse errors, and print `loaded 12 entries. ok.` on success. Writes nothing. With no specs, stdout still says `loaded 0 entries. ok.` and stderr suggests `init` or `import`.

| Check | Reports |
|-------|------------|
| Hook events | A hook spec's `event:` missing, or supported by no configured target (with the supported list). |
| Orphaned kinds | Hook or MCP specs no enabled target consumes, one line per kind naming targets that would. |
| Declared sources | An explicit `sources.<kind>` path in `agnostic-ai.yaml` with no directory. Warning only. |
| Entry-point fences | A `::target` / `::targets` name in `.agnostic-ai/AGNOSTIC_AI.md` that is not a built-in target or listed in `targets` (external adapter), or that reads no entry-point file (`cursor`, or any target with `outputs.<target>.rules-file`). |

Hook events accepted per target:

| Target | Events |
|--------|--------|
| Claude | `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `SessionStart`, `SessionEnd`, `Stop`, `SubagentStop`, `PreCompact`, `PostCompact`, `Notification` |
| Codex | `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `SessionStart`, `SessionEnd`, `Stop`, `PreCompact`, `PostCompact` |
| Gemini | `BeforeTool`, `AfterTool`, `BeforeAgent`, `AfterAgent`, `Notification`, `SessionStart`, `SessionEnd`, `PreCompress`, `BeforeModel`, `AfterModel`, `BeforeToolSelection` |
| Cursor | `beforeShellExecution`, `afterShellExecution`, `beforeMCPExecution`, `afterMCPExecution`, `beforeReadFile`, `afterFileEdit`, `beforeSubmitPrompt`, `preToolUse`, `postToolUse`, `postToolUseFailure`, `sessionStart`, `sessionEnd`, `subagentStart`, `subagentStop`, `preCompact`, `stop`, `afterAgentResponse`, `afterAgentThought`, `beforeTabFileRead`, `afterTabFileEdit`, `workspaceOpen` |

## lint

Semantic checks beyond the schema. Exits 1 on error findings. `agnostic-ai lint --strict` treats warnings as errors too, for CI.

It flags empty specs, dead specs (kinds no enabled target supports), and hooks that set a matcher on an event that ignores it. Three codes catch specs that never reach a target:

| Code | Finding |
|------|---------|
| LINT003 | Two specs of one kind share a `name`; the loader keeps one body. Hooks sharing an event and matcher are fine: all run (`gofmt` and `vet` on one save). |
| LINT006 | Error. Frontmatter opens `---` and never closes, so the raw YAML is emitted as body. `validate` and `sync` both pass. |
| LINT008 | Error. A stdio MCP server lacks `command:`, or an `http`/`sse`/`ws` one lacks `url:`. Trae, Antigravity, and Windsurf drop it; Claude Code, Codex, Cursor, Gemini, Copilot, and the rest write an invalid server object. `x-<target>` cannot set either reserved field. |

LINT009 warns on a permission that approves more than it says. An `allow` rule such as `Bash(git * main)` puts `*` before the end of the command, so it also matches `git push --force main`. Claude Code warns about the same rule at startup; the targets that translate it widen it without a word. Write the exact value, or keep `*` at the end (`Bash(go test:*)`, `Bash(npm run *)`). `deny` and `ask` rules are skipped, since widening them only blocks or prompts more.

## list

Print all loaded specs as `kind<tab>name`. With no specs, the hint goes to stderr and stdout stays empty.

## new

Scaffold one agent, skill, rule, hook, or mcp spec with kind-appropriate frontmatter in that kind's `sources:` directory.

```bash
agnostic-ai new rule no-console-log     # → <rules>/no-console-log.md
agnostic-ai new hook fmt-on-save        # → <hooks>/fmt-on-save.yaml
agnostic-ai new rule payments-context --scope services/payments
```

| Flag | Description |
|------|-------------|
| `--scope <dir>` | `new rule` only. Create a flat rule for a project-relative directory, without global catch-all selectors. See [scoped context](@/docs/scoped-context.md). |

Errors if the destination exists. Names must be lowercase slugs (`[a-z0-9][a-z0-9-]*`), the form Cursor and Cline expect.

## explain

List every output file and section one spec contributes to, the reverse of the `<!-- source: ... -->` markers in merged documents. With `--file`, list the instructions configured for one source file instead. Writes nothing.

```bash
agnostic-ai explain rules/conventional-commits.md --json
```

| Flag | Description |
|------|-------------|
| `--json` | Stable schema for editor extensions and scripts. |

Contributions are grouped by configured target, plus a "would emit if enabled" list for inactive adapters. Entries are tagged `(full file)` or `(section "<name>")`.

```json
{"version": "1", "command": "explain", "spec": {"kind": "rule", "name": "...", "path": "..."},
 "contributions": [{"target": "...", "path": "...", "section": "...", "mode": "full|section"}],
 "would_emit_if_enabled": []}
```

### Explain a source file

Start from a project file instead of a spec. The report lists every instruction the target would read from the planned sync output, with its canonical source, output path, selector, and reason. Cursor is the only supported target.

```bash
agnostic-ai explain --file services/payments/handler.go --target cursor
```

| Flag | Description |
|------|-------------|
| `--file <path>` | Project file to inspect. The file does not have to exist. Cannot be combined with a spec or error code argument. |
| `--target <name>` | Required with `--file`. Must be a configured target. Other targets fail with an unsupported-target error. |

Each instruction gets one status:

| Status | Meaning |
|--------|---------|
| `always` | Loads with no file condition: `alwaysApply: true`, or the project-root `AGENTS.md`. |
| `match` | A `globs` pattern or a nested `AGENTS.md` directory covers the file. |
| `no-match` | A selector exists and misses the file. |
| `model-selected` | `alwaysApply: false` with a description and no globs. Cursor's agent decides. |
| `manual` | `alwaysApply: false` with neither. Loads only when `@`-mentioned. |
| `unknown` | Glob syntax Cursor does not document (braces, classes, negation), or unreadable frontmatter. |
| `excluded` | Target selection (`target`, `targets`, `target-exclude`) leaves the target out. |
| `not-emitted` | The rule targets Cursor but sync writes nothing for it, such as an unsupported scoped selector. |

A root `AGENTS.md` written for a peer target such as Codex reaches Cursor too, so a rule excluded from Cursor can still show up there. The report is configured applicability, not a record of the model's active context. Opening a file does not guarantee Cursor loads a matching instruction.

```json
{"version": "1", "command": "explain", "file": "...", "target": "cursor", "note": "...",
 "instructions": [{"status": "match", "source": "...", "output": "...", "selector": "...", "reason": "..."}]}
```

## compare

Compare how two built-in targets represent the project's agents and rule activation, before you switch or add a tool. Writes nothing.

```bash
agnostic-ai compare claude cursor
```

| Flag | Description |
|------|-------------|
| `--json` | Stable schema for scripts. |

Coverage is agent fields plus rule `scope`, `paths`, `globs`, and `alwaysApply`. The report says so on its first lines. Rules without those fields are left out, and other spec kinds are not compared.

Each spec emits in memory to both targets with the project's output options and `x-<target>` overrides, once as written and once per field with that field removed. The difference, plus the coverage notes the adapter raises, gives each field one result per target:

| Result | Meaning |
|---|---|
| `preserved` | Written under the same key with the same values. |
| `translated` | Written under another key or file, with rewritten values, or only in part. |
| `unsupported` | The target has no home for the field or the kind. |
| `excluded` | The spec never reaches the target: a target filter, an opt-in output, or a scope the target cannot express. |
| `unknown` | The emission gives no evidence either way. |

`preserved` describes the written file, not the tool's runtime behavior. A field marked `(differs)` has a different result on each target. Each result names the output paths or the reason, plus a `next:` step when one is known.

```json
{"version": "1", "command": "compare", "targets": ["claude", "cursor"], "coverage": "...", "caveat": "...",
 "specs": [{"kind": "agent", "name": "...", "path": "...", "fields": [{"field": "tools", "differs": true,
   "results": [{"target": "cursor", "status": "unsupported", "reason": "...", "next": "..."}]}]}],
 "fields": 6, "differences": 3}
```

Unknown targets, the same target twice, and invalid specs or config fail the command. External adapters are not accepted.

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

Emit per-target configs, for example `agnostic-ai sync --only claude,cursor`.

| Flag | Description |
|------|-------------|
| `-t, --target <list>` | Comma-separated targets (default: all in config) |
| `--only <list>` | Emit only these targets. Errors on unknown names. |
| `--except <list>` | Emit all configured targets except these. Errors on unknown names. `--only` and `--except` are mutually exclusive. |
| `--all` | Emit every configured target without the [first-sync picker](#first-sync-target-picker). |
| `--dry-run` | Print to stdout instead of writing files. Does not preview the orphan sweep. |
| `--check` | Exit non-zero if disk differs from emitted output. Writes nothing. |
| `--diff` | With `--check`, print a unified diff per drifted file (on-disk vs what sync would write). |
| `--format <human\|github>` | With `--check`: `human` (default) table or `github` Actions annotations. `--json` wins. |
| `--backup` | Copy each existing target file to `<path>.bak` before overwriting. Pair with `revert`. |
| `--gitignore <on\|off>` | Override `gitignore.enabled` for this run. |
| `--watch` | Stay running and re-emit on changes. Incompatible with `--check`. See [watch mode](#watch-mode). |
| `--watch-poll` | With `--watch`, force the 200 ms polling backend, for network mounts or container volumes where fsnotify misses events. |
| `--jobs <n>` | Targets emitted in parallel. `0` (default) is one worker per CPU; `1` is serial. See [parallel emission](#parallel-emission). |
| `--json` | Output as JSON. See [JSON output](#json-output). |
| `--global` | Install user-level instructions, unconditional rules, hooks, and skills from `$AGNOSTIC_AI_HOME` (default `~/.agnostic-ai/`) into 22 tools' user config, plus native agents for 18 targets. Works outside a project; never loads project config or packs. See [global output](@/docs/target-behavior.md#global-output). Accepts `--target`, `--only`, `--except` (unsupported targets fail with the supported list), `--dry-run`, `--check`, `--check --diff` (managed block only for instructions files), `--backup`; rejects `--watch`, `--plan`, `--json`, `--gitignore`, `--jobs` before any write. |

Paths listed under [`sync.unmanaged`](@/docs/configuration.md#syncunmanaged) are skipped and reported as `~ skip (unmanaged) <path>`.

**Orphan sweep.** `sync` records every file it writes in `.agnostic-ai/.sync-state`. A full run deletes files it no longer emits (a removed skill's folder with its `references/`) and prunes empty directories. It deletes only what it can prove it wrote: by provenance header, or by recorded content hash for verbatim copies (skill assets, targets with `provenance_header: false`). A file edited since the last sync is kept as `~ kept orphan <path>`. It counts as drift in `sync --check` and `doctor` until you delete it or list it under `sync.unmanaged`.

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

A drifting `--check` exits non-zero in every format. Stderr names the fix, `agnostic-ai sync`, and points at `agnostic-ai doctor` for a full diagnosis. To read the failure without a local re-run, add `--diff` or `--format=github`. `--diff` prints changed lines; a missing file gets a one-line create summary, and a large diff truncates with a count. `--format=github` emits `::error file=...,line=...::` annotations on the pull request. Neither has a config-file key.

### Watch mode

`sync --watch` watches `agnostic-ai.yaml`, `agnostic-ai.local.yaml`, every `sources.*` directory, `.agnostic-ai.local/`, and `.agnostic-ai/overlays/` (including `claude.settings.json` / `codex.config.toml`). It uses fsnotify with a 50 ms debounce, polls every 200 ms where fsnotify fails, and exits on Ctrl+C.

A spec change re-syncs only targets that emit that kind (a `claude`-scoped agent hits only claude), and the summary names them. Config and overlay edits, deletes, and renames re-sync everything.

### Parallel emission {#parallel-emission}

`sync` emits each target on its own worker; `--jobs <n>` bounds how many run at once, capped at the number of targets. There is no config-file key.

Output never depends on the value: files, summary counts, JSON, the `.gitignore` block, and capability warnings are byte-identical. For a shared path (the `AGENTS.md` pointer family) one target creates the file and the rest skip it, as in serial emission. Use `--jobs 1` only to debug or pin ordering.

### Profiling a slow sync

`--profile <file>` or `AGNOSTIC_AI_PROFILE=<file>` writes a CPU profile. `--verbose` adds wall time per target, as in `→ claude: 12 created, 3 updated, 0 unchanged in 42ms`. Under `--jobs > 1` these times overlap: read them as per-adapter cost, not a serial breakdown.

### JSON output

`sync --json` and `sync --check --json` share one schema:

| Field | Description |
|-------|-------------|
| `version` | Schema version, currently `"1"`. Breaking changes bump it. |
| `command` | `"sync"` or `"sync --check"`. |
| `writes` | Files written (`"create"`, `"update"`), orphans removed (`"delete"`), or, for `--check`, files needing attention (`"missing"`, `"stale"`, `"orphan"`). |
| `skipped` | Files already matching (`"skip"`), user-owned (`"unmanaged"`), or edited orphans kept (`"orphan"`). Empty for `--check`. |
| `errors` | Per-target errors with `target` and `message`. |

`writes` and `skipped` entries have `target`, `path`, `action` (strings), and `bytes` (number), as in `{"target": "claude", "path": "CLAUDE.md", "action": "create", "bytes": 1284}`.

## verify

Run a project-owned behavior check against each selected AI harness, for example `agnostic-ai verify --target codex` (omit `--target` for all configured targets). It first runs the target-scoped `sync --check`; a missing or stale generated file stops verification.

The command in [`verify.command`](@/docs/configuration.md#verify) runs once per target, without a shell, and receives this JSON document on stdin. Stdout and stderr pass through. A non-zero verifier exit stops the run and becomes the `agnostic-ai` exit code.

| Field | Meaning |
|------|---------|
| `version` | Contract version, currently `1`. |
| `target` | Target being verified, such as `"codex"`. |
| `configured_model` | Optional. Model from the rendered native config (portable Settings, target config, or imported overlay, by overlay precedence), not the model used at runtime. |
| `cli` | Optional. Detected CLI command, resolved path, and `--version` output. Omitted when the binary is missing or unproven; `version` is omitted when the command reports none. |
| `harness_fingerprint` | Stable SHA-256 digest of target-relevant canonical specs and rendered files. It identifies the harness; it is not an approval record, result cache, or score. |

The verifier owns datasets, judging model output, results, and baselines. agnostic-ai owns none of them.

## doctor

Report missing (never synced), stale (hand-edited or out of date), and orphaned (no longer generated, kept because edited) files. Read-only unless `--fix`. Exits non-zero on any drift.

| Flag | Description |
|------|-------------|
| `-t, --target <list>` | Comma-separated targets (default: all in config) |
| `--fix` | Write missing and stale files. Orphans stay for you to delete, so the exit stays non-zero while any remain. |
| `--backup` | With `--fix`, copy each existing file to `<path>.bak` before overwriting. |
| `--check-globs` | Flag rules whose `globs:` match no files. Off by default, since monorepos may ship globs for future paths. |
| `--check-references` | Flag relative Markdown links in generated skills whose file is missing on disk. Off by default. |
| `--json` | Drift report as JSON, same schema as `sync --check --json`. With `--check-references`, adds a `references` list. |

`--check-references` reads each Markdown document a selected target writes for its skills, including a skill a target flattens to one file. It resolves every inline link, image, and reference definition from the document's own directory and checks the file exists, gitignored outputs included. Code spans, code blocks, URLs, absolute paths, and `#fragment`-only links are skipped. A fragment on a file link is dropped: only the file is checked, not the heading. A document missing on disk is left to the drift report. The check writes nothing and exits non-zero on any broken link:

```
Skill references:
  ✗ claude: .claude/skills/deploy/SKILL.md:8 links to missing references/setup.md
      source: .agnostic-ai/skills/deploy/SKILL.md
```

Each `references` entry in the JSON has `target`, `source` (the canonical spec file, omitted when unknown), `path`, `line`, and `destination`. The key is absent without the flag.

Then doctor prints:

| Block | What it shows | Counts as drift |
|-------|---------------|-----------|
| **MCP** | Whether each stdio `command:` resolves on PATH, with install hints for `npx`, `uvx`, `python`, `docker`. `url:`-only servers are skipped. | No |
| **Script divergence** | Basenames under `.agnostic-ai/scripts/<tool>/` whose bodies differ by SHA-256 across tools, with sizes, short hashes, and the suggested path `.agnostic-ai/scripts/<basename>`. | Yes, not auto-fixable |
| **Unmanaged config** | Config files without a provenance marker (a pre-agnostic-ai `CLAUDE.md`, hand-written `.cursor/rules/*.mdc`), grouped by the `import` source that adopts each. Only markdown and TOML are scanned. | No |
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

Undo a `sync --backup`. For every emitted file and entry-point file (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, `CONVENTIONS.md`, `.agnostic-ai/AGNOSTIC_AI.md`), `revert` restores `<path>.bak` and removes the .bak. Files without a `.bak` stay unless you pass `--force`, so user files sharing a path with adapter output (helper scripts next to `SKILL.md`) survive.

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

Show an emitted file's adapter, source spec(s), `outputs.<target>.*` keys, and last sync time. Full guide in [why](@/docs/why.md).

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

Install a pre-commit hook that runs `sync --check`. See [git hooks](@/docs/git-hooks.md).

```bash
agnostic-ai install-hook            # writes .git/hooks/pre-commit (local)
agnostic-ai install-hook --shared   # writes .githooks/ and sets core.hooksPath
```

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

Upgrade the running binary to the latest release with the method it was installed with, for example `agnostic-ai upgrade --version v0.56.1`. `update` is an alias.

| Flag | Description |
|------|-------------|
| `--check` | Print install details and exit without changing anything. With `--version`, adds a `Requested:` line and downloads nothing. |
| `--version <tag>` | Install one release, downgrades included. The leading `v` is optional; non-release values are refused. Standalone binaries only; package-manager installs are told to pin through their manager. |
| `--run` | Accepted for compatibility; upgrading is the default. |

| Binary location | Upgrade |
|-----------------|---------|
| `*/Cellar/*`, `*/Caskroom/*`, `/opt/homebrew/*`, `/home/linuxbrew/.linuxbrew/*` | `brew update && brew upgrade --cask Chemaclass/tap/agnostic-ai` |
| `$GOBIN` or `$GOPATH/bin` (defaults to `$HOME/go/bin`) | `go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@latest` |
| `*\scoop\apps\*`, `*\scoop\shims\*` | `scoop update agnostic-ai` |
| `*\Microsoft\WinGet\*` | `winget upgrade Chemaclass.agnostic-ai` |
| `*/node_modules/*` | `npm install -g agnostic-ai@latest` |
| Standalone binary on macOS or Linux | Download the release archive, verify checksum and version, replace the binary atomically. |
| Standalone binary on Windows | Use the [PowerShell install script](@/docs/installation.md#windows); Windows cannot replace a running executable. |

Scoop, WinGet, and `node_modules` markers match case-insensitively.

`upgrade` lists any other `agnostic-ai` on `PATH` that shadows the resolved executable, such as an old `~/go/bin/agnostic-ai` or `/usr/local/bin/agnostic-ai` ahead of Homebrew.

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
| `AGNOSTIC_AI_HOME` | `~/.agnostic-ai` | Source root for `sync --global`. Project sync does not load it. See [global configuration](@/docs/configuration.md#global-configuration). |

## Config precedence

Last wins:

1. Built-in defaults (see [configuration](@/docs/configuration.md))
2. `agnostic-ai.yaml`
3. CLI flags (e.g. `-t`)

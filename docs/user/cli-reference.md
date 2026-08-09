# CLI reference

```
agnostic-ai [command] [flags]
```

## Global flags

| Flag | Description |
|------|-------------|
| `-h, --help` | Help for any command |
| `--version` | Print version and exit |
| `-q, --quiet` | Errors only |
| `-v, --verbose` | Increase output verbosity (repeatable). Mutually exclusive with `--quiet`. |
| `--profile <file>` | Write a `runtime/pprof` CPU profile of the run to `<file>` (or set `AGNOSTIC_AI_PROFILE`). Off by default; stdlib profiler only. Read it with `go tool pprof <file>`. |

## init

Scaffold a project: `agnostic-ai.yaml` plus empty `agents/`, `skills/`, `rules/`, `hooks/`, `mcps/`. Errors if `agnostic-ai.yaml` exists.

```bash
agnostic-ai init                  # prompt for targets when stdin is a TTY, base dir .agnostic-ai/
agnostic-ai init --all            # skip the prompt, enable every supported target
agnostic-ai init specs            # custom base: specs/{agents,skills,...}/
agnostic-ai init .                # legacy root-level layout
agnostic-ai init --demo           # seed each source folder with one example spec
agnostic-ai init --preset go      # seed idiomatic specs for a stack (go, ts-react, python)
echo "claude,codex" | agnostic-ai init   # non-TTY: pipe a comma-separated target list
```

| Flag | Description |
|------|-------------|
| `--demo` | Seed each source folder with one minimal example spec so a fresh project produces real output on the first `sync`. Never overwrites existing files. |
| `--preset <name>` | Seed stack-flavored starter specs: `go`, `ts-react`, `python`. Composes with `--demo` and `--all`. Errors on unknown names with the available list. Never overwrites existing files, so re-running on a partial tree is safe. |
| `-a, --all` | Skip the target picker and enable every supported target. Useful for CI or scripted scaffolds. |

Target selection by default: multi-select prompt on a TTY, piped comma-separated list on a pipe, all targets when stdin is closed.

The positional `[dir]` arg sets the base directory for the source folders. The generated `agnostic-ai.yaml` writes matching `sources:` paths.

Pull in an existing AI CLI config after init with `import`.

## import

Translate an existing AI CLI configuration into agnostic specs. Reads `sources:` from `agnostic-ai.yaml`, then writes specs into those directories.

```bash
agnostic-ai import claude         # CLAUDE.md, .claude/{agents,skills,commands,settings.json,rules}, .mcp.json
agnostic-ai import codex          # AGENTS.md (root + nested), .codex/{prompts,config.toml}
agnostic-ai import claude codex   # multiple sources; AGNOSTIC_AI.md reflects the last (last-wins)
agnostic-ai import cursor         # .cursor/rules/, .cursor/agents/, .cursor/skills/, .cursor/commands/
agnostic-ai import cline          # .cline/rules/, .cline/agents/ (or legacy .clinerules/)
agnostic-ai import windsurf       # .devin/rules/ (or legacy .windsurf/rules/)
agnostic-ai import continue       # .continue/rules/
```

- Touches only spec files under `sources:`. Never modifies `targets:` or other config.
- Re-running overwrites by filename. Run after `init`, in the same project root.
- Multiple sources import in order. Each mirrors its target's top-level instructions file to `.agnostic-ai/AGNOSTIC_AI.md`; with multiple sources, the last argument wins.
- When another target's entry-point exists with different hand-authored content (e.g. a distinct `AGENTS.md` alongside `CLAUDE.md`), import warns that it holds unique content the next `sync` would overwrite. Merge that content into `.agnostic-ai/AGNOSTIC_AI.md` before syncing to keep it.
- `all` cannot combine with other sources. It auto-detects every CLI present in the project.
- Valid sources: `claude`, `codex`, `cursor`, `aider`, `amp`, `warp`, `gemini`, `copilot`, `opencode`, `zed`, `antigravity`, `continue`, `cline`, `windsurf`, `junie`, `trae`, `kiro`, `crush`, `qoder`, plus `all`. The newest targets `kilo`, `factory`, `openhands`, `jules`, `goose`, and `augment` are emit-only for now; every other emitted target can also be imported.

`import claude`:

| Source | Becomes |
|--------|---------|
| `.claude/rules/**/*.md` (preferred) | `<rules>/<sub>/<name>.md` (byte-identical copy, nested subdirectories preserved) |
| `CLAUDE.md` (split on `## headings`) | `<rules>/<slug>.md` per section (only when `.claude/rules/` is absent) |
| `CLAUDE.md` (no headings) | single `<rules>/<projectname>.md` (only when `.claude/rules/` is absent) |
| `CLAUDE.md` (any form) | `.agnostic-ai/AGNOSTIC_AI.md` (byte-identical copy) |
| `.claude/agents/*.md` | `<agents>/<name>.md` (byte-identical copy) |
| `.claude/skills/<name>/SKILL.md` | `<skills>/<name>/SKILL.md` |
| `.claude/commands/*.md` | `<commands>/<name>.md` (byte-identical copy) |
| `.claude/settings.json` hooks | `<hooks>/<event>[-<matcher-slug>]-<hash8>.yaml` (filename derived from event, matcher, and commands so re-imports converge on the same path) |
| `.claude/settings.json` non-hook keys | `.agnostic-ai/overlays/claude.settings.json` (captures statusLine, enabledPlugins, model overrides, and any other top-level keys so `sync -t claude` reproduces the full settings.json after `.claude/` is wiped) |
| `.mcp.json` (`mcpServers.<name>`) | `<mcps>/<name>.yaml` (one spec per server; round-trips to every MCP-aware target on the next `sync`) |

When `.claude/rules/` exists (even if empty), slicing `CLAUDE.md` is skipped so the on-disk rules layout is the single source of truth for rule files. `.agnostic-ai/AGNOSTIC_AI.md` is still written from `CLAUDE.md` to keep a CLI-agnostic top-level instructions file alongside `CLAUDE.md` / `AGENTS.md` / `GEMINI.md`.

`import codex` walks the project for `AGENTS.md` at any depth and reads the rest of the Codex tree:

| Source | Becomes |
|--------|---------|
| `AGENTS.md` (split on `## headings`) | `<rules>/<slug>.md` per section |
| `AGENTS.md` (no headings) | single `<rules>/<projectname>.md` |
| `<dir>/AGENTS.md` (nested) | `<rules>/<slug>.md` with inferred `globs: <dir>/**` |
| `## Conventions` / `## Agents` / `## Skills` wrapper sections | unwrapped: their `### children` become the rules |
| Single-line italic (`_text_`) immediately under a rule heading | extracted into the rule's `description` (and removed from the body) |
| `.codex/agents/*.toml` and `.agents/agents/*.toml` | `<agents>/<name>.md` |
| `.agents/skills/<name>/SKILL.md` (+ `agents/openai.yaml`, asset folders) | `<skills>/<name>/SKILL.md` (+ nested assets, exec bits preserved) |
| `.codex/config.toml` `[[hooks.<event>]]` | `<hooks>/<event>-<hash8>.yaml` (one spec per entry) |
| `.codex/config.toml` `[mcp_servers.<name>]` | `<mcps>/<name>.yaml` |
| `.codex/config.toml` remaining keys (model, sandbox, approval_policy, notify, `[history]`, `[profiles.*]`, `[model_providers.*]`, …) | `.agnostic-ai/overlays/codex.config.toml` (`hooks` + `mcp_servers` stripped; the codex emitter prepends this overlay before the spec-derived sections on each sync) |
| `.codex/prompts/*.md` | `<commands>/<name>.md` (byte-identical copy) |

Slug collisions across files are deduplicated (`style.md`, `style-2.md`). The walk skips hidden directories, the configured source directories, `node_modules/`, and `vendor/`.

**Overlay precedence.**

- **Codex**: when both the overlay and `outputs.codex.config.*` declare the same key (`model`, `sandbox`, `approval_policy`, `notify`, `[history]`, `[profiles.*]`, `[model_providers.*]`), the overlay wins and the first-class key is dropped to avoid a TOML duplicate-key error.
- **Claude**: the opposite. `outputs.claude.settings.*` overrides the overlay for any key it declares, so `agnostic-ai.yaml` changes always reach `.claude/settings.json`. Re-run `import claude` to refresh the overlay after hand-editing `.claude/settings.json`.
- Keys in only one place pass through unchanged either way.

`import cursor`:

| Source | Becomes |
|--------|---------|
| `.cursor/rules/<name>.mdc` | `<rules>/<name>.md` with frontmatter (`description`, `globs`, `alwaysApply`, plus any custom keys) preserved verbatim |
| `.cursor/rules/<sub>/<name>.mdc` | `<rules>/<sub>/<name>.md`, nested subdirectories preserved |
| (no `name:` in frontmatter) | `name:` injected from the filename |
| `.cursor/agents/<name>.md` | `<agents>/<name>.md` (byte-identical copy, provenance header stripped) |
| `.cursor/skills/<name>/` | `<skills>/<name>/`, full folder tree (SKILL.md + bundled assets) copied byte-for-byte |
| `.cursor/commands/<name>.md` | `<commands>/<name>.md` (byte-identical copy, provenance header stripped) |

Reads `.cursor/rules/**` recursively, so nested rule directories are imported too. Round-trips cleanly: a later `sync` regenerates equivalent `.cursor/rules/*.mdc`, skill folders, and command files.

`import cline`, `import windsurf`, `import trae`, `import qoder`, and `import continue` read the matching rules directory (`.cline/rules/` with `.clinerules/` fallback, `.devin/rules/` with `.windsurf/rules/` fallback, `.trae/rules/`, `.qoder/rules/`, `.continue/rules/`) and reclassify each file by filename prefix:

| Source filename | Becomes |
|-----------------|---------|
| `agent-<name>.md` | `<agents>/<name>.md` |
| `skill-<name>.md` | `<skills>/<name>.md` |
| `<name>.md` | `<rules>/<name>.md` |
| `<scope>/<file>.md` | nested under the same `<scope>` in the destination |

The leading `# <heading>` block (which the adapter prepends on emit) is stripped on import, and a minimal `name:` frontmatter is injected.

`import cline` additionally reconstructs agents from `.cline/agents/<name>.md`, Cline's native per-agent directory (target-audit 2026-08-01, #534): each file copies byte-for-byte minus the provenance header (no `agent-` prefix to strip, and no synthesized heading, since sync no longer writes one there). The `agent-<name>.md` prefix in the table above only fires when a project still carries the pre-migration `.clinerules/` layout, where rules and agents shared one directory.

`import junie` reads rules and agents from `.junie/AGENTS.md`'s sentinel-marked Rules and Agents blocks: the file Junie's guidelines lookup opens, since that lookup is strict precedence and `sync` always writes it first (target-audit 2026-08-08, #552). It also reconstructs skills from `.junie/skills/<name>/SKILL.md`, Junie's native Agent Skills folder tree (target-audit 2026-08-01): bundled sibling assets copy byte-for-byte, same as `import cursor`'s and `import codex`'s skill-folder handling above. A project synced by an agnostic-ai version before that fix still has real content flattened under `.junie/rules/` (reclassified by filename prefix, the same scheme as the group above); that directory takes precedence over `.junie/AGENTS.md` when it still exists. A legacy flat `.junie/rules/skill-<name>.md` (from a project synced before Native Agent Skills shipped) still imports as a skill too.

`import kiro` reverses the Kiro steering layout. Steering files are flat under `.kiro/steering/` and carry a frontmatter-first `inclusion:` block; the filename prefix picks the kind:

| Source | Becomes |
|--------|---------|
| `.kiro/steering/<name>.md` (`inclusion: always`) | `<rules>/<name>.md` (unscoped rule) |
| `.kiro/steering/<name>.md` (`inclusion: fileMatch` + `fileMatchPattern`) | `<rules>/<name>.md` with `globs: <fileMatchPattern>` |
| `.kiro/steering/agent-<name>.md` | `<agents>/<name>.md` |
| `.kiro/steering/skill-<name>.md` | `<skills>/<name>/SKILL.md` |
| `.kiro/settings/mcp.json` (`mcpServers.<name>`) | `<mcps>/<name>.yaml` |
| `AGENTS.md` | `.agnostic-ai/AGNOSTIC_AI.md` |

Lossy on round-trip (Kiro's emit cannot carry these, so the reconstructed spec drops them without changing Kiro's output): a rule's source-layout scope collapses into an equivalent `globs:`, a steering agent keeps only its body, and a steering skill keeps only its SKILL.md (bundled sibling assets are flattened on emit).

`import crush` reverses the Crush layout. Crush has no per-rule directory, so rules ride inside the shared `AGENTS.md`:

| Source | Becomes |
|--------|---------|
| `AGENTS.md` inlined `## Rules` block (`### <name>` children) | `<rules>/<name>.md` per rule |
| `.agents/skills/<name>/SKILL.md` (+ bundled assets) | `<skills>/<name>/SKILL.md` (folder copied byte-for-byte) |
| `crush.json` (`mcp.<name>`, `type: stdio` / `type: http`) | `<mcps>/<name>.yaml` |
| `AGENTS.md` | `.agnostic-ai/AGNOSTIC_AI.md` |

Lossy on round-trip: rules reach Crush only through the inlined block, which carries no `globs`/scope, so rule scoping does not round-trip (Crush's output is unaffected either way).

## validate

Load all specs, report parse errors. Writes nothing.

```bash
agnostic-ai validate
```

```
loaded 12 entries. ok.
```

When no specs load, `validate` prints a stderr hint suggesting `init` or `import`. Stdout still reports `loaded 0 entries. ok.` so scripts that read stdout do not break.

Once specs load, `validate` runs three native-support checks:

- **Hook events.** Each hook spec's `event:` must be in the union of every configured target's supported list. Unknown values report inline with the supported list. Per-target sets:
  - Claude: `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `SessionStart`, `SessionEnd`, `Stop`, `SubagentStop`, `PreCompact`, `PostCompact`, `Notification`.
  - Codex: `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `SessionStart`, `SessionEnd`, `Stop`, `PreCompact`, `PostCompact`.
  - Gemini: `BeforeTool`, `AfterTool`, `BeforeAgent`, `AfterAgent`, `Notification`, `SessionStart`, `SessionEnd`, `PreCompress`, `BeforeModel`, `AfterModel`, `BeforeToolSelection`.
  - Cursor: `beforeShellExecution`, `afterShellExecution`, `beforeMCPExecution`, `afterMCPExecution`, `beforeReadFile`, `afterFileEdit`, `beforeSubmitPrompt`, `preToolUse`, `postToolUse`, `postToolUseFailure`, `sessionStart`, `sessionEnd`, `subagentStart`, `subagentStop`, `preCompact`, `stop`, `afterAgentResponse`, `afterAgentThought`, `beforeTabFileRead`, `afterTabFileEdit`, `workspaceOpen`.
- **Missing fields.** Hook specs missing `event:` are flagged.
- **Orphaned kinds.** When a project has hook or MCP specs but no enabled target consumes them, validate prints one summary line per orphaned kind with the targets that would unblock it.
- **Declared sources.** Each `sources.<kind>` path set in `agnostic-ai.yaml` whose directory is missing is flagged, so a config cannot advertise coverage it never delivers. Undeclared kinds (using the default) are not flagged. Warning only. (#444)

## list

Print all loaded specs as `kind<tab>name`.

```bash
agnostic-ai list
```

When no specs load, `list` prints the same stderr hint and keeps stdout empty so pipes stay clean.

## new

Scaffold a single spec file with kind-appropriate frontmatter under the source directory for that kind. Replaces "copy from `--demo` and edit" for one new agent, skill, rule, hook, or MCP.

```bash
agnostic-ai new rule no-console-log     # → <rules>/no-console-log.md
agnostic-ai new agent code-reviewer     # → <agents>/code-reviewer.md
agnostic-ai new skill yaml-validator    # → <skills>/yaml-validator.md
agnostic-ai new hook fmt-on-save        # → <hooks>/fmt-on-save.yaml
agnostic-ai new mcp filesystem          # → <mcps>/filesystem.yaml
```

Errors if the destination exists. Names must be lowercase slugs (`[a-z0-9][a-z0-9-]*`), the form Cursor and Cline expect. Honors `sources:` from `agnostic-ai.yaml`, so a project under `specs/` lands files there.

## explain

Reverse provenance: list every output file and section one spec contributes to. Pairs with the `<!-- source: ... -->` forward markers adapters write into merged documents.

```bash
agnostic-ai explain rules/conventional-commits.md
agnostic-ai explain rules/conventional-commits.md --json
```

| Flag | Description |
|------|-------------|
| `--json` | Stable schema for editor extensions and scripts. |

Output groups contributions by configured target (those in `agnostic-ai.yaml`), plus a "would emit if enabled" list for adapters that exist but are not active. Each entry tags itself `(full file)` (the spec owns the file) or `(section "<name>")` (the spec contributes one section to a merged document). Writes nothing.

JSON envelope:

```json
{
  "version": "1",
  "command": "explain",
  "spec": { "kind": "rule", "name": "...", "path": "..." },
  "contributions":         [{ "target": "...", "path": "...", "section": "...", "mode": "full|section" }],
  "would_emit_if_enabled": [{ "target": "...", "path": "...", "section": "...", "mode": "full|section" }]
}
```

## render

Print the emission for a single spec to stdout, per target. Iterate on one spec and see what each adapter produces, without writing files or rerunning a full sync.

```bash
agnostic-ai render rules/no-console-log.md --target cursor
agnostic-ai render rules/no-console-log.md --target claude,codex
agnostic-ai render rules/no-console-log.md         # all configured targets
```

| Flag | Description |
|------|-------------|
| `-t, --target <list>` | Target(s) to render. Repeat or comma-separate. Default: every target in `agnostic-ai.yaml`. |

Output format is `# target: <name> — <output path>` followed by the file body, one block per emitted file. Targets that produce no output for the spec's kind print a short note. Writes nothing; pair with `sync` once the output looks right.

## sync

Emit per-target configs.

```bash
agnostic-ai sync [flags]
```

| Flag | Description |
|------|-------------|
| `-t, --target <list>` | Comma-separated targets (default: all in config) |
| `--only <list>` | Emit only these targets (comma-separated). Mutually exclusive with `--except`. Errors on unknown names. |
| `--except <list>` | Emit all configured targets except these (comma-separated). Mutually exclusive with `--only`. Errors on unknown names. |
| `--dry-run` | Print to stdout instead of writing files |
| `--check` | Compare emitted output to disk; exit non-zero on drift. Writes nothing. |
| `--diff` | With `--check`, print a unified diff per drifted file (on-disk vs what sync would write). Off by default so CI logs stay lean. Large diffs are truncated with a summary line. |
| `--format <human\|github>` | With `--check`, the drift report format. `human` (default) is the per-target table; `github` emits GitHub Actions `::error` annotations so drift surfaces inline on the pull request. `--json` still selects JSON and takes precedence. |
| `--backup` | Copy each existing target file to `<path>.bak` before overwriting. Pair with `revert` to restore. |
| `--gitignore <on\|off>` | Override `gitignore.enabled` for this run. |
| `--watch` | Keep the process alive and re-emit on spec, config, or overlay changes. Watched roots: `agnostic-ai.yaml` + `agnostic-ai.local.yaml`, every `sources.*` directory, `.agnostic-ai.local/`, and `.agnostic-ai/overlays/` (so a hand-edit to `claude.settings.json` / `codex.config.toml` re-emits). Uses OS file events (fsnotify) with a 50 ms debounce; falls back to a 200 ms poll where fsnotify fails. Re-emits incrementally: a spec change re-syncs only the targets that emit that kind (a rule hits every rule-emitting target; a `claude`-scoped agent hits only claude), and the summary names what re-emitted. Config and overlay edits, deletes, and renames trigger a full re-sync. Ctrl+C exits cleanly. Incompatible with `--check`. |
| `--watch-poll` | Force the polling backend (200 ms) even when fsnotify is available. Use on network mounts or container volumes where fsnotify misses events. Requires `--watch`. |
| `--all` | Emit every configured target without the first-sync picker. Useful for ad-hoc full emission or scripted runs that bypass prompts. |
| `--jobs <n>` | Number of targets to emit in parallel. `0` (default) uses one worker per CPU; `1` forces serial emission. Output (files, summary, JSON, gitignore, warnings) is byte-identical regardless of the value, so lower it only to debug or pin ordering. |
| `--json` | Output as JSON instead of plain text. Stable schema; breaking changes bump `version`. |

### Profiling a slow sync

Two opt-in hooks show where `sync` spends its time.

- `--profile <file>` (or `AGNOSTIC_AI_PROFILE=<file>`) writes a `runtime/pprof` CPU profile of the whole run. Open it with `go tool pprof <file>`. Off by default, stdlib profiler only.
- `--verbose` appends per-target wall time to each target line, so cost attributes to a specific adapter.

```
→ claude: 12 created, 3 updated, 0 unchanged in 42ms
✓ synced 1 target · 3 files · 44ms
```

The top summary line keeps the run total. Per-target times are measured around each emit, so under `--jobs > 1` they overlap: read them as per-adapter cost, not a serial breakdown.

### First-sync target picker

On the first `sync` (no `.agnostic-ai/.sync-state` yet) where the config still lists every supported target, `sync` opens an interactive multi-select to narrow the list. The selection persists to `agnostic-ai.yaml` so future syncs skip the prompt.

- **TTY:** multi-select widget (same UI as `init`'s default prompt).
- **Piped stdin:** `echo "claude,codex" | agnostic-ai sync` selects and persists without a prompt.
- **Non-TTY, no piped data (CI):** silent fallback. Emits every configured target. CI keeps working.
- **Bypass:** pass `--all`, `-t`, `--only`, or `--except` to skip the picker for one run.

```bash
agnostic-ai sync                       # first run: prompt for targets; later: all in config
agnostic-ai sync --all                 # skip the first-sync picker; emit every configured target
agnostic-ai sync -t claude             # only claude (legacy form)
agnostic-ai sync --only claude,cursor  # only claude and cursor
agnostic-ai sync --except codex        # all configured targets except codex
agnostic-ai sync --dry-run             # preview
agnostic-ai sync --check               # CI gate: fail if outputs are stale
agnostic-ai sync --check --diff        # show a unified diff of every drifted file
agnostic-ai sync --check --format=github  # GitHub Actions annotations, inline on the PR
agnostic-ai sync --backup              # leave a .bak trail for revert
agnostic-ai sync --jobs 1              # force serial emission (default: one worker per CPU)
agnostic-ai sync --watch               # re-emit on spec changes; Ctrl+C exits
agnostic-ai sync --json                # machine-readable output
agnostic-ai sync --check --json        # machine-readable drift report
```

`--only` and `--except` validate names against the configured targets and error on unknown names (no silent skip).

### Reading a failing `--check`

A drifting `--check` exits non-zero and, on stderr, prints the one command that reconciles it: `agnostic-ai sync`. The error line itself points at `agnostic-ai doctor` for a full diagnosis. Two flags make the failure self-explanatory without a local re-run:

- `--diff` prints a unified diff per drifted file (on-disk content vs what sync would write), so you see the exact changed lines. Missing files show a one-line create summary; large diffs truncate with a count.
- `--format=github` swaps the human table for GitHub Actions `::error file=...,line=...::` annotations. Drop it in a CI step and drift surfaces inline on the pull request. `--json` still selects the machine-readable report and wins if both are set.

Exit codes are unchanged: zero when in sync, non-zero on drift, in every format.

**JSON output schema (version 1):**

```json
{
  "version": "1",
  "command": "sync",
  "writes": [
    {"target": "claude", "path": "CLAUDE.md", "action": "create", "bytes": 1284},
    {"target": "cursor", "path": ".cursor/rules/foo.mdc", "action": "update", "bytes": 312}
  ],
  "skipped": [
    {"target": "claude", "path": "CLAUDE.md", "action": "skip", "bytes": 1284}
  ],
  "errors": []
}
```

| Field | Type | Description |
|-------|------|-------------|
| `version` | string | Schema version. Currently `"1"`. |
| `command` | string | Command that produced the output (`"sync"` or `"sync --check"`). |
| `writes` | array | Files written (action `"create"` or `"update"`) or, for `--check`, files needing writing (action `"missing"` or `"stale"`). |
| `skipped` | array | Files whose on-disk content already matched (action `"skip"`). Empty for `--check`. |
| `errors` | array | Per-target errors with `target` and `message` fields. |

Each entry in `writes` and `skipped` has: `target` (string), `path` (string), `action` (string), `bytes` (number).

## revert

Undo a previous sync. For every file an adapter would emit, plus the entry-point files sync distributes (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, `CONVENTIONS.md`, `.agnostic-ai/AGNOSTIC_AI.md`), restores `<path>.bak` when present (and removes the .bak). With no `.bak`, the file is left in place by default so user-authored content sharing a path with adapter output (helper scripts next to `SKILL.md`, templates inside a propagated skill folder) is not deleted. Pass `--force` to delete those unbacked files, including the generated entry-point files.

```bash
agnostic-ai revert [flags]
```

| Flag | Description |
|------|-------------|
| `-t, --target <list>` | Comma-separated targets (default: all in config) |
| `--only <list>` | Revert only these targets (comma-separated). Mutually exclusive with `--except`. Errors on unknown names. |
| `--except <list>` | Revert all configured targets except these (comma-separated). Mutually exclusive with `--only`. Errors on unknown names. |
| `--dry-run` | Report intended actions without touching disk |
| `--force` | Also delete adapter-emitted files that lack a `.bak`. Use with care: removes user-authored files sharing a path with adapter output. |
| `--json` | Output as JSON instead of plain text. |

```bash
agnostic-ai sync --backup              # 1. snapshot existing files
# ...edits or experiments...
agnostic-ai revert                     # 2. restore .bak files, leave unbacked alone
agnostic-ai revert --force             # 3. also delete generated files without .bak
agnostic-ai revert --only claude       # roll back only claude
agnostic-ai revert --except codex      # roll back everything except codex
agnostic-ai revert --json              # machine-readable output
```

`--json` uses the same schema as `sync --json`. Actions are `"restore"` (`.bak` applied), `"remove"` (file deleted), `"preserve"` (kept because no `.bak` and no `--force`), or `"skip"` (file already absent).

Without a prior `--backup`, `revert` is a no-op unless `--force` is passed. This protects helper files from accidental deletion.

## doctor

Diagnose drift between source specs and emitted artifacts. Reports missing files (never synced) and stale files (hand-edited or out of date). Exits non-zero on any drift.

```bash
agnostic-ai doctor                  # all targets in config (read-only)
agnostic-ai doctor -t claude        # subset
agnostic-ai doctor --fix            # reconcile drift in place
agnostic-ai doctor --fix --backup   # save .bak of hand-edits before overwrite
```

| Flag | Description |
|------|-------------|
| `-t, --target <list>` | Comma-separated targets (default: all in config) |
| `--fix` | Write missing/stale files. In-sync files untouched. |
| `--backup` | With `--fix`, copy each existing file to `<path>.bak` before overwriting. |
| `--check-globs` | Flag rules whose `globs:` pattern matches no files in the working tree. Off by default (some monorepos ship globs for paths added later). |
| `--json` | Output drift report as JSON. Same schema as `sync --check --json`. |

Use the no-flag form as a CI gate alongside `sync --check`, or after rebases to spot files the merge resolved manually. Use `--fix` for an interactive cleanup pass. Pair with `--backup` when reconciling files you may have hand-edited.

After the drift report, doctor prints an **MCP block**: each MCP spec's stdio `command:` and whether it resolves on PATH. Missing common commands (`npx`, `uvx`, `python`, `docker`) include an inline install hint. HTTP/SSE servers (no command, only `url:`) skip the check. Advisory only: a missing binary does not change doctor's exit code.

doctor also prints an **Unmanaged config block**: agentic config files on disk that carry no provenance marker (a pre-agnostic-ai `CLAUDE.md`, hand-written `.cursor/rules/*.mdc`, ...), grouped by the `import` source that adopts each. Only header-bearing formats (markdown, TOML) are scanned; JSON config is merge-managed and covered by the drift block. Advisory: does not change the exit code.

Subcommands run a single check in isolation: `doctor config` (validate `agnostic-ai.yaml`), `doctor install` (which AI CLIs are on PATH), `doctor mcp` (resolve each MCP server's command binary).

After the MCP block, doctor walks `.agnostic-ai/scripts/<tool>/` per tool and groups files by basename. When the same basename exists under two or more tools with different SHA-256 bodies, it prints one finding per divergent script (sizes + truncated hashes per variant) and the suggested consolidation path `.agnostic-ai/scripts/<basename>`. Divergence counts as drift and contributes to a non-zero exit, since it usually means an unnoticed import diff between tools. Not auto-fixable: choosing the winning body needs human judgement.

## status

Show project configuration and current sync state. Exits 0 even on drift. Use `sync --check` or `doctor` as CI gates.

```bash
agnostic-ai status          # human-readable output
agnostic-ai status --json   # machine-readable JSON (for editor extensions, dashboards)
```

| Flag | Description |
|------|-------------|
| `--json` | Output as JSON instead of plain text. |

**Human-readable output:**

```
Project: my-project
Layers:  project (.agnostic-ai/)
Specs:   3 rules, 1 agent, 2 skills
Targets: claude, cursor, copilot
Last sync: 2026-05-07 14:22 (5 files changed)
Drift:   in sync
```

**JSON output keys:**

| Key | Type | Description |
|-----|------|-------------|
| `project` | string | Base name of the project directory. |
| `layers` | array | Active spec layers. Each has `name` and `path`. |
| `specs` | object | Counts: `agents`, `skills`, `rules`, `hooks`, `mcps`. |
| `targets` | array | Targets listed in `agnostic-ai.yaml`. |
| `last_sync` | string or null | RFC 3339 timestamp of the last successful `sync`, or `null` if unknown. |
| `files_changed_last_sync` | number or null | Files written during the last sync, or `null` when the timestamp came from an mtime fallback. |
| `drift_files` | number | Count of emitted files whose on-disk content differs from what `sync` would produce. |

**State file:** Each successful `sync` writes `.agnostic-ai/.sync-state` (JSON) with the timestamp and file count. When absent, `status` falls back to the newest mtime of generated files. With no files yet, the timestamp is `unknown`.

## completion

Generate a shell completion script and install it for your shell.

```bash
# Bash, system-wide (may need sudo)
agnostic-ai completion bash > /etc/bash_completion.d/agnostic-ai

# Bash, current user only
agnostic-ai completion bash > ~/.local/share/bash-completion/completions/agnostic-ai

# Zsh
agnostic-ai completion zsh > "${fpath[1]}/_agnostic-ai"

# Fish
agnostic-ai completion fish > ~/.config/fish/completions/agnostic-ai.fish

# PowerShell
agnostic-ai completion powershell | Out-String | Invoke-Expression
```

After installing, restart your shell (or `source` the file). Tab-completing `--target` reads `agnostic-ai.yaml` in the current directory and falls back to the full default target list when no config is found.

Run `agnostic-ai completion <shell> --help` for shell-specific setup instructions.

## upgrade

Detect how the running binary was installed and report (or run) the matching upgrade command. Does not self-replace the binary: package managers stay in charge of installed versions.

```bash
agnostic-ai upgrade           # print the upgrade command for the current install
agnostic-ai upgrade --check   # diagnose install location + PATH shadowing, exit
agnostic-ai upgrade --run     # exec the detected upgrade command
```

Detection:

- `*/Cellar/*`, `*/Caskroom/*`, `/opt/homebrew/*`, `/home/linuxbrew/.linuxbrew/*` → `brew update && brew upgrade --cask Chemaclass/tap/agnostic-ai`
- `$GOBIN` or `$GOPATH/bin` (defaults to `$HOME/go/bin`) → `go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@latest`
- `*\scoop\apps\*`, `*\scoop\shims\*` → `scoop update agnostic-ai`
- `*\Microsoft\WinGet\*` → `winget upgrade Chemaclass.agnostic-ai`
- `*/node_modules/*` → `npm install -g agnostic-ai@latest`
- Anything else → manual download from the [releases page](https://github.com/Chemaclass/agnostic-ai/releases), or re-run the [install script](getting-started.md#install), which overwrites in place.

The three Windows-and-Node markers match case-insensitively, since those path segments carry whatever casing the user's profile uses.

If another `agnostic-ai` on `PATH` shadows the resolved executable, `upgrade` lists each shadow so you can remove the stale copy. Common cause: a Homebrew install behind an older `~/go/bin/agnostic-ai` or `/usr/local/bin/agnostic-ai`, where `brew upgrade` reports the brew copy is current but `agnostic-ai --version` keeps resolving to the older binary.

## help

```bash
agnostic-ai help              # top-level help
agnostic-ai help sync         # help for a subcommand
agnostic-ai sync --help       # same
```

## graph

Render the spec → target → file dependency graph. Read-only: no writes. Full guide in [graph](graph.md).

```bash
agnostic-ai graph                     # aligned text matrix
agnostic-ai graph --format mermaid    # text | mermaid | dot | json
agnostic-ai graph --target claude     # narrow by target, --spec, or --kind
```

| Flag | Description |
|------|-------------|
| `--format` | Output format: `text` (default), `mermaid`, `dot`, `json`. |
| `--target` | Restrict to one target. |
| `--spec` | Restrict to one spec name. |
| `--kind` | Restrict to one kind: agent, skill, rule, hook, mcp, command. |

## why

Reverse provenance for an emitted file. Reports the adapter, source spec(s), the `outputs.<target>.*` keys used, and the last sync timestamp. Full guide in [why](why.md).

```bash
agnostic-ai why .claude/rules/no-console-log.md
agnostic-ai why .claude/rules/no-console-log.md --format json
```

| Flag | Description |
|------|-------------|
| `--format` | Output format: `text` (default) or `json`. |

## lint

Semantic checks beyond schema: empty specs, duplicate names, dead specs (kinds no enabled target supports), hooks setting a matcher on an event that ignores it, and unterminated frontmatter. Exit 1 on error findings, or on warnings when `--strict` is set.

Two specs of the same kind declaring the same `name` (LINT003) is the one to watch. The loader collapses them, so one body silently never reaches any target. Several hooks sharing an event and matcher is **not** a collision: they all run, which is usually the point (`gofmt` and `vet` on the same save).

Unterminated frontmatter (LINT006) is the one worth knowing about: a spec that opens `---` and never closes it parses as body-only, so the raw YAML is emitted verbatim into every target and the spec silently never loads. `validate` and `sync` both pass in that state, which is why it is a lint error rather than a warning.

```bash
agnostic-ai lint
agnostic-ai lint --strict   # treat warnings as errors (CI)
```

## packs

Manage shareable spec packs. Packs load as a layer below the project, so the project overrides any pack entry by name. Full guide in [packs](packs.md).

```bash
agnostic-ai packs add github.com/chemaclass/go-rules@v1.2.0
agnostic-ai packs add ./path/to/pack
agnostic-ai packs list
agnostic-ai packs update [name]
agnostic-ai packs remove go-rules
```

## install-hook

Install a pre-commit hook that runs `sync --check`. See [git hooks](git-hooks.md).

```bash
agnostic-ai install-hook            # writes .git/hooks/pre-commit (local)
agnostic-ai install-hook --shared   # writes .githooks/ and sets core.hooksPath
```

## cleanup

Remove housekeeping leftovers. Today: the `<path>.bak` backups `sync --backup` writes, scoped to emitted paths. Unrelated `.bak` files are never touched.

```bash
agnostic-ai cleanup             # remove the .bak backups
agnostic-ai cleanup --dry-run   # preview deletions
```

## lsp

Start the Language Server on stdin/stdout. Point your editor at `agnostic-ai lsp` for spec files (`.agnostic-ai/**/*.md`, `*.mdc`). Pushes lint diagnostics on open and save.

```bash
agnostic-ai lsp
```

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Any error (parse failure, IO error, missing config) |

## Environment variables

| Var | Default | Description |
|-----|---------|-------------|
| `AGNOSTIC_AI_HOME` | `~/.agnostic-ai` | Root of the user-global spec layer. See [configuration: layered specs](configuration.md#layered-specs). |

## Config precedence

Last wins:

1. Built-in defaults (see [configuration](configuration.md))
2. `agnostic-ai.yaml`
3. CLI flags (e.g. `-t`)

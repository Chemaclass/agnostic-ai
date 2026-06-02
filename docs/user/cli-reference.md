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
| `-v, --verbose` | Per-target detail. Mutually exclusive with `--quiet`. |

## init

Scaffold a project: `agnostic-ai.yaml` plus empty `agents/`, `skills/`, `rules/`, `hooks/`, `mcps/`. Errors if `agnostic-ai.yaml` exists.

```bash
agnostic-ai init                  # default: prompt for targets when stdin is a TTY, base dir .agnostic-ai/
agnostic-ai init --all            # skip the prompt, enable every supported target
agnostic-ai init specs            # custom base: specs/{agents,skills,...}/
agnostic-ai init .                # legacy root-level layout
agnostic-ai init --demo           # seed each source folder with one example spec
agnostic-ai init --preset go      # seed idiomatic specs for a stack (go, ts-react, python)
echo "claude,codex" | agnostic-ai init   # non-TTY: pipe a comma-separated target list
```

| Flag | Description |
|------|-------------|
| `--demo` | Seed each source folder with one minimal example spec so a fresh project produces real output on the first `sync`. Existing files are never overwritten. |
| `--preset <name>` | Seed stack-flavored starter specs. Available: `go`, `ts-react`, `python`. Composes with `--demo` and `--all`. Errors on unknown names with the available list. Existing files are never overwritten, so re-running against a partially populated tree is safe. |
| `-a, --all` | Skip the target picker and enable every supported target in `agnostic-ai.yaml`. Useful for CI or scripted scaffolds. By default, `init` runs a multi-select prompt when stdin is a TTY, parses a piped comma-separated list when stdin is a pipe, and falls back to all targets when stdin is closed. |

The optional positional `[dir]` arg sets the base directory under which
the source folders are created. The generated `agnostic-ai.yaml`
writes matching `sources:` paths.

To pull in an existing AI CLI configuration after init, use `import`.

## import

Translate an existing AI CLI configuration into agnostic specs. Reads
`agnostic-ai.yaml` to resolve `sources:` paths, then writes specs
into those directories.

```bash
agnostic-ai import claude         # CLAUDE.md, .claude/{agents,skills,commands,settings.json,rules}, .mcp.json
agnostic-ai import codex          # AGENTS.md (root + nested), .codex/{prompts,config.toml}
agnostic-ai import claude codex   # multiple sources; AGNOSTIC_AI.md reflects the last (last-wins)
agnostic-ai import cursor         # .cursor/rules/*.mdc
agnostic-ai import cline          # .clinerules/
agnostic-ai import windsurf       # .windsurf/rules/
agnostic-ai import continue       # .continue/rules/
```

`import` does not modify `targets:` or any other config field; only
spec files under `sources:` are touched. Re-running overwrites by
filename. Run after `init`, in the same project root.

Pass multiple sources to import from each in order. Each importer
mirrors its target's top-level instructions file to
`.agnostic-ai/AGNOSTIC_AI.md`; with multiple sources, the last argument
wins. `all` cannot be combined with other sources; it auto-detects every
CLI present in the project.

`import claude`:

| Source | Becomes |
|--------|---------|
| `.claude/rules/*.md` (preferred) | `<rules>/<name>.md` (byte-identical copy) |
| `CLAUDE.md` (split on `## headings`) | `<rules>/<slug>.md` per section (only when `.claude/rules/` is absent) |
| `CLAUDE.md` (no headings) | single `<rules>/<projectname>.md` (only when `.claude/rules/` is absent) |
| `CLAUDE.md` (any form) | `.agnostic-ai/AGNOSTIC_AI.md` (byte-identical copy) |
| `.claude/agents/*.md` | `<agents>/<name>.md` (byte-identical copy) |
| `.claude/skills/<name>/SKILL.md` | `<skills>/<name>/SKILL.md` |
| `.claude/commands/*.md` | `<commands>/<name>.md` (byte-identical copy) |
| `.claude/settings.json` hooks | `<hooks>/<event>[-<matcher-slug>]-<hash8>.yaml` (filename derived from event, matcher, and commands so re-imports converge on the same path) |
| `.claude/settings.json` non-hook keys | `.agnostic-ai/overlays/claude.settings.json` (captures statusLine, enabledPlugins, model overrides, and any other top-level keys so `sync -t claude` reproduces the full settings.json after `.claude/` is wiped) |
| `.mcp.json` (`mcpServers.<name>`) | `<mcps>/<name>.yaml` (one spec per server; round-trips to every MCP-aware target on the next `sync`) |

When `.claude/rules/` exists, slicing `CLAUDE.md` is skipped entirely (even if the directory is empty) so the on-disk rules layout is the single source of truth for rule files. `.agnostic-ai/AGNOSTIC_AI.md` is still written from `CLAUDE.md` so the project keeps a CLI-agnostic top-level instructions file under the managed directory alongside `CLAUDE.md` / `AGENTS.md` / `GEMINI.md` at the project root.

`import codex` walks the project for `AGENTS.md` files at any depth and reads the rest of the Codex tree:

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

Slug collisions across files are deduplicated (`style.md`, `style-2.md`). Hidden directories, the configured source directories, `node_modules/`, and `vendor/` are skipped during the walk to avoid picking up unrelated `AGENTS.md` files.

**Overlay precedence.**

- **Codex**: when both the captured overlay and `outputs.codex.config.*` declare the same key (`model`, `sandbox`, `approval_policy`, `notify`, `[history]`, `[profiles.*]`, `[model_providers.*]`), the overlay wins on conflict and the first-class key is dropped to avoid a TOML duplicate-key error.
- **Claude**: the opposite. `outputs.claude.settings.*` overrides the overlay for any key it declares, so changes you make in `agnostic-ai.yaml` always reach `.claude/settings.json`. Re-run `import claude` to refresh the overlay whenever you hand-edit `.claude/settings.json`.

Keys declared in only one place are passed through unchanged either way.

`import cursor`:

| Source | Becomes |
|--------|---------|
| `.cursor/rules/<name>.mdc` | `<rules>/<name>.md` with frontmatter (`description`, `globs`, `alwaysApply`, plus any custom keys) preserved verbatim |
| (no `name:` in frontmatter) | `name:` injected from the filename |

Round-trips cleanly: a subsequent `sync` regenerates equivalent `.cursor/rules/*.mdc`.

`import cline`, `import windsurf`, `import continue` read the matching rules directory (`.clinerules/`, `.windsurf/rules/`, `.continue/rules/`) and reclassify each file by filename prefix:

| Source filename | Becomes |
|-----------------|---------|
| `agent-<name>.md` | `<agents>/<name>.md` |
| `skill-<name>.md` | `<skills>/<name>.md` |
| `<name>.md` | `<rules>/<name>.md` |
| `<scope>/<file>.md` | nested under the same `<scope>` in the destination |

The leading `# <heading>` block (which the adapter prepends on emit) is stripped on import, and a minimal `name:` frontmatter is injected.

## validate

Load all specs, report parse errors. Writes nothing.

```bash
agnostic-ai validate
```

```
loaded 12 entries. ok.
```

When no specs are loaded, `validate` prints a hint to stderr suggesting
`init` or `import` to populate sources. Stdout still reports `loaded 0
entries. ok.` so scripts that rely on stdout do not break.

`validate` also surfaces three native-support checks once specs load:

- **Hook events.** Each hook spec's `event:` value must be in the
  union of every configured target's supported list. Unknown values
  report inline with the supported list. Per-target sets:
  - Claude: `PreToolUse`, `PostToolUse`, `UserPromptSubmit`,
    `SessionStart`, `SessionEnd`, `Stop`, `SubagentStop`, `PreCompact`,
    `PostCompact`, `Notification`.
  - Codex: `PreToolUse`, `PostToolUse`, `UserPromptSubmit`,
    `SessionStart`, `SessionEnd`, `Stop`, `PreCompact`, `PostCompact`.
  - Gemini: `BeforeTool`, `AfterTool`, `SessionStart`, `SessionEnd`.
- **Missing fields.** Hook specs missing `event:` are flagged.
- **Orphaned kinds.** When a project has hook or MCP specs but no
  enabled target consumes them, validate prints one summary line per
  orphaned kind with the targets that would unblock it.

## list

Print all loaded specs as `kind<tab>name`.

```bash
agnostic-ai list
```

When no specs are loaded, `list` prints the same stderr hint and keeps
stdout empty so pipes stay clean.

## new

Scaffold a single spec file with kind-appropriate frontmatter under the
source directory configured for that kind. Replaces "copy from `--demo`
and edit" as the starting point for one new agent, skill, rule, hook,
or MCP.

```bash
agnostic-ai new rule no-console-log     # → <rules>/no-console-log.md
agnostic-ai new agent code-reviewer     # → <agents>/code-reviewer.md
agnostic-ai new skill yaml-validator    # → <skills>/yaml-validator.md
agnostic-ai new hook fmt-on-save        # → <hooks>/fmt-on-save.yaml
agnostic-ai new mcp filesystem          # → <mcps>/filesystem.yaml
```

Errors if the destination file already exists. Names must be lowercase
slugs (`[a-z0-9][a-z0-9-]*`); the same form Cursor and Cline expect.
Honors `sources:` from `agnostic-ai.yaml`, so a project that puts
specs under `specs/` lands files there automatically.

## explain

Reverse provenance: list every output file and section that one spec
contributes to. Pairs with the `<!-- source: ... -->` forward markers
adapters write into merged documents.

```bash
agnostic-ai explain rules/conventional-commits.md
agnostic-ai explain rules/conventional-commits.md --json
```

| Flag | Description |
|------|-------------|
| `--json` | Stable schema for editor extensions and scripts. |

Output groups contributions by configured target (the ones in
`agnostic-ai.yaml`) and a separate "would emit if enabled" list for
adapters that exist but are not currently activated. Each entry tags
itself as `(full file)` (the spec owns the file) or
`(section "<name>")` (the spec contributes one section to a merged
document). Writes nothing to disk.

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

Print the emission for a single spec to stdout, per target. Iterate on
one spec and see exactly what each adapter would produce, without
writing files or rerunning a full sync.

```bash
agnostic-ai render rules/no-console-log.md --target cursor
agnostic-ai render rules/no-console-log.md --target claude,codex
agnostic-ai render rules/no-console-log.md         # all configured targets
```

| Flag | Description |
|------|-------------|
| `-t, --target <list>` | Target(s) to render. Repeat or comma-separate. Default: every target in `agnostic-ai.yaml`. |

Output format is `# target: <name> — <output path>` followed by the file
body, one block per emitted file. Targets that produce no output for the
spec's kind print a short note. Render writes nothing to disk; pair with
`sync` once the output looks right.

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
| `--backup` | Copy each existing target file to `<path>.bak` before overwriting. Pair with `revert` to restore. |
| `--gitignore <on\|off>` | Override `gitignore.enabled` for this run. |
| `--watch` | Keep the process alive and re-emit on spec, config, or overlay changes. Watched roots: `agnostic-ai.yaml` + `agnostic-ai.local.yaml`, every `sources.*` directory, `.agnostic-ai.local/`, and `.agnostic-ai/overlays/` (so a hand-edit to `claude.settings.json` / `codex.config.toml` re-emits). Uses OS file events (fsnotify) with a 50 ms debounce; falls back to a 200 ms poll on filesystems where fsnotify fails. Ctrl+C exits cleanly. Incompatible with `--check`. |
| `--watch-poll` | Force the polling backend (200 ms) even when fsnotify is available. Use on network mounts or container volumes where fsnotify misses events. Requires `--watch`. |
| `--all` | Emit every configured target without running the first-sync target picker. Useful for ad-hoc full emission or scripted runs that should bypass interactive prompts. |
| `--json` | Output as JSON instead of plain text. Stable schema; breaking changes bump `version`. |

### First-sync target picker

On the very first `sync` (no `.agnostic-ai/.sync-state` file yet) where
the config still lists every supported target, `sync` opens an
interactive multi-select to narrow the list. The selection is persisted
to `agnostic-ai.yaml` so future syncs skip the prompt.

- **TTY:** multi-select widget (same UI as `init`'s default prompt).
- **Piped stdin:** `echo "claude,codex" | agnostic-ai sync` selects + persists without a prompt.
- **Non-TTY with no piped data (CI):** silent fallback. Emits every configured target. CI runs keep working without changes.
- **Bypass:** pass `--all`, `-t`, `--only`, or `--except` to skip the picker for one run.

```bash
agnostic-ai sync                       # first run: prompt for targets; later: all in config
agnostic-ai sync --all                 # skip the first-sync picker; emit every configured target
agnostic-ai sync -t claude             # only claude (legacy form)
agnostic-ai sync --only claude,cursor  # only claude and cursor
agnostic-ai sync --except codex        # all configured targets except codex
agnostic-ai sync --dry-run             # preview
agnostic-ai sync --check               # CI gate: fail if outputs are stale
agnostic-ai sync --backup              # leave a .bak trail for revert
agnostic-ai sync --watch               # re-emit on spec changes; Ctrl+C exits
agnostic-ai sync --json                # machine-readable output
agnostic-ai sync --check --json        # machine-readable drift report
```

`--only` and `--except` validate names against the configured targets list and return an error on unknown names (rather than silently skipping).

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
| `writes` | array | Files written (action `"create"` or `"update"`) or, for `--check`, files that need writing (action `"missing"` or `"stale"`). |
| `skipped` | array | Files whose on-disk content already matched (action `"skip"`). Empty for `--check`. |
| `errors` | array | Per-target errors with `target` and `message` fields. |

Each entry in `writes` and `skipped` has: `target` (string), `path` (string), `action` (string), `bytes` (number).

## revert

Undo a previous sync. For every file an adapter would emit, restores
`<path>.bak` when present (and removes the .bak). When there is no
`.bak`, the file is left in place by default so user-authored content
that happens to share a path with adapter output (helper scripts next
to `SKILL.md`, templates inside a propagated skill folder, etc.) is
not silently deleted. Pass `--force` to delete those unbacked files.

```bash
agnostic-ai revert [flags]
```

| Flag | Description |
|------|-------------|
| `-t, --target <list>` | Comma-separated targets (default: all in config) |
| `--only <list>` | Revert only these targets (comma-separated). Mutually exclusive with `--except`. Errors on unknown names. |
| `--except <list>` | Revert all configured targets except these (comma-separated). Mutually exclusive with `--only`. Errors on unknown names. |
| `--dry-run` | Report intended actions without touching disk |
| `--force` | Also delete adapter-emitted files that lack a `.bak`. Use with care: removes user-authored files that share a path with adapter output. |
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

The `--json` output uses the same schema as `sync --json`. Actions are
`"restore"` (`.bak` was applied), `"remove"` (file was deleted),
`"preserve"` (file kept because no `.bak` exists and `--force` was not
set), or `"skip"` (file was already absent).

Without a prior `--backup`, `revert` becomes a no-op unless `--force`
is also passed. This protects helper files from accidental deletion.

## doctor

Diagnose drift between source specs and emitted artifacts. Reports missing
files (never synced) and stale files (hand-edited or out of date). Exits
non-zero when any drift is found.

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

Use the no-flag form as a CI gate alongside `sync --check`, or after rebases to
spot files the merge resolved manually. Use `--fix` for an interactive cleanup
pass. Pair with `--backup` when reconciling files you may have hand-edited.

After the drift report, doctor prints an MCP block listing each MCP
spec's stdio `command:` and whether it resolves on PATH. Missing common
commands (`npx`, `uvx`, `python`, `docker`) include an inline install
hint. HTTP/SSE servers (no command, only `url:`) skip the check. The
report is advisory: a missing binary is an environment problem, not a
spec problem, so it does not change doctor's exit code.

After the MCP block, doctor walks `.agnostic-ai/scripts/<tool>/` per
tool and groups files by basename. When the same basename exists under
two or more tools with different SHA-256 bodies it prints one finding
per divergent script (sizes + truncated hashes per variant) and the
suggested consolidation path `.agnostic-ai/scripts/<basename>`.
Divergence registers as drift and contributes to a non-zero exit
because it usually represents an unnoticed import diff between tools.
Not auto-fixable: choosing which body wins requires human judgement.

## status

Show project configuration and current sync state. Exits 0 even when drift is
detected. Use `sync --check` or `doctor` as CI gates.

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

**State file:** Each successful `sync` writes `.agnostic-ai/.sync-state` (JSON) recording the
timestamp and file count. When that file is absent, `status` falls back to the newest mtime of
generated files. When no files exist yet, the timestamp is reported as `unknown`.

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

After installing, restart your shell (or `source` the completion file). Tab-completing `--target` reads `agnostic-ai.yaml` in the current directory and falls back to the full default target list when no config is found.

Run `agnostic-ai completion <shell> --help` for shell-specific setup instructions.

## upgrade

Detect how the running binary was installed and report (or run) the matching upgrade command. Does not self-replace the binary: package managers stay in charge of installed versions.

```bash
agnostic-ai upgrade           # print the upgrade command for the current install
agnostic-ai upgrade --check   # diagnose install location + PATH shadowing, exit
agnostic-ai upgrade --run     # exec the detected upgrade command
```

Detection:

- `*/Cellar/*`, `*/Caskroom/*`, `/opt/homebrew/*`, `/home/linuxbrew/.linuxbrew/*` → `brew update && brew upgrade Chemaclass/tap/agnostic-ai`
- `$GOBIN` or `$GOPATH/bin` (defaults to `$HOME/go/bin`) → `go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@latest`
- Anything else → manual download from the [releases page](https://github.com/Chemaclass/agnostic-ai/releases).

If another `agnostic-ai` binary on `PATH` shadows the resolved executable, `upgrade` lists each shadow so you can remove the stale copy. Common cause: a Homebrew install behind an older `~/go/bin/agnostic-ai` or `/usr/local/bin/agnostic-ai`, where `brew upgrade` correctly reports the brew copy is current but `agnostic-ai --version` keeps resolving to the older binary.

## help

```bash
agnostic-ai help              # top-level help
agnostic-ai help sync         # help for a subcommand
agnostic-ai sync --help       # same
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

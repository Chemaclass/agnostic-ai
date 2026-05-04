# CLI reference

```
agnostic-ai [command] [flags]
```

## Global flags

| Flag | Description |
|------|-------------|
| `-h, --help` | Help for any command |
| `-v, --version` | Print version and exit |

## init

Scaffold a project: `agnostic.config.yaml` plus empty `agents/`, `skills/`, `rules/`, `hooks/`, `mcps/`. Errors if `agnostic.config.yaml` exists.

```bash
agnostic-ai init                  # empty scaffold
agnostic-ai init --from claude    # import existing Claude Code config
agnostic-ai init --from codex     # import existing Codex / AGENTS.md config
agnostic-ai init --from cursor    # import existing Cursor config
agnostic-ai init --from cline     # import existing Cline config
agnostic-ai init --from windsurf  # import existing Windsurf config
agnostic-ai init --from continue  # import existing Continue config
```

| Flag | Description |
|------|-------------|
| `--from <source>` | Import existing config from a source. Supported: `claude`, `codex`, `cursor`, `cline`, `windsurf`, `continue`. |

`--from claude` reads the current directory:

| Source | Becomes |
|--------|---------|
| `CLAUDE.md` (split on `## headings`) | `rules/<slug>.md` per section |
| `CLAUDE.md` (no headings) | single `rules/<projectname>.md` |
| `.claude/agents/*.md` | `agents/<name>.md` (byte-identical copy) |
| `.claude/skills/<name>/SKILL.md` | `skills/<name>/SKILL.md` |
| `.claude/settings.json` hooks | `hooks/<event>-<group>-<index>.yaml` |

Writes `targets: [claude]` only. Add other targets to the config and run `agnostic-ai sync`.

`--from codex` walks the project for `AGENTS.md` files at any depth:

| Source | Becomes |
|--------|---------|
| `AGENTS.md` (split on `## headings`) | `rules/<slug>.md` per section |
| `AGENTS.md` (no headings) | single `rules/<projectname>.md` |
| `<dir>/AGENTS.md` (nested) | `rules/<slug>.md` with inferred `globs: <dir>/**` |
| `## Conventions` / `## Agents` / `## Skills` wrapper sections | unwrapped: their `### children` become the rules |
| Single-line italic (`_text_`) immediately under a rule heading | extracted into the rule's `description` (and removed from the body) |

Slug collisions across files are deduplicated (`style.md`, `style-2.md`). Hidden directories and `agents/`, `skills/`, `rules/`, `hooks/`, `node_modules/`, `vendor/` are skipped during the walk to avoid picking up unrelated `AGENTS.md` files. Writes `targets: [codex]` only.

`--from cursor` reads the current directory:

| Source | Becomes |
|--------|---------|
| `.cursor/rules/<name>.mdc` | `rules/<name>.md` with frontmatter (`description`, `globs`, `alwaysApply`, plus any custom keys) preserved verbatim |
| (no `name:` in frontmatter) | `name:` injected from the filename |

Round-trips cleanly: a subsequent `sync` regenerates equivalent `.cursor/rules/*.mdc`. Writes `targets: [cursor]` only.

`--from cline`, `--from windsurf`, `--from continue` read the matching rules directory (`.clinerules/`, `.windsurf/rules/`, `.continue/rules/`) and reclassify each file by filename prefix:

| Source filename | Becomes |
|-----------------|---------|
| `agent-<name>.md` | `agents/<name>.md` |
| `skill-<name>.md` | `skills/<name>.md` |
| `<name>.md` | `rules/<name>.md` |
| `<scope>/<file>.md` | nested under the same `<scope>` in the destination |

The leading `# <heading>` block (which the adapter prepends on emit) is stripped on import, and a minimal `name:` frontmatter is injected. Each importer writes a single-target config.

## validate

Load all specs, report parse errors. Writes nothing.

```bash
agnostic-ai validate
```

```
loaded 12 entries. ok.
```

## list

Print all loaded specs as `kind<tab>name`.

```bash
agnostic-ai list
```

## sync

Emit per-target configs.

```bash
agnostic-ai sync [flags]
```

| Flag | Description |
|------|-------------|
| `-t, --target <list>` | Comma-separated targets (default: all in config) |
| `--dry-run` | Print to stdout instead of writing files |
| `--check` | Compare emitted output to disk; exit non-zero on drift. Writes nothing. |
| `--backup` | Copy each existing target file to `<path>.bak` before overwriting. Pair with `revert` to restore. |
| `--gitignore <on\|off>` | Override `gitignore.enabled` for this run. |

```bash
agnostic-ai sync                    # all targets in config
agnostic-ai sync -t claude          # only claude
agnostic-ai sync -t claude,cursor   # subset
agnostic-ai sync --dry-run          # preview
agnostic-ai sync --check            # CI gate: fail if outputs are stale
agnostic-ai sync --backup           # leave a .bak trail for revert
```

Unknown targets log a warning to stderr and skip.

## revert

Undo a previous sync. For every file an adapter would emit, restores
`<path>.bak` if present (and removes the .bak), otherwise removes the file.

```bash
agnostic-ai revert [flags]
```

| Flag | Description |
|------|-------------|
| `-t, --target <list>` | Comma-separated targets (default: all in config) |
| `--dry-run` | Report intended actions without touching disk |

```bash
agnostic-ai sync --backup           # 1. snapshot existing files
# ...edits or experiments...
agnostic-ai revert                  # 2. roll back to the snapshot
```

Without a prior `--backup`, `revert` removes the generated files;
nothing is restored.

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

Use the no-flag form as a CI gate alongside `sync --check`, or after rebases to
spot files the merge resolved manually. Use `--fix` for an interactive cleanup
pass. Pair with `--backup` when reconciling files you may have hand-edited.

## completion

Shell completion scripts (cobra).

```bash
agnostic-ai completion bash      # for bash
agnostic-ai completion zsh       # for zsh
agnostic-ai completion fish      # for fish
agnostic-ai completion powershell
```

Install: `agnostic-ai completion <shell> --help`.

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
2. `agnostic.config.yaml`
3. CLI flags (e.g. `-t`)

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

Scaffold a project: `agnostic.config.yaml` plus empty `agents/`, `skills/`, `rules/`, `hooks/`. Errors if `agnostic.config.yaml` exists.

```bash
agnostic-ai init                  # empty scaffold
agnostic-ai init --from claude    # import existing Claude Code config
```

| Flag | Description |
|------|-------------|
| `--from <source>` | Import existing config from a source. Supported: `claude`. |

`--from claude` reads the current directory:

| Source | Becomes |
|--------|---------|
| `CLAUDE.md` (split on `## headings`) | `rules/<slug>.md` per section |
| `CLAUDE.md` (no headings) | single `rules/<projectname>.md` |
| `.claude/agents/*.md` | `agents/<name>.md` (byte-identical copy) |
| `.claude/skills/<name>/SKILL.md` | `skills/<name>/SKILL.md` |
| `.claude/settings.json` hooks | `hooks/<event>-<group>-<index>.yaml` |

Writes `targets: [claude]` only. Add other targets to the config and run `agnostic-ai sync`.

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

```bash
agnostic-ai sync                    # all targets in config
agnostic-ai sync -t claude          # only claude
agnostic-ai sync -t claude,cursor   # subset
agnostic-ai sync --dry-run          # preview
agnostic-ai sync --check            # CI gate: fail if outputs are stale
```

Unknown targets log a warning to stderr and skip.

## doctor

Diagnose drift between source specs and emitted artifacts. Reports missing
files (never synced) and stale files (hand-edited or out of date). Exits
non-zero when any drift is found. Writes nothing.

```bash
agnostic-ai doctor                  # all targets in config
agnostic-ai doctor -t claude        # subset
```

Use as a CI gate alongside `sync --check`, or after rebases to spot files
the merge resolved manually.

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

None. All config lives in `agnostic.config.yaml`.

## Config precedence

Last wins:

1. Built-in defaults (see [configuration](configuration.md))
2. `agnostic.config.yaml`
3. CLI flags (e.g. `-t`)

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
agnostic-ai init                  # default base dir: .agnostic-ai/
agnostic-ai init specs            # custom base: specs/{agents,skills,...}/
agnostic-ai init .                # legacy root-level layout
agnostic-ai init --demo           # seed each source folder with one example spec
agnostic-ai init -i               # interactive: pick which targets land in config
```

| Flag | Description |
|------|-------------|
| `--demo` | Seed each source folder with one minimal example spec so a fresh project produces real output on the first `sync`. Existing files are never overwritten. |
| `-i, --interactive` | Multi-select prompt to pick which targets land in `agnostic.config.yaml`. Accepts piped comma-separated input for non-TTY use (e.g. `echo "claude,codex" \| agnostic-ai init -i`). |

The optional positional `[dir]` arg sets the base directory under which
the source folders are created. The generated `agnostic.config.yaml`
writes matching `sources:` paths.

To pull in an existing AI CLI configuration after init, use `import`.

## import

Translate an existing AI CLI configuration into agnostic specs. Reads
`agnostic.config.yaml` to resolve `sources:` paths, then writes specs
into those directories.

```bash
agnostic-ai import claude     # CLAUDE.md, .claude/{agents,skills,settings.json}
agnostic-ai import codex      # AGENTS.md (root + nested)
agnostic-ai import cursor     # .cursor/rules/*.mdc
agnostic-ai import cline      # .clinerules/
agnostic-ai import windsurf   # .windsurf/rules/
agnostic-ai import continue   # .continue/rules/
```

`import` does not modify `targets:` or any other config field; only
spec files under `sources:` are touched. Re-running overwrites by
filename. Run after `init`, in the same project root.

`import claude`:

| Source | Becomes |
|--------|---------|
| `CLAUDE.md` (split on `## headings`) | `<rules>/<slug>.md` per section |
| `CLAUDE.md` (no headings) | single `<rules>/<projectname>.md` |
| `.claude/agents/*.md` | `<agents>/<name>.md` (byte-identical copy) |
| `.claude/skills/<name>/SKILL.md` | `<skills>/<name>/SKILL.md` |
| `.claude/settings.json` hooks | `<hooks>/<event>-<group>-<index>.yaml` |

`import codex` walks the project for `AGENTS.md` files at any depth:

| Source | Becomes |
|--------|---------|
| `AGENTS.md` (split on `## headings`) | `<rules>/<slug>.md` per section |
| `AGENTS.md` (no headings) | single `<rules>/<projectname>.md` |
| `<dir>/AGENTS.md` (nested) | `<rules>/<slug>.md` with inferred `globs: <dir>/**` |
| `## Conventions` / `## Agents` / `## Skills` wrapper sections | unwrapped: their `### children` become the rules |
| Single-line italic (`_text_`) immediately under a rule heading | extracted into the rule's `description` (and removed from the body) |

Slug collisions across files are deduplicated (`style.md`, `style-2.md`). Hidden directories, the configured source directories, `node_modules/`, and `vendor/` are skipped during the walk to avoid picking up unrelated `AGENTS.md` files.

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

## list

Print all loaded specs as `kind<tab>name`.

```bash
agnostic-ai list
```

When no specs are loaded, `list` prints the same stderr hint and keeps
stdout empty so pipes stay clean.

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
| `--watch` | Keep the process alive and re-emit on spec or config changes (200 ms poll). Ctrl+C exits cleanly. Incompatible with `--check`. |
| `--auto-sync <yes\|no>` | Persist an answer to the first-run auto-sync prompt. Writes an `auto-sync` rule spec instructing agents to run `agnostic-ai sync` when specs change. Persists `autoSync: true/false` to `agnostic.config.yaml`. Skipped under `--dry-run`. Without the flag, on a TTY, `sync` prompts once. |

```bash
agnostic-ai sync                    # all targets in config
agnostic-ai sync -t claude          # only claude
agnostic-ai sync -t claude,cursor   # subset
agnostic-ai sync --dry-run          # preview
agnostic-ai sync --check            # CI gate: fail if outputs are stale
agnostic-ai sync --backup           # leave a .bak trail for revert
agnostic-ai sync --watch            # re-emit on spec changes; Ctrl+C exits
agnostic-ai sync --auto-sync=yes    # opt in to agent-managed auto-sync
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

Generate a shell completion script and install it for your shell.

```bash
# Bash — system-wide (may need sudo)
agnostic-ai completion bash > /etc/bash_completion.d/agnostic-ai

# Bash — current user only
agnostic-ai completion bash > ~/.local/share/bash-completion/completions/agnostic-ai

# Zsh
agnostic-ai completion zsh > "${fpath[1]}/_agnostic-ai"

# Fish
agnostic-ai completion fish > ~/.config/fish/completions/agnostic-ai.fish

# PowerShell
agnostic-ai completion powershell | Out-String | Invoke-Expression
```

After installing, restart your shell (or `source` the completion file). Tab-completing `--target` reads `agnostic.config.yaml` in the current directory and falls back to the full default target list when no config is found.

Run `agnostic-ai completion <shell> --help` for shell-specific setup instructions.

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

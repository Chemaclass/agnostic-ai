# Getting started

## Install

Homebrew:
```bash
brew install Chemaclass/tap/agnostic-ai
```

Direct binary:
```bash
curl -fsSL https://github.com/Chemaclass/agnostic-ai/releases/latest/download/agnostic-ai-$(uname -s)-$(uname -m) \
  -o /usr/local/bin/agnostic-ai && chmod +x /usr/local/bin/agnostic-ai
```

From source:
```bash
go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@latest
```

## Shell completion

Enable tab completion for subcommands and `--target`:

```bash
# Zsh
agnostic-ai completion zsh > "${fpath[1]}/_agnostic-ai"

# Bash (user-level)
agnostic-ai completion bash > ~/.local/share/bash-completion/completions/agnostic-ai

# Fish
agnostic-ai completion fish > ~/.config/fish/completions/agnostic-ai.fish
```

Restart your shell after installing. See `agnostic-ai completion <shell> --help` for more options.

## Scaffold

```bash
agnostic-ai init                 # default: prompt for targets when TTY, base dir .agnostic-ai/
agnostic-ai init --all           # skip the prompt, enable every supported target
agnostic-ai init specs           # custom base
agnostic-ai init .               # legacy root-level layout
agnostic-ai init --demo          # plus one example spec per source folder
echo "claude,codex" | agnostic-ai init   # non-TTY: pipe a comma-separated target list
```

`--demo` seeds each source folder with a minimal example spec so the
first `sync` produces real output. By default `init` opens a target
picker when stdin is a TTY; pipe a comma-separated list for non-TTY use,
or pass `--all` (`-a`) to skip the picker and enable every target.

The generated `agnostic-ai.yaml` includes a `yaml-language-server` comment pointing at the published JSON Schema. Editors with YAML Language Server support (VS Code, JetBrains, Neovim) validate and autocomplete the config automatically.

```
.
├── agnostic-ai.yaml
└── .agnostic-ai/
    ├── agents/
    ├── skills/
    ├── rules/
    ├── hooks/
    └── mcps/
```

### Already on another AI CLI? Import it

After `init`, run `import <source>` to translate an existing
configuration into agnostic specs under the configured `sources:` paths:

```bash
agnostic-ai init                  # scaffold
agnostic-ai import claude         # CLAUDE.md + .claude/{agents,skills,settings.json}
agnostic-ai import codex          # AGENTS.md (root + nested)
agnostic-ai import cursor         # .cursor/rules/*.mdc
agnostic-ai import cline          # .clinerules/
agnostic-ai import windsurf       # .windsurf/rules/
agnostic-ai import continue       # .continue/rules/
agnostic-ai sync                  # fan out to every target in the config
```

`import` does not touch `targets:` or other config fields; it only
writes spec files. The default `init` config enables every target, so
one `sync` covers them all. To narrow output, edit the `targets:` list.

## First rule

`rules/conventional-commits.md`:

```markdown
---
name: conventional-commits
description: Always use Conventional Commits.
alwaysApply: true
---

Use `feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:` prefixes. Subject under 72 chars.
```

## Sync

```bash
agnostic-ai sync
```

On the first run, `sync` opens a multi-select to pick which targets to
enable. The choice is saved to `agnostic-ai.yaml` and never asked
again. To emit every target without the prompt, run `sync --all` or
pipe a selection (`echo "claude,codex" | agnostic-ai sync`).

| Output | Target |
|--------|--------|
| `CLAUDE.md` | Claude Code |
| `AGENTS.md` | Codex |
| `GEMINI.md` | Gemini CLI |
| `.cursor/rules/conventional-commits.mdc` | Cursor |

Full tree after `sync` with the default targets (only files with content
are written; empty stubs are skipped):

```
.
├── agnostic-ai.yaml
├── rules/
│   └── conventional-commits.md
├── CLAUDE.md                                    # for Claude Code
├── AGENTS.md                                    # for Codex / Amp / Warp (shared open standard)
├── .codex/agents/<name>.toml                    # for Codex subagents
├── .agents/commands/<name>.md                   # for Amp slash commands
├── GEMINI.md                                    # for Gemini CLI
├── .gemini/commands/<name>.toml                 # for Gemini CLI slash commands
├── CONVENTIONS.md                               # for Aider
├── .rules                                       # for Zed
├── .opencode/AGENTS.md                          # for OpenCode
├── .opencode/commands/<name>.md                 # for OpenCode slash commands
├── .github/copilot-instructions.md              # for Copilot (always-on rules)
├── .github/instructions/<name>.instructions.md  # for Copilot path-scoped rules + agents + skills
├── .cursor/rules/conventional-commits.mdc       # for Cursor
├── .clinerules/conventional-commits.md          # for Cline
├── .windsurf/rules/conventional-commits.md      # for Windsurf
└── .continue/rules/conventional-commits.md      # for Continue
```

## Check project status

See what the tool knows about your project at a glance:

```bash
agnostic-ai status
```

Output includes the project name, active spec layers, spec counts, configured
targets, the last sync timestamp, and whether any generated files are out of
date. Exits 0 regardless of drift. Use `sync --check` or `doctor` in CI.

## Roll back a sync

```bash
agnostic-ai sync --backup    # snapshot existing outputs to <path>.bak
# ...experiment with spec changes...
agnostic-ai revert           # restore from .bak when present, else delete
```

Pair `--backup` with `revert` for safe iteration. Without `--backup`,
`revert` removes the generated files.

## Watch mode

Skip the manual `sync` after every spec edit:

```bash
agnostic-ai sync --watch
```

Watches the source directories and `agnostic-ai.yaml` via OS file
events (fsnotify) with a 50 ms debounce, so saves trigger a re-sync in
under 100 ms. On filesystems where fsnotify fails (some network mounts,
specific container volumes), `sync` falls back to a 200 ms poll. Force
the poll backend with `--watch-poll`. Ctrl+C exits cleanly. Incompatible
with `--check`.

## Auto-sync rule

On the first `sync` in a TTY, `agnostic-ai` offers to write an
`auto-sync` rule spec instructing agents to run `agnostic-ai sync`
whenever specs change. Skip the prompt with the flag:

```bash
agnostic-ai sync --auto-sync=yes   # write the rule, persist autoSync: true
agnostic-ai sync --auto-sync=no    # skip, persist autoSync: false
```

The answer is saved to `agnostic-ai.yaml` as `autoSync: true|false`
so the prompt fires only once. `--dry-run` skips the prompt and the
persistence step.

## Auto-manage .gitignore

Add to `agnostic-ai.yaml` to keep generated paths out of git:

```yaml
gitignore:
  enabled: true
```

`sync` rewrites a managed block in `.gitignore` listing every emitted
path. Lines outside the block are preserved.

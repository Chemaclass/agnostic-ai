# Getting started

## Install

| Method | Command |
|--------|---------|
| Homebrew | `brew install --cask Chemaclass/tap/agnostic-ai` |
| Direct binary | `curl -fsSL https://github.com/Chemaclass/agnostic-ai/releases/latest/download/agnostic-ai-$(uname -s)-$(uname -m) -o /usr/local/bin/agnostic-ai && chmod +x /usr/local/bin/agnostic-ai` |
| From source | `go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@latest` |

## Shell completion

Tab completion for subcommands and `--target`:

```bash
agnostic-ai completion zsh > "${fpath[1]}/_agnostic-ai"
agnostic-ai completion bash > ~/.local/share/bash-completion/completions/agnostic-ai
agnostic-ai completion fish > ~/.config/fish/completions/agnostic-ai.fish
```

Restart your shell after installing. Run `agnostic-ai completion <shell> --help` for more options.

## Scaffold

```bash
agnostic-ai init                 # prompt for targets (TTY), base dir .agnostic-ai/
agnostic-ai init --all           # enable every target, skip the prompt
agnostic-ai init specs           # custom base dir
agnostic-ai init .               # legacy root-level layout
agnostic-ai init --demo          # plus one example spec per source folder
echo "claude,codex" | agnostic-ai init   # non-TTY: pipe a comma-separated target list
```

- `init` opens a target picker when stdin is a TTY.
- Non-TTY: pipe a comma-separated list.
- `--all` (`-a`): skip the picker, enable every target.
- `--demo`: seed each source folder with a minimal spec so the first `sync` produces output.

The generated `agnostic-ai.yaml` carries a `yaml-language-server` comment pointing at the published JSON Schema. Editors with YAML Language Server support (VS Code, JetBrains, Neovim) validate and autocomplete the config.

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

### Import an existing AI CLI config

After `init`, run `import <source>` to translate an existing config into agnostic specs under the configured `sources:` paths:

```bash
agnostic-ai init                  # scaffold
agnostic-ai import claude         # CLAUDE.md + .claude/{agents,skills,settings.json}
agnostic-ai import codex          # AGENTS.md (root + nested)
agnostic-ai import cursor         # .cursor/rules/*.mdc
agnostic-ai import cline          # .clinerules/
agnostic-ai import windsurf       # .devin/rules/ (or legacy .windsurf/rules/)
agnostic-ai import continue       # .continue/rules/
agnostic-ai sync                  # fan out to every target in the config
```

`import` writes spec files only. It never touches `targets:` or other config. The default `init` config enables every target, so one `sync` covers them all. Edit `targets:` to narrow output.

#### Recommended adoption workflow

The first `sync` after `import` rewrites every generated file (adds the agnostic-ai header, normalizes key order). Split into two commits so the diff is reviewable:

```bash
# 1. Capture existing CLI config into agnostic-ai specs
agnostic-ai init
agnostic-ai import claude          # or codex / cursor / cline / ...
git add .agnostic-ai/ agnostic-ai.yaml AGNOSTIC_AI.md
git commit -m "chore(agnostic-ai): import existing claude config"

# 2. Regenerate every target's files from the imported specs
agnostic-ai sync
git add -A
git commit -m "chore(agnostic-ai): regenerate per-target configs (no semantic change)"
```

- Reviewers focus on commit 1 (content) and skim commit 2 (cosmetic).
- Importing from multiple CLIs: run each `import` + commit pair before the final `sync`.
- Gitignoring generated files instead (see [`gitignore.enabled`](configuration.md#gitignore)): skip commit 2. Version only `.agnostic-ai/`; each contributor runs `sync` locally.
- `sync --backup` keeps a `.bak` next to each overwritten file so you can `revert`. Clear them with `agnostic-ai cleanup` (scoped to the sync-written backups; unrelated `.bak` files are left alone).

#### What `import` does NOT capture

`import <cli>` translates rules, agents, skills, hooks, commands, and a `settings.json`-style overlay into specs. Out of scope:

- `.claude/statusline.sh`, helper scripts referenced from `settings.json`, ad-hoc config files. Keep these in git next to `.agnostic-ai/` so they survive a fresh checkout.
- Exception: helper files inside a skill directory (e.g. `.claude/skills/yaml-validator/check.mjs`) round-trip via the nested skill layout (see [Skills](spec-format.md#skills)).

#### `.agnostic-ai/.sync-state`

Every `sync` writes `.agnostic-ai/.sync-state`, a JSON document recording the last sync timestamp and files changed.

- `status` reads it for "last sync at ...".
- `doctor` reads it to tell never-synced from post-sync-edits.
- Auto-added to the managed `.gitignore` block. Do not commit it.
- Safe to delete: the next `sync` regenerates it. Until then `status` reports "never synced".

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

The first run opens a multi-select to pick targets. The choice saves to `agnostic-ai.yaml` and is never asked again. Emit every target without the prompt via `sync --all` or pipe a selection (`echo "claude,codex" | agnostic-ai sync`).

| Output | Target |
|--------|--------|
| `CLAUDE.md` | Claude Code |
| `AGENTS.md` | Codex |
| `GEMINI.md` | Gemini CLI |
| `.cursor/rules/conventional-commits.mdc` | Cursor |

Full tree after `sync` with the default targets (only files with content are written; empty stubs are skipped):

```
.
├── agnostic-ai.yaml
├── rules/
│   └── conventional-commits.md
├── .claude/rules/<name>.md                      # for Claude Code (project rules)
├── AGENTS.md                                    # shared open standard (Codex / Amp / Warp / Zed / Cline / Junie / Kiro / Crush / Trae)
├── .codex/agents/<name>.toml                    # for Codex subagents
├── .agents/commands/<name>.md                   # for Amp slash commands
├── GEMINI.md                                    # for Gemini CLI
├── .gemini/commands/<name>.toml                 # for Gemini CLI slash commands
├── CONVENTIONS.md                               # for Aider
├── .agents/skills/<name>/SKILL.md               # shared skills tree (Codex / Amp / Zed / Crush)
├── .opencode/AGENTS.md                          # for OpenCode
├── .opencode/agents/<name>.md                   # for OpenCode subagents
├── .github/copilot-instructions.md              # for Copilot (always-on rules)
├── .github/instructions/<name>.instructions.md  # for Copilot path-scoped rules
├── .github/agents/<name>.agent.md               # for Copilot custom agents
├── .cursor/rules/conventional-commits.mdc       # for Cursor
├── .clinerules/conventional-commits.md          # for Cline
├── .devin/rules/conventional-commits.md         # for Windsurf / Devin Desktop
├── .junie/rules/conventional-commits.md         # for Junie
├── .kiro/steering/conventional-commits.md       # for Kiro
├── .trae/rules/conventional-commits.md          # for Trae
└── .continue/rules/conventional-commits.md      # for Continue
```

## Check project status

```bash
agnostic-ai status
```

Reports the project name, active spec layers, spec counts, configured targets, last sync timestamp, and whether generated files are out of date. Exits 0 regardless of drift. Use `sync --check` or `doctor` in CI.

## Roll back a sync

```bash
agnostic-ai sync --backup    # snapshot existing outputs to <path>.bak
# ...experiment with spec changes...
agnostic-ai revert           # restore from .bak when present, else delete
```

Pair `--backup` with `revert` for safe iteration. Without `--backup`, `revert` deletes the generated files.

## Watch mode

```bash
agnostic-ai sync --watch
```

Watches the source directories and `agnostic-ai.yaml` via OS file events (fsnotify) with a 50 ms debounce, so saves re-sync in under 100 ms.

- On filesystems where fsnotify fails (some network mounts, container volumes), `sync` falls back to a 200 ms poll.
- `--watch-poll` forces the poll backend.
- Ctrl+C exits cleanly.
- Incompatible with `--check`.

## Auto-manage .gitignore

Add to `agnostic-ai.yaml` to keep generated paths out of git:

```yaml
gitignore:
  enabled: true
```

`sync` rewrites a managed block in `.gitignore` listing every emitted path. Lines outside the block are preserved.

Gitignored outputs are not in git, so a fresh clone or a new `git worktree` starts without them until `agnostic-ai sync` runs. A contributor cloning the repo runs `sync` by hand; automated worktree creation does not. Wire `sync` into your worktree bootstrap, or add a `post-checkout` hook (see [Git hooks](git-hooks.md)), so an AI session opened in a new worktree always finds its config.

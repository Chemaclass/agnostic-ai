# Getting started

## Install

Homebrew:
```bash
brew install chemaclass/tap/agnostic-ai
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

## Scaffold

```bash
agnostic-ai init                 # default base dir: .agnostic-ai/
agnostic-ai init specs           # custom base
agnostic-ai init .               # legacy root-level layout
```

```
.
├── agnostic.config.yaml
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

| Output | Target |
|--------|--------|
| `CLAUDE.md` | Claude Code |
| `AGENTS.md` | Codex |
| `GEMINI.md` | Gemini CLI |
| `.cursor/rules/conventional-commits.mdc` | Cursor |

Full tree after sync with the default targets:

```
.
├── agnostic.config.yaml
├── rules/
│   └── conventional-commits.md
├── CLAUDE.md                                    # for Claude Code
├── AGENT.md                                     # for Amp
├── AGENTS.md                                    # for Codex
├── .codex/agents/<name>.toml                    # for Codex subagents
├── GEMINI.md                                    # for Gemini CLI
├── WARP.md                                      # for Warp
├── CONVENTIONS.md                               # for Aider
├── .rules                                       # for Zed
├── .opencode/AGENTS.md                          # for OpenCode
├── .github/copilot-instructions.md              # for Copilot
├── .cursor/rules/conventional-commits.mdc       # for Cursor
├── .clinerules/conventional-commits.md          # for Cline
├── .windsurf/rules/conventional-commits.md      # for Windsurf
└── .continue/rules/conventional-commits.md      # for Continue
```

## Roll back a sync

```bash
agnostic-ai sync --backup    # snapshot existing outputs to <path>.bak
# ...experiment with spec changes...
agnostic-ai revert           # restore from .bak when present, else delete
```

Pair `--backup` with `revert` for safe iteration. Without `--backup`,
`revert` removes the generated files.

## Auto-manage .gitignore

Add to `agnostic.config.yaml` to keep generated paths out of git:

```yaml
gitignore:
  enabled: true
```

`sync` rewrites a managed block in `.gitignore` listing every emitted
path. Lines outside the block are preserved.

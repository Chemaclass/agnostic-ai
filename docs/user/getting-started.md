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
agnostic-ai init
```

```
.
├── agnostic.config.yaml
├── agents/
├── skills/
├── rules/
└── hooks/
```

### Already on Claude Code?

```bash
agnostic-ai init --from claude
```

Reads `CLAUDE.md` and `.claude/` from the current directory, writes equivalent agnostic specs and a `targets: [claude]` config. Add more targets to the config and run `agnostic-ai sync`.

### Already on Codex (or any AGENTS.md project)?

```bash
agnostic-ai init --from codex
```

Walks the project for `AGENTS.md` files (root and any nested), splits each by `##` heading into one rule per section, and writes a `targets: [codex]` config. Nested files inherit `globs:` inferred from their path (`src/AGENTS.md` → `globs: src/**`) so a later `sync` routes each rule back to the correct nested `AGENTS.md`. Add more targets and run `agnostic-ai sync` to fan the same rules out everywhere.

### Already on Cursor?

```bash
agnostic-ai init --from cursor
```

Reads `.cursor/rules/*.mdc`, translates each into `rules/<name>.md` with frontmatter (`description`, `globs`, `alwaysApply`, plus any custom keys) preserved, and writes a `targets: [cursor]` config. Add more targets and run `agnostic-ai sync` to fan the same rules out to every supported tool.

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
├── AGENTS.md                                    # for Codex
├── GEMINI.md                                    # for Gemini CLI
├── CONVENTIONS.md                               # for Aider
├── .github/copilot-instructions.md              # for Copilot
├── .cursor/rules/conventional-commits.mdc       # for Cursor
├── .clinerules/conventional-commits.md          # for Cline
├── .windsurf/rules/conventional-commits.md      # for Windsurf
└── .continue/rules/conventional-commits.md      # for Continue
```

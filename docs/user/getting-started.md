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

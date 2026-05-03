# Targets

Unsupported features are skipped with a warning.

## Capability matrix

| Target          | Agents              | Skills | Rules                    | Hooks |
|-----------------|---------------------|--------|--------------------------|-------|
| **claude**      | `.claude/agents/`   | `.claude/skills/` | `CLAUDE.md`   | `.claude/settings.json` |
| **codex**       | merged in `AGENTS.md` | -    | `AGENTS.md`              | -     |
| **gemini**      | merged in `GEMINI.md` | -    | `GEMINI.md`              | -     |
| **cursor**      | as `.mdc` (alwaysApply: false) | - | `.cursor/rules/*.mdc` | - |
| **copilot**     | merged in instructions | -  | `.github/copilot-instructions.md` | - |
| **aider**       | merged in `CONVENTIONS.md` | - | `CONVENTIONS.md`     | -     |
| **cline**       | as `.md` rule       | -      | `.clinerules/*.md`       | -     |
| **windsurf**    | as `.md` rule       | -      | `.windsurf/rules/*.md`   | -     |
| **continue**    | as `.md` rule       | -      | `.continue/rules/*.md`   | -     |

## Per-target output

### Claude Code (`claude`)

```
.claude/
├── agents/<name>.md
├── skills/<name>/SKILL.md
└── settings.json
CLAUDE.md
```

### Codex (`codex`)

```
AGENTS.md
```

### Gemini CLI (`gemini`)

```
GEMINI.md
```

### Cursor (`cursor`)

```
.cursor/rules/<name>.mdc
```

### GitHub Copilot (`copilot`)

```
.github/copilot-instructions.md
```

### Aider (`aider`)

```
CONVENTIONS.md
```

Pair with `aider --read CONVENTIONS.md` or add to `.aider.conf.yml`.

### Cline (`cline`)

```
.clinerules/<name>.md
```

### Windsurf (`windsurf`)

```
.windsurf/rules/<name>.md
```

### Continue (`continue`)

```
.continue/rules/<name>.md
```

## Selecting targets

```yaml
targets:
  - claude
  - cursor
  - copilot
```

Or via flag:

```bash
agnostic-ai sync -t claude,cursor,copilot
```

## New targets

See [adding-adapters](../internal/adding-adapters.md). New adapter is ~50 lines plus one registry entry.

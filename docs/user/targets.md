# Targets

Unsupported features skip with a warning by default. Override via `on-unsupported` in [configuration](configuration.md).

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

Config keys: `outputs.claude.dir` (default `.claude`), `outputs.claude.rules-file` (default `CLAUDE.md`).

### Codex (`codex`)

```
AGENTS.md
```

Config key: `outputs.codex.file` (default `AGENTS.md`).

### Gemini CLI (`gemini`)

```
GEMINI.md
```

Config key: `outputs.gemini.file` (default `GEMINI.md`).

### Cursor (`cursor`)

```
.cursor/rules/<name>.mdc
```

Config key: `outputs.cursor.rules-dir` (default `.cursor/rules`).

Rules emit with `alwaysApply: true`; agents as rules with `alwaysApply: false`. Override in spec frontmatter.

### GitHub Copilot (`copilot`)

```
.github/copilot-instructions.md
```

Config key: `outputs.copilot.file` (default `.github/copilot-instructions.md`).

### Aider (`aider`)

```
CONVENTIONS.md
```

Config key: `outputs.aider.file` (default `CONVENTIONS.md`).

Pair with `aider --read CONVENTIONS.md` or add to `.aider.conf.yml`.

### Cline (`cline`)

```
.clinerules/<name>.md
```

Config key: `outputs.cline.rules-dir` (default `.clinerules`).

### Windsurf (`windsurf`)

```
.windsurf/rules/<name>.md
```

Config key: `outputs.windsurf.rules-dir` (default `.windsurf/rules`).

### Continue (`continue`)

```
.continue/rules/<name>.md
```

Config key: `outputs.continue.rules-dir` (default `.continue/rules`).

## Selecting targets

Persistent (config):

```yaml
targets:
  - claude
  - cursor
  - copilot
```

Per-run (CLI):

```bash
agnostic-ai sync -t claude,cursor,copilot
```

CLI flag overrides config. Unknown targets log a warning and skip.

## New targets

See [adding-adapters](../internal/adding-adapters.md). ~50 lines plus one registry entry.

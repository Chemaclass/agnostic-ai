# Targets

Unsupported features skip with a warning by default. Override via `on-unsupported` in [configuration](configuration.md).

## Capability matrix

| Target          | Agents              | Skills | Rules                    | Hooks |
|-----------------|---------------------|--------|--------------------------|-------|
| **claude**      | `.claude/agents/`   | `.claude/skills/` | `CLAUDE.md`   | `.claude/settings.json` |
| **codex**       | merged in `AGENTS.md` | listed in `AGENTS.md` | `AGENTS.md` (nested per-dir by globs) | - |
| **gemini**      | merged in `GEMINI.md` | listed in `GEMINI.md` | `GEMINI.md`     | -     |
| **cursor**      | as `.mdc` (alwaysApply: false) | as `.mdc` (`skill-<name>.mdc`) | `.cursor/rules/*.mdc` | - |
| **copilot**     | merged in instructions | listed in instructions | `.github/copilot-instructions.md` | - |
| **aider**       | merged in `CONVENTIONS.md` | listed in `CONVENTIONS.md` | `CONVENTIONS.md` | - |
| **cline**       | as `.md` rule       | as `.md` (`skill-<name>.md`) | `.clinerules/*.md`       | -     |
| **windsurf**    | as `.md` rule       | as `.md` (`skill-<name>.md`) | `.windsurf/rules/*.md`   | -     |
| **continue**    | as `.md` rule       | as `.md` (`skill-<name>.md`) | `.continue/rules/*.md`   | -     |

Skills emitted to non-Claude targets are reference material. Only Claude
Code has native skill execution. For all other targets, the agent or
human reads the skill file and follows its instructions.

Hooks are Claude-specific. They run as shell commands on lifecycle
events (e.g. PreToolUse, PostToolUse, SessionStart). No other supported
target has an equivalent concept, so hooks emit only for Claude.

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
src/AGENTS.md           # if any rule has globs: src/**
docs/api/AGENTS.md      # if any rule has globs: docs/api/**
```

Config key: `outputs.codex.file` (default `AGENTS.md`).

Codex emits a hierarchy of `AGENTS.md` files. Rules with a `globs` frontmatter field that names a fixed directory prefix (e.g. `src/**`, `docs/api/**`) route into that subdirectory. Unscoped rules and all agents go to the root file. Skills are listed by name + description in the root with a pointer to the source path; Codex has no native skill execution.

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

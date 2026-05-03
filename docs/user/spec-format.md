# Spec format

| Kind   | Source         | Format                      |
|--------|----------------|-----------------------------|
| Agent  | `agents/*.md`  | Markdown + YAML frontmatter |
| Skill  | `skills/*.md`  | Markdown + YAML frontmatter |
| Rule   | `rules/*.md`   | Markdown + YAML frontmatter |
| Hook   | `hooks/*.yaml` | YAML                        |

## Agents

```markdown
---
name: code-reviewer
description: Reviews diffs for bugs, style, and architectural issues.
tools: [Read, Grep, Bash]
model: sonnet
---

You are a code reviewer. Examine the diff for:
- Logic bugs and edge cases
- Style consistency
- Security issues

Report concise findings with `file:line` references.
```

| Field | Required | Description |
|-------|----------|-------------|
| `name` | yes | Agent identifier |
| `description` | no | One-liner shown in tool listings |
| `tools` | no | Tools the agent may invoke |
| `model` | no | `sonnet`, `opus`, `haiku` |

## Skills

```markdown
---
name: yaml-validator
description: Validate YAML against a schema.
---

# YAML Validator

## Steps
1. Read target file
2. Parse YAML
3. Compare against `schema.yaml`
4. Report violations as `path: message`
```

## Rules

```markdown
---
name: conventional-commits
description: Always use Conventional Commits format.
globs: "**/*"
alwaysApply: true
---

Use `feat:`, `fix:`, `docs:`, etc. Subject under 72 chars.
```

| Field | Description |
|-------|-------------|
| `name` | Rule identifier |
| `description` | Short summary |
| `globs` | File patterns where this applies (Cursor) |
| `alwaysApply` | Always inject, or only on matching context (Cursor) |

## Hooks

Only Claude Code emits hooks natively. Other adapters skip with a warning.

`hooks/format-on-save.yaml`:

```yaml
name: format-on-save
description: Run formatter after Edit/Write tools modify files.
event: PostToolUse
matcher: "Edit|Write"
command: "npx prettier --write \"$CLAUDE_FILE_PATHS\""
```

| Field | Description |
|-------|-------------|
| `name` | Identifier |
| `event` | `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, etc |
| `matcher` | Regex on tool name or event-specific selector |
| `command` | Shell command |

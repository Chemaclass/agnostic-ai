# Spec format

| Kind   | Source                          | Format                      |
|--------|---------------------------------|-----------------------------|
| Agent  | `agents/*.md`                   | Markdown + YAML frontmatter |
| Skill  | `skills/*.md` or `skills/<name>/SKILL.md` | Markdown + YAML frontmatter |
| Rule   | `rules/*.md`                    | Markdown + YAML frontmatter |
| Hook   | `hooks/*.yaml`                  | YAML                        |

Spec discovery is recursive. Any `.md` under `agents/`, `skills/`, `rules/` is picked up; any `.yaml` under `hooks/`.

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

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | no | filename without `.md` | Agent identifier. Used for the output filename. |
| `description` | no | empty | One-liner shown in tool listings. |
| `tools` | no | unset | Tools the agent may invoke. Format depends on target CLI. |
| `model` | no | unset | Preferred model. Common values: `sonnet`, `opus`, `haiku`. |

Any other frontmatter fields pass through to the target CLI unchanged.

## Skills

Two layouts supported:

**Flat:**
```
skills/yaml-validator.md
```

**Nested (preferred for skills with attached resources):**
```
skills/yaml-validator/SKILL.md
skills/yaml-validator/schema.yaml
```

Only `SKILL.md` and flat `*.md` files are parsed. Other files in nested skill directories are ignored by agnostic-ai but available to Claude Code at runtime.

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

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | no | dir or filename | Skill identifier. Used for the output directory. |
| `description` | no | empty | One-liner shown when the model decides whether to invoke the skill. |

Skills are emitted natively by Claude Code only. Other targets log a warning and skip.

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

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | no | filename | Rule identifier. |
| `description` | no | empty | Short summary. |
| `globs` | no | `**/*` | File patterns where this rule applies. Used by Cursor. |
| `alwaysApply` | no | `true` for rules, `false` for agents emitted as Cursor rules | Whether the rule injects unconditionally or only on matching context. |

## Hooks

Pure YAML, no markdown body.

```yaml
name: format-on-save
description: Run formatter after Edit/Write tools modify files.
event: PostToolUse
matcher: "Edit|Write"
command: "npx prettier --write \"$CLAUDE_FILE_PATHS\""
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | no | filename | Hook identifier. |
| `description` | no | empty | Free-form documentation. |
| `event` | yes | none | Hook event. See list below. |
| `matcher` | no | empty | Regex on tool name (or other event-specific selector). |
| `command` | yes | none | Shell command to run when triggered. |

### Supported events (Claude Code)

| Event | When it fires |
|-------|---------------|
| `PreToolUse` | Before any tool call. Matcher is the tool name regex. |
| `PostToolUse` | After any tool call. Matcher is the tool name regex. |
| `UserPromptSubmit` | Before the model reads a new user message. |
| `Stop` | When the model stops generating. |
| `Notification` | When Claude Code surfaces a system notification. |

Hooks are emitted natively by Claude Code only. Other targets log a warning and skip. Refer to Claude Code documentation for the full event list and matcher semantics.

## Frontmatter rules

- Frontmatter is YAML between two `---` lines at the top of the file.
- Empty frontmatter (`---\n---\n`) is allowed; it is treated as no metadata.
- Files with no frontmatter are still loaded; the spec name defaults to the filename.
- Malformed frontmatter is treated as no metadata, with the entire content used as the body.
- Pass-through: any field not listed above is preserved on emit. Useful for target-specific extensions.

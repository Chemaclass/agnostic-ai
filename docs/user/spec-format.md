# Spec format

| Kind   | Source                          | Format                      |
|--------|---------------------------------|-----------------------------|
| Agent  | `agents/*.md`                   | Markdown + YAML frontmatter |
| Skill  | `skills/*.md` or `skills/<name>/SKILL.md` | Markdown + YAML frontmatter |
| Rule   | `rules/*.md`                    | Markdown + YAML frontmatter |
| Hook   | `hooks/*.yaml`                  | YAML                        |
| MCP    | `mcps/*.yaml`                   | YAML                        |

Discovery is recursive. Any `.md` under `agents/`, `skills/`, `rules/` is picked up; any `.yaml` under `hooks/` and `mcps/`.

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

Two layouts:

**Flat:**
```
skills/yaml-validator.md
```

**Nested (for skills with attached resources):**
```
skills/yaml-validator/SKILL.md
skills/yaml-validator/schema.yaml
```

Only `SKILL.md` and flat `*.md` are parsed. Other files in nested skill directories are ignored by agnostic-ai but available to Claude Code at runtime.

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

Emitted natively by Claude Code only. Other targets log a warning and skip.

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

Emitted natively by Claude Code only. Other targets log a warning and skip. See Claude Code docs for the full event list and matcher semantics.

## MCP servers

Pure YAML, no markdown body. One file per MCP server.

```yaml
name: filesystem
description: Local filesystem access for the model.
type: stdio
command: npx
args:
  - -y
  - "@modelcontextprotocol/server-filesystem"
env:
  ROOT: /tmp
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | yes | none | Server identifier. Becomes the key in the generated config. |
| `description` | no | empty | Free-form documentation. |
| `type` | no | `stdio` | Transport: `stdio`, `http`, or `sse`. |
| `command` | stdio only | none | Executable to launch. |
| `args` | no | empty | Argument list for the command. |
| `env` | no | empty | Environment variables passed to the server. |
| `url` | http/sse only | none | Endpoint URL. |
| `headers` | no | empty | HTTP headers for `http`/`sse` transports. |

Targets with native MCP propagation: Claude Code (`.mcp.json`), Cursor (`.cursor/mcp.json`), GitHub Copilot / VS Code (`.vscode/mcp.json`). Other targets log a warning and skip.

## Frontmatter rules

- YAML between two `---` lines at the top of the file.
- Empty (`---\n---\n`) is allowed and treated as no metadata.
- Files without frontmatter still load; name defaults to the filename.
- Malformed frontmatter is treated as no metadata; entire content becomes the body.
- Any field not listed above passes through on emit. Useful for target-specific extensions.

## Target-specific extensions: `x-<target>` namespace

Use `x-<target>:` blocks to attach fields only one adapter consumes. Other
adapters strip the block on emit, so the spec stays portable.

```markdown
---
name: code-reviewer
description: Reviews diffs.
model: sonnet
x-claude:
  allowed-tools: [Read, Grep, Bash]
x-cursor:
  globs: "src/**"
  alwaysApply: false
---
```

Resolution per target:

- All `x-*` keys are dropped first.
- The matching `x-<target>` block is flattened into top-level meta.
- Flattened keys override top-level keys with the same name.

Examples:

| Target | Resulting frontmatter |
|--------|-----------------------|
| `claude` | `name`, `description`, `model`, `allowed-tools` |
| `cursor` | `name`, `description`, `model`, `globs`, `alwaysApply` |
| `gemini` | `name`, `description`, `model` |

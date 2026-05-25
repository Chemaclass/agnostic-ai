# Spec format

| Kind    | Source                                    | Format                      |
|---------|-------------------------------------------|-----------------------------|
| Agent   | `agents/*.md`                             | Markdown + YAML frontmatter |
| Skill   | `skills/*.md` or `skills/<name>/SKILL.md` | Markdown + YAML frontmatter |
| Rule    | `rules/*.md`                              | Markdown + YAML frontmatter |
| Hook    | `hooks/*.yaml`                            | YAML                        |
| MCP     | `mcps/*.yaml`                             | YAML                        |
| Command | `commands/*.md`                           | Markdown + YAML frontmatter |

Discovery is recursive. Any `.md` under `agents/`, `skills/`, `rules/`, `commands/` is picked up; any `.yaml` under `hooks/` and `mcps/`.

## Nested layout: per-directory scope

Specs placed in subdirectories of their source dir carry an implicit
**scope** equal to that subpath. Example:

```
rules/
├── conventional-commits.md      # scope: ""    (root)
├── backend/
│   └── auth.md                  # scope: "backend"
└── backend/api/
    └── limits.md                # scope: "backend/api"
```

Adapters that produce per-directory output honor the scope:

| Target          | Scoped layout                              |
|-----------------|--------------------------------------------|
| `codex`         | `<scope>/AGENTS.md` (already routed via globs; layout wins) |
| `cursor`        | `<scope>/.cursor/rules/<name>.mdc`         |
| `cline`         | `<scope>/.clinerules/<name>.md`            |
| `windsurf`      | `<scope>/.windsurf/rules/<name>.md`        |
| `continue`      | `<scope>/.continue/rules/<name>.md`        |
| `copilot`       | `<scope>/**` applied to a single `.github/instructions/<name>.instructions.md` (no nested dirs; the glob targets the scope) |
| `gemini`        | `<scope>/GEMINI.md`                        |
| `amp`           | `<scope>/AGENTS.md`                        |
| `warp`          | `<scope>/AGENTS.md`                        |

Single-document targets (`claude` CLAUDE.md, `aider` CONVENTIONS.md) merge regardless of scope. The scope is preserved as part of the source provenance comment (`<!-- source: rules/backend/auth.md -->`).

A frontmatter `scope:` field is also accepted as a fallback when source layout is impractical (e.g. a single rule that needs to apply only in a subtree without moving the file).

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

Only `SKILL.md` and flat `*.md` are parsed. Other files in nested skill directories (scripts, templates, fixtures, nested subdirectories) are copied verbatim to the same relative location under each target's skills dir — useful for shipping a `check.mjs`, `templates/*.tpl`, or `fixtures/*.json` that the skill body references. Executable bits are preserved both directions through import + sync.

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

Emitted natively by Claude Code (`.claude/skills/<name>/SKILL.md`) and
Codex (`.agents/skills/<name>/SKILL.md`). Cursor, Cline, Windsurf, and
Continue emit each skill as a rule file (`skill-<name>.{mdc,md}`).
Gemini, Amp, and OpenCode list skills by default and can opt into
native slash-command emission via `outputs.<target>.emit-skills-as-commands: true`.
Other targets (Aider, Copilot, Zed, Warp) list skills in a `## Skills`
section. See [targets](targets.md) for the full matrix.

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
| `target` | no | empty | Single target name. Emits only there. Use to scope a hook to one tool. |
| `targets` | no | empty | List of target names. Emits only to those. Use when a hook makes sense for two tools but not all. |
| `target-exclude` | no | empty | Single target name to block. Emits everywhere else. |
| `targets-exclude` | no | empty | List of target names to block. Emits to every other configured target. |

When none of these fields is set the hook emits to every target that supports hooks (legacy behavior). `target` and `targets` are mutually informative; `target` takes precedence when both appear. Exclude wins: a target in both an include AND an exclude list is excluded.

The same four scoping fields work on every spec kind (agents, skills, rules, commands, mcps), not just hooks. A `target: codex` agent emits only into `.codex/agents/`; a `targets-exclude: [gemini]` skill emits to every configured target except gemini.

### Per-target body fences

When the spec emits to multiple targets but the prose needs to diverge (e.g. codex wants a "Workflow" section claude does not), wrap the divergent prose in `::target` fences. Outside-fence content emits everywhere; inside-fence content emits only to the listed targets. The marker lines themselves never reach the rendered output.

```md
---
name: test
description: Run the test suite
---

# Test

Shared intro paragraph.

::target claude
## Scope mapping (Claude)

| Changed | Command |
|---------|---------|
| ... | ... |
::end

::target codex
## Workflow (Codex)

1. Choose scope
2. Run `composer test`
::end

Shared outro.
```

- `::target <name>` and `::targets <a> <b>` open a fence pinned to one or more targets.
- `::end` closes the most recent fence.
- An unterminated fence runs to end-of-body so a missing `::end` does not drop the tail of the file.
- The empty target (e.g. the source view used by `import` round-trips) returns the body with fences intact so a re-emit stays byte-stable.

```yaml
event: PostToolUse
matcher: apply_patch|Edit|Write
command: "$(git rev-parse --show-toplevel)/.codex/hooks/format-php.sh"
target: codex   # shell-expanded codex path; do not leak to other tools
```

Hooks imported from a tool-native source auto-set `target` to that tool (codex import → `target: codex`, claude import → `target: claude`, gemini import → `target: gemini`). Remove the field by hand if you want the hook to flow everywhere.

`import claude` and `import codex` apply the same auto-scoping to agents and skills: when both `.claude/` and `.codex/` exist on disk but only one carries a given spec, the captured frontmatter gains `target: <tool>`. A spec present in both tools stays un-scoped (cross-emit). Pure single-tool projects also stay un-scoped so byte-identical round-trips still hold.

### Supported events (Claude Code)

| Event | When it fires |
|-------|---------------|
| `PreToolUse` | Before any tool call. Matcher is the tool name regex. |
| `PostToolUse` | After any tool call. Matcher is the tool name regex. |
| `UserPromptSubmit` | Before the model reads a new user message. |
| `Stop` | When the model stops generating. |
| `Notification` | When Claude Code surfaces a system notification. |

Emitted natively by Claude Code (`.claude/settings.json`), Codex (`.codex/config.toml` `[[hooks.<event>]]`), and Gemini (`.gemini/settings.json` `hooks` — uses event names like `BeforeTool`/`AfterTool`). Other targets log a warning and skip. See each tool's docs for its full event list and matcher semantics.

### How agnostic-ai frontmatter renders per target

Agnostic-ai hook spec:

```yaml
event: PostToolUse
matcher: Bash(git commit*)
command: echo "tests please"
```

Renders to `.claude/settings.json`:

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Bash(git commit*)",
        "hooks": [
          {"type": "command", "command": "echo \"tests please\""}
        ]
      }
    ]
  }
}
```

Renders to `.codex/config.toml`:

```toml
[[hooks.PostToolUse]]
matcher = "Bash(git commit*)"
command = "echo \"tests please\""
```

Renders to `.gemini/settings.json`:

```json
{
  "hooks": {
    "AfterTool": [
      {"matcher": "Bash(git commit*)", "command": "echo \"tests please\""}
    ]
  }
}
```

When `command` is a list, each entry becomes a separate hook entry (Claude) or a separate `[[hooks.<event>]]` block (Codex).

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
| `disabled` | no | `false` | When `true`, passes `disabled: true` to targets that support it (Claude Code, Cursor, Copilot). |
| `roots` | no | empty | List of `{uri, name}` objects. Passed to targets that support MCP roots (Claude Code, Cursor, Copilot). |

Targets with native MCP propagation:

| Target | File | Schema |
|--------|------|--------|
| Claude Code | `.mcp.json` | standard `mcpServers` |
| Cursor | `.cursor/mcp.json` | standard `mcpServers` |
| Copilot / VS Code | `.vscode/mcp.json` | `servers` with `type` field |
| Codex | `.codex/config.toml` | `[mcp_servers.<name>]` table |
| Gemini | `.gemini/settings.json` | `mcpServers` (uses `httpUrl` for HTTP) |
| Continue | `.continue/mcpServers/<name>.yaml` | one YAML per server |
| Amp | `.amp/settings.json` | `amp.mcpServers` (dotted key) |
| Zed | `.zed/settings.json` | `context_servers` (stdio only; HTTP bridges via `npx mcp-remote`) |
| Warp | `.warp/.mcp.json` | standard `mcpServers` |
| OpenCode | `opencode.json` | `mcp` with `type: local\|remote` |

Aider, Cline, and Windsurf have no project-scoped MCP file and skip with a warning.

## Commands

Markdown with optional YAML frontmatter. Each spec becomes one native
slash command on supported targets.

```markdown
---
name: deploy
description: Deploy the app to staging.
argument-hint: <env>
---

Deploy the app to {{env}}.

1. Run tests.
2. Build artifacts.
3. Push to the {{env}} environment.
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | no | filename | Command identifier. Becomes the slash name (e.g. `/deploy`). |
| `description` | no | empty | One-liner shown in slash-command pickers. |
| `argument-hint` | no | empty | Hint string shown after the command. Claude-only; passes through. |

Any other frontmatter passes through to the target unchanged. Use the
`x-<target>` namespace for target-specific keys (e.g. `x-claude.allowed-tools`).

Emitted natively by Claude Code (`.claude/commands/<name>.md`) and
Codex (`.codex/prompts/<name>.md`). Other targets log a warning and skip.

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

For Codex agents, `x-codex` fields (`model`, `model_reasoning_effort`,
`sandbox_mode`, `nickname_candidates`) pass through to the generated
`.agents/agents/<name>.toml`. For Codex skills, `x-codex.interface`,
`x-codex.policy`, and `x-codex.dependencies` trigger an additional
`.agents/skills/<name>/agents/openai.yaml` for UI customization, policy,
and tool dependencies.

+++
title = "Augment"
description = "How agnostic-ai emits Augment configuration: native paths, capability limits, and output options."
weight = 250

[extra]
group = "Reference"
target_id = "augment"
+++

# Augment (`augment`)

## Output

```
AGENTS.md                     # entry-point pointer body + inlined rules (written by sync, shared path)
.augment/
├── rules/<name>.md           # one per rule
├── agents/<name>.md          # one per agent
├── commands/<name>.md        # one per command; nested source scope becomes a namespace
└── settings.json             # mcpServers + hooks + toolPermissions, merged; only with MCP, hook, or settings specs
.agents/skills/<name>/SKILL.md  # one folder per skill (shared cross-tool tree)
.augmentignore                # workspace indexing exclusions
.augment-guidelines           # opt-in legacy concatenated rules, only when rules-file is set
```

- **Rules**: [Augment Code](https://docs.augmentcode.com/setup-augment/guidelines) reads the root `AGENTS.md`, with rule bodies inlined into the shared `## Rules` block, and also loads `.augment/rules/`. A spec with `alwaysApply: false` gets `type: agent_requested` and a `description` (falling back to the rule name). The default `always_apply` stays implicit. Rules have no `name` key.
- **Legacy guidelines**: set `outputs.augment.rules-file: .augment-guidelines` to also write the concatenated document. It stays opt-in because Augment truncates it first under budget pressure.
- **Agents**: one `.augment/agents/<name>.md` per agent, with `name` (required), `description` (falls back to the spec name), `color`, and `model` when set. `color` is written verbatim; Augment expects an ANSI color name ([subagents docs](https://docs.augmentcode.com/cli/subagents)).
  - Augment's `tools` and `disabled_tools` use its own tool names (`view`, `codebase-retrieval`, `str-replace-editor`, ...). The generic `tools` field never reaches them and raises a coverage note.
  - Use `x-augment: {tools: [...]}` or `x-augment: {disabled_tools: [...]}` (`x-augment.tools` / `x-augment.disabled_tools`) for per-tool access control.
- **Skills**: emit to the shared `.agents/skills/` tree. Augment also scans `.claude/skills/` and `.augment/skills/`.
- **Commands**: emit to `.augment/commands/<scope>/<name>.md`. A source-layout scope becomes a nested command namespace. `description`, `argument-hint`, and `model` stay in frontmatter.
- **Ignore**: ignore specs emit to `.augmentignore`. `import augment` restores a hand-authored file with pattern order and negation intact.

### MCP

MCP servers merge into `.augment/settings.json` under `mcpServers`. Stdio uses `command`/`args`/`env` with no `type`; remote uses `type: http|sse` plus `url`/`headers`. This is the Auggie CLI project settings file, which Augment recommends for team-shared MCP servers ([config docs](https://docs.augmentcode.com/cli/config)), not the IDE extension's Settings Panel.

- The file also holds `shell`, `startupScript`, `theme`, plugin keys, and tool permissions. Sync merges only its own keys and leaves the rest untouched.
- The file may be JSONC. Sync strips comments and trailing commas before reading, so keys survive but comments do not, and that sync prints a one-line warning.
- Augment documents no per-server `disabled` key, so `disabled: true` is stripped with a coverage note. Use `auggie mcp remove` instead.
- Transports are `stdio`, `sse`, and `http` only ([integrations docs](https://docs.augmentcode.com/cli/integrations)). A `type: ws` spec emits no server and raises a coverage note.

### Hooks

Hooks merge into the same `.augment/settings.json` under `hooks`, in the same write as `mcpServers`. Five events are supported: `PreToolUse`, `PostToolUse`, `Stop`, `SessionStart`, `SessionEnd` ([hooks docs](https://docs.augmentcode.com/cli/hooks)).

- `timeout` is written in **milliseconds**: the spec's seconds value times 1000 (vendor default 60000).
- `command` must be a script path ending in `.sh`, `.ps1`, `.cmd`, or `.bat`. Augment never runs an inline shell string. A command without one of these extensions still emits verbatim, with a coverage note.
- `matcher` is optional on `PreToolUse`/`PostToolUse` (vendor default `.*`) and omitted on the three session events, which do not use it.
- Matchers use Augment's own tool names (`launch-process`, `str-replace-editor`, `save-file`, ...). A Claude-style matcher (`Bash`, `Write`, ...) matches nothing and raises a coverage note.

### Permissions

Settings specs merge into the same file under `toolPermissions`, in the same write. Auggie CLI and Cosmos cloud agents honor it, and Augment recommends committing `.augment/settings.json` to enforce policy on every cloud agent in the repo ([permissions docs](https://docs.augmentcode.com/cli/permissions)).

- Rules are an ordered array, and the first match wins, so deny rules emit ahead of allow rules.
- Each rule's `permission` is an **object** such as `{ "type": "deny" }`. Augment drops a rule with a bare-string permission.
- A bare tool name maps onto Augment's six tools (`Read` to `read`, `Bash` to `terminal`, `WebFetch` to `web-fetch`, and so on). `Bash(prefix:*)` becomes a `terminal` rule with `shellInputRegex: ^prefix`, and an exact `Bash(command)` anchors both ends. `mcp__<server>__<tool>` becomes `{tool-name}_{server-name}`.

Three portable inputs reach nothing, each with a coverage note:

- `model`: this file has no documented key for it.
- The `ask` list: Augment's permission types are `allow`, `deny`, `webhook-policy`, and `script-policy`, and none prompts.
- Path- or URL-scoped rules such as `Read(src/**)`: only `terminal` takes a matcher, and a bare `read` would widen the rule to every file.

Set `x-augment.toolPermissions` to write Augment's own rule objects. They pass through verbatim, ahead of the translated ones. Any other `x-augment` key on a settings spec, such as `shell` or `startupScript`, merges into the file as written.

## Config keys

| Key | Default | Notes |
|-----|---------|-------|
| `outputs.augment.rules-dir` | `.augment/rules` | |
| `outputs.augment.agents-dir` | `.augment/agents` | |
| `outputs.augment.skills-dir` | `.agents/skills` | |
| `outputs.augment.commands-dir` | `.augment/commands` | |
| `outputs.augment.ignore-file` | `.augmentignore` | |
| `outputs.augment.rules-file` | unset | opt-in, writes the legacy concatenated `.augment-guidelines` document |
| `outputs.augment.mcp-file` | `.augment/settings.json` | also the hooks and permissions file, since all three merge into the same document |

## Import

`import augment` reads the workspace indexing exclusions and the `toolPermissions` policy from `.augment/settings.json`, for Augment's six tool names. It skips entries that match on `shellInputRegex` (no portable form) and entries with a bare-string `permission` (malformed in Augment). Other Augment surfaces are not imported.

## Verify

1. Install the Augment Code extension ([guidelines docs](https://docs.augmentcode.com/setup-augment/guidelines)).
2. Check the tree: `ls AGENTS.md .augment/rules/ .augment/agents/ .agents/skills/`, plus `.augment-guidelines` when `outputs.augment.rules-file` is set and `.augment/settings.json` when MCP, hook, or settings specs exist.
3. Open the project. Augment reads `AGENTS.md`, `.augment/rules/`, `.augment/agents/`, `.agents/skills/`, `.augment-guidelines` when present, and (through Auggie CLI) `.augment/settings.json`.
4. With hook specs, `auggie` prints no "invalid hook" warning at startup, and a `PreToolUse` hook pointing at a `.sh`/`.ps1`/`.cmd`/`.bat` script runs on the matching tool call.
5. With settings specs, `auggie` prints no dropped-rule warning at startup, a tool covered by `deny` is blocked, and one covered by `allow` runs without approval.

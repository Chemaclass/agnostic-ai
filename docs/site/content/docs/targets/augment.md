+++
title = "Augment"
description = "How agnostic-ai emits Augment configuration: native paths, capability limits, and output options."
weight = 250

[extra]
group = "Reference"
target_id = "augment"
+++

# Augment (`augment`)

[Augment Code](https://docs.augmentcode.com/setup-augment/guidelines) reads the root `AGENTS.md`, `.augment/`, and the shared `.agents/skills/` tree.

## Output

```
AGENTS.md                     # entry-point pointer body + inlined rules (written by sync, shared path)
.augment/
├── rules/<name>.md           # one per rule the root AGENTS.md does not carry, and per agent_requested rule
├── agents/<name>.md          # one per agent
├── commands/<name>.md        # one per command; nested source scope becomes a namespace
└── settings.json             # mcpServers + hooks + toolPermissions, merged; only with MCP, hook, or settings specs
.agents/skills/<name>/SKILL.md  # one folder per skill (shared cross-tool tree)
.augmentignore                # workspace indexing exclusions
.augment-guidelines           # opt-in legacy concatenated rules, only when rules-file is set
```

- **Rules**: rule bodies inline into the shared `## Rules` block of `AGENTS.md`, and Augment also loads `.augment/rules/`.
  - An always-applied rule whose text matches the block gets no `.augment/rules/` file. See [target behavior](@/docs/target-behavior.md#entry-point-files).
  - A spec with `alwaysApply: false` gets `type: agent_requested` and a `description` (falling back to the rule name). Rules have no `name` key.
- **Legacy guidelines**: `outputs.augment.rules-file: .augment-guidelines` also writes the concatenated document. It stays opt-in, because Augment truncates it first under budget pressure.
- **Agents** ([subagents docs](https://docs.augmentcode.com/cli/subagents)): `.augment/agents/<name>.md` with `name` (required), `description` (falls back to the spec name), and, when set, `color` (written verbatim; Augment expects an ANSI color name) and `model`.
  - Augment's `tools` and `disabled_tools` take its own tool names (`view`, `codebase-retrieval`, `str-replace-editor`, ...), so the generic `tools` field raises a coverage note. Set `x-augment: {tools: [...]}` or `x-augment: {disabled_tools: [...]}` (`x-augment.tools` / `x-augment.disabled_tools`) instead.
- **Skills**: Augment also scans `.claude/skills/` and `.augment/skills/`.
- **Commands**: `.augment/commands/<scope>/<name>.md`, with a source-layout scope as a nested namespace. `description`, `argument-hint`, and `model` stay in frontmatter.
- **Ignore**: `import augment` restores a hand-authored `.augmentignore`, keeping pattern order and negation.

{% <details summary="Entry point off AGENTS.md"> %}
A `file` output override can move the entry point off the root `AGENTS.md`, or `AGENTS.md` can be in `sync.unmanaged`, which stops sync writing it, so codex and every other reader miss rule changes there. Either way every rule gets a `.augment/rules/` file, and the default `always_apply` stays implicit.
{% </details> %}

### MCP

Servers merge into `.augment/settings.json` under `mcpServers`: stdio as `command`/`args`/`env` with no `type`, remote as `type: http|sse` plus `url`/`headers`. This is the Auggie CLI project file Augment recommends for team-shared servers ([config docs](https://docs.augmentcode.com/cli/config)), not the IDE Settings Panel.

- Sync merges only its own keys, leaving `shell`, `startupScript`, `theme`, plugin keys, and tool permissions alone.
- For a JSONC file, sync strips comments and trailing commas before reading, and prints a one-line warning. Keys survive; comments do not.
- Augment documents no per-server `disabled` key, so `disabled: true` is stripped with a coverage note. Use `auggie mcp remove`.
- Transports are `stdio`, `sse`, and `http` only ([integrations docs](https://docs.augmentcode.com/cli/integrations)). A `type: ws` spec emits no server and raises a coverage note.

### Hooks

Hooks merge in under `hooks`, in one write with `mcpServers`. Supported events: `PreToolUse`, `PostToolUse`, `Stop`, `SessionStart`, `SessionEnd` ([hooks docs](https://docs.augmentcode.com/cli/hooks)). [`agnostic-ai hook run`](@/docs/spec-format/hooks.md#hook-run) runs them with Auggie's payload, shell, and timeout before a session does.

- `timeout` is in **milliseconds**: the spec's seconds times 1000 (vendor default 60000).

- `command` must be a script ending in `.sh`, `.ps1`, `.cmd`, or `.bat`, since Augment never runs an inline shell string. Other commands emit verbatim with a coverage note.
- `matcher` is optional on `PreToolUse`/`PostToolUse` (vendor default `.*`) and omitted on the session events.
- Matchers use Augment's tool names (`launch-process`, `str-replace-editor`, `save-file`, ...). A Claude-style matcher (`Bash`, `Write`, ...) matches nothing and raises a coverage note.

### Permissions

Settings specs merge into the same file under `toolPermissions`, which Auggie CLI and Cosmos cloud agents honor. Augment recommends committing `.augment/settings.json` to enforce policy on every cloud agent in the repo ([permissions docs](https://docs.augmentcode.com/cli/permissions)).

- Rules are an ordered array where the first match wins, so deny rules come first.
- Each `permission` is an **object** such as `{ "type": "deny" }`. Augment drops a bare-string permission.
- Bare tool names map onto Augment's six tools (`Read` to `read`, `Bash` to `terminal`, `WebFetch` to `web-fetch`, ...).
- `Bash(prefix:*)` becomes a `terminal` rule with `shellInputRegex: ^prefix`; an exact `Bash(command)` anchors both ends. `mcp__<server>__<tool>` becomes `{tool-name}_{server-name}`.

These reach nothing, with a coverage note:

- `model`: the file has no documented key.
- The `ask` list: none of Augment's types (`allow`, `deny`, `webhook-policy`, `script-policy`) prompts.
- Path- or URL-scoped rules such as `Read(src/**)`: only `terminal` takes a matcher, and a bare `read` would cover every file.

`x-augment.toolPermissions` writes Augment's rule objects verbatim, ahead of the translated ones. Other `x-augment` keys, such as `shell` or `startupScript`, merge in as written.


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

`import augment` reads the workspace indexing exclusions and the `toolPermissions` policy in `.augment/settings.json` for Augment's six tools. It skips `shellInputRegex` entries (no portable form) and bare-string `permission` entries (malformed in Augment). Nothing else is imported.

## Protected paths

Advisory. This target has no native edit guard that sync writes, so sync prints a coverage note for [protected paths](@/docs/spec-format/settings.md#protected-paths). State the paths in a rule if the agent should know about them.

## Verify

1. Install the Augment Code extension ([guidelines docs](https://docs.augmentcode.com/setup-augment/guidelines)).
2. Check the tree: `ls AGENTS.md .augment/agents/ .agents/skills/`, plus `.augment/rules/` (a rule with `alwaysApply: false`), `.augment-guidelines` (`outputs.augment.rules-file` set), and `.augment/settings.json` (MCP, hook, or settings specs).
3. Open the project. Augment reads `AGENTS.md`, `.augment/rules/`, `.augment/agents/`, `.agents/skills/`, `.augment-guidelines` when present, and (through Auggie CLI) `.augment/settings.json`.
4. With hook specs, `auggie` starts with no "invalid hook" warning, and a `PreToolUse` hook on a `.sh`/`.ps1`/`.cmd`/`.bat` script runs on the matching tool call.
5. With settings specs, `auggie` starts with no dropped-rule warning. A `deny` tool is blocked, and an `allow` tool runs without approval.

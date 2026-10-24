+++
title = "Augment"
description = "What agnostic-ai writes for Augment: file paths, what Augment cannot do, and output options."
weight = 250

[extra]
group = "Reference"
target_id = "augment"

[extra.moved]
set-up-this-tool = "@/docs/targets/augment.md#first-rule"
+++

# Augment (`augment`)

## Set up this tool {#set-up-this-tool .legacy-anchor}

[Augment Code](https://docs.augmentcode.com/setup-augment/guidelines) reads the root `AGENTS.md`, `.augment/`, and the shared `.agents/skills/` tree.

## First rule

Select `augment` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule) to add the shared commit-prefix rule and sync it. With the default paths, it reaches `AGENTS.md`.

MCP, hooks, and permission settings on this page apply to Auggie CLI; the IDE reads the project rules and skills.

Already configured? [Import](#import) reads ignore patterns and selected permission rules only. Review the remaining native files before syncing.

## Verify

Run `agnostic-ai sync --check` and inspect `AGENTS.md` for the commit-prefix rule. Open the project in Augment and ask for a commit subject for a small fix; it should use `fix:`.

Next, [add a skill](@/docs/spec-format/skills.md).

## Output

```
AGENTS.md                     # entry-point pointer body + rules (written by sync, shared path)
.augment/
├── rules/<name>.md           # one per rule the root AGENTS.md does not carry, and per agent_requested rule
├── agents/<name>.md          # one per agent
├── commands/<name>.md        # one per command; nested source scope becomes a namespace
└── settings.json             # mcpServers + hooks + toolPermissions, merged; only with MCP, hook, or settings specs
.agents/skills/<name>/SKILL.md  # one folder per skill (shared cross-tool tree)
.augmentignore                # workspace indexing exclusions
.augment-guidelines           # opt-in legacy joined rules, only when rules-file is set
```

## Native mappings

<details>
<summary>Field mappings, discovery, and compatibility</summary>


- **Rules**: rule bodies go into the shared `## Rules` block of `AGENTS.md`. Augment also loads `.augment/rules/`.
  - An always-applied rule that is already in the block gets no `.augment/rules/` file. See [target behavior](@/docs/target-behavior.md#entry-point-files).
  - A spec with `alwaysApply: false` gets `type: agent_requested` and a `description` (falling back to the rule name). Rules have no `name` key.
- **Legacy guidelines**: `outputs.augment.rules-file: .augment-guidelines` also writes all rules as one document. It is opt-in, because Augment cuts it first when context runs short.
- **Agents** ([subagents docs](https://docs.augmentcode.com/cli/subagents)): `.augment/agents/<name>.md` with `name` (required), `description` (falls back to the spec name), and, when set, `color` (written as is; Augment expects an ANSI color name) and `model`.
  - Augment's `tools` and `disabled_tools` take its own tool names (`view`, `codebase-retrieval`, `str-replace-editor`, ...), so the generic `tools` field raises a sync note. Set `x-augment: {tools: [...]}` or `x-augment: {disabled_tools: [...]}` instead.
- **Skills**: Augment also scans `.claude/skills/` and `.augment/skills/`.
- **Commands**: `.augment/commands/<scope>/<name>.md`, with a directory scope from the source folder as a nested folder name. `description`, `argument-hint`, and `model` stay in frontmatter.
- **Ignore**: ignore specs write `.augmentignore`. `import augment` restores a hand-written one, keeping pattern order and negation.

<details>
<summary>Entry point off AGENTS.md</summary>

Moving the entry point off root `AGENTS.md` with a `file` override, or listing it in `sync.unmanaged`, makes every rule keep a `.augment/rules/` file. The default `always_apply` is left out. Before making the shared file unmanaged, check [its effect on other targets](@/docs/target-behavior.md#inlined-rules).

</details>

### MCP

Servers merge into `.augment/settings.json` under `mcpServers`. Stdio uses `command`, `args`, and `env` with no `type`. Remote uses `type: http` or `type: sse` plus `url` and `headers`. This is the Auggie CLI project file Augment recommends for team-shared servers ([config docs](https://docs.augmentcode.com/cli/config)), not the IDE Settings Panel.

- Sync changes only its own keys. `shell`, `startupScript`, `theme`, plugin keys, and tool permissions stay.
- If the file is JSONC, sync drops comments and trailing commas and prints a one-line warning. Keys stay, comments do not.
- Augment documents no per-server `disabled` key, so `disabled: true` is dropped with a sync note. Use `auggie mcp remove`.
- Transports are `stdio`, `sse`, and `http` only ([integrations docs](https://docs.augmentcode.com/cli/integrations)). A `type: ws` spec writes no server and raises a sync note.

### Hooks

Hooks merge in under `hooks`, in the same write as `mcpServers`. Supported events: `PreToolUse`, `PostToolUse`, `Stop`, `SessionStart`, `SessionEnd` ([hooks docs](https://docs.augmentcode.com/cli/hooks)). [`agnostic-ai hook run`](@/docs/cli-reference/hook-diagnostics.md#hook-run) tests them before a session, using Auggie's event data, shell, and timeout.

- `timeout` is in **milliseconds**: the spec's seconds times 1000 (vendor default 60000).
- `command` must be a script ending in `.sh`, `.ps1`, `.cmd`, or `.bat`, because Augment never runs an inline shell string. Other commands are written as is with a sync note. A hook with `args` is skipped with a note, because Augment has no shell to fold them into.
- `matcher` is optional on `PreToolUse` and `PostToolUse` (vendor default `.*`) and left out on the session events.
- Matchers use Augment's tool names (`launch-process`, `str-replace-editor`, `save-file`, ...). A Claude-style matcher (`Bash`, `Write`, ...) matches nothing and raises a sync note.

### Shared memory

With `builtins: [memory]`, sync adds an executable `.augment/hooks/memory-augment-session-start.sh` and a `SessionStart` entry for Auggie CLI on its [supported platforms](https://docs.augmentcode.com/cli/setup-auggie/install-auggie-cli#system-requirements): macOS, Linux, and Windows WSL. The script finds the project from its working directory and returns both memory indexes through `hookSpecificOutput.additionalContext`. The same script is installed for `sync --global`.

The IDE keeps the memory rule. See [shared memory](@/docs/memory.md#load-at-session-start) for the context limit and how to keep the tool's own memory.

### Permissions

Settings specs merge into the same file under `toolPermissions`, which Auggie CLI and Cosmos cloud agents follow. Augment recommends committing `.augment/settings.json` so the policy applies to every cloud agent in the repo ([permissions docs](https://docs.augmentcode.com/cli/permissions)).

- Rules are an ordered array and the first match wins, so deny rules come first.
- Each `permission` is an **object** such as `{ "type": "deny" }`. Augment ignores a bare-string permission.
- Bare tool names map onto Augment's six tools (`Read` to `read`, `Bash` to `terminal`, `WebFetch` to `web-fetch`, ...).
- `Bash(prefix:*)` becomes a `terminal` rule with `shellInputRegex: ^prefix`. An exact `Bash(command)` is anchored at both ends. `mcp__<server>__<tool>` becomes `{tool-name}_{server-name}`.

These are not written and raise a sync note:

- `model`: the file has no documented key.
- The `ask` list: none of Augment's types (`allow`, `deny`, `webhook-policy`, `script-policy`) prompts.
- Path- or URL-scoped rules such as `Read(src/**)`: only `terminal` takes a matcher, and a bare `read` would cover every file.

`x-augment.toolPermissions` writes Augment's rule objects as is, ahead of the translated ones. Other `x-augment` keys, such as `shell` or `startupScript`, merge in as written.

</details>

## Config keys

| Key | Default | Notes |
|-----|---------|-------|
| `outputs.augment.rules-dir` | `.augment/rules` | |
| `outputs.augment.agents-dir` | `.augment/agents` | |
| `outputs.augment.skills-dir` | `.agents/skills` | |
| `outputs.augment.commands-dir` | `.augment/commands` | |
| `outputs.augment.ignore-file` | `.augmentignore` | |
| `outputs.augment.rules-file` | unset | opt-in, writes the legacy `.augment-guidelines` document |
| `outputs.augment.mcp-file` | `.augment/settings.json` | also the hooks and permissions file, because all three merge into one document |

## Import

`import augment` reads `.augmentignore` and the `toolPermissions` policy in `.augment/settings.json` for Augment's six tools. It skips `shellInputRegex` entries (no portable form) and bare-string `permission` entries (invalid in Augment). Nothing else is imported.

## Protected paths

Not enforced. Sync cannot write an edit-blocking rule or hook for this tool, so it prints a [protected-path coverage note](@/docs/spec-format/settings.md#protected-paths).

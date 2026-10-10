+++
title = "OpenCode"
description = "What agnostic-ai writes for OpenCode: file paths, what OpenCode cannot do, and output options."
weight = 130

[extra]
group = "Reference"
target_id = "opencode"
+++

# OpenCode (`opencode`)

OpenCode reads the root `AGENTS.md`, `.opencode/`, and `opencode.json`. Hooks become TypeScript plugins.

## Output

```
AGENTS.md                                 # entry-point pointer body + rules (written by sync)
.opencode/agents/<name>.md                # one subagent per agent
.opencode/skills/<name>/SKILL.md          # one folder per skill, bundled assets included
.opencode/commands/<name>.md              # one per command spec
.opencode/commands/skill-<name>.md        # extra command form, only when emit-skills-as-commands: true
.opencode/plugins/<name>.ts               # one plugin module per hook spec
opencode.json                             # when MCP entries exist (merged with user config)
```

- **Rules**: [OpenCode](https://opencode.ai/docs/rules/) finds the repo-root `AGENTS.md` by walking up from the current directory. Codex, Amp, Warp, and the other `AGENTS.md` tools share the same file, so `sync` writes it once.
- **Agents** ([docs](https://opencode.ai/docs/agents/)): `.opencode/agents/<name>.md` (the singular directory is legacy), with `description`, `mode`, `model`, `temperature`, `permission`, and any `x-opencode` key. The body is the system prompt.
- **Skills** ([docs](https://opencode.ai/docs/skills/)): `.opencode/skills/<name>/SKILL.md` plus bundled files. OpenCode also scans `.claude/skills/` and `.agents/skills/`. A source-layout scope moves the tree under that directory, and import keeps it.
  - `outputs.opencode.emit-skills-as-commands: true` also writes the command form. Its relative links to bundled files point into the skill folder (`../skills/<name>/references/x.md`). Other links stay as written.
  - Names must be 1-64 lowercase letters or digits, with single hyphens between segments (the vendor's `name` rule, which Zed also enforces). `Deploy`, `my_skill`, and `my--skill` fail sync with the required format. Sync does not rename them, because OpenCode would skip the folder.
- **Commands**: `.opencode/commands/<name>.md` keeps `description`, `agent`, `model`, and `subtask`.
- **Hooks**: one [OpenCode plugin](https://opencode.ai/docs/plugins/) module per hook spec. This is the only generated output that is code. OpenCode loads every JS or TS file in `.opencode/plugins/` at startup.
  - Modules follow the vendor's example: `import type { Plugin } from "@opencode-ai/plugin"`, then `export const <Name>Plugin: Plugin = async ({ $ }) => { return { ... } }`. The type-only import is removed at runtime, so `@opencode-ai/plugin` is optional. A `//` comment at the top marks the file as generated.
  - `PreToolUse` and `PostToolUse` map to `tool.execute.before` and `tool.execute.after`. OpenCode's dotted names are kept. Other documented events (`session.idle`, `file.edited`, `permission.asked`, ...) use the single `event` hook with an `event.type` guard.
  - `matcher` becomes an anchored `new RegExp("^(?:...)$")` tested against `input.tool`, so `write` does not match `todowrite`. OpenCode tool names are lowercase, so a Claude-style `Edit` or `Edit|Write` is written as is with a coverage note.
  - Claude's `*` gets no guard. Neither does a regex that JavaScript reads differently from RE2 (`(?i)`, `\A`, `(?P<name>`), and that case adds a note.
  - Bun's `$` runs each command as written. On `tool.execute.before`, exit code 2 blocks the call and stderr is the reason. Other failures log a warning, and the next command runs. This includes commands Bun's shell cannot parse (no `>&2`).
  - `disabled: true` writes no module, because OpenCode runs every module in the directory.
  - `matcher` on an event hook raises a coverage note, because event data has no tool name. Any `timeout` raises one too, because OpenCode sets no time limit for handlers.
  - `shell.env` and `experimental.session.compacting` are declined with a note. They rewrite output that a command spec cannot express.
  - `import opencode` does not read `.opencode/plugins/` back.
- **MCP**: `opencode.json` in the project root gets a `$schema` link and the `mcp` map.
  - Stdio becomes `{type: "local", command: [...], cwd}`, and HTTP or SSE becomes `{type: "remote", url, headers}`. `cwd` (relative to the workspace) and `timeout` (ms for fetching tools, default 5000) keep their names.
  - `disabled: true` writes `"enabled": false` ([MCP docs](https://opencode.ai/docs/mcp-servers/)), and import reads `enabled: false` back. An enabled server gets no key.
  - Other documented fields, such as the `oauth` object for pre-registered remote servers, go through `x-opencode`.
  - Sync changes only `$schema` and `mcp`. `theme`, `model`, and other keys stay. They never show as drift in `sync --check` or `doctor`, and `doctor --fix` keeps them.
- **Settings**: a settings spec's default `model` merges into `opencode.json`, and import restores it to `settings/opencode.yaml`. An `x-opencode` block merges in too. Its `permission` merges tool by tool with the translated rules.
- **Permissions**: portable `allow`, `deny`, and `ask` lists become [OpenCode's `permission` map](https://opencode.ai/docs/permissions/).
  - A bare tool name covers the whole tool (`Read` becomes `read: allow`). A scoped rule becomes a glob (`Bash(go test:*)` becomes `bash: {"go test *": "allow"}`).
  - `Write` and `Edit` both land on `edit`, which covers edit, write, and patch.
  - If two lists claim one tool and pattern, the stricter wins. OpenCode takes the last match, so sync puts the `*` catch-all first, as OpenCode recommends.
  - `mcp__<server>__<tool>` becomes `<server>_<tool>` (`mcp__github__create_issue` becomes `github_create_issue: deny`), the key OpenCode uses for MCP tools ([agents docs](https://opencode.ai/docs/agents/)). The server name keeps its hyphens.
  - Scoped `WebFetch` or `WebSearch` rules (those keys take a bare action) and denying a whole MCP server (`github_*` has no portable spelling) raise a coverage note.
  - `x-opencode.permission` writes OpenCode's own map and replaces the translated rules for that tool.
  - `import opencode` reads the map back, except namespaced MCP keys, where the server name cannot be told from the tool name.

{% <details summary="Old .opencode/AGENTS.md leftovers"> %}
Sync removes a generated file at the old `.opencode/AGENTS.md` path, which OpenCode never read. It leaves a hand-written one.
{% </details> %}

## Config keys

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.opencode.agents-dir` | `.opencode/agents` | |
| `outputs.opencode.skills-dir` | `.opencode/skills` | |
| `outputs.opencode.commands-dir` | `.opencode/commands` | |
| `outputs.opencode.hooks-dir` | `.opencode/plugins` | OpenCode only loads plugins from its own directory, so moving this stops the hooks loading |
| `outputs.opencode.mcp-file` | `opencode.json` | |
| `outputs.opencode.emit-skills-as-commands` | `false` | |
| `outputs.opencode.rules-file` | unset | writes legacy joined rules and skips the pointer-body write |

With `builtins: [memory]`, sync adds both [shared memory](@/docs/memory.md) indexes to `instructions` and keeps your own entries. Dropping the built-in removes only those two.

With `memory.personal: repo`, sync also adds `"<folder>/**": "allow"` to `permission.external_directory`, so OpenCode saves to the [personal memory folder](@/docs/memory.md#one-store-per-repository) without asking. Your own entries stay in the order you wrote them, and the folder entry goes last, since OpenCode applies the last rule that matches. A bare action you set there, such as `"deny"`, becomes the `"*"` entry. A bare `"permission": "deny"` becomes `{"*": "deny"}`, which means the same and stays after repo mode ends.

## Import

`agnostic-ai import opencode` reads:

- **Instructions and rules**: root `AGENTS.md`, falling back to legacy `.opencode/AGENTS.md` when the root file is absent. The shared body becomes `.agnostic-ai/AGNOSTIC_AI.md`. Rule sections that sync wrote become rule specs. Hand-written instructions stay whole.
- **Agents and commands**: top-level Markdown files in `.opencode/agents/` and `.opencode/commands/`.
- **Skills**: `.opencode/skills/`, `.claude/skills/`, then `.agents/skills/` (first wins within a scope), with scoped paths and bundled assets.
- **MCP servers**: the `mcp` map in `opencode.json`.
- **Settings and permissions**: the default `model` becomes `settings/opencode.yaml`; portable entries in the `permission` map become `settings/permissions-opencode.yaml`. Namespaced MCP permission keys have no portable spelling and stay in the file.

Plugin hooks in `.opencode/plugins/` are not imported.

## Protected paths

Not enforced. Sync cannot write an edit-blocking rule or hook for this tool, so it prints a coverage note for [protected paths](@/docs/spec-format/settings.md#protected-paths). State the paths in a rule if the agent should know about them.

## Verify

1. Install: `npm install -g opencode-ai` ([install docs](https://opencode.ai/docs/#install)).
2. Check the tree:
   - `ls AGENTS.md .opencode/agents/ .opencode/skills/ .opencode/commands/ .opencode/plugins/ opencode.json`
   - `grep "Generated by agnostic-ai" .opencode/agents/*.md` (the header follows the frontmatter)
   - `python -m json.tool opencode.json > /dev/null`
3. Launch `opencode`. `AGENTS.md` rules are in context, and each agent, skill, and command is in its picker.
4. The MCP panel shows each `mcp.<name>` ready, or disabled for disabled specs.
5. Trigger a hook's event and confirm its command ran. OpenCode reports plugins it cannot parse at startup, so a clean launch means every `.opencode/plugins/<name>.ts` loaded.

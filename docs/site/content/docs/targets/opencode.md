+++
title = "OpenCode"
description = "How agnostic-ai emits OpenCode configuration: native paths, capability limits, and output options."
weight = 130

[extra]
group = "Reference"
target_id = "opencode"
+++

# OpenCode (`opencode`)

OpenCode reads the root `AGENTS.md`, `.opencode/`, and `opencode.json`. Hooks become TypeScript plugins.

## Output

```
AGENTS.md                                 # entry-point pointer body + inlined rules (written by sync)
.opencode/agents/<name>.md                # one subagent per agent
.opencode/skills/<name>/SKILL.md          # one folder per skill, bundled assets included
.opencode/commands/<name>.md              # one per command spec
.opencode/commands/skill-<name>.md        # extra command form, only when emit-skills-as-commands: true
.opencode/plugins/<name>.ts               # one plugin module per hook spec
opencode.json                             # when MCP entries exist (merged with user config)
```

- **Routing**: [OpenCode's rules lookup](https://opencode.ai/docs/rules/) finds the repo-root `AGENTS.md` by walking up from the current directory. The AGENTS.md family (Codex, Amp, Warp, ...) shares the same bytes, so `sync` writes it once.
- **Agents** ([docs](https://opencode.ai/docs/agents/)): `.opencode/agents/<name>.md` (the singular directory is legacy), with `description`, `mode`, `model`, `temperature`, `permission`, and any `x-opencode` key. The body is the system prompt.
- **Skills** ([docs](https://opencode.ai/docs/skills/)): `.opencode/skills/<name>/SKILL.md` plus bundled files. OpenCode also scans `.claude/skills/` and `.agents/skills/`. A source-layout scope moves the tree under that directory and survives import.
  - `outputs.opencode.emit-skills-as-commands: true` also emits the command form. Its relative links to bundled files point into the skill folder (`../skills/<name>/references/x.md`); other links stay as written.
  - Names must be 1-64 lowercase letters or digits, with single hyphens between segments (the vendor's `name` rule, also enforced by Zed). `Deploy`, `my_skill`, or `my--skill` fail sync with the required format and are not renamed, since OpenCode would skip the folder.
- **Commands**: `.opencode/commands/<name>.md` keeps `description`, `agent`, `model`, and `subtask`.
- **Hooks**: one [OpenCode plugin](https://opencode.ai/docs/plugins/) module per hook spec. This is the only generated output that is code. OpenCode loads every JS or TS file in `.opencode/plugins/` at startup.
  - Modules follow the vendor's example: `import type { Plugin } from "@opencode-ai/plugin"`, then `export const <Name>Plugin: Plugin = async ({ $ }) => { return { ... } }`. The type-only import disappears at runtime, so `@opencode-ai/plugin` is optional. A `//` provenance comment sits at the top of the file.
  - `PreToolUse` and `PostToolUse` map to `tool.execute.before` and `tool.execute.after`. OpenCode's dotted names pass through. Other documented events (`session.idle`, `file.edited`, `permission.asked`, ...) use the single `event` hook with an `event.type` guard.
  - `matcher` becomes an anchored `new RegExp("^(?:...)$")` tested against `input.tool`, so `write` does not match `todowrite`. OpenCode tool names are lowercase, so a Claude-style `Edit` or `Edit|Write` emits with a coverage note.
  - Claude's `*` emits no guard. Neither does a regex that JavaScript reads differently from RE2 (`(?i)`, `\A`, `(?P<name>`); that case adds a note.
  - Bun's `$` runs each command as written. On `tool.execute.before`, exit code 2 blocks the call with stderr as the reason. Other failures log a warning, and the next command runs. This includes commands Bun's shell can't parse (no `>&2`).
  - `disabled: true` writes no module, since OpenCode runs every module in the directory.
  - `matcher` on an event-bus hook raises a coverage note, since event payloads carry no tool name. Any `timeout` raises one too, since OpenCode sets no handler deadline.
  - `shell.env` and `experimental.session.compacting` are declined with a note. They rewrite output that a command spec can't express.
  - `import opencode` does not read `.opencode/plugins/` back.
- **MCP**: `opencode.json` at the project root gets a `$schema` link and the `mcp` map.
  - Stdio maps to `{type: "local", command: [...], cwd}`, HTTP/SSE to `{type: "remote", url, headers}`. `cwd` (relative to the workspace) and `timeout` (ms for fetching tools, default 5000) keep their names.
  - `disabled: true` writes `"enabled": false` ([MCP docs](https://opencode.ai/docs/mcp-servers/)), and import reads `enabled: false` back. Enabled servers get no key.
  - Other documented fields, such as the `oauth` object for pre-registered remote servers, go through `x-opencode`.
  - Sync overwrites only `$schema` and `mcp`. `theme`, `model`, and other keys survive. They never show as drift in `sync --check` or `doctor`, and `doctor --fix` keeps them.
- **Settings**: a settings spec's default `model` merges into `opencode.json`, and import restores it to `settings/opencode.yaml`. An `x-opencode` block merges in too. Its `permission` merges tool by tool with the translated rules.
- **Permissions**: portable `allow`, `deny`, and `ask` lists become [OpenCode's `permission` map](https://opencode.ai/docs/permissions/).
  - A bare tool name covers the whole tool (`Read` becomes `read: allow`). A scoped rule becomes a glob (`Bash(go test:*)` becomes `bash: {"go test *": "allow"}`).
  - `Write` and `Edit` both land on `edit`, which covers edit, write, and patch.
  - If two lists claim one tool and pattern, the stricter wins. OpenCode uses last-match-wins, and sorted keys put the `*` catch-all first, as OpenCode recommends.
  - `mcp__<server>__<tool>` becomes `<server>_<tool>` (`mcp__github__create_issue` becomes `github_create_issue: deny`), the key OpenCode registers MCP tools under ([agents docs](https://opencode.ai/docs/agents/)). The server name keeps its hyphens.
  - Scoped `WebFetch` or `WebSearch` rules (those keys take a bare action) and whole-server MCP denial (`github_*` has no portable spelling) raise a coverage note.
  - `x-opencode.permission` writes OpenCode's shape directly and replaces the translated rules for that tool.
  - `import opencode` reads the map back, except namespaced MCP keys, whose server name boundary is ambiguous.

{% <details summary="Old .opencode/AGENTS.md leftovers"> %}
Sync removes a managed leftover at the old `.opencode/AGENTS.md` path, which OpenCode never read. It leaves a hand-written one in place.
{% </details> %}

## Config keys

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.opencode.agents-dir` | `.opencode/agents` | |
| `outputs.opencode.skills-dir` | `.opencode/skills` | |
| `outputs.opencode.commands-dir` | `.opencode/commands` | |
| `outputs.opencode.hooks-dir` | `.opencode/plugins` | OpenCode only loads plugins from its own directory, so moving this means OpenCode no longer loads the hooks |
| `outputs.opencode.mcp-file` | `opencode.json` | |
| `outputs.opencode.emit-skills-as-commands` | `false` | |
| `outputs.opencode.rules-file` | unset | writes legacy concatenated rules and skips the pointer-body write |

With `builtins: [memory]`, sync adds both [shared memory](@/docs/memory.md) indexes to `instructions` and keeps your own entries. Dropping the built-in removes only those two.

## Import

`agnostic-ai import opencode` reads:

- **Instructions and rules**: root `AGENTS.md`, falling back to legacy `.opencode/AGENTS.md` when the root file is absent. The shared body becomes `.agnostic-ai/AGNOSTIC_AI.md`. Rule sections from generated files become rule specs; hand-written instructions stay whole.
- **Agents and commands**: top-level Markdown files in `.opencode/agents/` and `.opencode/commands/`.
- **Skills**: `.opencode/skills/`, `.claude/skills/`, then `.agents/skills/` (first wins within a scope), with scoped paths and bundled assets.
- **MCP servers**: the `mcp` map in `opencode.json`.
- **Settings and permissions**: the default `model` becomes `settings/opencode.yaml`; portable entries in the `permission` map become `settings/permissions-opencode.yaml`. Namespaced MCP permission keys have no unambiguous portable spelling and stay in the file.

Plugin hooks in `.opencode/plugins/` are not imported.

## Protected paths

Advisory. Sync writes no native edit guard for this target, so it prints a coverage note for [protected paths](@/docs/spec-format/settings.md#protected-paths). State the paths in a rule if the agent should know about them.

## Verify

1. Install: `npm install -g opencode-ai` ([install docs](https://opencode.ai/docs/#install)).
2. Check the tree:
   - `ls AGENTS.md .opencode/agents/ .opencode/skills/ .opencode/commands/ .opencode/plugins/ opencode.json`
   - `grep "Generated by agnostic-ai" .opencode/agents/*.md` (the header follows the frontmatter)
   - `python -m json.tool opencode.json > /dev/null`
3. Launch `opencode`. `AGENTS.md` rules are in context, and each agent, skill, and command is in its picker.
4. The MCP panel shows each `mcp.<name>` ready, or disabled for disabled specs.
5. Trigger a hook's event and confirm its command ran. OpenCode reports unparsable plugins at startup, so a clean launch means every `.opencode/plugins/<name>.ts` loaded.

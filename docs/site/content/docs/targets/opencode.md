+++
title = "OpenCode"
description = "How agnostic-ai emits OpenCode configuration: native paths, capability limits, and output options."
weight = 130

[extra]
group = "Reference"
target_id = "opencode"
+++

# OpenCode (`opencode`)

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

- **Routing**: the entry point is the repo-root `AGENTS.md`, which [OpenCode's rules lookup](https://opencode.ai/docs/rules/) finds by walking up from the current directory. Codex, Amp, Warp, and the rest of the AGENTS.md family share the file with byte-identical content, so `sync` writes it once. A managed leftover at the old `.opencode/AGENTS.md` path, which OpenCode never read, is swept on the next sync; a hand-authored one is left alone.
- **Agents**: native [OpenCode agents](https://opencode.ai/docs/agents/) at `.opencode/agents/<name>.md` (plural; the singular directory is legacy). Frontmatter is filtered to `description`, `mode`, `model`, `temperature`, and `permission`, and any `x-opencode` key passes through. The body is the system prompt.
- **Skills**: native [OpenCode skills](https://opencode.ai/docs/skills/) at `.opencode/skills/<name>/SKILL.md`, with bundled files copied byte-for-byte. OpenCode also scans `.claude/skills/` and `.agents/skills/`. A source-layout scope moves the whole tree under that directory and survives import. Set `outputs.opencode.emit-skills-as-commands: true` to also emit the command form; relative links to bundled files there point into the skill folder (`../skills/<name>/references/x.md`), and other links stay as written.

  Skill names must be 1-64 lowercase letters or digits, with single hyphens between segments (the vendor's `name` rule, also enforced by Zed). Names such as `Deploy`, `my_skill`, or `my--skill` fail sync with the required format rather than being renamed, because OpenCode would skip the folder.
- **Commands**: one Markdown file per command at `.opencode/commands/<name>.md`, frontmatter filtered to `description`, `agent`, `model`, and `subtask`.
- **Hooks**: one [OpenCode plugin](https://opencode.ai/docs/plugins/) module per hook spec at `.opencode/plugins/<name>.ts`. OpenCode loads every JS or TS file in that directory at startup. This is the only generated output here that is code, not config.
  - Each module follows the vendor's TypeScript example: `import type { Plugin } from "@opencode-ai/plugin"`, then `export const <Name>Plugin: Plugin = async ({ $ }) => { return { ... } }`. The type-only import is erased at runtime, so `@opencode-ai/plugin` need not be installed. The provenance header is a `//` comment above the import.
  - `PreToolUse` and `PostToolUse` map to `tool.execute.before` and `tool.execute.after`. OpenCode's own dotted names pass through unchanged. Other documented events (`session.idle`, `file.edited`, `permission.asked`, ...) use the single `event` hook with an `event.type` guard.
  - `matcher` becomes an anchored `new RegExp("^(?:...)$")` tested against `input.tool`, so `write` does not match `todowrite`. A capitalized Claude-style matcher (`Edit`, `Edit|Write`) still emits, with a coverage note, since OpenCode tool names are lowercase. Claude's `*` emits no guard. A regex JavaScript reads differently from RE2 (`(?i)`, `\A`, `(?P<name>`) also emits no guard, with a note.
  - Each command runs through Bun's `$` exactly as written. On `tool.execute.before`, exit code 2 blocks the tool call with stderr as the reason. Any other failure, including a command Bun's shell cannot parse (it has no `>&2`), logs a warning and the next command still runs.
  - `disabled: true` writes no module, since OpenCode runs every module in the directory.
  - `matcher` on an event-bus hook and `timeout` on any hook raise a coverage note: event payloads carry no tool name, and OpenCode sets no handler deadline.
  - `shell.env` and `experimental.session.compacting` are declined with a note, because they rewrite output that a command spec cannot express.
  - Hooks are one-way: `import opencode` does not read `.opencode/plugins/` back.
- **MCP**: written to `opencode.json` at the project root, with a `$schema` link and the `mcp` map.
  - Stdio maps to `{type: "local", command: [...], cwd}`; HTTP/SSE maps to `{type: "remote", url, headers}`.
  - `cwd` (local servers, relative to the workspace) and `timeout` (ms for fetching tools, default 5000) map with no rename.
  - `disabled: true` writes `"enabled": false`, per [OpenCode's MCP docs](https://opencode.ai/docs/mcp-servers/). Enabled servers get no key, and `import opencode` reads `enabled: false` back as `disabled: true`.
  - Other documented fields, including the `oauth` object for pre-registered remote servers, go through `x-opencode`.
  - Only `$schema` and `mcp` are overwritten; other keys (`theme`, `model`) survive. `sync --check` and `doctor` read the existing file, so user keys never report as drift and `doctor --fix` keeps them.
- **Settings**: a settings spec's default `model` merges into `opencode.json`, keeping existing keys. `import opencode` restores it to `settings/opencode.yaml`. An `x-opencode` block on a settings spec merges into the file too, except `permission`, which merges tool by tool with the translated rules.
- **Permissions**: portable `allow`, `deny`, and `ask` lists become [OpenCode's `permission` map](https://opencode.ai/docs/permissions/) in `opencode.json`.
  - A bare tool name covers the whole tool (`Read` becomes `read: allow`). A scoped rule becomes a glob (`Bash(go test:*)` becomes `bash: {"go test *": "allow"}`).
  - `Write` and `Edit` both land on `edit`, which covers edit, write, and patch.
  - When two lists claim the same tool and pattern, the more restrictive wins. OpenCode uses last-match-wins, and keys are sorted, which puts the `*` catch-all first as recommended.
  - `mcp__<server>__<tool>` becomes `<server>_<tool>` (`mcp__github__create_issue` becomes `github_create_issue: deny`), the key OpenCode registers MCP tools under ([agents docs](https://opencode.ai/docs/agents/)). The server name passes through verbatim, hyphens included.
  - Rules with no OpenCode key raise a coverage note: scoped `WebFetch` or `WebSearch` rules (those keys take a bare action), and whole-server MCP denial (OpenCode's `github_*` has no portable spelling).
  - `x-opencode.permission` writes OpenCode's shape directly and replaces the translated rules for that tool.
  - `import opencode` reads the map back, except namespaced MCP keys, which stay in `opencode.json` because the server name boundary is ambiguous.

## Config keys

Skills import from `.opencode/skills/`, `.claude/skills/`, and `.agents/skills/`, in that order for duplicate names within the same scope. Scoped paths and bundled assets survive import.

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.opencode.agents-dir` | `.opencode/agents` | |
| `outputs.opencode.skills-dir` | `.opencode/skills` | |
| `outputs.opencode.commands-dir` | `.opencode/commands` | |
| `outputs.opencode.hooks-dir` | `.opencode/plugins` | OpenCode only loads plugins from its own directory, so moving this takes the hooks out of range |
| `outputs.opencode.mcp-file` | `opencode.json` | |
| `outputs.opencode.emit-skills-as-commands` | `false` | |
| `outputs.opencode.rules-file` | unset | writes legacy concatenated rules and skips the pointer-body write |

## Verify

1. Install: `npm install -g sst/opencode` ([install docs](https://opencode.ai/)).
2. Check the tree: `ls AGENTS.md .opencode/agents/ .opencode/skills/ .opencode/commands/ .opencode/plugins/ opencode.json`, `grep "Generated by agnostic-ai" .opencode/agents/*.md` for the provenance header (after the frontmatter), and `python -m json.tool opencode.json > /dev/null`.
3. Launch `opencode`. Rule bodies from `AGENTS.md` are in context, and each agent, skill, and command appears in its picker.
4. The MCP panel shows each `mcp.<name>` from `opencode.json` ready, and disabled specs as disabled.
5. Trigger a hook's event and confirm its command ran. OpenCode reports a plugin that fails to parse at startup, so a clean launch means every `.opencode/plugins/<name>.ts` loaded.

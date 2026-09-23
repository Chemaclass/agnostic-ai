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
AGENTS.md                                 # canonical entry-point pointer body + inlined rules (written by sync)
.opencode/agents/<name>.md                # one native subagent definition per agent
.opencode/skills/<name>/SKILL.md          # one folder per skill, bundled assets included
.opencode/commands/<name>.md              # one per command spec
.opencode/commands/skill-<name>.md        # additional command form, only when emit-skills-as-commands: true
.opencode/plugins/<name>.ts               # one plugin module per hook spec
opencode.json                             # when MCP entries exist (merged with existing user config)
```

- **Routing**: the entry point is the repo-root `AGENTS.md`.
  - [OpenCode's rules lookup](https://opencode.ai/docs/rules/) walks up the directory tree for it ("Local files by traversing up from the current directory (`AGENTS.md`, `CLAUDE.md`)"), confirmed in the vendor's own `packages/core/src/instruction-context.ts` on branch `dev`: `fs.up({ targets: ["AGENTS.md"] })`.
  - Codex, amp, warp, and the rest of the AGENTS.md family share that same path. The pointer body and the inlined rules block are byte-identical across them, so `sync` writes the file once instead of colliding.
  - Before #623, the adapter wrote `.opencode/AGENTS.md` to stay clear of Codex, but no OpenCode doc or code path ever read it: in a project syncing only opencode, no rule reached the tool at all.
  - A managed leftover at the old path is swept on the next `sync`; a hand-authored one is left alone.
- **Agents**: native [OpenCode agents](https://opencode.ai/docs/agents/) at `.opencode/agents/<name>.md` (plural dir; the singular is legacy). Frontmatter is filtered to `description`, `mode`, `model`, `temperature`, `permission`, with arbitrary `x-opencode` keys passing through. The body is the system prompt.
- **Skills**: native [OpenCode skills](https://opencode.ai/docs/skills/) folders at `.opencode/skills/<name>/SKILL.md` (OpenCode also scans `.claude/skills/` and `.agents/skills/`), with bundled sibling files propagated byte-for-byte. A source-layout scope moves the full native tree under that directory and survives import. Set `outputs.opencode.emit-skills-as-commands: true` to additionally emit the command form. A command file sits away from the skill's bundled files, so a relative link to one points into the native skill folder instead (`../skills/<name>/references/x.md`); links to anything else stay as written.

  Skill names must contain 1-64 lowercase letters or digits, with single hyphens between segments, the regex the vendor states for `name`. Invalid names such as `Deploy`, `my_skill`, and `my--skill` fail sync with the name and required format; they are not renamed. OpenCode skips a folder that breaks the rule, so the skill would never appear in the `skill` tool catalog (#857). Zed enforces the same rule.
- **Commands**: one markdown file per command spec under `.opencode/commands/<name>.md`, frontmatter filtered to the OpenCode command keys (`description`, `agent`, `model`, `subtask`).
- **Hooks**: one [OpenCode plugin](https://opencode.ai/docs/plugins/) module per hook spec at `.opencode/plugins/<name>.ts`. This is the only generated surface here that is code rather than config: the vendor documents `.opencode/plugins/` as a project-level plugin directory whose "JavaScript or TypeScript files ... are automatically loaded at startup", and a plugin is a module exporting a function that returns the hook object.
  - The module matches the vendor's own TypeScript example: `import type { Plugin } from "@opencode-ai/plugin"`, then `export const <Name>Plugin: Plugin = async ({ $ }) => { return { ... } }`. The type-only import is erased before the module runs, so `@opencode-ai/plugin` does not have to be installed for the plugin to load. Only `$` is destructured, because that is all the generated body reads.
  - The provenance header is a `//` line comment at the top of the file, above the import. Markdown gets an HTML comment and TOML a `#` comment; TypeScript gets the comment syntax its parser accepts.
  - `PreToolUse` and `PostToolUse` map onto the `tool.execute.before` and `tool.execute.after` keys, which the vendor's own `.env` protection example returns from the plugin function. OpenCode's own dotted spellings pass through unchanged.
  - Every other documented event (`session.idle`, `file.edited`, `permission.asked`, ...) rides the single `event` hook with an `event.type` guard, the shape the vendor's notification example uses.
  - `matcher` becomes a `new RegExp(...).test(input.tool)` guard on the two tool hooks. A Claude-style `Edit` or `Bash` still emits, with a coverage note: OpenCode's own tool names are lowercase, so the capitalized form compiles and then matches nothing. Claude's `*` wildcard and any value that does not compile emit no guard at all, since `new RegExp("*")` throws at load and would stop OpenCode reading the plugin.
  - `matcher` on an event-bus hook and `timeout` on any hook each raise a coverage note. The event payload carries no tool name, and OpenCode awaits a hook handler with no deadline.
  - `shell.env` and `experimental.session.compacting` are declined with a note rather than guessed at. Both exist to rewrite `output.env`, `output.context`, or `output.prompt`, which a spec carrying a command to run cannot express.
  - Hooks are one-way. `import opencode` does not read `.opencode/plugins/` back, because recovering a spec would mean parsing TypeScript.
- **MCP**: written into `opencode.json` at the project root with a `$schema` link and the `mcp` map.
  - Each entry carries a `type: local|remote` key. Stdio maps to `{type: "local", command: [...], cwd}`; HTTP/SSE/remote maps to `{type: "remote", url, headers}`.
  - `cwd` is documented on the local-server table only ("Working directory for the MCP server process. Relative paths resolve from the workspace."). `timeout` is documented on both ("Timeout in ms for fetching tools from the MCP server. Defaults to 5000 (5 seconds)."). Both names match the spec field exactly, so both map top-level with no rename (target-audit 2026-09-03, #641).
  - A spec's `disabled: true` writes `"enabled": false`, the key [OpenCode's own MCP docs](https://opencode.ai/docs/mcp-servers/) document ("You can also disable a server by setting `enabled` to `false`"). An enabled server gets no key at all, and `import opencode` reads `enabled: false` back into `disabled: true`.
  - Any other documented field, including the `oauth` client-credentials object for a "Pre-registered" remote server (same doc), reaches the entry through `x-opencode`, the same passthrough commands and agents already have.
  - Pre-existing non-managed keys (`theme`, `model`) are preserved; only `$schema` and `mcp` are overwritten. Drift checks (`sync --check`, `doctor`) read the existing file, so user keys never report as drift and `doctor --fix` keeps them.
- **Settings**: a settings spec's default `model` merges into the same `opencode.json`. Existing native keys survive. `import opencode` restores the field to `settings/opencode.yaml`. An `x-opencode` block on a settings spec merges into that file too, for the keys this tool does not model; `permission` is the exception, since it already merges tool by tool with the translated rules.
- **Permissions**: the portable `allow`, `deny`, and `ask` lists translate into [OpenCode's `permission` map](https://opencode.ai/docs/permissions/) in `opencode.json`. A bare tool name covers the whole tool (`Read` becomes `read: allow`); a scoped rule becomes a pattern (`Bash(go test:*)` becomes `bash: {"go test *": "allow"}`), since OpenCode matches a bare glob rather than this project's `:*` convention. `Write` and `Edit` both land on `edit`, which the vendor says covers edit, write and patch. Two lists claiming the same tool and pattern resolve to the more restrictive one. OpenCode evaluates last-match-wins, and keys are written sorted, which puts the `*` catch-all first as the vendor recommends. An `mcp__<server>__<tool>` rule becomes the key OpenCode registers that tool under, `<server>_<tool>`, so `mcp__github__create_issue` becomes `github_create_issue: deny`. "Permission keys are matched as wildcard patterns against the underlying tool name, so the same syntax works for built-ins, custom tools, and MCP tools" ([opencode.ai/docs/agents](https://opencode.ai/docs/agents/)), and the server name passes through verbatim, hyphens included. Rules with no OpenCode key raise a coverage note: any scoped `WebFetch` or `WebSearch` rule, since those keys take a bare action with no pattern object, and whole-server MCP denial, since a portable `mcp__` rule names one tool and OpenCode's `github_*` wildcard has no portable spelling. Set `x-opencode.permission` to write OpenCode's own shape directly; it replaces the translated rules for that tool. `import opencode` reads the map back, apart from the namespaced MCP keys: nothing in `github_create_issue` says where the server name ends, so it stays in `opencode.json` untouched.

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
2. Check the tree: `ls AGENTS.md .opencode/agents/ .opencode/skills/ .opencode/commands/ .opencode/plugins/ opencode.json`, `grep "Generated by agnostic-ai" .opencode/agents/*.md` for the provenance header (it sits after the frontmatter), `python -m json.tool opencode.json > /dev/null`.
3. Launch `opencode`. Every rule body from `AGENTS.md` is in context, every `.opencode/agents/<name>.md` appears in the agent picker, every `.opencode/skills/<name>/` in the skills list, and every `.opencode/commands/<name>.md` in the slash-command picker.
4. The MCP panel shows each `mcp.<name>` from `opencode.json` ready, with a disabled spec showing as disabled.
5. Trigger a hook's event and confirm its command ran. A plugin that fails to parse is reported at startup, so a clean launch means every `.opencode/plugins/<name>.ts` loaded.

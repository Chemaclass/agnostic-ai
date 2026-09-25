+++
title = "OpenHands"
description = "How agnostic-ai emits OpenHands configuration: native paths, capability limits, and output options."
weight = 200

[extra]
group = "Reference"
target_id = "openhands"
+++

# OpenHands (`openhands`)

## Output

```
AGENTS.md                          # pointer body + inlined always-on rules (shared path)
.agents/agents/<name>.md           # one project agent (shared with Goose)
.agents/skills/<name>/SKILL.md     # one folder per skill or path-triggered rule (shared with codex/amp/zed/crush)
config.toml                        # when MCP entries exist
.openhands/hooks.json              # when hook entries exist
.openhands/setup.sh                # when an environment spec sets `install`
```

[OpenHands](https://docs.openhands.dev/overview/skills) reads the root `AGENTS.md` and loads skills from `.agents/skills/`, the same tree codex, amp, zed, and crush emit. The render is byte-identical, so the shared tree is written once.

Local conversations auto-register project agents from `.agents/agents/<name>.md`. The shared Goose/OpenHands renderer writes `name`, `description`, optional free-form `model`, and the prompt body.

- A generic `tools` list is omitted with a coverage note, because OpenHands uses its own tool names (`file_editor`, `terminal`). Set `x-openhands.tools` with native names, and move `outputs.openhands.agents-dir` to the secondary `.openhands/agents` path when the OpenHands profile must differ from Goose's.
- A portable `color` is omitted with a coverage note. [Goose's agent frontmatter](https://github.com/aaif-goose/goose/blob/main/documentation/docs/guides/context-engineering/custom-agents.md) has no `color`, and adding it for OpenHands alone would break the shared file. Set `x-openhands.color` to a [Rich color name](https://rich.readthedocs.io/en/stable/appendix/colors.html) to emit it.

An always-on rule (no `globs`/`paths` and no source-layout or frontmatter scope) inlines into the `## Rules` block of `AGENTS.md`, like every AGENTS.md-family target. A scoped rule emits as a **path-triggered rule**: `.agents/skills/<name>/SKILL.md` with a `paths:` list. OpenHands [loads it only when a matching file is touched](https://docs.openhands.dev/overview/skills/path), so it costs no context until then. This adapter writes the folder form (the vendor also accepts a flat `.md`), so path-triggered rules share `outputs.openhands.skills-dir` with skills. A catch-all `globs`/`paths` value stays inlined.

- **Hooks**: land in `.openhands/hooks.json` (override via `outputs.openhands.hooks-file`), which OpenHands [reads per repository across Cloud, CLI, and local GUI](https://docs.openhands.dev/openhands/usage/customization/hooks).
  - Six events: `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `Stop`, `SessionStart`, `SessionEnd`. OpenHands' native form uses snake_case keys with no wrapper, but it also accepts the Claude form (PascalCase keys inside a `{"hooks": {...}}` wrapper). This adapter emits the Claude form, so one renderer serves both targets.
  - Per entry: `command`, `type` (always `command`), optional `timeout` (seconds, vendor default 60) and `async`. A `matcher` applies only to the two ToolUse events.
  - OpenHands uses its own tool names (`terminal`, not `Bash`), so a matcher from a Claude spec matches nothing. That case gets a coverage note instead of a guessed rename, because only `terminal` (plus `*` and regex) is documented.
- **MCP**: merges into `./config.toml` under a `[mcp]` table with three arrays instead of a `type` field: `stdio_servers` (`[[mcp.stdio_servers]]` tables with `name`/`command`/`args`/`env`), `sse_servers`, and `shttp_servers` (streamable HTTP, the spec's `type: http`).
  - Each remote element is a bare URL string, or a `{ url, api_key, timeout }` object when the entry sets `api_key` and/or (shttp only) `timeout`. Both forms can mix in one array.
  - `timeout` (int, 1-3600 seconds, default 60) is documented for shttp only. An sse entry that sets it gets a coverage note.
  - Generic `headers` have no equivalent (OpenHands documents only `api_key`) and get a coverage note.
  - A transport with no documented array (e.g. `type: ws`) is not written and gets a coverage note.
  - The project `config.toml` is managed: its `[mcp]` table is overwritten on each sync. Keep unmanaged OpenHands config elsewhere.
- **Environments**: an environment spec's `install` writes `.openhands/setup.sh`, the [repository setup script](https://docs.openhands.dev/openhands/usage/customization/repository) OpenHands runs each time it starts working with the repo.
  - The script has a `#!/bin/bash` shebang, the provenance header, then `install` verbatim. OpenHands runs `chmod +x` itself, so no executable bit is set on write.
  - `terminals` (Cursor's long-running dev processes) has no equivalent, since the script runs once at repo start. It gets a coverage note.
  - Multiple environment specs merge like Cursor's: the last spec's `install` wins.

## Import

`agnostic-ai import openhands` reverses the OpenHands layout:

| Source | Becomes |
|--------|---------|
| `AGENTS.md` inlined `## Rules` block (`### <name>` children) | `<rules>/<name>.md` per rule |
| `.agents/skills/<name>/SKILL.md` with `paths` (path-triggered rule) | `<rules>/<name>.md` with `paths` |
| `.agents/skills/<name>/SKILL.md` (+ bundled assets) | `<skills>/<name>/SKILL.md` |
| `.openhands/skills/` and `.openhands/microagents/` (legacy) | the same, read after `.agents/skills/` |
| flat `<name>.md` in any of those three directories | a rule, or a skill when it carries `triggers` (kept under `x-openhands`) |
| `.agents/agents/<name>.md` | `<agents>/<name>.md` |
| `.openhands/hooks.json` | one hook spec per matcher group |
| `config.toml` `[mcp]` table | `<mcps>/<name>.yaml` per server |
| `.openhands/setup.sh` | `<environments>/openhands-setup.yaml` with the script as `install` |
| `AGENTS.md` | `.agnostic-ai/AGNOSTIC_AI.md` |

`.agents/skills/` wins over the legacy trees on a name clash, as in OpenHands. Hooks import from both layouts: snake_case keys (`pre_tool_use`) come back as `PreToolUse`.

Lossy fields (none change what OpenHands loads):

- `sse_servers` and `shttp_servers` entries have no name, so the MCP spec is named after the URL host (`docs-example-test`). The next sync may reorder a bucket's servers.
- A rule's source-layout scope comes back as the `paths` glob it widened to.
- An environment spec's name and `terminals` are lost; the setup script holds only `install`.

## Config keys

| Key | Default |
| --- | --- |
| `outputs.openhands.agents-dir` | `.agents/agents` |
| `outputs.openhands.skills-dir` | `.agents/skills` |
| `outputs.openhands.mcp-file` | `config.toml` |
| `outputs.openhands.hooks-file` | `.openhands/hooks.json` |
| `outputs.openhands.setup-file` | `.openhands/setup.sh` |

## Verify

1. Install OpenHands ([docs](https://docs.openhands.dev/overview/skills)).
2. Check the tree:
   - `ls AGENTS.md .agents/agents/ .agents/skills/ config.toml .openhands/setup.sh`
   - `test -f .agents/agents/*.md`
   - `test -f .agents/skills/*/SKILL.md`
   - `head -1 config.toml` shows the provenance comment.
3. Launch OpenHands:
   - The context loads `AGENTS.md`.
   - Each `.agents/skills/<name>/` appears as a skill.
   - A path-triggered rule injects only when a file matching its `paths:` globs is touched.
   - Each `[mcp]` server in `config.toml` connects.
   - `.openhands/setup.sh` runs at session start.
   - Each `.openhands/hooks.json` entry fires on its event (`OPENHANDS_EVENT_TYPE` in the hook's environment shows which).

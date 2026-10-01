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
.openhands/hooks.json              # when hook entries exist
.openhands/setup.sh                # when an environment spec sets `install`
```

[OpenHands](https://docs.openhands.dev/overview/skills) reads the root `AGENTS.md` and loads skills from `.agents/skills/`, the same tree codex, amp, zed, and crush emit. The render is byte-identical, so the shared tree is written once.

Local conversations auto-register project agents from `.agents/agents/<name>.md`. The shared Goose/OpenHands renderer writes `name`, `description`, optional free-form `model`, and the prompt body.

- A generic `tools` list is omitted with a coverage note, because OpenHands uses its own tool names (`file_editor`, `terminal`). Set `x-openhands.tools` with native names, and move `outputs.openhands.agents-dir` to the secondary `.openhands/agents` path when the OpenHands profile must differ from Goose's.
- A portable `color` is omitted with a coverage note. [Goose's agent frontmatter](https://github.com/aaif-goose/goose/blob/main/documentation/docs/guides/context-engineering/custom-agents.md) has no `color`, and adding it for OpenHands alone would break the shared file. Set `x-openhands.color` to a [Rich color name](https://rich.readthedocs.io/en/stable/appendix/colors.html) to emit it.

An always-on rule (no `globs`/`paths` and no source-layout or frontmatter scope) inlines into the `## Rules` block of `AGENTS.md`, like every AGENTS.md-family target. A scoped rule emits as a **path-triggered rule**: `.agents/skills/<name>/SKILL.md` with a `paths:` list for the union of its scope directory and file patterns. OpenHands [loads it only when a matching file is touched](https://docs.openhands.dev/overview/skills/path), so it costs no context until then. This adapter writes the folder form (the vendor also accepts a flat `.md`), so path-triggered rules share `outputs.openhands.skills-dir` with skills. An unscoped catch-all `globs`/`paths` value stays inlined.

- **Hooks**: land in `.openhands/hooks.json` (override via `outputs.openhands.hooks-file`), which OpenHands [reads per repository across Cloud, CLI, and local GUI](https://docs.openhands.dev/openhands/usage/customization/hooks). [`agnostic-ai hook run`](@/docs/spec-format/hooks.md#hook-run) runs these hooks with OpenHands's payload, shell, and timeout before a session does.
  - Six events: `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `Stop`, `SessionStart`, `SessionEnd`. OpenHands' native form uses snake_case keys with no wrapper, but it also accepts the Claude form (PascalCase keys inside a `{"hooks": {...}}` wrapper). This adapter emits the Claude form, so one renderer serves both targets.
  - Per entry: `command`, `type` (always `command`), optional `timeout` (seconds, vendor default 60) and `async`. A `matcher` applies only to the two ToolUse events.
  - OpenHands uses its own tool names (`terminal`, not `Bash`), so a matcher from a Claude spec matches nothing. That case gets a coverage note instead of a guessed rename, because only `terminal` (plus `*` and regex) is documented.
- **MCP**: project sync writes no MCP file. Current OpenHands releases read MCP servers from Agent Canvas, `~/.openhands/mcp.json`, or the SDK. They ignore a project `config.toml` `[mcp]` section, which the vendor calls legacy V0.
  - Put MCP specs in `~/.agnostic-ai/mcps/` and run `agnostic-ai sync --global` to install them in `~/.openhands/mcp.json`. See [global MCP servers](@/docs/configuration.md#global-mcp-servers).
  - A remote server's `api_key` becomes an `Authorization: Bearer <key>` header, which is what [OpenHands sends](https://docs.openhands.dev/openhands/usage/settings/mcp-settings) for an API key. An `Authorization` entry in `headers` wins. `mcp.json` documents no per-server timeout, so a `timeout` gets a coverage note.
  - A project MCP spec gets one coverage note that points at `sync --global`.
  - The next sync removes a `config.toml` that an earlier release wrote. `import openhands` still reads a legacy `config.toml` `[mcp]` table into specs.
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
| `config.toml` `[mcp]` table (legacy V0) | `<mcps>/<name>.yaml` per server |
| `.openhands/setup.sh` | `<environments>/openhands-setup.yaml` with the script as `install` |
| `AGENTS.md` | `.agnostic-ai/AGNOSTIC_AI.md` |

`.agents/skills/` wins over the legacy trees on a name clash, as in OpenHands. Hooks import from both layouts: snake_case keys (`pre_tool_use`) come back as `PreToolUse`.

Lossy fields (none change what OpenHands loads):

- `sse_servers` and `shttp_servers` entries have no name, so the MCP spec is named after the URL host (`docs-example-test`).
- A rule's source-layout scope comes back as the `paths` glob it widened to.
- An environment spec's name and `terminals` are lost; the setup script holds only `install`.

## Config keys

| Key | Default |
| --- | --- |
| `outputs.openhands.agents-dir` | `.agents/agents` |
| `outputs.openhands.skills-dir` | `.agents/skills` |
| `outputs.openhands.hooks-file` | `.openhands/hooks.json` |
| `outputs.openhands.setup-file` | `.openhands/setup.sh` |

`outputs.openhands.mcp-file` no longer affects OpenHands: project sync writes no MCP file.

## Protected paths

Advisory. This target takes no settings specs, so sync reports a spec with a `protected` block as unsupported. See [Protected paths](@/docs/spec-format/settings.md#protected-paths). State the paths in a rule if the agent should know about them.

## Verify

1. Install OpenHands ([docs](https://docs.openhands.dev/overview/skills)).
2. Check the tree:
   - `ls AGENTS.md .agents/agents/ .agents/skills/ .openhands/setup.sh`
   - `test -f .agents/agents/*.md`
   - `test -f .agents/skills/*/SKILL.md`
3. Launch OpenHands:
   - The context loads `AGENTS.md`.
   - Each `.agents/skills/<name>/` appears as a skill.
   - A path-triggered rule injects only when a file matching its `paths:` globs is touched.
   - After `agnostic-ai sync --global`, each server in `~/.openhands/mcp.json` connects.
   - `.openhands/setup.sh` runs at session start.
   - Each `.openhands/hooks.json` entry fires on its event (`OPENHANDS_EVENT_TYPE` in the hook's environment shows which).

+++
title = "OpenHands"
description = "How agnostic-ai emits OpenHands configuration: native paths, capability limits, and output options."
weight = 200

[extra]
group = "Reference"
target_id = "openhands"
+++

# OpenHands (`openhands`)

[OpenHands](https://docs.openhands.dev/overview/skills) gets `AGENTS.md`, agents and skills under `.agents/`, hooks, and a setup script. MCP servers install globally.

## Output

```
AGENTS.md                          # pointer body + inlined always-on rules (shared path)
.agents/agents/<name>.md           # one project agent (shared with Goose)
.agents/skills/<name>/SKILL.md     # one folder per skill or path-triggered rule (shared tree)
.openhands/hooks.json              # when hook entries exist
.openhands/setup.sh                # when an environment spec sets `install`
```

- **Skills**: render byte-identical to the other `.agents/skills/` targets, so the shared tree is written once.
- **Agents**: local conversations auto-register `.agents/agents/<name>.md`. The renderer shared with Goose writes `name`, `description`, optional free-form `model`, and the prompt body.
  - A generic `tools` list drops with a coverage note, since OpenHands names tools its own way (`file_editor`, `terminal`). Set `x-openhands.tools` with native names. For an OpenHands profile that differs from Goose's, move `outputs.openhands.agents-dir` to the secondary `.openhands/agents` path.
  - A portable `color` drops with a coverage note, since [Goose's agent frontmatter](https://github.com/aaif-goose/goose/blob/main/documentation/docs/guides/context-engineering/custom-agents.md) has none. Set `x-openhands.color` to a [Rich color name](https://rich.readthedocs.io/en/stable/appendix/colors.html) to emit it.
- **Rules**: an always-on rule inlines into the `AGENTS.md` `## Rules` block. So does a rule with a catch-all `globs`/`paths`. Always-on means no `globs`/`paths` and no source-layout or frontmatter scope.
- **Path-triggered rules**: a scoped rule becomes `.agents/skills/<name>/SKILL.md` with a `paths:` list (its scope directory plus file patterns). OpenHands [loads it only when a matching file is touched](https://docs.openhands.dev/overview/skills/path). The adapter writes the folder form (a flat `.md` also works), so these rules share `outputs.openhands.skills-dir` with skills.
- **Hooks**: OpenHands [reads `.openhands/hooks.json` (`outputs.openhands.hooks-file`) per repository in Cloud, CLI, and local GUI](https://docs.openhands.dev/openhands/usage/customization/hooks). [`agnostic-ai hook run`](@/docs/spec-format/hooks.md#hook-run) runs them with OpenHands's payload, shell, and timeout, so you can test them before a session does.
  - Six events: `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `Stop`, `SessionStart`, `SessionEnd`.
  - Sync emits the Claude form (PascalCase keys in a `{"hooks": {...}}` wrapper), so one renderer serves both. OpenHands also accepts its native unwrapped snake_case form.
  - Per entry: `command`, `type` (always `command`), optional `timeout` (seconds, vendor default 60) and `async`. `matcher` applies only to the two ToolUse events.
  - A Claude matcher such as `Bash` matches nothing (OpenHands says `terminal`). Sync notes it rather than guess a rename. Only `terminal`, `*`, and regex are documented.
- **MCP**: no project file. OpenHands reads servers from Agent Canvas, `~/.openhands/mcp.json`, or the SDK, and ignores a project `config.toml` `[mcp]` section (legacy V0).
  - Put MCP specs in `~/.agnostic-ai/mcps/` and run `agnostic-ai sync --global` to install them in `~/.openhands/mcp.json` ([global MCP servers](@/docs/configuration.md#global-mcp-servers)). A project MCP spec gets one note pointing at `sync --global`.
  - A remote `api_key` becomes an `Authorization: Bearer <key>` header, as [OpenHands sends](https://docs.openhands.dev/openhands/usage/settings/mcp-settings) it. An `Authorization` entry in `headers` wins.
  - `timeout` gets a coverage note, since `mcp.json` documents no per-server timeout.
- **Environments**: `install` writes `.openhands/setup.sh`, the [repository setup script](https://docs.openhands.dev/openhands/usage/customization/repository) OpenHands runs each time it opens the repo.
  - It holds a `#!/bin/bash` shebang, the provenance header, then `install` as written. OpenHands runs `chmod +x` itself, so sync sets no executable bit.
  - `terminals` (Cursor's long-running dev processes) gets a coverage note, since the script runs once at repo start.
  - Several environment specs merge as they do for Cursor: the last `install` wins.

{% <details summary="Legacy config.toml"> %}
The next sync removes a `config.toml` that an earlier release wrote. `import openhands` still reads a legacy `config.toml` `[mcp]` table into specs.
{% </details> %}

## Config keys

| Key | Default |
| --- | --- |
| `outputs.openhands.agents-dir` | `.agents/agents` |
| `outputs.openhands.skills-dir` | `.agents/skills` |
| `outputs.openhands.hooks-file` | `.openhands/hooks.json` |
| `outputs.openhands.setup-file` | `.openhands/setup.sh` |

`outputs.openhands.mcp-file` no longer has an effect.

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

`.agents/skills/` wins a name clash with the legacy trees, as in OpenHands. Hooks import from both layouts. Snake_case `pre_tool_use` comes back as `PreToolUse`.

Lossy fields (none change what OpenHands loads):

- Unnamed `sse_servers` and `shttp_servers` entries take the URL host as spec name (`docs-example-test`).
- A rule's source-layout scope comes back as the `paths` glob it widened to.
- An environment spec keeps only `install`; its name and `terminals` are lost.

## Protected paths

Advisory. This target takes no settings specs, so sync reports a spec with a `protected` block as unsupported. See [Protected paths](@/docs/spec-format/settings.md#protected-paths). State the paths in a rule if the agent should know about them.

## Verify

1. Install OpenHands ([docs](https://docs.openhands.dev/overview/skills)).
2. Check the tree: `ls AGENTS.md .agents/agents/ .agents/skills/ .openhands/setup.sh`, `ls .agents/agents/*.md >/dev/null`, `ls .agents/skills/*/SKILL.md >/dev/null`.
3. Launch OpenHands:
   - The context loads `AGENTS.md`.
   - Each `.agents/skills/<name>/` appears as a skill.
   - A path-triggered rule injects only when a file matching its `paths:` globs is touched.
   - After `agnostic-ai sync --global`, each `~/.openhands/mcp.json` server connects.
   - `.openhands/setup.sh` runs at session start.
   - Each `.openhands/hooks.json` entry fires on its event (`OPENHANDS_EVENT_TYPE` in the hook's environment shows which).

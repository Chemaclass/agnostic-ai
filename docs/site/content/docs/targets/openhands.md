+++
title = "OpenHands"
description = "What agnostic-ai writes for OpenHands: file paths, what it supports, and output settings."
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

- **Skills**: written identically to the other `.agents/skills/` tools, so the shared folder is written once.
- **Agents**: OpenHands loads `.agents/agents/<name>.md` in local conversations. The writer is shared with Goose and writes `name`, `description`, an optional free-form `model`, and the prompt body.
  - A generic `tools` list is dropped with a coverage note, because OpenHands uses its own tool names (`file_editor`, `terminal`). Set `x-openhands.tools` with those names instead. For an OpenHands profile that differs from Goose's, point `outputs.openhands.agents-dir` at `.openhands/agents`.
  - A portable `color` is dropped with a coverage note, because [Goose's agent frontmatter](https://github.com/aaif-goose/goose/blob/main/documentation/docs/guides/context-engineering/custom-agents.md) has no color. Set `x-openhands.color` to a [Rich color name](https://rich.readthedocs.io/en/stable/appendix/colors.html) to write it.
- **Rules**: an always-on rule goes into the `## Rules` block of `AGENTS.md`. So does a rule with a catch-all `globs` or `paths`. A rule is always-on when it has no `globs` or `paths` and no scope from its folder or frontmatter.
- **Path-triggered rules**: a scoped rule becomes `.agents/skills/<name>/SKILL.md` with a `paths:` list (its scope directory plus file patterns). OpenHands [loads it only when a matching file is touched](https://docs.openhands.dev/overview/skills/path). It uses the folder form (a flat `.md` also works), so these rules share `outputs.openhands.skills-dir` with skills.
- **Hooks**: OpenHands [reads `.openhands/hooks.json` (`outputs.openhands.hooks-file`) per repository in Cloud, CLI, and local GUI](https://docs.openhands.dev/openhands/usage/customization/hooks). [`agnostic-ai hook run`](@/docs/spec-format/hooks.md#hook-run) runs them with OpenHands's event data, shell, and timeout, so you can test them before a session does.
  - Six events: `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `Stop`, `SessionStart`, `SessionEnd`.
  - Sync writes the Claude form (PascalCase keys inside a `{"hooks": {...}}` wrapper). OpenHands also accepts its own unwrapped snake_case form.
  - Each entry has `command`, `type` (always `command`), and optional `timeout` (seconds, default 60) and `async`. `matcher` applies only to the two ToolUse events.
  - A Claude matcher such as `Bash` matches nothing, because OpenHands calls that tool `terminal`. Sync adds a note and does not rename it. OpenHands documents only `terminal`, `*`, and regex.
- **MCP**: no project file. OpenHands reads servers from Agent Canvas, `~/.openhands/mcp.json`, or the SDK. It ignores the `[mcp]` section of a project `config.toml` (legacy V0).
  - Put MCP specs in `~/.agnostic-ai/mcps/` and run `agnostic-ai sync --global` to install them in `~/.openhands/mcp.json` ([global MCP servers](@/docs/configuration.md#global-mcp-servers)). A project MCP spec gets one note pointing at `sync --global`.
  - A remote `api_key` becomes an `Authorization: Bearer <key>` header, as [OpenHands sends](https://docs.openhands.dev/openhands/usage/settings/mcp-settings) it. An `Authorization` entry in `headers` wins.
  - `timeout` gets a coverage note, because `mcp.json` has no per-server timeout.
- **Environments**: `install` writes `.openhands/setup.sh`, the [repository setup script](https://docs.openhands.dev/openhands/usage/customization/repository) OpenHands runs each time it opens the repo.
  - The file has a `#!/bin/bash` line, the generated-file header, then `install` as written. OpenHands runs `chmod +x` itself, so sync does not set the executable bit.
  - `terminals` (Cursor's long-running dev processes) gets a coverage note, because the script runs once when the repo starts.
  - With several environment specs, the last `install` wins, as for Cursor.

{% <details summary="Legacy config.toml"> %}
The next sync removes a `config.toml` that an earlier release wrote. `import openhands` still reads the `[mcp]` table of a legacy `config.toml` into specs.
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

`agnostic-ai import openhands` reads the OpenHands files back into specs:

| Source | Becomes |
|--------|---------|
| `AGENTS.md` inlined `## Rules` block (`### <name>` children) | `<rules>/<name>.md` per rule |
| `.agents/skills/<name>/SKILL.md` with `paths` (path-triggered rule) | `<rules>/<name>.md` with `paths` |
| `.agents/skills/<name>/SKILL.md` (+ bundled assets) | `<skills>/<name>/SKILL.md` |
| `.openhands/skills/` and `.openhands/microagents/` (legacy) | the same, read after `.agents/skills/` |
| flat `<name>.md` in any of those three directories | a rule, or a skill when it has `triggers` (kept under `x-openhands`) |
| `.agents/agents/<name>.md` | `<agents>/<name>.md` |
| `.openhands/hooks.json` | one hook spec per matcher group |
| `config.toml` `[mcp]` table (legacy V0) | `<mcps>/<name>.yaml` per server |
| `.openhands/setup.sh` | `<environments>/openhands-setup.yaml` with the script as `install` |
| `AGENTS.md` | `.agnostic-ai/AGNOSTIC_AI.md` |

`.agents/skills/` wins a name clash with the legacy folders, as in OpenHands. Hooks import from both layouts. Snake_case `pre_tool_use` comes back as `PreToolUse`.

Import loses these details (none change what OpenHands loads):

- Unnamed `sse_servers` and `shttp_servers` entries are named after the URL host (`docs-example-test`).
- A rule scoped by its folder comes back as the `paths` glob it was widened to.
- An environment spec keeps only `install`. Its name and `terminals` are lost.

## Protected paths

Not enforced. This target takes no settings specs, so sync reports a spec with a `protected` block as unsupported. See [Protected paths](@/docs/spec-format/settings.md#protected-paths). State the paths in a rule if the agent should know about them.

## Verify

1. Install OpenHands ([docs](https://docs.openhands.dev/overview/skills)).
2. Check the files: `ls AGENTS.md .agents/agents/ .agents/skills/ .openhands/setup.sh`, `ls .agents/agents/*.md >/dev/null`, `ls .agents/skills/*/SKILL.md >/dev/null`.
3. Launch OpenHands:
   - It loads `AGENTS.md` as context.
   - Each `.agents/skills/<name>/` appears as a skill.
   - A path-triggered rule injects only when a file matching its `paths:` globs is touched.
   - After `agnostic-ai sync --global`, each `~/.openhands/mcp.json` server connects.
   - `.openhands/setup.sh` runs at session start.
   - Each `.openhands/hooks.json` entry runs on its event (`OPENHANDS_EVENT_TYPE` in the hook's environment shows which).

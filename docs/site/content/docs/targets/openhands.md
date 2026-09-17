+++
title = "OpenHands"
description = "How agnostic-ai emits OpenHands configuration: native paths, capability limits, and output options."
weight = 200

[extra]
group = "Reference"
target_id = "openhands"
+++

# OpenHands (`openhands`)

```
AGENTS.md                          # canonical entry-point pointer body + inlined always-on rules (written by sync, shared path)
.agents/agents/<name>.md           # one project agent (shared with Goose)
.agents/skills/<name>/SKILL.md     # one folder per skill, plus one per path-triggered rule (shared tree with codex/amp/zed/crush)
config.toml                        # when MCP entries exist
.openhands/hooks.json              # when hook entries exist
.openhands/setup.sh                # when an environment spec sets `install`
```

All Hands [OpenHands](https://docs.openhands.dev/overview/skills) reads the root `AGENTS.md` natively and loads skills from `.agents/skills/`, the same cross-tool tree codex, amp, zed, and crush emit. The render is byte-identical, so the shared tree dedupes into one write.

Local conversations also auto-register top-level project agents from `.agents/agents/<name>.md`, the vendor's primary project path. The shared Goose/OpenHands renderer writes `name`, `description`, optional free-form `model`, and the prompt body byte-identically.

A generic `tools` list is omitted with a coverage note because OpenHands uses its own `file_editor` and `terminal` vocabulary. Set `x-openhands.tools` with native names and move `outputs.openhands.agents-dir` to the documented secondary `.openhands/agents` path when an OpenHands profile must differ from Goose's shared file.

An always-on rule (no `globs`/`paths` value and no source-layout or frontmatter scope) inlines into the shared `AGENTS.md` `## Rules` block, same as every other AGENTS.md-family target. A rule that carries one of those instead emits as a **path-triggered rule**: `.agents/skills/<name>/SKILL.md` with a `paths:` frontmatter list, OpenHands' own [deterministic per-file mechanism](https://docs.openhands.dev/overview/skills/path): "guaranteed to load for the files they scope, with no reliance on the model choosing them", and "zero baseline cost" to the context window until a matching file is touched.

The vendor documents two locations for this, a flat `.md` file and a folder; this adapter writes the folder form, so a path-triggered rule shares `outputs.openhands.skills-dir` with regular skills instead of needing a separate rules-dir key. A catch-all `globs`/`paths` value stays inlined because it scopes to every file.

- **Hooks**: land in `.openhands/hooks.json` (override via `outputs.openhands.hooks-file`), the file OpenHands reads "per-repository" and honors "across Cloud, CLI, and local GUI setups" ([docs.openhands.dev/openhands/usage/customization/hooks](https://docs.openhands.dev/openhands/usage/customization/hooks)).
  - Six events: `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `Stop`, `SessionStart`, `SessionEnd`. OpenHands' own layout keys them in snake_case with no wrapper, and documents the Claude form as equally valid: "PascalCase event keys (e.g., `PreToolUse`) and the `{\"hooks\": {...}}` wrapper are both supported, so you can share hook scripts between the two tools". This adapter emits that shared form, so one renderer serves both targets and a hook spec renders comparably across them.
  - Per entry: `command`, `type` (always `command`), optional `timeout` (seconds, vendor default 60) and `async`. A `matcher` only applies to the two ToolUse events.
  - **OpenHands names its own tools**, and the vendor flags the trap itself ("tool names (e.g., `terminal` vs `Bash`)"), so a matcher carried over from a Claude spec parses and then matches nothing. That case surfaces a coverage note rather than a guessed rename, since only `terminal` is documented alongside `*` and regex, leaving no vendor-stated counterpart for the rest.
- **MCP**: merges into `./config.toml` under a `[mcp]` table with three arrays instead of a `type` field: `stdio_servers` (`[[mcp.stdio_servers]]` tables carrying `name`/`command`/`args`/`env`) and `sse_servers` / `shttp_servers` (`shttp_servers` is OpenHands' streamable-HTTP transport, the cross-tool spec's `type: http`).
  - Each remote element is a bare URL string, OpenHands' simplest documented form, or the vendor's `{ url, api_key, timeout }` object once the entry sets a top-level `api_key` and/or (shttp only) `timeout` field. TOML allows mixing both forms in one array, as OpenHands' own example does.
  - `timeout` (int, 1-3600 seconds, default 60, vendor example `timeout = 1800`) is documented for the SHTTP tab only; an sse entry that sets it gets a coverage note instead of a silent no-op.
  - The spec's generic `headers` field has no equivalent here (OpenHands documents only the single `api_key` credential, never a header map) and surfaces a coverage note instead of reaching the target with the credential silently missing.
  - A transport OpenHands documents no array for (e.g. `type: ws`) reaches neither array and surfaces a coverage note instead of guessing one.
  - The project-tier `config.toml` is managed (its `[mcp]` table is overwritten each sync); keep unmanaged OpenHands config elsewhere.
- **Environments**: an environment spec's `install` field writes `.openhands/setup.sh`, the vendor's [documented repository bootstrap script](https://docs.openhands.dev/openhands/usage/customization/repository) ("You can add a `.openhands/setup.sh` file, which will run every time OpenHands begins working with your repository... an ideal location for installing dependencies, setting environment variables, and performing other setup tasks").
  - The script gets a `#!/bin/bash` shebang, then the provenance header, then `install` verbatim as the body. OpenHands chmods the script itself before running it (`chmod +x {script} && source {script}`), so this file needs no executable bit set on write.
  - `terminals` (Cursor's long-running dev processes) has no equivalent here, since the script runs once, synchronously, at repo start; it surfaces a coverage note instead of being silently dropped.
  - Multiple environment specs merge the same way Cursor's do: last spec's `install` wins.

| Key | Default |
| --- | --- |
| `outputs.openhands.agents-dir` | `.agents/agents` |
| `outputs.openhands.skills-dir` | `.agents/skills` |
| `outputs.openhands.mcp-file` | `config.toml` |
| `outputs.openhands.hooks-file` | `.openhands/hooks.json` |
| `outputs.openhands.setup-file` | `.openhands/setup.sh` |

Verify with the real CLI:

1. Install OpenHands ([docs](https://docs.openhands.dev/overview/skills)).
2. Check the tree:
   - `ls AGENTS.md .agents/agents/ .agents/skills/ config.toml .openhands/setup.sh`
   - `test -f .agents/agents/*.md`
   - `test -f .agents/skills/*/SKILL.md`
   - `head -1 config.toml` for the provenance comment
3. Launch OpenHands:
   - The context loads `AGENTS.md`.
   - Each `.agents/skills/<name>/` appears as a skill.
   - A path-triggered rule's `.agents/skills/<name>/SKILL.md` injects only when a file matching its `paths:` globs is touched.
   - Each `[mcp]` server from `config.toml` connects.
   - `.openhands/setup.sh` runs at session start.
   - Each `.openhands/hooks.json` entry fires on its event (check `OPENHANDS_EVENT_TYPE` in the hook's environment to confirm which).

+++
title = "Pi native reference"
description = "Native field mappings, output keys, and import coverage for pi."
weight = 35

[extra]
nav_title = "Pi reference"
group = "Reference"
+++

# Pi native reference

Start with the [Pi setup](@/docs/targets/pi.md).

## Native mappings

### Instructions

- Pi reads instruction files along the path from the filesystem root to its current directory.
- A scoped rule at `services/api/AGENTS.md` reaches a session started there. A root session that reads a file in `services/api/` does not load it.
- In one directory, Pi takes the first readable file of `AGENTS.override.md`, `AGENTS.md`, `AGENTS.MD`, `CLAUDE.md`, `CLAUDE.MD`.
- See [Pi configuration](https://github.com/earendil-works/pi/blob/v1.4.0/packages/coding-agent/docs/configuration.md).

### Skills

- `SKILL.md` metadata and bundled files stay in `.pi/skills/`.
- Pi also discovers shared `.agents/skills/` directories. It keeps the first skill for a duplicate name, so a user-level skill can win over the project copy.

### Commands

- Commands become prompt templates in `.pi/prompts/`, with `description` and `argument-hint` frontmatter.
- Pi expands `$ARGUMENTS`, `$@`, and positional arguments in bodies. Sync keeps that text as written.
- See [prompt templates](https://github.com/earendil-works/pi/blob/v1.4.0/packages/coding-agent/docs/prompt-templates.md).

### MCP

Entries live under `mcpServers` in `.pi/mcp.json` ([Pi MCP](https://github.com/earendil-works/pi/blob/v1.4.0/packages/coding-agent/docs/mcp.md)).

- Stdio: `command`, `args`, `env`, optional `cwd`. HTTP: `url`, optional `headers`.
- Portable `disabled: true` becomes native `enabled: false`.
- Sync accepts a leading UTF-8 byte order mark (BOM) in native MCP files and keeps it when updating.
- Pi reads `timeout` in seconds. The spec holds milliseconds, so sync divides by 1000 exactly (1500 becomes 1.5) and import multiplies by 1000.
- A value under 1000 gets a note, since it most likely holds seconds.
- `x-pi.timeout` is already seconds, must be positive, and wins.
- Environment placeholders stay literal. Native extras belong under `x-pi`.
- `toolExposure` patterns keep source order. Exact tool names take priority over patterns.

### Settings

- Portable `model` becomes `defaultModel`. `effort` becomes `defaultThinkingLevel`.
- Pi accepts `off`, `minimal`, `low`, `medium`, `high`, `xhigh`, `max`.
- Choose a provider with native `x-pi.defaultProvider`. Sync does not split model IDs.
- Unrelated settings stay in place.

## Global configuration

`agnostic-ai sync --global -t pi` writes instructions, skills, MCP, and settings under `~/.pi/agent/`. See [global configuration](@/docs/global-configuration.md).

- `PI_CODING_AGENT_DIR` replaces the whole agent directory.
- Global HTTP MCP entries also keep `x-pi.auth: {provider: radius}`. Pi limits provider auth to global `mcp.json` and extensions ([release reference](https://github.com/earendil-works/pi/releases/tag/v0.99.2)), so project output omits it.
- Existing `x-pi.oauth` stays separate.
- Global import scopes `auth` under `x-pi`, so equal native entries can be adopted without rewriting user content.

## Config keys

For the shared root instruction file, see [entry-point files](@/docs/configuration-reference.md#entry-point-files).

| Key | Default |
|---|---|
| `outputs.pi.rules-file` | unset, uses instruction files |
| `outputs.pi.skills-dir` | `.pi/skills` |
| `outputs.pi.commands-dir` | `.pi/prompts` |
| `outputs.pi.mcp-file` | `.pi/mcp.json` |
| `outputs.pi.conf-file` | `.pi/settings.json` |

Changing a path does not change where Pi looks. Keep overrides aligned with your native configuration.

## Import

```bash
agnostic-ai import pi
agnostic-ai lint
agnostic-ai sync -t pi
agnostic-ai sync --check -t pi
```

Import reads root and scoped instructions, skill folders and resources, direct prompt templates, `.pi/mcp.json`, and `.pi/settings.json`.

- Native MCP `enabled` is inverted back to portable `disabled`.
- Project control-only overrides import with `targets: [pi]`.
- Model and thinking level become portable settings. `defaultProvider` stays under `x-pi`.
- `agnostic-ai import --global pi` reads supported user-level files.

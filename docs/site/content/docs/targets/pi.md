+++
title = "Pi"
description = "Native Pi instructions, skills, prompt commands, MCP servers, and model settings."
weight = 34

[extra]
group = "Reference"
target_id = "pi"
+++

# Pi (`pi`)

agnostic-ai writes Pi's native project files and imports existing ones. This target follows [Pi v1.4.0](https://github.com/earendil-works/pi/releases/tag/v1.4.0), published as `@earendil-works/pi-coding-agent`.

## First rule

Select the target in `agnostic-ai.yaml`:

```yaml
targets: [pi]
```

Save `.agnostic-ai/rules/commit-prefix.md`:

```markdown
---
name: commit-prefix
---

Start commit subjects with fix:, feat:, docs:, or chore:.
```

Run:

```bash
agnostic-ai sync -t pi
agnostic-ai sync --check -t pi
```

Inspect `AGENTS.md` for the rule. This checks generated files. To check discovery, start Pi from the project root and ask for a commit subject. For an existing setup, [import it](#import) before syncing.

## Output

```text
AGENTS.md                      # root instructions and rules
<scope>/AGENTS.md              # directory-scoped rules
.pi/skills/<name>/SKILL.md     # skills, with bundled files
.pi/prompts/<name>.md          # slash commands as prompt templates
.pi/mcp.json                   # MCP servers
.pi/settings.json              # model and thinking settings
```

## Native mappings

Pi reads instruction files along the path from the filesystem root to its current directory. A scoped rule at `services/api/AGENTS.md` reaches a session started there. Reading a file in `services/api/` from a root session does not load that descendant instruction file. Within one directory, Pi selects the first readable file from `AGENTS.override.md`, `AGENTS.md`, `AGENTS.MD`, `CLAUDE.md`, and `CLAUDE.MD`. See [Pi configuration](https://github.com/earendil-works/pi/blob/v1.4.0/packages/coding-agent/docs/configuration.md).

Skills keep their `SKILL.md` metadata and bundled files in `.pi/skills/`. Pi also discovers shared `.agents/skills/` directories. It keeps the first skill for a duplicate name, so a user-level skill can win over the project copy.

Commands become prompt templates in `.pi/prompts/`, with `description` and `argument-hint` frontmatter. Pi expands `$ARGUMENTS`, `$@`, and positional arguments in their bodies. Sync keeps that text as written. See [prompt templates](https://github.com/earendil-works/pi/blob/v1.4.0/packages/coding-agent/docs/prompt-templates.md).

MCP entries live under `mcpServers` in `.pi/mcp.json`. Stdio uses `command`, `args`, `env`, and optional `cwd`; HTTP uses `url` and optional `headers`. Portable `disabled: true` becomes native `enabled: false`. Timeouts are seconds. Environment placeholders stay literal. Target-native extras belong under `x-pi`. See [Pi MCP](https://github.com/earendil-works/pi/blob/v1.4.0/packages/coding-agent/docs/mcp.md).

Settings map portable `model` to `defaultModel` and `effort` to `defaultThinkingLevel`. Pi accepts `off`, `minimal`, `low`, `medium`, `high`, `xhigh`, and `max`. Choose a provider separately with native `x-pi.defaultProvider`; sync does not split model IDs. Unrelated settings stay in place.

Save `.agnostic-ai/mcps/docs-search.yaml` to define an HTTP server without enabling it:

```yaml
name: docs-search
type: http
url: https://mcp.example.com/mcp
disabled: true
```

After sync, `.pi/mcp.json` contains the server with `enabled: false`. Replace the example URL with your server's endpoint before enabling it.

## Limits and trust

- Project settings, MCP, skills, and prompt templates need Pi's project trust approval. Instruction files load without that approval. Trust remains Pi's decision; sync does not write trust approvals or `defaultProjectTrust`.
- Pi has no native agent definitions, lifecycle hooks, or per-tool permission rules. Agent, hook, ignore, review, and environment specs have no native output; portable permission settings are unsupported. Skill `allowed-tools` is metadata, not an enforced permission limit.
- MCP supports stdio and HTTP; SSE is unsupported. Invalid global MCP specifications stop sync before native files change.
- The default prompt directory loads direct Markdown children. Nested command names are unsupported.
- Commands sync only at project scope. Global command sync is outside this target's support.

## Global configuration

`agnostic-ai sync --global -t pi` writes instructions, skills, MCP, and settings under `~/.pi/agent/`. `PI_CODING_AGENT_DIR` replaces that whole agent directory. See [global configuration](@/docs/global-configuration.md).

## Config keys

| Key | Default |
|---|---|
| `outputs.pi.file` | `AGENTS.md` |
| `outputs.pi.rules-file` | unset, uses instruction files |
| `outputs.pi.skills-dir` | `.pi/skills` |
| `outputs.pi.commands-dir` | `.pi/prompts` |
| `outputs.pi.mcp-file` | `.pi/mcp.json` |
| `outputs.pi.conf-file` | `.pi/settings.json` |

Changing a path does not change where Pi looks for it. Keep overrides aligned with your native configuration.

## Import

```bash
agnostic-ai import pi
agnostic-ai lint
agnostic-ai sync -t pi
agnostic-ai sync --check -t pi
```

Import reads root and scoped instructions, skill folders and resources, direct prompt templates, `.pi/mcp.json`, and `.pi/settings.json`. Native MCP `enabled` is inverted back to portable `disabled`. Project control-only overrides import with `targets: [pi]`. Model and thinking level become portable settings; `defaultProvider` stays under `x-pi`. Use `agnostic-ai import --global pi` for supported user-level files.

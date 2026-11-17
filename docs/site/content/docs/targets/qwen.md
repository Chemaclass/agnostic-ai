+++
title = "Qwen Code"
description = "Native Qwen Code instructions, agents, skills, commands, hooks, MCP, settings, and ignore files."
weight = 32

[extra]
group = "Reference"
target_id = "qwen"

[extra.moved]
native-mappings = "@/docs/targets/qwen-reference.md"
global-configuration = "@/docs/targets/qwen-reference.md#global-configuration"
config-keys = "@/docs/targets/qwen-reference.md#config-keys"
import = "@/docs/targets/qwen-reference.md#import"
limits-and-trust = "@/docs/targets/qwen.md#loading-and-limits"
+++

# Qwen Code (`qwen`)

## Native mappings {#native-mappings .legacy-anchor}

agnostic-ai writes Qwen Code's native project files and imports existing ones. It follows [Qwen Code v0.27.0](https://github.com/QwenLM/qwen-code/releases/tag/v0.27.0).

## First rule

1. Add `qwen` to `targets` in `agnostic-ai.yaml`.
2. Save `.agnostic-ai/rules/commit-prefix.md`:

```markdown
---
name: commit-prefix
---

Start commit subjects with fix:, feat:, docs:, or chore:.
```

3. Run `agnostic-ai sync -t qwen`.

Have an existing setup? [Import it](@/docs/targets/qwen-reference.md#import) before syncing.

## Output

| Source | Default output |
|---|---|
| Root instructions and rules | `QWEN.md` |
| Directory-scoped rules | `<scope>/QWEN.md` |
| Rules with file patterns | `.qwen/rules/<name>.md` |
| Agents (native subagents) | `.qwen/agents/<name>.md` |
| Skills, with bundled files | `.qwen/skills/<name>/SKILL.md` |
| Slash commands | `.qwen/commands/<name>.md` |
| MCP, hooks, and settings | `.qwen/settings.json` |
| Ignore patterns | `.qwenignore` |

Field mappings, global configuration, and config keys: [native reference](@/docs/targets/qwen-reference.md).

## Verify

1. Run `agnostic-ai sync --check -t qwen`.
2. Find the rule in `QWEN.md`.
3. Start Qwen Code from the project root and ask for a commit subject.

## Loading and limits

- `.qwen/rules/` needs a trusted folder. Project hooks need a trusted folder or Qwen's hook approval.
- Qwen owns trust and approval. Safe mode and bare mode can disable project customizations.
- `.qwenignore` applies only to tools that honor it. It does not stop every form of file access.
- Commands and ignore files sync at project scope only.

{% <details summary="More limits"> %}
- From Qwen Code 0.26.7, interactive sessions skip hooks from `.qwen/settings.json`, agent and skill hooks, and skill permission grants until you set `security.folderTrust.enabled` to `true` and trust the folder. Folder trust is off by default.
- From 0.26.8, runs with `-p`, piped input, or ACP and SDK clients follow the same rule for `.qwen/settings.json` hooks. With folder trust off, those hooks also run once you approve that exact hooks block.
- Global hook specs emit command handlers. Reviews and environment specs have no native output.
- Portable agent effort has no equivalent.
- Qwen's approval modes differ from Claude's `permissionMode` names, so sync leaves `permissionMode` out with a note. Set `x-qwen.approvalMode` instead.
- Agent `background` and `hooks` are written as given. A `Stop` hook becomes `SubagentStop`, since Qwen does not treat one as the other in an agent file. An event outside Qwen's [hook events](https://qwenlm.github.io/qwen-code-docs/en/users/features/hooks/) raises a note.
- Import keeps native agent hooks under `x-qwen`, because their matchers use Qwen tool names.
- Portable lists of agent MCP server names cannot replace Qwen's record of server definitions.
{% </details> %}

{% <details summary="Recipe: add an MCP server"> %}
Save `.agnostic-ai/mcps/docs-search.yaml`:

```yaml
name: docs-search
type: http
url: https://mcp.example.com/mcp
```

Replace the example URL with your endpoint. After sync, `.qwen/settings.json` has `mcpServers.docs-search.httpUrl` with that URL.
{% </details> %}

{% <details summary="Recipe: block an inherited MCP server"> %}
To exclude a server inherited from user or extension configuration, put this in a settings spec:

```yaml
x-qwen:
  mcp:
    excluded: [server-name]
```

Qwen combines this exclusion list across trusted settings scopes. Project and global sync merge entries individually, so removing a source exclusion keeps user entries and other MCP settings.
{% </details> %}

## Protected paths

Not enforced. This target cannot write a rule or hook that blocks edits. Sync prints a [protected-path coverage note](@/docs/spec-format/settings.md#protected-paths).

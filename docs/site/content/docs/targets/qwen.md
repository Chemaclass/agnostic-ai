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

agnostic-ai writes Qwen Code's native project files and imports existing ones. This target follows [Qwen Code v0.27.0](https://github.com/QwenLM/qwen-code/releases/tag/v0.27.0).

## First rule

Select the target in `agnostic-ai.yaml`:

```yaml
targets: [qwen]
```

Save `.agnostic-ai/rules/commit-prefix.md`:

```markdown
---
name: commit-prefix
---

Start commit subjects with fix:, feat:, docs:, or chore:.
```

Then run `agnostic-ai sync -t qwen`. For an existing setup, [import it](@/docs/targets/qwen-reference.md#import) before syncing.

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

For field mappings, global configuration, and config keys, see the [native reference](@/docs/targets/qwen-reference.md).

## Verify

Run `agnostic-ai sync --check -t qwen` and inspect `QWEN.md` for the rule. Start Qwen Code from the project root and ask for a commit subject to check that it loaded.

## Loading and limits

- `.qwen/rules/` requires a trusted folder. Project hooks need a trusted folder or Qwen's hook approval. Safe mode and bare mode can disable project customizations. Trust and approval remain Qwen decisions.
- From Qwen Code 0.26.7, interactive sessions skip hooks from `.qwen/settings.json`, agent and skill hooks, and skill permission grants until you set `security.folderTrust.enabled` to `true` and trust the folder. Folder trust is off by default. From 0.26.8, runs with `-p`, piped input, or ACP and SDK clients follow the same rule for `.qwen/settings.json` hooks instead of trusting the folder on their own. With folder trust off, those hooks also run once you approve that exact hooks block.
- `.qwenignore` applies to tools that honor it; it does not prevent every form of file access.
- Commands and ignore files sync only at project scope. Global hook specs emit command handlers. Reviews and environment specs have no native output.

{% <details summary="More limits"> %}
- Portable agent effort has no equivalent. Qwen maps Claude's agent `permissionMode` names to its own approval modes, which approve different actions, so sync leaves `permissionMode` out with a note; set `x-qwen.approvalMode` instead. Agent `background` and `hooks` are written as given, with two limits: a `Stop` hook becomes `SubagentStop`, since Qwen does not treat one as the other in an agent file, and an event outside Qwen's [hook events](https://qwenlm.github.io/qwen-code-docs/en/users/features/hooks/) raises a note. Import keeps native agent hooks under `x-qwen`, because their matchers use Qwen tool names. Portable lists of agent MCP server names cannot replace Qwen's record of server definitions.
{% </details> %}

## Protected paths

Not enforced. This target cannot write a rule or hook that blocks edits. Sync prints a [protected-path coverage note](@/docs/spec-format/settings.md#protected-paths).

## Add an MCP server

Save `.agnostic-ai/mcps/docs-search.yaml` to use your own HTTP endpoint:

```yaml
name: docs-search
type: http
url: https://mcp.example.com/mcp
```

Replace the example URL with your server's endpoint. After sync, `.qwen/settings.json` contains `mcpServers.docs-search.httpUrl` with that URL.

## Block an inherited MCP server

To exclude a server inherited from user or extension configuration, put this in a settings spec:

```yaml
x-qwen:
  mcp:
    excluded: [server-name]
```

Qwen combines this native exclusion list across trusted settings scopes. Project and global sync merge entries individually, so removing a source exclusion keeps user entries and other MCP settings.

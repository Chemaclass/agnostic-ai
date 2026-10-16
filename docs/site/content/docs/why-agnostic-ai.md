+++
title = "Start here: understand the setup"
description = "What coding agents read, which instructions to add first, and how agnostic-ai shares them across tools."
weight = 5
aliases = ["/docs/alternatives-why-not-symlinks/"]

[extra]
group = "Start"
next = "@/docs/getting-started.md"
next_label = "Build your first shared setup"
+++

# Understand the setup {#why-agnostic-ai}

Claude Code and Codex are coding agents: tools you ask to inspect a project, change files, and run commands. They work better when they know where the code lives, how to check changes, and which project conventions to follow.

Start small. Give your tool a few project notes and one useful convention. Add a longer procedure or an outside connection when a task needs it.

## A small useful setup

A project's starting instructions can say:

```markdown
Application code is in src/. Tests are in tests/.
Before changing code, find the existing test command in CONTRIBUTING.md.
Run the relevant tests before finishing a change.
```

A rule can add a standing convention, such as the prefixes your team uses in commit messages. These are instructions to the model. They do not enforce themselves like a test or a permission setting.

## What to add, and when

| You need | Use | Example |
|---|---|---|
| Basic orientation every session | **Project instructions** | Where code lives and how to test it |
| A standing convention | A [rule](@/docs/spec-format/rules.md) | Use the project's commit format |
| A repeatable task with several steps | A [skill](@/docs/spec-format/skills.md) | Prepare a release using a checklist |
| A separate specialist role | An [agent](@/docs/spec-format/agents.md) | Review changes for defects |
| A command at a tool event | A [hook](@/docs/spec-format/hooks.md) | Run a check when a supported event occurs |
| Operating choices for the tool | [Settings](@/docs/spec-format/settings.md) | Set a default model or permissions |
| Access to an outside tool or data | An [MCP connection](@/docs/spec-format/mcps.md) | Let the agent read an issue tracker |

A skill supplies a procedure; an agent supplies a specialist role. A hook runs a command at an event; a rule asks the model to follow an instruction. MCP means Model Context Protocol, a way to connect an agent to other tools and data. Each coding tool supports a different subset of these features.

## What agnostic-ai does

Claude Code reads `CLAUDE.md` and `.claude/` files. Codex reads `AGENTS.md` and `.codex/` files. Other tools use other paths, formats, and setting names. Maintaining these separately makes the same project convention easy to change in one place and forget in another.

agnostic-ai keeps editable Markdown and YAML in `.agnostic-ai/`. Each source file is called a **spec**. You select your coding tools, called **targets**, in `agnostic-ai.yaml`.

```text
.agnostic-ai/ source files
          |
   agnostic-ai sync
          |
   +------+------+
   |             |
CLAUDE.md     AGENTS.md
.claude/      .codex/
   |             |
Claude Code    Codex
```

Edit the sources, then sync. The generated files are ordinary files the installed coding tools read. agnostic-ai does not run the agents or give them an account. When a tool cannot use a spec or field, sync reports the gap. See [tool support](@/docs/targets/_index.md).

`sync --check` checks that generated files match the sources. It does not prove a model followed an instruction or that an external connection works.

## When you do not need it

With one tool, its own files can be enough. agnostic-ai helps when you want to share a setup across tools or keep several formats current. The generated files keep working if you stop using agnostic-ai.

<details>
<summary>Why copies, symlinks, and one AGENTS.md have limits</summary>

## Why the usual fixes fall short

Copying instructions creates several places to maintain. A symlink shares bytes but cannot translate formats or keys. One `AGENTS.md` works well for tools that read plain project instructions there; it cannot configure their native skills, hooks, or MCP connections. A custom script can translate the formats you maintain yourself.

## The same server, four formats

One MCP connection in `.agnostic-ai/mcps/github.yaml`:

```yaml
name: github
type: http
url: https://api.githubcopilot.com/mcp/
```

Sync translates it into each selected tool's own configuration:

| Tool | File | Shape |
|---|---|---|
| Claude Code | `.mcp.json` | `mcpServers.github.url` |
| Codex | `.codex/config.toml` | `[mcp_servers.github]` with `url` |
| Gemini CLI | `.gemini/settings.json` | `mcpServers.github.httpUrl` |
| Zed | `.zed/settings.json` | `context_servers.github.url` |

This writes the connection settings. You still authenticate with the service as required.

</details>

## Start

Follow [Getting started](@/docs/getting-started.md) to share one useful setup between Claude Code and Codex. Already have tool files? [Import your existing setup](@/docs/migration.md).

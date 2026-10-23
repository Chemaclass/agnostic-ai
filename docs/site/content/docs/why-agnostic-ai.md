+++
title = "Why agnostic-ai?"
description = "Keep one project setup and generate the files each coding tool reads."
weight = 5
aliases = ["/docs/alternatives-why-not-symlinks/"]

[extra]
nav_title = "Why agnostic-ai?"
group = "Start"
next = "@/docs/getting-started.md"
next_label = "Set up your project"
+++

# Why agnostic-ai? {#why-agnostic-ai}

Using several coding tools means keeping several sets of instructions and settings current. agnostic-ai lets you edit one source and generate the files each tool reads.

## A small useful setup

Start with a short project note in `.agnostic-ai/AGNOSTIC_AI.md`:

```markdown
Application code is in src/. Tests are in tests/.
Read CONTRIBUTING.md for the test command.
Run the relevant tests before finishing a change.
```

Run `agnostic-ai sync`. Claude Code gets `CLAUDE.md`, Codex gets `AGENTS.md`, and other tools get their own files. Change the source note and sync again to update them together.

## What to add, and when

Add a source file when you need to share more than the project note:

| You need | Use | Example |
|---|---|---|
| Project instructions | `.agnostic-ai/AGNOSTIC_AI.md` | Where code lives and how to test it |
| A coding convention | A [rule](@/docs/spec-format/rules.md) | Use the project's commit format |
| Steps for a repeatable task | A [skill](@/docs/spec-format/skills.md) | Prepare a release |
| A specialist to delegate to | An [agent](@/docs/spec-format/agents.md) | Review a change for defects |
| A command at a tool event | A [hook](@/docs/spec-format/hooks.md) | Format a file after an edit |
| Tool settings | [Settings](@/docs/spec-format/settings.md) | Choose a model or permissions |
| An outside service or data source | An [MCP connection](@/docs/spec-format/mcps.md) | Read an issue tracker |

MCP connects coding tools to other tools and data. Each tool supports different features; check [tool support](@/docs/targets/_index.md) for the ones you use.

## What agnostic-ai does

Your editable Markdown and YAML live in `.agnostic-ai/`. These source files are called **specs**. Choose your coding tools, called **targets**, in `agnostic-ai.yaml`.

Sync translates the sources into each tool's paths, formats, and setting names. It reports features a tool cannot use. You still install and run the coding tools yourself.

`agnostic-ai sync --check` checks whether the generated files match the sources. Instructions guide the model; tests and tool permissions enforce checks and limits. Connecting to a service still requires its authentication.

## When you do not need it

For one tool and a small setup, start with that tool's own files. Use agnostic-ai when you need to share a setup across tools or keep several formats current. The generated files keep working if you stop using agnostic-ai.

<details>
<summary>Why copies, symlinks, and one AGENTS.md have limits</summary>

## Why the usual fixes fall short

Copies need separate updates. A symlink shares text but cannot translate formats or setting names. One `AGENTS.md` shares instructions with tools that read it, but cannot configure their native skills, hooks, or service connections. A custom script can translate the formats you maintain yourself.

## The same server, four formats

One connection in `.agnostic-ai/mcps/github.yaml`:

```yaml
name: github
type: http
url: https://api.githubcopilot.com/mcp/
```

Sync writes the same connection in each tool's format:

| Tool | File | Connection setting |
|---|---|---|
| Claude Code | `.mcp.json` | `mcpServers.github.url` |
| Codex | `.codex/config.toml` | `[mcp_servers.github]` with `url` |
| Gemini CLI | `.gemini/settings.json` | `mcpServers.github.httpUrl` |
| Zed | `.zed/settings.json` | `context_servers.github.url` |

</details>

## Start

[Set up your project](@/docs/getting-started.md), or [import the files you already use](@/docs/cli-reference/start.md#import).

+++
title = "Why agnostic-ai?"
description = "Keep one project setup and generate the files each coding tool reads."
weight = 5
aliases = ["/docs/alternatives-why-not-symlinks/"]

[extra]
nav_title = "Why agnostic-ai?"
group = "Start"
next = "@/docs/installation.md"
next_label = "Install agnostic-ai"
+++

# Why agnostic-ai? {#why-agnostic-ai}

Each coding tool reads its instructions and settings from its own files. With several tools, you keep several copies current. agnostic-ai lets you edit one source and generates the files each tool reads.

## A small useful setup

Start with a short project note in `.agnostic-ai/AGNOSTIC_AI.md`:

```markdown
Application code is in src/. Tests are in tests/.
Read CONTRIBUTING.md for the test command.
Run the relevant tests before finishing a change.
```

Run `agnostic-ai sync`. Claude Code gets `CLAUDE.md`, Codex gets `AGENTS.md`, and other tools get their own files. Change the note and sync again to update them all.

## What agnostic-ai does

Your source files live in `.agnostic-ai/`, in plain Markdown and YAML. They are called **specs**. The coding tools you use are called **targets**; you choose them in `agnostic-ai.yaml`.

Sync translates the specs into each tool's paths, formats, and setting names, and reports features a tool cannot use. `agnostic-ai sync --check` tells you whether the generated files still match the specs.

You still install and sign in to the coding tools yourself. Instructions guide the model; tests and tool permissions enforce checks and limits. A connection to an outside service still needs that service's login or key.

## What to add, and when

Start with the project note. Add another kind of spec only when you need it:

| You need | Use | Example |
|---|---|---|
| Project instructions | `.agnostic-ai/AGNOSTIC_AI.md` | Where code lives and how to test it |
| A coding convention | A [rule](@/docs/spec-format/rules.md) | Use the project's commit format |
| Steps for a repeatable task | A [skill](@/docs/spec-format/skills.md) | Prepare a release |
| A specialist to delegate to | An [agent](@/docs/spec-format/agents.md) | Review a change for defects |
| Limits on what an agent may read, run, or call | Agent [capabilities](@/docs/spec-format/agents.md#capabilities) | Let a reviewer read but not edit |
| A command at a tool event | A [hook](@/docs/spec-format/hooks.md) | Format a file after an edit |
| Tool settings | [Settings](@/docs/spec-format/settings.md) | Choose a model or permissions |
| One model choice for every tool | [Model roles](@/docs/configuration.md#models) | Use a fast model for lookups |
| An outside service or data source | An [MCP connection](@/docs/spec-format/mcps.md) | Read an issue tracker |
| Guidance for code review bots | [Reviews](@/docs/spec-format/reviews.md) | Flag missing tests |
| Facts every tool can recall | [Shared memory](@/docs/memory.md) | A decision and its reason |
| Work you pick up in another tool | [Session handoffs](@/docs/handoff.md) | One private note per branch |
| Shorter command output and replies | [RTK and Caveman](@/docs/rtk-and-caveman.md) | Fewer tokens per session |

MCP (Model Context Protocol) is how coding tools connect to other tools and data. An MCP spec names the environment variables that hold credentials, so keys stay out of the repository. Each tool supports a different set of features; the [capability matrix](@/docs/targets/_index.md#capability-matrix) shows what each one gets.

## When you do not need it

For one tool and a small setup, start with that tool's own files. Use agnostic-ai when you share a setup across tools or keep several formats current. If you stop using agnostic-ai, the generated files keep working; commit them first if Git ignores them.

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

[Install agnostic-ai](@/docs/installation.md), then [set up your project](@/docs/getting-started.md). Already have tool files? [Import them first](@/docs/migration.md).

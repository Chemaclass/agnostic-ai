+++
title = "Why agnostic-ai"
description = "Every AI coding tool wants its own config files. Why symlinks, copies, one AGENTS.md, or a script fall short, and what agnostic-ai does instead."
weight = 170
aliases = ["/docs/alternatives-why-not-symlinks/"]

[extra]
group = "Reference"
+++

# Why agnostic-ai

Every AI coding tool wants its own config: `CLAUDE.md`, `AGENTS.md`, `.cursor/rules/*.mdc`, skills, hooks, MCP servers. Each has its own path, format, and keys. Use two tools and you keep two copies, which grow different over time.

## The same server, four formats

One MCP server, written once in `.agnostic-ai/mcps/github.yaml`:

```yaml
name: github
type: http
url: https://api.githubcopilot.com/mcp/
```

`agnostic-ai sync` writes it where each tool reads it:

| Tool | File | Shape |
|---|---|---|
| Claude Code | `.mcp.json` | `mcpServers.github.url` |
| Codex | `.codex/config.toml` | `[mcp_servers.github]` with `url` |
| Gemini CLI | `.gemini/settings.json` | `mcpServers.github.httpUrl` |
| Zed | `.zed/settings.json` | `context_servers.github.url` |

Hooks differ the same way. The event before a tool call is `PreToolUse` in Claude Code and `BeforeTool` in Gemini CLI.

## Why the usual fixes fall short

- **A symlink** shares bytes. It cannot turn YAML into TOML or rename a key. On Windows it needs admin rights or Developer Mode, and Git for Windows checks it out as a text file by default.
- **Copy and paste** works until someone edits one copy.
- **One `AGENTS.md`** is read by many tools and carries plain instructions well. It cannot carry skills, hooks, MCP servers, agents, rules scoped to some files, or tool-specific YAML settings such as a model name.
- **Your own script** works for the formats you know today. Tools change their formats often, and you fix the script each time.

## What agnostic-ai does

- **One source.** Plain Markdown and YAML in `.agnostic-ai/`, committed with your code.
- **Each tool's own format.** `sync` writes each tool's own files for [25 targets](@/docs/targets/_index.md), `AGENTS.md` included.
- **An automated check.** [`sync --check`](@/docs/ci.md) fails the build when a generated file differs from its spec.
- **Easy to adopt.** [`init --from all`](@/docs/migration.md) imports the config you already have and keeps its behavior.
- **Safe on shared files.** Sync keeps your hand-written keys in files such as `.claude/settings.json`, and [`revert`](@/docs/cli-reference/maintain.md#revert) undoes a `sync --backup`.
- **Easy to leave.** The generated files are yours and keep working without agnostic-ai.

## When you do not need it

With one tool, use its own files. With two tools that read the same Markdown at the same path, a symlink is enough. agnostic-ai helps once a second tool wants a different format.

## Start

[Install agnostic-ai](@/docs/installation.md), then at your project root:

```bash
agnostic-ai init --from all
agnostic-ai sync --plan
agnostic-ai sync
```

New project with no config yet? Follow [Getting started](@/docs/getting-started.md).

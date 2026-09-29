+++
title = "Why not symlinks or manual copies?"
description = "A symlink gives every tool the same file, but each AI tool needs a different one. What agnostic-ai does instead, and when a symlink is enough."
weight = 170

[extra]
group = "Reference"
+++

# Why agnostic-ai instead of symlinks

A symlink gives every tool the same file. But each AI coding tool expects a different file: its own path, its own format, its own keys.

agnostic-ai keeps one source and writes each tool's native files from it. The source is plain Markdown and YAML, aligned with the [`AGENTS.md`](https://agents.md/) open standard.

## One source, one file per tool

Write an MCP server once, in `.agnostic-ai/mcps/github.yaml`:

```yaml
name: github
type: http
url: https://api.githubcopilot.com/mcp/
```

`agnostic-ai sync` writes it for Claude Code in `.mcp.json`:

```json
{
  "mcpServers": {
    "github": {
      "type": "http",
      "url": "https://api.githubcopilot.com/mcp/"
    }
  }
}
```

And for Codex in `.codex/config.toml`:

```toml
[mcp_servers.github]
url = "https://api.githubcopilot.com/mcp/"
```

Gemini CLI wants `httpUrl` instead of `url` in `.gemini/settings.json`. Zed wants `context_servers` instead of `mcpServers` in `.zed/settings.json`. No single file can serve all four.

Rules, skills, agents, and hooks differ the same way. The hook that runs before a tool call is `PreToolUse` in Claude Code and `BeforeTool` in Gemini CLI. Each [target page](@/docs/targets/_index.md) shows its exact output.

## What you get over a symlink or copy

| | Symlink | Copy | agnostic-ai |
|---|---|---|---|
| Two tools read the same file | Yes | Yes | Yes |
| Each tool gets its own format, path, and keys | No | No | Yes |
| Stays current without a manual step | Yes | No | Yes, with `sync --watch` |
| Works on Windows out of the box | No | Yes | Yes |
| [Fails CI](@/docs/ci.md) when a generated file drifts | No | No | Yes |
| [Imports](@/docs/migration.md) your existing tool config | No | No | Yes |
| Keeps your hand-written keys in shared files, with backup and [revert](@/docs/cli-reference/maintain.md#revert) | No | No | Yes |

On Windows, creating a symlink needs admin rights or Developer Mode. Git for Windows defaults to `core.symlinks=false`, which checks symlinks out as plain text files.

## When a symlink is enough

You do not need agnostic-ai for:

- **One tool.** Use its own files.
- **Two tools that read the same Markdown at the same path.** A symlink or copy works.
- **Shared text in Claude Code.** `@/RULES_SHARED.md` in `.claude/rules/base.md` pulls a shared file in. Most other tools do not resolve `@` imports.

These stop working once a tool wants a different format, such as Codex TOML agents or Cursor `.mdc` rules.

agnostic-ai still uses symlinks where they are safe. With [`sync.shared-skills: true`](@/docs/configuration.md#syncshared-skills), byte-identical skill folders share one copy through relative symlinks. A folder that differs for one tool gets a real copy, and so does a system without symlink support.

## Next steps

- New here: start with [Getting started](@/docs/getting-started.md).
- To trace a generated file back to its spec: see [`agnostic-ai why <file>`](@/docs/trace.md).

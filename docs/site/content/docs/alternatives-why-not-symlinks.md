+++
title = "Why not symlinks or manual copies?"
description = "See where native formats diverge and when agnostic-ai is worth using."
weight = 170

[extra]
group = "Reference"
+++

# Why agnostic-ai instead of symlinks or manual copies

You write `CLAUDE.md`. Then `.cursor/rules`. Then `GEMINI.md`. Then `AGENTS.md`. Same content, four formats. The instinct is to symlink one file into each path, or copy it on every change.

That solves the wrong problem. Each tool reads a different format at a different path, so one spec has to become different bytes for each tool. A symlink shares bytes and a copy duplicates them. Neither can translate. agnostic-ai keeps one source in Markdown and YAML, aligned with the [`AGENTS.md`](https://agents.md/) open standard, and writes each tool's native files from it.

## One spec, different bytes

The same MCP server spec, as four tools need it:

| Tool | File | Shape |
|---|---|---|
| Claude Code | `.mcp.json` | JSON `mcpServers` map, remote entries carry `type` and `url` |
| Codex | `.codex/config.toml` | TOML `[mcp_servers.<name>]`, remote entries use `url` and `bearer_token_env_var` |
| Gemini CLI | `.gemini/settings.json` | JSON `mcpServers`, streamable HTTP uses `httpUrl` instead of `url` |
| Zed | `.zed/settings.json` | JSON `context_servers`, not `mcpServers` |

Everything else diverges the same way. Claude Code keeps full skill frontmatter while Codex reduces it to `name` and `description`. Hook events are `PreToolUse` on Claude Code and Codex, `BeforeTool` on Gemini, and `beforeShellExecution` on Cursor. Entry points live at `CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, `CONVENTIONS.md`, or `.rules`. Each [target page](@/docs/targets/_index.md) shows its exact output.

## Comparison

| Need | Symlink | Manual copy | Shared file + `@`-includes | agnostic-ai |
|---|---|---|---|---|
| Same prose, same format and path | Yes | Yes | Yes | Yes |
| Different format per tool (Markdown, TOML, YAML) | No | No | No | Yes |
| Different path per tool (`CLAUDE.md`, `AGENTS.md`, `.cursor/rules/`) | No | No | Partial | Yes |
| Different frontmatter and MCP schema per tool | No | No | No | Yes |
| Works on Windows and with `git core.symlinks=false` | No | Yes | Yes | Yes |
| No manual step on every change | Yes | No | Yes | Yes |
| Import an existing tool config | No | No | No | Yes |
| Fail CI when generated files drift | No | No | No | Yes |
| Merge into hand-written config keys, back up and revert | No | No | No | Yes |

Symlinks need admin rights or developer mode on Windows, and with `git core.symlinks=false`, the Git for Windows default, they check out as plain text files. `@`-includes only help where every tool resolves them: Claude Code does, most other tools do not.

## Where symlinks do work

When several tools need identical bytes, sync links them for you. `sync.shared-skills: true` keeps one copy of a byte-identical skill folder plus relative symlinks, unlinks a folder the moment one tool's output differs, and falls back to real copies where symlinks are unsupported. See [`sync.shared-skills`](@/docs/configuration.md#syncshared-skills).

## When you do not need agnostic-ai

- **One tool only.** Claude Code alone needs nothing more than its own files.
- **Two tools reading the same Markdown at the same path.** A symlink or copy is enough until the format or path differs.
- **Claude Code `@`-imports.** `@/RULES_SHARED.md` in `.claude/rules/base.md` pulls a shared body in natively.

Add a second tool with a different format, such as Codex TOML agents or Cursor `.mdc` rules, and each of these breaks. That is where agnostic-ai earns its place.

## Next steps

- New here: start with [Getting started](@/docs/getting-started.md).
- To trace a generated file back to its spec: see [`agnostic-ai why <file>`](@/docs/why.md).

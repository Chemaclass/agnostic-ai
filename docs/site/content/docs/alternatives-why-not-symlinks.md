+++
title = "Why not symlinks or manual copies?"
description = "See where native formats diverge and when agnostic-ai is worth using."
weight = 170

[extra]
group = "Reference"
+++

# Why agnostic-ai instead of symlinks or manual copies

You write `CLAUDE.md`. Then `.cursor/rules`. Then `GEMINI.md`. Then `AGENTS.md`. Same content, four formats. Switch tools and you rewrite everything.

The instinct is to fix this with a symlink or a copy. Both assume the problem is sharing one file across tools. It is not. Each tool reads a different native format at a different path, so the same logical spec must become different bytes per target. A symlink shares an inode and a copy duplicates bytes; neither can transform. Sharing one file violates at least one target's schema, carries fields it rejects, and breaks drift detection.

agnostic-ai keeps one source in plain Markdown plus YAML frontmatter, aligned with the [`AGENTS.md`](https://agents.md/) open standard. Run `sync` and every tool gets the config it expects, in its native location, byte-stable across runs.

## Three places the bytes diverge

### Skills

The same skill spec lands as a different file per target.

| Target | What it emits |
|--------|---------------|
| claude | `SKILL.md` with full `DocumentStyled` frontmatter (all keys from `spec.Meta`); sibling assets propagated byte-for-byte |
| codex | `SKILL.md` with reduced `FrontmatterOrdered` (only `name` + `description`, plus `x-codex` extras); optional `agents/openai.yaml` with interface/policy/dependencies; sibling assets propagated |
| amp | Folder per skill under `.agents/skills/<name>/SKILL.md`, reduced frontmatter (`name` + `description` only); sibling assets propagated |
| antigravity | Folder per skill under `.agents/skills/<name>/SKILL.md`, minimal frontmatter (`name` + `description`); sibling assets propagated |
| cursor | Native `.cursor/skills/<name>/SKILL.md` folder; bundled sibling assets propagate byte-for-byte |
| gemini | Reference-only by default; optional `.gemini/commands/skill-<name>.toml` when `emit-skills-as-commands: true` |

Claude emits full frontmatter. Codex, amp, cursor, and antigravity reduce it to `name` + `description` plus each tool's optional keys. Gemini has no skill folder at all. One shared file cannot be full-frontmatter and reduced-frontmatter at once, and one shared folder cannot carry codex's `agents/openai.yaml` or cursor's `metadata` without leaking them to tools that reject unknown files. Per-target override keys such as `x-cursor` have the same problem.

### MCP servers

| Target | What it emits |
|--------|---------------|
| claude | JSON `mcpServers` map at `.mcp.json`; stdio uses `command`/`args`/`env` (type-less, inferred default), HTTP/SSE uses `type`/`url`/`headers` |
| copilot | JSON `servers` map at `.vscode/mcp.json` (VS Code shape with explicit `type` field) |
| codex | TOML `[mcp_servers.<name>]` at `.codex/config.toml`; HTTP/SSE uses `url`/`bearer_token_env_var`/`http_headers` |
| gemini | JSON `mcpServers` map at `.gemini/settings.json`; http uses `httpUrl` (not `url`), sse uses `url` |
| amp | JSON `amp.mcpServers` dotted key at `.amp/settings.json`; HTTP/SSE uses `url`/`headers`, no `type` field |
| zed | JSON `context_servers` map at `.zed/settings.json` (note: not `mcpServers`); no `type` field |
| opencode | JSON `mcp` map at `opencode.json`; stdio uses `{type:'local', command:[...], environment}`, remote uses `{type:'remote', url, headers}` |
| continue | YAML per-server files at `.continue/mcpServers/<name>.yaml`; wrapper `{name, version, schema:v1, mcpServers:[...]}` |
| antigravity | JSON `mcpServers` map at `.agents/mcp_config.json`; remote uses `serverUrl` (the vendor doc says the legacy `url` / `httpUrl` names "are not supported"), no `type` field |

The JSON keys, the file path, and the file format (JSON vs TOML vs YAML) all differ. Gemini uses `httpUrl` for streamable-HTTP where most use `url`, and antigravity uses `serverUrl` and rejects both. Zed names the map `context_servers`. A shared file fits exactly one of these.

### Entry-point files

| Target | Entry-point file |
|--------|------------------|
| claude | `CLAUDE.md` |
| codex | `AGENTS.md` (shared with amp/warp/cline/junie/kiro/crush/trae/jules/goose/augment/qoder/openhands/factory/kilo/opencode) |
| amp | `AGENTS.md` (shared) |
| warp | `AGENTS.md` (shared) |
| opencode | `AGENTS.md` (shared) |
| gemini | `GEMINI.md` |
| aider | `CONVENTIONS.md` |
| copilot | `.github/copilot-instructions.md` |
| antigravity | `.agent/AGENTS.md` |

The pointer body is shared; the path is not. `AGENTS.md` is byte-identical across its seventeen consumers, but `CLAUDE.md` adds an optional `@`-import rules block, `GEMINI.md`, `CONVENTIONS.md`, and `.agent/AGENTS.md` each inline different rule content, and Zed reads `.rules`. One symlink points at one path.

Rules and hooks diverge the same way. Tools consume native per-rule files, root instruction blocks, or nested directory documents, and sync preserves [directory scope](@/docs/scoped-context.md) where supported. Hook event names differ (`SessionStart`/`PreToolUse`/`PostToolUse` on Claude and Codex, `BeforeTool`/`AfterTool` on Gemini, camelCase `beforeShellExecution`/`afterFileEdit` on Cursor), as do their formats. Zed has no lifecycle-hook surface, emitting hook specs as on-demand Zed Tasks when `outputs.zed.tasks-file` is set.

These differences live in the emit functions (`DocumentStyled` vs `FrontmatterOrdered`, the `httpUrl`/`url` branch in `buildMCPServer`, the inline-rules map), not in the spec you write.

## Where symlinks do work, sync manages them for you

When several targets render identical bytes, the duplication is real. `sync.shared-skills: true` collapses it into one canonical copy plus per-skill relative symlinks, planned from the rendered output each sync. See [`sync.shared-skills`](@/docs/configuration.md#syncshared-skills). Unlike a hand-made link, sync only links folders whose rendered bytes match, unlinks them the moment a target's render diverges, sweeps them with the skill, and degrades to real copies on filesystems without symlink support.

## Comparison

| Need | Symlink | Manual copy | Shared file + includes | agnostic-ai |
|------|---------|-------------|------------------------|-------------|
| Same prose to two tools, same format and path | Yes | Yes | Yes | Yes |
| Per-target format (Markdown vs TOML vs YAML) | No | No | No | Yes |
| Per-target path (`CLAUDE.md` vs `AGENTS.md` vs `.cursor/rules/`) | No | No | Partial (needs per-tool path) | Yes |
| Per-target frontmatter (full vs `name`+`description`) | No | No | No | Yes |
| Per-target MCP schema (`httpUrl` vs `url`, JSON vs TOML) | No | No | No | Yes |
| Bundled skill assets into nested layouts | No | No | No | Yes |
| Works on Windows without admin / dev mode | No | Yes | Yes | Yes |
| Survives `git core.symlinks=false` and archive extraction | No | Yes | Yes | Yes |
| No manual re-sync on every change | Yes | No | Yes | Yes |
| Round-trip import from an existing tool config | No | No | No | Yes |
| Drift detection as a CI gate | No | No | No | Yes |
| Backup and revert | No | No | No | Yes |
| Merge into user-authored config keys without clobbering | No | No | No | Yes |
| Works on tools that do not resolve `@`-imports | n/a | n/a | No | Yes |

Symlinks fail on Windows, where creating them needs admin privileges or developer mode. With `git core.symlinks=false`, the Git installer default there, they check out as plain text files holding the link target path. Archive extraction expands them too.

Manual copy carries no format or path translation, and no round-trip: a teammate who edits one tool's config has no path back to the source.

Shared file plus `@`-includes works only where every tool resolves imports. Claude Code does. Codex, Cursor, Copilot, Gemini, Aider, Cline, Windsurf, Continue, Zed, Warp, Amp, OpenCode, and Antigravity do not.

## What agnostic-ai adds that file-sharing cannot

Each of these runs adapter logic against both the source and the current disk state, so no static link or copy can do them.

| Capability | What it does |
|---|---|
| **Drift detection** (`sync --check`, `doctor`) | Compares emitted artifacts against disk to find missing and stale files, as text or JSON. Exits non-zero on drift for a CI gate. |
| **Revert** (`sync --backup`, `revert`) | Snapshots to `<path>.bak` before overwriting, restores those `.bak` files to pre-sync state, or with `--force` deletes unbacked generated files. |
| **Merge-preserving JSON emit** (`MergeJSONFile`) | Merges managed keys into `opencode.json`, `.amp/settings.json`, `.zed/settings.json` while preserving user-authored sibling keys and insertion order. |
| **Provenance headers** | Inserts a format-aware `<!-- Generated by agnostic-ai -->` marker so imports and migrations can detect generated files. Disable per target with `outputs.<target>.provenance-header: false`. |
| **Managed `.gitignore` block** | Rewrites a delimited block listing every path the enabled adapters emit, preserving hand-written lines outside it. Override per run with `--gitignore on|off`. |
| **Import** (`agnostic-ai import`) | Translates `CLAUDE.md`, `.cursor/rules`, `.codex/agents`, and others into specs, capturing non-spec keys into overlays for round-trip. |
| **Per-target overrides** (`outputs.<target>.*`) | Sets output paths and emission modes per target (`outputs.claude.dir`, `outputs.codex.skills-dir`, `outputs.cursor.commands-dir`, `outputs.gemini.emit-skills-as-commands`) without touching source specs. |
| **Per-target model maps** | `model: {claude: opus, codex: gpt-5.5, default: gpt-4o}`, or `x-<target>.model: null` to delete a model line that would otherwise emit. |
| **Capability reporting** | Reports which spec kinds each target supports and warns on specs a target cannot handle. Set `on-unsupported: warn|error|silent`. |

## When you do NOT need agnostic-ai

Reach for the simple option when there is no format or path divergence.

- **One tool only.** Claude Code alone, reading `.claude/agents/` and `.claude/skills/` with no second format, needs nothing more than a plain file.
- **Identical-format pairs at compatible paths.** Two tools reading the same Markdown at the same relative path can share by symlink or copy. That breaks once either the format or the structure differs.
- **Claude-only `@`-imports.** `@/RULES_SHARED.md` in `.claude/rules/base.md` pulls the shared body natively.

Add a second tool with a different format (Codex TOML agents, Cursor `.mdc` rules) and every one of these breaks. That is the boundary where agnostic-ai earns its place.

## Next steps

- New here: start with [Getting started](@/docs/getting-started.md).
- To trace an emitted file back to its source spec, adapter, and sync time: see [`agnostic-ai why <file>`](@/docs/why.md).

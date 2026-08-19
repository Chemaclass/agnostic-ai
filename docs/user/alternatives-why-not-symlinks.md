# Why agnostic-ai instead of symlinks or manual copies

You write `CLAUDE.md`. Then `.cursor/rules`. Then `GEMINI.md`. Then `AGENTS.md`. Same content, four formats. Switch tools and you rewrite everything.

The instinct is to fix this with a symlink or a copy. Link one file into each tool's path, or copy it by hand on every change. Both assume the real problem is "share one file across tools". It is not.

The real problem is that each tool reads a different native format at a different path. A symlink shares bytes. It cannot transform them. The same logical spec must become different bytes for each target, because each target's adapter emits a different schema. A shared inode or an unmodified copy violates at least one target's native schema, carries unsupported fields, and breaks sync drift detection.

agnostic-ai keeps one source of truth in plain Markdown plus YAML frontmatter, aligned with the [`AGENTS.md`](https://agents.md/) open standard. Run `sync` and every tool gets the config it expects, in its native location, byte-stable across runs.

## The core argument: one spec, divergent bytes

A symlink shares the same inode. A copy duplicates the same bytes. Both deliver identical content to every path. But identical content is wrong, because each adapter transforms the source into a target-specific shape. Three examples make this concrete.

### Skills diverge in frontmatter, layout, and assets

The same skill spec lands as different files per target.

| Target | What it emits |
|--------|---------------|
| claude | `SKILL.md` with full `DocumentStyled` frontmatter (all keys from `spec.Meta`); sibling assets propagated byte-for-byte |
| codex | `SKILL.md` with reduced `FrontmatterOrdered` (only `name` + `description`, plus `x-codex` extras); optional `agents/openai.yaml` with interface/policy/dependencies; sibling assets propagated |
| amp | Folder per skill under `.agents/skills/<name>/SKILL.md`, reduced frontmatter (`name` + `description` only); sibling assets propagated |
| antigravity | Folder per skill under `.agents/skills/<name>/SKILL.md`, minimal frontmatter (`name` + `description`); sibling assets propagated |
| cursor | Native `.cursor/skills/<name>/SKILL.md` folder; bundled sibling assets propagate byte-for-byte |
| gemini | Reference-only by default; optional `.gemini/commands/skill-<name>.toml` when `emit-skills-as-commands: true` |

Claude emits full frontmatter. Codex, amp, cursor, and antigravity reduce it to `name` + `description` plus each tool's documented optional keys. Gemini has no skill folder at all. A single shared file cannot be full-frontmatter and reduced-frontmatter at once, and a shared folder cannot carry per-tool extras (codex's `agents/openai.yaml`, cursor's `metadata`) without leaking them to tools that reject unknown files or keys.

### MCP server config diverges in schema and key names

The same MCP server spec emits a different schema per target.

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

Gemini uses `httpUrl` for streamable-HTTP; everyone else uses `url`, except antigravity, which uses `serverUrl` and rejects `url`/`httpUrl` outright. Amp and Zed omit the `type` field and infer transport from shape. Zed names the map `context_servers`, not `mcpServers`. OpenCode wraps stdio in `{type:'local'}`. Continue wraps each server in a YAML block with `name`/`version`/`schema`. The JSON keys, the file path, and even the file format (JSON vs TOML vs YAML) differ. A shared file fits exactly one of these.

### Entry-point files share a pointer body but diverge on path and append blocks

Each tool reads its instruction file at a tool-specific path.

| Target | Entry-point file |
|--------|------------------|
| claude | `CLAUDE.md` |
| codex | `AGENTS.md` (shared with amp/warp/zed/cline/junie/kiro/crush/trae/jules/goose/augment/qoder/openhands/factory/kilo) |
| amp | `AGENTS.md` (shared) |
| warp | `AGENTS.md` (shared) |
| gemini | `GEMINI.md` |
| aider | `CONVENTIONS.md` |
| copilot | `.github/copilot-instructions.md` |
| opencode | `AGENTS.md` |
| antigravity | `.agent/AGENTS.md` |

The canonical pointer body is shared. The path is not. `AGENTS.md` is byte-identical across its sixteen consumers (codex, amp, warp, zed, cline, junie, kiro, crush, trae, jules, goose, augment, qoder, openhands, factory, kilo) for the pointer body and the rules inline block. But `CLAUDE.md` lives at its own path with an optional `@`-import rules block. `GEMINI.md`, `CONVENTIONS.md`, `.rules`, and `.agent/AGENTS.md` each have a different path and different inlined rule content. One symlink points at one path, so it cannot serve every tool's location at once.

### Rules delivery is path-divergent

Claude emits per-file `.claude/rules/*.md`, which current Claude Code versions auto-load at session start (`outputs.claude.rules-mode: import` wires them via `@`-imports for older versions). Codex, amp, warp, zed, gemini, aider, opencode, crush, jules, goose, openhands, factory, and kilo have no native rules directory, so they inline every rule body into their shared entry-point file under a sentinel-marked `## Rules` block. Junie has no native rules directory either, but inlines into its own preferred `.junie/AGENTS.md` instead of that shared file, alongside a sentinel-marked `## Agents` block no other inlining target has (Junie has no native per-agent surface at all, see #552). Cursor, cline, windsurf, continue, kiro, trae, qoder, antigravity, and augment each get one file per rule in their own directory and format; augment inlines into `AGENTS.md` too, on top of its native `.augment/rules/` directory. A shared source directory cannot satisfy "separate per-rule files" and "inlined block in one file" at the same time.

### Hooks are target-specific

Event names differ: Claude and Codex use `SessionStart`/`PreToolUse`/`PostToolUse`, Gemini uses `BeforeTool`/`AfterTool`, Cursor uses camelCase `beforeShellExecution`/`afterFileEdit`. Format differs: JSON vs TOML. Zed has no lifecycle-hook surface; when `outputs.zed.tasks-file` is set, hook specs emit as on-demand Zed Tasks runnable from the command palette. Aider, Cline, Windsurf, Continue, Antigravity, Amp, Warp, Copilot, Junie, Kiro, Crush, and Trae skip hooks entirely. No single hook file is correct for more than one of these.

These divergences live at the emit-function level (`DocumentStyled` vs `FrontmatterOrdered`, the `httpUrl`/`url` branch in `buildMCPServer`, the inline-rules map), not at the spec input level. A skill with per-target overrides (`x-cursor` keys, codex-only `agents/openai.yaml`) must become different bytes per target. A symlink or unmodified copy cannot transform.

### Where symlinks do work, sync manages them for you

When several targets DO render identical bytes (a plain skill folder on codex, amp, zed, and crush), the duplication is real, and `sync.shared-skills: true` collapses it: one canonical copy plus per-skill relative symlinks, planned from the rendered output each sync. See [`sync.shared-skills`](configuration.md#syncshared-skills). The difference from hand-rolled links: sync only links folders whose rendered bytes match, unlinks them the moment a target's render diverges, sweeps them with the skill, and degrades to real copies on filesystems without symlink support. A hand-made `.claude/skills -> ../.cursor/skills` link has none of those guards.

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

Symlinks fail on Windows: creating them needs admin privileges or developer mode. With `git core.symlinks=false` (the Git installer default on Windows), symlinks check out as plain text files holding the link target path, not working links. Archive extractions expand symlinks to their target content too.

Manual copy carries no format or path translation. `cp SHARED.md .codex/rules/base.md` produces invalid TOML at a Codex path that wants TOML. It also has no round-trip: a teammate who edits one tool's config has no path back to the source.

Shared file plus `@`-includes works only where every tool resolves imports and accepts the same format. Claude Code resolves `@`-imports natively. Codex, Cursor, Copilot, Gemini, Aider, Cline, Windsurf, Continue, Zed, Warp, Amp, OpenCode, and Antigravity do not. Add any of those and the include directive does not bridge the format or path gap.

## What agnostic-ai adds that file-sharing cannot

File-sharing copies bytes. These features require executing adapter logic against both source and disk state. A static link or copy cannot do any of them.

- **Drift detection** (`sync --check` / `doctor`): compares emitted artifacts against disk to find missing files (never synced) and stale files (hand-edited or out of date). Reports per-target drift as human-readable or JSON output. Exits non-zero on any drift for a CI gate. A symlink has no knowledge of its source or what changed, so it cannot compare.
- **Revert with `.bak` restore** (`sync --backup` / `revert`): snapshots existing files to `<path>.bak` before overwriting on content change. `revert` restores `.bak` files to pre-sync state, or with `--force` deletes unbacked generated files. Preserves user-authored content sharing a path with adapter output. Symlinks and copies are one-way and track no original state.
- **Merge-preserving JSON emit** (`MergeJSONFile`): reads existing JSON config (`opencode.json`, `.amp/settings.json`, `.zed/settings.json`), merges managed keys on top, and preserves user-authored sibling keys and insertion order. A copy overwrites the whole file and loses user keys.
- **Provenance headers**: inserts a format-aware `<!-- Generated by agnostic-ai -->` marker (Markdown comment, TOML comment, shell comment) so imports and migrations can detect generated files. Suppress per-target with `outputs.<target>.provenance-header: false`. A static copy cannot inject context-sensitive headers per format.
- **Auto-managed `.gitignore` block**: rewrites a delimited managed block listing every path the enabled adapters emit, preserving hand-written lines outside the block. Override per run with `--gitignore on|off`. A symlink cannot maintain a delimited block keyed to which adapters are enabled.
- **Import of existing tool config** (`agnostic-ai import`): translates `CLAUDE.md`, `.cursor/rules`, `.codex/agents`, and others into agnostic-ai specs. Parses Markdown frontmatter, TOML, and shell commands; captures non-spec keys into overlays for round-trip. A symlink cannot convert formats or restructure semantics.
- **Per-target overrides** (`outputs.<target>.*`): configure output paths and emission modes per target (`outputs.claude.dir`, `outputs.codex.skills-dir`, `outputs.cursor.commands-dir`, `outputs.gemini.emit-skills-as-commands`) without touching source specs. A fixed reference does not adapt to configuration.
- **Per-target model maps with null-delete**: specs declare `model: {claude: opus, codex: gpt-5.5, default: gpt-4o}`, or `x-<target>.model: null` to delete a model line that would otherwise emit. A copy cannot run conditional logic by target.
- **Capability and unsupported-kind reporting**: reports which spec kinds each target supports and warns when specs exist for kinds a target cannot handle. Configurable with `on-unsupported: warn|error|silent`. This is dynamic metadata that cannot be precomputed into a static link.

## When you do NOT need agnostic-ai

Reach for the simple option when there is no format or path divergence.

- **One tool only.** If you use Claude Code alone (reading `.claude/agents/` and `.claude/skills/` with no second format), a symlink or plain file is enough.
- **Identical-format pairs at compatible paths.** Two tools that both read the same Markdown at the same relative path can share by symlink or copy. The pairing breaks once either the format or the structure differs (per-rule files vs one concatenated entry-point).
- **Claude-only `@`-imports.** If you use only Claude Code, `@/RULES_SHARED.md` in `.claude/rules/base.md` pulls the shared body natively.

Add a second tool with a different format (Codex TOML agents, Cursor `.mdc` rules) and every one of these breaks. That is the boundary where agnostic-ai earns its place.

## Next steps

- New here: start with [Getting started](getting-started.md).
- To trace an emitted file back to its source spec, adapter, and sync time: see [`agnostic-ai why <file>`](why.md).

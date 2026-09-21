+++
title = "Warp"
description = "How agnostic-ai emits Warp configuration: native paths, capability limits, and output options."
weight = 120

[extra]
group = "Reference"
target_id = "warp"
+++

# Warp (`warp`)

## Output

```
AGENTS.md                              # canonical entry-point pointer body (written by sync, shared across the AGENTS.md consumers)
.agents/skills/<name>/SKILL.md         # one folder per skill (shared tree with codex/amp/zed)
.warp/workflows/<name>.yaml            # one per agent, only when workflows-dir is set
.warp/.mcp.json                        # when MCP entries exist
```

- **Rules**: root rules inline into `AGENTS.md`. Scoped rules use `<scope>/AGENTS.md`; see [directory-specific instructions](@/docs/scoped-context.md) for shared-reader compatibility.
- **Skills**: native [Warp skills](https://docs.warp.dev/agents/capabilities/skills) folders at `.agents/skills/<name>/SKILL.md`, the vendor's own recommended path and the cross-tool tree codex, amp, and zed emit too. A source-layout scope moves the native tree under that directory and survives import. Identical root renders dedupe into one write.
  - Warp's docs list ten scanned directories in total: `.agents/skills/` (recommended), plus `.warp/skills/`, `.claude/skills/`, `.codex/skills/`, `.cursor/skills/`, `.gemini/skills/`, `.copilot/skills/`, `.factory/skills/`, `.github/skills/`, and `.opencode/skills/`. A `WARP_SKILL_DIRS` env var indexes further directories, scoped to Cloud agents indexing skills that live outside the repo, not a general extension of the ten scanned directories. This adapter only writes the recommended path. `.opencode/skills/` is OpenCode's own default skills directory, so a project running both tools gets Warp skill-scanning for free with no extra write.
- **Workflows**: when `outputs.warp.workflows-dir` is set, each agent emits as a [Warp Workflow](https://docs.warp.dev/terminal/entry/yaml-workflows) YAML at `<dir>/<name>.yaml` (`name`/`command`/`description`/`tags`). The vendor page now opens with a caution that it recommends new workflows in Warp Drive instead "for a better editing experience"; nothing breaks here, since `{{path_to_git_repo}}/.warp/workflows/` still loads and Warp Drive workflows are cloud-stored, so they are not a file this adapter could emit as an alternative.
  - The `command:` is the agent body verbatim; tailor it to a Warp-friendly shell snippet. Other documented workflow fields (`shells`, `arguments`, `source_url`, `author`, `author_url`) pass through when declared under `x-warp`; `import warp` captures them back the same way.
- **MCP**: written into `.warp/.mcp.json` under the standard `mcpServers` map. A stdio server's `command`/`args`/`env` carry through, plus `working_directory`: [docs.warp.dev/agents/capabilities/mcp](https://docs.warp.dev/agents/capabilities/mcp) documents it as "Working directory path where the command is run, used for resolving relative paths," Warp's own name for the cross-tool spec's `cwd` field. A remote server (HTTP/SSE/WS) carries `url`/`headers`; Warp's remote-server table has no transport discriminant at all, so no `type` field is ever emitted, unlike claude, cursor, and the rest of the shared-builder targets. Those two tables are the whole emitted key set. The CLI Server table marks `args` required ("| `args` | string[] | **Yes** | Array of command-line arguments passed to `command`"), so a stdio server with no arguments emits `"args": []` rather than omitting the key (target-audit 2026-09-18, #859).
  - `description`, `disabled`, and `roots` appear in neither and no longer emit (target-audit 2026-08-27, #641): they came from the shared builder this adapter used before it grew its own, carried through that split unexamined. Writing a key from outside a closed vendor list asserts support no vendor sentence backs. `disabled` raises a coverage note instead of vanishing, and little is lost either way, since the same page says "project-scoped servers never auto-spawn" there. `description` and `roots` stay reachable through `x-warp` for anyone who wants them written anyway.
  - `import warp` reads `.warp/.mcp.json` back, renaming `working_directory` to `cwd`. An entry missing the field its table marks required (`command` on the CLI Server table, `url` on the URL Server one) emits nothing at all, the same call trae, antigravity, and windsurf make. Until #753 the missing key was left out and the rest of the entry was written, so a spec carrying only `args` or `env` produced a server Warp lists and cannot launch.
- **Legacy rename**: Warp's docs recommend `AGENTS.md` for new projects but still fully support `WARP.md`, and rank it first: "If both `WARP.md` and `AGENTS.md` exist in the same directory, `WARP.md` takes priority" (docs.warp.dev/agents/capabilities/rules, target-audit 2026-09-08, #691). On first sync after upgrading, any agnostic-generated `WARP.md` at the configured root is renamed to `WARP.md.bak` so the new `AGENTS.md` layout takes over. A user-authored `WARP.md` (no `Generated by agnostic-ai` marker) is left untouched, but since it still outranks `AGENTS.md`, sync warns that none of the synced rules reach Warp until that file is renamed or removed.

## Config keys

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.warp.skills-dir` | `.agents/skills` | |
| `outputs.warp.workflows-dir` | empty | opt-in |
| `outputs.warp.mcp-file` | `.warp/.mcp.json` | |
| `outputs.warp.rules-file` | unset | writes legacy concatenated rules and skips the pointer-body write |

## Import

Skills import from each documented project root: `.agents`, `.warp`, `.claude`, `.codex`, `.cursor`, `.gemini`, `.copilot`, `.factory`, `.github`, and `.opencode`, each followed by `/skills/`. Earlier roots win duplicate names within the same scope; scoped paths and bundled assets stay together.

`agnostic-ai import warp` reads `AGENTS.md`, workflows from `.warp/workflows/`, and MCP servers from `.warp/.mcp.json`, as described under **Workflows** and **MCP**. It copies `.agents/skills/<name>/SKILL.md` and all bundled assets.

## Verify

1. Install Warp from [warp.dev](https://www.warp.dev).
2. Check the tree: `ls AGENTS.md .agents/skills/ .warp/workflows/ .warp/.mcp.json`, `test -f .agents/skills/*/SKILL.md`, `grep "Generated by agnostic-ai" .warp/workflows/*.yaml` for the provenance header, `python -m json.tool .warp/.mcp.json > /dev/null`.
3. Open the project; confirm the rules panel surfaces `AGENTS.md` (no "unrecognized file" warnings), the skills picker lists each `.agents/skills/<name>/`, the Warp Drive workflows picker shows every `<workflows-dir>/<name>.yaml`, and the MCP picker lists every `mcpServers.<name>` from `.warp/.mcp.json`.

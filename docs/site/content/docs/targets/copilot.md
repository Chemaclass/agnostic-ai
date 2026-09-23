+++
title = "GitHub Copilot"
description = "How agnostic-ai emits GitHub Copilot configuration: native paths, capability limits, and output options."
weight = 50

[extra]
group = "Reference"
target_id = "copilot"
+++

# GitHub Copilot (`copilot`)

## Output

```
.github/copilot-instructions.md                            # canonical entry-point pointer body (written by sync)
.github/instructions/<name>.instructions.md                # scoped rule per file
.github/agents/<name>.agent.md                             # one custom-agent profile per agent
.github/skills/<name>/SKILL.md                             # one folder per skill, bundled assets included
.vscode/mcp.json                                           # when MCP entries exist; VS Code's file, servers key
.github/mcp.json                                           # when MCP entries exist; Copilot CLI's file, mcpServers key
.mcp.json                                                  # only with outputs.copilot.root-mcp-file; mcpServers key
.github/hooks/agnostic-ai.json                              # when hook entries exist
.github/copilot/settings.json                              # when a Settings model or a disabled MCP server exists
```

- **Rules**: Copilot supports path-scoped instructions via `applyTo:` frontmatter. Rules with `globs` (or a source-layout scope like `rules/backend/auth.md`) emit as a separate `.instructions.md` with `applyTo` derived from globs (explicit `globs` wins, else `<scope>/**`). Always-on rules (no globs, no scope, or `alwaysApply: true`) skip per-file emission and are reachable via the pointer body plus the source spec dir. The adapter manages no rule frontmatter keys: every `x-copilot` key lands alongside `applyTo`.
- **Agents**: native [custom agent profiles](https://docs.github.com/en/copilot/reference/custom-agents-configuration) at `.github/agents/<name>.agent.md`: frontmatter `name` + `description` plus `tools` and `model` when the spec declares them. The profile has no effort key, so a portable `effort` is not written and raises a coverage note. Per-agent `effortLevel` exists only in the user-tier `subagents.agents` setting of `~/.copilot/settings.json`. Arbitrary `x-copilot` keys (`target`, `user-invocable`, `mcp-servers`, `effort`, ...) pass through as written. The body is the agent prompt. The old flattened `agent-<name>.instructions.md` copies are gone; the ledger sweeps them.
- **Skills**: native [Copilot skills](https://docs.github.com/en/copilot/how-tos/copilot-on-github/customize-copilot/customize-cloud-agent/add-skills) folders at `.github/skills/<name>/SKILL.md` (Copilot also scans `.claude/skills/` and `.agents/skills/`), with bundled sibling files propagated byte-for-byte. The old flattened `skill-<name>.instructions.md` copies are gone; the ledger sweeps them.
- **Chat modes**: when `outputs.copilot.chatmodes-dir` is set, each agent also emits as a [Custom Chat Mode](https://code.visualstudio.com/docs/copilot/customization/custom-chat-modes) at `<dir>/<name>.chatmode.md` with `description`/`model`/`tools` frontmatter. Chat modes are a VS Code Copilot surface, not a docs.github.com one; the three GitHub Docs successors to the old citation page carry no chat-mode section, which is why this one deliberately points at VS Code's own docs instead. The native agent profile still emits alongside.
- **Settings**: the last portable `model` value merges into `.github/copilot/settings.json`, where Copilot CLI documents it as the repository default model. Other repository settings survive. `import copilot` restores the model to `settings/imported.yaml` and leaves target-only keys in the native file. An `x-copilot` block on a settings spec merges into the same file, so a repository key this tool does not model, such as `respectGitignore`, is reachable from a spec instead of a hand edit the next sync overwrites. The repository table's one deny route rides the same hatch: `x-copilot.deniedUrls` writes the list Copilot documents as "URLs or domains blocked". No portable rule reaches it. This project has no domain scope, the nearest spelling is Claude's `WebFetch(domain:...)`, and the vendor documents wildcard support on `allowedUrls` alone, which is user-tier and absent from the repository table. So deny reaches the repository file and allow never can (#959).
  - Every MCP spec marked `disabled: true` has its name written to `disabledMcpServers` in the same file, sorted. That is the one file-based route Copilot publishes for it: "`disabledMcpServers` | `string[]` | Union—repository can add entries, never remove | MCP servers configured but not started" ([cli-config-dir-reference](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-config-dir-reference)), under "Only the keys listed in the following table are supported at the repository level." Until #888 a `disabled: true` server started anyway under Copilot CLI while the coverage note said no such route existed.
- **MCP**: two files, because Copilot has two readers that disagree on which one to open.

  `.vscode/mcp.json` uses the VS Code schema: top-level `servers` key, each entry carrying a `type` (`stdio`, `http`, or `sse`). VS Code's Agent Host does not read that file itself: [the vendor doc](https://code.visualstudio.com/docs/agent-customization/mcp-servers) says it "doesn't read `.vscode/mcp.json` directly" and that VS Code forwards the config instead, except servers needing interactive input.

  `.github/mcp.json` carries the same servers under `mcpServers` for Copilot CLI, which its [project-level discovery table](https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-mcp-servers) lists as "Shared configuration that is committed to the repository". That second file exists because the CLI rejects the VS Code wrapper outright: "The `.vscode/mcp.json` file for VS Code is not read by Copilot CLI. It uses the unsupported top-level key `servers`." Before #646 only the VS Code file emitted, so a Copilot CLI user with no VS Code in the loop got no MCP server at all. Override either path with `outputs.copilot.mcp-file` / `outputs.copilot.cli-mcp-file`.

  VS Code MCP output owns the complete `servers` map and preserves unrelated top-level keys, including `inputs` and `sandbox`, at the default or configured path. JSONC is accepted; sync removes comments and normalizes formatting. Invalid JSONC aborts the write. `.github/mcp.json` and the opt-in root mirror remain managed as whole documents.

  A per-server `tools` allowlist reaches `.github/mcp.json` and the opt-in root mirror: "Enter `*` to include all tools, or provide a comma-separated list of tool names (no quotes needed). The default is `*`" ([add-mcp-servers](https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-mcp-servers)), whose own configuration-file example carries `"tools": ["*"]` on both a local and an http server. It stays off `.vscode/mcp.json`, whose reference documents no such key, and off every other target: Codex has a `tools` key too and it is a map of per-tool sub-tables, a different shape (#888).

  VS Code alone accepts stdio `cwd`, `envFile` (for example `${workspaceFolder}/.env`), and `sandboxEnabled` (macOS and Linux only), remote `oauth: {clientId, enterpriseManaged}`, and `dev.watch` (a glob or glob array that restarts the server on change) on stdio, HTTP, and SSE servers. `dev.debug`, shaped `debug: {type: "node"|"debugpy", debugpyPath}`, is stdio-only; a remote server that sets it gets a coverage note while its watch patterns still emit. These fields stay out of the Copilot CLI file and root mirror. See [VS Code MCP configuration](https://code.visualstudio.com/docs/agents/reference/mcp-configuration). A `roots` list still emits when a spec declares one, but no VS Code or GitHub page documents `roots` as a per-server key, so treat it as passthrough rather than a supported field: the string does not appear at all on the VS Code reference this bullet cites (target-audit 2026-09-20). The same claim stood on the Claude and Cursor pages and was wrong there too; all three describe one shared MCP builder, so check that emitter rather than any single target page.

  The same table's third entry, a `.mcp.json` anywhere from the working directory up to the repository root, stays opt-in behind `outputs.copilot.root-mcp-file: .mcp.json`. The project root is shared ground with Claude Code rather than Copilot's own directory, and a repository-root file is a surprise for a project that does not need it. VS Code itself reads that file: [MCP servers](https://code.visualstudio.com/docs/agent-customization/mcp-servers) names it a standard location, "Workspace, portable format: create .mcp.json at the root of your project. This format defines servers in a top-level mcpServers object and works across compatible tools," next to `.vscode/mcp.json`. So turning on `root-mcp-file` in a trusted VS Code workspace starts its servers alongside `.vscode/mcp.json`'s, both exempt from the interactive trust prompt: "servers in .vscode/mcp.json and workspace-root .mcp.json can start without a separate MCP server trust prompt." Whether VS Code dedupes a same-name server across the two files is undocumented; the page states neither a precedence nor a merge rule, so do not assume either. Claude Code writes its own root `.mcp.json` under `mcpServers` too, so enabling both targets produces identical bytes at one path; the collision check compares content, not owners, so it stays quiet and sync writes the file once.
- **Hooks**: land in `.github/hooks/agnostic-ai.json` (override via `outputs.copilot.hooks-file`), one of possibly several `*.json` files Copilot loads and merges from that directory: "Repository-level hook files: `.github/hooks/*.json` in the repository root", read by both Copilot CLI and Copilot cloud agent ([docs.github.com/en/copilot/reference/hooks-reference](https://docs.github.com/en/copilot/reference/hooks-reference), "Hooks locations"). The wrapper is `{"version": 1, "hooks": {...}}` with an **integer** version. Each hook entry is a flat object carrying `matcher` directly (`{"type": "command", "matcher": ..., "command": ..., "timeoutSec": ...}`), not Claude Code's nested `{matcher, hooks: [...]}` group. 14 events exist today, one more than when #629 was filed.

  `event:` passes through verbatim. The vendor documents both the PascalCase vocabulary Claude Code, Codex, OpenHands, Windsurf, and Qoder share (`PreToolUse`) and Copilot's own camelCase form (`preToolUse`) as independently valid keys in this same file, selecting between its "VS Code compatible" and "camelCase" hook payload formats respectively. The PascalCase form also carries Claude's own matcher semantics and tool names, so `matcher: Bash` reaches it unchanged. The camelCase form answers only to Copilot's own lowercase tool names (`bash`, `edit`, `view`, ...), and pairing it with a Claude-style matcher earns a coverage note rather than a guessed rename.

  Two of the 14 rows are camelCase-only: `userPromptTransformed` and `subagentStart` ("A subagent is spawned (before it runs)."). Neither has a PascalCase pairing anywhere on the page, so a spec spelling either one in PascalCase emits a key Copilot parses and never fires. `SubagentStart` is the one that bites, since Claude Code and Codex both accept it (target-audit 2026-09-11, #737). `validate` now flags it. It used to pass, because the validator's event table is a separate list from the emitter's and only the emitter carried the caveat (#888).

  A command hook also carries `cwd` and `env`, Copilot's own two command-hook fields: "`cwd` | string | No | Working directory for the command (relative to repository root or absolute)." and "`env` | object | No | Environment variables to set (supports variable expansion)." Set them at the spec's top level or under `x-copilot`. Both stay copilot-only, because Claude Code's command-hook field table has neither, and both round-trip through `import copilot` (#888).

  Command, HTTP (`url`, `headers`, `allowedEnvVars`), and `sessionStart` prompt handlers all survive `import copilot` and re-emission.

  A hook spec that sets `args` emits Copilot's own shell-free form instead: `{"type": "command", "exec": <command>, "args": [...]}`. The field table is explicit that the executable moves out of `command` (`exec` is accepted "Instead of `bash`, `powershell`, and `command`", and "Do not combine `exec` with `bash`, `powershell`, or `command`"), which is where this differs from Claude Code, whose exec form keeps the executable in `command`. That is what `args` is for: a path or argument carrying a space, running as written.

  It costs one surface: `exec` and `args` are both marked "Only supported in Copilot CLI", and a cloud agent job reads the same `.github/hooks/*.json` files where "A subset of events fires, and only `bash` (or `command`) entries are honored." An exec-form entry is skipped whole there. `sync` raises a coverage note naming that trade. Leave `args` unset for a hook that must run under cloud agent (#755).

  **Every hook runs twice when you sync `claude` and `copilot` together.** The same "Hooks locations" section names both files: `.github/hooks/*.json` and, as a cross-tool read, `.claude/settings.json` and `.claude/settings.local.json`. Sources are "loaded ... and combined", and "When the same event appears in multiple sources, all hook entries from all sources are run."

  Both targets are in the default target list, so this is the out-of-the-box path: a formatter runs twice, an audit hook double-writes, and a blocking `preToolUse` returns two decisions per tool call. Copilot names no toggle for that read, unlike Cursor and Trae, which gate theirs behind an off-by-default switch. Until there is one, give the hook spec a single `target:` rather than both (#755).

  **Hooks are only part of that cross-read.** The [Copilot CLI configuration directory](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-config-dir-reference) names a five-key subset: "The CLI also reads `.claude/settings.json` and `.claude/settings.local.json` for the shared cross-tool subset of repository settings (such as `companyAnnouncements`, `disableAllHooks`, `enabledPlugins`, `extraKnownMarketplaces`, and `hooks`)." Two of the five are keys agnostic-ai writes today: `enabledPlugins`, through `outputs.claude.settings.enabledPlugins`, and `hooks`, through either target. So `outputs.claude.settings.enabledPlugins` sets Copilot CLI's repository plugin policy too, from a file the copilot adapter never touches and `.github/copilot/settings.json` never mentions. Set it deliberately, or drop `claude` from the target list, if the two tools should differ (#956).

## Config keys

| Key | Default | Notes |
|---|---|---|
| `outputs.copilot.instructions-dir` | `.github/instructions` | |
| `outputs.copilot.agents-dir` | `.github/agents` | |
| `outputs.copilot.skills-dir` | `.github/skills` | |
| `outputs.copilot.mcp-file` | `.vscode/mcp.json` | |
| `outputs.copilot.cli-mcp-file` | `.github/mcp.json` | Copilot CLI's own file, `mcpServers` key |
| `outputs.copilot.root-mcp-file` | unset, opt-in | writes the same servers to a workspace-root `.mcp.json` under the `mcpServers` key |
| `outputs.copilot.chatmodes-dir` | empty, opt-in | emits one Custom Chat Mode per agent |
| `outputs.copilot.rules-file` | unset | writes always-on rules concatenated at that path and skips the pointer-body write |
| `outputs.copilot.hooks-file` | `.github/hooks/agnostic-ai.json` | |

## Import

`agnostic-ai import copilot` reads:

| Source | Becomes |
|---|---|
| `.github/copilot-instructions.md` | `<rules>/<slug>.md` per `##` section, plus a copy at `.agnostic-ai/AGNOSTIC_AI.md` |
| `.github/instructions/<name>.instructions.md` | `<rules>/<name>.md`; the `agent-` and `skill-` filename prefixes become agents and skills |
| `.github/agents/<name>.agent.md` and `.github/chatmodes/<name>.chatmode.md` | `<agents>/<name>.md`, keeping the frontmatter keys copilot does not emit |
| `.github/skills/<name>/`, `.claude/skills/<name>/`, `.agents/skills/<name>/` | `<skills>/<name>/` |
| `.github/hooks/*.json` | one target-scoped hook spec per handler |
| `.vscode/mcp.json` | `<mcps>/<name>.yaml` |
| `.github/copilot/settings.json` `model` | the portable Settings source |

All three skill directories are read because [Add skills](https://docs.github.com/en/copilot/how-tos/copilot-on-github/customize-copilot/customize-cloud-agent/add-skills) documents all three as project skill locations. The emit path wins a same-name collision, then `.claude/skills`, then `.agents/skills` (#854).

## Verify

1. Install the [GitHub Copilot extension](https://marketplace.visualstudio.com/items?itemName=GitHub.copilot) (and optional Copilot Chat) in VS Code.
2. Check the tree: `ls .github/copilot-instructions.md .github/instructions/ .vscode/mcp.json .github/mcp.json`, `grep "Generated by agnostic-ai" .github/instructions/*.md` for the provenance header (it sits after the `applyTo` frontmatter), `python -m json.tool .vscode/mcp.json > /dev/null && python -m json.tool .github/mcp.json > /dev/null`.
3. Open the project. Copilot loads `.github/copilot-instructions.md` plus every matching `.instructions.md`. The `Output → GitHub Copilot` channel shows no "failed to parse instructions" warnings.
4. Open a file matching a rule glob (e.g. a `.go` file for `applyTo: "**/*.go"`) and trigger chat; the rule body shows in context.
5. If MCPs are configured, run `MCP: Show Installed Servers`; each `servers.<name>` from `.vscode/mcp.json` is ready. In Copilot CLI, run `/mcp` from the project root after accepting the folder-trust prompt; each `mcpServers.<name>` from `.github/mcp.json` is listed.
6. If hooks are configured, `python -m json.tool .github/hooks/agnostic-ai.json > /dev/null` confirms the file parses. In Copilot CLI, a hook set on `PreToolUse` or `preToolUse` fires on the next matching tool call; the CLI's own hook log names which file and entry ran.

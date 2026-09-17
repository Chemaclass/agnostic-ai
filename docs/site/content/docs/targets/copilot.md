+++
title = "GitHub Copilot"
description = "How agnostic-ai emits GitHub Copilot configuration: native paths, capability limits, and output options."
weight = 50

[extra]
group = "Reference"
target_id = "copilot"
+++

# GitHub Copilot (`copilot`)

```
.github/copilot-instructions.md                            # canonical entry-point pointer body (written by sync)
.github/instructions/<name>.instructions.md                # scoped rule per file
.github/agents/<name>.agent.md                             # one custom-agent profile per agent
.github/skills/<name>/SKILL.md                             # one folder per skill, bundled assets included
.vscode/mcp.json                                           # when MCP entries exist; VS Code's file, servers key
.github/mcp.json                                           # when MCP entries exist; Copilot CLI's file, mcpServers key
.mcp.json                                                  # only with outputs.copilot.root-mcp-file; mcpServers key
.github/hooks/agnostic-ai.json                              # when hook entries exist
.github/copilot/settings.json                              # when a Settings model exists
```

- **Rules**: Copilot supports path-scoped instructions via `applyTo:` frontmatter. Rules with `globs` (or a source-layout scope like `rules/backend/auth.md`) emit as a separate `.instructions.md` with `applyTo` derived from globs (explicit `globs` wins, else `<scope>/**`). Always-on rules (no globs, no scope, or `alwaysApply: true`) skip per-file emission and are reachable via the pointer body plus the source spec dir. The adapter manages no rule frontmatter keys: every `x-copilot` key lands alongside `applyTo`.
- **Agents**: native [custom agent profiles](https://docs.github.com/en/copilot/reference/custom-agents-configuration) at `.github/agents/<name>.agent.md`: frontmatter `name` + `description` plus `tools` and `model` when the spec declares them; arbitrary `x-copilot` keys (`target`, `user-invocable`, `mcp-servers`, ...) pass through. The body is the agent prompt. The old flattened `agent-<name>.instructions.md` copies are gone; the ledger sweeps them.
- **Skills**: native [Copilot skills](https://docs.github.com/en/copilot/how-tos/copilot-on-github/customize-copilot/customize-cloud-agent/add-skills) folders at `.github/skills/<name>/SKILL.md` (Copilot also scans `.claude/skills/` and `.agents/skills/`), with bundled sibling files propagated byte-for-byte. The old flattened `skill-<name>.instructions.md` copies are gone; the ledger sweeps them.
- **Chat modes**: when `outputs.copilot.chatmodes-dir` is set, each agent also emits as a [Copilot Custom Chat Mode](https://docs.github.com/en/copilot/customizing-copilot/adding-custom-instructions-for-github-copilot#about-custom-chat-modes) at `<dir>/<name>.chatmode.md` with `description`/`model`/`tools` frontmatter. The native agent profile still emits alongside.
- **Settings**: the last portable `model` value merges into `.github/copilot/settings.json`, where Copilot CLI documents it as the repository default model. Other repository settings survive. `import copilot` restores the model to `settings/imported.yaml` and leaves target-only keys in the native file.
- **MCP**: two files, because Copilot has two readers that disagree on which one to open.

  `.vscode/mcp.json` uses the VS Code schema: top-level `servers` key, each entry carrying a `type` (`stdio`, `http`, or `sse`). VS Code's Agent Host does not read that file itself: [the vendor doc](https://code.visualstudio.com/docs/agent-customization/mcp-servers) says it "doesn't read `.vscode/mcp.json` directly" and that VS Code forwards the config instead, except servers needing interactive input.

  `.github/mcp.json` carries the same servers under `mcpServers` for Copilot CLI, which its [project-level discovery table](https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-mcp-servers) lists as "Shared configuration that is committed to the repository". That second file exists because the CLI rejects the VS Code wrapper outright: "The `.vscode/mcp.json` file for VS Code is not read by Copilot CLI. It uses the unsupported top-level key `servers`." Before #646 only the VS Code file emitted, so a Copilot CLI user with no VS Code in the loop got no MCP server at all. Override either path with `outputs.copilot.mcp-file` / `outputs.copilot.cli-mcp-file`.

  VS Code MCP output owns the complete `servers` map and preserves unrelated top-level keys, including `inputs` and `sandbox`, at the default or configured path. JSONC is accepted; sync removes comments and normalizes formatting. Invalid JSONC aborts the write. `.github/mcp.json` and the opt-in root mirror remain managed as whole documents.

  VS Code alone accepts stdio `cwd`, `envFile` (for example `${workspaceFolder}/.env`), and `sandboxEnabled` (macOS and Linux only), remote `oauth: {clientId, enterpriseManaged}`, and `dev.watch` (a glob or glob array that restarts the server on change) on stdio, HTTP, and SSE servers. `dev.debug`, shaped `debug: {type: "node"|"debugpy", debugpyPath}`, is stdio-only; a remote server that sets it gets a coverage note while its watch patterns still emit. These fields stay out of the Copilot CLI file and root mirror. Every entry also accepts MCP `roots`, a list of `{uri, name}` objects. See [VS Code MCP configuration](https://code.visualstudio.com/docs/agents/reference/mcp-configuration).

  The same table's third entry, a `.mcp.json` anywhere from the working directory up to the repository root, stays opt-in behind `outputs.copilot.root-mcp-file: .mcp.json`. The project root is shared ground with Claude Code rather than Copilot's own directory, and a repository-root file is a surprise for a project that does not need it. Claude Code writes its own root `.mcp.json` under `mcpServers` too, so enabling both targets produces identical bytes at one path; the collision check compares content, not owners, so it stays quiet and sync writes the file once.
- **Hooks**: land in `.github/hooks/agnostic-ai.json` (override via `outputs.copilot.hooks-file`), one of possibly several `*.json` files Copilot loads and merges from that directory: "Repository-level hook files: `.github/hooks/*.json` in the repository root", read by both Copilot CLI and Copilot cloud agent ([docs.github.com/en/copilot/reference/hooks-reference](https://docs.github.com/en/copilot/reference/hooks-reference), "Hooks locations"). The wrapper is `{"version": 1, "hooks": {...}}` with an **integer** version. Each hook entry is a flat object carrying `matcher` directly (`{"type": "command", "matcher": ..., "command": ..., "timeoutSec": ...}`), not Claude Code's nested `{matcher, hooks: [...]}` group. 14 events exist today, one more than the 13 recorded when #629 was filed.

  `event:` passes through verbatim, same as every other hook emitter in this repo. The vendor documents both the PascalCase vocabulary Claude Code, Codex, OpenHands, Windsurf, and Qoder share (`PreToolUse`) and Copilot's own camelCase form (`preToolUse`) as independently valid keys in this same file, selecting between its "VS Code compatible" and "camelCase" hook payload formats respectively. The PascalCase form also carries Claude's own matcher semantics and tool names, so `matcher: Bash` reaches it unchanged. The camelCase form answers only to Copilot's own lowercase tool names (`bash`, `edit`, `view`, ...), and pairing it with a Claude-style matcher earns a coverage note rather than a guessed rename.

  Two of the 14 rows are camelCase-only: `userPromptTransformed` and `subagentStart` ("A subagent is spawned (before it runs)."). Neither has a PascalCase pairing anywhere on the page, so a spec spelling either one in PascalCase emits a key Copilot parses and never fires. `SubagentStart` is the one that bites, since Claude Code and Codex both accept it (target-audit 2026-09-11, #737).

  Command, HTTP (`url`, `headers`, `allowedEnvVars`), and `sessionStart` prompt handlers all survive `import copilot` and re-emission.

  A hook spec that sets `args` emits Copilot's own shell-free form instead: `{"type": "command", "exec": <command>, "args": [...]}`. The field table is explicit that the executable moves out of `command` (`exec` is accepted "Instead of `bash`, `powershell`, and `command`", and "Do not combine `exec` with `bash`, `powershell`, or `command`"), which is where this differs from Claude Code, whose exec form keeps the executable in `command`. It buys what `args` is for: a path or argument carrying a space, running as written.

  It costs one surface: `exec` and `args` are both marked "Only supported in Copilot CLI", and a cloud agent job reads the same `.github/hooks/*.json` files where "A subset of events fires, and only `bash` (or `command`) entries are honored." An exec-form entry is skipped whole there. `sync` raises a coverage note naming that trade. Leave `args` unset for a hook that must run under cloud agent (#755).

  **Every hook runs twice when you sync `claude` and `copilot` together.** The same "Hooks locations" section names both files: `.github/hooks/*.json` and, as a cross-tool read, `.claude/settings.json` and `.claude/settings.local.json`. Sources are "loaded ... and combined", and "When the same event appears in multiple sources, all hook entries from all sources are run."

  Both targets are in the default target list, so this is the out-of-the-box path: a formatter runs twice, an audit hook double-writes, and a blocking `preToolUse` returns two decisions per tool call. Copilot names no toggle for that read, unlike Cursor and Trae, which gate theirs behind an off-by-default switch. Until there is one, give the hook spec a single `target:` rather than both (#755).

Config keys:

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
| `.github/agents/<name>.agent.md` and `.github/chatmodes/<name>.chatmode.md` | `<agents>/<name>.md` |
| `.github/skills/<name>/` | `<skills>/<name>/` |
| `.github/hooks/*.json` | one target-scoped hook spec per handler |
| `.vscode/mcp.json` | `<mcps>/<name>.yaml` |
| `.github/copilot/settings.json` `model` | the portable Settings source |

Verify with the real extension:

1. Install the [GitHub Copilot extension](https://marketplace.visualstudio.com/items?itemName=GitHub.copilot) (and optional Copilot Chat) in VS Code.
2. Check the tree: `ls .github/copilot-instructions.md .github/instructions/ .vscode/mcp.json .github/mcp.json`, `grep "Generated by agnostic-ai" .github/instructions/*.md` for the provenance header (it sits after the `applyTo` frontmatter), `python -m json.tool .vscode/mcp.json > /dev/null && python -m json.tool .github/mcp.json > /dev/null`.
3. Open the project. Copilot loads `.github/copilot-instructions.md` plus every matching `.instructions.md`. The `Output → GitHub Copilot` channel shows no "failed to parse instructions" warnings.
4. Open a file matching a rule glob (e.g. a `.go` file for `applyTo: "**/*.go"`) and trigger chat; the rule body shows in context.
5. If MCPs are configured, run `MCP: Show Installed Servers`; each `servers.<name>` from `.vscode/mcp.json` is ready. In Copilot CLI, run `/mcp` from the project root after accepting the folder-trust prompt; each `mcpServers.<name>` from `.github/mcp.json` is listed.
6. If hooks are configured, `python -m json.tool .github/hooks/agnostic-ai.json > /dev/null` confirms the file parses. In Copilot CLI, a hook set on `PreToolUse` or `preToolUse` fires on the next matching tool call; the CLI's own hook log names which file and entry ran.

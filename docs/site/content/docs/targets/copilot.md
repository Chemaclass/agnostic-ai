+++
title = "GitHub Copilot"
description = "How agnostic-ai emits GitHub Copilot configuration: native paths, capability limits, and output options."
weight = 50

[extra]
group = "Reference"
target_id = "copilot"
+++

# GitHub Copilot (`copilot`)

GitHub Copilot reads most project configuration from `.github/`, and VS Code reads MCP servers from `.vscode/mcp.json`.

## Output

```
.github/copilot-instructions.md                            # canonical entry-point pointer body (written by sync)
.github/instructions/<name>.instructions.md                # scoped rule per file
.github/agents/<name>.agent.md                             # one custom-agent profile per agent
.github/skills/<name>/SKILL.md                             # one folder per skill, bundled assets included
.vscode/mcp.json                                           # when MCP entries exist; VS Code's file (deprecated in 1.140), servers key
.github/mcp.json                                           # when MCP entries exist; Copilot CLI's file, mcpServers key
.mcp.json                                                  # only with outputs.copilot.root-mcp-file; mcpServers key
.github/hooks/agnostic-ai.json                              # when hook entries exist
.github/hooks/scripts/agnostic-ai-portable-hook.sh          # when a portable before-tool hook exists
.github/copilot/settings.json                              # when a Settings model or a disabled MCP server exists
```
- **Rules**: a rule with `globs` or a source-layout scope (like `rules/backend/auth.md`) gets its own `.instructions.md` with `applyTo`.
  - Scope and patterns form a union: `scope: src/a` with `globs: tests/a/**` emits `applyTo: src/a/**,tests/a/**`.
  - An unscoped always-on rule (no globs, or `alwaysApply: true`) gets `applyTo: "**"`.
  - A rule with `alwaysApply: false` and neither gets no `applyTo`. VS Code loads it when its `description` matches the task ([custom instructions](https://code.visualstudio.com/docs/copilot/customization/custom-instructions)), and `import copilot` reads it back that way.
  - `description` and every `x-copilot` key land next to `applyTo`.
- **Agents**: [custom agent profiles](https://docs.github.com/en/copilot/reference/custom-agents-configuration) at `.github/agents/<name>.agent.md`. An existing `<name>.md` keeps its path, since VS Code reads any `.md` there and a second file would load twice.
  - Frontmatter carries `name`, `description`, and any set `tools` and `model`.
  - `x-copilot.name` sets a display name apart from the file name. Import fills it when the two differ.
  - Other `x-copilot` keys (`target`, `user-invocable`, `mcp-servers`, `effort`, ...) pass through.
  - The profile has no effort key, so a portable `effort` raises a coverage note. Per-agent `effortLevel` exists only in the user-tier `subagents.agents` setting of `~/.copilot/settings.json`, which `sync --global` writes (see [global output](@/docs/target-behavior.md#global-output)).
- **Skills**: [Copilot skills](https://docs.github.com/en/copilot/how-tos/copilot-on-github/customize-copilot/customize-cloud-agent/add-skills) at `.github/skills/<name>/SKILL.md`. Copilot also scans `.claude/skills/` and `.agents/skills/`, so a skill found there and not in `.github/skills/` is written in place. `outputs.copilot.skills-dir` sends every skill to one directory.
- **Old copies**: sync removes flattened `agent-<name>.instructions.md` and `skill-<name>.instructions.md` files.
- **Chat modes**: with `outputs.copilot.chatmodes-dir` set, each agent also emits as a VS Code [Custom Chat Mode](https://code.visualstudio.com/docs/copilot/customization/custom-chat-modes) at `<dir>/<name>.chatmode.md` with `description`/`model`/`tools` frontmatter.
- **Settings**: `.github/copilot/settings.json` keeps other keys and gets:
  - the last portable `model`, as Copilot CLI's repository default.
  - a portable `effort` of `low`, `medium`, `high`, or `xhigh`, as `effortLevel`. Other values raise a coverage note.
  - a settings spec's `x-copilot` block, for unmodeled keys such as `respectGitignore`.
  - `x-copilot.deniedUrls`, Copilot's blocked URL list. No portable rule reaches it. `allowedUrls` is user-tier only.
  - every MCP spec with `disabled: true`, sorted, in `disabledMcpServers`. It is Copilot's only file-based way to stop a configured server from starting ([CLI config reference](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-config-dir-reference)).
  - `import copilot` restores the model to `settings/copilot.yaml` and `effortLevel` as `effort` (or `x-copilot.effortLevel` when another settings spec sets a different effort). Target-only keys stay in the native file.

### MCP

VS Code and Copilot CLI read different files, so sync writes two. Override them with `outputs.copilot.mcp-file` and `outputs.copilot.cli-mcp-file`.

- `.vscode/mcp.json` uses the VS Code schema: top-level `servers`, each with a `type` (`stdio`, `http`, or `sse`). VS Code forwards it to the Agent Host, except servers that need interactive input ([VS Code MCP servers](https://code.visualstudio.com/docs/agent-customization/mcp-servers)). Sync owns `servers` and keeps other top-level keys, such as `inputs` and `sandbox`. Sync strips JSONC comments and normalizes formatting. Invalid JSONC aborts the write.
- `.github/mcp.json` carries the same servers under `mcpServers`, since Copilot CLI rejects the `servers` key ([Copilot CLI MCP docs](https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-mcp-servers)). Sync manages it and the root mirror as whole documents.
- A per-server `tools` allowlist (default `*`) reaches `.github/mcp.json` and the root mirror only. VS Code documents no such key, and Codex's `tools` differs.
- Only `.vscode/mcp.json` gets the VS Code fields ([VS Code MCP configuration](https://code.visualstudio.com/docs/agents/reference/mcp-configuration)): stdio `cwd`, `envFile` (for example `${workspaceFolder}/.env`), and `sandboxEnabled` (macOS and Linux only); remote `oauth: {clientId, enterpriseManaged}`; and `dev.watch` (a glob or glob array that restarts the server on change) on all transports. `dev.debug` (`debug: {type: "node"|"debugpy", debugpyPath}`) is stdio-only. On a remote server it gets a coverage note, and its watch patterns still emit.
- A `roots` list emits as passthrough. Neither VS Code nor GitHub documents it per server.
- VS Code 1.140 calls `.vscode/mcp.json` deprecated but still loads it, with no removal date. **MCP: Add Server** now saves workspace servers to a root `.mcp.json` under the portable `mcpServers` key ([VS Code MCP servers](https://code.visualstudio.com/docs/agent-customization/mcp-servers), [VS Code MCP configuration](https://code.visualstudio.com/docs/agents/reference/mcp-configuration), [1.140 release notes](https://code.visualstudio.com/updates/v1_140)). A project without the `claude` target can opt in:

    ```yaml
    outputs:
      copilot:
        root-mcp-file: .mcp.json
    ```

    Sync then also writes the servers there, with the `tools` allowlist and without the VS Code-only fields.

{% <details summary="Why root .mcp.json is opt-in"> %}
VS Code always reads a root `.mcp.json`, and in a trusted workspace starts its servers next to `.vscode/mcp.json`'s with no prompt. VS Code does not document which wins, or whether it dedupes, when both define the same server name.

The `claude` target writes `.mcp.json` too. A plain server is byte-identical from both, so sync writes it once. A field only one tool documents, such as Claude Code's `timeout` or Copilot CLI's `tools`, makes the copies differ, and sync stops with an output collision. VS Code does not document which copy wins ([design notes on #1533](https://github.com/Chemaclass/agnostic-ai/issues/1533#issuecomment-5919420911)).
{% </details> %}

### Hooks

Hooks go to `.github/hooks/agnostic-ai.json` (override with `outputs.copilot.hooks-file`). Copilot CLI and cloud agent load and merge every `.github/hooks/*.json` ([hooks reference](https://docs.github.com/en/copilot/reference/hooks-reference)). Copilot documents 14 events. With `builtins: [memory]`, a `sessionStart` hook adds the [shared memory](@/docs/memory.md) index to the session.

- The wrapper is `{"version": 1, "hooks": {...}}` with an **integer** version. Each entry is flat, `{"type": "command", "matcher": ..., "command": ..., "timeoutSec": ...}`, not Claude Code's nested `{matcher, hooks: [...]}` group.
- `event:` passes through. Copilot accepts PascalCase (`PreToolUse`, the "VS Code compatible" payload) and camelCase (`preToolUse`).
  - PascalCase uses Claude's matcher semantics and tool names, so `matcher: Bash` works unchanged.
  - camelCase full-matches the matcher as a regex against a per-event value: Copilot's lowercase tool names (`bash`, `edit`, `view`, ...) on `preToolUse`, `postToolUse`, and `permissionRequest`, the subagent's name on `subagentStart`, `notification_type` on `notification`, and `manual` or `auto` on `preCompact` ([matcher filtering](https://docs.github.com/en/copilot/reference/hooks-reference#matcher-filtering)).
  - Copilot skips a hook with an invalid regex matcher. `sync` notes that, a Claude tool name on a camelCase event, and a matcher on an event that documents none.
  - `userPromptTransformed` and `subagentStart` exist only in camelCase. A PascalCase `SubagentStart` (valid in Claude Code and Codex) never fires, and `validate` flags it.
- Command hooks also carry `cwd` (relative to the repository root, or absolute) and `env` (with variable expansion), set at the spec's top level or under `x-copilot`. Both are copilot-only and round-trip. With a relative `cwd`, sync writes the script path relative to that directory. The path sits in `command` (its first word, or the second after an interpreter such as `bash`) or in exec `exec` and `args`. So `.agnostic-ai/hooks/guard.sh` with `cwd: sub` becomes `../.agnostic-ai/hooks/guard.sh`, and import restores the repository-relative path. Absolute, `$`-prefixed, and non-path commands stay as written, as does every path when `cwd` is absolute or leaves the repository.
- Command, HTTP (`url`, `headers`, `allowedEnvVars`), and `sessionStart` prompt handlers round-trip through `import copilot`.
- A spec with `args` emits Copilot's shell-free form, `{"type": "command", "exec": <command>, "args": [...]}`, for paths or arguments with spaces. Copilot does not allow `exec` next to `command`, so the executable moves out of `command`. Claude Code's exec form keeps it there. Only Copilot CLI runs it. Cloud agent honors only `bash` or `command` entries, so `sync` raises a coverage note. Leave `args` unset for a hook that must run under cloud agent.

- A [portable](@/docs/spec-format/hooks.md#portable-events) `before-tool` hook lands on `PreToolUse` and runs through `.github/hooks/scripts/agnostic-ai-portable-hook.sh`. Exit 2 denies the call with stderr as `permissionDecisionReason`. Exit 1 still denies, so a broken guard keeps blocking, where Claude Code reports an error and goes on. A hook with [`decision: stdout`](@/docs/spec-format/hooks.md#decision-on-stdout) runs through the same wrapper. Import reads it back as the portable spec.

**Every hook runs twice when you sync `claude` and `copilot` together.** Copilot also reads `.claude/settings.json` and `.claude/settings.local.json` and runs every entry for an event. Both targets are defaults, so a formatter runs twice, an audit hook writes twice, and a blocking `preToolUse` returns two decisions. Unlike Cursor and Trae, Copilot has no toggle. Give the hook spec a single `target:`.

**The cross-read covers settings too.** Copilot CLI reads `companyAnnouncements`, `disableAllHooks`, `enabledPlugins`, `extraKnownMarketplaces`, and `hooks` from `.claude/settings.json` and `.claude/settings.local.json` ([CLI config reference](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-config-dir-reference)). agnostic-ai writes `hooks`, and `enabledPlugins` via `outputs.claude.settings.enabledPlugins`, so that key also sets Copilot CLI's repository plugin policy. Set it on purpose, or drop `claude` if the two tools need different plugins.

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
| `.github/copilot-instructions.md` | `.agnostic-ai/AGNOSTIC_AI.md`, whole; only a rules block `sync` wrote also becomes `<rules>/<name>.md` specs |
| `.github/instructions/<name>.instructions.md` | `<rules>/<name>.md`; the `agent-` and `skill-` filename prefixes become agents and skills |
| `.github/agents/<name>.agent.md` and `.github/chatmodes/<name>.chatmode.md` | `<agents>/<name>.md`, keeping the frontmatter keys copilot does not emit |
| `.github/skills/<name>/`, `.claude/skills/<name>/`, `.agents/skills/<name>/` | `<skills>/<name>/` |
| `.github/hooks/*.json` | one target-scoped hook spec per handler |
| `.vscode/mcp.json` | `<mcps>/<name>.yaml` |
| `.github/copilot/settings.json` `model` | the portable Settings source |

All three skill directories are [documented](https://docs.github.com/en/copilot/how-tos/copilot-on-github/customize-copilot/customize-cloud-agent/add-skills). On a name clash the emit path wins, then `.claude/skills`, then `.agents/skills`.

## Protected paths

Advisory. Copilot takes deny and ask rules only from device-level MDM settings, not from a repository file, so sync prints a coverage note for [protected paths](@/docs/spec-format/settings.md#protected-paths). State the paths in a rule if the agent should know about them.

## Verify

1. Install the [GitHub Copilot extension](https://marketplace.visualstudio.com/items?itemName=GitHub.copilot) (and optional Copilot Chat) in VS Code.
2. Check the tree: `ls .github/copilot-instructions.md .github/instructions/ .vscode/mcp.json .github/mcp.json`, `grep "Generated by agnostic-ai" .github/instructions/*.md` (the header sits after the `applyTo` frontmatter), `python -m json.tool .vscode/mcp.json > /dev/null && python -m json.tool .github/mcp.json > /dev/null`.
3. Open the project. Copilot loads `.github/copilot-instructions.md` plus every matching `.instructions.md`. The `Output → GitHub Copilot` channel shows no "failed to parse instructions" warnings.
4. Open a file matching a rule glob (for example a `.go` file for `applyTo: "**/*.go"`) and trigger chat; the rule body shows in context.
5. With MCPs, run `MCP: Show Installed Servers`; each `servers.<name>` from `.vscode/mcp.json` is ready. In Copilot CLI, accept the folder-trust prompt and run `/mcp` from the project root; each `mcpServers.<name>` from `.github/mcp.json` is listed.
6. With hooks, `python -m json.tool .github/hooks/agnostic-ai.json > /dev/null` parses. In Copilot CLI, a `PreToolUse` or `preToolUse` hook fires on the next matching tool call, and the hook log names the file and entry that ran.

+++
title = "GitHub Copilot"
description = "What agnostic-ai writes for GitHub Copilot: file paths, what Copilot supports, and config options."
weight = 50

[extra]
group = "Reference"
target_id = "copilot"
+++

# GitHub Copilot (`copilot`)

GitHub Copilot reads most project configuration from `.github/`, and VS Code reads MCP servers from `.vscode/mcp.json`.

## Output

```
.github/copilot-instructions.md                            # entry-point file with a short pointer body
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
- **Rules**: a rule with `globs` or a folder scope (like `rules/backend/auth.md`) gets its own `.instructions.md` file with `applyTo`.
  - Scope and patterns combine: `scope: src/a` with `globs: tests/a/**` writes `applyTo: src/a/**,tests/a/**`.
  - An always-on rule with no scope (no globs, or `alwaysApply: true`) gets `applyTo: "**"`.
  - A rule with `alwaysApply: false` and no scope or globs gets no `applyTo`. VS Code loads it when its `description` matches the task ([custom instructions](https://code.visualstudio.com/docs/copilot/customization/custom-instructions)). `import copilot` reads it back the same way.
  - `description` and every `x-copilot` key go next to `applyTo`.
- **Agents**: [custom agent profiles](https://docs.github.com/en/copilot/reference/custom-agents-configuration) at `.github/agents/<name>.agent.md`. An existing `<name>.md` keeps its path. VS Code reads any `.md` there, so a second file would load twice.
  - Frontmatter has `name`, `description`, and `tools` and `model` when set.
  - `x-copilot.name` sets a display name that differs from the file name. Import fills it when the two differ.
  - Other `x-copilot` keys (`target`, `user-invocable`, `mcp-servers`, `effort`, ...) are copied as written.
  - The profile has no effort key, so a portable `effort` gets a coverage note. Per-agent `effortLevel` exists only in the `subagents.agents` setting of your personal `~/.copilot/settings.json`, which `sync --global` writes (see [global output](@/docs/target-behavior.md#global-output)).
- **Skills**: [Copilot skills](https://docs.github.com/en/copilot/how-tos/copilot-on-github/customize-copilot/customize-cloud-agent/add-skills) at `.github/skills/<name>/SKILL.md`. Copilot also reads `.claude/skills/` and `.agents/skills/`. A skill found there and not in `.github/skills/` is written in place. `outputs.copilot.skills-dir` sends every skill to one directory.
- **Old copies**: sync removes old `agent-<name>.instructions.md` and `skill-<name>.instructions.md` files.
- **Chat modes**: with `outputs.copilot.chatmodes-dir` set, each agent is also written as a VS Code [Custom Chat Mode](https://code.visualstudio.com/docs/copilot/customization/custom-chat-modes) at `<dir>/<name>.chatmode.md` with `description`, `model`, and `tools` frontmatter.
- **Settings**: `.github/copilot/settings.json` keeps its other keys and gets:
  - the last portable `model`, as the Copilot CLI repository default.
  - a portable `effort` of `low`, `medium`, `high`, or `xhigh`, as `effortLevel`. Other values get a coverage note.
  - a settings spec's `x-copilot` block, for keys agnostic-ai has no field for, such as `respectGitignore`.
  - `x-copilot.deniedUrls`, Copilot's blocked URL list. No portable rule sets it. `allowedUrls` works only in your personal settings.
  - every MCP spec with `disabled: true`, sorted, in `disabledMcpServers`. It is the only file setting that stops a configured server from starting ([CLI config reference](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-config-dir-reference)).
  - `import copilot` restores the model to `settings/copilot.yaml` and `effortLevel` as `effort` (or `x-copilot.effortLevel` when another settings spec sets a different effort). Keys only Copilot uses stay in the native file.

### MCP

VS Code and Copilot CLI read different files, so sync writes both. Override them with `outputs.copilot.mcp-file` and `outputs.copilot.cli-mcp-file`.

- `.vscode/mcp.json` uses the VS Code schema: top-level `servers`, each with a `type` (`stdio`, `http`, or `sse`). VS Code passes it to the Agent Host, except servers that need interactive input ([VS Code MCP servers](https://code.visualstudio.com/docs/agent-customization/mcp-servers)). Sync owns `servers` and keeps other top-level keys, such as `inputs` and `sandbox`. It strips JSONC comments and reformats the file. Invalid JSONC stops the write.
- `.github/mcp.json` carries the same servers under `mcpServers`, since Copilot CLI rejects the `servers` key ([Copilot CLI MCP docs](https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-mcp-servers)). Sync owns the whole file, and the whole root copy.
- A per-server `tools` allowlist (default `*`) goes only to `.github/mcp.json` and the root copy. VS Code documents no such key, and Codex's `tools` means something else.
- A `roots` list is copied as written. Neither VS Code nor GitHub documents it per server.
- Only `.vscode/mcp.json` gets the VS Code fields ([VS Code MCP configuration](https://code.visualstudio.com/docs/agents/reference/mcp-configuration)):
  - stdio servers: `cwd`, `envFile` (for example `${workspaceFolder}/.env`), and `sandboxEnabled` (macOS and Linux only).
  - remote servers: `oauth: {clientId, enterpriseManaged}`.
  - all servers: `dev.watch`, a glob or list of globs that restarts the server on change.
  - stdio servers: `dev.debug` (`debug: {type: "node"|"debugpy", debugpyPath}`). On a remote server it gets a coverage note, and its watch patterns are still written.
- VS Code 1.140 calls `.vscode/mcp.json` deprecated but still loads it, with no removal date. **MCP: Add Server** now saves workspace servers to a root `.mcp.json` under the `mcpServers` key ([VS Code MCP servers](https://code.visualstudio.com/docs/agent-customization/mcp-servers), [VS Code MCP configuration](https://code.visualstudio.com/docs/agents/reference/mcp-configuration), [1.140 release notes](https://code.visualstudio.com/updates/v1_140)). A project without the `claude` tool can opt in:

    ```yaml
    outputs:
      copilot:
        root-mcp-file: .mcp.json
    ```

    Sync then also writes the servers there, with the `tools` allowlist and without the VS Code fields.

{% <details summary="Why root .mcp.json is opt-in"> %}
VS Code always reads a root `.mcp.json`. In a trusted workspace it starts those servers, along with the ones from `.vscode/mcp.json`, with no prompt. VS Code does not say which file wins when both define the same server name.

The `claude` target writes `.mcp.json` too. A plain server is identical from both, so sync writes it once. A field only one tool documents, such as Claude Code's `timeout` or Copilot CLI's `tools`, makes the copies differ, and sync stops with an output collision. VS Code does not say which copy wins ([design notes on #1533](https://github.com/Chemaclass/agnostic-ai/issues/1533#issuecomment-5919420911)).
{% </details> %}

### Hooks

Hooks go to `.github/hooks/agnostic-ai.json` (override with `outputs.copilot.hooks-file`). Copilot CLI and cloud agent load every `.github/hooks/*.json` and merge them ([hooks reference](https://docs.github.com/en/copilot/reference/hooks-reference)). Copilot documents 14 events. With `builtins: [memory, memory-hook]`, a `sessionStart` hook adds the [shared memory](@/docs/memory.md) index to the session.

- The wrapper is `{"version": 1, "hooks": {...}}` with an **integer** version. Each entry is flat, `{"type": "command", "matcher": ..., "command": ..., "timeoutSec": ...}`, not Claude Code's nested `{matcher, hooks: [...]}` group.
- `event:` is copied as written. Copilot accepts PascalCase (`PreToolUse`, the "VS Code compatible" format) and camelCase (`preToolUse`).
  - PascalCase matches the way Claude Code does and uses its tool names, so `matcher: Bash` works unchanged.
  - camelCase matches the whole value against the matcher as a regex. The value depends on the event: Copilot's lowercase tool names (`bash`, `edit`, `view`, ...) on `preToolUse`, `postToolUse`, and `permissionRequest`, the subagent's name on `subagentStart`, `notification_type` on `notification`, and `manual` or `auto` on `preCompact` ([matcher filtering](https://docs.github.com/en/copilot/reference/hooks-reference#matcher-filtering)).
  - Copilot skips a hook with an invalid regex matcher. `sync` reports that, a Claude tool name on a camelCase event, and a matcher on an event that documents none.
  - `userPromptTransformed` and `subagentStart` exist only in camelCase. A PascalCase `SubagentStart` (valid in Claude Code and Codex) never fires, and `validate` flags it.
- Command hooks also take `cwd` (relative to the repository root, or absolute) and `env` (with variable expansion), set at the spec's top level or under `x-copilot`. Both are Copilot-only and survive import.
  - With a relative `cwd`, sync writes the script path relative to that directory. So `.agnostic-ai/hooks/guard.sh` with `cwd: sub` becomes `../.agnostic-ai/hooks/guard.sh`, and import restores the repository-relative path.
  - The path is the first word of `command`, or the second after an interpreter such as `bash`, or it is in `exec` and `args`.
  - Absolute paths, `$`-prefixed paths, and commands that are not paths stay as written. So does every path when `cwd` is absolute or outside the repository.
- `import copilot` reads back command handlers, HTTP handlers (`url`, `headers`, `allowedEnvVars`), and `sessionStart` prompt handlers.
- A spec with `args` is written in Copilot's form without a shell, `{"type": "command", "exec": <command>, "args": [...]}`, for paths or arguments with spaces. Copilot does not allow `exec` next to `command`, so the program moves out of `command`. Claude Code's form keeps it there. Only Copilot CLI runs this form. Cloud agent runs only `bash` or `command` entries, so `sync` adds a coverage note. Leave `args` unset for a hook that must run under cloud agent.

- A [portable](@/docs/spec-format/hooks.md#portable-events) `before-tool` hook lands on `PreToolUse` and runs through `.github/hooks/scripts/agnostic-ai-portable-hook.sh`. Exit 2 denies the call and uses stderr as `permissionDecisionReason`. Exit 1 also denies, so a broken guard keeps blocking. Claude Code reports an error and goes on instead. A hook with [`decision: stdout`](@/docs/spec-format/hooks.md#decision-on-stdout) runs through the same wrapper. Import reads it back as the portable spec.

**Every hook runs twice when you sync `claude` and `copilot` together.** Copilot also reads `.claude/settings.json` and `.claude/settings.local.json` and runs every entry for an event. Both are default targets, so a formatter runs twice, an audit hook writes twice, and a blocking `preToolUse` returns two decisions. Unlike Cursor and Trae, Copilot has no switch to stop this. Give the hook spec a single `target:`.

**Copilot reads Claude settings too.** Copilot CLI reads `companyAnnouncements`, `disableAllHooks`, `enabledPlugins`, `extraKnownMarketplaces`, and `hooks` from `.claude/settings.json` and `.claude/settings.local.json` ([CLI config reference](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-config-dir-reference)). agnostic-ai writes `hooks`, and `enabledPlugins` via `outputs.claude.settings.enabledPlugins`, so that key also sets the Copilot CLI plugin policy for the repository. Set it on purpose, or drop `claude` if the two tools need different plugins.

## Config keys

| Key | Default | Notes |
|---|---|---|
| `outputs.copilot.instructions-dir` | `.github/instructions` | |
| `outputs.copilot.agents-dir` | `.github/agents` | |
| `outputs.copilot.skills-dir` | `.github/skills` | |
| `outputs.copilot.mcp-file` | `.vscode/mcp.json` | |
| `outputs.copilot.cli-mcp-file` | `.github/mcp.json` | Copilot CLI's own file, `mcpServers` key |
| `outputs.copilot.root-mcp-file` | unset, opt-in | writes the same servers to `.mcp.json` in the workspace root, under the `mcpServers` key |
| `outputs.copilot.chatmodes-dir` | empty, opt-in | writes one Custom Chat Mode per agent |
| `outputs.copilot.rules-file` | unset | writes all always-on rules into that one file and skips the pointer body |
| `outputs.copilot.hooks-file` | `.github/hooks/agnostic-ai.json` | |

## Import

`agnostic-ai import copilot` reads:

| Source | Becomes |
|---|---|
| `.github/copilot-instructions.md` | `.agnostic-ai/AGNOSTIC_AI.md`, whole. Only a rules block that `sync` wrote also becomes `<rules>/<name>.md` specs |
| `.github/instructions/<name>.instructions.md` | `<rules>/<name>.md`. Files with an `agent-` or `skill-` prefix become agents and skills |
| `.github/agents/<name>.agent.md` and `.github/chatmodes/<name>.chatmode.md` | `<agents>/<name>.md`, keeping the frontmatter keys Copilot output does not use |
| `.github/skills/<name>/`, `.claude/skills/<name>/`, `.agents/skills/<name>/` | `<skills>/<name>/` |
| `.github/hooks/*.json` | one target-scoped hook spec per handler |
| `.vscode/mcp.json` | `<mcps>/<name>.yaml` |
| `.github/copilot/settings.json` `model` | the portable Settings source |

All three skill directories are [documented](https://docs.github.com/en/copilot/how-tos/copilot-on-github/customize-copilot/customize-cloud-agent/add-skills). On a name clash, `.github/skills` wins, then `.claude/skills`, then `.agents/skills`.

## Protected paths

Advisory. Copilot takes deny and ask rules only from device-level MDM settings, not from a repository file, so sync prints a coverage note for [protected paths](@/docs/spec-format/settings.md#protected-paths). State the paths in a rule if the agent should know about them.

## Verify

1. Install the [GitHub Copilot extension](https://marketplace.visualstudio.com/items?itemName=GitHub.copilot) (and optional Copilot Chat) in VS Code.
2. Check the tree: `ls .github/copilot-instructions.md .github/instructions/ .vscode/mcp.json .github/mcp.json`, `grep "Generated by agnostic-ai" .github/instructions/*.md` (the header sits after the `applyTo` frontmatter), `python -m json.tool .vscode/mcp.json > /dev/null && python -m json.tool .github/mcp.json > /dev/null`.
3. Open the project. Copilot loads `.github/copilot-instructions.md` plus every matching `.instructions.md`. The `Output → GitHub Copilot` channel shows no "failed to parse instructions" warnings.
4. Open a file matching a rule glob (for example a `.go` file for `applyTo: "**/*.go"`) and trigger chat; the rule body shows in context.
5. With MCPs, run `MCP: Show Installed Servers`; each `servers.<name>` from `.vscode/mcp.json` is ready. In Copilot CLI, accept the folder-trust prompt and run `/mcp` from the project root; each `mcpServers.<name>` from `.github/mcp.json` is listed.
6. With hooks, `python -m json.tool .github/hooks/agnostic-ai.json > /dev/null` parses. In Copilot CLI, a `PreToolUse` or `preToolUse` hook fires on the next matching tool call, and the hook log names the file and entry that ran.

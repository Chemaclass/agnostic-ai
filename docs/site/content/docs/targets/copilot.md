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
.vscode/mcp.json                                           # when MCP entries exist; VS Code's file (deprecated in 1.140), servers key
.github/mcp.json                                           # when MCP entries exist; Copilot CLI's file, mcpServers key
.mcp.json                                                  # only with outputs.copilot.root-mcp-file; mcpServers key
.github/hooks/agnostic-ai.json                              # when hook entries exist
.github/copilot/settings.json                              # when a Settings model or a disabled MCP server exists
```

- **Rules**: a rule with `globs` or a source-layout scope (like `rules/backend/auth.md`) emits as its own `.instructions.md` with `applyTo` frontmatter. Scope and patterns form a union, so `scope: src/a` with `globs: tests/a/**` emits `applyTo: src/a/**,tests/a/**`. An unscoped always-on rule (no globs, or `alwaysApply: true`) gets `applyTo: "**"`. A rule with `alwaysApply: false` and neither gets no `applyTo`, so VS Code loads it on demand when its `description` matches the task ([custom instructions](https://code.visualstudio.com/docs/copilot/customization/custom-instructions)); `import copilot` reads such a file back that way. `description` is written to the frontmatter, where VS Code reads it. Every `x-copilot` key lands next to `applyTo`.
- **Agents**: [custom agent profiles](https://docs.github.com/en/copilot/reference/custom-agents-configuration) at `.github/agents/<name>.agent.md`, or at `<name>.md` when the agent already lives there, since VS Code reads any `.md` in that folder and a second file would load it twice. The frontmatter carries `name` and `description`, plus `tools` and `model` when set; `x-copilot.name` sets a display name apart from the file name, which `import copilot` fills when a profile's `name` differs from its file. The body is the agent prompt. Arbitrary `x-copilot` keys (`target`, `user-invocable`, `mcp-servers`, `effort`, ...) pass through.
  - The profile has no effort key, so a portable `effort` raises a coverage note. Per-agent `effortLevel` exists only in the user-tier `subagents.agents` setting of `~/.copilot/settings.json`, which `sync --global` writes (see [global output](@/docs/target-behavior.md#global-output)).
  - Old flattened `agent-<name>.instructions.md` copies are swept.
- **Skills**: [Copilot skills](https://docs.github.com/en/copilot/how-tos/copilot-on-github/customize-copilot/customize-cloud-agent/add-skills) at `.github/skills/<name>/SKILL.md`, with bundled files copied byte-for-byte. Copilot also scans `.claude/skills/` and `.agents/skills/`, so a skill that already lives in one of them, and not in `.github/skills/`, is written where it lives instead of copied; `outputs.copilot.skills-dir` sends every skill to one directory. Old flattened `skill-<name>.instructions.md` copies are swept.
- **Chat modes**: with `outputs.copilot.chatmodes-dir` set, each agent also emits as a VS Code [Custom Chat Mode](https://code.visualstudio.com/docs/copilot/customization/custom-chat-modes) at `<dir>/<name>.chatmode.md` with `description`/`model`/`tools` frontmatter. The agent profile still emits.
- **Settings**: the last portable `model` merges into `.github/copilot/settings.json` as Copilot CLI's repository default model. A portable `effort` of `low`, `medium`, `high`, or `xhigh` merges as `effortLevel`; another value raises a coverage note. Other repository settings survive.
  - `import copilot` restores the model to `settings/copilot.yaml` and `effortLevel` as `effort` (or `x-copilot.effortLevel` when another settings spec sets a different effort). Target-only keys stay in the native file.
  - An `x-copilot` block on a settings spec merges into the same file, for keys this tool does not model, such as `respectGitignore`.
  - `x-copilot.deniedUrls` writes Copilot's blocked URL list. No portable rule reaches it, and no allow list can: `allowedUrls` is user-tier only.
  - Every MCP spec with `disabled: true` is written to `disabledMcpServers` in the same file, sorted. It is Copilot's only file-based way to keep a configured server from starting ([CLI config reference](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-config-dir-reference)).
- **MCP**: two files, because VS Code and Copilot CLI read different ones.
  - `.vscode/mcp.json` uses the VS Code schema: top-level `servers`, each entry with a `type` (`stdio`, `http`, or `sse`). VS Code's Agent Host does not read the file itself; VS Code forwards the config, except servers that need interactive input ([VS Code MCP servers](https://code.visualstudio.com/docs/agent-customization/mcp-servers)).
  - `.github/mcp.json` carries the same servers under `mcpServers` for Copilot CLI, which rejects the VS Code `servers` key ([Copilot CLI MCP docs](https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-mcp-servers)). Override the paths with `outputs.copilot.mcp-file` and `outputs.copilot.cli-mcp-file`.
  - `.vscode/mcp.json` output owns the whole `servers` map and keeps other top-level keys, such as `inputs` and `sandbox`. JSONC is accepted; sync strips comments and normalizes formatting, and invalid JSONC aborts the write. `.github/mcp.json` and the root mirror are managed as whole documents.
  - A per-server `tools` allowlist (default `*`) reaches `.github/mcp.json` and the root mirror only. VS Code documents no such key, and Codex's `tools` has a different shape.
  - Only `.vscode/mcp.json` gets the VS Code fields: stdio `cwd`, `envFile` (for example `${workspaceFolder}/.env`), and `sandboxEnabled` (macOS and Linux only), remote `oauth: {clientId, enterpriseManaged}`, and `dev.watch` (a glob or glob array that restarts the server on change) on all transports. `dev.debug` (`debug: {type: "node"|"debugpy", debugpyPath}`) is stdio-only; a remote server that sets it gets a coverage note, and its watch patterns still emit. See [VS Code MCP configuration](https://code.visualstudio.com/docs/agents/reference/mcp-configuration).
  - A `roots` list still emits, but neither VS Code nor GitHub documents it as a per-server key, so treat it as passthrough.
  - VS Code 1.140 calls `.vscode/mcp.json` deprecated. It still loads the file and forwards its servers to the Agent Host, and no removal date is announced. **MCP: Add Server** now saves workspace servers to a root `.mcp.json`, which uses the portable `mcpServers` key instead of `servers` ([VS Code MCP servers](https://code.visualstudio.com/docs/agent-customization/mcp-servers), [VS Code MCP configuration](https://code.visualstudio.com/docs/agents/reference/mcp-configuration), [1.140 release notes](https://code.visualstudio.com/updates/v1_140)).
  - A project that does not target `claude` can add the root file:

    ```yaml
    outputs:
      copilot:
        root-mcp-file: .mcp.json
    ```

    Sync then writes the same servers to `.mcp.json` under `mcpServers`, with the `tools` allowlist and without the VS Code-only fields, and keeps writing `.vscode/mcp.json`. VS Code reads a root `.mcp.json` whether or not this is on, and in a trusted workspace starts its servers next to `.vscode/mcp.json`'s with no extra prompt. VS Code does not document which file wins, or whether it dedupes, when both define a server with the same name.
  - The root file is not the default because the `claude` target writes `.mcp.json` too. A plain server comes out byte-identical from both targets, so sync writes it once. A field that only one tool documents, such as Claude Code's `timeout` or Copilot CLI's `tools`, makes the two copies differ, and sync stops with an output collision. The default will be revisited once VS Code documents precedence between the two files ([design notes on #1533](https://github.com/Chemaclass/agnostic-ai/issues/1533#issuecomment-5919420911)).
- **Hooks**: written to `.github/hooks/agnostic-ai.json` (override via `outputs.copilot.hooks-file`). Copilot CLI and cloud agent load and merge every `.github/hooks/*.json` ([hooks reference](https://docs.github.com/en/copilot/reference/hooks-reference)). The wrapper is `{"version": 1, "hooks": {...}}` with an **integer** version. Each entry is flat, `{"type": "command", "matcher": ..., "command": ..., "timeoutSec": ...}`, not Claude Code's nested `{matcher, hooks: [...]}` group. Copilot documents 14 events.
  - `event:` passes through verbatim. Copilot accepts both PascalCase (`PreToolUse`, the "VS Code compatible" payload) and camelCase (`preToolUse`). PascalCase uses Claude's matcher semantics and tool names, so `matcher: Bash` works unchanged. camelCase tests the matcher as a full-match regex against a different value per event: Copilot's lowercase tool names (`bash`, `edit`, `view`, ...) on `preToolUse`, `postToolUse`, and `permissionRequest`, the subagent's name on `subagentStart`, `notification_type` on `notification`, and `manual` or `auto` on `preCompact` ([matcher filtering](https://docs.github.com/en/copilot/reference/hooks-reference#matcher-filtering)). Copilot skips a hook whose matcher is not a valid regex. `sync` raises a coverage note for a Claude tool name on a camelCase event, a matcher on an event that documents none, and an invalid regex.
  - `userPromptTransformed` and `subagentStart` exist only in camelCase. A PascalCase spelling, such as `SubagentStart` (valid in Claude Code and Codex), never fires, and `validate` flags it.
  - Command hooks also carry `cwd` (relative to the repository root, or absolute) and `env` (with variable expansion). Set them at the spec's top level or under `x-copilot`. Both are copilot-only and round-trip through `import copilot`.
  - Command, HTTP (`url`, `headers`, `allowedEnvVars`), and `sessionStart` prompt handlers all survive `import copilot` and re-emission.
  - A hook spec with `args` emits Copilot's shell-free form, `{"type": "command", "exec": <command>, "args": [...]}`. Copilot does not allow `exec` next to `command`, so the executable moves out of `command`, unlike Claude Code's exec form. Use it for paths or arguments with spaces.
  - `exec` and `args` work only in Copilot CLI. Cloud agent honors only `bash` or `command` entries and skips an exec-form entry whole, so `sync` raises a coverage note. Leave `args` unset for a hook that must run under cloud agent.

  **Every hook runs twice when you sync `claude` and `copilot` together.** Copilot also reads `.claude/settings.json` and `.claude/settings.local.json`, combines all sources, and runs every entry for an event. Both targets are in the default target list, so this happens out of the box: a formatter runs twice, an audit hook writes twice, and a blocking `preToolUse` returns two decisions. Copilot has no toggle for this, unlike Cursor and Trae. Give the hook spec a single `target:` instead of both.

  **The cross-read covers settings too.** Copilot CLI reads a shared subset of `.claude/settings.json` and `.claude/settings.local.json`: `companyAnnouncements`, `disableAllHooks`, `enabledPlugins`, `extraKnownMarketplaces`, and `hooks` ([CLI config reference](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-config-dir-reference)). agnostic-ai writes two of them: `hooks`, and `enabledPlugins` via `outputs.claude.settings.enabledPlugins`. So that key also sets Copilot CLI's repository plugin policy. Set it deliberately, or drop `claude` from the targets if the two tools should differ.

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

All three skill directories are [documented project locations](https://docs.github.com/en/copilot/how-tos/copilot-on-github/customize-copilot/customize-cloud-agent/add-skills). On a same-name collision the emit path wins, then `.claude/skills`, then `.agents/skills`.

## Protected paths

Advisory. Copilot takes deny and ask rules only from device-level MDM settings, not from a repository file, so sync prints a coverage note for [protected paths](@/docs/spec-format/settings.md#protected-paths). State the paths in a rule if the agent should know about them.

## Verify

1. Install the [GitHub Copilot extension](https://marketplace.visualstudio.com/items?itemName=GitHub.copilot) (and optional Copilot Chat) in VS Code.
2. Check the tree: `ls .github/copilot-instructions.md .github/instructions/ .vscode/mcp.json .github/mcp.json`, `grep "Generated by agnostic-ai" .github/instructions/*.md` (the header sits after the `applyTo` frontmatter), `python -m json.tool .vscode/mcp.json > /dev/null && python -m json.tool .github/mcp.json > /dev/null`.
3. Open the project. Copilot loads `.github/copilot-instructions.md` plus every matching `.instructions.md`. The `Output → GitHub Copilot` channel shows no "failed to parse instructions" warnings.
4. Open a file matching a rule glob (e.g. a `.go` file for `applyTo: "**/*.go"`) and trigger chat; the rule body shows in context.
5. With MCPs, run `MCP: Show Installed Servers`; each `servers.<name>` from `.vscode/mcp.json` is ready. In Copilot CLI, accept the folder-trust prompt and run `/mcp` from the project root; each `mcpServers.<name>` from `.github/mcp.json` is listed.
6. With hooks, `python -m json.tool .github/hooks/agnostic-ai.json > /dev/null` parses. In Copilot CLI, a `PreToolUse` or `preToolUse` hook fires on the next matching tool call, and the hook log names the file and entry that ran.

+++
title = "Cursor"
description = "How agnostic-ai writes Cursor configuration: file paths, what Cursor cannot hold, and output options."
weight = 40

[extra]
group = "Reference"
target_id = "cursor"
+++

# Cursor (`cursor`)

Cursor reads its project configuration from `.cursor/`, including `.cursor/rules/`.

## Output

```
.cursor/rules/<name>.mdc
.cursor/agents/<name>.md             # one native subagent per agent spec
.cursor/skills/<name>/SKILL.md       # one folder per skill, bundled assets included
.cursor/commands/<name>.md           # one per command spec
.cursor/hooks.json                   # when hook specs exist (sync updates its entries, keeps yours)
.cursor/hooks/agnostic-ai-portable-hook.sh  # when a portable before-tool hook exists
.cursor/mcp.json                     # when MCP entries exist
.cursor/cli.json                     # from settings permissions and deny protected paths (merged)
```
- **Rules**: `alwaysApply: true`, or `false` when the spec sets `globs` without `alwaysApply`.
  - A catch-all such as `**/*` stays always-on (override in frontmatter). An always-apply rule omits `globs`.
  - A non-always rule without `globs` falls back to the `paths` list (comma-joined).
  - With neither, `globs` is omitted, so Cursor treats the rule as "Apply Intelligently" (description-driven) or "Apply Manually".
- **Per-file check**: `agnostic-ai explain --file <path> --target cursor` checks each planned `.mdc` rule and every `AGENTS.md` Cursor reads against one file, using the [Rules](https://cursor.com/docs/rules) matrix.
- **Agents**: [Cursor subagents](https://cursor.com/docs/subagents.md) (Cursor 2.4+). Frontmatter has `name`, `description`, and any declared `model`, `readonly`, and `is_background`. The body is the system prompt.
  - Cursor has no `tools` field, so sync drops a `tools` list with a coverage note. Use `readonly: true` instead.
  - Cursor has no effort field, so a portable `effort` raises a coverage note. Put it in the model id: `model: {cursor: "claude-opus-5[effort=high]"}`. See [per-target `model` and `effort`](@/docs/spec-format/agents.md#per-target-model-and-effort).
- **Commands**: Markdown files under `.cursor/commands/`, body as the prompt. Cursor no longer documents this path; see its [migrate commands to skills](https://cursor.com/help/customization/skills.md) FAQ.
- **Old copies**: sync removes older flattened `.mdc`, agent-as-command, and `skill-<name>.mdc` files.
- **Skills**: `.cursor/skills/<name>/SKILL.md`, the [Agent Skills](https://cursor.com/docs/skills.md) layout (Cursor 2.4+), bundled files copied byte-for-byte.
  - Frontmatter has `name`, `description`, and any declared `paths`, `disable-model-invocation`, `icon`, `color`, and `metadata`. `icon` and `color` style the badge when the skill backs a [Custom Mode](https://cursor.com/docs/agent/prompting.md#custom-modes).
  - A source-layout scope moves the tree under that directory.
  - Cursor loads skills only from the workspace it opens, so a skill's `workspaces` list adds a `<dir>/.cursor/skills/<name>/` copy per directory, beside the root copy.
  - Skill `model` and `effort` are omitted with a coverage note when a value resolves for Cursor.
- **Review**: [Bugbot](https://cursor.com/docs/bugbot) files, root `.cursor/BUGBOT.md` for unscoped specs and `<scope>/.cursor/BUGBOT.md` for scoped ones.
  - Same-scope specs are joined into one file.
  - [Rule limits](https://cursor.com/docs/bugbot#rule-limits): Bugbot truncates each BUGBOT.md at 30,000 characters and caps one review's rules at 100,000 in total.
  - agnostic-ai never truncates. An over-cap file is written in full, with a coverage note. The size counts the whole file, header included, so several small specs joined can pass the cap. Split the review across sibling scopes.
  - `sync` also notes a scope whose chain (the scope and its parent directories) passes 100,000 characters. Team and repository rules share that budget, so a shorter chain can still lose rules. To see what Bugbot dropped, run `bugbot run verbose=true` or `cursor review verbose=true` on a pull request.
- **Environment**: `.cursor/environment.json` ([background-agent](https://docs.cursor.com/background-agent) bootstrap).
  - Spec keys pass through as written, except routing fields and `cleanup`, which gets a no-effect note. Specs merge by top-level key.
  - `setup` and `setup-windows` go to `.cursor/worktrees.json` as `setup-worktree` (every OS) and `setup-worktree-windows` (Windows only) ([worktrees](https://cursor.com/docs/configuration/worktrees)).
  - A script path, which Cursor resolves from `.cursor/`, or a unix-only `setup-worktree-unix`, goes under `x-cursor:` with its native key, such as `setup-worktree-unix: setup.sh`.
- **Ignore**: `.cursorignore` (gitignore syntax). Specs are joined into one file.
- **Hooks**: [Cursor Hooks](https://cursor.com/docs/hooks) in a managed `.cursor/hooks.json` (`version` + per-event arrays). With `builtins: [memory]`, a `sessionStart` hook adds the [shared memory](@/docs/memory.md) index to the session.
  - A `type: prompt` hook writes `prompt` and optional `model`. Command and prompt hooks keep `timeout`, `loop_limit` (including `null`), `failClosed`, and `matcher`.
  - camelCase event names (`beforeShellExecution`, `afterFileEdit`, ...) pass through. `validate` flags unknown ones.
  - Fifteen events take a `matcher`: `preToolUse`, `postToolUse`, `postToolUseFailure` (tool name), `subagentStart`, `subagentStop` (subagent type), `beforeShellExecution`, `afterShellExecution` (full command string), `beforeReadFile`, `afterFileEdit` (tool name), and `beforeTabFileRead`, `afterTabFileEdit`, `beforeSubmitPrompt`, `stop`, `afterAgentResponse`, `afterAgentThought` (one fixed value each). `lint` flags a matcher on any other event, except `beforeMCPExecution` and `afterMCPExecution`.
  - When a command hook exists, sync adds a `sessionStart` hook that prints `{"env":{"AGNOSTIC_AI_TARGET":"cursor"}}`. Cursor passes that `env` to every later hook in the session, including loaded Claude Code hooks, and on Windows too. `sessionStart` hooks, and any hook that fires before this one returns, do not see the variable. See [which target ran a hook](@/docs/spec-format/hooks.md#hook-target).
  - A hook has only `command`, run through `$SHELL -c` (PowerShell on Windows). A spec's `args` fold into `command`, each in single quotes (`node 'guard.js'`).
  - A [portable](@/docs/spec-format/hooks.md#portable-events) `before-tool` hook lands on `preToolUse` and a `prompt-submit` hook on `beforeSubmitPrompt`, each through `.cursor/hooks/agnostic-ai-portable-hook.sh`. Exit 2 becomes a deny reply with stderr as the message, and exit 0 an allow reply. `session-start` and `session-end` land on `sessionStart` and `sessionEnd`. `sync --global` writes the wrapper to `~/.cursor/hooks/`.
- **MCP**: `.cursor/mcp.json` under `mcpServers`. Each sync replaces the whole file from MCP specs. Cursor-only fields ([cursor.com/docs/mcp](https://cursor.com/docs/mcp.md)):
  - A stdio server accepts `envFile`, a path to an env file with extra variables.
  - A `${NAME}` reference in `env`, `headers`, `url`, or `args` is written as Cursor's `${env:NAME}`; `${workspaceFolder}` and Cursor's other variables stay as written. See [environment references](@/docs/spec-format/mcps.md#environment-references).
  - A remote (`url`) server accepts a static-OAuth `auth`, `{CLIENT_ID, CLIENT_SECRET, scopes}` with `CLIENT_ID` required, for providers without OAuth Dynamic Client Registration.
  - A stdio server carries an explicit `"type": "stdio"`, which Cursor requires.
  - A `roots` list passes through, though Cursor documents no per-server `roots` key.
  - `disabled: true` has no effect here; see [`disabled` support by target](@/docs/spec-format/mcps.md#disabled-support-by-target).

{% <details summary="Hook args with apostrophes or spaces"> %}
PowerShell reads that quoting the same way unless an arg has an apostrophe. A command path with a space is quoted too, and PowerShell does not run it. Use a shell-form `command` in those cases.
{% </details> %}

### Third-party reads

The **Include Third-Party Plugins, Skills, and Other Configs** setting (Cursor Settings → Agents → Third-Party Imports, [on by default](https://cursor.com/docs/reference/third-party-hooks.md)) turns on Claude and Codex compatibility reads. Turn it off to stop those reads, or give the spec a single `target:`. Cursor reads `.agents/skills/` (written by `codex`) either way.

- **Skills** ([docs](https://cursor.com/docs/skills.md)): Native project roots are `.cursor/skills/` and `.agents/skills/`. Compatibility roots include `.claude/skills/`, `.codex/skills/`, and their home equivalents. Cursor does not document which root wins on a duplicate.
- **Subagents** ([docs](https://cursor.com/docs/subagents.md)): Cursor reads `.claude/agents/` and `.codex/agents/` (current project only) beside `.cursor/agents/`. On a name clash, `.cursor/` wins. Cursor documents only Markdown agents, so it may not parse the `codex` target's TOML files.
- **Hooks** ([docs](https://cursor.com/docs/reference/third-party-hooks.md)): Cursor merges seven hook sources, including `.cursor/hooks.json` and `.claude/settings.json`, runs every matching hook, and maps Claude event names to Cursor ones. A repo syncing `claude` and `cursor` runs every hook twice, except a portable hook, whose Claude Code copy exits under Cursor ([details](@/docs/spec-format/hooks.md#claude-settings-copies)). The setting is the only way to stop the rest.
- **Hook `args`**: Cursor's third-party hooks docs do not list `args`, so a claude hook spec with `args` may run as a bare interpreter that reads the JSON event data as its program. `sync` notes this when `claude` runs and `cursor` is configured. Use a shell-form `command`, or turn off the setting.

## Config keys

| Key | Default |
|---|---|
| `outputs.cursor.rules-dir` | `.cursor/rules` |
| `outputs.cursor.agents-dir` | `.cursor/agents` |
| `outputs.cursor.skills-dir` | `.cursor/skills` |
| `outputs.cursor.commands-dir` | `.cursor/commands` |
| `outputs.cursor.mcp-file` | `.cursor/mcp.json` |
| `outputs.cursor.review-file` | `BUGBOT.md` |
| `outputs.cursor.environment-file` | `.cursor/environment.json` |
| `outputs.cursor.ignore-file` | `.cursorignore` |
| `outputs.cursor.hooks-file` | `.cursor/hooks.json` |

## Permissions

A settings spec's portable `allow` and `deny` lists become Cursor CLI rules in `permissions.allow` and `permissions.deny` of `.cursor/cli.json`, beside the protected-path rules. Sync writes only spellings the [CLI permissions](https://cursor.com/docs/cli/reference/permissions) page documents, and never widens a rule. The rules guard the Cursor CLI only, not the IDE agent.

| Portable rule | Cursor rule | Notes |
|---|---|---|
| `Bash(git:*)`, `Bash(git *)` | `Shell(git)` | One command word only. `Shell(git)` covers bare `git` too. |
| `Read(<path>)` | `Read(<path>)` | `/path` loses its `/`, since Cursor scopes a relative path to the workspace. `//path` becomes the absolute `/path`. On `deny`, a path with no `*` or `?` also gets `Read(<path>/**)`. |
| `Edit(<path>)`, `Write(<path>)` | `Write(<path>)` | Same path rules as `Read`. |
| `WebFetch(domain:<host>)` | `WebFetch(<host>)` | An exact host, `*.host`, or `*`. |
| `mcp__<server>__<tool>` | `Mcp(<server>:<tool>)` | `mcp__<server>` becomes `Mcp(<server>:*)`. On `deny` only, `mcp__*` becomes `Mcp(*:*)` and `mcp__*__<tool>` becomes `Mcp(*:<tool>)`. |

These stay out:

- A multi-word command such as `Bash(go test:*)` or `Bash(git push:*)`. `Shell` takes one command word, and `Shell(go)` would cover every `go` command.
- An exact command such as `Bash(ls)` or `Bash(rm)`, since `Shell(rm)` also matches `rm` with arguments.
- A path starting with `~` or `!`, or using `[]`, `{}`, or `\`.
- A `WebFetch` host with a path, a port, or a wildcard other than a leading `*.`.
- `ask` rules. The CLI has no ask list and already prompts before a call no `allow` rule covers.
- Other tools, such as `WebSearch`, `Grep`, or a bare `Bash`.

A dropped `allow` rule raises a coverage note. A dropped `deny` rule loosens the policy, so sync names each one: `deny rule not enforced on cursor: Bash(git push:*)`. Block those with a [Cursor hook](https://cursor.com/docs/hooks) on `beforeShellExecution`.

Cursor does not say how `Shell` matches a chained command such as `git status && rm -rf build`, or a pipe. Claude Code checks each subcommand of a chain, so an `allow` rule may approve more in the Cursor CLI.

A relative path means the current directory in Claude Code and the workspace in Cursor.

A rule you remove from a spec leaves `cli.json` on the next sync. A rule you wrote there yourself stays. Sync always writes both lists, since the Cursor CLI rejects a `permissions` object without one, and removes the file it created once no rule is left. With `memory.personal: repo`, sync also allows writes to the [personal memory folder](@/docs/memory.md#one-store-per-repository).

## Import

`agnostic-ai import cursor` reads:

| Source | Becomes |
|--------|---------|
| `.cursor/rules/<name>.mdc` | `<rules>/<name>.md`, frontmatter kept as written |
| `.cursor/rules/<sub>/<name>.mdc` | `<rules>/<sub>/<name>.md` |
| (no `name:` in frontmatter) | `name:` added from the filename |
| `.cursor/agents/<name>.md` | `<agents>/<name>.md`, generated header stripped |
| `.cursor/skills/<name>/` and `.agents/skills/<name>/` | `<skills>/<name>/`, full folder tree, SKILL.md merged onto the existing spec |
| `.cursor/commands/<name>.md` | `<commands>/<name>.md`, generated header stripped |
| `.cursor/BUGBOT.md` | `<reviews>/review.md` |
| `<scope>/.cursor/BUGBOT.md` | `<reviews>/<scope-slug>.md` with `scope: <scope>`, such as `services-api.md` for `services/api` |

Import skips a `BUGBOT.md` that sync wrote.

Both skill directories are project-level, at the root and in nested subdirectories. The nesting becomes the spec scope. `.cursor/skills` wins a same-name clash at the same scope.

- Cursor writes no `argument-hint` or `allowed-tools` on a skill, so import leaves both on the spec.
- Keys Cursor does write, such as a skill's `icon` or an agent's `model`, follow the native file.
- A rule's frontmatter comes from the `.mdc` alone, so widening `globs` to `**/*` unscopes the spec.

`import cursor` also reads environment files:

- A hand-written `worktrees.json` becomes `environments/worktree.yaml`, mapped the same way sync writes it.
- A hand-written `environment.json` (comments allowed) becomes `environments/cursor.yaml`, keeping every key and turning `//` comments into YAML comments. A file that sets a key the spec reads itself, such as `name` or `setup`, stays as written with a note.

## Protected paths

Enforced (permission). In the Cursor CLI, each path of a `decision: deny` block becomes `Write(<path>)` and `Write(<path>/**)` rules in `permissions.deny` of `.cursor/cli.json`, the project CLI config ([configuration](https://cursor.com/docs/cli/reference/configuration)). A path that already ends in `**` gets the first rule only. The rules carry no leading `/`: Cursor scopes a relative path to the workspace and reads a leading `/` as an absolute path ([CLI permissions](https://cursor.com/docs/cli/reference/permissions)).

- **IDE agent**: not covered ([permissions](https://cursor.com/docs/reference/permissions.md)). State the paths in a rule if the IDE agent should know about them.
- **`decision: ask`**: not enforced, with a coverage note. The CLI has no ask list and already prompts before a write no `allow` rule covers. Use `decision: deny` to block.
- **`reason`**: not written. A CLI permission rule has no message field.
- **Other settings fields**: `model`, `effort`, and `x-cursor` raise a coverage note, since a project `cli.json` takes `permissions` only.

`cli.json` merges as [Permissions](#permissions) describes: your own rules and other keys stay, and removing a path or its block removes its rules on the next sync. A matching rule that was in `cli.json` before sync added one stays yours. See [Protected paths](@/docs/spec-format/settings.md#protected-paths).

## Verify

1. Install Cursor from [cursor.com](https://cursor.com).
2. Check the tree: `ls .cursor/rules/ .cursor/skills/ .cursor/commands/ .cursor/mcp.json`, `grep "Generated by agnostic-ai" .cursor/rules/*.mdc` (the header sits after the frontmatter), `python -m json.tool .cursor/mcp.json > /dev/null`.
3. Open the project. The Rules panel loads every `.cursor/rules/*.mdc` with the right `alwaysApply` and no "failed to parse" warnings. The Skills list, agent picker, and `/` picker show each skill, agent, and command.
4. If MCPs are configured, Settings → MCP shows every `mcpServers.<name>` green.
5. If hooks are configured, `python -m json.tool .cursor/hooks.json > /dev/null` parses. Trigger the matched event (for example a shell command for `beforeShellExecution`) and check that the `command` runs.

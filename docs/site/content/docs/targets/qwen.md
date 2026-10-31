+++
title = "Qwen Code"
description = "Native Qwen Code instructions, agents, skills, commands, hooks, MCP, settings, and ignore files."
weight = 32

[extra]
group = "Reference"
target_id = "qwen"
+++

# Qwen Code (`qwen`)

agnostic-ai writes Qwen Code's native project files and imports existing ones. This target follows [Qwen Code v0.26.0](https://github.com/QwenLM/qwen-code/releases/tag/v0.26.0).

## First rule

Select the target in `agnostic-ai.yaml`:

```yaml
targets: [qwen]
```

Save `.agnostic-ai/rules/commit-prefix.md`:

```markdown
---
name: commit-prefix
---

Start commit subjects with fix:, feat:, docs:, or chore:.
```

Run:

```bash
agnostic-ai sync -t qwen
agnostic-ai sync --check -t qwen
```

Inspect `QWEN.md` for the rule. This checks generated files. To check discovery, start Qwen Code from the project root and ask for a commit subject. For an existing setup, [import it](#import) before syncing.

## Output

```text
QWEN.md                         # root instructions and rules
<scope>/QWEN.md                 # directory-scoped rules
.qwen/rules/<name>.md           # rules with file patterns
.qwen/agents/<name>.md          # native subagents
.qwen/skills/<name>/SKILL.md    # skills, with bundled files
.qwen/commands/<name>.md        # slash commands
.qwen/settings.json            # MCP, hooks, and settings
.qwenignore                    # ignore patterns
```

## Native mappings

Project-wide rules go into `QWEN.md`, and `scope` rules go into `<scope>/QWEN.md`. A rule with `globs` or `paths` and no `scope` goes to `.qwen/rules/<name>.md` with a `paths:` list. Qwen leaves that rule out of the prompt until a tool reads or edits a matching file. `alwaysApply: true` keeps a rule in `QWEN.md`. See [scoped context](@/docs/scoped-context.md#qwen-code-rules-with-globs-alone) for the limits.

Agents get YAML frontmatter and a prompt body. Portable tool names map to Qwen names; use `x-qwen.tools` for an explicit native list. Native agent fields such as `approvalMode` and record-valued `mcpServers` belong under `x-qwen`.

Skills keep their bundled files. Set `x-qwen.model` on a skill to select its native model; the shared skill `model` field is not promoted. Commands use Markdown with optional `description` frontmatter. Qwen interprets `{% raw %}{{args}}{% endraw %}`, `!{shell}`, and `@{path}` in command bodies; sync keeps that text as written. Nested command paths become colon-separated command names, and sync writes a command named `git:commit` back as `.qwen/commands/git/commit.md`, on Windows too. On Windows, a target that writes such a name as one file refuses it; see [path semantics](@/docs/configuration.md#path-semantics).

MCP servers share `settings.json` under `mcpServers`. Stdio uses `command`, `args`, `env`, and optional `cwd`; HTTP uses `httpUrl`, and SSE uses `url`. MCP timeouts are milliseconds. Servers marked `disabled: true` are omitted from this output because Qwen ignores that per-server field. A same-name server in existing or inherited configuration can still load; use native `mcp.excluded` to block it. Environment placeholders stay literal until Qwen resolves them. See the [native MCP reference](https://qwenlm.github.io/qwen-code-docs/en/users/features/mcp/).

Hooks share `settings.json` under `hooks`. Portable `on`, `match`, and `decision: stdout` map to Qwen events, tool names, and blocking decisions. Hook timeouts are seconds. Sync quotes portable arguments into the shell command because Qwen has no separate `args` field. Portable hooks that exit 2 block pre-tool, prompt-submit, and stop events; after-tool and after-edit stop further execution. Session-start and session-end keep their native lifecycle behavior. Project import preserves native command, HTTP, and prompt handlers and their metadata.

Settings map portable `model` to `model.name` and permission lists to `permissions.allow`, `permissions.ask`, and `permissions.deny`. Keys under `x-qwen` keep their native spelling. Native `x-qwen.mcpServers` records override portable servers with the same name. Native and portable hook entries are combined. Sync preserves unrelated settings, including the active `disableAllHooks` setting. Legacy `hooks.enabled`, `hooks.disabled`, and `hooks.notifications` metadata is preserved but does not disable hooks in this release. See [Qwen settings](https://qwenlm.github.io/qwen-code-docs/en/users/configuration/settings/).

Save `.agnostic-ai/mcps/docs-search.yaml` to use your own HTTP endpoint:

```yaml
name: docs-search
type: http
url: https://mcp.example.com/mcp
```

Replace the example URL with your server's endpoint. After sync, `.qwen/settings.json` contains `mcpServers.docs-search.httpUrl` with that URL.

To exclude a server inherited from user or extension configuration, put this in a settings spec:

```yaml
x-qwen:
  mcp:
    excluded: [server-name]
```

Qwen combines this native exclusion list across trusted settings scopes. Project and global sync merge entries individually, so removing a source exclusion keeps user entries and other MCP settings.

## Protected paths

Not enforced. Sync cannot write an edit-blocking rule or hook for this tool, so it prints a [protected-path coverage note](@/docs/spec-format/settings.md#protected-paths).

## Limits and trust

- Portable agent effort and agent permission mode have no equivalent mapping. Use explicit native approval fields under `x-qwen` when needed. Portable lists of agent MCP server names cannot replace Qwen's record of server definitions.
- Project hooks and `.qwen/rules/` require a trusted folder. Safe mode and bare mode can disable project customizations. Trust and approval remain Qwen decisions.
- `.qwenignore` applies to tools that honor it; it does not prevent every form of file access.
- Commands and ignore files sync only at project scope. Global hook specs emit command handlers.
- Reviews and environment specs have no native output.

## Global configuration

Portable [voice settings](@/docs/spec-format/settings.md#voice-dictation) write `general.voice.enabled`, `general.voice.mode`, and `general.voice.language`; choose a native `voiceModel` before using dictation.

`agnostic-ai sync --global -t qwen` writes instructions, agents, skills, MCP, hooks, and settings under `~/.qwen/`. Set `QWEN_HOME` to replace that whole directory. `QWEN_RUNTIME_DIR` moves runtime state, not configuration. `agnostic-ai import --global qwen` reads the user model, permission lists, and MCP servers. See [global configuration](@/docs/global-configuration.md).

## Config keys

| Key | Default |
|---|---|
| `outputs.qwen.rules-file` | unset, uses instruction files |
| `outputs.qwen.rules-dir` | `.qwen/rules` |
| `outputs.qwen.agents-dir` | `.qwen/agents` |
| `outputs.qwen.skills-dir` | `.qwen/skills` |
| `outputs.qwen.commands-dir` | `.qwen/commands` |
| `outputs.qwen.conf-file` | `.qwen/settings.json` |
| `outputs.qwen.mcp-file` | `.qwen/settings.json` |
| `outputs.qwen.hooks-file` | `.qwen/settings.json` |
| `outputs.qwen.ignore-file` | `.qwenignore` |

Root instructions use the shared [entry-point configuration](@/docs/configuration.md#outputs). Changing a path does not change where Qwen looks for it. Keep overrides aligned with your native configuration.

## Import

```bash
agnostic-ai import qwen
agnostic-ai lint
agnostic-ai sync -t qwen
agnostic-ai sync --check -t qwen
```

Import reads root and scoped instructions, rule files directly under `.qwen/rules/`, agents, skill folders and resources, Markdown and legacy TOML commands, `.qwenignore`, and settings, hooks, and MCP servers in `.qwen/settings.json`. A rule file's `paths:` becomes portable `globs`. Import names each rule file without `paths:`: sync moves that rule into `QWEN.md`, so delete the original file. It also names and skips rule files in subfolders, and stops when a rule file shares its name with a rule in `QWEN.md`. Native fields stay under `x-qwen`; HTTP `httpUrl` becomes portable `type: http`, while SSE `url` becomes `type: sse`. Imported legacy commands emit as Markdown.

Import rejects credentials in retained agent and skill headers, hooks, and settings. Settings can refer to environment variables with `$NAME` or `${NAME}`. Qwen does not expand those references in an agent's or skill's credential fields, so import rejects them there.

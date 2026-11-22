+++
title = "Gemini CLI"
description = "Share project rules, agents, and skills with Gemini CLI, then check the native files it loads."
weight = 30

[extra]
group = "Reference"
target_id = "gemini"

[extra.moved]
native-mappings = "@/docs/targets/gemini-reference.md"
rules = "@/docs/targets/gemini-reference.md#rules"
agents = "@/docs/targets/gemini-reference.md#agents"
skills = "@/docs/targets/gemini-reference.md#skills"
commands = "@/docs/targets/gemini-reference.md#commands"
settings = "@/docs/targets/gemini-reference.md#settings"
mcp = "@/docs/targets/gemini-reference.md#mcp"
hooks = "@/docs/targets/gemini-reference.md#hooks"
ignore = "@/docs/targets/gemini-reference.md#ignore"
config-keys = "@/docs/targets/gemini-reference.md#config-keys"
import = "@/docs/targets/gemini-reference.md#import"
set-up-this-tool = "@/docs/targets/gemini.md#first-rule"
+++

# Gemini CLI (`gemini`)

## Set up this tool {#set-up-this-tool .legacy-anchor}

## Native mappings {#native-mappings .legacy-anchor}

## Config keys {#config-keys .legacy-anchor}

## Import {#import .legacy-anchor}

agnostic-ai writes `GEMINI.md`, native subagents, commands, and skills under `.gemini/`, and merges MCP, hooks, and settings into `.gemini/settings.json`. To write for Antigravity CLI, which replaced Gemini CLI for unpaid-tier and Google One users on June 18, 2026, add the [`antigravity`](@/docs/targets/antigravity.md) target. This target is unchanged.

## First rule

1. Follow the [official setup guide](https://geminicli.com/docs/).
2. Select `gemini` in `agnostic-ai.yaml`.
3. Follow [First rule](@/docs/getting-started.md#first-rule). The commit-prefix rule becomes a section in `GEMINI.md`.

Directory rules use nested `GEMINI.md` files ([scoped context](@/docs/scoped-context.md)). Already have Gemini files? See [what `import gemini` reads](@/docs/targets/gemini-reference.md#import).

## Output

| Source | Default output |
|---|---|
| Shared instructions | `GEMINI.md` |
| Agents | `.gemini/agents/<name>.md` |
| Commands | `.gemini/commands/<name>.toml` |
| Skills and bundled files | `.gemini/skills/<name>/SKILL.md` |
| Skills as commands | `.gemini/commands/skill-<name>.toml`, only with `emit-skills-as-commands: true` |
| MCP, hooks, settings | `.gemini/settings.json`, when entries exist (merged with user config) |
| Ignore entries | `.geminiignore`, when entries exist |

Overrides, field mappings, and import are in the [native reference](@/docs/targets/gemini-reference.md).

## Verify

1. Run `agnostic-ai sync --check`.
2. Look for the commit-prefix rule in `GEMINI.md`.
3. Start Gemini CLI at the project root and ask for a commit subject for a small fix. It should use `fix:`.

## Loading and limits

- Start Gemini CLI at the project root. It reads `.gemini/settings.json` from where you start, and runs project hooks only in a trusted folder.
- A rule with `scope: services/payments` goes to `services/payments/GEMINI.md` ([rules](@/docs/targets/gemini-reference.md#rules)).
- Unknown `tools` names drop with a sync note. Omit `tools` to inherit all ([agents](@/docs/targets/gemini-reference.md#agents)).
- Portable permission rules raise a sync note. Use [protected paths](#protected-paths) to block edits.

{% <details summary="More limits"> %}
- Per-agent MCP servers have no agnostic-ai field. Set `x-gemini.mcp_servers`. Gemini rejects `mcpServers` and skips the agent.
- Gemini also reads `.agents/skills/`. It wins when a name is in both trees.
- Hook `command` runs through `bash -c` (PowerShell on Windows) and has no `args` field, so sync folds `args` into `command` ([hooks](@/docs/targets/gemini-reference.md#hooks)).
- On Windows, sync refuses an agent named with a colon ([path semantics](@/docs/configuration.md#path-semantics)).
- At user scope, [voice settings](@/docs/spec-format/settings-reference.md#voice-dictation) write `experimental.voiceMode` and `experimental.voice.activationMode`. Gemini has no dictation language key.
{% </details> %}

## Shared memory

Gemini CLI edits its own memory files and cannot be stopped, so it would save team facts to `GEMINI.md`, which sync overwrites. With `builtins: [memory]`, sync adds the `gemini-memory-tiers` rule to `GEMINI.md`:

| Gemini CLI tier | Saves to |
| --- | --- |
| Project instructions: team conventions, architecture rules, workflows | `.agnostic-ai/memory/`, after you confirm, not `GEMINI.md` |
| Private project memory: your machine and local setup | `~/.gemini/tmp/<project>/memory/`, plus a copy in your personal memory folder |
| Personal memory for every project: your preferences | `~/.gemini/GEMINI.md`, outside the block `sync --scope user` writes, plus a copy in your personal memory folder |

{% <details summary="Details: copies, edits, shared files"> %}
- The copies let other tools read what Gemini CLI saves.
- If Gemini CLI edits `GEMINI.md` anyway, the next sync keeps the edit in `GEMINI.md.bak` and suggests saving it in `.agnostic-ai/memory/`.
- If another target also reads Gemini's instructions file (such as `AGENTS.md`), sync leaves the rule out of it and prints a note.
- `sync --scope user` also leaves it out, because the user block reaches every tool.
- See [shared memory](@/docs/memory.md).
{% </details> %}

## Protected paths

Enforced (hook). Gemini CLI's policy engine can deny `write_file` and `replace`, but its workspace tier "is currently non-functional" ([policy engine](https://geminicli.com/docs/reference/policy-engine/)). So sync writes `.gemini/hooks/agnostic-ai-protect.sh` and a `BeforeTool` hook on `^(write_file|replace)$` in `.gemini/settings.json`. For a protected `tool_input.file_path`, the script exits 2 with the reason on stderr. Gemini CLI then blocks the call and shows the reason ([hooks reference](https://geminicli.com/docs/hooks/reference/)). The script needs only `sh` and `awk`.

{% <details summary="Details: ask, shell commands, script failures, odd paths"> %}
- `decision: ask` also blocks and tells the agent to ask.
- Start Gemini CLI at the project root (see above).
- The hook does not catch shell commands that write files.
- Gemini CLI lets an edit through on exit 1 or a timeout, so the script blocks when it cannot run: no `awk`, an unreadable project root, or an `awk` failure.
- It ignores case and reads `\` as a path separator.
- It checks a path with a leading `@` both ways, because Gemini CLI strips the `@`.
- `replace` searches the workspace for a relative path missing from the project root. The hook blocks that call and asks for the full path.
- On Windows it needs `sh` on `PATH` (Git for Windows has one). Without it the edit goes through.
{% </details> %}

See [Protected paths](@/docs/spec-format/settings.md#protected-paths).

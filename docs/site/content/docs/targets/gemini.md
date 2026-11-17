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

agnostic-ai writes `GEMINI.md`, native subagents, commands, and skills under `.gemini/`, and merges MCP, hooks, and settings into `.gemini/settings.json`.

Gemini CLI's own docs say Antigravity CLI replaced it for unpaid-tier and Google One users on June 18, 2026. Gemini CLI still ships and this target is unchanged. To write for Antigravity, add the [`antigravity`](@/docs/targets/antigravity.md) target.

## First rule

Follow the [official setup guide](https://geminicli.com/docs/), select `gemini` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule). The shared commit-prefix rule becomes a section in `GEMINI.md`.

Directory-specific rules use nested `GEMINI.md` files. See [scoped context](@/docs/scoped-context.md) before sharing file-pattern rules. Already have Gemini files? Check [what `import gemini` reads](@/docs/targets/gemini-reference.md#import) first.

## Output

| Source | Default output |
|---|---|
| Shared instructions | `GEMINI.md` |
| Agents | `.gemini/agents/<name>.md` |
| Commands | `.gemini/commands/<name>.toml` |
| Skills and bundled files | `.gemini/skills/<name>/SKILL.md` |
| Skills as commands | `.gemini/commands/skill-<name>.toml`, only when `emit-skills-as-commands: true` |
| MCP, hooks, settings | `.gemini/settings.json`, when entries exist (merged with user config) |
| Ignore entries | `.geminiignore`, when entries exist |

For output overrides, field mappings, and import coverage, see the [native reference](@/docs/targets/gemini-reference.md).

## Verify

Run `agnostic-ai sync --check` and inspect `GEMINI.md` for the commit-prefix rule. Start Gemini CLI from the project root and ask for a commit subject for a small fix; it should use `fix:`.

## Loading and limits

- Rules without a directory scope go into the root `GEMINI.md`. A rule with `scope: services/payments` goes to `services/payments/GEMINI.md`. See [rules](@/docs/targets/gemini-reference.md#rules).
- A generic agent `tools` list maps to Gemini's tool names. Other names drop with a sync note. Omit `tools` to inherit all tools. See [agents](@/docs/targets/gemini-reference.md#agents).
- Gemini CLI runs project hooks only in a trusted folder and reads `.gemini/settings.json` from the directory you start in, so start at the project root.
- Portable permission rules raise a sync note. Use [protected paths](#protected-paths) to block edits.

{% <details summary="More limits"> %}
- Per-agent MCP servers have no agnostic-ai field. Set `x-gemini.mcp_servers`. Gemini rejects the documented `mcpServers` key and skips the agent.
- Gemini CLI also reads `.agents/skills/`, which wins when a name appears in both trees.
- Gemini runs hook `command` through `bash -c` (PowerShell on Windows) and has no `args` field, so sync folds `args` into `command`. See [hooks](@/docs/targets/gemini-reference.md#hooks).
- On Windows, sync refuses an agent named with a colon. See [path semantics](@/docs/configuration.md#path-semantics).
- At user scope, portable [voice settings](@/docs/spec-format/settings.md#voice-dictation) write the experimental `experimental.voiceMode` and `experimental.voice.activationMode` keys; Gemini has no dictation language key.
{% </details> %}

## Shared memory

Gemini CLI saves facts by editing its memory files itself, and has no setting to turn that off. On its own, it saves team facts to `GEMINI.md`, which sync writes over. With `builtins: [memory]`, sync adds the `gemini-memory-tiers` rule to `GEMINI.md`. Gemini CLI keeps its own memory, with one change for the file sync owns:

| Gemini CLI tier | Saves to |
| --- | --- |
| Project instructions: team conventions, architecture rules, workflows | `.agnostic-ai/memory/`, after you confirm, instead of `GEMINI.md` |
| Private project memory: your machine and local setup | its own private memory (`~/.gemini/tmp/<project>/memory/`), plus a copy in your personal memory folder |
| Global personal memory: preferences for every project | `~/.gemini/GEMINI.md`, outside the block `sync --global` writes, plus a copy in your personal memory folder |

The copies let other tools read facts Gemini CLI saves. If it edits `GEMINI.md` anyway, the next sync keeps the edit in `GEMINI.md.bak` and suggests saving it as a fact in `.agnostic-ai/memory/`.

When Gemini CLI's instructions file is one another target also reads, such as `AGENTS.md`, sync leaves the rule out of that file and prints a note. `sync --global` leaves it out too, because the global block reaches every tool. See [shared memory](@/docs/memory.md).

## Protected paths

Enforced (hook). Gemini CLI's policy engine can deny `write_file` and `replace`, but its workspace tier "is currently non-functional" ([policy engine](https://geminicli.com/docs/reference/policy-engine/)). So sync writes `.gemini/hooks/agnostic-ai-protect.sh` and a `BeforeTool` hook on `^(write_file|replace)$` in `.gemini/settings.json`. For a protected `tool_input.file_path`, the script exits 2 with the reason on stderr. Gemini CLI then blocks the call and shows the reason ([hooks reference](https://geminicli.com/docs/hooks/reference/)). The script needs only `sh` and `awk`.

`decision: ask` also blocks and tells the agent to ask. Gemini CLI runs project hooks only in a trusted folder and reads `.gemini/settings.json` from the directory you start in, so start at the project root. The hook does not catch shell commands that write files.

{% <details summary="Script failures and odd paths"> %}

Gemini CLI lets an edit through on exit 1 or a timeout, so the script blocks when it cannot run: no `awk`, an unreadable project root, or an `awk` failure. It ignores case, reads `\` as a path separator, and checks a path with a leading `@` both ways, because Gemini CLI strips the `@`. `replace` searches the workspace for a relative path that is missing from the project root, so the hook blocks that call and asks for the full path. On Windows it needs `sh` on `PATH` (Git for Windows has one), or the edit goes through.

{% </details> %}

See [Protected paths](@/docs/spec-format/settings.md#protected-paths).

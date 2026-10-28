+++
title = "Lint codes"
description = "Look up lint findings, their limits, and the source change that fixes them."
weight = 35

[extra]
group = "Reference"
+++

# Lint codes

Run `agnostic-ai explain LINT011` for one code, or [lint](@/docs/cli-reference/check.md#lint) to check your specs. An error fails `lint`; with `lint --strict`, any warning fails too.

| Code | Finding |
|------|---------|
| LINT001 | Warning. A spec has no body and no description. Hook specs are skipped; see LINT052. |
| LINT003 | Error. Two specs of one kind share a `name`. Hooks that share an event and matcher are fine. |
| LINT004 | Warning. No enabled target supports the spec's kind, so sync writes it nowhere. The finding lists the tools that do. |
| LINT005 | Warning. A hook sets `matcher` on an event that does not read one. Drop it or use a tool-call event such as `PreToolUse`. |
| LINT006 | Error. Frontmatter opens with `---` and never closes. |
| LINT007 | Warning, also printed by `sync`. A frontmatter key one edit away from a key agnostic-ai reads (`glob:` for `globs:`), so the setting is lost. Put tool-native keys under `x-<target>:`. A key some tools read at the top level (Qoder's `glob:`, OpenCode's and Kilo's `mode:`) is flagged only when none of them is in `targets`. Settings and environment specs are not checked. |
| LINT008 | Error. A stdio MCP server lacks `command:`, or an `http`/`sse`/`ws` one lacks `url:`. `x-<target>` cannot set either. |
| LINT009 | Warning. A permission that approves more than it says: `allow: Bash(git * main)` also matches a force push to main. Write the exact value, or keep `*` at the end (`Bash(go test:*)`). A `deny` rule of that shape blocks nothing in Claude Code, so it is flagged too. |
| LINT010 | Error, `--global` only. A rule with scope, path, glob, or target conditions. |
| LINT011 | Warning. Startup instructions exceed a word or byte budget. See [context budgets](#context-budgets). |
| LINT012 | Warning. A skill or agent description (or `x-<target>.description`) is longer than [`lint.description-chars`](@/docs/configuration.md#lint) (default 1024). A skill description past 1024 warns even with a raised budget. |
| LINT013 | Error, also from `validate`. A rule's `globs`, `paths`, or `x-<target>.globs` is neither a string nor a list of strings, or `paths` and `globs` name different patterns. |
| LINT014 | Error, `--global` only. A settings `effort` or `permissions.default-mode` that a tool's user settings cannot take, such as effort `max` for Claude or Copilot. `sync --global` drops it with a note. |
| LINT015 | Warning. A spec body names another spec by a target-native path such as `.claude/skills/style/SKILL.md`. The finding names the `.agnostic-ai/` source path to use. |
| LINT016 | Error. A `dev-commands` entry in an environment spec has no `name:`, has neither `command:` nor `url:`, repeats a name, is not a mapping, sets a key no tool reads, or gives `cwd`, `url`, `auto-port`, `port`, or `env` the wrong type. A `url:` entry with no `command:` is reported only when it reaches a tool that needs a command, such as Codex. |
| LINT017 | Warning. A `gitignore.commit` entry such as `cursor:environments` names a target missing from `targets`. |
| LINT018 | Warning. A skill's frontmatter sets `scope`, which does nothing; the folder under `skills/` sets the scope. |
| LINT019 | Warning. A skill or command uses Claude Code body syntax (`` !`command` ``, a ` ```! ` block, `$ARGUMENTS`, `$0`, `$1`) outside a `::target` fence, so other enabled tools read it as text. See [Claude Code body syntax](@/docs/spec-format/skills.md#claude-code-body-syntax). |
| LINT020 | Warning. A rule sits in a folder that names a project directory, such as `rules/backend/`, but its `scope:` points elsewhere. `scope:` wins. Move the file or drop `scope`. |
| LINT021 | Warning. With Codex enabled: a Bash `allow` or `deny` rule in portable Settings or `outputs.claude.settings.permissions` has no Codex prefix with the same decision. See [Bash permission translation](@/docs/targets/codex-reference.md#translate-bash-permissions). |
| LINT022 | Warning. A settings `protected` path covers a file sync writes, such as `.claude/**`. Protect the spec under `.agnostic-ai/` instead. See [protected paths](@/docs/spec-format/settings.md#protected-paths). |
| LINT023 | Error, and `sync` fails. A settings `protected` block has no `paths`, a `decision` other than `ask` or `deny`, an unknown key, or a path outside the project or with a character class, brace, or negation. |
| LINT024 | Warning. A [`coverage.accept`](@/docs/configuration.md#coverageaccept) entry matches no coverage note on one of its targets. Remove the entry or that target. |
| LINT025 | Warning. A [`models`](@/docs/configuration.md#models) tier a spec names has no entry and no `default` for an enabled tool that writes the spec, so that tool uses its default model. Also raised for a tier named like a Claude model, such as `opus`. |
| LINT026 | Warning. A Claude model name reaches a tool that cannot load it, through an agent's `model` or the `default` of a tier it names. Write `model: {claude: <name>}` or add the tool to the tier. |
| LINT027 | Error, also from `validate`, and `sync` fails. An MCP spec holds a value JSON cannot hold, such as YAML `.nan` or `.inf`. The finding names the field, for example `x-amp.timeout`. |
| LINT028 | Warning. An MCP `url` or `args` element holds a reference form only one tool reads, such as `${env:NAME}`, `{env:NAME}`, or `{% raw %}${{ secrets.NAME }}{% endraw %}`; the other tools get it as text. Write `${NAME}`, or move the value under `x-<target>:`. |
| LINT029 | Warning. With Kiro and a tool that writes rules into `AGENTS.md` (such as Codex) enabled, a Kiro agent sets `x-kiro.resources` without `file://AGENTS.md`. With Kiro's `chat.disableInheritingDefaultResources` on, that agent loads no always-on rules. Add `file://AGENTS.md` to its resources. |
| LINT030 | Warning. A Gemini CLI hook command has a bare `$GEMINI_PROJECT_DIR`, `$GEMINI_CWD`, `$GEMINI_PLANS_DIR`, `$GEMINI_SESSION_ID`, or `$CLAUDE_PROJECT_DIR`, which Gemini replaces as text before the shell runs. Write `"${NAME}"`. |
| LINT031 | Warning. A spec's `description` starts with `TODO`, the placeholder `agnostic-ai new` writes. It shows in tool pickers and decides when a skill fires. |
| LINT032 | Error. A hook's `on:` or `match:` holds an unknown value, is mixed with `event:` or `matcher:`, names an event that an enabled tool it reaches does not read the same way (such as `on: stop` on Crush), or a tool kind that tool lacks (such as `match: read` on Codex). Sync leaves the hook out for that tool. See [portable events](@/docs/spec-format/hook-reference.md#portable-events). |
| LINT033 | Error. A spec body holds an [agent or skill reference](@/docs/spec-format/_index.md#agent-and-skill-references) to a name that does not exist or does not sync to a tool the spec reaches. The finding lists the known names or the tools. |
| LINT034 | Warning, project only. A hook's `event:` and `matcher:` have a portable form that gives every enabled tool the same native event and matcher, such as `event: PreToolUse` with `matcher: Bash`. The finding names the `on:` and `match:` to write; `agnostic-ai migrate --only hooks` rewrites them. |
| LINT035 | Error, and `sync` stops. An MCP `env` or `headers` value is neither a `${NAME}` reference nor marked `!literal`, so it may be a secret sync writes into every tool. The finding names the server, field, and key, never the value. `agnostic-ai migrate --only secrets` turns a credential into a reference and marks the rest. For a server written into an agent file ([agent-only servers](@/docs/spec-format/mcps.md#agent-only)), it also fires on a credential in `url`, `args`, an `x-<target>` block, or a reference default. See [secrets and plain settings](@/docs/spec-format/mcps.md#plain-settings). |
| LINT036 | Error, also from `validate`, and `sync` stops. An agent's `can:` or skill's `allowed-tools` holds an unknown capability, a malformed `read(<path>)`, `edit(<path>)`, `shell(<pattern>)`, or `mcp:<server>`, or is not a list; for an agent, also `can:` beside `tools:` or under `x-<target>`. A settings permission rule that is an unknown capability, has a pattern on one that takes none (such as `write(.env)`), has an empty pattern, or is not a string (such as an unquoted rule holding `: `). See [capabilities](@/docs/spec-format/agents.md#capabilities) and [permission rules](@/docs/spec-format/settings.md#permission-rules). |
| LINT037 | Warning, only with `--suggest-capabilities`. A tool alias has an exact neutral capability form. |
| LINT038 | Warning. A bare capability under settings `permissions.allow` or `permissions.ask` covers a whole tool. The finding names the tools that take it and a scoped alternative. Scoped and `deny` rules do not warn. See [permission rules](@/docs/spec-format/settings.md#permission-rules). |
| LINT039 | Warning, project only, with the `memory` built-in. A [shared memory](@/docs/memory.md) index has more than 100 lines, or the two indexes pass 6,000 bytes. When a target loads memory through the [session-start hook](@/docs/memory.md#load-at-session-start), the finding names the first 10 facts that hook never loads; otherwise it names the targets that load the whole indexes every session. Merge or drop facts. |
| LINT040 | Error, project only. A memory index line links a fact file that does not exist. Restore the file, or run [`memory index`](@/docs/cli-reference/memory.md#memory) to drop the line. |
| LINT041 | Warning, project only. A memory fact file has no index line, so no tool loads it. [`memory index`](@/docs/cli-reference/memory.md#memory) adds one. |
| LINT042 | Error, project only. A line in a memory index or fact file looks like a credential, such as a prefixed token, a URL password, or a `Bearer` token. The finding names the line, never the value. |
| LINT043 | Warning, project only. A memory store no tool reads any longer holds facts personal memory lacks: Claude Code's own store once sync points Claude at personal memory, or the other personal folder after a `memory.personal` switch. The finding names the folder and the [`memory import`](@/docs/cli-reference/memory.md#memory) command that copies the facts. It counts facts by file name, plus one for notes kept in that store's index, and never prints a fact. |
| LINT044 | Error in project memory, warning in personal memory; not checked by `lint --global`. A memory index or fact line holds a character a reviewer cannot see, such as a zero-width character, a bidirectional control, or a Unicode tag. The finding names the line and code points, never the text. [`hook memory`](@/docs/cli-reference/maintain.md#hook-memory) leaves these characters out. A zero-width joiner or non-joiner between two non-ASCII characters, as in an emoji or a Persian word, is fine. |
| LINT045 | Error. `session-retention-days` must be an integer of 1 or more. Remove the field to leave native retention unchanged. |
| LINT046 | Warning. A spec writes one top-level key more than once, in frontmatter or in a YAML spec such as a hook or MCP server. Sync uses the last value; other YAML parsers reject the file. `import cursor` in 0.83.0 left a repeated `workspaces`. Remove the extra lines. |
| LINT047 | Warning. With Claude Code enabled, a skill or command name holds a character Claude Code refuses, so it does not load: a control, zero-width, text-direction, or private-use character, `<` or `>`, or a blank other than one plain space between words. The finding names the code points. Rename the folder or file, or edit `name:`. |
| LINT048 | Warning. An agent with a body has no `description`. Tools read it to decide when to hand the agent a task, and Claude Code skips an agent file without one, so sync writes the name there instead. Add a `description:` that says when to use the agent. |
| LINT049 | Warning. An MCP spec holds what looks like a credential where LINT035 does not look: user info or a token in `url`, a token in `args`, an `env` or `headers` value in an `x-<target>` block, or the default of a `${NAME:-default}` reference. The finding names the server and field, never the value. Write a `${NAME}` reference with no default; `!literal` plain settings pass. See [secrets and plain settings](@/docs/spec-format/mcps.md#plain-settings). |
| LINT050 | Error, and sync stops before writing any agent. An agent's `maxTurns` is not a whole number of 1 or more, or its `background` is not `true` or `false`. Values under `x-<target>` are not checked. |
| LINT051 | Warning, also printed by `sync`. A file under `.agnostic-ai/overlays/` is not one sync reads, so its keys reach no tool. A `<target>.settings.*` file of a tool that merges `x-<target>` from a [settings spec](@/docs/spec-format/settings.md), such as `gemini.settings.json`, points there. Any other file, such as a misspelled `claude.setings.json`, gets the list of [overlays](@/docs/spec-format/overlays.md) sync reads; a case-only mismatch is told to rename. Dot files, dot folders, and top-level README files are skipped. |
| LINT052 | Error, also reported by `validate`. A hook spec has no `command`, no other handler such as `prompt` or `url`, at the top level or under an `x-<target>` block, and no native form such as `x-kiro.action` or `x-gemini.hooks`, so no tool runs it. A key such as `run:` or `comand:` is named as the likely misspelling. |

## Context budgets

LINT011 warns when a tool loads more words at session start than [`lint.instructions-words`](@/docs/configuration.md#lint) allows (default 2000), counting the entry-point file as `sync` writes it, always-on rule files, skill and agent descriptions, and `@`-imported rule files. [`explain --context`](@/docs/cli-reference/inspect.md#rank-context-contributions) ranks what adds the most. Tools with the same numbers share one line:

```
LINT011 [warn] AGENTS.md: cline, windsurf, trae load 2396 words every session: AGENTS.md 1187 (AGNOSTIC_AI.md 136, rules 1032), always-on rule files 908, skill descriptions 195, agent descriptions 106; budget 2000 (lint.instructions-words).
```

It also warns past a tool's own cap:

| Tool | Cap |
|------|-----|
| Codex | Stops reading `AGENTS.md` at 32 KiB (`project_doc_max_bytes`). |
| Antigravity | Truncates rule files past 24,000 bytes. |
| Claude Code | Warns at startup when the combined agent descriptions pass 15,000 tokens. LINT011 estimates 4 characters per token and warns past the same figure. |

<details>
<summary>LINT011 for the Codex AGENTS.md chain</summary>

Codex also reads each scoped `AGENTS.md` between the root and its working directory, and stops at the same 32 KiB across the chain. LINT011 sums the chain per scope and warns past [`lint.codex-chain-bytes`](@/docs/configuration.md#lint) (default 32768), naming the scope, the total, and each file with its size. A review spec with `target: cursor` is left out of Codex's `AGENTS.md`.

</details>

+++
title = "Windsurf / Devin Desktop"
description = "What agnostic-ai writes for Windsurf / Devin Desktop: file paths, what the tool supports, and config options."
weight = 80

[extra]
group = "Reference"
target_id = "windsurf"

[extra.moved]
set-up-this-tool = "@/docs/targets/windsurf.md#first-rule"
+++

# Windsurf / Devin Desktop (`windsurf`)

## Set up this tool {#set-up-this-tool .legacy-anchor}

Windsurf became Devin Desktop (2026-06). The tool keeps its `windsurf` name, so existing `outputs.windsurf.*` keys and `x-windsurf` meta still work.

## First rule

Install the client using the [official setup guide](https://devin.ai/desktop), then open your project in it.

Select `windsurf` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule) to add the shared commit-prefix rule and sync it. With the default paths, it reaches `.devin/rules/conventional-commits.md`.

The target ID stays `windsurf`. It writes the current `.devin/` paths; Desktop and CLI differ in when scoped rules load.

For existing files, check [what `import windsurf` reads](#import) before syncing.

## Verify

Run `agnostic-ai sync --check` and inspect `.devin/rules/conventional-commits.md` for the commit-prefix rule. Open the project in Devin Desktop or CLI and ask for a commit subject for a small fix; it should use `fix:`.

Next, [add a skill](@/docs/spec-format/skills.md).

## Shared memory

With `builtins: [memory]`, a Devin CLI `SessionStart` hook adds shared memory indexes. It needs POSIX `sh` and prefers the project's installed agnostic-ai. Native memory stays available. See [shared memory](@/docs/memory.md#load-at-session-start).

Devin does not document the hook's default shell, working directory, timeout, or Windows execution. Local modeled checks do not establish native recall or Windows support; the memory rule remains the fallback when hooks do not run.

## Output

```
AGENTS.md                            # shared pointer body, plus the rules block when another tool adds it
.devin/rules/<name>.md
<scope>/.devin/rules/<name>.md       # one per scoped rule
.devin/agents/<name>.md              # one per agent (custom subagent profile)
.agents/skills/<name>/SKILL.md       # one folder per skill (shared folder)
.devinignore                         # when ignore entries exist (indexing and agent access)
.windsurfignore                      # when ignore entries exist (legacy name, older builds)
.devin/mcp_config.json               # when MCP entries exist
.devin/hooks.v1.json                 # when hook entries exist
.devin/config.json                   # when settings entries carry permission rules, or with repo-mode memory
```

## Native mappings

<details>
<summary>Field mappings, discovery, and compatibility</summary>

### Rules

Devin prefers `.devin/rules/*.md` and still reads `.windsurf/rules/` (`.windsurfrules` is legacy). Set `outputs.windsurf.rules-dir: .windsurf/rules` to keep the old layout. Otherwise sync removes the files it wrote there earlier and leaves hand-written files alone.

### Entry point

Devin also reads the root `AGENTS.md`, so `sync` writes the shared instructions that point to source specs there.

- When Codex or another tool that copies rules into `AGENTS.md` adds the `## Rules` block, Devin loads each always-on rule twice, because every rule also keeps its `.devin/rules/` file.
- Legacy Cascade [limits](https://docs.devin.ai/desktop/cascade/memories#rules) each workspace rule file to 12,000 characters and reads `AGENTS.md` the same way. The [Devin CLI rules reference](https://docs.devin.ai/cli/extensibility/rules) gives no size limit. Sync keeps the per-rule files for compatibility. See [target behavior](@/docs/target-behavior.md#entry-point-files).

### Agents

Custom subagent profiles ([subagents docs](https://docs.devin.ai/cli/subagents)) with `name`, `description`, `model`, `allowed-tools`, and `max-nesting`. The body is the system prompt. A scoped agent is written flat, because Devin looks in sub-folders for rules only.

- `allowed-tools` maps Claude-style names to Devin's ([permissions reference](https://docs.devin.ai/cli/reference/permissions), [CLI changelog](https://docs.devin.ai/cli/changelog/stable.md)): `Read`/`Grep`/`Glob`/`Bash` to `read`/`grep`/`glob`/`exec`, and `Write`/`Edit` to `write`/`edit`. `write` needs Devin CLI v3000.11.1 or later. `mcp__<server>__<tool>` is copied as written. Anything else is dropped with a sync note.
- **Devin also reads `.agents/agents/`**, flat `<name>.md` and nested `<name>/agent.md`. Profiles written for Antigravity, Goose, or OpenHands can therefore appear in Devin without `allowed-tools` restrictions, even when their specs exclude `windsurf`. A matching `.devin/agents/` profile can also produce a duplicate name. Check the discovered profiles; see [target behavior](@/docs/target-behavior.md#spec-kind-behavior).
- `x-windsurf.allowed-tools` writes Devin's names directly. `x-windsurf.max-nesting` sets nesting, which has no portable field. `model` is copied as written.
- Devin marks custom subagents as experimental, so the format may change.

### Scoped rules

Go to `<scope>/.devin/rules/<name>.md`, because Desktop finds `.devin/rules` or `.windsurf/rules` in any sub-folder ([Desktop rules reference](https://docs.devin.ai/desktop/cascade/memories)). Devin CLI v3000.11.1 also finds rules in nested folders under `.devin/rules/` and `.windsurf/rules/` ([CLI changelog](https://docs.devin.ai/cli/changelog/stable.md)). That is unconfirmed for Desktop, so the layout stays.

- With `outputs.windsurf.rules-dir` set, the prefix follows it: `<scope>/.windsurf/rules/<name>.md`.
- Devin CLI loads a sub-folder's rules once the agent touches files there. Desktop loads all of them at start. So the scope narrows what only the CLI sees.

### Rule activation

A rule gets a `trigger` frontmatter key when it sets `alwaysApply: false`, or sets `globs` other than a catch-all such as `**/*` without `alwaysApply`.

| Rule has | Trigger written |
| --- | --- |
| `globs` | `trigger: glob` (plus the pattern as written) |
| `description` alone | `trigger: model_decision` |
| neither | `trigger: manual` |

An always-on rule has no frontmatter. Devin treats a file without frontmatter as always-on and adds its body to every message, so a `description` would do nothing. Devin's fifth value, `agent`, has no portable counterpart, so sync never writes it.

### Skills

`.agents/skills/<name>/SKILL.md`. Devin reads `.agents/skills/`, `.devin/skills/`, and `.windsurf/skills/`. This adapter writes the first, so a skill is written once even when other tools also write `.agents/skills/`. Files next to the skill and file modes are kept.

- `disable-model-invocation: true` is written as `triggers: [user]`, which Devin documents to "prevent the agent from invoking a skill on its own" while `/skill-name` still runs it ([Devin skills](https://docs.devin.ai/cli/extensibility/skills/overview)). So each command reaches Devin as a skill the user runs by name ([tools without commands](@/docs/spec-format/commands.md#tools-without-commands)). When another target also writes the skills folder, the files must match, so the field is dropped with a note and commands are skipped; set `outputs.windsurf.skills-dir: .devin/skills` to keep them. Import reads `triggers: [user]` back as `disable-model-invocation: true`, so after import every selected tool treats that skill as user-only.
- Other native `triggers` live under `x-windsurf` in the spec and go back to top-level frontmatter on sync. This keeps `[model]` and combined invocation settings, and other tools never see a Devin-only field. An `x-windsurf.triggers` value wins over `disable-model-invocation`.

### Ignore

Ignore specs are written as `.devinignore` and `.windsurfignore`, in gitignore syntax under a `#` header comment. Devin skips matched paths when indexing and cannot view, edit, or create files there ([`.devinignore` docs](https://docs.devin.ai/desktop/context-awareness/devin-ignore)).

- Devin still enforces the legacy `.windsurfignore` and `.codeiumignore`. `outputs.windsurf.ignore-file: .codeiumignore` writes that name instead of `.devinignore`.
- `.windsurfignore` is for older Windsurf builds, where only it blocked agent file access. Sync always writes it, unless `outputs.windsurf.ignore-file` names it. That key moves only the main file.

### MCP

Servers are merged into `.devin/mcp_config.json` under `mcpServers`, which [Devin Local](https://docs.devin.ai/desktop/devin-local) reads for project scope. Cascade, with its own MCP file, was removed in Devin Desktop v3.9.19 ([changelog](https://docs.devin.ai/desktop/changelog.md), [Cascade MCP page](https://docs.devin.ai/desktop/cascade/mcp)).

- [The schema](https://docs.devin.ai/cli/extensibility/mcp/configuration) uses `command`/`args`/`env` for stdio and `url`/`transport` (`http` or `sse`)/`headers`/`oauthClientId`/`oauthClientSecret`/`oauthResource` for remote. The key is `transport`, not `type`, so this tool has its own format.
- Both accept `disabled`, also toggled by `devin mcp enable|disable` (see [`disabled` support by target](@/docs/spec-format/mcps.md#disabled-support-by-target)).
- A `type: ws` spec writes no server and gets a sync note. Devin remote servers use only `http` and `sse`.
- Cascade's personal file `~/.codeium/windsurf/mcp_config.json` is not written, since agnostic-ai writes project files only.

### Hooks

Hooks are merged into `.devin/hooks.v1.json` ([hooks docs](https://docs.devin.ai/cli/extensibility/hooks/overview)). Ten events: `PreToolUse`, `PostToolUse`, `PermissionRequest`, `UserPromptSubmit`, `Stop`, `PostCompaction`, `PreCompact`, `PostCompact`, `SessionStart`, `SessionEnd`. The hooks pages still list eight; `PreCompact` and `PostCompact` come from the [v3000.11.92 changelog](https://docs.devin.ai/cli/changelog/stable.md), which says they run before and after context compaction and that a failing hook does not block it.

- The file is the hooks object, `{"<Event>": [...]}`, with no `hooks` wrapper (unlike Claude Code, Codex, Gemini, and Qoder).
- Per entry: `type`, `command`, and optional `timeout` (seconds). `type` is `"command"`, or `"prompt"` for an LLM prompt. A hook spec gets `"prompt"` only through a hand-written `type`/`prompt` Meta pair.
- `matcher` on `PreCompact` and `PostCompact` is `manual` or `auto`, the changelog's two triggers. The built-in `handoff-pre-compact` hook does not reach Devin, because its payload is undocumented.
- `matcher` is a regex on `tool_name` for `PreToolUse`, `PostToolUse`, and `PermissionRequest`. **Devin CLI names tools in lowercase snake_case** (`exec`, `edit`, `read`, `write`, `apply_patch`, `grep`, `glob`, `webfetch`, ...), so a Claude-style matcher matches nothing. Sync adds a sync note and does not rename it.
- [`agnostic-ai hook run`](@/docs/cli-reference/hook-diagnostics.md#hook-run) runs them with Devin CLI's event data on an [assumed shell, working directory, and timeout](@/docs/cli-reference/hook-diagnostics.md#assumed-results). `--edit` is listed as not run, because Devin does not document the edit tools' input.

### Settings

Portable `permissions` are merged into `.devin/config.json`, the committed project policy ([permissions reference](https://docs.devin.ai/cli/reference/permissions)).

- Project configs accept only `permissions`, `read_config_from`, and `hooks` ([config file reference](https://docs.devin.ai/cli/reference/configuration/config-file)). Sync sets only `permissions`, so other keys, and other entries inside it, are kept.
- In `allow`, `deny`, and `ask`, sync owns only the rules your specs write. A rule you add by hand stays, and a rule leaves with its spec.
- An `x-windsurf` block on a settings spec is merged into the same file, which is how you set `read_config_from` and `hooks`. Its `permissions` are merged per spec and per list with the translated rules.
- A portable `model` is not written and gets a sync note, because Devin's `agent` block works only in your personal config.
- Translation: `Read(glob)` and `Write(glob)` are copied as written, `Edit(glob)` becomes `Write(glob)`, `Bash(prefix:*)` becomes `Exec(prefix)`, `WebFetch(pattern)` becomes `Fetch(pattern)`, and `mcp__<server>__<tool>` is copied as written.
- Bare tool names map to Devin's own (`Bash` to `exec`, and so on), the same way as in `allowed-tools`. `WebSearch` becomes `web_search`, accepted since Devin CLI v3000.10.21 ([CLI changelog](https://docs.devin.ai/cli/changelog/stable.md)), and imports back. A bare `WebFetch` is dropped, because `webfetch` is not a permissions name.
- Devin's `Exec` matches only the start of a command, so an exact `Bash(...)` rule without `:*` would match more commands. On `allow` and `ask` that would approve commands you did not list, so the rule is dropped with a sync note. On `deny` it blocks more, which is safe: `Bash(rm -rf /)` becomes `Exec(rm -rf /)`. A command deny wins over a broader `ask` or `allow` (Devin CLI v3000.10.31).
- `x-windsurf.permissions` writes Devin's rules directly and replaces that spec's portable ones.
- With `memory.personal: repo`, sync adds `Write(<folder>/**)` to `permissions.allow`, so Devin CLI saves to the [personal memory folder](@/docs/memory.md#one-store-per-repository) without asking. Your own rules stay. A committed settings kind (`gitignore.commit`) keeps the rule out. A folder path with pattern characters, including parentheses, gets no rule and a note; move `AGNOSTIC_AI_HOME` or set `memory.personal: checkout`.
- With `handoff` or `handoff-hook` enabled in a linked worktree, sync adds `Write(<store>/**)` for the main checkout's ignored [handoff store](@/docs/handoff.md#write-from-a-linked-worktree). Existing memory and user rules stay. A main checkout path with pattern characters, including parentheses, gets no rule. For a tracked, hand-written, or unmanaged file sync skips only this grant and reports the permission to add by hand (once you add it, it stays); the rest of its output is unchanged.

### Workflows

`outputs.windsurf.workflows-dir` writes nothing. It only prints a warning about moving to skills. `.devin/agents/<name>.md` is written either way.

<details>
<summary>Older paths and removed features</summary>

- `outputs.windsurf.workflows-dir` wrote each agent as a Cascade Workflow. Devin Desktop v3.9.19 removed Cascade ([changelog](https://docs.devin.ai/desktop/changelog.md)). Devin Local does not support Workflows and points users to skills ([Devin Local limitations](https://docs.devin.ai/desktop/devin-local)).
- Sync removes the `.devin/rules/agent-<name>.md` files it wrote for each current agent, and the old `.devin/rules/<scope>/<name>.md` folders.
- The MCP file moved here in v3000.3 (Local 3.6). Devin moves `mcpServers` from the older `.devin/config.json` on startup, so the path works for both.
- Devin CLI also reads a gitignored `.devin/mcp_config.local.json` for personal secrets, which agnostic-ai does not write. The generated `.devin/mcp_config.json` is in the managed `.gitignore` block by default (`gitignore.enabled: true`), so you do not need that split.

</details>

</details>

## Config keys

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.windsurf.rules-dir` | `.devin/rules` | |
| `outputs.windsurf.agents-dir` | `.devin/agents` | |
| `outputs.windsurf.skills-dir` | `.agents/skills` | |
| `outputs.windsurf.workflows-dir` | empty | when set, it only warns (see above) |
| `outputs.windsurf.ignore-file` | `.devinignore` | moves only the main ignore file; the legacy `.windsurfignore` copy follows the same spec |
| `outputs.windsurf.mcp-file` | `.devin/mcp_config.json` | |
| `outputs.windsurf.hooks-file` | `.devin/hooks.v1.json` | |
| `outputs.windsurf.conf-file` | `.devin/config.json` | project config; sync sets only `permissions` |

## Import

`agnostic-ai import windsurf` reads:

- **Rules**: from `outputs.windsurf.rules-dir` when set, else `.devin/rules/`, then legacy `.windsurf/rules/`, sorted by [filename prefix](@/docs/cli-reference/start.md#filename-prefix-reclassification). Scoped copies (`<scope>/<rules-dir>/*.md`) become scoped rules again, including scopes like `.github` or `vendor`. It skips every `node_modules/`.
- **Permissions**: from `.devin/config.json`. `Exec` has no exact-command form, so it imports as `Bash(<cmd>:*)`, not `Bash(<cmd>)`.
- **Skills**: from `.agents/skills/`, `.devin/skills/`, then `.windsurf/skills/`; the first same-name skill wins. Bundled assets and executable file modes are kept. `triggers` move under `x-windsurf`.
- **Ignore**: from `.devinignore`, or `.windsurfignore` when the first is absent. When both files have different hand-written patterns, combine them in the imported ignore spec, in that order, before you sync.

## Protected paths

Not enforced. Sync cannot write an edit-blocking rule or hook for this tool, so it prints a [protected-path coverage note](@/docs/spec-format/settings.md#protected-paths).

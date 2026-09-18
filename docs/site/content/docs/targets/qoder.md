+++
title = "Qoder"
description = "How agnostic-ai emits Qoder configuration: native paths, capability limits, and output options."
weight = 190

[extra]
group = "Reference"
target_id = "qoder"
+++

# Qoder (`qoder`)

## Output

```
AGENTS.md                        # canonical entry-point pointer body (written by sync, shared path)
.qoder/rules/<name>.md           # one per rule (native, one file per rule)
.qoder/agents/<name>.md          # one per agent (native, one file per agent)
.qoder/skills/<name>/SKILL.md    # one folder per skill, plus any bundled assets
.qoder/commands/<name>.md        # one per command
.qoder/settings.json             # when MCP, hook, or settings entries exist (merged; unrelated keys preserved)
```

Alibaba [Qoder](https://docs.qoder.com/user-guide/rules) reads project rules from `.qoder/rules/` natively, one Markdown file per rule, and also reads the root `AGENTS.md`. The per-rule files take precedence over `AGENTS.md`, so rules emit there rather than inlining into the shared pointer.

Skills emit into their own native folder tree at `.qoder/skills/<name>/SKILL.md` ([docs.qoder.com/extensions/skills](https://docs.qoder.com/extensions/skills), target-audit 2026-08-08, #558): "Each Skill contains a `SKILL.md` file", at project scope `.qoder/skills/{skill-name}/SKILL.md` (the vendor doc also lists a user-level `~/.qoder/skills/{skill-name}/SKILL.md` tier this adapter has no reach into). That doc does not list `.agents/skills/` as a compatible path, unlike Kilo Code, Augment, and OpenHands, so this is Qoder's own tree rather than a dedupe target for the shared one.

`import qoder` reads rules, agents, skill folders, commands, and portable settings fields. It does not yet read hooks or MCP servers back out of `.qoder/settings.json`.

- **Agents**: [Qoder Subagents](https://docs.qoder.com/extensions/subagent) reads `.qoder/agents/<name>.md`, one file per agent. `name` and `description` are required frontmatter; `model`, `tools`, `color`, `skills`, `mcpServers`, and `effort` are optional. See [`effort` support by target](@/docs/spec-format.md#effort-support-by-target).
  - `color` (one of eight named values: `red`, `blue`, `green`, `yellow`, `purple`, `orange`, `pink`, `cyan`) is documented on the [CLI field reference](https://docs.qoder.com/cli/subagent) rather than the smaller extensions page, which defers to the CLI page as "the complete guide" for the identical path. It is a shared portable field Augment and Kilo Code already promote the same way.
  - `tools` renders as a comma-separated string (`tools: Read, Grep, Bash`), the only form the vendor doc shows, not a YAML list. `import qoder` splits it back into agnostic-ai's generic list form so the spec stays usable by every other target.
  - Qoder's built-in tool vocabulary is Claude-style (`Bash`, `Edit`, `Write`, `Glob`, `Grep`, `Read`, `WebFetch`, `WebSearch`), which is what makes passing agnostic-ai's generic `tools` list straight through safe here, unlike Kilo Code and Augment, whose own vocabularies differ and which drop the field with a coverage note instead.
  - `skills` and `mcpServers` have no agnostic-ai-native shape and pass through whatever the spec declares via `x-qoder`.
  - The adapter manages `name`, `description`, `model`, `tools`, `skills`, and `mcpServers`. Any other `x-qoder` key passes through into the agent frontmatter.
- **Skills**: [Qoder Skills](https://docs.qoder.com/extensions/skills) reads a folder per skill at `.qoder/skills/<name>/SKILL.md`. Frontmatter is plain `name` + `description`, the only keys the vendor doc shows. Bundled sibling files (scripts, references, templates) copy byte-for-byte alongside `SKILL.md`, the same folder-layout render every other Agent Skills target here uses.
- **Commands**: one Markdown file per command spec at `.qoder/commands/<name>.md`.
  - [docs.qoder.com/cli/commands](https://docs.qoder.com/cli/commands) tables `.qoder/commands/<command_name>.md` as the project-level location, "Recommended (team sharing)", and [docs.qoder.com/user-guide/commands](https://docs.qoder.com/user-guide/commands) corroborates the same path for the IDE ("Project Commands", "Only effective in the current project root directory and its subdirectories"), so this is not a CLI-only surface.
  - The CLI page's field table documents exactly two frontmatter keys: `description` (Required: Yes) and `name` (Required: No, "serves only as the display name in the TUI; the invocation name is always derived from the file path"). Since the filename already drives invocation, this adapter never writes `name` and always writes `description`, falling back to the command's name when the spec has none.
  - One precedence quirk: the CLI page states that when a command with the same name exists at both the project and User levels, "the User-Level command takes precedence and overrides the project-level command with the same name", backwards from the read-order most targets document, and this adapter has no reach into the user-level tier (`~/.qoder/commands/`) to warn about a same-name collision.
  - The IDE page describes different behavior for the same case (both entries stay listed, distinguished by "a scope indicator", rather than one overriding the other), so a project-level command this adapter writes can still be shadowed depending on which Qoder product reads it.
- **MCP**: merges into `.qoder/settings.json` under the standard `mcpServers` map (stdio: `command`/`args`/`env`/`cwd`, no `type`; remote: `type` + `url`/`headers`).
  - Every entry also accepts the nine fields on [the vendor's "Common Optional Fields" table](https://docs.qoder.com/cli/mcp-reference): `timeout` (milliseconds), `description`, `trust` ("Trusts the server, skipping confirmation when its tools are called"), `includeTools`, `excludeTools`, `alwaysAllow`, `disabled` ("Disables the server (keeps the configuration without deleting it)"), and an `oauth` object passed through as declared, since the vendor's own field list for it is open-ended.
  - A `type: ws` spec emits no server and raises a coverage note. Qoder documents the transport on its own terms: the "ws Type (TCP)" table carries `tcp` ("TCP connection parameters (host/port)") and `type`, with `url` belonging to the `sse` and `http` tables only, and the spec has no field for a host and port (target-audit 2026-09-18, #855).
  - Unrelated keys in that file (`mcp.enableAllProjectMcpServers`, permissions, custom models) survive every sync; only `mcpServers` is overwritten.
  - That holds for a JSONC file too: the vendor documents this path as "JSON format (supporting `//` comments)" ([docs.qoder.com/cli/settings](https://docs.qoder.com/cli/settings)), so `sync` strips comments and trailing commas before reading. Keys survive, comments do not, and the sync that drops them prints a one-line warning (target-audit 2026-09-11, #725).
- **Settings**: the shared default model maps to `.qoder/settings.json` as `model.name`; `permissions.allow`, `permissions.deny`, and `permissions.ask` map directly. The write merges with hooks, MCP servers, and unrelated native keys. Import restores those portable fields to `settings/imported.yaml`.

  This adapter wrote the project-root `.mcp.json` until #641, byte-for-byte the file Claude Code writes there, and the two deduped into one write. That only held while their emitted bytes stayed identical, and they no longer can: Qoder documents nine per-server fields Claude Code does not, Claude Code documents four Qoder does not (`headersHelper`, `alwaysLoad`, and its own `oauth` and `timeout` shapes), and a spec using any of them would make two adapters write different bytes to one path.

  That is a hard error in the collision check, on a pair of targets both in the default set. `<project>/.qoder/settings.json → mcpServers` is the vendor's other documented project-level location and Claude Code never reads it, so moving there removes the shared path instead of arbitrating it.

  One migration step: a `.mcp.json` left by an older agnostic-ai still loads, and Qoder's precedence order puts it ahead of `.qoder/settings.json` for a same-named server. Sync cannot sweep it (a JSON file carries no provenance header, and the file may belong to Claude Code), so delete it by hand in a Qoder-only project. See [`disabled` support by target](@/docs/spec-format.md#disabled-support-by-target).

- **Hooks**: merge into that same `.qoder/settings.json`, under a `hooks` key alongside `mcpServers` ([docs.qoder.com/cli/hooks](https://docs.qoder.com/cli/hooks), "Configuration Format"): `{"hooks": {"<Event>": [{"matcher": ..., "hooks": [{"type": "command", "command": ..., ...}]}]}}`, the same nested shape Claude Code, Codex, and OpenHands use, so this adapter's renderer is the shared `claudehooks` wire structs rather than a fourth hand-rolled copy.
  - 27 events, PascalCase: `SessionStart`, `SessionEnd`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `PermissionRequest`, `PermissionDenied`, `Stop`, `StopFailure`, `SubagentStart`, `SubagentStop`, `PreCompact`, `PostCompact`, `Notification`, `InstructionsLoaded`, `ConfigChange`, `CwdChanged`, `FileChanged`, `WorktreeCreate`, `WorktreeRemove`, `Elicitation`, `ElicitationResult`, `TaskCreated`, `TaskCompleted`, `TeammateIdle`, `Setup`. That count grew from 6 at an earlier audit to 23 at #629; the authoritative list is [docs.qoder.com/cli/hooks-reference](https://docs.qoder.com/cli/hooks-reference), re-counted 2026-09-12, #737.
  - Every one of them emits, since `event:` passes through verbatim, and all 27 sort in the vendor's own order ahead of any event it does not list. The last four sorted behind an unlisted event until #744, so a project already using one of those names writes different bytes after that fix.
  - `PreToolUse`/`PostToolUse`'s `matcher` documents "Tool name (e.g. `Bash`, `Write`, `Edit`, `Read`, `Glob`, `Grep`; MCP tool names like `mcp__server__tool`)", Claude Code's own vocabulary, so a Claude-authored matcher reaches it unchanged, unlike OpenHands and Windsurf whose own tool names diverge.
  - Per entry: `command`, `type` (always `command`), optional `args`, `timeout` (seconds, vendor default 600), `statusMessage`, `async`, `asyncRewake`, `shell` (`bash` or `powershell`), and `if` (a permission-rule filter, e.g. `Bash(git *)`). All nine are documented with the same semantics `claudehooks.CommandEntry` already models for Claude Code.
  - `args` switches the entry to exec form: `command` becomes the path of a single executable and each `args` element is one literal argv entry, launched directly with no shell, so a path or argument holding a space, apostrophe, `$`, or backtick reaches the binary intact (#746). Qoder ignores `shell` once `args` is set; a spec that sets both still writes both, and `sync` prints a note saying the shell never runs.
  - Qoder additionally documents `env`, `rewakeMessage`, `rewakeSummary`, and three more hook entry types (`http`, `prompt`, `agent`); none has a field on the shared hook spec, so they are unreachable from a generic spec today.

  Merging `hooks` and `mcpServers` happens in one write, not two. `MergeJSONFile` re-reads `.qoder/settings.json` from disk on every call, and two separate calls in the same sync would each see the file before the other's write landed during sync's collision-detection pass, which reads as two targets disagreeing on one file's content when only Qoder writes it.

## Config keys

| Key | Default |
| --- | --- |
| `outputs.qoder.rules-dir` | `.qoder/rules` |
| `outputs.qoder.agents-dir` | `.qoder/agents` |
| `outputs.qoder.skills-dir` | `.qoder/skills` |
| `outputs.qoder.commands-dir` | `.qoder/commands` |
| `outputs.qoder.mcp-file` | `.qoder/settings.json` (also the hooks path since the two share one file) |

## Auto memory

Qoder keeps an automatic memory store it writes for itself ([docs.qoder.com/cli/memory](https://docs.qoder.com/cli/memory)): a project store at `~/.qoder/projects/<project>/memory/` and a user store at `~/.qoder/memory/`. Each one is a `MEMORY.md` index plus one file per topic. Run `/memory` for the overview and `/memory manage` to view, edit, or delete a topic file. A session loads the first 200 lines or about 25KB of each active `MEMORY.md` and drops everything past that.

Auto memory is off until you enable it, and agnostic-ai never reads or writes the store. See [Memory and local state](@/docs/targets/_index.md#memory-and-local-state).

## Verify

1. Install Qoder from [qoder.com](https://qoder.com).
2. Check the tree:
   - `ls AGENTS.md .qoder/rules/ .qoder/agents/ .qoder/skills/ .qoder/commands/ .qoder/settings.json`
   - `grep "Generated by agnostic-ai" .qoder/rules/*.md .qoder/agents/*.md .qoder/skills/*/SKILL.md .qoder/commands/*.md` for the provenance header
   - `python -m json.tool .qoder/settings.json > /dev/null`
3. Open the project:
   - The rules panel lists every `.qoder/rules/*.md` with no parse warnings.
   - The agent picker lists every `.qoder/agents/*.md`.
   - Each `.qoder/skills/<name>/` folder loads as a skill.
   - Each `.qoder/commands/<name>.md` runs from `/`.
   - The MCP picker shows each `mcpServers.<name>` from `.qoder/settings.json` connected (project-level servers need approval on first use).
   - A configured hook fires on its event (e.g. a `PreToolUse` hook prints or blocks before the matching tool runs).

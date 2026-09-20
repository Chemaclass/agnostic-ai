+++
title = "Augment"
description = "How agnostic-ai emits Augment configuration: native paths, capability limits, and output options."
weight = 250

[extra]
group = "Reference"
target_id = "augment"
+++

# Augment (`augment`)

## Output

```
AGENTS.md                     # canonical entry-point pointer body + inlined rules (written by sync, shared path)
.augment/
├── rules/<name>.md           # one per rule
├── agents/<name>.md          # one per agent
├── commands/<name>.md        # one per command; nested source scope becomes a namespace
└── settings.json             # mcpServers + hooks + toolPermissions, merged; only present with MCP, hook, or settings specs
.agents/skills/<name>/SKILL.md  # one folder per skill (shared cross-tool tree)
.augmentignore                # workspace indexing exclusions
.augment-guidelines           # opt-in legacy concatenated rules, only when rules-file is set
```

[Augment Code](https://docs.augmentcode.com/setup-augment/guidelines) reads the root `AGENTS.md`, with rule bodies inlined into the shared `## Rules` block, and also loads rules natively from `.augment/rules/`. Each rule frontmatter carries `type: agent_requested` (with a `description`, falling back to the rule name) when the spec sets `alwaysApply: false`; the vendor default `always_apply` stays implicit. There is no `name` key for rules.

Agents load from `.augment/agents/`, one `<name>.md` per agent, with `name` (required), `description` (falls back to the spec name), `color`, and `model` when set. `color` is free text that "should be a valid ANSI color name" ([docs.augmentcode.com/cli/subagents](https://docs.augmentcode.com/cli/subagents)); it is written verbatim without validation. `tools` and `disabled_tools` are real Augment fields, but only in Augment's own vocabulary (`view`, `codebase-retrieval`, `str-replace-editor`, ...), not Claude-style names. agnostic-ai's generic `tools` field never reaches them: an agent that sets it surfaces a coverage note. `x-augment: {tools: [...]}` and `x-augment: {disabled_tools: [...]}` (`x-augment.tools` / `x-augment.disabled_tools`) are the way to reach Augment's real per-tool access control.

Skills emit into the shared `.agents/skills/` tree, which Augment also scans directly alongside `.claude/skills/` and `.augment/skills/`. Set `outputs.augment.rules-file: .augment-guidelines` to additionally write the legacy concatenated document; the vendor's own precedence order truncates it first under budget pressure, which is why it stays opt-in rather than the default rules surface.

MCP servers merge into `<workspace>/.augment/settings.json` under `mcpServers` in the standard shape (stdio: `command`/`args`/`env`, no `type`; remote: `type: http|sse` plus `url`/`headers`): "Project settings (shared, per-project) ... Best for team-shared project configuration, such as shared MCP servers" (docs.augmentcode.com/cli/config, #633). This is Auggie CLI's own settings hierarchy, not the VS Code / JetBrains extension's Settings Panel, and the file also holds `shell`, `startupScript`, `theme`, plugin keys, and tool permissions, so the write merges in only the `mcpServers` key and leaves everything else untouched.

That holds for a JSONC file too: "The files support JSON with Comments (JSONC), allowing comments and trailing commas for better documentation" ([docs.augmentcode.com/cli/config](https://docs.augmentcode.com/cli/config)), so `sync` strips both before reading. Keys survive, comments do not, and the sync that drops them prints a one-line warning (target-audit 2026-09-11, #725). No per-server `disabled` key is documented, so a spec's `disabled: true` is stripped with a coverage note rather than written as a key Auggie would ignore. Use `auggie mcp remove` to drop the entry instead. The transport set is closed at those three names, "`-t, --transport <transport>` - stdio|sse|http (default: "stdio")" ([docs.augmentcode.com/cli/integrations](https://docs.augmentcode.com/cli/integrations)), and no Augment page names a WebSocket transport, so a `type: ws` spec emits no server and raises a coverage note (target-audit 2026-09-18, #855).

Hooks merge into that same `.augment/settings.json`, under a `hooks` key, in the same single write as `mcpServers` rather than a second `MergeJSONFile` call. A second call would re-read the pre-write file during sync's collision-capture pass and produce a second, divergent snapshot for the one path, tripping the collision check against this adapter's own two writes. Five events are supported: `PreToolUse`, `PostToolUse`, `Stop`, `SessionStart`, `SessionEnd` ([docs.augmentcode.com/cli/hooks](https://docs.augmentcode.com/cli/hooks)). `timeout` reaches the file in **milliseconds**; the shared hook spec's own `timeout` field is seconds, so this adapter multiplies by 1000 (vendor default 60000 when absent).

`command` must be a path ending in `.sh`, `.ps1`, `.cmd`, or `.bat`: "Path to the script to execute (must use a supported script extension: .ps1, .cmd, .bat, or .sh)". Unlike Claude Code, Codex, and Qoder, Augment never runs an inline shell string; a command missing one of the four extensions still emits verbatim (no guessed rename) but surfaces a coverage note. `matcher` is optional even on `PreToolUse`/`PostToolUse` (vendor default `.*`) and omitted entirely on the three session events, which the vendor documents as not using it at all. Augment's own PreToolUse/PostToolUse matcher vocabulary is its own tool names (`launch-process`, `str-replace-editor`, `save-file`, ...), the same set `x-augment.tools` already documents above, so a Claude-style matcher (`Bash`, `Write`, ...) parses and then matches nothing; that case surfaces a coverage note too.

Settings specs merge into that same file under `toolPermissions`, in the same one write: "`toolPermissions` is honored by the Auggie CLI and by Cosmos cloud agents", and "Committing a `.augment/settings.json` to your repository is the recommended way to enforce an organizational policy (for example, blocking `git merge`) on every cloud agent that runs there" ([docs.augmentcode.com/cli/permissions](https://docs.augmentcode.com/cli/permissions), #856).

The shape is an ordered array, and one rule's `permission` is an **object**: "`permission` must be an object with a `type` field, `{ "type": "deny" }`, not the bare string `"deny"`. A rule with a bare-string permission is malformed and is dropped." Order decides the outcome ("Rules are evaluated in order from top to bottom" and "The first matching rule determines the permission"), so deny rules emit ahead of allow rules. A bare tool name maps onto Augment's own six (`Read` to `read`, `Bash` to `terminal`, `WebFetch` to `web-fetch`, and so on), `Bash(prefix:*)` becomes a `terminal` rule with `shellInputRegex: ^prefix`, an exact `Bash(command)` anchors both ends, and `mcp__<server>__<tool>` becomes the documented `{tool-name}_{server-name}` spelling.

Three portable things reach nothing here, each with a coverage note rather than a guess. `model` has no documented key in this file. The `ask` list has no Augment permission type: the four are `allow`, `deny`, `webhook-policy`, and `script-policy`, and none of them prompts. And a path- or URL-scoped rule such as `Read(src/**)` has no Augment matcher, since only `terminal` takes one, so flattening it onto a bare `read` would widen it to every file on disk. Set `x-augment.toolPermissions` to write Augment's own rule objects; they pass through verbatim, ahead of the translated ones. Every other key under `x-augment` on a settings spec merges into the file as written, `shell` and `startupScript` among them.

Commands emit to `.augment/commands/<scope>/<name>.md`; a source-layout scope becomes a nested command namespace, while `description`, `argument-hint`, and `model` stay in frontmatter. Ignore specs emit to `.augmentignore`, and `import augment` restores a hand-authored file without changing pattern order or negation semantics.

## Config keys

| Key | Default | Notes |
|-----|---------|-------|
| `outputs.augment.rules-dir` | `.augment/rules` | |
| `outputs.augment.agents-dir` | `.augment/agents` | |
| `outputs.augment.skills-dir` | `.agents/skills` | |
| `outputs.augment.commands-dir` | `.augment/commands` | |
| `outputs.augment.ignore-file` | `.augmentignore` | |
| `outputs.augment.rules-file` | unset | opt-in, writes the legacy concatenated `.augment-guidelines` document |
| `outputs.augment.mcp-file` | `.augment/settings.json` | also the hooks and permissions file, since all three merge into the same document |

## Import

`import augment` reads the workspace indexing exclusions and the committed `toolPermissions` policy from `.augment/settings.json`, for the six tool names Augment publishes. An entry matching on `shellInputRegex` has no portable spelling and is skipped rather than guessed at, and so is one whose `permission` is a bare string, which Augment itself treats as malformed. Every other Augment surface stays out of import.

## Verify

1. Install the Augment Code extension ([guidelines docs](https://docs.augmentcode.com/setup-augment/guidelines)).
2. Check the tree: `ls AGENTS.md .augment/rules/ .augment/agents/ .agents/skills/`, plus `.augment-guidelines` when `outputs.augment.rules-file` is set and `.augment/settings.json` when MCP, hook, or settings specs are present.
3. Open the project; Augment reads `AGENTS.md`, `.augment/rules/`, `.augment/agents/`, `.agents/skills/`, and (via Auggie CLI) `.augment/settings.json` (and `.augment-guidelines` when present).
4. When hook specs exist, confirm each `.augment/settings.json` `hooks.<Event>` entry loads. `auggie` prints no "invalid hook" warning at startup, and a `PreToolUse` hook against a script ending in `.sh`/`.ps1`/`.cmd`/`.bat` actually runs on the matching tool call.
5. When settings specs exist, `auggie` prints no dropped-rule warning at startup (the vendor warns on a malformed rule), a tool covered by a `deny` rule is blocked, and one covered by `allow` runs without approval.

# Spec format

| Kind    | Source                                    | Format                      |
|---------|-------------------------------------------|-----------------------------|
| Agent   | `agents/*.md`                             | Markdown + YAML frontmatter |
| Skill   | `skills/*.md` or `skills/<name>/SKILL.md` | Markdown + YAML frontmatter |
| Rule    | `rules/*.md`                              | Markdown + YAML frontmatter |
| Hook    | `hooks/*.yaml`                            | YAML                        |
| MCP      | `mcps/*.yaml`                             | YAML                        |
| Command  | `commands/*.md`                           | Markdown + YAML frontmatter |
| Settings    | `settings/*.yaml`                      | YAML                        |
| Review      | `reviews/*.md`                         | Markdown + YAML frontmatter |
| Environment | `environments/*.yaml`                  | YAML                        |
| Ignore      | `ignore/*.md`                          | Markdown + YAML frontmatter |

Discovery is recursive. Every `.md` under `agents/`, `skills/`, `rules/`, `commands/`, `reviews/`, `ignore/` is picked up; every `.yaml` under `hooks/`, `mcps/`, `settings/`, and `environments/`.

## Nested layout: per-directory scope

A spec in a subdirectory of its source dir carries an implicit **scope** equal to that subpath.

```
rules/
├── conventional-commits.md      # scope: ""    (root)
├── backend/
│   └── auth.md                  # scope: "backend"
└── backend/api/
    └── limits.md                # scope: "backend/api"
```

Adapters that produce per-directory output honor the scope:

| Target          | Scoped layout                              |
|-----------------|--------------------------------------------|
| `claude`        | `.claude/rules/<scope>/<name>.md`          |
| `cursor`        | `.cursor/rules/<scope>/<name>.mdc`         |
| `cline`         | `.clinerules/<scope>/<name>.md`            |
| `windsurf`      | `.devin/rules/<scope>/<name>.md`           |
| `continue`      | `.continue/rules/<scope>/<name>.md`        |
| `junie`         | `.junie/rules/<scope>/<name>.md`           |
| `trae`          | `.trae/rules/<scope>/<name>.md`            |
| `antigravity`   | `.agent/rules/<scope>/<name>.md`           |
| `augment`       | `.augment/rules/<scope>/<name>.md`         |
| `kiro`          | `inclusion: fileMatch` + `fileMatchPattern: <scope>/**` on one flat `.kiro/steering/<name>.md` (no nested dirs) |
| `copilot`       | `<scope>/**` glob on one `.github/instructions/<name>.instructions.md` (no nested dirs) |

The scoped output nests inside the tool's rules directory (not a `<scope>/.cursor/...` tree at the repo root), so drift detection and the orphan sweep keep working. Single-document targets (`aider` CONVENTIONS.md) merge regardless of scope. Inline targets (`codex`, `gemini`, `amp`, `warp`, `zed`, `opencode`, `crush`, `jules`, `goose`, `openhands`, `factory`, `kilo`) carry rule bodies in their entry-point file and do not emit per-directory scoped files (e.g. `src/AGENTS.md`); augment also inlines into its entry-point file but, unlike the others in this group, additionally emits scoped `.augment/rules/<scope>/<name>.md` files (see the table above). The scope stays in the source provenance comment (`<!-- source: rules/backend/auth.md -->`).

A frontmatter `scope:` field is also accepted as a fallback when moving the file is impractical (a single rule scoped to a subtree).

## Agents

```markdown
---
name: code-reviewer
description: Reviews diffs for bugs, style, and architectural issues.
tools: [Read, Grep, Bash]
model: sonnet
---

You are a code reviewer. Examine the diff for:
- Logic bugs and edge cases
- Style consistency
- Security issues

Report concise findings with `file:line` references.
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | no | filename without `.md` | Agent identifier. Used for the output filename. |
| `description` | no | empty | One-liner shown in tool listings. |
| `tools` | no | unset | Tools the agent may invoke. Format depends on target CLI. |
| `model` | no | unset | Preferred model. String applies everywhere; a map selects per target (see below). |

Any other frontmatter field passes through to the target CLI unchanged.

### Per-target models

`model:` accepts a string (same model everywhere) or a map keyed by target name with an optional `default` fallback.

```yaml
---
name: code-reviewer
model:
  claude: opus
  codex: gpt-5.5
  default: gpt-4o
---
```

Resolution per target: matching key wins, else `default`, else no `model` line is emitted (the CLI uses its own built-in default). An `x-<target>.model` override beats the map; `x-<target>.model: null` deletes it.

| Want | Write |
|------|-------|
| Same model everywhere | `model: sonnet` |
| Per-target, with a concrete fallback | `model: {claude: sonnet, default: gpt-4o}` |
| Per-target, native default elsewhere | `model: {claude: sonnet}` |

## Skills

Two layouts:

**Flat:** `skills/yaml-validator.md`

**Nested** (for skills with attached resources):
```
skills/yaml-validator/SKILL.md
skills/yaml-validator/schema.yaml
```

Only `SKILL.md` and flat `skills/*.md` are parsed as skills. Every other file inside a nested skill directory (scripts, templates, fixtures, subdirectories, and extra `*.md` such as `examples.md`) is a bundled asset: it copies verbatim to the same relative location under each target's skills dir and is never promoted to its own skill. Ship a `check.mjs`, `templates/*.tpl`, or `fixtures/*.json` the skill body references. Executable bits are preserved both directions through import + sync.

```markdown
---
name: yaml-validator
description: Validate YAML against a schema.
---

# YAML Validator

## Steps
1. Read target file
2. Parse YAML
3. Compare against `schema.yaml`
4. Report violations as `path: message`
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | no | dir or filename | Skill identifier. Used for the output directory. |
| `description` | no | empty | One-liner shown when the model decides whether to invoke the skill. |

Emission by target:

- **Native** (`<dir>/<name>/SKILL.md`, one folder per skill): Claude Code (`.claude/skills/`), Codex + Amp (shared `.agents/skills/`), Cursor (`.cursor/skills/`), Antigravity (`.agent/skills/`).
- **As a rule file** (`skill-<name>.{mdc,md}`): Cursor, Cline, Windsurf, Continue.
- **Listed by default, opt into native slash commands** via `outputs.<target>.emit-skills-as-commands: true`: Gemini, OpenCode.
- **Listed in a `## Skills` section**: Aider, Copilot, Zed, Warp.

See [targets](targets.md) for the full matrix.

## Rules

```markdown
---
name: conventional-commits
description: Always use Conventional Commits format.
globs: "**/*"
alwaysApply: true
---

Use `feat:`, `fix:`, `docs:`, etc. Subject under 72 chars.
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | no | filename | Rule identifier. |
| `description` | no | empty | Short summary. |
| `globs` | no | `**/*` | File patterns where this rule applies. Used by Cursor. |
| `alwaysApply` | no | `true` for rules, `false` for agents emitted as Cursor rules | Inject unconditionally, or only on matching context. |

## Hooks

Pure YAML, no markdown body.

```yaml
name: format-on-save
description: Run formatter after Edit/Write tools modify files.
event: PostToolUse
matcher: "Edit|Write"
command: "npx prettier --write \"$CLAUDE_FILE_PATHS\""
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | no | filename | Hook identifier. |
| `description` | no | empty | Free-form documentation. |
| `event` | yes | none | Hook event. See list below. |
| `matcher` | no | empty | Regex on tool name (or other event-specific selector). |
| `command` | yes | none | Shell command to run when triggered. |
| `timeout` | no | none | Seconds before the tool cancels the hook. Claude + Codex. |
| `statusMessage` | no | empty | Spinner message while the hook runs. Claude + Codex. |
| `async` | no | `false` | Run in the background without blocking. Claude. |
| `asyncRewake` | no | `false` | Background run that wakes Claude on exit code 2 (implies `async`). Claude. |
| `shell` | no | empty | `bash` or `powershell`. Claude. |
| `if` | no | empty | Permission-rule filter (e.g. `Bash(git *)`) gating when the hook fires. Claude. |
| `commandWindows` | no | empty | Windows-specific command override. Codex. |
| `target` | no | empty | Single target name. Emits only there. |
| `targets` | no | empty | List of target names. Emits only to those. |
| `target-exclude` | no | empty | Single target name to block. Emits everywhere else. |
| `targets-exclude` | no | empty | List of target names to block. Emits to every other configured target. |

Tool-specific fields emit only where that tool's schema defines them; other targets ignore them.

With none of the scoping fields set, the hook emits to every target that supports hooks. `target` takes precedence over `targets` when both appear. Exclude wins: a target in both an include and an exclude list is excluded.

These four scoping fields work on every spec kind (agents, skills, rules, commands, mcps), not only hooks. A `target: codex` agent emits only into `.codex/agents/`; a `targets-exclude: [gemini]` skill emits to every configured target except gemini.

### Per-target body fences

When a spec emits to multiple targets but the prose must diverge (codex wants a "Workflow" section claude does not), wrap the divergent prose in `::target` fences. Outside-fence content emits everywhere; inside-fence content emits only to the listed targets. Marker lines never reach the output.

```md
---
name: test
description: Run the test suite
---

# Test

Shared intro paragraph.

::target claude
## Scope mapping (Claude)

| Changed | Command |
|---------|---------|
| ... | ... |
::end

::target codex
## Workflow (Codex)

1. Choose scope
2. Run `composer test`
::end

Shared outro.
```

- `::target <name>` and `::targets <a> <b>` open a fence pinned to one or more targets.
- `::end` closes the most recent fence.
- An unterminated fence runs to end-of-body, so a missing `::end` keeps the tail of the file.
- The empty target (the source view used by `import` round-trips) returns the body with fences intact, so a re-emit stays byte-stable.
- `import codex` builds these fences automatically when both tools ship the same agent or skill name with diverging bodies: the longest common prefix and suffix stay un-fenced, and each tool's unique middle gets its own `::target` block.

### Import auto-scoping

Hooks imported from a tool-native source auto-set `target` to that tool (codex import → `target: codex`, claude → `target: claude`, gemini → `target: gemini`). Remove the field by hand to let the hook flow everywhere.

```yaml
event: PostToolUse
matcher: apply_patch|Edit|Write
command: "$(git rev-parse --show-toplevel)/.codex/hooks/format-php.sh"
target: codex   # shell-expanded codex path; do not leak to other tools
```

`import claude` and `import codex` apply the same auto-scoping to agents and skills: when both `.claude/` and `.codex/` exist but only one carries a given spec, the captured frontmatter gains `target: <tool>`. A spec present in both tools stays un-scoped (cross-emit). Pure single-tool projects also stay un-scoped, so byte-identical round-trips hold.

### Supported events (Claude Code)

| Event | When it fires |
|-------|---------------|
| `PreToolUse` | Before any tool call. Matcher is the tool name regex. |
| `PostToolUse` | After any tool call. Matcher is the tool name regex. |
| `PostToolUseFailure` | After a tool call fails. |
| `PermissionRequest` | When a permission dialog appears. |
| `UserPromptSubmit` | Before the model reads a new user message. |
| `SubagentStart` / `SubagentStop` | When a subagent spawns / finishes. |
| `Stop` | When the model stops generating. |
| `Notification` | When Claude Code surfaces a system notification. |
| `SessionStart` / `SessionEnd` | When a session begins / terminates. |
| `PreCompact` / `PostCompact` | Around context compaction. |

Claude Code defines more events (`Setup`, `InstructionsLoaded`, `TaskCompleted`, `TeammateIdle`, `FileChanged`, ...); the `event:` value passes through verbatim, so any documented name works. See the [Claude Code hooks reference](https://code.claude.com/docs/en/hooks) for the full list. Codex shares the `SessionStart`/`SubagentStart`/`UserPromptSubmit`/`PreToolUse`/`PermissionRequest`/`PostToolUse`/`PreCompact`/`PostCompact`/`Stop`/`SubagentStop` vocabulary.

Native emission: Claude Code (`.claude/settings.json`), Codex (`.codex/hooks.json`, per-event arrays), Gemini (`.gemini/settings.json` `hooks`), Cursor (`.cursor/hooks.json`, `version` + per-event arrays). Other targets log a warning and skip. Event names pass through verbatim, so a Cursor hook sets `event:` to a Cursor name (`beforeShellExecution`, `afterFileEdit`, `beforeSubmitPrompt`, `sessionStart`, `stop`, ...). See each tool's docs for its full event list and matcher semantics.

agnostic-ai emits the `event:` value **verbatim** into each target's schema; it does not translate event names between tools. Claude and Codex share the `PreToolUse` / `PostToolUse` / `UserPromptSubmit` vocabulary, so one hook spec feeds both. Gemini uses its own names (`BeforeTool`, `AfterTool`, `BeforeAgent`, `AfterAgent`, `Notification`, `SessionStart`, `SessionEnd`, `PreCompress`, `BeforeModel`, `AfterModel`, `BeforeToolSelection`), so a Gemini hook must set `event:` to one of those. `agnostic-ai validate` flags any event a target does not recognize.

### Per-target rendering

This spec:

```yaml
event: PostToolUse
matcher: Bash(git commit*)
command: echo "tests please"
```

Renders to `.claude/settings.json`:

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Bash(git commit*)",
        "hooks": [
          {"type": "command", "command": "echo \"tests please\""}
        ]
      }
    ]
  }
}
```

Renders to `.codex/hooks.json` (same nested shape as Claude, routed into per-event arrays):

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Bash(git commit*)",
        "hooks": [
          {"type": "command", "command": "echo \"tests please\""}
        ]
      }
    ]
  }
}
```

Gemini uses different event names, so a Gemini hook sets `event:` to one of its own. This spec:

```yaml
event: AfterTool
matcher: Bash(git commit*)
command: echo "tests please"
```

Renders to `.gemini/settings.json` (flat shape, event name passed through unchanged):

```json
{
  "hooks": {
    "AfterTool": [
      {"matcher": "Bash(git commit*)", "command": "echo \"tests please\""}
    ]
  }
}
```

When `command` is a list, each entry becomes a separate hook entry (Claude, Codex).

## MCP servers

Pure YAML, no markdown body. One file per server.

```yaml
name: filesystem
description: Local filesystem access for the model.
type: stdio
command: npx
args:
  - -y
  - "@modelcontextprotocol/server-filesystem"
env:
  ROOT: /tmp
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | yes | none | Server identifier. Becomes the key in the generated config. |
| `description` | no | empty | Free-form documentation. |
| `type` | no | `stdio` | Transport: `stdio`, `http`, `sse`, or `ws`. Remote transports (`http`, `sse`, `ws`) emit an explicit `type`; `stdio` stays type-less since it is the inferred default. |
| `command` | stdio only | none | Executable to launch. |
| `args` | no | empty | Argument list for the command. |
| `env` | no | empty | Environment variables passed to the server. |
| `url` | http/sse only | none | Endpoint URL. |
| `headers` | no | empty | HTTP headers for `http`/`sse` transports. |
| `disabled` | no | `false` | Support varies by target; see [`disabled` support by target](#disabled-support-by-target) below. |
| `roots` | no | empty | List of `{uri, name}` objects. Passed to targets that support MCP roots (Claude Code, Cursor, Copilot). |

Targets with native MCP propagation:

| Target | File | Schema |
|--------|------|--------|
| Claude Code | `.mcp.json` | standard `mcpServers` |
| Cursor | `.cursor/mcp.json` | standard `mcpServers` |
| Copilot / VS Code | `.vscode/mcp.json` | `servers` with `type` field |
| Codex | `.codex/config.toml` | `[mcp_servers.<name>]` table |
| Gemini | `.gemini/settings.json` | `mcpServers` (uses `httpUrl` for HTTP) |
| Continue | `.continue/mcpServers/<name>.yaml` | one YAML per server |
| Amp | `.amp/settings.json` | `amp.mcpServers` (dotted key) |
| Zed | `.zed/settings.json` | `context_servers` (stdio: `command`/`args`/`env`; HTTP/SSE: native `url`/`headers`) |
| Warp | `.warp/.mcp.json` | standard `mcpServers` |
| OpenCode | `opencode.json` | `mcp` with `type: local\|remote` |
| Antigravity | `.agents/mcp_config.json` | `mcpServers` (remote uses `serverUrl`, not `url`) |
| Factory | `.factory/mcp.json` | standard `mcpServers` |
| Qoder | `.mcp.json` | standard `mcpServers` (same file Claude Code writes; the two dedupe when both are enabled) |
| OpenHands | `config.toml` | `[mcp]` table with `stdio_servers` / `sse_servers` / `shttp_servers` arrays, no `type` field |

Aider, Cline, and Windsurf have no project-scoped MCP file and skip with a warning.

### `disabled` support by target

Confirmed per target, never generalized: a target not listed here has not been checked.

| Target | Behavior |
|--------|----------|
| Codex | Maps to `enabled = false` in `.codex/config.toml`. |
| Factory | Native `disabled` boolean in `.factory/mcp.json` (default `false`); passes through unchanged. |
| Kilo Code | Maps to `"enabled": false` in `kilo.jsonc`; an enabled server gets no key at all. |
| Claude Code, Cursor, Copilot | No file-based way to pre-disable a project-scoped MCP server; the field has no effect and agnostic-ai does not emit it. Disable the server from the target's own UI instead. |
| Qoder | Not vendor-confirmed either way. `.mcp.json` is the same file Claude Code reads, so agnostic-ai drops the field there too rather than risk the two targets disagreeing on one shared file. |

## Commands

Markdown with optional YAML frontmatter. Each spec becomes one native slash command on supported targets.

```markdown
---
name: deploy
description: Deploy the app to staging.
argument-hint: <env>
---

Deploy the app to {{env}}.

1. Run tests.
2. Build artifacts.
3. Push to the {{env}} environment.
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | no | filename | Command identifier. Becomes the slash name (e.g. `/deploy`). |
| `description` | no | empty | One-liner shown in slash-command pickers. |
| `argument-hint` | no | empty | Hint string shown after the command. Claude-only; passes through. |

Any other frontmatter passes through unchanged. Use the `x-<target>` namespace for target-specific keys (e.g. `x-claude.allowed-tools`).

Native emission: Claude Code (`.claude/commands/<name>.md`), Cursor (`.cursor/commands/<name>.md`), Gemini (`.gemini/commands/<name>.toml`), OpenCode (`.opencode/commands/<name>.md`), and Amp (`.agents/commands/<name>.md`). Codex deprecated project prompts, so its commands emit only when `outputs.codex.commands-dir` is set; otherwise `sync` prints a coverage note. Other targets log a warning and skip.

## Settings

Pure YAML, one file per settings group under `settings/`. A tool-neutral place to single-source agent permissions and the default model instead of hand-editing each tool's settings file.

```yaml
permissions:
  allow:
    - Bash(go test:*)
  deny:
    - Bash(rm:*)
  ask:
    - Bash(git push:*)
model: claude-opus-4-8
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `permissions.allow` | no | empty | Permission rules auto-approved without prompting. |
| `permissions.deny` | no | empty | Permission rules always blocked. |
| `permissions.ask` | no | empty | Permission rules that prompt before running. |
| `model` | no | empty | Default model the tool should use. |

Multiple settings files merge: permission lists concatenate (de-duped, source order preserved); the last non-empty `model` wins.

Native emission: Claude Code (`.claude/settings.json` `permissions` + `model`). Other targets have no equivalent settings surface and report the spec as unsupported.

When the Claude-specific `outputs.claude.settings` config in `agnostic-ai.yaml` also sets these fields, scalars like `model` take the config value (it is the more specific source), while `permissions` lists are unioned across the captured overlay, the settings spec, and the config so no layer silently drops another's allow/deny/ask rules.

`settings` specs are hand-authored: `import claude` does not create them. An imported `.claude/settings.json` keeps its `permissions` and `model` in the Claude-only overlay (`.agnostic-ai/overlays/claude.settings.json`), since a value like a Claude model id does not necessarily port to other tools. Author a `settings` spec when you want one source to drive multiple tools; a settings spec's `permissions` still union with the imported overlay on sync.

## Reviews

Markdown with optional YAML frontmatter, one file per guidance group under `reviews/`. A single source for code-review-bot guidance that maps to each ecosystem's native review-rule file.

```markdown
---
scope: backend
---

Flag any handler that talks to the database directly instead of going through a repository.
```

Review specs honor `scope` (and the source-directory layout) exactly like rules, so per-directory guidance is supported. Specs that share a scope concatenate into that scope's single review file.

Native emission: Cursor [Bugbot](https://docs.cursor.com/bugbot) `BUGBOT.md`: the repo root for unscoped specs, `<scope>/BUGBOT.md` for scoped ones. Override the basename with `outputs.cursor.review-file`. Other targets have no equivalent review-rule file yet and report the spec as unsupported.

## Environments

Pure YAML, one file per environment group under `environments/`. A single source for how a coding agent boots its dev env (install dependencies, start services, forward ports, open terminals).

```yaml
install: go mod download
terminals:
  - name: dev
    command: go run ./cmd/agnostic-ai
```

Native emission: Cursor background-agent [environment.json](https://docs.cursor.com/background-agent). The spec body is the `.cursor/environment.json` content: every key except the agnostic-ai routing fields (`name`, `scope`, `target(s)`, `target(s)-exclude`, `description`) passes through verbatim, so you author Cursor's schema while agnostic-ai single-sources it. Multiple environment specs merge by top-level key (last wins). Override the path with `outputs.cursor.environment-file`. Other targets (devcontainers, Codex setup scripts) have no emitter yet and report the spec as unsupported.

## Ignore

Markdown with optional YAML frontmatter, one file per group under `ignore/`. The body is gitignore-syntax patterns naming what an agent must not read or index. One source fans out to each tool's native ignore file.

```markdown
# Secrets and build artifacts the agent should never read
*.env
secrets/
dist/
```

Native emission (gitignore syntax, under a `#` provenance header): Cursor `.cursorignore`, Gemini `.aiexclude`, Aider `.aiderignore`. Each spec body is trimmed and the specs are concatenated with a blank line between them. Each path is overridable via `outputs.<target>.ignore-file`. Targets without an ignore-file convention report the spec as unsupported.

## Frontmatter rules

- YAML between two `---` lines at the top of the file.
- Empty (`---\n---\n`) is allowed and treated as no metadata.
- Files without frontmatter still load; name defaults to the filename.
- Malformed frontmatter is treated as no metadata; the entire content becomes the body.
- Any field not listed above passes through on emit. Useful for target-specific extensions.

## Target-specific extensions: `x-<target>` namespace

Use `x-<target>:` blocks to attach fields only one adapter consumes. Other adapters strip the block on emit, so the spec stays portable.

```markdown
---
name: code-reviewer
description: Reviews diffs.
model: sonnet
x-claude:
  allowed-tools: [Read, Grep, Bash]
x-cursor:
  globs: "src/**"
  alwaysApply: false
---
```

Resolution per target:

- All `x-*` keys are dropped first.
- The matching `x-<target>` block is flattened into top-level meta.
- Flattened keys override top-level keys with the same name.

| Target | Resulting frontmatter |
|--------|-----------------------|
| `claude` | `name`, `description`, `model`, `allowed-tools` |
| `cursor` | `name`, `description`, `model`, `globs`, `alwaysApply` |
| `gemini` | `description` (`name` becomes the `.toml` filename; `model` has no native command surface and is not emitted) |

For Codex agents, `x-codex` fields (`model`, `model_reasoning_effort`, `sandbox_mode`, `nickname_candidates`) pass through to the generated `.codex/agents/<name>.toml`. For Codex skills, `x-codex.interface`, `x-codex.policy`, and `x-codex.dependencies` trigger an additional `.agents/skills/<name>/agents/openai.yaml` for UI customization, policy, and tool dependencies.

### Arbitrary custom keys

Any other key under `x-<target>` emits verbatim into that target's output surface. Declaring it under `x-<target>` is the opt-in: shared top-level keys stay stripped, so plain specs keep emitting valid files. Keys emit in sorted order and never leak across targets. Validate them against the target's schema yourself.

Per surface:

| Target | Surface | Custom key lands in |
|--------|---------|---------------------|
| `claude` | `SKILL.md` frontmatter | every `x-claude` key (e.g. `disable-model-invocation: true`) |
| `codex` | `SKILL.md` frontmatter | every `x-codex` key except `interface`/`policy`/`dependencies` (those route to `openai.yaml`) |
| `amp`, `zed`, `crush`, `gemini`, `opencode`, `copilot` | `SKILL.md` frontmatter (shared renderer) | every `x-<target>` key beyond `name`/`description` |
| `cursor` | `SKILL.md` frontmatter | every `x-cursor` key beyond `name`/`description`/`paths`/`disable-model-invocation`/`metadata` |
| `cursor` | agent `.md` frontmatter | every `x-cursor` key beyond `name`/`description`/`model`/`readonly`/`is_background` |
| `copilot` | rule `.instructions.md` frontmatter | every `x-copilot` key, alongside `applyTo` |
| `copilot` | `.agent.md` frontmatter | every `x-copilot` key beyond `name`/`description`/`tools`/`model` |
| `opencode` | agent `.md` frontmatter | every `x-opencode` key beyond `description`/`mode`/`model`/`temperature`/`permission` |
| `opencode` | command `.md` frontmatter | every `x-opencode` key beyond `description`/`agent`/`model`/`subtask` |
| `gemini` | command `.toml` | every `x-gemini` key (string, bool, number, or string array) |
| `kiro` | agent `.md` frontmatter | every `x-kiro` key beyond `description`/`model` (`name` is excluded: Kiro's agent schema has none, identity comes from the filename) |

Targets that emit no surface for a spec kind drop arbitrary custom keys (kiro rule and skill steering files carry no passthrough; kiro agents do, see above). Gemini TOML accepts scalars and string arrays only; nested tables are skipped.

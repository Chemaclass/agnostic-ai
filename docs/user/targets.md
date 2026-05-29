# Targets

Each adapter emits in its tool's native format — separate files where the tool supports them, a merged document otherwise. Unsupported features (e.g. hooks for a non-hook-aware target) skip with a warning by default. Override via `on-unsupported` in [configuration](configuration.md).

## Entry-point files

`sync` writes `.agnostic-ai/AGNOSTIC_AI.md` plus a conventional root entry-point file for every enabled target. All entry-point files share the same canonical pointer body, so editing one tool's entry-point in place is a no-op (it gets overwritten on the next sync) and switching tools never surfaces inconsistent project conventions.

| Target          | Entry-point file               |
|-----------------|--------------------------------|
| **claude**      | `CLAUDE.md`                    |
| **codex**       | `AGENTS.md`                    |
| **amp**         | `AGENTS.md`                    |
| **warp**        | `AGENTS.md`                    |
| **gemini**      | `GEMINI.md`                    |
| **aider**       | `CONVENTIONS.md`               |
| **copilot**     | `.github/copilot-instructions.md` |
| **opencode**    | `.opencode/AGENTS.md`          |
| **antigravity** | `.agent/AGENTS.md`             |

Targets that share an entry-point path (codex + amp + warp at `AGENTS.md`) write the file once; the dedup is automatic. Targets without a row above (cursor, cline, windsurf, continue, zed) emit only per-file artifacts under their own directory and have no project-root entry-point.

Set `outputs.<target>.rules-file: <path>` to opt back into the legacy concatenated rules layout for that target. The adapter then writes a single merged document at `<path>` and `sync` skips the pointer-body write for that target so the two do not collide.

## Capability matrix

| Target          | Agents              | Skills | Rules                    | Hooks | MCPs | Commands |
|-----------------|---------------------|--------|--------------------------|-------|------|----------|
| **claude**      | `.claude/agents/`   | `.claude/skills/` | `.claude/rules/*.md`   | `.claude/settings.json` | `.mcp.json` | `.claude/commands/<name>.md` |
| **codex**       | `.codex/agents/*.toml` | `.codex/skills/<name>/SKILL.md` | source-dir only (legacy concat via `outputs.codex.rules-file`) | `.codex/hooks.json` (per-event arrays) | `.codex/config.toml` (`[mcp_servers.<name>]`) | `.codex/prompts/<name>.md` |
| **gemini**      | `.gemini/commands/<name>.toml` | `.gemini/commands/skill-<name>.toml` w/ opt-in | source-dir only (legacy concat via `outputs.gemini.rules-file`) | `.gemini/settings.json` (`hooks`) | `.gemini/settings.json` (`mcpServers`) |
| **cursor**      | as `.mdc` (alwaysApply: false) | as `.mdc` (`skill-<name>.mdc`) | `.cursor/rules/*.mdc` | - | `.cursor/mcp.json` |
| **copilot**     | `.github/instructions/agent-<name>.instructions.md` | `.github/instructions/skill-<name>.instructions.md` | `.github/instructions/<name>.instructions.md` (scoped); always-on via `outputs.copilot.rules-file` | - | `.vscode/mcp.json` |
| **aider**       | source-dir only (legacy merge via `outputs.aider.rules-file`) | source-dir only | source-dir only (`.aider.conf.yml` w/ opt-in) | - | - |
| **cline**       | as `.md` rule (+ `.clinerules/workflows/<name>.md` w/ opt-in) | as `.md` (`skill-<name>.md`) | `.clinerules/*.md`       | -     | - |
| **windsurf**    | as `.md` rule (+ `.windsurf/workflows/<name>.md` w/ opt-in) | as `.md` (`skill-<name>.md`) | `.windsurf/rules/*.md`   | -     | - |
| **continue**    | as `.md` rule (+ `.continue/assistants/<name>.yaml` w/ opt-in) | as `.md` (`skill-<name>.md`) | `.continue/rules/*.md`   | -     | `.continue/mcpServers/*.yaml` |
| **amp**         | `.agents/commands/<name>.md` | `.agents/commands/skill-<name>.md` w/ opt-in | source-dir only (legacy concat via `outputs.amp.rules-file`) | - | `.amp/settings.json` (`amp.mcpServers`) |
| **zed**         | merged in `.rules` | listed in `.rules` | `.rules` | `.zed/tasks.json` w/ opt-in | `.zed/settings.json` (`context_servers`) |
| **warp**        | `.warp/workflows/<name>.yaml` w/ opt-in | source-dir only | source-dir only (legacy concat via `outputs.warp.rules-file`) | - | `.warp/.mcp.json` |
| **opencode**    | `.opencode/commands/<name>.md` | `.opencode/commands/skill-<name>.md` w/ opt-in | source-dir only (legacy concat via `outputs.opencode.rules-file`) | - | `opencode.json` (`mcp`) |
| **antigravity** | as `.md` rule (`agent-<name>.md`) | - | `.agent/rules/*.md` (legacy merge via `outputs.antigravity.rules-file`) | - | - |

Skills emitted to non-Claude targets are reference material. Only Claude
Code has native skill execution. For all other targets, the agent or
human reads the skill file and follows its instructions.

Hooks run as shell commands on lifecycle events (e.g. `PreToolUse`,
`PostToolUse`, `SessionStart`). Emitted natively by Claude Code, Codex,
and Gemini in each tool's own schema. Zed picks them up too via the
opt-in `outputs.zed.tasks-file`, but as on-demand tasks rather than
event-triggered hooks. Other targets have no equivalent concept and
skip with a warning.

MCP servers propagate to every target that has a project-scoped MCP file
(10 of 14 — see the matrix above). Aider, Cline, Windsurf, and Antigravity
have no project-scoped MCP surface and skip with a warning.

Commands are slash-prompt files authored under `commands/` and emitted to the
target's native slash-command surface. Supported on Claude Code
(`.claude/commands/<name>.md`) and Codex (`.codex/prompts/<name>.md`). Other
targets skip with a warning.

## Per-target output

### Claude Code (`claude`)

```
CLAUDE.md                # canonical entry-point pointer body (written by sync)
.claude/
├── agents/<name>.md
├── skills/<name>/SKILL.md
├── rules/<name>.md
├── commands/<name>.md
└── settings.json
.mcp.json
```

Rules emit one file per spec under `.claude/rules/`. `CLAUDE.md` is owned by `sync` and overwritten on every run with the canonical pointer body. Reference per-rule files from `CLAUDE.md` with `@.claude/rules/<name>.md` imports — those imports survive across syncs because the pointer body lists `.claude/rules/` as the rules directory. Set `outputs.claude.rules-file: CLAUDE.md` to fall back to the legacy concatenated layout; `sync` then skips the pointer-body write for `claude`.

Commands emit one file per spec under `.claude/commands/`. Each command becomes a Claude Code slash command (e.g. a spec named `deploy` is invoked as `/deploy`). Frontmatter passes through; the body is the prompt template.

`agnostic-ai import claude` captures the non-`hooks` portion of `.claude/settings.json` (statusLine, enabledPlugins, any other top-level key) into `.agnostic-ai/overlays/claude.settings.json`. `sync -t claude` reads that overlay and layers the spec-derived `hooks` key on top, so a re-sync from a fresh checkout reproduces the full settings.json even after `.claude/` has been wiped. Re-run `import claude` whenever you add a key to settings.json by hand.

`import claude` also reads `.mcp.json` and writes one spec under `<mcps>/<name>.yaml` per `mcpServers.<name>` entry. The next `sync` then distributes those MCP servers to every other target that supports them (codex, copilot, cursor, continue, amp, zed, warp, gemini, opencode).

`outputs.claude.settings.*` declares first-class settings keys (model, outputStyle, statusLine, permissions, enabledPlugins, env, apiKeyHelper, cleanupPeriodDays, includeCoAuthoredBy). They merge on top of the captured overlay and below the spec-derived hooks block. See [Claude settings](configuration.md#claude-settings).

Config keys: `outputs.claude.dir` (default `.claude`), `outputs.claude.rules-dir` (default `.claude/rules`), `outputs.claude.rules-file` (unset; setting it switches back to the legacy concatenated single-file layout, typically `CLAUDE.md`), `outputs.claude.commands-dir` (default `.claude/commands`), `outputs.claude.mcp-file` (default `.mcp.json`), `outputs.claude.settings` (first-class settings block).

#### Verifying with the real Claude Code CLI

1. Install Claude Code: `npm install -g @anthropic-ai/claude-code` (or download the desktop app — both read the same project files).
2. Sanity-check the emitted tree:

   ```bash
   ls CLAUDE.md .claude/agents/ .claude/skills/ .claude/rules/ .claude/commands/ .claude/settings.json .mcp.json
   head -1 .claude/agents/*.md .claude/rules/*.md .claude/commands/*.md  # provenance header on each
   ```

3. Validate the JSON syntactically: `python -m json.tool .claude/settings.json > /dev/null && python -m json.tool .mcp.json > /dev/null`.
4. Launch Claude Code in the project (`claude` from the project root) and confirm:
   - `/agents` lists every entry under `.claude/agents/`.
   - `/skills` lists every skill folder under `.claude/skills/`.
   - The slash-command picker exposes every entry under `.claude/commands/` as `/<name>`.
   - The MCP picker shows each `mcpServers.<name>` entry from `.mcp.json` with a green status.
5. Trigger a hook by performing one of the matcher actions (e.g. an `Edit` for `PostToolUse`/`Edit`) and confirm the configured command runs without "schema mismatch" errors in the Claude Code log.
6. Confirm `outputs.claude.settings.*` keys (model, statusLine, etc.) are honored by inspecting them under `/config` from inside Claude Code.

### Codex (`codex`)

```
AGENTS.md                                    # canonical entry-point pointer body (written by sync)
.codex/agents/<name>.toml                    # one TOML per agent (Codex CLI's native path)
.codex/skills/<name>/SKILL.md                # one folder per skill (Codex CLI's native path)
.codex/skills/<name>/agents/openai.yaml      # optional, when x-codex provides UI/policy/deps
.codex/prompts/<name>.md                     # one per command (slash prompt)
.codex/config.toml                           # when hook and/or MCP entries exist
```

Config keys: `outputs.codex.agents-dir` (default `.codex/agents` — Codex CLI's native lookup path; override to `.agents/agents` for the community shared layout), `outputs.codex.skills-dir` (default `.codex/skills` — Codex CLI's native lookup path; override to `.agents/skills` for the community shared layout), `outputs.codex.shared-subagents` (default `false` when `claude` is also in `targets` to avoid duplicating `.claude/skills/<name>/`, `true` when codex is alone; set explicitly to override), `outputs.codex.commands-dir` (default `.codex/prompts`), `outputs.codex.mcp-file` (default `.codex/config.toml` — also holds hooks), `outputs.codex.rules-file` (unset; setting it writes a legacy concatenated rules document at that path and `sync` skips the pointer-body write for `codex`).

The project-root `AGENTS.md` is written by `sync` with the canonical pointer body. Codex still loads rules from the spec source directory referenced in that pointer; per-directory `AGENTS.md` scoping (e.g. `src/AGENTS.md` from `globs: src/**`) is no longer emitted by default. Use `outputs.codex.rules-file: AGENTS.md` if you need the legacy concatenated single-file layout back.

Skills follow the [Codex skills layout](https://developers.openai.com/codex/skills): one folder per skill under `.agents/skills/<name>/` with a required `SKILL.md` (frontmatter `name` + `description`, plus the body). When the spec carries `x-codex.interface`, `x-codex.policy`, or `x-codex.dependencies`, an additional `agents/openai.yaml` is written in the skill folder for UI customization and policy declarations.

Hooks and MCP servers both land in `.codex/config.toml`. Hook specs route by their `event` frontmatter (e.g. `SessionStart`, `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `PreCompact`, `PostCompact`) into `[[hooks.<event>]]` array-of-tables entries with `matcher` and `command`. MCP servers emit as `[mcp_servers.<name>]` tables — stdio uses `command`/`args`/`env`; HTTP/SSE uses `url`/`bearer_token_env_var`/`http_headers`. The project-tier config.toml is agnostic-ai-managed (overwritten on each sync); add unmanaged Codex config to `~/.codex/config.toml` user-global instead.

Commands emit one file per spec under `.codex/prompts/` (project-tier mirror of `~/.codex/prompts/`). Each becomes a Codex slash prompt. Frontmatter passes through; the body is the prompt template.

`agnostic-ai import codex` captures `.codex/config.toml` minus the `hooks` and `mcp_servers` keys (which round-trip through specs) into `.agnostic-ai/overlays/codex.config.toml`. `sync -t codex` prepends that overlay before the spec-derived hook and MCP sections, so `model`, `sandbox`, `approval_policy`, `notify`, `[history]`, `[profiles.*]`, `[model_providers.*]`, and any other Codex key survive a wipe of `.codex/` between import and sync. On conflict with `outputs.codex.config.*` the overlay wins and the first-class key is dropped to keep TOML valid. `import codex` also reads `.codex/prompts/*.md` and writes them byte-for-byte to `<commands>/`, so user-authored slash prompts round-trip instead of being silently overwritten on the next sync.

#### Verifying with the real Codex CLI

Run this smoke after a `sync -t codex` (or before opening a release) to confirm Codex picks up every emitted artifact end to end. Replace the install line with whatever package manager your environment uses.

1. **Install Codex CLI.** `npm install -g @openai/codex` (or follow the [official quickstart](https://developers.openai.com/codex/cli)). Then `codex --version` to confirm it is on PATH.
2. **Sanity-check the emitted tree.** From the project root:

   ```bash
   agnostic-ai sync -t codex
   ls .codex/agents/ .codex/skills/ .codex/prompts/
   test -f .codex/config.toml && head -1 .codex/config.toml
   test -f .codex/hooks.json && jq '.hooks | keys' .codex/hooks.json
   ```

   Every spec kind you authored under `.agnostic-ai/` should have a corresponding file. The first line of `config.toml` must be the `# Generated by agnostic-ai. ...` provenance comment.

3. **Validate the TOML + JSON syntactically.** `toml-test .codex/config.toml` (or any TOML linter) and `jq empty .codex/hooks.json` should both exit `0`. A non-zero exit means the emitter wrote something Codex CLI will refuse to load.
4. **Run a one-off Codex command in this directory.** `codex run "list one rule from this project"`. Codex should pick up the project-tier `AGENTS.md`, the agents under `.codex/agents/`, and the skill folders without warning. Look for `loaded N agents` / `loaded N skills` in the Codex log.
5. **Trigger a hook.** Issue a command that fires whichever `event` your hook specs target (e.g. an `Edit` to trigger a `PostToolUse` hook). The hook's `command` should appear in Codex's hook log.
6. **List MCP servers.** `codex mcp list` should show every `[mcp_servers.<name>]` table from `.codex/config.toml`. Disabled servers must appear with the disabled flag.

If any step fails, file a bug against the codex adapter and link the failing command output. The audit issue [#329](https://github.com/Chemaclass/agnostic-ai/issues/329) tracks the smoke checklist; close its "Real CLI smoke" box only after every step above has passed against the live Codex CLI build documented in the linked PR.

### Gemini CLI (`gemini`)

```
GEMINI.md                              # canonical entry-point pointer body (written by sync)
.gemini/commands/<name>.toml           # one per agent
.gemini/commands/skill-<name>.toml     # one per skill, only when emit-skills-as-commands: true
.gemini/settings.json                  # when MCP and/or hook entries exist (merged with any existing user config)
```

Config keys: `outputs.gemini.commands-dir` (default `.gemini/commands`), `outputs.gemini.mcp-file` (default `.gemini/settings.json` — also holds hooks), `outputs.gemini.emit-skills-as-commands` (default `false`), `outputs.gemini.rules-file` (unset; setting it writes a legacy concatenated rules document at that path and `sync` skips the pointer-body write for `gemini`).

The project-root `GEMINI.md` is written by `sync` with the canonical pointer body. Per-directory scoping (e.g. `src/GEMINI.md` from `globs: src/**`) is no longer emitted by default. Use `outputs.gemini.rules-file: GEMINI.md` for the legacy concatenated single-file layout.

Agents emit as TOML slash commands under `.gemini/commands/` per [Gemini CLI custom commands](https://geminicli.com/docs/cli/custom-commands/). Skills default to source-only; flip `outputs.gemini.emit-skills-as-commands: true` to emit one `.gemini/commands/skill-<name>.toml` per skill.

When MCP and/or hook entries are present, the adapter writes (or updates) `.gemini/settings.json` with the `mcpServers` map and the `hooks` map. Gemini uses `httpUrl` (not `url`) for HTTP MCP servers — the adapter handles the rename automatically. Hook specs route by their `event` frontmatter (e.g. `BeforeTool`, `AfterTool`, `SessionStart`); each event entry is `{matcher, command}`. Pre-existing user-managed keys in `.gemini/settings.json` survive across syncs.

### Cursor (`cursor`)

```
.cursor/rules/<name>.mdc
.cursor/commands/<name>.md           # one per agent, only when commands-dir is set
```

Config keys: `outputs.cursor.rules-dir` (default `.cursor/rules`), `outputs.cursor.commands-dir` (default empty — opt-in), `outputs.cursor.mcp-file` (default `.cursor/mcp.json`).

Rules emit with `alwaysApply: true`; agents as rules with `alwaysApply: false`. Override in spec frontmatter.

When `outputs.cursor.commands-dir` is set, each agent additionally emits as a [Cursor Custom Command](https://docs.cursor.com/agent/custom-commands): a Markdown file with optional `description` and `model` frontmatter. The rule-form emission (`.cursor/rules/<name>.mdc`) still happens, so existing setups keep working.

### GitHub Copilot (`copilot`)

> When `outputs.copilot.chatmodes-dir` is set, each agent additionally emits as a [Copilot Custom Chat Mode](https://docs.github.com/en/copilot/customizing-copilot/adding-custom-instructions-for-github-copilot#about-custom-chat-modes) at `<dir>/<name>.chatmode.md` with `description`/`model`/`tools` frontmatter. The catch-all `agent-<name>.instructions.md` emission keeps working.


```
.github/copilot-instructions.md                            # canonical entry-point pointer body (written by sync)
.github/instructions/<name>.instructions.md                # scoped rule per file
.github/instructions/agent-<name>.instructions.md          # one per agent
.github/instructions/skill-<name>.instructions.md          # one per skill
.vscode/mcp.json                                           # when MCP entries exist
```

Config keys: `outputs.copilot.instructions-dir` (default `.github/instructions`), `outputs.copilot.mcp-file` (default `.vscode/mcp.json`), `outputs.copilot.rules-file` (unset; setting it writes the always-on rules concatenated at that path and `sync` skips the pointer-body write for `copilot`).

Copilot natively supports path-scoped instructions via `applyTo:` frontmatter. Rules with a `globs` field (or a source-layout scope like `rules/backend/auth.md`) emit as a separate `.instructions.md` file with `applyTo` derived from the globs (explicit `globs` wins, else `<scope>/**`). Always-on rules (no globs, no scope, or `alwaysApply: true`) skip per-file emission — they are reachable via the canonical pointer body at `.github/copilot-instructions.md` plus the source spec dir. Agents and skills always emit as catch-all (`applyTo: "**"`) so they remain discoverable across the repo.

The Copilot MCP file uses the VS Code schema: a top-level `servers` key with each entry carrying a `type` field (`stdio`, `http`, or `sse`).

### Aider (`aider`)

```
CONVENTIONS.md           # canonical entry-point pointer body (written by sync)
.aider.conf.yml          # only when conf-file is set
```

Config keys: `outputs.aider.conf-file` (default empty — opt-in), `outputs.aider.model`, `outputs.aider.weak-model`, `outputs.aider.rules-file` (unset; setting it writes a legacy merged document at that path and `sync` skips the pointer-body write for `aider`).

By default the adapter only emits the conventions document and you wire it in yourself via `aider --read CONVENTIONS.md`. Set `outputs.aider.conf-file: .aider.conf.yml` to have `sync` also merge a `read:` entry into Aider's [project config](https://aider.chat/docs/config/aider_conf.html) so the file auto-loads. `model` and `weak-model` propagate into the same file when set. Pre-existing keys in the conf file are preserved; the `read:` list de-duplicates.

#### Verifying with the real Aider CLI

1. Install Aider: `python -m pip install -U aider-chat` (or `pipx install aider-chat`).
2. Sanity-check the emitted tree:

   ```bash
   ls CONVENTIONS.md .aider.conf.yml
   head -1 .aider.conf.yml  # must start with the agnostic-ai provenance header
   ```

3. Validate the YAML syntactically: `python -c "import yaml,sys; yaml.safe_load(open('.aider.conf.yml'))"`.
4. Run aider with the project config and confirm it picks the conventions document up without warnings:

   ```bash
   aider --config .aider.conf.yml --no-stream --message "list the rules you were told to follow"
   ```

   The opening banner should print the resolved `read:` paths (including `CONVENTIONS.md`) and the response should reflect the rules text.
5. Confirm aider does not flag the file as untrusted: there should be no `Warning:` lines mentioning `CONVENTIONS.md` or `.aider.conf.yml`.

### Cline (`cline`)

```
.clinerules/<name>.md
.clinerules/workflows/<name>.md      # one per agent, only when workflows-dir is set
```

Config keys: `outputs.cline.rules-dir` (default `.clinerules`), `outputs.cline.workflows-dir` (default empty — opt-in).

When `outputs.cline.workflows-dir` is set, each agent additionally emits as a [Cline Workflow](https://docs.cline.bot/features/workflows): a Markdown file invokable from chat as `/<name>.md`. The italic description prefixes the body when present. The rule-form emission (`.clinerules/agent-<name>.md`) still happens, so existing setups keep working.

#### Verifying with the real Cline extension

1. Install the [Cline extension](https://marketplace.visualstudio.com/items?itemName=saoudrizwan.claude-dev) in VS Code (or open the project in a VS Code distribution with Cline pre-installed).
2. Sanity-check the emitted tree:

   ```bash
   ls .clinerules/
   head -1 .clinerules/*.md  # provenance header on each
   ```

3. Open the project in VS Code, then open the Cline panel. Cline auto-loads every `.clinerules/*.md` file. Confirm each entry appears in the rules list with no "failed to parse" warnings in the Cline output channel.
4. If `outputs.cline.workflows-dir` is set, open the chat and confirm each `<workflows-dir>/<name>.md` is invokable as `/<name>.md`. The italic description (when present) should preview the workflow.

### Windsurf (`windsurf`)

```
.windsurf/rules/<name>.md
.windsurf/workflows/<name>.md        # one per agent, only when workflows-dir is set
```

Config keys: `outputs.windsurf.rules-dir` (default `.windsurf/rules`), `outputs.windsurf.workflows-dir` (default empty — opt-in).

When `outputs.windsurf.workflows-dir` is set, each agent additionally emits as a [Windsurf Workflow](https://docs.windsurf.com/windsurf/cascade/workflows): a Markdown file with `description` frontmatter, invokable in Cascade as `/<name>`. The rule-form emission (`.windsurf/rules/agent-<name>.md`) still happens, so existing setups keep working.

### Continue (`continue`)

```
.continue/rules/<name>.md
.continue/mcpServers/<name>.yaml       # one per MCP entry
.continue/assistants/<name>.yaml       # one per agent, only when assistants-dir is set
```

Config keys: `outputs.continue.rules-dir` (default `.continue/rules`), `outputs.continue.mcp-dir` (default `.continue/mcpServers`), `outputs.continue.assistants-dir` (default empty — opt-in).

Continue picks up each YAML under `.continue/mcpServers/` as a single MCP server config. Stdio servers emit `command`/`args`/`env`; HTTP / SSE / streamable-http variants emit `type`/`url`/`headers`.

When `outputs.continue.assistants-dir` is set, each agent additionally emits as a [Continue local Assistant](https://docs.continue.dev/hub/assistants/intro) YAML at `<dir>/<name>.yaml` (`schema: v1`, `version: 0.0.1` by default). The agent body wraps as a single named prompt so Continue surfaces it in the assistant picker. Models and rules are intentionally omitted so the user's defaults apply. The rule-form emission (`.continue/rules/agent-<name>.md`) still happens, so existing setups keep working.

#### Verifying with the real Continue extension

1. Install the [Continue extension](https://marketplace.visualstudio.com/items?itemName=Continue.continue) in VS Code (or JetBrains IDEs).
2. Sanity-check the emitted tree:

   ```bash
   ls .continue/rules/ .continue/mcpServers/
   head -1 .continue/rules/*.md .continue/mcpServers/*.yaml  # provenance header on each
   python -c "import yaml,sys; [yaml.safe_load(open(f)) for f in __import__('glob').glob('.continue/mcpServers/*.yaml')]"
   ```

3. Open the project in VS Code. Continue auto-loads every `.continue/rules/*.md`. Confirm each entry appears in the Continue rules picker without "failed to parse" warnings.
4. Open the MCP picker inside Continue and confirm each `.continue/mcpServers/<name>.yaml` entry appears with a green status.
5. If `outputs.continue.assistants-dir` is set, confirm each `<dir>/<name>.yaml` is listed under the assistants picker with the configured name + description.

### Amp (`amp`)

```
AGENTS.md                              # canonical entry-point pointer body (written by sync, shared with codex/warp)
.agents/commands/<name>.md             # one per agent
.agents/commands/skill-<name>.md       # one per skill, only when emit-skills-as-commands: true
.amp/settings.json                     # when MCP entries exist (merged with any existing user config)
```

Config keys: `outputs.amp.commands-dir` (default `.agents/commands`), `outputs.amp.mcp-file` (default `.amp/settings.json`), `outputs.amp.emit-skills-as-commands` (default `false`), `outputs.amp.rules-file` (unset; setting it writes a legacy concatenated rules document at that path and `sync` skips the pointer-body write for `amp`).

The project-root `AGENTS.md` is written by `sync` with the canonical pointer body — shared byte-for-byte with codex and warp so the three targets coexist at the same path with no collision.

When MCP entries are present, the adapter writes (or updates) `.amp/settings.json` with the `amp.mcpServers` map (note: a single dotted key, not a nested object). Stdio servers emit `command`/`args`/`env`; HTTP/SSE emit `url`/`headers`. Pre-existing non-managed keys in `.amp/settings.json` (theme, editor settings, etc.) are preserved across syncs; only `amp.mcpServers` is overwritten. Workspace MCPs require explicit approval inside Amp the first time the project is opened — Amp's safety model is intentional.

The adapter previously wrote `AGENT.md` (singular). Amp's owner's manual specifies `AGENTS.md` (plural); the misnamed file is retired. On first sync after upgrading, any agnostic-generated `AGENT.md` at the configured root is renamed to `AGENT.md.bak`. A user-authored `AGENT.md` (no `Generated by agnostic-ai` marker) is left untouched.

#### Verifying with the real Amp CLI

1. Install Amp: `npm install -g @sourcegraph/amp` (or use the VS Code extension; both read the same project files).
2. Sanity-check the emitted tree:

   ```bash
   ls AGENTS.md .agents/commands/ .amp/settings.json
   head -1 .agents/commands/*.md  # every command file must start with the provenance header
   ```

3. Validate the JSON syntactically: `python -m json.tool .amp/settings.json > /dev/null`.
4. Open the project in Amp and confirm it indexes `AGENTS.md` (Amp's left sidebar surfaces it under "Project rules"). There should be no warning about `AGENT.md.bak`.
5. List the available slash commands inside Amp — each `.agents/commands/<name>.md` file should appear as `/<name>` with the rendered description.
6. List configured MCP servers via Amp's MCP picker — every entry under `amp.mcpServers` in `.amp/settings.json` should appear with a green status. Workspace MCPs prompt for approval the first time; that prompt is expected, not an error.

### Zed (`zed`)

```
.rules
.zed/settings.json                     # when MCP entries exist (merged with any existing user config)
.zed/tasks.json                        # one task per hook, only when tasks-file is set
```

Config keys: `outputs.zed.file` (default `.rules`), `outputs.zed.mcp-file` (default `.zed/settings.json`), `outputs.zed.tasks-file` (default empty — opt-in).

When MCP entries are present, the adapter writes (or updates) `.zed/settings.json` with the `context_servers` map (Zed's key — not `mcpServers`). Each server emits with Zed's nested `command: {path, args, env}` shape plus an empty `settings: {}`. User-managed keys (theme, buffer_font_size, etc.) are preserved across syncs.

Zed only supports stdio transport natively. HTTP / SSE MCP entries auto-bridge through `npx mcp-remote <url>` so they still work — a one-line stderr warning fires when this fallback is taken.

When `outputs.zed.tasks-file` is set, every hook spec emits as a [Zed Task](https://zed.dev/docs/tasks) into that JSON file. Each task uses `sh -c "<hook command>"` so arbitrary one-liners work; the hook's name is the label (with the description appended after an em-dash when present). Zed has no lifecycle-hook surface, so these tasks run on demand from the command palette rather than firing on `PreToolUse` / `PostToolUse` events.

### Warp (`warp`)

```
AGENTS.md                              # canonical entry-point pointer body (written by sync, shared with codex/amp)
.warp/workflows/<name>.yaml            # one per agent, only when workflows-dir is set
.warp/.mcp.json                        # when MCP entries exist
```

Config keys: `outputs.warp.workflows-dir` (default empty — opt-in), `outputs.warp.mcp-file` (default `.warp/.mcp.json`), `outputs.warp.rules-file` (unset; setting it writes a legacy concatenated rules document at that path and `sync` skips the pointer-body write for `warp`).

The project-root `AGENTS.md` is written by `sync` with the canonical pointer body — shared byte-for-byte with codex and amp so the three targets coexist at the same path with no collision.

The adapter previously wrote `WARP.md`. Warp's current Rules docs specify `AGENTS.md` per the open AGENTS.md standard; the legacy name is retired. On first sync after upgrading, any agnostic-generated `WARP.md` at the configured root is renamed to `WARP.md.bak`. A user-authored `WARP.md` (no `Generated by agnostic-ai` marker) is left untouched.

When `outputs.warp.workflows-dir` is set, each agent emits as a [Warp Workflow](https://docs.warp.dev/features/warp-drive/workflows) YAML at `<dir>/<name>.yaml` (`name`/`command`/`description`/`tags`). The workflow `command:` is the agent body verbatim — tailor it to a Warp-friendly shell snippet from there.

### OpenCode (`opencode`)

```
.opencode/AGENTS.md                       # canonical entry-point pointer body (written by sync)
.opencode/commands/<name>.md              # one per agent
.opencode/commands/skill-<name>.md        # one per skill, only when emit-skills-as-commands: true
opencode.json                             # when MCP entries exist (merged with any existing user config)
```

Config keys: `outputs.opencode.commands-dir` (default `.opencode/commands`), `outputs.opencode.mcp-file` (default `opencode.json`), `outputs.opencode.emit-skills-as-commands` (default `false`), `outputs.opencode.rules-file` (unset; setting it writes a legacy concatenated rules document at that path and `sync` skips the pointer-body write for `opencode`).

When MCP entries are present, the adapter writes (or updates) `opencode.json` at the project root with a `$schema` link and the `mcp` map. Stdio servers map to `{type: "local", command: [...]}`; HTTP/SSE/remote servers map to `{type: "remote", url, headers}`. Any pre-existing non-managed keys (`theme`, `model`, etc.) are preserved across syncs; only `$schema` and `mcp` are overwritten. Note: drift checks (`sync --check`) read in capture mode and skip the read step, so unrelated user keys may be reported as drift until the next non-check sync — this is by design.

Routed under `.opencode/` rather than the repo root to avoid clashing with Codex's `AGENTS.md`. Each command file carries frontmatter filtered to the OpenCode-supported keys (`description`, `agent`, `model`, `subtask`) — pass `agent`, `model`, or `subtask` through the `x-opencode` namespace in your spec frontmatter to route them into the command file without polluting other targets.

### Google Antigravity (`antigravity`)

```
.agent/AGENTS.md           # canonical entry-point pointer body (written by sync)
.agent/rules/<name>.md     # one per rule
.agent/rules/agent-<name>.md  # one per agent
```

Config keys: `outputs.antigravity.rules-dir` (default `.agent/rules`), `outputs.antigravity.rules-file` (unset; setting it writes a legacy merged document at that path and `sync` skips the pointer-body write for `antigravity`).

Antigravity reads project instructions from a top-level AGENTS.md-style file and per-rule files under `.agent/rules/`. The adapter emits per-rule files and `sync` writes the canonical pointer body to `.agent/AGENTS.md`. The path stays under `.agent/` to avoid clashing with codex / amp / warp at the project-root `AGENTS.md`.

Skills, hooks, MCPs, and commands are not yet confirmed in the Antigravity public preview spec and are skipped with a warning. Add them to your `on-unsupported: silent` config to suppress the warning, or wait for a future release once the upstream spec stabilises.

#### Verifying with the real Antigravity IDE

1. Install Antigravity from the Google Antigravity public-preview download page.
2. Sanity-check the emitted tree:

   ```bash
   ls .agent/AGENTS.md .agent/rules/
   head -1 .agent/rules/*.md  # every rule + agent file must start with the provenance header
   ```

3. Open the project in Antigravity and confirm it surfaces `.agent/AGENTS.md` in the project-instructions panel — the canonical pointer body should appear without "unrecognized file" warnings.
4. Open one of `.agent/rules/<name>.md` from inside Antigravity and verify the per-rule file is also picked up.
5. Trigger an agent action (e.g. ask for a refactor) and confirm the rules apply in the response. There should be no schema-validation log entries referencing `.agent/`.

## Selecting targets

Persistent (config):

```yaml
targets:
  - claude
  - cursor
  - copilot
```

Per-run (CLI):

```bash
agnostic-ai sync -t claude,cursor,copilot
```

CLI flag overrides config. Unknown targets log a warning and skip.

Interactive `init` pre-ticks any target whose marker is present in the working directory (e.g. `.claude/`, `.codex/`, `.gemini/`, `.cursor/`, `.github/copilot-instructions.md`). The first-time sync prompt does the same. Toggle entries in the picker before confirming.

## New targets

See [adding-adapters](../internal/adding-adapters.md). ~50 lines plus one registry entry.

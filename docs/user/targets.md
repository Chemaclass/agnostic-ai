# Targets

Each adapter emits in its tool's native format: separate files where the tool supports them, a merged document otherwise. Unsupported features (e.g. hooks for a non-hook-aware target) skip with a warning by default. Override via `on-unsupported` in [configuration](configuration.md).

## Entry-point files

`sync` writes `.agnostic-ai/AGNOSTIC_AI.md` plus a root entry-point file per enabled target. All entry-point files share the same canonical pointer body. Editing one in place is a no-op (overwritten on next sync), and switching tools never surfaces inconsistent conventions.

| Target          | Entry-point file                  |
|-----------------|-----------------------------------|
| **claude**      | `CLAUDE.md`                       |
| **codex**       | `AGENTS.md`                       |
| **amp**         | `AGENTS.md`                       |
| **warp**        | `AGENTS.md`                       |
| **gemini**      | `GEMINI.md`                       |
| **aider**       | `CONVENTIONS.md`                  |
| **copilot**     | `.github/copilot-instructions.md` |
| **opencode**    | `.opencode/AGENTS.md`             |
| **antigravity** | `.agent/AGENTS.md`                |

Targets sharing a path (codex + amp + warp at `AGENTS.md`) write it once; dedup is automatic. Targets absent from the table above (cursor, cline, windsurf, continue, zed) have no root entry-point: they emit only per-file artifacts under their own directory.

Targets with no native rules directory (codex, amp, warp, gemini, aider, opencode) inline every rule body into their entry-point file under a sentinel-marked `## Rules` block, after the pointer body. That file is the only always-on context surface these tools read, so the rule reaches them by default. The block is identical across targets that share a path, so the dedup still holds. `import` strips the block, keeping the AGNOSTIC_AI.md round-trip lossless.

Set `outputs.<target>.rules-file: <path>` to use the legacy concatenated rules layout instead. The adapter writes a single merged document at `<path>` and `sync` skips the pointer-body write for that target so they do not collide.

Set `sync.target-overview: true` to append a generated section to each entry-point file listing where that tool's generated artifacts live (rules dir, MCP file, ...). The canonical body stays identical across targets; only the appendix differs per file. See [configuration](configuration.md#synctarget-overview).

## Capability matrix

| Target          | Agents              | Skills | Rules                    | Hooks | MCPs | Commands | Settings | Reviews | Environments | Ignore |
|-----------------|---------------------|--------|--------------------------|-------|------|----------|----------|---------|--------------|--------|
| **claude**      | `.claude/agents/`   | `.claude/skills/` | `.claude/rules/*.md`   | `.claude/settings.json` | `.mcp.json` | `.claude/commands/<name>.md` | `.claude/settings.json` (`permissions`, `model`) | - | - | - |
| **codex**       | `.codex/agents/*.toml` | `.agents/skills/<name>/SKILL.md` | inlined into `AGENTS.md` (legacy concat via `outputs.codex.rules-file`) | `.codex/hooks.json` (per-event arrays) | `.codex/config.toml` (`[mcp_servers.<name>]`) | `.codex/prompts/<name>.md` w/ opt-in (deprecated by Codex) | - | - | - | - |
| **gemini**      | `.gemini/commands/<name>.toml` | `.gemini/commands/skill-<name>.toml` w/ opt-in | inlined into `GEMINI.md` (legacy concat via `outputs.gemini.rules-file`) | `.gemini/settings.json` (`hooks`) | `.gemini/settings.json` (`mcpServers`) | `.gemini/commands/<name>.toml` | - | - | - | `.aiexclude` |
| **cursor**      | `.cursor/agents/<name>.md` | `.cursor/skills/<name>/SKILL.md` | `.cursor/rules/*.mdc` | `.cursor/hooks.json` | `.cursor/mcp.json` | `.cursor/commands/<name>.md` | - | `.cursor/BUGBOT.md` (per scope) | `.cursor/environment.json` | `.cursorignore` |
| **copilot**     | `.github/instructions/agent-<name>.instructions.md` | `.github/instructions/skill-<name>.instructions.md` | `.github/instructions/<name>.instructions.md` (scoped `applyTo:<glob>`; always-on rules use `applyTo:"**"`, or set `outputs.copilot.rules-file` for the legacy concatenated layout) | - | `.vscode/mcp.json` | - | - | - | - | - |
| **aider**       | source-dir only (legacy merge via `outputs.aider.rules-file`) | source-dir only | inlined into `CONVENTIONS.md` (legacy merge via `outputs.aider.rules-file`) | - | - | - | - | - | - | `.aiderignore` |
| **cline**       | as `.md` rule (+ `.clinerules/workflows/<name>.md` w/ opt-in) | as `.md` (`skill-<name>.md`) | `.clinerules/*.md`       | -     | - | - | - | - | - | - |
| **windsurf**    | as `.md` rule (+ `.windsurf/workflows/<name>.md` w/ opt-in) | as `.md` (`skill-<name>.md`) | `.windsurf/rules/*.md`   | -     | - | - | - | - | - | - |
| **continue**    | as `.md` rule (+ `.continue/assistants/<name>.yaml` w/ opt-in) | as `.md` (`skill-<name>.md`) | `.continue/rules/*.md`   | -     | `.continue/mcpServers/*.yaml` | - | - | - | - | - |
| **amp**         | `.agents/commands/<name>.md` | `.agents/skills/<name>/SKILL.md` | inlined into `AGENTS.md` (legacy concat via `outputs.amp.rules-file`) | - | `.amp/settings.json` (`amp.mcpServers`) | `.agents/commands/<name>.md` | - | - | - | - |
| **zed**         | merged in `.rules` | listed in `.rules` | `.rules` | `.zed/tasks.json` w/ opt-in | `.zed/settings.json` (`context_servers`) | - | - | - | - | - |
| **warp**        | `.warp/workflows/<name>.yaml` w/ opt-in | source-dir only | inlined into `AGENTS.md` (legacy concat via `outputs.warp.rules-file`) | - | `.warp/.mcp.json` | - | - | - | - | - |
| **opencode**    | `.opencode/commands/<name>.md` | `.opencode/commands/skill-<name>.md` w/ opt-in | inlined into `.opencode/AGENTS.md` (legacy concat via `outputs.opencode.rules-file`) | - | `opencode.json` (`mcp`) | `.opencode/commands/<name>.md` | - | - | - | - |
| **antigravity** | as `.md` rule (`agent-<name>.md`) | `.agent/skills/<name>/SKILL.md` | `.agent/rules/*.md` (legacy merge via `outputs.antigravity.rules-file`) | - | - | - | - | - | - | - |

Cells marked "w/ opt-in" or "source-dir only" do not emit by default. When specs of that kind are present, `sync` prints a `note:` line naming the key to set (or stating the content stays source-dir only). See [Coverage notes](configuration.md#coverage-notes).

Cross-cutting kind notes:

- **Skills**: Claude Code, Codex, Cursor, Amp, and Antigravity execute skill folders natively (SKILL.md + bundled assets). Codex and Amp share one tree at `.agents/skills/`, which Cursor also scans. On the remaining targets skills are reference material the agent or human reads and follows.
- **Hooks**: shell commands on lifecycle events (`PreToolUse`, `PostToolUse`, `SessionStart`, etc.). Native on Claude Code, Codex, Gemini, and Cursor (`.cursor/hooks.json`; Cursor uses camelCase event names like `beforeShellExecution`). Zed runs them via opt-in `outputs.zed.tasks-file` as on-demand tasks. Other targets skip with a warning.
- **MCP servers**: propagate to every target with a project-scoped MCP file (10 of 14, see matrix). Aider, Cline, Windsurf, and Antigravity have no MCP surface and skip with a warning. On targets whose native schema uses a `type` field (claude, cursor, copilot, warp, continue, opencode), remote (HTTP / SSE) entries carry an explicit `type` and stdio entries omit it (the inferred default). amp, gemini, and zed have no `type` field and infer the transport from the emitted keys (gemini via `httpUrl` vs `url`; amp and zed via `url` vs `command`).
- **Commands**: slash-prompt files authored under `commands/`. Native on Claude Code (`.claude/commands/<name>.md`), Cursor (`.cursor/commands/<name>.md`), Gemini (`.gemini/commands/<name>.toml`), OpenCode (`.opencode/commands/<name>.md`), and Amp (`.agents/commands/<name>.md`). Codex deprecated project prompts (its commands stay source-only unless `outputs.codex.commands-dir` opts into the legacy `.codex/prompts/` layout). Other targets skip with a warning.

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

- **Rules**: one file per spec under `.claude/rules/`. Claude Code discovers every `.md` file under that directory (recursively) at session start, so the emitted rules load natively with no extra wiring. A spec with the cross-tool `globs` field (or a native `paths` list) emits `paths:` frontmatter, which scopes the rule to matching files. `outputs.claude.rules-mode: import` (appends a sentinel-marked block of `@.claude/rules/<name>.md` imports to the pointer body, round-trip-stripped on `import`) predates native rules loading; keep it only for Claude Code versions older than the `.claude/rules/` rollout. The legacy alternative `outputs.claude.rules-file: CLAUDE.md` concatenates rule bodies into a single file and skips the pointer-body write for `claude`.
- **Commands**: one file per spec under `.claude/commands/`. Spec `deploy` becomes `/deploy`. Frontmatter passes through; body is the prompt template.
- **Settings overlay**: `agnostic-ai import claude` captures the non-`hooks` portion of `.claude/settings.json` (statusLine, enabledPlugins, any top-level key) into `.agnostic-ai/overlays/claude.settings.json`. `sync -t claude` layers the spec-derived `hooks` key on top, reproducing the full settings.json from a fresh checkout. Re-run `import claude` after editing settings.json by hand.
- **MCP import**: `import claude` reads `.mcp.json` and writes one spec under `<mcps>/<name>.yaml` per `mcpServers.<name>`. The next `sync` distributes them to codex, copilot, cursor, continue, amp, zed, warp, gemini, opencode.
- **First-class settings**: `outputs.claude.settings.*` declares model, outputStyle, statusLine, permissions, enabledPlugins, env, apiKeyHelper, cleanupPeriodDays, includeCoAuthoredBy. They merge above the captured overlay and below the spec-derived hooks. See [Claude settings](configuration.md#claude-settings).

Config keys: `outputs.claude.dir` (default `.claude`), `outputs.claude.rules-dir` (default `.claude/rules`, auto-loaded by Claude Code), `outputs.claude.rules-mode` (unset; set to `import` to also wire `.claude/rules/*.md` into `CLAUDE.md` via `@`-imports, only needed on Claude Code versions without native rules loading), `outputs.claude.rules-file` (unset; switches to legacy concatenated single-file layout, typically `CLAUDE.md`), `outputs.claude.commands-dir` (default `.claude/commands`), `outputs.claude.mcp-file` (default `.mcp.json`), `outputs.claude.settings` (first-class settings block).

Verify with the real CLI:

1. Install: `npm install -g @anthropic-ai/claude-code` (or the desktop app; both read the same files).
2. Check the tree: `ls CLAUDE.md .claude/agents/ .claude/skills/ .claude/rules/ .claude/commands/ .claude/settings.json .mcp.json` and `grep "Generated by agnostic-ai" .claude/agents/*.md .claude/rules/*.md .claude/commands/*.md` for the provenance header (it sits after the YAML frontmatter, so `head -1` would only show `---`).
3. Validate JSON: `python -m json.tool .claude/settings.json > /dev/null && python -m json.tool .mcp.json > /dev/null`.
4. Launch `claude` from the project root. `/agents`, `/skills`, and the slash-command picker should list every entry. The MCP picker shows each `.mcp.json` server green.
5. Trigger a matcher action (e.g. an `Edit` for `PostToolUse`/`Edit`); the hook command runs with no "schema mismatch" in the log.
6. Confirm `outputs.claude.settings.*` keys under `/config`.

### Codex (`codex`)

```
AGENTS.md                                    # canonical entry-point pointer body (written by sync)
.codex/agents/<name>.toml                    # one TOML per agent (Codex CLI's native path)
.agents/skills/<name>/SKILL.md               # one folder per skill (the path Codex CLI scans)
.agents/skills/<name>/agents/openai.yaml     # optional, when x-codex provides UI/policy/deps
.codex/config.toml                           # when MCP entries exist
.codex/hooks.json                            # when hook entries exist
.codex/rules/default.rules                   # opt-in, from outputs.codex.exec-policies
.codex/prompts/<name>.md                     # opt-in via outputs.codex.commands-dir (deprecated by Codex)
```

- **Rules**: `sync` writes `AGENTS.md` with the pointer body plus a sentinel-marked `## Rules` block holding every rule body inline. Codex reads `AGENTS.md` as always-on context, so the rules reach it by default. `import codex` strips that block (the canonical rules live under the source dir). Per-directory scoping (e.g. `src/AGENTS.md` from `globs: src/**`) is no longer emitted by default. Use `outputs.codex.rules-file: AGENTS.md` for the legacy concatenated layout.
- **Skills**: [Codex skills layout](https://developers.openai.com/codex/skills), one folder per skill under `.agents/skills/` (the directory Codex scans from the cwd up to the repo root) with a required `SKILL.md` (frontmatter `name` + `description`, plus body). When the spec carries `x-codex.interface`, `x-codex.policy`, or `x-codex.dependencies`, an `agents/openai.yaml` is also written for UI customization and policy declarations. Amp reads the same path; identical emitted bytes dedupe, so enabling both targets is safe. A stale managed tree at the pre-v0.43 `.codex/skills/` default is swept on sync.
- **Hooks**: land in `.codex/hooks.json` (override via `outputs.codex.hooks-file`), routed by `event` frontmatter (`SessionStart`, `SubagentStart`, `UserPromptSubmit`, `PreToolUse`, `PermissionRequest`, `PostToolUse`, `PreCompact`, `PostCompact`, `Stop`, `SubagentStop`) into per-event arrays with `matcher` and `command`. Optional `timeout`, `statusMessage`, and `commandWindows` pass through. The JSON form preserves matcher metadata the inline `[[hooks.<event>]]` TOML cannot.
- **Exec policies**: opt-in. Set `outputs.codex.exec-policies` (inline list) or `outputs.codex.exec-policies-file` (external YAML) to write `.codex/rules/default.rules` in Codex's Skylark `prefix_rule(...)` form. Unset writes nothing.
- **MCP**: lands in `.codex/config.toml`. Servers emit as `[mcp_servers.<name>]`: stdio uses `command`/`args`/`env`, HTTP/SSE uses `url`/`bearer_token_env_var`/`http_headers`. The project-tier config.toml is managed (overwritten each sync); put unmanaged Codex config in `~/.codex/config.toml`.
- **Commands**: not emitted by default. Codex loads custom prompts from `~/.codex/prompts/` only (no project-level discovery) and [deprecates them in favor of skills](https://developers.openai.com/codex/custom-prompts), so a project-tier prompts tree would never be read; `sync` prints a coverage note instead and sweeps a stale managed `.codex/prompts/` tree. Set `outputs.codex.commands-dir` to emit the legacy layout anyway.
- **Import**: `import codex` captures `.codex/config.toml` minus `hooks` and `mcp_servers` into `.agnostic-ai/overlays/codex.config.toml`. `sync -t codex` prepends it before the spec-derived sections, so `model`, `sandbox`, `approval_policy`, `notify`, `[history]`, `[profiles.*]`, `[model_providers.*]`, and any other key survive a `.codex/` wipe. On conflict with `outputs.codex.config.*` the overlay wins and the first-class key is dropped to keep TOML valid. `import codex` also reads `.codex/prompts/*.md` and writes them byte-for-byte to `<commands>/`, so user-authored prompts round-trip.

Config keys: `outputs.codex.agents-dir` (default `.codex/agents`; override to `.agents/agents` for the community shared layout), `outputs.codex.skills-dir` (default `.agents/skills`, the path Codex scans), `outputs.codex.shared-subagents` (default `true`; emits the per-skill tree at `skills-dir`. Set `false` to skip codex skill emission), `outputs.codex.commands-dir` (unset; set to e.g. `.codex/prompts` to emit the deprecated project prompts layout), `outputs.codex.mcp-file` (default `.codex/config.toml`), `outputs.codex.hooks-file` (default `.codex/hooks.json`), `outputs.codex.rules-file` (unset; writes legacy concatenated rules and skips the pointer-body write), `outputs.codex.exec-policies` / `outputs.codex.exec-policies-file` (unset; write `.codex/rules/default.rules`).

Verify with the real CLI:

1. Install: `npm install -g @openai/codex` ([quickstart](https://developers.openai.com/codex/cli)); `codex --version` to confirm PATH.
2. Check the tree: `agnostic-ai sync -t codex`, then `ls .codex/agents/ .agents/skills/`, `test -f .codex/config.toml && head -1 .codex/config.toml`, `test -f .codex/hooks.json && jq '.hooks | keys' .codex/hooks.json`. First line of config.toml must be the `# Generated by agnostic-ai` provenance comment.
3. Validate syntax: `toml-test .codex/config.toml` and `jq empty .codex/hooks.json` should both exit `0`.
4. `codex run "list one rule from this project"`. Codex picks up `AGENTS.md`, the agents, and skill folders. Look for `loaded N agents` / `loaded N skills`.
5. Trigger a hook by firing the targeted `event` (e.g. an `Edit` for a `PostToolUse` hook); the `command` appears in the hook log.
6. `codex mcp list` shows every `[mcp_servers.<name>]`. Disabled servers appear with the disabled flag.

The audit issue [#329](https://github.com/Chemaclass/agnostic-ai/issues/329) tracks this smoke checklist; close its "Real CLI smoke" box only after every step passes against the live Codex CLI build in the linked PR.

### Gemini CLI (`gemini`)

```
GEMINI.md                              # canonical entry-point pointer body (written by sync)
.gemini/commands/<name>.toml           # one per agent
.gemini/commands/skill-<name>.toml     # one per skill, only when emit-skills-as-commands: true
.gemini/settings.json                  # when MCP and/or hook entries exist (merged with existing user config)
```

- **Rules**: `sync` writes `GEMINI.md` with the pointer body plus a sentinel-marked `## Rules` block holding every rule body inline, so Gemini reads them as always-on context by default. Per-directory scoping (e.g. `src/GEMINI.md` from `globs: src/**`) is no longer emitted by default. Use `outputs.gemini.rules-file: GEMINI.md` for the legacy concatenated layout.
- **Agents**: TOML slash commands under `.gemini/commands/` per [Gemini CLI custom commands](https://geminicli.com/docs/cli/custom-commands/). Skills default to source-only; set `outputs.gemini.emit-skills-as-commands: true` to emit one `skill-<name>.toml` per skill.
- **Commands**: one TOML per command spec under `.gemini/commands/<name>.toml`, the same surface as agents. `description` frontmatter maps to the TOML `description`; the body becomes the `prompt`.
- **MCP + hooks**: written into `.gemini/settings.json` (`mcpServers` map, `hooks` map). Gemini keys the endpoint by transport: streamable-HTTP servers (`type: http`) use `httpUrl`, SSE servers (`type: sse`) use `url`; the adapter routes each automatically. Hooks route by `event` frontmatter (`BeforeTool`, `AfterTool`, `SessionStart`); each entry is `{matcher, command}`. Pre-existing user keys survive syncs.

Config keys: `outputs.gemini.commands-dir` (default `.gemini/commands`), `outputs.gemini.mcp-file` (default `.gemini/settings.json`, also holds hooks), `outputs.gemini.emit-skills-as-commands` (default `false`), `outputs.gemini.rules-file` (unset; writes legacy concatenated rules and skips the pointer-body write), `outputs.gemini.ignore-file` (default `.aiexclude`).

Verify with the real CLI:

1. Install: `npm install -g @google/gemini-cli` ([docs](https://geminicli.com/docs/)).
2. Check the tree: `ls GEMINI.md .gemini/commands/ .gemini/settings.json`, `head -1 .gemini/commands/*.toml` for the provenance header, `python -m json.tool .gemini/settings.json > /dev/null`.
3. `gemini --list-commands` parses every `<name>.toml` with no "invalid TOML" / "unknown field" errors.
4. `gemini --list-mcp-servers` shows each `mcpServers.<name>` ready.
5. Trigger a hook by performing the matcher action (e.g. an `AfterTool`); the hook command runs.

### Cursor (`cursor`)

```
.cursor/rules/<name>.mdc
.cursor/agents/<name>.md             # one native subagent per agent spec
.cursor/skills/<name>/SKILL.md       # one folder per skill, bundled assets included
.cursor/commands/<name>.md           # one per command spec
.cursor/hooks.json                   # when hook specs exist (managed, overwritten each sync)
```

- Rules emit with `alwaysApply: true` (override in spec frontmatter). An always-apply rule omits `globs`; otherwise empty globs default to `**/*`. Scalar globs keep minimal quoting so a hand-authored `.mdc` round-trips clean. (#443)
- **Agents**: native [Cursor subagents](https://cursor.com/docs/subagents.md) at `.cursor/agents/<name>.md` (Cursor 2.4+): frontmatter `name` + `description` plus optional `model`, `readonly`, and `is_background` when the spec declares them; the body is the system prompt. The old flattened `.mdc` and agent-as-command emissions are gone; the ledger sweeps stale copies.
- Each command spec emits as a [Cursor command](https://cursor.com/docs/agent/chat/commands) under `.cursor/commands/`: Markdown whose body is the prompt. Override the directory via `outputs.cursor.commands-dir`.
- **Skills**: native folders under `.cursor/skills/<name>/SKILL.md` (the [Agent Skills](https://cursor.com/docs/skills.md) layout Cursor 2.4+ discovers), with every bundled sibling file (scripts, references, assets) propagated byte-for-byte. Frontmatter carries `name` + `description`; optional `paths` and `disable-model-invocation` pass through when the spec declares them. The pre-native flattened `skill-<name>.mdc` copies are no longer written and get swept by the ledger on the next sync.
- Review specs emit as [Bugbot](https://cursor.com/docs/bugbot) files inside `.cursor/` directories: `.cursor/BUGBOT.md` at the repo root for unscoped specs, `<scope>/.cursor/BUGBOT.md` for scoped ones, with same-scope specs concatenated. Bugbot always includes the root file and picks up per-directory copies while traversing up from changed files. Override the basename via `outputs.cursor.review-file`. (#433)
- Environment specs emit as `.cursor/environment.json` (background-agent bootstrap). The spec keys pass through verbatim minus agnostic routing fields; multiple specs merge by top-level key. Override the path via `outputs.cursor.environment-file`. (#434)
- Ignore specs emit as `.cursorignore` (gitignore syntax). Multiple specs concatenate. Override via `outputs.cursor.ignore-file`. (#435)
- Hook specs emit as [Cursor Hooks](https://cursor.com/docs/hooks) in a managed `.cursor/hooks.json` (`version` + per-event `{command, matcher?}` arrays; optional `timeout`, `loop_limit`, and `failClosed` pass through). Cursor uses camelCase event names (`beforeShellExecution`, `afterFileEdit`, ...), passed through verbatim; `validate` flags unrecognized ones. Override via `outputs.cursor.hooks-file`. (#438)

Config keys: `outputs.cursor.rules-dir` (default `.cursor/rules`), `outputs.cursor.agents-dir` (default `.cursor/agents`), `outputs.cursor.skills-dir` (default `.cursor/skills`), `outputs.cursor.commands-dir` (default `.cursor/commands`), `outputs.cursor.mcp-file` (default `.cursor/mcp.json`), `outputs.cursor.review-file` (default `BUGBOT.md`), `outputs.cursor.environment-file` (default `.cursor/environment.json`), `outputs.cursor.ignore-file` (default `.cursorignore`), `outputs.cursor.hooks-file` (default `.cursor/hooks.json`).

Verify with the real IDE:

1. Install Cursor from [cursor.com](https://cursor.com).
2. Check the tree: `ls .cursor/rules/ .cursor/skills/ .cursor/commands/ .cursor/mcp.json`, `grep "Generated by agnostic-ai" .cursor/rules/*.mdc` for the provenance header (it sits after the frontmatter block), `python -m json.tool .cursor/mcp.json > /dev/null`.
3. Open the project. The Rules panel loads every `.cursor/rules/*.mdc` (confirm `alwaysApply` matches each rule's frontmatter, no "failed to parse" warnings), the Skills list shows each `.cursor/skills/<name>/`, the agent picker lists each `.cursor/agents/<name>.md`, and the `/` command picker lists each `.cursor/commands/<name>.md`.
4. If MCPs are configured, Settings → MCP shows every `mcpServers.<name>` green.
5. If hooks are configured, `python -m json.tool .cursor/hooks.json > /dev/null` parses; trigger the matched event (e.g. a shell command for `beforeShellExecution`) and confirm the `command` runs.

### GitHub Copilot (`copilot`)

```
.github/copilot-instructions.md                            # canonical entry-point pointer body (written by sync)
.github/instructions/<name>.instructions.md                # scoped rule per file
.github/instructions/agent-<name>.instructions.md          # one per agent
.github/instructions/skill-<name>.instructions.md          # one per skill
.vscode/mcp.json                                           # when MCP entries exist
```

- **Rules**: Copilot supports path-scoped instructions via `applyTo:` frontmatter. Rules with `globs` (or a source-layout scope like `rules/backend/auth.md`) emit as a separate `.instructions.md` with `applyTo` derived from globs (explicit `globs` wins, else `<scope>/**`). Always-on rules (no globs, no scope, or `alwaysApply: true`) skip per-file emission and are reachable via the pointer body plus the source spec dir.
- **Agents + skills**: always emit as catch-all (`applyTo: "**"`) so they stay discoverable repo-wide.
- **Chat modes**: when `outputs.copilot.chatmodes-dir` is set, each agent also emits as a [Copilot Custom Chat Mode](https://docs.github.com/en/copilot/customizing-copilot/adding-custom-instructions-for-github-copilot#about-custom-chat-modes) at `<dir>/<name>.chatmode.md` with `description`/`model`/`tools` frontmatter. The `agent-<name>.instructions.md` emission keeps working.
- **MCP**: VS Code schema, top-level `servers` key, each entry carrying a `type` (`stdio`, `http`, or `sse`).

Config keys: `outputs.copilot.instructions-dir` (default `.github/instructions`), `outputs.copilot.mcp-file` (default `.vscode/mcp.json`), `outputs.copilot.chatmodes-dir` (default empty, opt-in; emits one Custom Chat Mode per agent), `outputs.copilot.rules-file` (unset; writes always-on rules concatenated at that path and skips the pointer-body write).

Verify with the real extension:

1. Install the [GitHub Copilot extension](https://marketplace.visualstudio.com/items?itemName=GitHub.copilot) (and optional Copilot Chat) in VS Code.
2. Check the tree: `ls .github/copilot-instructions.md .github/instructions/ .vscode/mcp.json`, `grep "Generated by agnostic-ai" .github/instructions/*.md` for the provenance header (it sits after the `applyTo` frontmatter), `python -m json.tool .vscode/mcp.json > /dev/null`.
3. Open the project. Copilot loads `.github/copilot-instructions.md` plus every matching `.instructions.md`. The `Output → GitHub Copilot` channel shows no "failed to parse instructions" warnings.
4. Open a file matching a rule glob (e.g. a `.go` file for `applyTo: "**/*.go"`) and trigger chat; the rule body shows in context.
5. If MCPs are configured, run `MCP: Show Installed Servers`; each `servers.<name>` from `.vscode/mcp.json` is ready.

### Aider (`aider`)

```
CONVENTIONS.md           # pointer body + inlined rules block (written by sync)
.aider.conf.yml          # only when conf-file is set
```

`CONVENTIONS.md` carries the pointer body plus a sentinel-marked `## Rules` block with every rule body inline, so the conventions reach Aider by default; `import aider` strips that block. Wire the file in via `aider --read CONVENTIONS.md`. Set `outputs.aider.conf-file: .aider.conf.yml` to also merge a `read:` entry into Aider's [project config](https://aider.chat/docs/config/aider_conf.html) so the file auto-loads. `model` and `weak-model` propagate into the same file when set. Pre-existing keys are preserved; the `read:` list de-duplicates.

Config keys: `outputs.aider.conf-file` (default empty, opt-in), `outputs.aider.model`, `outputs.aider.weak-model`, `outputs.aider.rules-file` (unset; writes a legacy merged document and skips the pointer-body write), `outputs.aider.ignore-file` (default `.aiderignore`).

Verify with the real CLI:

1. Install: `python -m pip install -U aider-chat` (or `pipx install aider-chat`).
2. Check the tree: `ls CONVENTIONS.md .aider.conf.yml`, `head -1 .aider.conf.yml` must start with the provenance header.
3. Validate YAML: `python -c "import yaml,sys; yaml.safe_load(open('.aider.conf.yml'))"`.
4. `aider --config .aider.conf.yml --no-stream --message "list the rules you were told to follow"`. The banner prints the resolved `read:` paths (including `CONVENTIONS.md`); the response reflects the rules text.
5. No `Warning:` lines mentioning `CONVENTIONS.md` or `.aider.conf.yml`.

### Cline (`cline`)

```
.clinerules/<name>.md
.clinerules/workflows/<name>.md      # one per agent, only when workflows-dir is set
```

When `outputs.cline.workflows-dir` is set, each agent also emits as a [Cline Workflow](https://docs.cline.bot/features/workflows): Markdown invokable from chat as `/<name>.md`. The italic description prefixes the body when present. The rule-form emission (`.clinerules/agent-<name>.md`) still happens.

Config keys: `outputs.cline.rules-dir` (default `.clinerules`), `outputs.cline.workflows-dir` (default empty, opt-in).

Verify with the real extension:

1. Install the [Cline extension](https://marketplace.visualstudio.com/items?itemName=saoudrizwan.claude-dev) in VS Code.
2. Check the tree: `ls .clinerules/`, `grep "Generated by agnostic-ai" .clinerules/*.md` for the provenance header.
3. Open the project, open the Cline panel. Cline loads every `.clinerules/*.md`; each appears in the rules list with no "failed to parse" warnings.
4. If `outputs.cline.workflows-dir` is set, each `<workflows-dir>/<name>.md` is invokable as `/<name>.md`; the italic description previews the workflow.

### Windsurf (`windsurf`)

```
.windsurf/rules/<name>.md
.windsurf/workflows/<name>.md        # one per agent, only when workflows-dir is set
```

When `outputs.windsurf.workflows-dir` is set, each agent also emits as a [Windsurf Workflow](https://docs.windsurf.com/windsurf/cascade/workflows): Markdown with `description` frontmatter, invokable in Cascade as `/<name>`. The rule-form emission (`.windsurf/rules/agent-<name>.md`) still happens.

Config keys: `outputs.windsurf.rules-dir` (default `.windsurf/rules`), `outputs.windsurf.workflows-dir` (default empty, opt-in).

Verify with the real IDE:

1. Install Windsurf from [windsurf.com](https://windsurf.com).
2. Check the tree: `ls .windsurf/rules/`, `grep "Generated by agnostic-ai" .windsurf/rules/*.md` for the provenance header.
3. Open the project. Cascade loads every `.windsurf/rules/*.md`; each appears in the Rules panel with no "failed to parse" warnings.
4. When `outputs.windsurf.workflows-dir` is set, each `<workflows-dir>/<name>.md` is invokable in Cascade as `/<name>` with the rendered description.

### Continue (`continue`)

```
.continue/rules/<name>.md
.continue/mcpServers/<name>.yaml       # one per MCP entry
.continue/assistants/<name>.yaml       # one per agent, only when assistants-dir is set
```

- **MCP**: each YAML under `.continue/mcpServers/` is a Continue block file: a `name` + `version` + `schema: v1` wrapper with the server nested under an `mcpServers:` list (a flat single-server file does not load). Stdio emits `command`/`args`/`env`; HTTP / SSE / streamable-http emit `type`/`url`/`headers`.
- **Assistants**: when `outputs.continue.assistants-dir` is set, each agent also emits as a [Continue local Assistant](https://docs.continue.dev/hub/assistants/intro) YAML at `<dir>/<name>.yaml` (`schema: v1`, `version: 0.0.1` by default). The agent body wraps as one named prompt for the assistant picker. Models and rules are omitted so user defaults apply. The rule-form emission (`.continue/rules/agent-<name>.md`) still happens.

Config keys: `outputs.continue.rules-dir` (default `.continue/rules`), `outputs.continue.mcp-dir` (default `.continue/mcpServers`), `outputs.continue.assistants-dir` (default empty, opt-in).

Verify with the real extension:

1. Install the [Continue extension](https://marketplace.visualstudio.com/items?itemName=Continue.continue) in VS Code (or JetBrains).
2. Check the tree: `ls .continue/rules/ .continue/mcpServers/`, `grep "Generated by agnostic-ai" .continue/rules/*.md .continue/mcpServers/*.yaml` for the provenance header, `python -c "import yaml,sys; [yaml.safe_load(open(f)) for f in __import__('glob').glob('.continue/mcpServers/*.yaml')]"`.
3. Open the project. Continue loads every `.continue/rules/*.md`; each appears in the rules picker with no "failed to parse" warnings.
4. The MCP picker shows each `.continue/mcpServers/<name>.yaml` green.
5. If `outputs.continue.assistants-dir` is set, each `<dir>/<name>.yaml` is listed under the assistants picker with name + description.

### Amp (`amp`)

```
AGENTS.md                              # canonical entry-point pointer body (written by sync, shared with codex/warp)
.agents/commands/<name>.md             # one per agent
.agents/skills/<name>/SKILL.md         # one folder per skill (Amp's native skills path)
.amp/settings.json                     # when MCP entries exist (merged with existing user config)
```

- **Rules**: `sync` writes `AGENTS.md` with the pointer body plus a sentinel-marked `## Rules` block holding every rule body inline, shared byte-for-byte with codex and warp so the three coexist at the same path.
- **Skills**: one folder per skill under `.agents/skills/<name>/SKILL.md` (Amp's [native skills layout](https://ampcode.com/manual)). Amp [removed custom slash commands in favor of skills](https://ampcode.com/news/slashing-custom-commands), so skills no longer emit as `.agents/commands/skill-<name>.md`. The SKILL.md frontmatter carries `name` + `description`; sibling assets next to the source SKILL.md are copied byte-for-byte. Arbitrary `x-amp` keys pass through.
- **Agents**: still emit as custom slash commands under `.agents/commands/<name>.md`. Amp has no documented per-agent file surface after the command removal; this path may be read for back-compat.
- **Commands**: one markdown file per command spec under `.agents/commands/<name>.md`, the same surface as agents. `description` frontmatter passes through; the body is the prompt template.
- **MCP**: written into `.amp/settings.json` under `amp.mcpServers` (a single dotted key, not a nested object). Stdio emits `command`/`args`/`env`; HTTP/SSE emit `url`/`headers`. Pre-existing non-managed keys (theme, editor settings) are preserved; only `amp.mcpServers` is overwritten. Workspace MCPs require explicit approval on first open (Amp's safety model).
- **Legacy rename**: Amp's owner's manual specifies `AGENTS.md` (plural). On first sync after upgrading, any agnostic-generated `AGENT.md` at the configured root is renamed to `AGENT.md.bak`. A user-authored `AGENT.md` (no `Generated by agnostic-ai` marker) is left untouched.

Config keys: `outputs.amp.commands-dir` (default `.agents/commands`), `outputs.amp.skills-dir` (default `.agents/skills`), `outputs.amp.mcp-file` (default `.amp/settings.json`), `outputs.amp.rules-file` (unset; writes legacy concatenated rules and skips the pointer-body write).

Verify with the real CLI:

1. Install: `npm install -g @sourcegraph/amp` (or the VS Code extension; both read the same files).
2. Check the tree: `ls AGENTS.md .agents/commands/ .agents/skills/ .amp/settings.json`, `grep "Generated by agnostic-ai" .agents/commands/*.md` for the provenance header (it sits after the frontmatter), `test -f .agents/skills/*/SKILL.md`.
3. Validate JSON: `python -m json.tool .amp/settings.json > /dev/null`.
4. Open the project; Amp indexes `AGENTS.md` under "Project rules" with no `AGENT.md.bak` warning, and loads each `.agents/skills/<name>/SKILL.md` as a skill.
5. The slash-command list shows each `.agents/commands/<name>.md` as `/<name>` with the rendered description.
6. The MCP picker lists every `amp.mcpServers` entry green. Workspace MCPs prompt for approval the first time (expected, not an error).

### Zed (`zed`)

```
.rules
.zed/settings.json                     # when MCP entries exist (merged with existing user config)
.zed/tasks.json                        # one task per hook, only when tasks-file is set
```

- **MCP**: written into `.zed/settings.json` under `context_servers` (Zed's key, not `mcpServers`). Stdio servers use a flat `command`/`args`/`env` shape; remote (HTTP / SSE) servers use a native `url`/`headers` shape. User-managed keys (theme, buffer_font_size) are preserved.
- **Hooks**: when `outputs.zed.tasks-file` is set, every hook spec emits as a [Zed Task](https://zed.dev/docs/tasks) using `sh -c "<hook command>"`. The hook name is the label (description appended after an em-dash when present). Zed has no lifecycle-hook surface, so tasks run on demand from the command palette.

Config keys: `outputs.zed.file` (default `.rules`), `outputs.zed.mcp-file` (default `.zed/settings.json`), `outputs.zed.tasks-file` (default empty, opt-in).

Verify with the real editor:

1. Install Zed from [zed.dev](https://zed.dev).
2. Check the tree: `ls .rules .zed/settings.json .zed/tasks.json`, `grep "Generated by agnostic-ai" .rules` for the intro line (it follows the `# Project rules` title, not on line 1), `python -m json.tool .zed/settings.json > /dev/null`, `python -m json.tool .zed/tasks.json > /dev/null`.
3. Open the project. The inline assistant reads `.rules` as system context; confirm a prompt sees the rule content.
4. The MCP picker shows each `context_servers.<name>` ready.
5. The command palette runs every entry from `.zed/tasks.json` as a Zed Task.

### Warp (`warp`)

```
AGENTS.md                              # canonical entry-point pointer body (written by sync, shared with codex/amp)
.warp/workflows/<name>.yaml            # one per agent, only when workflows-dir is set
.warp/.mcp.json                        # when MCP entries exist
```

- **Rules**: `sync` writes `AGENTS.md` with the pointer body plus a sentinel-marked `## Rules` block holding every rule body inline, shared byte-for-byte with codex and amp so the three coexist at the same path.
- **Workflows**: when `outputs.warp.workflows-dir` is set, each agent emits as a [Warp Workflow](https://docs.warp.dev/features/warp-drive/workflows) YAML at `<dir>/<name>.yaml` (`name`/`command`/`description`/`tags`). The `command:` is the agent body verbatim; tailor it to a Warp-friendly shell snippet.
- **Legacy rename**: Warp's current Rules docs specify `AGENTS.md` per the AGENTS.md standard. On first sync after upgrading, any agnostic-generated `WARP.md` at the configured root is renamed to `WARP.md.bak`. A user-authored `WARP.md` (no `Generated by agnostic-ai` marker) is left untouched.

Config keys: `outputs.warp.workflows-dir` (default empty, opt-in), `outputs.warp.mcp-file` (default `.warp/.mcp.json`), `outputs.warp.rules-file` (unset; writes legacy concatenated rules and skips the pointer-body write).

Verify with the real terminal:

1. Install Warp from [warp.dev](https://www.warp.dev).
2. Check the tree: `ls AGENTS.md .warp/workflows/ .warp/.mcp.json`, `grep "Generated by agnostic-ai" .warp/workflows/*.yaml` for the provenance header, `python -m json.tool .warp/.mcp.json > /dev/null`.
3. Open the project; confirm the rules panel surfaces `AGENTS.md` (no "unrecognized file" warnings), the Warp Drive workflows picker shows every `<workflows-dir>/<name>.yaml`, and the MCP picker lists every `mcpServers.<name>` from `.warp/.mcp.json`.

### OpenCode (`opencode`)

```
.opencode/AGENTS.md                       # canonical entry-point pointer body (written by sync)
.opencode/commands/<name>.md              # one per agent and one per command spec
.opencode/commands/skill-<name>.md        # one per skill, only when emit-skills-as-commands: true
opencode.json                             # when MCP entries exist (merged with existing user config)
```

- **Routing**: under `.opencode/` rather than the repo root to avoid clashing with Codex's `AGENTS.md`. Each command file carries frontmatter filtered to the OpenCode-supported keys (`description`, `agent`, `model`, `subtask`). Pass `agent`, `model`, or `subtask` through the `x-opencode` namespace to route them in without polluting other targets.
- **Commands**: one markdown file per command spec under `.opencode/commands/<name>.md`, the same surface as agents, with the same filtered frontmatter.
- **MCP**: written into `opencode.json` at the project root with a `$schema` link and the `mcp` map. Stdio maps to `{type: "local", command: [...]}`; HTTP/SSE/remote maps to `{type: "remote", url, headers}`. Pre-existing non-managed keys (`theme`, `model`) are preserved; only `$schema` and `mcp` are overwritten. Drift checks (`sync --check`, `doctor`) read the existing file, so user keys never report as drift and `doctor --fix` keeps them.

Config keys: `outputs.opencode.commands-dir` (default `.opencode/commands`), `outputs.opencode.mcp-file` (default `opencode.json`), `outputs.opencode.emit-skills-as-commands` (default `false`), `outputs.opencode.rules-file` (unset; writes legacy concatenated rules and skips the pointer-body write).

Verify with the real CLI:

1. Install: `npm install -g sst/opencode` ([install docs](https://opencode.ai/)).
2. Check the tree: `ls .opencode/AGENTS.md .opencode/commands/ opencode.json`, `grep "Generated by agnostic-ai" .opencode/commands/*.md` for the provenance header (it sits after the frontmatter), `python -m json.tool opencode.json > /dev/null`.
3. Launch `opencode`. Every `.opencode/commands/<name>.md` appears in the slash-command picker with the rendered description.
4. The MCP panel shows each `mcp.<name>` from `opencode.json` ready.

### Google Antigravity (`antigravity`)

```
.agent/AGENTS.md              # canonical entry-point pointer body (written by sync)
.agent/rules/<name>.md        # one per rule
.agent/rules/agent-<name>.md  # one per agent
.agent/skills/<name>/SKILL.md # one folder per skill (Antigravity's native path)
```

Antigravity reads project instructions from a top-level AGENTS.md-style file and per-rule files under `.agent/rules/`. The adapter emits per-rule files; `sync` writes the pointer body to `.agent/AGENTS.md`. The path stays under `.agent/` to avoid clashing with codex / amp / warp at the project-root `AGENTS.md`.

- **Skills**: one folder per skill under `.agent/skills/<name>/SKILL.md` (Antigravity's [native skills layout](https://codelabs.developers.google.com/getting-started-with-antigravity-skills), one folder per skill). The SKILL.md frontmatter is reduced to `name` + `description`; the body follows. Sibling files next to the source SKILL.md (helper scripts, fixtures) are copied byte-for-byte into the emitted folder.

Hooks, MCPs, and commands are not yet confirmed in the Antigravity public-preview spec and skip with a warning. Add `on-unsupported: silent` to suppress it, or wait for a future release once the upstream spec stabilises.

Config keys: `outputs.antigravity.rules-dir` (default `.agent/rules`), `outputs.antigravity.skills-dir` (default `.agent/skills`), `outputs.antigravity.rules-file` (unset; writes a legacy merged document and skips the pointer-body write).

Verify with the real IDE:

1. Install Antigravity from the Google Antigravity public-preview download page.
2. Check the tree: `ls .agent/AGENTS.md .agent/rules/ .agent/skills/`, `grep "Generated by agnostic-ai" .agent/rules/*.md` for the provenance header (it sits after the frontmatter), `test -f .agent/skills/*/SKILL.md`.
3. Open the project; it surfaces `.agent/AGENTS.md` in the project-instructions panel with no "unrecognized file" warnings.
4. Open one of `.agent/rules/<name>.md` and verify the per-rule file is picked up; confirm each `.agent/skills/<name>/SKILL.md` loads as a skill.
5. Trigger an agent action (e.g. ask for a refactor); the rules apply, with no schema-validation log entries referencing `.agent/`.

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

The default target set is 12: claude, codex, gemini, cursor, copilot, aider, cline, windsurf, continue, zed, opencode, antigravity. **Amp** and **Warp** are opt-in. Both share the root `AGENTS.md` entry-point with Codex and add no new entry-point, so plain `sync` skips them. Add them to `targets:` (or pass `-t amp,warp`) to emit their target-specific files (`.agents/`, `.amp/settings.json`, `.warp/`).

Interactive `init` pre-ticks any target whose marker is present in the working directory (e.g. `.claude/`, `.codex/`, `.gemini/`, `.cursor/`, `.github/copilot-instructions.md`). The first-time sync prompt does the same. Toggle entries before confirming.

## New targets

See [adding-adapters](../internal/adding-adapters.md). ~50 lines plus one registry entry.

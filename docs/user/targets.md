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
| **zed**         | `AGENTS.md`                       |
| **cline**       | `AGENTS.md`                       |
| **junie**       | `AGENTS.md`                       |
| **kiro**        | `AGENTS.md`                       |
| **crush**       | `AGENTS.md`                       |
| **trae**        | `AGENTS.md`                       |
| **jules**       | `AGENTS.md`                       |
| **goose**       | `AGENTS.md`                       |
| **augment**     | `AGENTS.md`                       |
| **qoder**       | `AGENTS.md`                       |
| **openhands**   | `AGENTS.md`                       |
| **factory**     | `AGENTS.md`                       |
| **kilo**        | `AGENTS.md`                       |
| **gemini**      | `GEMINI.md`                       |
| **aider**       | `CONVENTIONS.md`                  |
| **copilot**     | `.github/copilot-instructions.md` |
| **opencode**    | `.opencode/AGENTS.md`             |
| **antigravity** | `.agent/AGENTS.md`                |

Targets sharing a path (codex, amp, warp, zed, cline, junie, kiro, crush, trae, jules, goose, augment, qoder, openhands, factory, and kilo at `AGENTS.md`) write it once; dedup is automatic. Targets absent from the table above (cursor, windsurf, continue) have no root entry-point: they emit only per-file artifacts under their own directory.

Junie is the one target with a second, preferred entry-point file. Its guidelines lookup is strict precedence, first match wins, not a merge: `.junie/AGENTS.md`, then the root `AGENTS.md` above, then the legacy `.junie/guidelines.md` / `.junie/guidelines/`. `sync` always writes `.junie/AGENTS.md`, so step 1 always matches and everything after it is unreachable in a synced project (target-audit 2026-08-08, #552). Rule and agent bodies therefore inline directly into `.junie/AGENTS.md`, under their own sentinel-marked blocks (see the Junie section below): the only file Junie itself ever reads. The shared root `AGENTS.md` above still carries the same canonical pointer body, plus its own inlined-rules block once junie or another AGENTS.md-family target below needs one, but never an agents block, so the two files are not byte-identical.

Targets with no native rules directory (codex, amp, warp, zed, gemini, aider, opencode, crush, jules, goose, openhands, factory, junie) inline every rule body into their entry-point file under a sentinel-marked `## Rules` block, after the pointer body. That file is the only always-on context surface these tools read, so the rule reaches them by default. The block is identical across targets that share a path, so the dedup still holds. `import` strips the block, keeping the AGNOSTIC_AI.md round-trip lossless. Junie is the one member of this list whose entry-point file (`.junie/AGENTS.md`) is not the shared path the others write: it also gets a sibling `## Agents` block no other target here has, since Junie has no native per-agent surface at all to prefer instead. Augment now also has a native `.augment/rules/<name>.md` directory (see below) but keeps inlining into AGENTS.md too: the vendor does not cleanly establish the two surfaces' relative precedence, so this adapter does not assume the native directory makes the inline copy redundant. Kilo Code now also has a native `.kilo/rules/<name>.md` directory (see below), referenced from `kilo.jsonc`'s `instructions` array, which outranks AGENTS.md in Kilo Code's own documented precedence order (agent prompt > project `instructions` > AGENTS.md > global); this adapter keeps inlining into AGENTS.md too since AGENTS.md is always loaded when present regardless, so the inline copy is a fallback layer, not dead weight.

Set `outputs.<target>.rules-file: <path>` to use the legacy concatenated rules layout instead. The adapter writes a single merged document at `<path>` and `sync` skips the pointer-body write for that target so they do not collide.

Set `sync.target-overview: true` to append a generated section to each entry-point file listing where that tool's generated artifacts live (rules dir, MCP file, ...). The canonical body stays identical across targets; only the appendix differs per file. See [configuration](configuration.md#synctarget-overview).

## Capability matrix

| Target          | Agents              | Skills | Rules                    | Hooks | MCPs | Commands | Settings | Reviews | Environments | Ignore |
|-----------------|---------------------|--------|--------------------------|-------|------|----------|----------|---------|--------------|--------|
| **claude**      | `.claude/agents/`   | `.claude/skills/` | `.claude/rules/*.md`   | `.claude/settings.json` | `.mcp.json` | `.claude/commands/<name>.md` | `.claude/settings.json` (`permissions`, `model`) | - | - | - |
| **codex**       | `.codex/agents/*.toml` | `.agents/skills/<name>/SKILL.md` | inlined into `AGENTS.md` (legacy concat via `outputs.codex.rules-file`) | `.codex/hooks.json` (per-event arrays) | `.codex/config.toml` (`[mcp_servers.<name>]`) | `.codex/prompts/<name>.md` w/ opt-in (deprecated by Codex) | - | - | - | - |
| **gemini**      | `.gemini/commands/<name>.toml` | `.gemini/skills/<name>/SKILL.md` (+ command TOML w/ opt-in) | inlined into `GEMINI.md` (legacy concat via `outputs.gemini.rules-file`) | `.gemini/settings.json` (`hooks`) | `.gemini/settings.json` (`mcpServers`) | `.gemini/commands/<name>.toml` | - | - | - | `.aiexclude` |
| **cursor**      | `.cursor/agents/<name>.md` | `.cursor/skills/<name>/SKILL.md` | `.cursor/rules/*.mdc` | `.cursor/hooks.json` | `.cursor/mcp.json` | `.cursor/commands/<name>.md` | - | `.cursor/BUGBOT.md` (per scope) | `.cursor/environment.json` | `.cursorignore` |
| **copilot**     | `.github/agents/<name>.agent.md` | `.github/skills/<name>/SKILL.md` | `.github/instructions/<name>.instructions.md` (scoped `applyTo:<glob>`; always-on rules use `applyTo:"**"`, or set `outputs.copilot.rules-file` for the legacy concatenated layout) | - | `.vscode/mcp.json` | - | - | - | - | - |
| **aider**       | source-dir only (legacy merge via `outputs.aider.rules-file`) | source-dir only | inlined into `CONVENTIONS.md` (legacy merge via `outputs.aider.rules-file`) | - | - | - | - | - | - | `.aiderignore` |
| **cline**       | `.cline/agents/<name>.md` (+ `.cline/workflows/<name>.md` w/ opt-in) | `.cline/skills/<name>/SKILL.md` | `.cline/rules/*.md` (opt-in legacy path via `outputs.cline.rules-dir: .clinerules`) | -     | - | - | - | - | - | - |
| **windsurf**    | as `.md` rule (+ `.windsurf/workflows/<name>.md` w/ opt-in) | `.agents/skills/<name>/SKILL.md` | `.devin/rules/*.md`      | -     | - | - | - | - | - | `.devinignore` |
| **continue**    | as `.md` rule (+ `.continue/assistants/<name>.yaml` w/ opt-in) | as `.md` (`skill-<name>.md`) | `.continue/rules/*.md`   | -     | `.continue/mcpServers/*.yaml` | - | - | - | - | - |
| **amp**         | `.agents/commands/<name>.md` | `.agents/skills/<name>/SKILL.md` | inlined into `AGENTS.md` (legacy concat via `outputs.amp.rules-file`) | - | `.amp/settings.json` (`amp.mcpServers`) | `.agents/commands/<name>.md` | - | - | - | - |
| **zed**         | merged doc w/ opt-in (`outputs.zed.rules-file`) | `.agents/skills/<name>/SKILL.md` | inlined into `AGENTS.md` (legacy merge via `outputs.zed.rules-file`) | `.zed/tasks.json` w/ opt-in | `.zed/settings.json` (`context_servers`) | - | - | - | - | - |
| **warp**        | `.warp/workflows/<name>.yaml` w/ opt-in | source-dir only | inlined into `AGENTS.md` (legacy concat via `outputs.warp.rules-file`) | - | `.warp/.mcp.json` | - | - | - | - | - |
| **opencode**    | `.opencode/agents/<name>.md` | `.opencode/skills/<name>/SKILL.md` (+ command form w/ opt-in) | inlined into `.opencode/AGENTS.md` (legacy concat via `outputs.opencode.rules-file`) | - | `opencode.json` (`mcp`) | `.opencode/commands/<name>.md` | - | - | - | - |
| **antigravity** | as `.md` rule (`agent-<name>.md`) | `.agents/skills/<name>/SKILL.md` | `.agents/rules/*.md` (legacy merge via `outputs.antigravity.rules-file`) | - | `.agents/mcp_config.json` | - | - | - | - | - |
| **junie**       | inlined into `.junie/AGENTS.md` | `.junie/skills/<name>/SKILL.md` | inlined into `.junie/AGENTS.md` | - | `.junie/mcp/mcp.json` | - | - | - | - | - |
| **kiro**        | `.kiro/agents/<name>.md` (native agent profile) | `.kiro/steering/skill-<name>.md` (`inclusion: auto`) | `.kiro/steering/<name>.md` (`inclusion: always` or `fileMatch`) | `.kiro/hooks/<name>.json` | `.kiro/settings/mcp.json` | - | - | - | - | - |
| **crush**       | - | `.agents/skills/<name>/SKILL.md` | inlined into `AGENTS.md` | - | `crush.json` (`mcp`) | - | - | - | - | - |
| **trae**        | as `.md` rule (`agent-<name>.md`) | `.trae/skills/<name>/SKILL.md` | `.trae/rules/*.md` | - | - | `.trae/commands/<name>.md` | - | - | - | - |
| **qoder**       | `.qoder/agents/<name>.md` | - | `.qoder/rules/<name>.md` | - | `.mcp.json` | - | - | - | - | - |
| **openhands**   | - | `.agents/skills/<name>/SKILL.md` | inlined into `AGENTS.md` | - | `config.toml` (`[mcp]`) | - | - | - | - | - |
| **factory**     | `.factory/droids/<name>.md` | - | inlined into `AGENTS.md` | - | `.factory/mcp.json` | - | - | - | - | - |
| **kilo**        | `.kilo/agents/<name>.md` | `.agents/skills/<name>/SKILL.md` | `.kilo/rules/<name>.md` (+ `kilo.jsonc` `instructions` array; also inlined into `AGENTS.md`) | - | `kilo.jsonc` (`mcp`) | - | - | - | - | - |
| **jules**       | - | - | inlined into `AGENTS.md` | - | - | - | - | - | - | - |
| **goose**       | - | - | inlined into `AGENTS.md` (opt-in `.goosehints`) | - | - | - | - | - | - | - |
| **augment**     | `.augment/agents/<name>.md` | `.agents/skills/<name>/SKILL.md` | `.augment/rules/<name>.md` (+ inlined into `AGENTS.md`; opt-in `.augment-guidelines`) | - | - | - | - | - | - | - |

Cells marked "w/ opt-in" or "source-dir only" do not emit by default. When specs of that kind are present, `sync` prints a `note:` line naming the key to set (or stating the content stays source-dir only). See [Coverage notes](configuration.md#coverage-notes).

Cross-cutting kind notes:

- **Skills**: Claude Code, Codex, Cursor, Amp, Zed, Crush, Gemini, OpenCode, Copilot, OpenHands, Antigravity, Cline, Windsurf, Trae, Augment, Junie, and Kilo Code execute skill folders natively (SKILL.md + bundled assets). Codex, Amp, Zed, Crush, OpenHands, Antigravity, Windsurf, Augment, and Kilo Code share one tree at `.agents/skills/`, which Cursor, Gemini, OpenCode, and Copilot also scan alongside their own dirs (`.gemini/skills/`, `.opencode/skills/`, `.github/skills/`); Cline, Trae, and Junie each read their own tree (`.cline/skills/`, `.trae/skills/`, `.junie/skills/`), and Augment additionally scans `.claude/skills/` and `.augment/skills/` directly, so the shared tree covers it without a third on-disk copy, though a hand-authored `.augment/skills/<name>` wins a same-name collision over the synced one (lowest of Augment's six precedence slots). Kilo Code also documents `.agents/skills/` itself as a "loaded by default" compatibility path, which is why it defaults there directly instead of adding a second on-disk copy under `.kilo/skills/`. Warp has no skill surface; on Aider and Continue skills flatten to rule-form files, and on Kiro they become `inclusion: auto` steering files. With `sync.shared-skills: true`, byte-identical skill folders across these targets collapse into one canonical copy (`.agents/skills/<name>` preferred) plus per-skill symlinks; see [`sync.shared-skills`](configuration.md#syncshared-skills).
- **Hooks**: shell commands on lifecycle events (`PreToolUse`, `PostToolUse`, `SessionStart`, etc.). Native on Claude Code, Codex, Gemini, Cursor (`.cursor/hooks.json`; Cursor uses camelCase event names like `beforeShellExecution`), and Kiro (`.kiro/hooks/<name>.json`, one file per hook; Kiro also has `PreTaskExec`/`PostTaskExec`/`PostFileCreate`/`PostFileSave`/`PostFileDelete` events the others do not). Zed runs them via opt-in `outputs.zed.tasks-file` as on-demand tasks. Other targets skip with a warning.
- **MCP servers**: propagate to every target with a project-scoped MCP file (18 of 25, see matrix). Aider, Cline, Windsurf, Trae, Jules, Goose, and Augment have no MCP surface and skip with a warning. On targets whose native schema uses a `type` field (claude, cursor, copilot, continue, opencode, crush, kilo, factory, qoder), remote (HTTP / SSE) entries carry an explicit `type` and stdio entries omit it (crush and kilo tag stdio explicitly, with kilo also combining `command`+`args` into one array; the others infer it as the default). amp, gemini, zed, junie, kiro, and antigravity have no `type` field and infer the transport from the emitted keys (gemini via `httpUrl` vs `url`; antigravity via `serverUrl` vs `command`, and its doc is explicit that the legacy `url` / `httpUrl` names "are not supported"; the rest via `url` vs `command`). OpenHands has no `type` field either, but unlike that group it has no shared key shape to infer from: transport is implied entirely by which `[mcp]` array (`stdio_servers`, `sse_servers`, `shttp_servers`) a server lands in. Qoder's `.mcp.json` is the identical file and schema Claude Code writes there; enabling both dedupes into one write instead of colliding. See [`disabled` support by target](spec-format.md#disabled-support-by-target) for which of these targets honor a spec's `disabled: true` (Codex, Factory, Kilo Code, and OpenCode do; Claude Code, Cursor, Copilot, and Qoder do not).
- **Commands**: slash-prompt files authored under `commands/`. Native on Claude Code (`.claude/commands/<name>.md`), Cursor (`.cursor/commands/<name>.md`), Gemini (`.gemini/commands/<name>.toml`), OpenCode (`.opencode/commands/<name>.md`), Amp (`.agents/commands/<name>.md`), and Trae (`.trae/commands/<name>.md`, `name` + `description` frontmatter only). Codex deprecated project prompts (its commands stay source-only unless `outputs.codex.commands-dir` opts into the legacy `.codex/prompts/` layout). Other targets skip with a warning.

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
- **First-class settings**: `outputs.claude.settings.*` declares model, outputStyle, statusLine, permissions, enabledPlugins, env, apiKeyHelper, cleanupPeriodDays, and attribution. The deprecated includeCoAuthoredBy key remains available for older Claude Code versions. These settings merge above the captured overlay and below the spec-derived hooks. See [Claude settings](configuration.md#claude-settings).

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
- **Agents**: [Codex custom agents](https://learn.chatgpt.com/docs/agent-configuration/subagents) use one TOML file per agent with `name`, `description`, and `developer_instructions`, plus optional session config such as `model`, `model_reasoning_effort`, and `sandbox_mode`. A generic `tools: [Read, Bash, ...]` list is not emitted because Codex defines `tools` as a config table, not a tool allowlist. Sync reports the dropped field. Use `x-codex.tools` for native settings such as `web_search` and `view_image`. Use a per-target `model` map when another CLI's model name, such as `sonnet`, must not reach Codex.
- **Skills**: [Codex skills layout](https://developers.openai.com/codex/skills), one folder per skill under `.agents/skills/` (the directory Codex scans from the cwd up to the repo root) with a required `SKILL.md` (frontmatter `name` + `description`, plus body). When the spec carries `x-codex.interface`, `x-codex.policy`, or `x-codex.dependencies`, an `agents/openai.yaml` is also written for UI customization and policy declarations. Amp reads the same path; identical emitted bytes dedupe, so enabling both targets is safe. A stale managed tree at the pre-v0.43 `.codex/skills/` default is swept on sync.
- **Hooks**: land in `.codex/hooks.json` (override via `outputs.codex.hooks-file`), routed by `event` frontmatter (`SessionStart`, `SubagentStart`, `UserPromptSubmit`, `PreToolUse`, `PermissionRequest`, `PostToolUse`, `PreCompact`, `PostCompact`, `Stop`, `SubagentStop`, `SessionEnd`) into per-event arrays with `matcher` and `command`. Optional `timeout`, `statusMessage`, `commandWindows`, and `additionalContextLimit` pass through and survive `import codex`; an explicit `additionalContextLimit: 0` is preserved because Codex uses it to pass complete hook context. The JSON form preserves matcher metadata the inline `[[hooks.<event>]]` TOML cannot.
- **Exec policies**: opt-in. Set `outputs.codex.exec-policies` (inline list) or `outputs.codex.exec-policies-file` (external YAML) to write `.codex/rules/default.rules` in Codex's Starlark `prefix_rule(...)` form. Unset writes nothing.
- **MCP**: lands in `.codex/config.toml`. Servers emit as `[mcp_servers.<name>]`: stdio uses `command`/`args`/`env`/`cwd` plus the mixed `env_vars` array, while HTTP/SSE uses `url`/`bearer_token_env_var`/`http_headers`/`env_http_headers`/`auth` (`oauth` or `chatgpt`). All of these fields survive `import codex`. The project-tier config.toml is managed (overwritten each sync); put unmanaged Codex config in `~/.codex/config.toml`.
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
.gemini/skills/<name>/SKILL.md         # one folder per skill, bundled assets included
.gemini/commands/skill-<name>.toml     # additional command form, only when emit-skills-as-commands: true
.gemini/settings.json                  # when MCP and/or hook entries exist (merged with existing user config)
```

- **Rules**: `sync` writes `GEMINI.md` with the pointer body plus a sentinel-marked `## Rules` block holding every rule body inline, so Gemini reads them as always-on context by default. Per-directory scoping (e.g. `src/GEMINI.md` from `globs: src/**`) is no longer emitted by default. Use `outputs.gemini.rules-file: GEMINI.md` for the legacy concatenated layout.
- **Agents**: TOML slash commands under `.gemini/commands/` per [Gemini CLI custom commands](https://geminicli.com/docs/cli/custom-commands/).
- **Skills**: native [Agent Skills](https://geminicli.com/docs/cli/skills/) folders under `.gemini/skills/<name>/SKILL.md` (the workspace tier Gemini CLI scans; it also reads the cross-tool `.agents/skills/` alias), with bundled sibling files propagated byte-for-byte. Set `outputs.gemini.emit-skills-as-commands: true` to additionally emit one `skill-<name>.toml` command per skill.
- **Commands**: one TOML per command spec under `.gemini/commands/<name>.toml`, the same surface as agents. `description` frontmatter maps to the TOML `description`; the body becomes the `prompt`.
- **MCP + hooks**: written into `.gemini/settings.json` (`mcpServers` map, `hooks` map). Gemini keys the endpoint by transport: streamable-HTTP servers (`type: http`) use `httpUrl`, SSE servers (`type: sse`) use `url`; the adapter routes each automatically. Stdio servers also accept `cwd` (working directory), the same cross-tool field Codex reads. Hooks route by `event` frontmatter (e.g. `BeforeTool`, `AfterTool`, `SessionStart`, `SessionEnd`; Gemini CLI documents 11 events in total); each entry is `{matcher, command}`. Pre-existing user keys survive syncs.

Config keys: `outputs.gemini.commands-dir` (default `.gemini/commands`), `outputs.gemini.skills-dir` (default `.gemini/skills`), `outputs.gemini.mcp-file` (default `.gemini/settings.json`, also holds hooks), `outputs.gemini.emit-skills-as-commands` (default `false`), `outputs.gemini.rules-file` (unset; writes legacy concatenated rules and skips the pointer-body write), `outputs.gemini.ignore-file` (default `.aiexclude`).

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

- Rules emit with `alwaysApply: true` (override in spec frontmatter). An always-apply rule omits `globs`. A non-always rule without `globs` falls back to the Claude-spelled `paths` list (comma-joined); when both are absent, `globs` is omitted too rather than defaulted to `**/*`, so Cursor treats the rule as description-driven ("Apply Intelligently") or manual-only ("Apply Manually") instead of auto-attaching it to every file. Scalar globs keep minimal quoting so a hand-authored `.mdc` round-trips clean. (#443, #536)
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
.github/agents/<name>.agent.md                             # one custom-agent profile per agent
.github/skills/<name>/SKILL.md                             # one folder per skill, bundled assets included
.vscode/mcp.json                                           # when MCP entries exist
```

- **Rules**: Copilot supports path-scoped instructions via `applyTo:` frontmatter. Rules with `globs` (or a source-layout scope like `rules/backend/auth.md`) emit as a separate `.instructions.md` with `applyTo` derived from globs (explicit `globs` wins, else `<scope>/**`). Always-on rules (no globs, no scope, or `alwaysApply: true`) skip per-file emission and are reachable via the pointer body plus the source spec dir.
- **Agents**: native [custom agent profiles](https://docs.github.com/en/copilot/reference/custom-agents-configuration) at `.github/agents/<name>.agent.md`: frontmatter `name` + `description` plus `tools` and `model` when the spec declares them; arbitrary `x-copilot` keys (`target`, `user-invocable`, `mcp-servers`, ...) pass through. The body is the agent prompt. The old flattened `agent-<name>.instructions.md` copies are gone; the ledger sweeps them.
- **Skills**: native [Copilot skills](https://docs.github.com/en/copilot/how-tos/copilot-on-github/customize-copilot/customize-cloud-agent/add-skills) folders at `.github/skills/<name>/SKILL.md` (Copilot also scans `.claude/skills/` and `.agents/skills/`), with bundled sibling files propagated byte-for-byte. The old flattened `skill-<name>.instructions.md` copies are gone; the ledger sweeps them.
- **Chat modes**: when `outputs.copilot.chatmodes-dir` is set, each agent also emits as a [Copilot Custom Chat Mode](https://docs.github.com/en/copilot/customizing-copilot/adding-custom-instructions-for-github-copilot#about-custom-chat-modes) at `<dir>/<name>.chatmode.md` with `description`/`model`/`tools` frontmatter. The native agent profile still emits alongside.
- **MCP**: VS Code schema, top-level `servers` key, each entry carrying a `type` (`stdio`, `http`, or `sse`).

Config keys: `outputs.copilot.instructions-dir` (default `.github/instructions`), `outputs.copilot.agents-dir` (default `.github/agents`), `outputs.copilot.skills-dir` (default `.github/skills`), `outputs.copilot.mcp-file` (default `.vscode/mcp.json`), `outputs.copilot.chatmodes-dir` (default empty, opt-in; emits one Custom Chat Mode per agent), `outputs.copilot.rules-file` (unset; writes always-on rules concatenated at that path and skips the pointer-body write).

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
AGENTS.md                            # canonical entry-point pointer body (written by sync, shared across the AGENTS.md consumers)
.cline/rules/<name>.md
.cline/agents/<name>.md
.cline/workflows/<name>.md           # one per agent, only when workflows-dir is set
.cline/skills/<name>/SKILL.md        # one folder per skill (Cline's recommended skills path)
```

Cline reads the cross-tool root `AGENTS.md`, so `sync` distributes the shared pointer body there (deduplicated with the other AGENTS.md consumers). Rules and agents each emit per file into their own directory under `.cline/`, the layout Cline's [current config reference](https://docs.cline.bot/getting-started/config) documents (`.cline/{rules,skills,hooks,agents,plugins,cron}/`). The older [cline-rules page](https://docs.cline.bot/customization/cline-rules) still calls the pre-migration `.clinerules/` the "Primary rule format", so this is a live migration rather than a completed removal (target-audit 2026-08-01): set `outputs.cline.rules-dir: .clinerules` to keep rules at that path; a stale managed tree there is swept on sync otherwise. Agents always emit at `.cline/agents/`, independent of that override: Cline's own file format for that directory has no dedicated doc page, so this adapter writes the spec body verbatim, no synthesized heading and no invented frontmatter.

- **Skills**: one folder per skill under `.cline/skills/<name>/SKILL.md`, the path [Cline's skills docs](https://docs.cline.bot/customization/skills) recommend. A flat file directly under the rules directory never loads as a skill, so this is a folder, not a rule-form file. The SKILL.md frontmatter carries `name` + `description`; sibling assets next to the source SKILL.md are copied byte-for-byte.

When `outputs.cline.workflows-dir` is set, each agent also emits as a [Cline Workflow](https://docs.cline.bot/features/workflows): Markdown invokable from chat as `/<name>.md`. The italic description prefixes the body when present. The native `.cline/agents/<name>.md` emission still happens either way.

Config keys: `outputs.cline.rules-dir` (default `.cline/rules`; set to `.clinerules` for the pre-migration layout), `outputs.cline.agents-dir` (default `.cline/agents`), `outputs.cline.skills-dir` (default `.cline/skills`), `outputs.cline.workflows-dir` (default empty, opt-in).

Verify with the real extension:

1. Install the [Cline extension](https://marketplace.visualstudio.com/items?itemName=saoudrizwan.claude-dev) in VS Code.
2. Check the tree: `ls .cline/rules/ .cline/agents/ .cline/skills/`, `grep "Generated by agnostic-ai" .cline/rules/*.md` for the provenance header, `test -f .cline/skills/*/SKILL.md`.
3. Open the project, open the Cline panel. Cline loads every `.cline/rules/*.md`; each appears in the rules list with no "failed to parse" warnings. Each `.cline/agents/<name>.md` appears wherever Cline surfaces project agent definitions, and each `.cline/skills/<name>/` loads as a skill.
4. If `outputs.cline.workflows-dir` is set, each `<workflows-dir>/<name>.md` is invokable as `/<name>.md`; the italic description previews the workflow.

### Windsurf / Devin Desktop (`windsurf`)

```
.devin/rules/<name>.md
.windsurf/workflows/<name>.md        # one per agent, only when workflows-dir is set
.agents/skills/<name>/SKILL.md       # one folder per skill (shared tree with codex/amp/zed/crush/openhands)
.devinignore                         # when ignore entries exist
```

Windsurf became Devin Desktop (2026-06). Devin Desktop prefers `.devin/rules/*.md` and keeps `.windsurf/rules/` as a backward-compat fallback (`.windsurfrules` is legacy), so rules now emit at the preferred path. The target keeps its `windsurf` name: existing `outputs.windsurf.*` keys and `x-windsurf` meta continue to work. Set `outputs.windsurf.rules-dir: .windsurf/rules` to stay on the old layout; otherwise sync sweeps managed leftovers at the pre-rename path (hand-authored files survive).

- **Skills**: one folder per skill under `.agents/skills/<name>/SKILL.md`, the tree [Devin Desktop discovers skills from](https://docs.devin.ai/desktop/cascade/skills). This is the same cross-tool tree codex, amp, zed, crush, and openhands already emit into byte-identically, so identical skills dedupe under `sync.shared-skills`. A flat file directly under `.devin/rules/` never loads as a skill. The SKILL.md frontmatter carries `name` + `description`; sibling assets next to the source SKILL.md are copied byte-for-byte.
- **Ignore**: merges into `.devinignore`, gitignore syntax under a `#` provenance header: "you can add a `.devinignore` file to your repo root, with the same syntax as .gitignore" ([docs.devin.ai/desktop/context-awareness/windsurf-ignore](https://docs.devin.ai/desktop/context-awareness/windsurf-ignore)). Devin Desktop also still respects the legacy `.codeiumignore` filename and `.windsurfignore`, but this adapter only writes the current `.devinignore` path; override via `outputs.windsurf.ignore-file` to write one of the legacy names instead.

When `outputs.windsurf.workflows-dir` is set, each agent also emits as a [Workflow](https://docs.devin.ai/desktop/cascade/workflows): Markdown with `description` frontmatter, invokable in Cascade as `/<name>`. Upstream still documents `.windsurf/workflows/` for workflows. The rule-form emission (`.devin/rules/agent-<name>.md`) still happens.

Config keys: `outputs.windsurf.rules-dir` (default `.devin/rules`), `outputs.windsurf.skills-dir` (default `.agents/skills`), `outputs.windsurf.workflows-dir` (default empty, opt-in), `outputs.windsurf.ignore-file` (default `.devinignore`).

Verify with the real IDE:

1. Install Devin Desktop from [devin.ai](https://devin.ai) (formerly windsurf.com).
2. Check the tree: `ls .devin/rules/ .agents/skills/`, `grep "Generated by agnostic-ai" .devin/rules/*.md` for the provenance header, `test -f .agents/skills/*/SKILL.md`.
3. Open the project. Cascade loads every `.devin/rules/*.md`; each appears in the Rules panel with no "failed to parse" warnings. Each `.agents/skills/<name>/` loads as a skill.
4. When `outputs.windsurf.workflows-dir` is set, each `<workflows-dir>/<name>.md` is invokable in Cascade as `/<name>` with the rendered description.
5. When ignore specs exist, `cat .devinignore` shows the concatenated patterns and indexing skips them.

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
AGENTS.md                              # canonical entry-point pointer body (written by sync, shared across the AGENTS.md consumers)
.agents/commands/<name>.md             # one per agent
.agents/skills/<name>/SKILL.md         # one folder per skill (Amp's native skills path)
.amp/settings.json                     # when MCP entries exist (merged with existing user config)
```

- **Rules**: `sync` writes `AGENTS.md` with the pointer body plus a sentinel-marked `## Rules` block holding every rule body inline, shared byte-for-byte with codex, warp, zed, and crush so they coexist at the same path.
- **Skills**: one folder per skill under `.agents/skills/<name>/SKILL.md` (Amp's [native skills layout](https://ampcode.com/manual)). Amp [removed custom slash commands in favor of skills](https://ampcode.com/news/slashing-custom-commands), so skills no longer emit as `.agents/commands/skill-<name>.md`. The SKILL.md frontmatter carries `name` + `description`; sibling assets next to the source SKILL.md are copied byte-for-byte. Arbitrary `x-amp` keys pass through.
- **Agents**: still emit as custom slash commands under `.agents/commands/<name>.md`. Amp [removed custom commands in favor of skills on 2026-01-29](https://ampcode.com/news/slashing-custom-commands) and has no documented per-agent file surface; this path is no longer read, kept only until Amp ships a native agent surface.
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
AGENTS.md                              # canonical entry-point pointer body + inlined rules (written by sync, shared across the AGENTS.md consumers)
.agents/skills/<name>/SKILL.md         # one folder per skill (shared tree with codex/amp/crush)
.zed/settings.json                     # when MCP entries exist (merged with existing user config)
.zed/tasks.json                        # one task per hook, only when tasks-file is set
```

- **Rules**: Zed 1.4.2 retired its rules library; always-on project instructions are the root `AGENTS.md`. `sync` writes the pointer body plus the sentinel-marked `## Rules` block there, shared byte-for-byte with codex, amp, warp, and crush. Set `outputs.zed.rules-file: .rules` to keep the legacy merged document (which also carries agent bodies) for older Zed versions.
- **Skills**: native [Zed skills](https://zed.dev/docs/ai/skills) folders at `.agents/skills/<name>/SKILL.md`, the cross-tool path codex, amp, and crush emit too. Identical rendered bytes dedupe into one write; divergent `x-zed` overrides surface through the collision check.
- **Agents**: no per-agent surface in current Zed; sync prints a coverage note unless `outputs.zed.rules-file` is set (the merged document carries agent sections).
- **MCP**: written into `.zed/settings.json` under `context_servers` (Zed's key, not `mcpServers`). Stdio servers use a flat `command`/`args`/`env` shape; remote (HTTP / SSE) servers use a native `url`/`headers` shape. User-managed keys (theme, buffer_font_size) are preserved.
- **Hooks**: when `outputs.zed.tasks-file` is set, every hook spec emits as a [Zed Task](https://zed.dev/docs/tasks) using `sh -c "<hook command>"`. The hook name is the label (description appended after an em-dash when present). Zed has no lifecycle-hook surface, so tasks run on demand from the command palette. Other documented Task fields (`cwd`, `env`, `shell`, `reveal`, `hide`, `save`, `allow_concurrent_runs`, `use_new_terminal`, `tags`, `reevaluate_context`) pass through when declared under `x-zed`; `import zed` captures them back the same way.

Config keys: `outputs.zed.skills-dir` (default `.agents/skills`), `outputs.zed.mcp-file` (default `.zed/settings.json`), `outputs.zed.tasks-file` (default empty, opt-in), `outputs.zed.rules-file` (unset; writes the legacy merged `.rules`-style document and skips the pointer-body write).

Verify with the real editor:

1. Install Zed from [zed.dev](https://zed.dev).
2. Check the tree: `ls AGENTS.md .agents/skills/ .zed/settings.json .zed/tasks.json`, `test -f .agents/skills/*/SKILL.md`, `python -m json.tool .zed/settings.json > /dev/null`, `python -m json.tool .zed/tasks.json > /dev/null`.
3. Open the project. The agent panel reads `AGENTS.md` as project instructions and lists each `.agents/skills/<name>/` as a skill (`@skill` / slash command).
4. The MCP picker shows each `context_servers.<name>` ready.
5. The command palette runs every entry from `.zed/tasks.json` as a Zed Task.

### Warp (`warp`)

```
AGENTS.md                              # canonical entry-point pointer body (written by sync, shared across the AGENTS.md consumers)
.warp/workflows/<name>.yaml            # one per agent, only when workflows-dir is set
.warp/.mcp.json                        # when MCP entries exist
```

- **Rules**: `sync` writes `AGENTS.md` with the pointer body plus a sentinel-marked `## Rules` block holding every rule body inline, shared byte-for-byte with codex, amp, zed, and crush so they coexist at the same path.
- **Workflows**: when `outputs.warp.workflows-dir` is set, each agent emits as a [Warp Workflow](https://docs.warp.dev/features/warp-drive/workflows) YAML at `<dir>/<name>.yaml` (`name`/`command`/`description`/`tags`). The `command:` is the agent body verbatim; tailor it to a Warp-friendly shell snippet. Other documented workflow fields (`shells`, `arguments`, `source_url`, `author`, `author_url`) pass through when declared under `x-warp`; `import warp` captures them back the same way.
- **Legacy rename**: Warp's current Rules docs specify `AGENTS.md` per the AGENTS.md standard. On first sync after upgrading, any agnostic-generated `WARP.md` at the configured root is renamed to `WARP.md.bak`. A user-authored `WARP.md` (no `Generated by agnostic-ai` marker) is left untouched.

Config keys: `outputs.warp.workflows-dir` (default empty, opt-in), `outputs.warp.mcp-file` (default `.warp/.mcp.json`), `outputs.warp.rules-file` (unset; writes legacy concatenated rules and skips the pointer-body write).

Verify with the real terminal:

1. Install Warp from [warp.dev](https://www.warp.dev).
2. Check the tree: `ls AGENTS.md .warp/workflows/ .warp/.mcp.json`, `grep "Generated by agnostic-ai" .warp/workflows/*.yaml` for the provenance header, `python -m json.tool .warp/.mcp.json > /dev/null`.
3. Open the project; confirm the rules panel surfaces `AGENTS.md` (no "unrecognized file" warnings), the Warp Drive workflows picker shows every `<workflows-dir>/<name>.yaml`, and the MCP picker lists every `mcpServers.<name>` from `.warp/.mcp.json`.

### OpenCode (`opencode`)

```
.opencode/AGENTS.md                       # canonical entry-point pointer body (written by sync)
.opencode/agents/<name>.md                # one native subagent definition per agent
.opencode/skills/<name>/SKILL.md          # one folder per skill, bundled assets included
.opencode/commands/<name>.md              # one per command spec
.opencode/commands/skill-<name>.md        # additional command form, only when emit-skills-as-commands: true
opencode.json                             # when MCP entries exist (merged with existing user config)
```

- **Routing**: under `.opencode/` rather than the repo root to avoid clashing with Codex's `AGENTS.md`.
- **Agents**: native [OpenCode agents](https://opencode.ai/docs/agents/) at `.opencode/agents/<name>.md` (plural dir; the singular is legacy): frontmatter filtered to `description`, `mode`, `model`, `temperature`, `permission`, with arbitrary `x-opencode` keys passing through. The body is the system prompt.
- **Skills**: native [OpenCode skills](https://opencode.ai/docs/skills/) folders at `.opencode/skills/<name>/SKILL.md` (OpenCode also scans `.claude/skills/` and `.agents/skills/`), with bundled sibling files propagated byte-for-byte. Set `outputs.opencode.emit-skills-as-commands: true` to additionally emit the command form.
- **Commands**: one markdown file per command spec under `.opencode/commands/<name>.md`, frontmatter filtered to the OpenCode command keys (`description`, `agent`, `model`, `subtask`).
- **MCP**: written into `opencode.json` at the project root with a `$schema` link and the `mcp` map. Stdio maps to `{type: "local", command: [...]}`; HTTP/SSE/remote maps to `{type: "remote", url, headers}`. A spec's `disabled: true` writes `"enabled": false`, the key [OpenCode's own MCP docs](https://opencode.ai/docs/mcp-servers/) document ("You can also disable a server by setting `enabled` to `false`"); an enabled server gets no key at all, and `import opencode` reads `enabled: false` back into `disabled: true`. Pre-existing non-managed keys (`theme`, `model`) are preserved; only `$schema` and `mcp` are overwritten. Drift checks (`sync --check`, `doctor`) read the existing file, so user keys never report as drift and `doctor --fix` keeps them.

Config keys: `outputs.opencode.agents-dir` (default `.opencode/agents`), `outputs.opencode.skills-dir` (default `.opencode/skills`), `outputs.opencode.commands-dir` (default `.opencode/commands`), `outputs.opencode.mcp-file` (default `opencode.json`), `outputs.opencode.emit-skills-as-commands` (default `false`), `outputs.opencode.rules-file` (unset; writes legacy concatenated rules and skips the pointer-body write).

Verify with the real CLI:

1. Install: `npm install -g sst/opencode` ([install docs](https://opencode.ai/)).
2. Check the tree: `ls .opencode/AGENTS.md .opencode/agents/ .opencode/skills/ .opencode/commands/ opencode.json`, `grep "Generated by agnostic-ai" .opencode/agents/*.md` for the provenance header (it sits after the frontmatter), `python -m json.tool opencode.json > /dev/null`.
3. Launch `opencode`. Every `.opencode/agents/<name>.md` appears in the agent picker, every `.opencode/skills/<name>/` in the skills list, and every `.opencode/commands/<name>.md` in the slash-command picker.
4. The MCP panel shows each `mcp.<name>` from `opencode.json` ready, with a disabled spec showing as disabled.

### Google Antigravity (`antigravity`)

```
.agent/AGENTS.md               # canonical entry-point pointer body (written by sync)
.agents/rules/<name>.md        # one per rule
.agents/rules/agent-<name>.md  # one per agent
.agents/skills/<name>/SKILL.md # one folder per skill (Antigravity's native path)
.agents/mcp_config.json        # when MCP entries exist
```

Antigravity reads project instructions from a top-level AGENTS.md-style file and per-rule files under `.agents/rules/`. The adapter emits per-rule files; `sync` writes the pointer body to `.agent/AGENTS.md`. The entry-point path stays under `.agent/` (singular) to avoid clashing with codex / amp / warp at the project-root `AGENTS.md`; rules, skills, and MCP default to the plural `.agents/` form Antigravity itself now prefers ([rules](https://antigravity.google/docs/rules-workflows), [skills](https://antigravity.google/docs/skills)), which still "maintains backward support" for the singular paths. A stale managed tree at the pre-plural `.agent/rules` / `.agent/skills` defaults is swept on sync unless `outputs.antigravity.rules-dir` / `skills-dir` opts back into the legacy path explicitly. `import antigravity` reads whichever of `.agents/rules` / `.agent/rules` exists, preferring the plural form.

- **Skills**: one folder per skill under `.agents/skills/<name>/SKILL.md` (Antigravity's [native skills layout](https://codelabs.developers.google.com/getting-started-with-antigravity-skills), one folder per skill; the same tree Codex, Amp, Zed, Crush, and OpenHands share, so identical skill folders dedupe). The SKILL.md frontmatter is reduced to `name` + `description`; the body follows. Sibling files next to the source SKILL.md (helper scripts, fixtures) are copied byte-for-byte into the emitted folder.
- **MCP**: servers land in `.agents/mcp_config.json` under a single `mcpServers` object ([antigravity.google/docs/mcp](https://antigravity.google/docs/mcp)). Remote servers carry `serverUrl`; the vendor doc states the legacy `url` / `httpUrl` field names "are not supported," so this is a dedicated schema, not the shared `mcpServers`-with-`url` shape claude and cursor use. stdio servers carry `command`, `args`, and `env`. `description`, `headers`, `roots`, `disabled`, and a `type` discriminant are not confirmed for Antigravity and are not emitted.

Hooks and commands are not yet confirmed in the Antigravity public-preview spec and skip with a warning. Add `on-unsupported: silent` to suppress it, or wait for a future release once the upstream spec stabilises.

Config keys: `outputs.antigravity.rules-dir` (default `.agents/rules`), `outputs.antigravity.skills-dir` (default `.agents/skills`), `outputs.antigravity.mcp-file` (default `.agents/mcp_config.json`), `outputs.antigravity.rules-file` (unset; writes a legacy merged document and skips the pointer-body write).

Verify with the real IDE:

1. Install Antigravity from the Google Antigravity public-preview download page.
2. Check the tree: `ls .agent/AGENTS.md .agents/rules/ .agents/skills/`, `grep "Generated by agnostic-ai" .agents/rules/*.md` for the provenance header (it sits after the frontmatter), `test -f .agents/skills/*/SKILL.md`, and `python -m json.tool .agents/mcp_config.json > /dev/null` when MCP specs exist.
3. Open the project; it surfaces `.agent/AGENTS.md` in the project-instructions panel with no "unrecognized file" warnings.
4. Open one of `.agents/rules/<name>.md` and verify the per-rule file is picked up; confirm each `.agents/skills/<name>/SKILL.md` loads as a skill, and any `.agents/mcp_config.json` server appears in the MCP panel.
5. Trigger an agent action (e.g. ask for a refactor); the rules apply, with no schema-validation log entries referencing `.agents/`.

### Junie (`junie`)

```
AGENTS.md                        # root entry-point pointer body (written by sync, shared path; fallback)
.junie/AGENTS.md                 # preferred entry-point: pointer body + inlined rules + inlined agents (written by this adapter)
.junie/skills/<name>/SKILL.md    # one folder per skill, plus any bundled assets
.junie/mcp/mcp.json              # when MCP entries exist
```

Junie's guidelines lookup is strict precedence, first match wins, not a merge: `.junie/AGENTS.md` ("the most preferred standard location"), then the root `AGENTS.md` "if no file is found in the `.junie` folder", then the legacy `.junie/guidelines.md` / `.junie/guidelines/` (junie.jetbrains.com/docs/junie-ide-plugin.html and guidelines-and-memory.html, target-audit 2026-08-08, #552). `sync` always writes `.junie/AGENTS.md`, so step 1 always matches and nothing after it is ever read in a synced project. Rule and agent bodies therefore inline directly into `.junie/AGENTS.md`: rules under a sentinel-marked `## Rules` block and agents under a sibling `## Agents` block, both immediately after the pointer body, using the same `### <name>` shape (source comment, optional description, full body) every other inlining target uses. A prior version of this adapter instead flattened rules and agents to one `.md` file each under `.junie/rules/`, a path outside Junie's documented lookup order that nothing ever read; any agnostic-ai-managed leftovers there are swept on sync (hand-authored files survive). Skills are unaffected by any of this: they emit into their own native folder tree at `.junie/skills/<name>/SKILL.md` (Junie's Native Agent Skills feature, shipped 2026-07-31): a flat file never loads as a skill, and bundled sibling assets copy byte-for-byte. MCP servers use the standard `mcpServers` schema at `.junie/mcp/mcp.json` (`command`/`args`/`env` local, `url`/`headers` remote).

Config keys: `outputs.junie.skills-dir` (default `.junie/skills`), `outputs.junie.mcp-file` (default `.junie/mcp/mcp.json`). `outputs.junie.rules-dir` (default `.junie/rules`) no longer controls where anything is written; it only redirects the legacy-tree sweep above, for a project that customized it before this fix. `.junie/AGENTS.md` is a fixed path, not configurable.

`import junie` reads `.junie/AGENTS.md`'s Rules and Agents blocks back into specs, falling back to the pre-fix `.junie/rules/` directory when that still exists on disk (a project synced by an older agnostic-ai version).

Verify with the real agent:

1. Install the Junie plugin in a JetBrains IDE (or the Junie CLI, [docs](https://junie.jetbrains.com/docs/)).
2. Check the tree: `ls AGENTS.md .junie/AGENTS.md .junie/skills/`, `grep "Generated by agnostic-ai" .junie/AGENTS.md`, `python -m json.tool .junie/mcp/mcp.json > /dev/null`.
3. Ask Junie to list its guidelines; the rule and agent bodies inlined in `.junie/AGENTS.md` apply, and each `.junie/skills/<name>/` folder appears as a skill.

### Kiro (`kiro`)

```
AGENTS.md                          # canonical entry-point pointer body (written by sync, shared path)
.kiro/steering/<name>.md           # one per rule (inclusion: always, or fileMatch + fileMatchPattern from globs)
.kiro/steering/skill-<name>.md     # one per skill (inclusion: auto + name + description)
.kiro/agents/<name>.md             # one per agent, Kiro's native agent-profile surface
.kiro/hooks/<name>.json            # one per hook, Kiro's native hook surface
.kiro/settings/mcp.json            # when MCP entries exist
```

AWS Kiro loads [steering files](https://kiro.dev/docs/steering/) whose YAML frontmatter must be the first content in the file. The adapter maps rule and skill spec kinds onto Kiro's inclusion modes: unscoped rules are `always`, globbed rules become `fileMatch` with `fileMatchPattern`, and skills are `auto` with `name` + `description` so Kiro activates them on matching requests. Kiro also reads the root `AGENTS.md` (always included), which carries the shared pointer body. Skills with bundled sibling assets surface a coverage note: a flat steering file cannot carry the assets.

Agents are [native custom agents](https://kiro.dev/docs/custom-agents/), not a steering-file convention: one YAML-frontmatter Markdown file per agent at `.kiro/agents/<name>.md`, the tree Kiro's agent picker reads. `description` (falls back to the agent name) and `model` pass through. Kiro's [documented agent schema](https://kiro.dev/docs/cli/custom-agents/configuration-reference/) also carries `tools`, `mcpServers`, `permissions`, `hooks`, `keyboardShortcut`, and `welcomeMessage`; only `tools` has an agnostic-ai spec equivalent, and this adapter does not emit it, since Kiro's own tool-identifier vocabulary is not documented anywhere and a Claude-style name would silently restrict nothing (the same failure class as Kilo Code and Augment). An agent that sets `tools` surfaces a coverage note instead. `name` is never written (absent from Kiro's schema; identity comes from the filename). Set `x-kiro.tools`, `x-kiro.mcpServers`, `x-kiro.permissions`, `x-kiro.hooks`, `x-kiro.keyboardShortcut`, or `x-kiro.welcomeMessage` directly once you know Kiro's own values; arbitrary `x-kiro` keys always pass through. A prior version of this adapter flattened agents into `.kiro/steering/agent-<name>.md` with `inclusion: manual`, which never reached the agent picker; `sync` sweeps a stale file of that shape left over from an older sync.

Hooks are [native](https://kiro.dev/docs/hooks/) too: one JSON file per hook spec, `{"version": 1, "hooks": [{name, trigger, matcher, action, timeout, enabled}]}`. `event` becomes `trigger`, passed through verbatim; `command` (string or list) becomes `action: {"type": "command", "command": ...}`, one entry per command sharing the file when the spec lists several, `name` suffixed `-2`, `-3`, ... to stay unique. `disabled: true` writes `"enabled": false`; the enabled default needs no key. Kiro also documents an `{"type": "agent", "prompt": ...}` action; agnostic-ai's hook spec has no prompt field, so this adapter never emits it. Unlike Claude Code, Codex, Gemini, and Cursor, stashed hook scripts under `.agnostic-ai/scripts/` do not materialize into `.kiro/hooks/`: that directory is where Kiro looks for hook definitions, not scripts.

Config keys: `outputs.kiro.rules-dir` (default `.kiro/steering`), `outputs.kiro.agents-dir` (default `.kiro/agents`), `outputs.kiro.hooks-dir` (default `.kiro/hooks`), `outputs.kiro.mcp-file` (default `.kiro/settings/mcp.json`).

Verify with the real IDE:

1. Install Kiro from [kiro.dev](https://kiro.dev).
2. Check the tree: `ls AGENTS.md .kiro/steering/ .kiro/agents/ .kiro/hooks/ .kiro/settings/mcp.json`, `head -2 .kiro/steering/*.md .kiro/agents/*.md` (frontmatter first, no leading blank lines), `python -m json.tool .kiro/settings/mcp.json > /dev/null`, and `python -m json.tool .kiro/hooks/*.json > /dev/null` for each hook file.
3. Open the project; the steering panel lists every rule and skill file with its inclusion mode and no parse warnings, and the agent picker lists every `.kiro/agents/<name>.md` profile.
4. Trigger a hook's `trigger` event (e.g. save a file for `PostFileSave`); the configured command runs with no schema warning.

### Crush (`crush`)

```
AGENTS.md                          # canonical entry-point pointer body + inlined rules (written by sync, shared path)
.agents/skills/<name>/SKILL.md     # one folder per skill (shared tree with codex/amp/zed)
crush.json                         # when MCP entries exist (merged with existing user config)
```

Charm [Crush](https://github.com/charmbracelet/crush) reads the root `AGENTS.md` natively and has no per-rule directory, so rule bodies inline into the shared entry-point. Skills emit into `.agents/skills/`, the first project path Crush scans; the render is byte-identical with codex/amp/zed so the shared tree dedupes. Set `x-crush.user-invocable: true` on a skill to also add it to Crush's command palette (ctrl+p). MCP servers merge into the `mcp` key of `crush.json` (`{type: stdio, command, args, env}` or `{type: http, url, headers, oauth, oauth_client_id, oauth_client_secret, oauth_callback_port}`, oauth fields optional, shipped in Crush v0.87.0); user-managed keys (`models`, `providers`, `lsp`, `options`) survive every sync. Agents have no Crush surface and skip with a warning.

Config keys: `outputs.crush.skills-dir` (default `.agents/skills`), `outputs.crush.mcp-file` (default `crush.json`).

Verify with the real CLI:

1. Install: `brew install charmbracelet/tap/crush` (or see the [README](https://github.com/charmbracelet/crush)).
2. Check the tree: `ls AGENTS.md .agents/skills/`, `python -m json.tool crush.json > /dev/null`.
3. Launch `crush`; the context loads `AGENTS.md`, the skills list shows each `.agents/skills/<name>/`, and each `mcp.<name>` connects.

### Trae (`trae`)

```
AGENTS.md                     # canonical entry-point pointer body (written by sync, shared path)
.trae/rules/<name>.md         # one per rule
.trae/rules/agent-<name>.md   # one per agent
.trae/skills/<name>/SKILL.md  # one folder per skill
.trae/commands/<name>.md      # one per command
```

ByteDance [Trae](https://docs.trae.ai/ide/rules) reads persistent rules from `.trae/rules/` and the root `AGENTS.md` natively. Agents flatten to rule-form files for now; Trae's dedicated custom-agent format moves here once its file layout is documented.

- **Skills**: one folder per skill under `.trae/skills/<name>/SKILL.md`, [Trae's native skills layout](https://docs.trae.ai/ide/skills). The SKILL.md frontmatter carries `name` + `description`; sibling assets (`examples/`, `templates/`, `resources/`) copy byte-for-byte alongside it. A flat file directly under `.trae/rules/` never loads as a skill, so this is a folder, not a rule-form file like agents.
- **Commands**: one `.md` per command under `.trae/commands/<name>.md`, `name` + `description` frontmatter and the body as the prompt. Trae's own docs do not cover the command format; this shape is confirmed from real `.trae/commands/*.md` files rather than a doc page, so only `name` and `description` are known-native and nothing else emits. Nesting under `.trae/commands/` is supported up to 3 levels and is organizational only.

Config keys: `outputs.trae.rules-dir` (default `.trae/rules`), `outputs.trae.skills-dir` (default `.trae/skills`), `outputs.trae.commands-dir` (default `.trae/commands`).

Verify with the real IDE:

1. Install Trae from [trae.ai](https://www.trae.ai).
2. Check the tree: `ls AGENTS.md .trae/rules/ .trae/skills/ .trae/commands/`, `grep "Generated by agnostic-ai" .trae/rules/*.md`.
3. Open the project; the Rules panel lists every `.trae/rules/*.md` with no parse warnings. Each `.trae/skills/<name>/` loads as a skill, and each `.trae/commands/<name>.md` is invokable from chat.

### Qoder (`qoder`)

```
AGENTS.md                     # canonical entry-point pointer body (written by sync, shared path)
.qoder/rules/<name>.md        # one per rule (native, one file per rule)
.qoder/agents/<name>.md       # one per agent (native, one file per agent)
.mcp.json                     # when MCP entries exist (shared with Claude Code)
```

Alibaba [Qoder](https://docs.qoder.com/user-guide/rules) reads project rules from `.qoder/rules/` natively, one Markdown file per rule, and also reads the root `AGENTS.md`. The per-rule files take precedence over `AGENTS.md`, so rules emit there rather than inlining into the shared pointer. Skills and hooks have no Qoder surface yet and skip with a warning. `import qoder` reads `.qoder/rules/*.md` back into rule specs and `.qoder/agents/*.md` back into agent specs.

- **Agents**: [Qoder Subagents](https://docs.qoder.com/extensions/subagent) reads `.qoder/agents/<name>.md`, one file per agent. `name` and `description` are required frontmatter; `model`, `tools`, `skills`, and `mcpServers` are optional. `tools` renders as a comma-separated string (`tools: Read, Grep, Bash`), the only form the vendor doc shows, not a YAML list; `import qoder` splits it back into agnostic-ai's generic list form so the spec stays usable by every other target. Qoder's built-in tool vocabulary is Claude-style (`Bash`, `Edit`, `Write`, `Glob`, `Grep`, `Read`, `WebFetch`, `WebSearch`), which is what makes passing agnostic-ai's generic `tools` list straight through safe here, unlike Kilo Code and Augment, whose own vocabularies differ and which drop the field with a coverage note instead. `skills` and `mcpServers` have no agnostic-ai-native shape and pass through whatever the spec declares.
- **MCP**: merges into `.mcp.json` under the standard `mcpServers` map (stdio: `command`/`args`/`env`, no `type`; remote: `type` + `url`/`headers`), byte-for-byte the same schema and the same literal path Claude Code already writes. Enabling both `qoder` and `claude` writes the file once; the sync collision check treats identical bytes at a shared path as a dedup, not a conflict. Qoder's own support for a per-server `disabled` key is not vendor-confirmed, and since the file is shared with Claude Code (which ignores the key there), agnostic-ai drops it for qoder too rather than risk the two targets disagreeing on one file. See [`disabled` support by target](spec-format.md#disabled-support-by-target).

Config keys: `outputs.qoder.rules-dir` (default `.qoder/rules`), `outputs.qoder.agents-dir` (default `.qoder/agents`), `outputs.qoder.mcp-file` (default `.mcp.json`).

Verify with the real IDE:

1. Install Qoder from [qoder.com](https://qoder.com).
2. Check the tree: `ls AGENTS.md .qoder/rules/ .qoder/agents/ .mcp.json`, `grep "Generated by agnostic-ai" .qoder/rules/*.md .qoder/agents/*.md` for the provenance header, `python -m json.tool .mcp.json > /dev/null`.
3. Open the project; the rules panel lists every `.qoder/rules/*.md` with no parse warnings, the agent picker lists every `.qoder/agents/*.md`, and the MCP picker shows each `mcpServers.<name>` from `.mcp.json` connected.

### OpenHands (`openhands`)

```
AGENTS.md                          # canonical entry-point pointer body + inlined rules (written by sync, shared path)
.agents/skills/<name>/SKILL.md     # one folder per skill (shared tree with codex/amp/zed/crush)
config.toml                        # when MCP entries exist
```

All Hands [OpenHands](https://docs.openhands.dev/overview/skills) reads the root `AGENTS.md` natively and loads skills from `.agents/skills/`, the same cross-tool tree codex, amp, zed, and crush emit. The render is byte-identical, so the shared tree dedupes into one write. OpenHands has no per-rule directory, so rule bodies inline into the shared `AGENTS.md` `## Rules` block. Agents and hooks have no OpenHands surface yet and skip with a warning.

- **MCP**: merges into `./config.toml` under a `[mcp]` table with three arrays instead of a `type` field: `stdio_servers` (`[[mcp.stdio_servers]]` tables carrying `name`/`command`/`args`/`env`) and `sse_servers` / `shttp_servers` (plain URL-string arrays; `shttp_servers` is OpenHands' streamable-HTTP transport, the cross-tool spec's `type: http`). A transport OpenHands documents no array for (e.g. `type: ws`) reaches neither array and surfaces a coverage note instead of guessing one. The project-tier `config.toml` is managed (its `[mcp]` table is overwritten each sync); keep unmanaged OpenHands config elsewhere.

Config keys: `outputs.openhands.skills-dir` (default `.agents/skills`), `outputs.openhands.mcp-file` (default `config.toml`).

Verify with the real CLI:

1. Install OpenHands ([docs](https://docs.openhands.dev/overview/skills)).
2. Check the tree: `ls AGENTS.md .agents/skills/ config.toml`, `test -f .agents/skills/*/SKILL.md`, `head -1 config.toml` for the provenance comment.
3. Launch OpenHands; the context loads `AGENTS.md`, each `.agents/skills/<name>/` appears as a skill, and each `[mcp]` server from `config.toml` connects.

### Factory (`factory`)

```
AGENTS.md                          # canonical entry-point pointer body + inlined rules (written by sync, shared path)
.factory/droids/<name>.md          # one custom-droid profile per agent
.factory/mcp.json                  # when MCP entries exist
```

Factory [Droid](https://docs.factory.ai/cli/configuration/custom-droids) reads the root `AGENTS.md` natively and loads custom droids from `.factory/droids/`. Each agent emits as one `<name>.md` profile with `name`, `description`, and optional `model` / `tools` frontmatter; arbitrary `x-factory` keys pass through. Factory has no per-rule directory, so rule bodies inline into the shared `AGENTS.md` `## Rules` block. Skills and hooks have no Factory surface yet and skip with a warning.

- **MCP**: merges into `.factory/mcp.json` under the standard `mcpServers` map, the same shape Claude Code and Cursor use (stdio: `command`/`args`/`env`, no `type`; remote: `type` + `url`/`headers`). Factory's schema documents a working per-server `disabled` boolean (default `false`), unlike Claude Code, Cursor, and Copilot, so agnostic-ai passes a spec's `disabled: true` straight through instead of stripping it. Factory also documents `disabledTools`, `timeout`, and `connectTimeout`, none of which the cross-tool MCP spec carries yet; they are not emitted. See [`disabled` support by target](spec-format.md#disabled-support-by-target).

Config keys: `outputs.factory.agents-dir` (default `.factory/droids`), `outputs.factory.mcp-file` (default `.factory/mcp.json`).

Verify with the real CLI:

1. Install the Factory CLI ([custom-droids docs](https://docs.factory.ai/cli/configuration/custom-droids)).
2. Check the tree: `ls AGENTS.md .factory/droids/ .factory/mcp.json`, `grep "Generated by agnostic-ai" .factory/droids/*.md` for the provenance header (it sits after the frontmatter), `python -m json.tool .factory/mcp.json > /dev/null`.
3. Launch `droid`; each `.factory/droids/<name>.md` appears in the droid picker and each `mcpServers.<name>` from `.factory/mcp.json` connects.

### Kilo (`kilo`)

```
AGENTS.md                          # canonical entry-point pointer body + inlined rules (written by sync, shared path)
.kilo/rules/<name>.md              # one per rule
.kilo/agents/<name>.md             # one per agent
.agents/skills/<name>/SKILL.md     # one folder per skill, plus any bundled assets (shared cross-tool tree)
kilo.jsonc                         # instructions array (one entry per rule) and/or mcp map (merged with existing user config)
```

Kilo [Code](https://kilo.ai/docs) reads the root `AGENTS.md` natively and loads agents from `.kilo/agents/`, one `<name>.md` per agent; Kilo Code takes the agent's name from the filename, so frontmatter carries only `description` and optional `model` (no `name:`, and no `tools:` — Kilo Code's full agent option table has no such key, so a spec's `tools` allowlist would silently do nothing; an agent with `tools` set surfaces a coverage note instead, and per-tool restriction goes through Kilo Code's native `permission` map via `x-kilo: {permission: {...}}`). Skills emit into the shared `.agents/skills/<name>/SKILL.md` tree (target-audit 2026-08-01): Kilo Code documents its own `.kilo/skills/` path, but also lists `.agents/skills/` as a compatibility directory "loaded by default", and that is the same tree codex, amp, zed, crush, openhands, windsurf, and augment already write byte-identically, so pointing here dedupes instead of adding a second on-disk copy.

Rules emit as one file per rule under `.kilo/rules/`, each one also listed by its own path in `kilo.jsonc`'s [`instructions`](https://kilo.ai/docs/customize/custom-rules) array (target-audit 2026-08-01): "Each entry points to a file path or glob pattern", and this adapter lists explicit paths rather than a `.kilo/rules/*.md` glob so a scoped rule nested under a subdirectory is never silently missed. Kilo Code's own [precedence order](https://kilo.ai/docs/customize/agents-md) is agent prompt > project `instructions` > AGENTS.md > global, so the `instructions` entry outranks the shared `AGENTS.md` block below it; AGENTS.md is always loaded when present regardless, so this adapter keeps inlining full rule bodies there too as a fallback, rather than treating `instructions` as a replacement. The legacy `.kilocode/rules/` tree (the pre-rename Kilo Code branding) is separate and still auto-included for backward compatibility, but this adapter never emits it.

MCP servers merge into the `mcp` map of `kilo.jsonc`: stdio combines `command`+`args` into one `command` array and sets `type: "local"`, using `environment` for env vars; remote sets `type: "remote"` and uses `url`/`headers`. A spec's `disabled: true` writes `"enabled": false`, the key Kilo Code's own documented MCP example carries; an enabled server gets no key at all. `instructions` and `mcp` merge into `kilo.jsonc` together; user-managed keys there survive every sync. Hooks have no Kilo surface yet and skip with a warning.

Config keys: `outputs.kilo.rules-dir` (default `.kilo/rules`), `outputs.kilo.agents-dir` (default `.kilo/agents`), `outputs.kilo.skills-dir` (default `.agents/skills`), `outputs.kilo.mcp-file` (default `kilo.jsonc`).

Verify with the real IDE:

1. Install Kilo Code ([docs](https://kilo.ai/docs)).
2. Check the tree: `ls AGENTS.md .kilo/rules/ .kilo/agents/ .agents/skills/ kilo.jsonc`, `grep "Generated by agnostic-ai" .kilo/rules/*.md .kilo/agents/*.md` for the provenance header.
3. Open the project; each `.kilo/rules/<name>.md` listed in `kilo.jsonc`'s `instructions` array appears in the loaded-rules list, each `.kilo/agents/<name>.md` appears in the agent picker, each `.agents/skills/<name>/` folder loads as a skill, and each `mcp.<name>` from `kilo.jsonc` connects, with a disabled spec showing as disabled.

### Jules (`jules`)

```
AGENTS.md                     # canonical entry-point pointer body + inlined rules (written by sync, shared path)
```

Google [Jules](https://jules.google/docs) is a cloud agent. It reads the root `AGENTS.md` and has no project-local surface of its own, so it contributes nothing but the shared pointer body and the inlined `## Rules` block. Enabling it adds no unique output, which is why it stays opt-in (see [Selecting targets](#selecting-targets)). Agents, skills, hooks, and MCP skip with a warning.

Config keys: none.

Verify with the real agent:

1. Sign in to Jules ([docs](https://jules.google/docs)).
2. Check the tree: `ls AGENTS.md`.
3. Point Jules at the repo; it reads `AGENTS.md` as project context.

### Goose (`goose`)

```
AGENTS.md                     # canonical entry-point pointer body + inlined rules (written by sync, shared path)
.goosehints                   # opt-in concatenated rules, only when rules-file is set
```

Block [Goose](https://goose-docs.ai) reads both the root `AGENTS.md` and a `.goosehints` file. By default rule bodies inline into the shared `AGENTS.md` `## Rules` block, so Goose needs no extra file. Set `outputs.goose.rules-file: .goosehints` to also write a concatenated `.goosehints` document. Goose adds no unique default output, so it stays opt-in (see [Selecting targets](#selecting-targets)). Agents, skills, hooks, and MCP skip with a warning.

Config keys: `outputs.goose.rules-file` (unset; opt-in, writes a concatenated `.goosehints` document).

Verify with the real CLI:

1. Install Goose ([docs](https://goose-docs.ai)).
2. Check the tree: `ls AGENTS.md`, plus `.goosehints` when `outputs.goose.rules-file` is set.
3. Launch `goose`; it reads `AGENTS.md` (and `.goosehints` when present) as context.

### Augment (`augment`)

```
AGENTS.md                     # canonical entry-point pointer body + inlined rules (written by sync, shared path)
.augment/
├── rules/<name>.md           # one per rule
└── agents/<name>.md          # one per agent
.agents/skills/<name>/SKILL.md  # one folder per skill (shared cross-tool tree)
.augment-guidelines           # opt-in legacy concatenated rules, only when rules-file is set
```

[Augment Code](https://docs.augmentcode.com/setup-augment/guidelines) reads the root `AGENTS.md`, with rule bodies inlined into the shared `## Rules` block, and also loads rules natively from `.augment/rules/`. Each rule frontmatter carries `type: agent_requested` (with a `description`, falling back to the rule name) when the spec sets `alwaysApply: false`; the vendor default `always_apply` stays implicit. There is no `name` key for rules. Agents load from `.augment/agents/`, one `<name>.md` per agent, with `name` (required), `description` (falls back to the spec name), `color`, and `model` when set. `tools` and `disabled_tools` are real Augment fields but only in Augment's own vocabulary (`view`, `codebase-retrieval`, `str-replace-editor`, ...), not Claude-style names, so agnostic-ai's generic `tools` field never reaches them: an agent that sets it surfaces a coverage note, and `x-augment: {tools: [...]}` / `x-augment: {disabled_tools: [...]}` is the way to reach Augment's real per-tool access control. Skills emit into the shared `.agents/skills/` tree, which Augment also scans directly alongside `.claude/skills/` and `.augment/skills/`. Set `outputs.augment.rules-file: .augment-guidelines` to additionally write the legacy concatenated document; the vendor's own precedence order truncates it first under budget pressure, which is why it stays opt-in rather than the default rules surface. Hooks and MCP skip with a warning.

Config keys: `outputs.augment.rules-dir` (default `.augment/rules`), `outputs.augment.agents-dir` (default `.augment/agents`), `outputs.augment.skills-dir` (default `.agents/skills`), `outputs.augment.rules-file` (unset; opt-in, writes the legacy concatenated `.augment-guidelines` document).

Verify with the real extension:

1. Install the Augment Code extension ([guidelines docs](https://docs.augmentcode.com/setup-augment/guidelines)).
2. Check the tree: `ls AGENTS.md .augment/rules/ .augment/agents/ .agents/skills/`, plus `.augment-guidelines` when `outputs.augment.rules-file` is set.
3. Open the project; Augment reads `AGENTS.md`, `.augment/rules/`, `.augment/agents/`, and `.agents/skills/` (and `.augment-guidelines` when present).

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

The default target set is 20: claude, codex, gemini, cursor, copilot, aider, cline, windsurf, continue, zed, opencode, antigravity, junie, kiro, crush, trae, qoder, openhands, factory, kilo. **Amp**, **Warp**, **Jules**, **Goose**, and **Augment** are opt-in, excluded from the default set so enabling them is a deliberate choice. Add them to `targets:` (or pass `-t amp,warp,jules,goose,augment`). Amp and Warp then emit their target-specific files (`.agents/`, `.amp/settings.json`, `.warp/`); Augment emits `.augment/rules/`, `.augment/agents/`, and shares `.agents/skills/` by default, plus the legacy `.augment-guidelines` document when `outputs.augment.rules-file` is set; Jules adds nothing beyond the shared pointer body; Goose writes its native rules file only when `outputs.goose.rules-file` is set.

Interactive `init` pre-ticks any target whose marker is present in the working directory (e.g. `.claude/`, `.codex/`, `.gemini/`, `.cursor/`, `.github/copilot-instructions.md`). The first-time sync prompt does the same. Toggle entries before confirming.

## New targets

See [adding-adapters](../internal/adding-adapters.md). ~50 lines plus one registry entry.

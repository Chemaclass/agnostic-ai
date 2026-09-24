# Upstream sources per target

Canonical vendor docs for every registered target. Auditors fetch from here instead of searching, which is the single biggest cost saver in a run: a fixed, known list of pages beats discovery every time.

One `## <target>` section is required per registered target, each with a `docs:` line and a `watch:` line. `tests/integration/target_audit_sources_test.go` enforces that, so a new adapter cannot merge without its vendor docs landing here too.

Keep each section's prose, everything but the `docs:` and `changelog:` lines, at 350 words or fewer; the same test enforces it. A section holds the URL lines, `watch:`, and one-line `quirk:` (fetch routes), `trap:` (a claim that once caused a wrong finding) and `decision:` (a choice already made) bullets. State a temporary gap as an open issue link, never as a standing fact. Do not append dated "verified on" notes or release summaries that changed nothing we emit. Git history, `scripts/target-audit/signals.tsv`, and the closed `target-audit` issues keep that record.

`docs` = the pages describing the file formats agnostic-ai emits. `changelog` = where new features land first; read it before the docs when hunting for "what changed since last audit".

## URL line grammar

`scripts/docfetch.sh` parses the `docs:` and `changelog:` lines, so they follow one grammar. Entries are separated by ` · `. A full URL sets the base for the entries after it on that line. `/path` resolves against that URL's origin, and `.../path` against its directory. Text in parentheses is commentary and is never fetched, and neither is anything inside backticks. Any other line in a section, including `watch:`, `quirk:`, and `trap:` notes, is prose for a human and is not fetched.

## What a fetch proves

Vendors move doc hosts often, so a 404 is itself a finding: record it as `docs-moved` and put the replacement URL in the report so this file gets patched.

**A 200 is not a correct entry.** One run found four entries whose URLs were live and whose named authority was dead, and each one had hidden a real finding from earlier runs. A URL health check cannot catch that class, so read the content of an entry you rely on, not its status code.

`scripts/docfetch.sh` runs the recovery ladder for client-rendered pages, moved URLs, app shells, and blocked hosts, and tags every row with the mode that produced its text. Read `.agnostic-ai/skills/target-audit/references/fetch-playbook.md` when a row comes back `failed`, `app-shell`, `soft-404`, or `redirected`. That file also holds the per-vendor routes and the fetch failures that have cost real audit time.

---

## claude

- docs: https://code.claude.com/docs/en/memory (rules) · /docs/en/hooks · /docs/en/sub-agents · /docs/en/skills (slash-commands merged in; `.claude/commands/` still works) · /docs/en/mcp · /docs/en/settings (prose on file precedence and reload) · /docs/en/settings-reference (the settings **key** table; that is the page an auditor needs, and it is a different page from /settings, both 200 as of 2026-09-11)
- changelog: https://github.com/anthropics/claude-code/blob/main/CHANGELOG.md
- watch: `.claude/rules/` native loading, settings.json keys, plugin and marketplace keys. The `.mcp.json` per-server fields `headersHelper`, `timeout`, `alwaysLoad` and `oauth`, emitted behind `emit.WithClaudeMCPExtras()`. The AGENTS.md fallback default, its Bedrock/Vertex/Foundry carve-out, the toggle gaining project scope, and `.claude/AGENTS.md` leaving the read list. Stable hook handler common fields, apart from command-only options.
- decision: `oauth.clientSecret` is not emitted. The secret "is stored securely in your system keychain ... not in your config".
- decision: experimental agent hooks stay excluded. Command, HTTP, MCP-tool and prompt handlers emit and import.
- quirk: when `/settings-reference` and the CHANGELOG disagree, prefer the CHANGELOG. The reference still documents `taskOutputMaxChars` without a marker, though v2.1.277 made it a no-op. It marks other keys deprecated, so a missing marker means stale.
- quirk: date a CHANGELOG entry by parsing the `## <version>` headings. Summarised fetches and flat dumps misdated it twice.
- decision: we keep emitting `taskOutputMaxChars` while npm `stable` sits below 2.1.277.
- trap: `.claude/settings.json` is not claude-only. Copilot CLI reads a five-key subset, `enabledPlugins` and `hooks` included (#956).
- trap: Claude does not read `AGENTS.local.md`, `AGENTS.override.md` or anything under `.agents/`.
- trap: Claude Code reads AGENTS.md when no CLAUDE.md exists (v2.1.277). So claude must always write CLAUDE.md, in every layout, or it inherits codex's AGENTS.md (#885). The user toggle cannot substitute: "Claude Code ignores it in project and local settings files."
- decision: `import claude` walks CLAUDE.md, `.claude/CLAUDE.md`, AGENTS.md, `.claude/AGENTS.md`; root AGENTS.md yields to codex, amp, warp, crush, kiro or opencode (#893). `doctor_unmanaged.go` keeps `{"AGENTS.md", "codex"}` single-owner on purpose.
- trap: "Claude Code mods" is not a documented surface. The docs name the built-in `agents-md` plugin under `pluginConfigs`, and `/docs/en/mods` 404s. Do not file on it. If it ships, it may be a new spec kind.

## codex

- docs: https://learn.chatgpt.com/docs/build-skills · /docs/agent-configuration/subagents · /docs/custom-prompts · /docs/hooks · /docs/config-file/config-reference · /docs/agent-configuration/rules (exec-policy precedence, not AGENTS.md discovery) · /docs/agent-configuration/agents-md (AGENTS.md discovery) · /docs/permissions (beta Permission Profiles, filesystem and network only)
- changelog: https://learn.chatgpt.com/docs/changelog
- watch: the skills dir, which moved `.codex/skills` to `.agents/skills`; prompts, deprecated in favour of skills; hooks JSON event names.
- trap: package-style MCP names (`/`, `@`, `:`) round-trip; do not re-file. `checkSpecName` no longer exists, and its only grep hits are stale `.claude/worktrees/` copies. The live check is `spec.ValidateName`, and `writeCodexMCPs` uses `spec.MCPFileName`. Non-bare TOML headers quote via `tomlKeySegment` (#706).
- trap: three permission surfaces, do not conflate them (#923). `approval_policy` is one global mode. Permission Profiles cover filesystem and network, with no ask verb. Exec-policy `prefix_rule` matches the portable lists, and we already write it through `outputs.codex.exec-policies`.
- trap: `model_reasoning_effort` is not a closed enum. `ReasoningEffort` in `codex-rs/protocol/src/openai_models.rs` parses any non-empty string as `Custom`. Treat the doc tier lists as guidance and re-pull that file if a value set is asserted.

## gemini

- docs: https://geminicli.com/docs/hooks/reference/ · https://geminicli.com/docs/core/subagents.md · https://geminicli.com/docs/reference/tools.md · https://geminicli.com/docs/cli/skills/ · /docs/cli/custom-commands/ · /docs/reference/configuration · /docs/cli/gemini-ignore/ (the ignore file Gemini CLI actually reads is `.geminiignore`; `.aiexclude` belongs to Gemini Code Assist, a different product, target-audit 2026-08-27)
- changelog: https://github.com/google-gemini/gemini-cli/releases
- watch: `.gemini/skills/` vs `.agents/skills` alias precedence, the `settings.json` hooks and mcpServers schema, the `/docs/core/subagents.md` frontmatter table for new fields, and whether `kind: remote` needs more than passthrough.
- trap: hooks need a nested `hooks` array; the v0.59.0 loader drops flat entries (`packages/core/src/hooks/hookRegistry.ts`, #762). Handler timeouts are milliseconds; `sequential` belongs to the definition.
- trap: subagent `tools` takes Gemini's snake_case names from `/docs/reference/tools.md` (`run_shell_command`, `replace`, `grep_search`), not Claude's. `geminiToolName` in `internal/adapters/gemini/agents.go` maps eight generic names and drops the rest with a coverage note. Re-check on tool renames (`grep_search` keeps `search_file_content` as legacy alias).
- decision: `.gemini/agents/*.md` emits since #733; the old per-agent TOML command survives behind `outputs.gemini.emit-agents-as-commands`.

## cursor

- docs: https://cursor.com/docs/skills · /docs/subagents · /docs/rules · /docs/hooks · /docs/mcp · /docs/bugbot · /docs/reference/third-party-hooks.md (the Claude Code hook-compatibility page; uncited until target-audit 2026-09-12, #756) · https://cursor.com/help/customization/skills.md (the old `/docs/agent/chat/commands` 308s here, to a "migrate commands to skills" FAQ; no Cursor page documents `.cursor/commands` any more, target-audit 2026-08-27)
- changelog: https://cursor.com/changelog
- watch: `.mdc` frontmatter fields; camelCase hook events (`beforeShellExecution`); environment.json schema; the Third-Party Imports default flipping or being renamed; `.cursor/hooks.json` changing rank; a skill precedence rule or a new compatibility root.
- trap: Cursor loads Claude Code hooks. `/docs/reference/third-party-hooks.md` ranks `.cursor/hooks.json` 3rd and `.claude/settings.json` 6th, and "All matching hooks from every source run." Syncing claude and cursor runs every hook twice. Gated by "Include Third-Party Plugins, Skills, and Other Configs", on by default. Do not assert whether camelCase names inside `.claude/settings.json` fire; the mapping covers PascalCase only.
- trap: Cursor also loads `.claude/skills/`, `.codex/skills/` and their `~/` forms, same switch (#957). Say "read from three roots, precedence undocumented", never "loaded three times". The non-merging sentence on `/docs/skills` is about Codex's loader.
- decision: cursor stdio MCP entries emit `type` (the field table marks it required, #895). Keep it cursor-only: Claude Code reads a missing `type` as stdio, and the shared builder serves claude, kiro, junie, qoder, factory and copilot.
- trap: `/docs/mcp` rows **Roots** as "Supported". That is the protocol capability, not a config key; no per-server field table has `roots`.
- quirk: `cursor.com/docs/<page>.md` times out on curl, so it looks dead. Use `https://r.jina.ai/https://cursor.com/docs/<page>`.
- note: prompt hooks emit `type`, `prompt`, optional `model` and common options (#768).

## copilot

- docs: https://docs.github.com/en/copilot/reference/custom-agents-configuration · https://docs.github.com/en/copilot/how-tos/copilot-on-github/customize-copilot/customize-cloud-agent/add-skills · https://code.visualstudio.com/docs/agent-customization/mcp-servers (MCP; VS Code's docs are authoritative for `.vscode/mcp.json`, docs.github.com does not cover it) · https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-mcp-servers (the Copilot CLI MCP page; this is the one that names the accepted top-level keys) · https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-config-dir-reference (Copilot CLI user, repository, and local settings plus repository model policy) · https://code.visualstudio.com/docs/agents/reference/mcp-configuration (the VS Code-only MCP fields `targets/copilot.md` cites) · https://docs.github.com/en/copilot/concepts/agents/hooks · https://docs.github.com/en/copilot/reference/hooks-reference (14 events) · https://docs.github.com/en/copilot/how-tos/copilot-on-github/customize-copilot/customize-cloud-agent/use-hooks (the default-branch requirement)
- changelog: https://github.blog/changelog/label/copilot/
- watch: `applyTo` semantics in `.github/instructions/*.instructions.md`; agent frontmatter keys; `cli-config-dir-reference` precedence, its fourteen repository keys (a `permissions` key retires the MDM note) and the model allowlist format; the hooks table (14 rows against 12 PascalCase spellings); cloud agent gaining `exec`; a sixth key in the `.claude/settings.json` cross-read subset; any page naming the `.mcp.json` top-level key or a cloud-agent `.github/mcp.json`.
- decision: Copilot CLI reads `.mcp.json` and `.github/mcp.json`, not `.vscode/mcp.json` ("It uses the unsupported top-level key `servers`"). We write `.github/mcp.json`; root `.mcp.json` is opt-in via `outputs.copilot.root-mcp-file` because claude shares the root (#646).
- decision: hooks pass `event:` through verbatim, since the page accepts camelCase and PascalCase. Wrapper is `{"version": 1}` with an integer; the field is `timeoutSec`.
- trap: `userPromptTransformed` and `subagentStart` are camelCase-only, so `SubagentStart` parses and never fires. The validator's event table is separate from the emitter's `hookLifecycle`; check both (#888).
- trap: `agentStop` pairs with `Stop`, not `AgentStop`. Grepping `AgentStop` finds nothing; no pairing was dropped.
- trap: hook sources combine, and the CLI also reads `.claude/settings.json` for five keys, `enabledPlugins` and `hooks` among them. One spec targeting claude and copilot runs twice (#755, #956).
- trap: `exec`/`args` are "Only supported in Copilot CLI"; cloud agent honors only `bash`/`command`.
- trap: there is no project `.copilot/settings.json`. Every mention is `~/.copilot/settings.json`; the repo file is `.github/copilot/settings.json`.
- trap: permissions are MDM-only (#917). Unlisted repository keys "are silently ignored", and `~/.copilot/permissions-config.json` "doesn't support deny rules".
- trap: no VS Code page documents a per-server `roots` key. When a field claim repeats across targets, check `BuildRoots` in `emit/mcp.go` and every page it feeds.
- decision: `disabledMcpServers` in `.github/copilot/settings.json` is the disable route; `mcp.json` has no `disabled` key.
- decision: hook `cwd` and `env` stay copilot-scoped. Do not share the MCP `tools` allowlist with codex, whose `tools` is a map of sub-tables.
- decision: preserve top-level `inputs` and `sandbox` in `.vscode/mcp.json`; CLI and root mirror files are wholly managed (#757). `dev.debug` is stdio only (#769).

## aider

- status: dormant. `main` last moved 2026-05-22 (sha 5dc9490). aider.chat is a Jekyll site in `aider/website/`, so `gh api repos/Aider-AI/aider --jq '.pushed_at'` bounds the whole target. Re-open the full sweep if it moves past 2026-05-22.
- docs: https://aider.chat/docs/usage/conventions.html · https://aider.chat/docs/config/aider_conf.html
- changelog: https://aider.chat/HISTORY.html
- watch: whether Aider gained any per-file rules/skills surface (today it is conventions + `.aiderignore` only).
- trap: `releases/latest` returns v0.86.0 and misleads. Aider stopped publishing Releases but kept tagging (newest v0.86.2 / v0.86.3.dev). Read tags.

## cline

- docs: https://docs.cline.bot/customization/cline-rules · /customization/skills · /getting-started/config (append `.md` to any path for a clean markdown mirror; `llms.txt` lists every page)
- changelog: https://github.com/cline/cline/releases
- quirk: the source of truth for project paths is `sdk/packages/shared/src/storage/paths.ts` in `cline/cline`, the `resolve*ConfigSearchPaths` functions. Read it before trusting `/getting-started/config`. Print it with `gh api repos/cline/cline/contents/sdk/packages/shared/src/storage/paths.ts --jq '.content' | base64 -d`.
- trap: `apps/vscode/src/core/storage/disk.ts` (`GlobalFileNames`) is no longer the authority. `GlobalFileNames.clineRules` has zero call sites. Cite it only for what the VS Code extension alone resolves.
- trap: rules live in two layouts, `.clinerules` and `.cline/rules`. Source: `resolveWorkspaceRulesConfigPaths`, "Every Cline surface ... must honor both". `.clinerules` stays our default because the VS Code Rules panel creates there. Do not call `.clinerules` deprecated: `resources/deprecations.md` does not list it.
- trap: agents at `.cline/agents/` must be `.yml`/`.yaml` with `---` frontmatter carrying `name` and `description`. Source: `configured-agent-config.ts` (`isYamlFile`, `ConfiguredAgentFrontmatterSchema`), repeated in `cline-hub` and `cli`. A file that fails the schema is dropped silently.
- trap: the skills doc and source disagree. The doc lists `.claude/skills`, the source (`getWorkspaceSkillDirectories`) lists `.agents/skills`. We emit to `.cline/skills`, which both name, and import reads all four. Inert, do not re-decide.
- decision: workflows emit to `.clinerules/workflows`. Only that path reaches the VS Code extension; `.cline/workflows` does not. `CLINERULES_EXCLUDED_SUBDIRECTORIES` keeps workflows, hooks and skills out of the rules scan.
- decision: hooks emit one executable script per event under `.cline/hooks` (#889). Source: `hook-file-config.ts`, ten event names. `/customization/hooks` is still a stub, so this is source-confirmed only. `PreCompact` maps to `undefined` in `HOOK_CONFIG_FILE_EVENT_MAP`, so it never runs.
- decision: `.cline/cron/` is not a surface. `resolveWorkspaceCronSpecsDir` is "reserved for future" use and scope defaults to `global`.
- quirk: `docs.cline.bot/features/workflows` 404s. `.clineignore` is vendor-flagged deprecated; no impact, cline declares no `KindIgnore`.
- watch: `paths.ts` resolvers for a project path we do not write, a fifth skills directory, or a VS Code reader for `.cline/workflows`. `isYamlFile` and the agent zod schema for a new required key. `HookConfigFileName` for an eleventh event, `PreCompact` gaining a runtime event, `/customization/hooks` gaining content. `.cline/plugins/` (unemitted, no schema). MCP config location. A second rule conditional beyond `paths`, since `alwaysApply: false` with no globs has no representation.

## windsurf

- docs: https://docs.devin.ai/desktop/cascade/memories · https://docs.devin.ai/desktop/cascade/mcp · https://docs.devin.ai/desktop/cascade/agents-md · https://docs.devin.ai/cli/subagents · https://docs.devin.ai/cli/reference/permissions (the complete `allowed-tools` vocabulary) · https://docs.devin.ai/cli/extensibility/hooks/overview · https://docs.devin.ai/desktop/cascade/skills · https://docs.devin.ai/desktop/cascade/workflows · https://docs.devin.ai/desktop/context-awareness/devin-ignore · https://docs.devin.ai/desktop/devin-local · https://docs.devin.ai/cli/extensibility/rules · https://docs.devin.ai/cli/extensibility/skills/overview · https://docs.devin.ai/cli/extensibility/skills/creating-skills.md (the full SKILL.md frontmatter reference) · https://docs.devin.ai/cli/extensibility/mcp/configuration
- changelog: https://docs.devin.ai/desktop/changelog.md (`windsurf.com/changelog` 308s to docs.devin.ai and the rendered page is client-side, so the `.md` mirror is the only fetchable form) · https://docs.devin.ai/cli/changelog/stable.md (the **CLI** release stream, read for the first time on 2026-09-11, #737: four of six windsurf outputs, `.devin/agents/`, `.devin/hooks.v1.json`, `.devin/mcp_config.json`, and `AGENTS.md`, are Devin CLI surfaces, and the Desktop changelog above never covers them)
- watch: the `.devin/rules/` read path after the Devin rebrand, and whether rule discovery reads subdirectories inside `.devin/rules/`, where a scoped rule lands. Also watch `/cli/subagents` for format changes (vendor calls custom subagents "experimental"), `/cli/reference/permissions` for a sixth `allowed-tools` name, and `Exec` for an exact-command form.
- quirk: Cascade is removed (changelog v3.9.19: "Devin Local is now the only agent available in Devin Desktop"). The `/desktop/cascade/*` and `/desktop/devin-local` pages lag it; trust the changelog.
- decision: Workflows went with Cascade, so `outputs.windsurf.workflows-dir` no longer emits and only warns (`warnWorkflowsDirRemoved` in `windsurf.go`). The workflows URL stays listed to catch a return; do not report the retired surface as missing.
- decision: `.devinignore` covers indexing and agent file access; `.windsurfignore` and `.codeiumignore` are legacy names "still read and enforced alongside `.devinignore`" (#1115, reverses the 2026-09-18 reading in #863). One Ignore spec still writes both, because older Windsurf builds honored only `.windsurfignore` for agent access; `outputs.windsurf.ignore-file` moves the main file only.
- trap: `allowed-tools` and `permissions` vocabularies differ. `allowed-tools` is `read`/`edit`/`grep`/`glob`/`exec` plus `mcp__<server>__<tool>`. `permissions` also takes `web_search` (CLI v3000.10.21). Keep `devinTool` (agent.go) and `devinPermissionTool` (settings.go) separate. `webfetch` stays out of both; only a sentence about `permissions` licenses an entry.
- decision: `Exec` prefix-matches, so an exact `Bash(cmd)` widens. A widened deny emits (CLI v3000.10.31: "A command deny such as `Exec(rm)` blocks the command"); allow and ask drop.
- decision: `.devin/config.json` takes only `permissions`, `read_config_from` and `hooks` in project scope, so portable `model` stays out with a coverage note.
- hazard: `/cli/subagents` also reads `.agents/agents/`, the tree antigravity, goose and openhands write, so Devin sees two profiles per agent. A shared file is blocked on `model` (Devin model ID vs antigravity tier enum); see `target-behavior.md`.
- quirk: `.devin/hooks.v1.json` tool names are lowercase snake_case, so a Claude-style matcher surfaces a coverage note. `.agents/skills/` is a documented compatibility path behind `.windsurf/skills/`.

## continue

- status: **wound down.** continue.dev reads "Continue has joined Cursor". The docs still resolve and the adapter still emits. Baseline: `continuedev/continue` `main` head 5522c6f, 2026-07-21. Compare `repos/continuedev/continue/commits/main` against it, not `pushed_at`, which tracks any branch. Low priority until `main` moves.
- docs: https://docs.continue.dev/customize/deep-dives/rules · /customize/deep-dives/mcp · /reference (all client-rendered with no `.md` mirror and no `llms.txt`; the working source is `continuedev/continue` at `docs/customize/deep-dives/*.mdx`)
- changelog: https://github.com/continuedev/continue/releases
- trap: do not bound activity with `pushed_at`. It tracks any branch, dependabot included. Read `repos/continuedev/continue/commits/main` instead.
- quirk: the MCP prose page names transports but not field placement. Read `packages/config-yaml/src/schemas/mcp/index.ts` for `type` literals and `requestOptions` nesting (#726, #730).
- decision: `outputs.continue.assistants-dir` is an opt-in export. Its shape matches `configYamlSchema`, but no doc says Continue scans a project directory for it.
- decision: `.continue/mcpServers/*.yaml` files hold exactly one server. JSONC `.json` files import as a root `mcpServers` map or a bare server (`loadJsonMcpConfigs.ts`).
- watch: a sixth rule frontmatter key in `docs/customize/deep-dives/rules.mdx`. A hub successor to `.continue/rules/`, since `/hub/` is gone. The MCP zod schema. A vendor-confirmed discovery path for assistants.

## amp

- docs: https://ampcode.com/llms.txt (the index; it lists every docs page and each link serves raw markdown) · https://ampcode.com/docs/customize/agents-md · /docs/customize/skills · /docs/customize/global-plugins-and-skills · /docs/customize/mcp · /docs/customize/plugins · /docs/cli/settings · /docs/tools · /docs/the-dial · /docs/orbs/customizing · /docs/orbs/portals
- changelog: https://ampcode.com/chronicle (`ampcode.com/news` 307s here; individual posts keep `/news/<slug>`)
- quirk: `https://ampcode.com/cli-settings.schema.json` is the authoritative `.amp/settings.json` key list. Diff it instead of `/docs/cli/settings` prose, which lists a subset.
- trap: `ampcode.com/manual` returns 200 but serves an empty SvelteKit shell. Do not cite it.
- blocker: `/docs/tools` lists no tool names; it defers to `amp tools list`. A tool table there would unblock mapping portable `deny` onto `amp.tools.disable` (written today via `x-amp`, #950). Portable `allow` and `ask` stay blocked: "By default, Amp does not ask for approval before running tools."
- watch: `.agents/skills/` (shared with codex/zed/crush), `amp.mcpServers` and `amp.tools.disable` in `.amp/settings.json`, and the orb files `.agents/setup`, `.agents/resume`, `.amp/services.yaml`, plus the tools table above.
- decision: `install` maps to `.agents/setup`, `terminals` to `.amp/services.yaml` (#637). `.agents/resume` stays unmapped: it runs on every wake with thread credentials, which no Environment field models.
- trap: `.agents/commands/` is retired (custom commands removed 2026-01-29). Agents reach amp only through `outputs.amp.rules-file` (#727). Re-file only on a vendor page naming a file-based agent or command path; plugins are TypeScript.
- trap: `.agents/checks/` is gone; `/docs/customize/checks` 404s and `llms.txt` has no `checks/` hit. Do not file a reviews emitter.
- decision: `includeTools` is implied, never shown, inside `amp.mcpServers`, so `x-amp` carries it (#634). A vendor example would settle a top-level mapping.

## zed

- docs: https://zed.dev/docs/ai/skills · /docs/ai/mcp · /docs/tasks · /docs/ai/instructions
- changelog: https://zed.dev/releases
- watch: `context_servers` (not `mcpServers`) key; a lifecycle-hook surface appearing; `.rules` leaving the instruction lookup list, which is the only reason to move the entry point back to `AGENTS.md`.
- quirk: read `crates/settings_content/src/project.rs`, not `/docs/ai/mcp`, for per-server fields. Every `ContextServerSettingsContent` variant has `enabled` (default true), plus `timeout`/`oauth` on `Http` and `remote` on `Stdio`/`Extension`. `enabled` emits from `disabled: true`; the rest go through `x-zed` (#641).
- trap: `configuring-zed.md` says project settings are editor-only, and the all-settings reference omits `context_servers`. Both are wrong: it is a `ProjectSettingsContent` field.
- trap: `context_server_timeout` beside `context_servers` is user-managed; our merge replaces only the `context_servers` map, so it survives. Not drift.
- decision: entry point is `.rules` (rank 1 of nine in `/docs/ai/instructions`, first match wins). Copilot's pointer file at rank 5 outranked `AGENTS.md` at rank 7 (#624).
- decision: `x-zed` Task passthrough is a generic merge (`MergeCustomTargetMeta`), not a field list, because `/docs/tasks` keeps growing fields (#563).
- note: the rules library was retired in 1.4.2. Skill names are 1-64 lowercase alphanumerics with single hyphens; sync validates it (#766).

## warp

- docs: https://docs.warp.dev/terminal/entry/yaml-workflows · https://docs.warp.dev/agents/capabilities/skills · https://docs.warp.dev/agents/capabilities/mcp
- changelog: https://docs.warp.dev/changelog/2026.md (dated release entries; advance the year when a new annual page appears) · https://docs.warp.dev/changelog/ (year index, retained to detect new annual pages; the old getting-started path redirects here)
- watch: a native rules-dir surface (today: AGENTS.md, `.agents/skills/`, workflows, `.warp/.mcp.json`); another move of the MCP page (it 308s from `/knowledge-and-collaboration/mcp`); whether either MCP table gains `description`, `disabled` or `roots`.
- fact: Warp indexes ten default skill directories, `.agents/skills/` recommended (#590). `.opencode/skills/` is one, so Warp reads OpenCode skills with no extra write. Native `.agents/skills/` folders with bundled assets import (#765).
- trap: the env var is `WARP_SKILL_DIRS`, not `SKILLS_DIRS` (#663). It serves Cloud agents indexing skills outside the repo. It does not extend the ten defaults.
- fact: the MCP command table has `command`/`args`/`env`/`working_directory` (Warp's name for `cwd`, #606). The URL table has only `url`/`headers`, no discriminant (#592).
- decision: we emit no `description`, `disabled` or `roots` (#641). `disabled` raises a coverage note; the other two go through `x-warp`.

## opencode

- docs: https://opencode.ai/docs/agents/ · /docs/skills/ · /docs/mcp-servers/ · /docs/commands/ · /docs/rules/ · /docs/policies/ (the experimental `experimental.policies` array in `opencode.json`, which gates which providers and resources OpenCode may use; separate from permissions, which gate what tools may do) · /docs/plugins/ (`.opencode/plugins/*.ts`, the hook surface) · /docs/permissions/ (the top-level `permission` key in `opencode.json`, the page that settles what the portable lists map onto)
- trap: `/docs/permissions/` makes `webfetch`/`websearch` look pattern-capable, but the exclusion at `internal/adapters/opencode/permission.go:47` is right. `/docs/agents/`: "The remaining keys accept the shorthand action only." `$defs.PermissionConfig` agrees.
- changelog: https://github.com/anomalyco/opencode/releases (the repo moved from `sst`; the redirect still works, and the default branch is `dev`)
- watch: `agents/` (plural) dir; which foreign skill trees it scans; the `mcp` block shape; new entries in the `/docs/plugins/` "Events" section; `experimental.session.compacting` losing its prefix; a published full tool list.
- trap: rules lookup is an upward walk for files named exactly `AGENTS.md` (`fs.up({ targets: ["AGENTS.md"] })` in `packages/core/src/instruction-context.ts`, branch `dev`). Nothing reads `.opencode/AGENTS.md`, so the entry point is root `AGENTS.md` (#623). Global `~/.config/opencode/AGENTS.md` is out of project scope.
- decision: agent `tools` is vendor-deprecated for `permission`, which we already emit. Not a gap.
- note: MCP `cwd` (local) and `timeout` (ms) emit (#641); remote `oauth` goes through `x-opencode`.
- decision: hooks are codegen in `.opencode/plugins/`, two shapes (#892): a direct `"tool.execute.before"` key and one `event` hook switching on `event.type`. `experimental.session.compacting` and `shell.env` get a coverage note: they rewrite output, not run a command. Vendor tool names are lowercase (`bash`, `read`), so Claude-cased matchers get a coverage note.
- decision: the top-level `permission` key in `opencode.json` is written by `internal/adapters/opencode/permission.go` (#922 closed). `Edit` and `Write` collapse to `edit`, globs are bare (`git *`, not `Bash(git:*)`), and rules are last-match-wins, so they flatten into one ordered array. `/docs/policies/` is a different feature.
- trap: MCP permissions DO map (#947). `/docs/agents/` matches keys against tool names "for built-ins, custom tools, and MCP tools". `mcp__<server>__<tool>` becomes `<server>_<tool>`, server name verbatim, as kilo does. Only whole-server rules stay noted: `spec.SplitPermissionRule` needs both halves.
- quirk: resolve the schema root `$ref` to `#/$defs/Config` before reading `$defs.PermissionConfig`; the bare root once produced "no permission key". Read `/docs/agents/` too; it carries permission detail `/docs/permissions/` omits.

## antigravity

- docs: https://antigravity.google/docs/rules (moved from `/docs/rules-workflows?tab=ide` on 2026-09-24; one page covers all three surfaces, and the `.md` mirror serves clean Markdown) · /docs/skills?tab=ide · /docs/mcp?tab=ide · /docs/hooks?tab=ide · /docs/plugins · /docs/slash-commands · /docs/ide/workflows (still on the old path, and deprecated: "Workflows are being deprecated in favor of **Agent Skills** by November 1, 2026") · https://antigravity.google/docs/subagents (the `/docs/ide/subagents` variant 404s; this is the real path) · https://codelabs.developers.google.com/getting-started-with-antigravity-skills
- changelog: https://antigravity.google/changelog (live, updates frequently; splits into four product tabs, Hub/2.0, IDE, CLI, and SDK, each on its own version track, confirmed 2026-08-09, #590; filter to the IDE tab, since that is what this adapter targets)
- watch: MCP fields beyond stdio `command`/`args`/`env`/`cwd`, remote `serverUrl`/`headers`, and `disabled`. `description`, `roots` and a `type` discriminant are unconfirmed and omitted on purpose; do not file them as missing. A fourth subagent `model` tier beyond `inherit`/`flash`/`pro`. Docs catching up on `inheritCustomizations`, the `rules:` key and `skills.json`/`agents.json`/`rules.json` discovery; they appear only in Hub changelog prose, so do not model them.
- quirk: append `.md` to a docs path for a clean markdown mirror. `changelog.md` 404s, but the changelog HTML reads fine with curl plus a tag-strip. Pass `--compressed`; one edge returned raw brotli.
- quirk: old `/docs/ide/rules|skills|mcp|hooks` are refresh stubs to the `?tab=ide` pages. `https://antigravity.google/llms.txt` lists the canonical set.
- decision: every rule starts with `trigger` frontmatter; a file without it is silently discarded. Mapping follows windsurf and cursor: `alwaysApply` true or unset is `always_on`, false with `globs` is `glob`, false with `description` is `model_decision`, false alone is `manual` (#1113, supersedes #865).
- decision: `.agents/AGENTS.md` is the documented per-subdirectory entry point (`<dir>/.agents/AGENTS.md`), still clear of codex/amp/warp's root `AGENTS.md` (#1114, corrects #865, which predates the docs page naming it).
- decision: a scoped rule emits to `<scope>/.agents/rules/<name>.md`, the vendor's own directory-scoped discovery; never nested deeper than one level (#1114).
- trap: hooks (`.agents/hooks.json`) run in the IDE: `transcriptPath` resolves under `~/.gemini/antigravity-ide`. Do not re-open.
- decision: subagents emit to nested `.agents/agents/<name>/agent.md` (#717). Never write a generic `tools`: the vocabulary (`view_file`, `run_command`) has zero Claude overlap and an unmapped name can hang the subagent. Use `x-antigravity.tools`.
- decision: global skills go to `~/.gemini/config/skills/`; `~/.gemini/antigravity/skills/` is legacy (#896).
- trap: `/docs/slash-commands` says `/learn` writes `.antigravity/rules.md`. Every rules page says `.agents/rules/`. Likely stale copy; only running `/learn` in the IDE settles it. Do not file on the prose.
- note: a rules file caps at 24,000 bytes and truncates over it, a coverage note (#1114, corrects the earlier 12,000-character/undocumented-outcome reading). A separate 20,000-token aggregate budget across `always_on` rules demotes the largest to a path-plus-description pointer; docs-only, no note.

## junie

- docs: https://junie.jetbrains.com/docs/starting-page-get-started-with-junie.json (machine-readable getting-started topic) · /docs/agent-skills.html · /docs/junie-ide-plugin.html (guidelines lookup order) · /docs/guidelines-and-memory.html (CLI guidelines lookup order, confirms the same order) · /docs/junie-cli-subagents.html (CLI-only native subagents, `.junie/agents/` or `.agents/`, live since 2026-03-10) · /docs/custom-slash-commands.html (CLI-only native slash commands, `.junie/commands/`, live since 2026-04-13) · /docs/junie-cli-hooks.html · /docs/junie-cli-mcp-configuration.html · /docs/junie-cli-extensions.html · /docs/junie-cli-configuration.html · /docs/junie-review-agent.html (the `/review` slash command and `junie --review`; a read-only subagent that "reads project guidelines and any code-review-related agent skills", so our skills and guidelines reach it) · https://junie.jetbrains.com/docs/custom-llm-models.html (project profiles under `.junie/models/*.json`) · /docs/environment-variables.html (the fullest statement of the guidelines lookup) · /docs/junie-ide-plugin.html again for `.aiignore` (the only page that names it) · /docs/action-allowlist-junie-cli.html (the allowlist, documented at `~/.junie/` only; reachable from `HelpTOC.json`, not from the visible nav) · /docs/junie-cli-demo.html (listed in `HelpTOC.json`, uncited until target-audit 2026-09-20)
- changelog: https://plugins.jetbrains.com/api/plugins/26104/updates?size=20 (Marketplace release versions, dates, and notes) · https://junie.jetbrains.com/blog/
- watch: the guidelines lookup is first match wins, not a merge (#552): `.junie/AGENTS.md`, then root `AGENTS.md` (combined with `.junie/playbook.md` and `.junie/rules/*.md`), then legacy `.junie/guidelines.md`. We always write `.junie/AGENTS.md`, so a change to step 1 revives everything below it. Also watch the `.junie/mcp/mcp.json` schema, a fourth skills root, a CLI page naming `.aiignore`, and an allow/deny/ask key in the project config table.
- quirk: the getting-started HTML and Marketplace pages serve app shells to curl. Read the topic JSON, the Marketplace updates API and `HelpTOC.json` instead.
- trap: audits missed the subagents and slash-command pages by only checking what we emit (#604, #605). Every audit, diff `HelpTOC.json` against the list above for new pages.
- trap: the subagent frontmatter table and the "Configure subagent usage" toggle (Early Access, `/settings` only) differ; only the toggle is caveated.
- decision: hooks stay declined. They are Early Access, and project-local hooks in `.junie/config.json` "are ignored by default".
- decision: `.aiignore` is emitted (#728) but documented on the IDE page only; import must tolerate its absence. Enforcement is soft: Junie asks for approval, and Brave Mode bypasses it.
- decision: skills also load from `.agents/skills/`, shared with ten other targets; `--skill-default-locations false` disables it.
- decision: no project-tier permissions (#917). The allowlist lives at `~/.junie/allowlist.json` only, and the project config table has no rule list. The adapter raises a coverage note.

## kiro

- docs: https://kiro.dev/docs/steering/ · /docs/mcp/ · /docs/mcp/configuration/ · /docs/hooks/ · /docs/hooks/types/ · /docs/hooks/actions/ · /docs/skills/ · /docs/custom-agents/ · /docs/custom-agents/configuration-reference/ · /docs/tools/ · /docs/powers/ · /docs/powers/installation/ · https://kiro.dev/docs/kiroignore/ · https://kiro.dev/docs/cli/v3/hooks-migration/ (the vendor's own current link for the 2.x-to-3.0 hook migration page; the `/docs/cli/v3/hooks/` form recorded below still resolves to the same page, both 200 through the proxy on 2026-09-20)
- changelog: https://kiro.dev/changelog/ (the slashless form 301s here)
- watch: steering `inclusion:` values (`always` / `fileMatch`); the agent `tools` category vocabulary (`read`/`write`/`shell`/`web`/`subagent`/`knowledge`/`todo_list`, plus `@server_name`/`@mcp`/`@builtin`/`*`), which `kiroToolCategory` in kiro.go maps onto; a new row in the `/docs/hooks/types/` triggers table; a `type` discriminant on remote MCP entries (none documented, we emit `"type": "http"`); the `/docs/kiroignore/` Capability table, since enforcement differs by surface; a project-level powers directory.
- quirk: kiro.dev returns 403 to curl and WebFetch. Fetch through `https://r.jina.ai/https://kiro.dev/<path>` before calling kiro unauditable.
- quirk: the old `/docs/cli/custom-agents/configuration-reference/` path is an HTTP 200 meta-refresh stub, not a 3xx. Inspect with `curl -D -` before calling a page gone.
- trap: take trigger casing from `/docs/hooks/actions/`, never from display names. "Prompt Submit" is `UserPromptSubmit`, "Pre Task Execution" is `PreTaskExec`.
- trap: camelCase triggers (`agentSpawn`, `preToolUse`, `fileEdited`) are the 2.x format inside agent config. 3.0 uses PascalCase `trigger` values in standalone `.kiro/hooks/*.json`, which we emit. Tell them apart by schema keys, never page dates: `trigger` in a `version: v1` envelope is 3.0.
- trap: the 11 v1 triggers are `SessionStart`, `Stop`, `PreToolUse`, `PostToolUse`, `PreTaskExec`, `PostTaskExec`, `UserPromptSubmit`, `PostFileCreate`, `PostFileSave`, `PostFileDelete`, `Manual`, all in `hookEventsByTarget["kiro"]`. camelCase is the 2.x agent-config format; documented renames include `agentSpawn` to `SessionStart`, `fileEdited` to `PostFileSave`, `fileCreated` to `PostFileCreate` (#907). Legacy Manual Hook is correctly absent. `Manual` looks ruled out, but that sentence covers IDE 0.x hooks. The CLI page lists it as current.
- trap: `AgentSpawn` was documented when #660 landed. 3.0 dropped it; it is not a misread.
- decision: hook `version` is the string `"v1"`, and the adapter writes it as a string (`hooksFile.Version` in `kiro/hooks.go`).
- decision: MCP `oauth` and `oauthScopes` (remote only), `autoApprove` and `disabledTools` emit behind `emit.WithKiroMCPExtras()` (#634). Skills write natively to `.kiro/skills/` (#642). Hook `description` emits generically, `confirm` only via `x-kiro`.
- decision: a valid `x-kiro.action` works without a command and replaces the fallback command list; invalid actions error (#772). An omitted timeout uses the vendor default.
- decision: we emit no powers. Powers install per user with no committed project path. Agents reach installed powers through `x-kiro.includePowers` (#1068).

## crush

- docs: https://github.com/charmbracelet/crush (README is the reference) · https://raw.githubusercontent.com/charmbracelet/crush/main/schema.json (the vendor's published JSON schema, and the only place the MCP property set appears closed) · https://github.com/charmbracelet/crush/blob/main/docs/hooks/README.md
- changelog: https://github.com/charmbracelet/crush/releases (the feed carries a rolling `nightly` tag whose timestamp bumps daily while its body stays static artifact-verification boilerplate; it reads like a daily ship and is not one, so compare against the newest real version tag, target-audit 2026-09-07)
- watch: the `crush.json` `mcp` block; agents or commands surfaces; `crushrc` / `.crushrc`, the Bash config format now preferred over JSON (#674); a second hook event beyond `PreToolUse`.
- quirk: the rolling `nightly` release bumps daily with static boilerplate. Compare against the newest real version tag.
- decision: map MCP fields explicitly. `schema.json` sets `"additionalProperties": false` on `MCPConfig`, so a passthrough typo breaks the config.
- decision: `timeout` stays unmapped. Crush reads seconds (default 10); the spec uses milliseconds, so 5000 would mean 5000 seconds.
- decision: `hooks.PreToolUse` merges into the same `crush.json` write as `mcp` (#629). Matchers are lowercase (`^bash$`); a Claude-style matcher raises a coverage note.
- decision: `.crushignore` at the project root is emitted and imported (#770); subdirectory files are excluded.

## trae

- docs: https://docs.trae.ai/ide/rules · https://docs.trae.ai/ide/model-context-protocol · https://docs.trae.ai/ide/add-mcp-servers · https://docs.trae.ai/ide/skills · https://docs.trae.ai/ide/subagents · https://docs.trae.ai/ide/slash-commands · https://docs.trae.ai/ide/hook-configuration-reference · https://docs.trae.ai/ide/automate-actions-with-hooks · https://docs.trae.ai/ide/ignore-files
- changelog: https://www.trae.ai/api/changelog (primary JSON feed, latest entry 2026-09-01) · https://docs.trae.ai/ide/changelog (documentation alternative, latest entry 2026-08-19; checked 2026-09-23 by extracting the router payload's text, which lags the JSON feed rather than stopping in 2025)
- quirk: WebFetch and a `.md` suffix both fail; the page is an SPA shell. Curl to disk, brace-match `window._ROUTER_DATA`, and collect the Quill delta's `ops[].insert` per `zoneId`.
- quirk: walk `window._ROUTER_DATA`'s nav tree for the full page list. The sidebar missed `/ide/ignore-files` and `/ide/automate-actions-with-hooks` for three audits.
- quirk: `docs.trae.ai/ide/mcp` 302s to a marketing page. The live MCP pages are `model-context-protocol` and `add-mcp-servers`.
- decision: `.trae/mcp.json` omits `type` and `disabled` because neither doc example shows them.
- decision: a generic `model:` on subagents is dropped when the "Available models" table lacks the value.
- trap: the hook `tool_name` vocabulary is not the subagent one. The terminal tool is `RunCommand`, not `Bash`; `LSP` and `TodoWrite` are absent. A Claude-style matcher matches nothing. `hooks.go` keeps its own list (#729).
- trap: Trae can read Claude Code hooks from `.claude/settings.json`. With both targets on, enabling that runs every hook twice. It is off by default ("Toggle the **Import Hook configuration in CLAUDE** switch on").
- watch: both MCP pages stay live and gain no `type` or `disabled`. The subagent Beta toggle's default and the "Available models" table. Hook schema `version` (only 1 supported) and `loop_limit`. The six hook events. The Claude hook import switch defaulting on, which makes it a finding.

## jules

- docs: https://jules.google/docs
- changelog: https://jules.google/docs/changelog/
- watch: today AGENTS.md only; any per-file surface is new.

## goose

- docs: https://github.com/aaif-goose/goose/blob/main/documentation/docs/guides/context-engineering/using-goosehints.md · .../using-skills.md · .../custom-agents.md · .../hooks.md · .../plugins.md (the Open Plugins package shape: manifest, component directories, and the project `.agents/plugins/<plugin-name>/` location) (the GitHub `documentation/docs/` tree is the working source; the rendered site at goose-docs.ai is not fetchable and its `llms.txt` lists no skills, agents, or hooks page)
- changelog: https://github.com/aaif-goose/goose/releases
- watch: `CONTEXT_FILE_NAMES` (default `AGENTS.md`, `.goosehints`) for a third name, the `plugins.md` component table for a third component, project plugins moving off `.agents/plugins/`, and recipes or extensions becoming project-scoped.
- quirk: `github.com/block/goose` 301s to `aaif-goose/goose`. Citation refresh only, not a docs-moved finding.
- decision: scoped rules route to nested `<scope>/.goosehints` through goose's nested hint discovery (#608).
- decision: slash commands stay declined. They are user-tier only, under `slash_commands:` in `~/.config/goose/config.yaml`.
- decision: `plugins.md` is the plugin authority, not the hooks page. "A plugin can provide skills, hooks, or both", so `plugin.json` follows a skills-only bundle too (#862).
- trap: custom-agent frontmatter is `name`, `description`, `model` only. Check the `AgentMetadata` struct in `crates/goose/src/agents/platform_extensions/summon.rs`, not the doc table. No `deny_unknown_fields`, so stray keys are ignored. This settled #864 against `color` in the shared `.agents/agents/` renderer.
- quirk: the agent loader skips nested directories, so Antigravity coexists at `<name>/agent.md` (#717).
- quirk: review checks read root `.agents/REVIEW.md` and `<scope>/.agents/REVIEW.md`, and they compose (`crates/goose/src/checks/mod.rs`, #771).

## augment

- docs: https://docs.augmentcode.com/setup-augment/guidelines · https://docs.augmentcode.com/cli/subagents · /cli/config · /cli/rules · /cli/skills · /cli/hooks · /cli/custom-commands · /cli/integrations · https://docs.augmentcode.com/cli/permissions (repository `toolPermissions` in `.augment/settings.json`; append `.md` to any path for a markdown mirror)
- changelog: https://www.augmentcode.com/changelog
- watch: `.augment-guidelines` vs AGENTS.md; whether `.augment/rules/` exists; a fifth permission type or a path matcher on `read`/`write`, either of which retires a coverage note.
- quirk: append `.md` to any docs path for a markdown mirror.
- decision: project MCP and hooks merge into `<workspace>/.augment/settings.json` in one `MergeJSONFile` call (#633, #629). `/cli/config` calls it the home for "shared MCP servers". That file is CLI-only; the IDE extensions use their own Settings Panel.
- decision: commands emit to `.augment/commands/<name>.md`, nested directories as namespaces, with `description`, `argument-hint` and `model` (#630).
- fact: hooks have five events (`PreToolUse`, `PostToolUse`, `Stop`, `SessionStart`, `SessionEnd`). `Notification` appears in `hook_event_name` but has no config section, so it is not a sixth. `timeout` is milliseconds, default 60000. `command` must end in `.sh`/`.ps1`/`.cmd`/`.bat` or Augment never runs it.
- fact: tool matchers use Augment's own names (`launch-process`, `str-replace-editor`, ...), so a Claude matcher matches nothing. Session events omit the matcher key.
- trap: `/cli/permissions` marks `launch-process`, `view`, `str-replace-editor` and `save-file` as legacy aliases of `terminal`, `read`, `edit` and `write`. That rename is permissions-only. `/cli/subagents` and `/cli/hooks` still use the old names. Check which page a name came from; do not rename hook matchers.
- fact: `toolPermissions` (#856) is an ordered array, first match wins. `permission` must be an object with `type`; a bare string is dropped. Types are `allow`, `deny`, `webhook-policy`, `script-policy`, with no prompt type, so `ask` raises a coverage note. Only `terminal` takes a matcher (`shellInputRegex`), so a path-scoped rule raises one too. MCP tools are named `{tool-name}_{server-name}`, truncated at 64 characters.
- decision: the optional rule field `eventType` (`tool-call` default, `tool-response`) is omitted. `x-augment` is the route for the response phase.

## qoder

- docs: https://docs.qoder.com/user-guide/rules · https://docs.qoder.com/extensions/subagent · https://docs.qoder.com/extensions/skills · https://docs.qoder.com/cli/subagent · https://docs.qoder.com/cli/mcp-servers · https://docs.qoder.com/cli/Skills · https://docs.qoder.com/user-guide/chat/model-context-protocol · https://docs.qoder.com/cli/mcp-reference · https://docs.qoder.com/cli/hooks-reference · https://docs.qoder.com/cli/hooks · https://docs.qoder.com/cli/commands · https://docs.qoder.com/user-guide/commands · https://docs.qoder.com/cli/settings (the canonical page for `.qoder/settings.json`, and the source of the JSONC statement behind #725) · https://docs.qoder.com/cli/permissions (project `permissions.allow`, `permissions.deny`, and `permissions.ask`) · https://docs.qoder.com/cli/settings-reference (every `settings.json` key with type, default, and whether it needs a restart; the fastest way to spot a new block) · https://docs.qoder.com/cli/config-scope (which of the three files a key belongs in: `~/.qoder/settings.json`, `<project>/.qoder/settings.json`, `<project>/.qoder/settings.local.json`) · https://docs.qoder.com/cli/memory (the auto-memory store paths; `docs/site/content/docs/target-behavior.md` claims Qoder cannot relocate it, and that claim had no tracked page until 2026-09-22)
- changelog: https://docs.qoder.com/release-notes/qoder-cli.md and /release-notes/desktop.md (`qoder.com/changelog` is client-rendered and defeats both WebFetch and payload extraction; the docs release notes are plain markdown and split CLI from IDE)
- watch: `.qoder/rules/`, `.qoder/agents/`, `.qoder/skills/<name>/SKILL.md`, `.qoder/commands/<name>.md`, `.qoder/settings.json`; whether `.agents/skills/` becomes a listed compatible path (not yet, #558); a new row on `/cli/hooks-reference`; the User-Level-wins command precedence on `/cli/commands`; a file-configurable form of the in-process `sdk` MCP transport; the sentence on `/cli/hooks` that `shell` is ignored when `args` is set, in case the pair becomes an error.
- quirk: `docs.qoder.com/llms.txt` needs `curl --compressed`.
- quirk: the CLI (`/cli/*`) and IDE trees document two products. Keep both.
- quirk: read `/cli/mcp-reference` for MCP, not `/cli/mcp-servers`. Nine of its ten keys map explicitly; `qoder_url` passes through `x-qoder` (`MergeCustomTargetMeta` in `qoder/mcp.go`). Not a gap.
- quirk: count hook events on `/cli/hooks-reference` (27), not `/cli/hooks`; the pages disagree. All 27 are in `hookEventsByTarget["qoder"]` (#744). Hook `timeout` is seconds, default 600.
- trap: "Local Subagent Full Field Reference" means `kind: local`, not the user tier. It is the project-tier frontmatter reference.
- fact: subagent `memory` (`user`, `project`, `local`) emits since #953, markdown only. It is inert unless top-level `autoMemoryEnabled` (default `false`) is on. Documented, not runtime-verified.
- decision: comma-separated `tools` and `disallowedTools` is our choice. The vendor also accepts arrays (#563).
- decision: MCP lives in `.qoder/settings.json` under `mcpServers`, not `.mcp.json` (#641). An old `.mcp.json` still outranks it and needs manual removal.
- decision: the `args` exec form emits (#746). The adapter writes `shell` anyway with a no-op note.
- decision: commands go to `.qoder/commands/<command_name>.md` with `description` required and `name` cosmetic (#630).
- decision: the `sdk` transport has no command or URL, so we emit nothing.

## openhands

- docs: https://docs.openhands.dev/overview/skills · /overview/skills/path · https://docs.openhands.dev/openhands/usage/settings/mcp-settings · /openhands/usage/customization/hooks · /openhands/usage/customization/repository · /sdk/guides/agent-file-based (append `.md` to any path for a markdown mirror)
- changelog: https://github.com/OpenHands/OpenHands/releases (the org renamed; the recorded `All-Hands-AI/OpenHands` URL 301s here and still resolves, corrected 2026-09-11, #737)
- watch: the `.agents/skills/` shared tree, and whether Cloud documents the same automatic agent registration as `LocalConversation`.
- trap: microagents are resolved. The vendor heads the section "Skills (formerly Microagents)" and says "use `.agents/skills/` for new skills".
- decision: path-triggered rules emit in folder form at `.agents/skills/<name>/SKILL.md`, avoiding loose `.md` files in a shared tree (#643).
- decision: `.openhands/hooks.json` uses the Claude-compatible PascalCase wrapper, with a coverage note for OpenHands matchers (`file_editor`, `terminal`).
- quirk: the agent loader skips subdirectories, so Antigravity coexists at `.agents/agents/<name>/agent.md` (#717).
- decision: agent `color` stays unemitted with a coverage note (#864). Goose shares `.agents/agents/` and has no such key. Use `x-openhands.color`. Re-open only if Goose adds `color` or OpenHands stops sharing the path.

## factory

- docs: https://docs.factory.ai/harness/subagents · /harness/skills · /harness/hooks · /harness/custom-slash-commands · /harness/mcp · /droid-cli/settings (the `settings.json` **key** reference: every key, its options, and its default. Its own location table lists `~/.factory/settings.json` alone. **Do not read that as a user-tier-only file and stop**, which is how this pairing hid the missing project-tier emitter for three runs) · /enterprise/hierarchical-settings-and-org-control (**the page that settles the project tier, and it is real**: "Settings are authored in `.factory/` folders, using the **same schema** at every level", with the levels table row "| **Project** | `<git-root>/.factory/` | Repo maintainers |" and "Each `.factory/` folder can contain: - `settings.json`: general settings (models, safety, preferences, telemetry)." Corroborated on /harness/skills: "The **User** tab writes the choice to `disabledSkills` in `~/.factory/settings.json`; the **Project** tab writes to `<project>/.factory/settings.json`." Read this page for scope and `/droid-cli/settings` for keys, never the reverse. It carries the org-managed schema and the precedence order too, and the org half stays out of our reach) · /enterprise/llm-safety-and-agent-controls (**the only page with the command-pattern examples**, and the one that names the sandbox's extra network keys) · https://docs.factory.ai/autonomy-and-safety/sandbox (the `sandbox.*` full reference; the hierarchical page is captioned "Where `sandbox.*` lives") · https://docs.factory.ai/ (append `.md` to any path for a markdown mirror)
- changelog: https://docs.factory.ai/changelog/release-notes.md (not linked from the docs nav, but listed in `docs.factory.ai/llms.txt` under `## Changelog`)
- watch: `.factory/droids/` frontmatter keys, project-scoped MCP config, removal of the legacy `.factory/commands/` loader, and the `tools` ID table.
- trap: `/droid-cli/settings` lists only `~/.factory/settings.json`. The project tier is real; read the hierarchical page for scope.
- trap: portable `deny` maps to `commandBlocklist`, not `commandDenylist`. The denylist prompts: "A denied command can still be run if you explicitly approve it." Portable `ask` maps to `commandDenylist`. Check `internal/adapters/factory/settings.go` before re-filing.
- trap: `tools` is not a Claude pass-through. The closed table rejects unknown IDs, so `Bash`/`Write`/`WebFetch` translate to `Execute`/`Create`/`FetchUrl`. `TodoWrite` and `Skill` stay unlisted; `tools: all` is rejected.
- decision: `sandbox.*` gets no portable kind (kernel enforcement, no `ask` tier, egress filtering). Reach it through `x-factory` on a settings spec (#949).
- decision: skills go to the shared `.agents/skills/` tree, which the vendor lists as "Compatibility". Its own tier is `.factory/skills/`. Framing, not drift.
- quirk: hooks.json is keyed by event with no `hooks` wrapper, nine events, `timeout` in seconds.
- quirk: MCP `timeout` and `connectTimeout` are milliseconds; remote `oauth` fields emit through a Factory-only mapping (#774).

## kilo

- docs: https://kilo.ai/docs (client-rendered; use WebFetch) · /docs/customize/custom-rules · /docs/customize/agents-md · /docs/customize/agent-permissions (the `permission` map `x-kilo` reaches: actions, patterns and precedence, **no tool names**) · /docs/automate/tools (the closed tool vocabulary, mirrored at `packages/kilo-docs/pages/automate/tools/index.md`) · /docs/getting-started/settings/auto-approving-actions (the same names as `permission` keys in `kilo.jsonc`) · /docs/code-with-ai/agents/goals (the reserved `goal` agent name, #736) · /docs/customize/custom-subagents · /docs/customize/skills (the `skills.paths` / `skills.urls` config keys, the `/.github/skills` absolute-then-project-relative fallback, and the three default skill trees, target-audit 2026-09-18 #861, #865; mirrored at `Kilo-Org/kilocode`'s `packages/kilo-docs/pages/customize/skills.md`) (raw source also mirrored at `Kilo-Org/kilocode`'s `packages/kilo-docs/pages/customize/custom-subagents.md`, useful when the rendered site defeats fetching; appending `.md` to a kilo.ai docs path used to serve that raw source directly but now 404s there, confirmed 2026-08-09, #590, so use the GitHub mirror instead).
- changelog: https://github.com/Kilo-Org/kilocode/releases (kilo.ai defeats fetching, so cite the GitHub mirror under `packages/kilo-docs/pages/`)
- quirk: kilo.ai `.md` paths 404. Use the GitHub mirror and try `<page>/index.md` before calling a page gone.
- trap: read shared-board releases in order. v7.7.0 and v7.7.1 still name `experimental.shared_agent_board`; v7.7.2, same day, moves it to top-level `shared_agent_board`.
- trap: tool names are not on `customize/agent-permissions`. They are on `automate/tools/index.md` and `auto-approving-actions`.
- watch: the `.kilo/plugin/` module shape (`export default { id, server }`, #1105), `kilo.jsonc` `mcp` shape, `.kilo/agents/` vs custom modes, `.kilocode/rules/` compatibility, a second reserved command name beyond `goal`, a fourth default skill tree, and `shared_agent_board`.
- decision: root `kilo.jsonc` loses to `.kilo/kilo.jsonc`, but configs merge by key, so only redeclared keys shadow. Documented, not moved (#644).
- decision: one `instructions` path per rule; glob recursion is unconfirmed.
- decision: `disable`/`hidden`/`steps`/`temperature`/`top_p` are `x-kilo` only by design (#562).
- decision: a `skills-dir` outside the three default trees writes `skills.paths` (#861).
- decision: `.kilocodeignore` is emitted natively; no invented permission translation (#773). MCP OAuth objects beyond `oauth:false` stay out (#775).

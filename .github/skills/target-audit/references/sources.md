# Upstream sources per target

Canonical vendor docs for every registered target. Auditors fetch from
here instead of searching, which is the single biggest cost saver in a
run: a fixed, known list of pages beats discovery every time.

One `## <target>` section is required per registered target, each with a
`docs:` line and a `watch:` line.
`tests/integration/target_audit_sources_test.go` enforces that, so a new
adapter cannot merge without its vendor docs landing here too.

Every URL below returned HTTP 200 on **2026-08-01**. Vendors move doc
hosts often (the Codex skills path moved twice in 2026), so a 404 is
itself a finding: record it as `docs-moved` and put the replacement URL
in the report so this file gets patched.

`docs` = the pages describing the file formats agnostic-ai emits.
`changelog` = where new features land first; read it before the docs
when hunting for "what changed since last audit".

Some vendor sites render client-side (goose, kilo, antigravity, trae).
`curl` returns 200 with an empty body there; use WebFetch or WebSearch
instead and do not conclude "page is empty" from a curl body.

---

## claude

- docs: https://code.claude.com/docs/en/memory (rules) · /hooks · /sub-agents · /skills (slash-commands merged in; `.claude/commands/` still works) · /mcp · /settings
- changelog: https://github.com/anthropics/claude-code/blob/main/CHANGELOG.md
- watch: `.claude/rules/` native loading, settings.json key surface, plugin/marketplace keys.

## codex

- docs: https://learn.chatgpt.com/docs/build-skills · /docs/agent-configuration/subagents · /docs/custom-prompts · /docs/hooks · /docs/config-file/config-reference · /docs/agent-configuration/rules (exec-policy precedence, not AGENTS.md discovery) · /docs/agent-configuration/agents-md (AGENTS.md discovery)
- changelog: https://learn.chatgpt.com/docs/changelog
- watch: skills dir has moved twice (`.codex/skills` -> `.agents/skills`); prompts are deprecated in favour of skills; hooks JSON event names.

## gemini

- docs: https://geminicli.com/docs/cli/skills/ · /docs/cli/custom-commands/ · /docs/reference/configuration
- changelog: https://github.com/google-gemini/gemini-cli/releases
- watch: `.gemini/skills/` vs the `.agents/skills` alias precedence, `settings.json` hooks + mcpServers schema.

## cursor

- docs: https://cursor.com/docs/skills · /docs/subagents · /docs/rules · /docs/hooks · /docs/mcp · /docs/bugbot · /docs/agent/chat/commands
- changelog: https://cursor.com/changelog
- watch: `.mdc` frontmatter fields, hooks event names (camelCase, e.g. `beforeShellExecution`), environment.json schema.

## copilot

- docs: https://docs.github.com/en/copilot/reference/custom-agents-configuration · https://docs.github.com/en/copilot/how-tos/copilot-on-github/customize-copilot/customize-cloud-agent/add-skills · https://code.visualstudio.com/docs/agent-customization/mcp-servers (MCP; VS Code's docs are authoritative for `.vscode/mcp.json`, docs.github.com does not cover it)
- changelog: https://github.blog/changelog/label/copilot/
- watch: `.github/instructions/*.instructions.md` `applyTo` semantics, `.vscode/mcp.json` vs a repo-level MCP file, agent frontmatter keys. Enable/disable state is stored outside `mcp.json`, so there is no per-server `disabled` key.

## aider

- docs: https://aider.chat/docs/usage/conventions.html · https://aider.chat/docs/config/aider_conf.html
- changelog: https://aider.chat/HISTORY.html
- watch: whether Aider gained any per-file rules/skills surface (today it is conventions + `.aiderignore` only).

## cline

- docs: https://docs.cline.bot/customization/cline-rules · /customization/skills · /getting-started/config
- changelog: https://github.com/cline/cline/releases
- watch: whether `.clinerules/` is fully dropped from `/getting-started/config` (still absent there as of 2026-08-01; this adapter treats it as a live fallback via `outputs.cline.rules-dir`, not a removal); `.cline/hooks/`, `.cline/plugins/`, `.cline/cron/` (listed in the config reference, unemitted, no confirmed schema); MCP config location. `docs.cline.bot/features/workflows` 404s and `llms.txt` lists no project-scoped replacement (target-audit 2026-08-08, #563): the current `customization/` tree covers Rules, `.clineignore`, Hooks, Plugins, and Skills only, no Workflows entry, so `outputs.cline.workflows-dir` is an unconfirmed export until a current doc backs it (see targets.md).

## windsurf

- docs: https://docs.windsurf.com/windsurf/cascade/memories · https://docs.windsurf.com/windsurf/cascade/mcp · https://docs.devin.ai/desktop/cascade/skills · https://docs.devin.ai/desktop/cascade/workflows · https://docs.devin.ai/desktop/context-awareness/windsurf-ignore · https://docs.devin.ai/desktop/devin-local · https://docs.devin.ai/cli/extensibility/rules · https://docs.devin.ai/cli/extensibility/skills/overview · https://docs.devin.ai/cli/extensibility/mcp/configuration
- changelog: https://windsurf.com/changelog
- watch: the adapter writes `.devin/rules/`; confirm that is still the read path after the Devin rebrand, and whether MCP config became project-scoped. `.agents/skills/` is a documented cross-agent-compatibility path behind Devin Desktop's own `.windsurf/skills/` (target-audit 2026-08-08, #563); re-check if that framing ever flips. Also watch `.devinignore` vs the legacy `.codeiumignore`/`.windsurfignore` names for a precedence change. The doc site also covers a separate Devin CLI / Devin Local Agent product tree (`/desktop/devin-local`, `/cli/extensibility/rules`, `/cli/extensibility/skills/overview`, `/cli/extensibility/mcp/configuration`), uncited here until now (confirmed 2026-08-09, #590); it documents a project-scoped `.devin/mcp_config.json` this adapter does not emit yet (#587).

## continue

- docs: https://docs.continue.dev/customize/deep-dives/rules · /customize/deep-dives/mcp · /reference
- changelog: https://github.com/continuedev/continue/releases
- watch: the `/hub/` namespace is gone from the doc source entirely (target-audit 2026-08-08, #563; confirmed by listing `continuedev/continue`'s `docs/` tree), so `.continue/rules/` vs any hub-successor surface if one reappears, `.continue/mcpServers/*.yaml` schema, and whether `outputs.continue.assistants-dir` ever gains a vendor-confirmed native discovery path (today: an opt-in export whose file shape matches `promptSchema`/`configYamlSchema` in `continuedev/continue`'s `packages/config-yaml/src/schemas/index.ts` and `/reference`'s `name`/`version`/`schema: v1`, but no doc says Continue scans a project directory for it; see targets.md).

## amp

- docs: https://ampcode.com/manual
- changelog: https://ampcode.com/news
- watch: `.agents/commands/` and `.agents/skills/` (shared tree with codex/zed/crush), `amp.mcpServers` key in `.amp/settings.json`.

## zed

- docs: https://zed.dev/docs/ai/skills · /docs/ai/mcp · /docs/tasks
- changelog: https://zed.dev/releases
- watch: rules library was retired in 1.4.2; `context_servers` (not `mcpServers`) key; whether a lifecycle-hook surface appeared. The `x-zed` Task passthrough is a generic merge (`MergeCustomTargetMeta` copies any key), not an enumerated field list: `/docs/tasks` has grown fields since this adapter's own doc comment last enumerated one (`hooks` with `create_worktree`, `show_summary`, `show_command`, confirmed 2026-08-08, #563), which is exactly why an enumerated list here goes stale.

## warp

- docs: https://docs.warp.dev/terminal/entry/yaml-workflows · https://docs.warp.dev/agents/capabilities/skills · https://docs.warp.dev/knowledge-and-collaboration/mcp
- changelog: https://docs.warp.dev/getting-started/changelog
- watch: whether Warp gained a native rules-dir surface (today: AGENTS.md + `.agents/skills/` + workflows + `.warp/.mcp.json`; skills confirmed #557). The 2026-08-07 changelog added a `SKILLS_DIRS` env var for indexing extra skill directories beyond the ten documented defaults (`.agents/skills/` recommended, `.warp/skills/`, `.claude/skills/`, `.codex/skills/`, `.cursor/skills/`, `.gemini/skills/`, `.copilot/skills/`, `.factory/skills/`, `.github/skills/`, `.opencode/skills/`, confirmed 2026-08-09, #590); re-check whether that changes which one this adapter should prefer. `.opencode/skills/` is OpenCode's own default, so Warp picks it up with no extra write.

## opencode

- docs: https://opencode.ai/docs/agents/ · /docs/skills/ · /docs/mcp-servers/ · /docs/commands/
- changelog: https://github.com/sst/opencode/releases
- watch: `agents/` (plural) dir, which foreign skill trees it also scans, `opencode.json` `mcp` block shape.

## antigravity

- docs: https://antigravity.google/docs/ide/rules · /docs/ide/workflows · /docs/ide/skills · /docs/ide/mcp · /docs/ide/hooks · https://codelabs.developers.google.com/getting-started-with-antigravity-skills
- changelog: https://antigravity.google/changelog (live, updates frequently; splits into four product tabs, Hub/2.0, IDE, CLI, and SDK, each on its own version track, confirmed 2026-08-09, #590; filter to the IDE tab, since that is what this adapter targets)
- watch: MCP schema fields beyond the confirmed set (stdio's `command`/`args`/`env`/`cwd`, remote's `serverUrl`/`headers`, both transports' `disabled`, confirmed #556). `description`, `roots`, and a `type` discriminant remain unconfirmed and are omitted from the adapter. The doc site now splits into `/docs/*` (general) and `/docs/ide/*`/`/docs/cli/*` (per-product); this adapter targets the IDE, and the `/docs/ide/*` pages above render the same content as their bare `/docs/*` predecessors (verified 2026-08-08, #563), so prefer the `/docs/ide/*` citations going forward. Hooks now have a documented schema (`/docs/ide/hooks`: `.agents/hooks.json`, five events, `PreToolUse`/`PostToolUse`/`PreInvocation`/`PostInvocation`/`Stop`), but whether the IDE itself executes them stays unconfirmed, so the adapter still skips hooks with a warning; commands support remains fully unconfirmed in the public-preview docs.

## junie

- docs: https://junie.jetbrains.com/docs/get-started-with-junie.html · /docs/agent-skills.html · /docs/junie-ide-plugin.html (guidelines lookup order) · /docs/guidelines-and-memory.html (CLI guidelines lookup order, confirms the same order) · /docs/junie-cli-subagents.html (CLI-only native subagents, `.junie/agents/` or `.agents/`, live since 2026-03-10) · /docs/custom-slash-commands.html (CLI-only native slash commands, `.junie/commands/`, live since 2026-04-13)
- changelog: https://plugins.jetbrains.com/plugin/26104-junie-the-ai-coding-agent-by-jetbrains/versions · https://junie.jetbrains.com/blog/
- watch: the guidelines lookup order is strict precedence, first match wins, not a merge (#552): `.junie/AGENTS.md`, then root `AGENTS.md`, then legacy `.junie/guidelines.md` / `.junie/guidelines/`. `sync` always writes `.junie/AGENTS.md`, so re-check whether that still holds on every audit: a vendor change to step 1 would make everything below it live again. Also watch `.junie/mcp/mcp.json` schema. Three consecutive audits (#552, #590, and whatever landed between) missed junie-cli-subagents.html and custom-slash-commands.html even though both pages predate junie's own adapter issue (#474, 2026-07-19), because each audit checked "does what we emit still land" rather than "did the vendor grow a surface we never modelled" (#604, #605). Re-run that second question explicitly for junie every audit: check `get-started-with-junie.html`'s own nav/sidebar for any page not already listed above, since that is how both were found. The subagent frontmatter table (`name`, `description`, `tools`, `disallowedTools`, `mcpServers`, `model`, `reasoningLevel`, `maxTurns`, `skills`, `allowPromptArgument`) and the "Configure subagent usage" auto model-selection toggle (Early Access, `/settings` only, not a file format) are two different things; only the latter is caveated.

## kiro

- docs: https://kiro.dev/docs/steering/ · /docs/mcp/ · /docs/hooks/ · /docs/custom-agents/ · /docs/custom-agents/configuration-reference/ · /docs/tools/
- changelog: https://kiro.dev/changelog
- watch: steering `inclusion:` values (`always` / `fileMatch`); the agent `tools` field's category vocabulary, confirmed 2026-08-08 (`read`/`write`/`shell`/`web`/`subagent`/`knowledge`/`todo_list`, plus `@server_name`/`@mcp`/`@builtin`/`*`). Re-check whenever Kiro adds, renames, or splits a category, since `kiroToolCategory` in kiro.go maps agnostic-ai's Read/Write/Edit/Bash/Grep/Glob/WebFetch/WebSearch onto it. Also watch whether Kiro documents further hook `trigger` values beyond the 10 confirmed. The old `/docs/cli/custom-agents/configuration-reference/` path now serves a client-side meta-refresh and JS redirect (HTTP 200, not a real 3xx) to the URL above; curl with `-D -` to inspect headers and body before concluding a vendor page is gone or undocumented, since WebFetch does not follow a meta-refresh and an earlier audit misread this exact stub as an unconfirmed vocabulary.

## crush

- docs: https://github.com/charmbracelet/crush (README is the reference)
- changelog: https://github.com/charmbracelet/crush/releases
- watch: `crush.json` `mcp` block, whether agents/commands surfaces landed.

## trae

- docs: https://docs.trae.ai/ide/rules · https://docs.trae.ai/ide/model-context-protocol · https://docs.trae.ai/ide/add-mcp-servers
- changelog: (none published; use the docs page diff)
- watch: `docs.trae.ai/ide/mcp`, the old URL for the MCP page, now 302s
  to a marketing page; model-context-protocol (the MCP overview) and
  add-mcp-servers (the config how-to) are the two live ones. Confirm
  both stay live and watch whether Trae ever adds a `type` discriminant
  or a `disabled` key: the current adapter (`.trae/mcp.json`)
  deliberately omits both because neither appears in the doc's two
  worked examples.

## jules

- docs: https://jules.google/docs
- changelog: https://jules.google/docs (changelog section)
- watch: today AGENTS.md only; any per-file surface is new.

## goose

- docs: https://goose-docs.ai (client-rendered; use WebFetch) · https://github.com/aaif-goose/goose
- changelog: https://github.com/aaif-goose/goose/releases
- watch: `.goosehints` vs AGENTS.md precedence, whether recipes/extensions became project-scoped files. `github.com/block/goose` 301s here (moved to the Agentic AI Foundation, 2026-04-07); the redirect still works, so this was a low-priority citation refresh, not a docs-moved finding (target-audit 2026-08-08, #563).

## augment

- docs: https://docs.augmentcode.com/setup-augment/guidelines · https://docs.augmentcode.com/cli/subagents
- changelog: https://www.augmentcode.com/changelog
- watch: `.augment-guidelines` vs AGENTS.md, whether `.augment/rules/` exists.

## qoder

- docs: https://docs.qoder.com/user-guide/rules · https://docs.qoder.com/extensions/subagent · https://docs.qoder.com/extensions/skills · https://docs.qoder.com/cli/subagent · https://docs.qoder.com/cli/mcp-servers · https://docs.qoder.com/cli/Skills · https://docs.qoder.com/user-guide/chat/model-context-protocol
- changelog: https://qoder.com/changelog
- watch: `.qoder/rules/`, `.qoder/agents/`, and `.qoder/skills/<name>/SKILL.md` (project scope; user scope is `~/.qoder/skills/{skill-name}/SKILL.md`, out of this adapter's reach) today, plus `.mcp.json`. Re-check on every audit whether `.agents/skills/` gains listing as a compatible path (#558 confirmed it does not yet, unlike kilo/augment/openhands). `docs.qoder.com/cli/subagent` confirms `tools` and `disallowedTools` both accept a comma-separated string or a string array (inline `[Read, Grep]` or YAML block-list), so the adapter's comma-separated emission is a choice, not a vendor limitation (target-audit 2026-08-08, #563); the CLI (`/cli/*`) and IDE (`/extensions/*`, `/user-guide/*`) doc trees cover the same features from two products, so both stay listed here.

## openhands

- docs: https://docs.openhands.dev/overview/skills · https://docs.openhands.dev/openhands/usage/settings/mcp-settings
- changelog: https://github.com/All-Hands-AI/OpenHands/releases
- watch: `.agents/skills/` shared tree, microagents (`.openhands/microagents/`), a surface the adapter does not emit to.

## factory

- docs: https://docs.factory.ai/harness/subagents · https://docs.factory.ai/
- changelog: https://docs.factory.ai/ (release notes section)
- watch: `.factory/droids/` frontmatter keys, whether MCP config became project-scoped. `docs.factory.ai/cli/configuration/custom-droids`, the prior URL here, 308s to the page above (Harness rebrand, target-audit 2026-08-08, #563).

## kilo

- docs: https://kilo.ai/docs (client-rendered; use WebFetch) · /docs/customize/custom-rules · /docs/customize/agents-md · /docs/customize/custom-subagents (raw source also mirrored at `Kilo-Org/kilocode`'s `packages/kilo-docs/pages/customize/custom-subagents.md`, useful when the rendered site defeats fetching; appending `.md` to a kilo.ai docs path used to serve that raw source directly but now 404s there, confirmed 2026-08-09, #590, so use the GitHub mirror instead)
- changelog: https://github.com/Kilo-Org/kilocode/releases
- watch: `kilo.jsonc` `mcp` shape (already confirmed distinct from the deprecated `mcpServers`), `.kilo/agents/` vs custom modes, whether `.kilocode/rules/` backward compatibility is dropped. `instructions` array entries: this adapter lists one explicit `.kilo/rules/<name>.md` path per rule rather than a `.kilo/rules/*.md` glob (unconfirmed whether a `**` recursive glob would also cover a scoped rule); revisit if the vendor doc ever confirms glob recursion. Agent Configuration Options table (target-audit 2026-08-08, #562): `color`/`mode` now read from the generic top-level field; `disable`/`hidden`/`steps`/`temperature`/`top_p` are a deliberate `x-kilo`-only decision, not a gap, unless a future revisit finds a cross-tool counterpart worth promoting one of them to.

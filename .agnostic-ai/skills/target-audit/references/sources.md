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
- watch: whether `.clinerules/` is fully dropped from `/getting-started/config` (still absent there as of 2026-08-01; this adapter treats it as a live fallback via `outputs.cline.rules-dir`, not a removal); `.cline/hooks/`, `.cline/plugins/`, `.cline/cron/` (listed in the config reference, unemitted, no confirmed schema); the `.cline/workflows/` path; MCP config location.

## windsurf

- docs: https://docs.windsurf.com/windsurf/cascade/memories · https://docs.windsurf.com/windsurf/cascade/mcp · https://docs.devin.ai/desktop/cascade/workflows · https://docs.devin.ai/desktop/context-awareness/windsurf-ignore
- changelog: https://windsurf.com/changelog
- watch: the adapter writes `.devin/rules/`; confirm that is still the read path after the Devin rebrand, and whether MCP config became project-scoped. Also watch `.devinignore` vs the legacy `.codeiumignore`/`.windsurfignore` names for a precedence change.

## continue

- docs: https://docs.continue.dev/customize/deep-dives/rules · /customize/deep-dives/mcp
- changelog: https://github.com/continuedev/continue/releases
- watch: `.continue/rules/` vs hub-hosted rules, `.continue/mcpServers/*.yaml` schema.

## amp

- docs: https://ampcode.com/manual
- changelog: https://ampcode.com/news
- watch: `.agents/commands/` and `.agents/skills/` (shared tree with codex/zed/crush), `amp.mcpServers` key in `.amp/settings.json`.

## zed

- docs: https://zed.dev/docs/ai/skills · /docs/ai/mcp · /docs/tasks
- changelog: https://zed.dev/releases
- watch: rules library was retired in 1.4.2; `context_servers` (not `mcpServers`) key; whether a lifecycle-hook surface appeared.

## warp

- docs: https://docs.warp.dev/features/warp-drive/workflows · https://docs.warp.dev/knowledge-and-collaboration/mcp
- changelog: https://docs.warp.dev/getting-started/changelog
- watch: whether Warp gained a skills or rules-dir surface (today: AGENTS.md + workflows + `.warp/.mcp.json`).

## opencode

- docs: https://opencode.ai/docs/agents/ · /docs/skills/ · /docs/mcp-servers/ · /docs/commands/
- changelog: https://github.com/sst/opencode/releases
- watch: `agents/` (plural) dir, which foreign skill trees it also scans, `opencode.json` `mcp` block shape.

## antigravity

- docs: https://antigravity.google/docs/rules-workflows · /docs/skills · /docs/mcp · https://codelabs.developers.google.com/getting-started-with-antigravity-skills
- changelog: (none published; use the docs page diff)
- watch: MCP schema fields beyond the confirmed set (stdio's `command`/`args`/`env`/`cwd`, remote's `serverUrl`/`headers`, both transports' `disabled`, confirmed #556). `description`, `roots`, and a `type` discriminant remain unconfirmed and are omitted from the adapter. Hooks and commands support is also still unconfirmed in the public-preview docs.

## junie

- docs: https://junie.jetbrains.com/docs/ · /docs/agent-skills.html · /docs/junie-ide-plugin.html (guidelines lookup order) · /docs/guidelines-and-memory.html (CLI guidelines lookup order, confirms the same order)
- changelog: https://plugins.jetbrains.com/plugin/26104-junie-the-ai-coding-agent-by-jetbrains/versions · https://junie.jetbrains.com/blog/
- watch: the guidelines lookup order is strict precedence, first match wins, not a merge (#552): `.junie/AGENTS.md`, then root `AGENTS.md`, then legacy `.junie/guidelines.md` / `.junie/guidelines/`. `sync` always writes `.junie/AGENTS.md`, so re-check whether that still holds on every audit: a vendor change to step 1 would make everything below it live again. Also watch `.junie/mcp/mcp.json` schema.

## kiro

- docs: https://kiro.dev/docs/steering/ · /docs/mcp/ · /docs/hooks/ · /docs/custom-agents/ · /docs/cli/custom-agents/configuration-reference/
- changelog: https://kiro.dev/changelog
- watch: steering `inclusion:` values (`always` / `fileMatch`), the agent `tools` field's identifier vocabulary (unconfirmed as of 2026-08-01, so this adapter drops it rather than guess), and whether Kiro documents further hook `trigger` values beyond the 10 confirmed.

## crush

- docs: https://github.com/charmbracelet/crush (README is the reference)
- changelog: https://github.com/charmbracelet/crush/releases
- watch: `crush.json` `mcp` block, whether agents/commands surfaces landed.

## trae

- docs: https://docs.trae.ai/ide/rules · https://docs.trae.ai/ide/mcp
- changelog: (none published; use the docs page diff)
- watch: MCP is documented but the adapter emits none. Confirm the project-scoped file path.

## jules

- docs: https://jules.google/docs
- changelog: https://jules.google/docs (changelog section)
- watch: today AGENTS.md only; any per-file surface is new.

## goose

- docs: https://goose-docs.ai (client-rendered; use WebFetch) · https://github.com/block/goose
- changelog: https://github.com/block/goose/releases
- watch: `.goosehints` vs AGENTS.md precedence, whether recipes/extensions became project-scoped files.

## augment

- docs: https://docs.augmentcode.com/setup-augment/guidelines
- changelog: https://www.augmentcode.com/changelog
- watch: `.augment-guidelines` vs AGENTS.md, whether `.augment/rules/` exists.

## qoder

- docs: https://docs.qoder.com/user-guide/rules · https://docs.qoder.com/extensions/subagent · https://docs.qoder.com/extensions/skills
- changelog: https://qoder.com/changelog
- watch: `.qoder/rules/`, `.qoder/agents/`, and `.qoder/skills/<name>/SKILL.md` (project scope; user scope is `~/.qoder/skills/{skill-name}/SKILL.md`, out of this adapter's reach) today, plus `.mcp.json`. Re-check on every audit whether `.agents/skills/` gains listing as a compatible path (#558 confirmed it does not yet, unlike kilo/augment/openhands). Also watch whether `tools:` gains a documented list form alongside the current comma-separated string.

## openhands

- docs: https://docs.openhands.dev/overview/skills
- changelog: https://github.com/All-Hands-AI/OpenHands/releases
- watch: `.agents/skills/` shared tree, microagents (`.openhands/microagents/`), a surface the adapter does not emit to.

## factory

- docs: https://docs.factory.ai/cli/configuration/custom-droids · https://docs.factory.ai/
- changelog: https://docs.factory.ai/ (release notes section)
- watch: `.factory/droids/` frontmatter keys, whether MCP config became project-scoped.

## kilo

- docs: https://kilo.ai/docs (client-rendered; use WebFetch, or append `.md` to a page path to fetch the raw source directly, e.g. https://kilo.ai/docs/customize/custom-rules.md) · /docs/customize/custom-rules · /docs/customize/agents-md · /docs/customize/custom-subagents (raw source also mirrored at `Kilo-Org/kilocode`'s `packages/kilo-docs/pages/customize/custom-subagents.md`, useful when the rendered site defeats fetching)
- changelog: https://github.com/Kilo-Org/kilocode/releases
- watch: `kilo.jsonc` `mcp` shape (already confirmed distinct from the deprecated `mcpServers`), `.kilo/agents/` vs custom modes, whether `.kilocode/rules/` backward compatibility is dropped. `instructions` array entries: this adapter lists one explicit `.kilo/rules/<name>.md` path per rule rather than a `.kilo/rules/*.md` glob (unconfirmed whether a `**` recursive glob would also cover a scoped rule); revisit if the vendor doc ever confirms glob recursion. Agent Configuration Options table (target-audit 2026-08-08, #562): `color`/`mode` now read from the generic top-level field; `disable`/`hidden`/`steps`/`temperature`/`top_p` are a deliberate `x-kilo`-only decision, not a gap, unless a future revisit finds a cross-tool counterpart worth promoting one of them to.

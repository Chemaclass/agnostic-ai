# Upstream sources per target

Canonical vendor docs for every registered target. Auditors fetch from
here instead of searching, which is the single biggest cost saver in a
run: 25 targets x 2-6 pages is a fixed, known list.

Every URL below returned HTTP 200 on **2026-08-01**. Vendors move doc
hosts often (the Codex skills path moved twice in 2026), so a 404 is
itself a finding: record it as `docs-moved` and put the replacement URL
in the report so this file gets patched.

`docs` = the pages describing the file formats agnostic-ai emits.
`changelog` = where new features land first; read it before the docs
when hunting for "what changed since last audit".

Some vendor sites render client-side (goose, kilo, antigravity). `curl`
returns 200 with an empty body there; use WebFetch or WebSearch instead
and do not conclude "page is empty" from a curl body.

---

## claude

- docs: https://code.claude.com/docs/en/memory (rules) · /hooks · /sub-agents · /skills · /mcp · /settings · /slash-commands
- changelog: https://github.com/anthropics/claude-code/blob/main/CHANGELOG.md
- watch: `.claude/rules/` native loading, settings.json key surface, plugin/marketplace keys.

## codex

- docs: https://developers.openai.com/codex/skills · /subagents · /custom-prompts · /hooks · /config-reference · /rules
- changelog: https://developers.openai.com/codex/changelog
- watch: skills dir has moved twice (`.codex/skills` -> `.agents/skills`); prompts are deprecated in favour of skills; hooks JSON event names.

## gemini

- docs: https://geminicli.com/docs/cli/skills/ · /docs/cli/custom-commands/ · https://geminicli.com/docs/get-started/configuration/
- changelog: https://github.com/google-gemini/gemini-cli/releases
- watch: `.gemini/skills/` vs the `.agents/skills` alias precedence, `settings.json` hooks + mcpServers schema.

## cursor

- docs: https://cursor.com/docs/skills · /docs/subagents · /docs/rules · /docs/hooks · /docs/bugbot · /docs/agent/chat/commands
- changelog: https://cursor.com/changelog
- watch: `.mdc` frontmatter fields, hooks event names (camelCase, e.g. `beforeShellExecution`), environment.json schema.

## copilot

- docs: https://docs.github.com/en/copilot/reference/custom-agents-configuration · https://docs.github.com/en/copilot/how-tos/copilot-on-github/customize-copilot/customize-cloud-agent/add-skills
- changelog: https://github.blog/changelog/label/copilot/
- watch: `.github/instructions/*.instructions.md` `applyTo` semantics, `.vscode/mcp.json` vs a repo-level MCP file, agent frontmatter keys.

## aider

- docs: https://aider.chat/docs/usage/conventions.html · https://aider.chat/docs/config/aider_conf.html
- changelog: https://aider.chat/HISTORY.html
- watch: whether Aider gained any per-file rules/skills surface (today it is conventions + `.aiderignore` only).

## cline

- docs: https://docs.cline.bot/customization/cline-rules
- changelog: https://github.com/cline/cline/releases
- watch: `.clinerules/workflows/` path, whether root `AGENTS.md` is read, MCP config location.

## windsurf

- docs: https://docs.windsurf.com/windsurf/cascade/memories · https://docs.windsurf.com/windsurf/cascade/mcp · https://docs.devin.ai/desktop/cascade/workflows
- changelog: https://windsurf.com/changelog
- watch: the adapter writes `.devin/rules/`; confirm that is still the read path after the Devin rebrand, and whether MCP config became project-scoped.

## continue

- docs: https://docs.continue.dev/customize/deep-dives/rules · /customize/deep-dives/mcp · https://docs.continue.dev/hub/assistants/intro
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

- docs: https://antigravity.google/docs · https://codelabs.developers.google.com/getting-started-with-antigravity-skills
- changelog: (none published; use the docs page diff)
- watch: unresolved `.agent/` vs `.agents/` rules-dir question — the adapter uses `.agent/`. Official confirmation either way is a high-value finding.

## junie

- docs: https://junie.jetbrains.com/docs/
- changelog: JetBrains plugin release notes
- watch: `.junie/rules/` vs `.junie/guidelines.md`, `.junie/mcp/mcp.json` schema.

## kiro

- docs: https://kiro.dev/docs/steering/ · /docs/mcp/ · /docs/hooks/
- changelog: https://kiro.dev/changelog
- watch: steering `inclusion:` values (`always` / `fileMatch` / `manual`), agent hooks — Kiro has a hooks surface the adapter does not emit to yet.

## crush

- docs: https://github.com/charmbracelet/crush (README is the reference)
- changelog: https://github.com/charmbracelet/crush/releases
- watch: `crush.json` `mcp` block, whether agents/commands surfaces landed.

## trae

- docs: https://docs.trae.ai/ide/rules · https://docs.trae.ai/ide/mcp
- changelog: (none published; use the docs page diff)
- watch: MCP is documented but the adapter emits none — confirm the project-scoped file path.

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

- docs: https://docs.qoder.com/user-guide/rules
- changelog: https://qoder.com/changelog
- watch: `.qoder/rules/` only today; skills/agents/MCP surfaces would each be new.

## openhands

- docs: https://docs.openhands.dev/overview/skills
- changelog: https://github.com/All-Hands-AI/OpenHands/releases
- watch: `.agents/skills/` shared tree, microagents (`.openhands/microagents/`) — a surface the adapter does not emit to.

## factory

- docs: https://docs.factory.ai/cli/configuration/custom-droids · https://docs.factory.ai/
- changelog: https://docs.factory.ai/ (release notes section)
- watch: `.factory/droids/` frontmatter keys, whether MCP config became project-scoped.

## kilo

- docs: https://kilocode.ai/docs (client-rendered; use WebFetch)
- changelog: https://github.com/Kilo-Org/kilocode/releases
- watch: `kilo.jsonc` `mcpServers` shape, `.kilo/agents/` vs custom modes, rules-dir surface.

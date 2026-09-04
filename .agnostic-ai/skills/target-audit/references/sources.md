# Upstream sources per target

Canonical vendor docs for every registered target. Auditors fetch from
here instead of searching, which is the single biggest cost saver in a
run: a fixed, known list of pages beats discovery every time.

One `## <target>` section is required per registered target, each with a
`docs:` line and a `watch:` line.
`tests/integration/target_audit_sources_test.go` enforces that, so a new
adapter cannot merge without its vendor docs landing here too.

Every URL below returned HTTP 200 on **2026-08-27**. Vendors move doc
hosts often (the Codex skills path moved twice in 2026), so a 404 is
itself a finding: record it as `docs-moved` and put the replacement URL
in the report so this file gets patched.

`docs` = the pages describing the file formats agnostic-ai emits.
`changelog` = where new features land first; read it before the docs
when hunting for "what changed since last audit".

Some vendor sites render client-side (goose, kilo, antigravity, trae,
continue). `curl` returns 200 with an empty body there; use WebFetch or
WebSearch instead and do not conclude "page is empty" from a curl body.

**The worse case is a 200 that looks like content and is not.**
`ampcode.com/manual` returns 25 KB of SvelteKit app shell carrying none of
the manual's text (target-audit 2026-08-27). A byte count is not proof of a
successful fetch: grep the body for a string you expect before trusting it.

Fetching tricks that each cost an auditor a wasted call, kept here so they
cost nobody again:

- Appending `.md` to a docs path serves a clean markdown mirror on
  **factory, qoder, augment, openhands, cline** (all Mintlify) and on
  **antigravity**. It does **not** work on kilo or trae.
- **amp**: append `/markdown` after `/docs` for a raw mirror;
  `https://ampcode.com/llms.txt` indexes all 45 pages in one call.
- **junie**: `https://junie.jetbrains.com/docs/HelpTOC.json` returns the full
  page list in one call. That is how three uncited pages were found.
- **trae**: neither WebFetch nor `.md` works (`ide/rules.md` returns 1.8 MB of
  SPA shell). Content is server-side inside `window._ROUTER_DATA` as a Quill
  delta: curl to disk, brace-match the object, collect `ops[].insert` per
  `zoneId` (table cells arrive as separate zones). The changelog is separate
  and easy: `https://www.trae.ai/api/changelog` returns JSON.
- **kiro**: Next.js server-rendered, so plain curl plus a tag-strip gives full
  text. WebFetch truncates the 280 KB pages.
- **kilo**: `kilo.ai` is client-rendered and `.md` append 404s. Use the
  `Kilo-Org/kilocode` `packages/kilo-docs/pages/` tree; `gh api search/code`
  scoped to that path finds which page names a string.
- **continue**: no `.md` mirror and no `llms.txt`. Use `continuedev/continue`
  at `docs/customize/deep-dives/*.mdx`.
- **`docs.qoder.com/llms.txt` needs `curl --compressed`**, or the response is
  not valid UTF-8 and reads as garbage.
- When a Mintlify site seems to have no changelog, read its `llms.txt` first:
  factory's is listed there under `## Changelog` but absent from the nav.

---

## claude

- docs: https://code.claude.com/docs/en/memory (rules) · /hooks · /sub-agents · /skills (slash-commands merged in; `.claude/commands/` still works) · /mcp · /settings
- changelog: https://github.com/anthropics/claude-code/blob/main/CHANGELOG.md
- watch: `.claude/rules/` native loading, settings.json key surface, plugin/marketplace keys. Also the `.mcp.json` per-server field set: `headersHelper`, `timeout`, `alwaysLoad`, and the `oauth` object (`clientId`, `clientSecret`, `callbackPort`, `scopes`, `authServerMetadataUrl`), all documented at `/docs/en/mcp` and none reachable today, since the claude adapter routes through the shared builder with no `x-claude` merge (target-audit 2026-08-27). Three audits read past this because this line named only rules, settings, and plugin keys.

## codex

- docs: https://learn.chatgpt.com/docs/build-skills · /docs/agent-configuration/subagents · /docs/custom-prompts · /docs/hooks · /docs/config-file/config-reference · /docs/agent-configuration/rules (exec-policy precedence, not AGENTS.md discovery) · /docs/agent-configuration/agents-md (AGENTS.md discovery)
- changelog: https://learn.chatgpt.com/docs/changelog
- watch: skills dir has moved twice (`.codex/skills` -> `.agents/skills`); prompts are deprecated in favour of skills; hooks JSON event names.

## gemini

- docs: https://geminicli.com/docs/cli/skills/ · /docs/cli/custom-commands/ · /docs/reference/configuration · /docs/cli/gemini-ignore/ (the ignore file Gemini CLI actually reads is `.geminiignore`; `.aiexclude` belongs to Gemini Code Assist, a different product, target-audit 2026-08-27)
- changelog: https://github.com/google-gemini/gemini-cli/releases
- watch: `.gemini/skills/` vs the `.agents/skills` alias precedence, `settings.json` hooks + mcpServers schema.

## cursor

- docs: https://cursor.com/docs/skills · /docs/subagents · /docs/rules · /docs/hooks · /docs/mcp · /docs/bugbot · https://cursor.com/help/customization/skills.md (the old `/docs/agent/chat/commands` 308s here, to a "migrate commands to skills" FAQ; no Cursor page documents `.cursor/commands` any more, target-audit 2026-08-27)
- changelog: https://cursor.com/changelog
- watch: `.mdc` frontmatter fields, hooks event names (camelCase, e.g. `beforeShellExecution`), environment.json schema.

## copilot

- docs: https://docs.github.com/en/copilot/reference/custom-agents-configuration · https://docs.github.com/en/copilot/how-tos/copilot-on-github/customize-copilot/customize-cloud-agent/add-skills · https://code.visualstudio.com/docs/agent-customization/mcp-servers (MCP; VS Code's docs are authoritative for `.vscode/mcp.json`, docs.github.com does not cover it) · https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-mcp-servers (the Copilot CLI MCP page; this is the one that names the accepted top-level keys) · https://docs.github.com/en/copilot/concepts/agents/hooks · https://docs.github.com/en/copilot/reference/hooks-reference (13 events) · https://docs.github.com/en/copilot/how-tos/copilot-on-github/customize-copilot/customize-cloud-agent/use-hooks (the default-branch requirement)
- changelog: https://github.blog/changelog/label/copilot/
- watch: `.github/instructions/*.instructions.md` `applyTo` semantics, agent frontmatter keys. Enable/disable state is stored outside `mcp.json`, so there is no per-server `disabled` key. **The MCP file question is settled and the answer is not what this line used to say** (target-audit 2026-08-27): Copilot CLI reads `.mcp.json` and `.github/mcp.json`, under either an `mcpServers` object or the bare top-level format, and does **not** read `.vscode/mcp.json` — "It uses the unsupported top-level key `servers`." VS Code forwards its own config to the Agent Host, so a VS Code user is unaffected; a CLI-only user gets nothing. Watch for a VS Code or GitHub page stating the accepted top-level key for the `.mcp.json` Agent Host reads, and for a cloud-agent MCP page naming `.github/mcp.json`; both are open questions that bound how far the fix should go.

## aider

- docs: https://aider.chat/docs/usage/conventions.html · https://aider.chat/docs/config/aider_conf.html
- changelog: https://aider.chat/HISTORY.html
- watch: whether Aider gained any per-file rules/skills surface (today it is conventions + `.aiderignore` only).

## cline

- docs: https://docs.cline.bot/customization/cline-rules · /customization/skills · /getting-started/config (append `.md` to any path for a clean markdown mirror; `llms.txt` lists every page)
- changelog: https://github.com/cline/cline/releases
- watch: whether `.clinerules/` is fully dropped from `/getting-started/config` (still absent there as of 2026-08-01; this adapter treats it as a live fallback via `outputs.cline.rules-dir`, not a removal); `.cline/hooks/`, `.cline/plugins/`, `.cline/cron/` (listed in the config reference, unemitted, no confirmed schema); MCP config location. `docs.cline.bot/features/workflows` 404s and `llms.txt` lists no project-scoped replacement (target-audit 2026-08-08, #563): the current `customization/` tree covers Rules, `.clineignore`, Hooks, Plugins, and Skills only, no Workflows entry, so `outputs.cline.workflows-dir` is an unconfirmed export until a current doc backs it (see targets.md). `.clineignore` is now vendor-flagged deprecated (`llms.txt` titles it "`.clineignore` (deprecate soon)"); no impact, cline declares no `KindIgnore`. Hooks stay correctly declined and the reason is worth keeping: `/customization/hooks` is a one-line stub pointing at SDK Plugins, where hooks are a TypeScript `AgentPlugin` API, not shell-command-on-event. Two project paths are named and they disagree (`/getting-started/config` lists `.cline/hooks/`; the v4.1.16 notes say `.clinerules/hooks`), so a file schema for either would settle it (target-audit 2026-08-27).

## windsurf

- docs: https://docs.devin.ai/desktop/cascade/memories · https://docs.devin.ai/desktop/cascade/mcp · https://docs.devin.ai/desktop/cascade/agents-md · https://docs.devin.ai/cli/subagents · https://docs.devin.ai/cli/extensibility/hooks/overview · https://docs.devin.ai/desktop/cascade/skills · https://docs.devin.ai/desktop/cascade/workflows · https://docs.devin.ai/desktop/context-awareness/windsurf-ignore · https://docs.devin.ai/desktop/devin-local · https://docs.devin.ai/cli/extensibility/rules · https://docs.devin.ai/cli/extensibility/skills/overview · https://docs.devin.ai/cli/extensibility/mcp/configuration
- changelog: https://docs.devin.ai/desktop/changelog.md (`windsurf.com/changelog` 308s to docs.devin.ai and the rendered page is client-side, so the `.md` mirror is the only fetchable form)
- watch: the adapter writes `.devin/rules/`; confirm that is still the read path after the Devin rebrand, and whether MCP config became project-scoped. `.agents/skills/` is a documented cross-agent-compatibility path behind Devin Desktop's own `.windsurf/skills/` (target-audit 2026-08-08, #563); re-check if that framing ever flips. Also watch `.devinignore` vs the legacy `.codeiumignore`/`.windsurfignore` names for a precedence change. The doc site also covers a separate Devin CLI / Devin Local Agent product tree (`/desktop/devin-local`, `/cli/extensibility/rules`, `/cli/extensibility/skills/overview`, `/cli/extensibility/mcp/configuration`), uncited here until now (confirmed 2026-08-09, #590); it documents a project-scoped `.devin/mcp_config.json` this adapter does not emit yet (#587), plus `.devin/hooks.v1.json` (eight events, unwrapped, `type` accepts `prompt` as well as `command`) and `.devin/agents/<name>.md` subagents, neither declared. Watch the rules-discovery mechanism specifically: the page describes `.devin/rules/` directories being found at each level between the workspace root and the cwd, and never says subdirectories *inside* `.devin/rules/` are read, which is where this adapter writes a scoped rule (target-audit 2026-08-27). A Devin CLI run showing a rule at `.devin/rules/x/y.md` absent from the loaded-rules list would settle it.

## continue

- docs: https://docs.continue.dev/customize/deep-dives/rules · /customize/deep-dives/mcp · /reference (all client-rendered with no `.md` mirror and no `llms.txt`; the working source is `continuedev/continue` at `docs/customize/deep-dives/*.mdx`)
- changelog: https://github.com/continuedev/continue/releases
- watch: the `/hub/` namespace is gone from the doc source entirely (target-audit 2026-08-08, #563; confirmed by listing `continuedev/continue`'s `docs/` tree), so `.continue/rules/` vs any hub-successor surface if one reappears, `.continue/mcpServers/*.yaml` schema, and whether `outputs.continue.assistants-dir` ever gains a vendor-confirmed native discovery path (today: an opt-in export whose file shape matches `promptSchema`/`configYamlSchema` in `continuedev/continue`'s `packages/config-yaml/src/schemas/index.ts` and `/reference`'s `name`/`version`/`schema: v1`, but no doc says Continue scans a project directory for it; see targets.md).

## amp

- docs: https://ampcode.com/llms.txt (the index; it lists all 45 pages and each link serves raw markdown) · https://ampcode.com/docs/customize/agents-md · /docs/customize/skills · /docs/customize/mcp · /docs/customize/plugins · /docs/cli/settings · /docs/orbs/customizing
- changelog: https://ampcode.com/chronicle (`ampcode.com/news` 307s here; individual posts keep `/news/<slug>`)
- watch: `.agents/commands/` and `.agents/skills/` (shared tree with codex/zed/crush), `amp.mcpServers` key in `.amp/settings.json`. **`ampcode.com/manual` is dead and its 200 is a lie**: it returns 25 KB of SvelteKit shell with none of the manual text, so every claim that cited it needs a new source (target-audit 2026-08-27). Two things moved with it. `.agents/checks/` (the reviews surface a prior audit filed as a gap) is **gone**: a grep of all 45 pages in `llms.txt` returns zero hits for `checks/` or `severity-default`, and `/docs/customize/checks` 404s — do not file a reviews emitter on the old evidence. `.agents/setup` and `.agents/resume` are **new** (`/docs/orbs/customizing`, "Commit them to the repository"), executable shell scripts, so they are closer to an Environment spec than anything we emit today. `includeTools` is documented under "Common fields" for skill MCP servers but never named inside an `amp.mcpServers` example; the passthrough fix is right either way.

## zed

- docs: https://zed.dev/docs/ai/skills · /docs/ai/mcp · /docs/tasks · /docs/ai/instructions
- changelog: https://zed.dev/releases
- watch: rules library was retired in 1.4.2; `context_servers` (not `mcpServers`) key; whether a lifecycle-hook surface appeared. `/docs/ai/instructions` holds the project-instruction lookup order, nine paths, first match wins with no merge: `.rules`, `.cursorrules`, `.windsurfrules`, `.clinerules`, `.github/copilot-instructions.md`, `AGENT.md`, `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`. sync writes zed's entry-point to `.rules` (rank 1) because copilot's pointer-only file at rank 5 outranked `AGENTS.md` at rank 7 and left Zed with no rules at all (#624). Zed calls `.rules` a compatibility instruction file and `AGENTS.md` its primary one, so **watch for `.rules` leaving that list**: it moves zed's entry-point back to `AGENTS.md`, and only then. The `x-zed` Task passthrough is a generic merge (`MergeCustomTargetMeta` copies any key), not an enumerated field list: `/docs/tasks` has grown fields since this adapter's own doc comment last enumerated one (`hooks` with `create_worktree`, `show_summary`, `show_command`, confirmed 2026-08-08, #563), which is exactly why an enumerated list here goes stale.

## warp

- docs: https://docs.warp.dev/terminal/entry/yaml-workflows · https://docs.warp.dev/agents/capabilities/skills · https://docs.warp.dev/agents/capabilities/mcp
- changelog: https://docs.warp.dev/changelog/ (the `/getting-started/changelog` path 308s here). Fetch `https://docs.warp.dev/changelog/2026.md` for entry text: it is the per-year markdown mirror and the only form that yields entries in one call, since `/changelog.md` is a 518-byte year index.
- watch: whether Warp gained a native rules-dir surface (today: AGENTS.md + `.agents/skills/` + workflows + `.warp/.mcp.json`; skills confirmed #557). The 2026-08-07 changelog added a `WARP_SKILL_DIRS` env var (not `SKILLS_DIRS`, confirmed 2026-09-03, #663) for indexing extra skill directories beyond the ten documented defaults (`.agents/skills/` recommended, `.warp/skills/`, `.claude/skills/`, `.codex/skills/`, `.cursor/skills/`, `.gemini/skills/`, `.copilot/skills/`, `.factory/skills/`, `.github/skills/`, `.opencode/skills/`, confirmed 2026-08-09, #590), scoped to Cloud agents indexing skills outside the repo, not a general extension of the ten; re-check whether that changes which one this adapter should prefer. `.opencode/skills/` is OpenCode's own default, so Warp picks it up with no extra write. The MCP doc URL used to be `/knowledge-and-collaboration/mcp`; it now 308s to `/agents/capabilities/mcp` (confirmed 2026-08-11), so re-check for another move on the next pass. That page confirmed `working_directory` on the CLI Server (Command) table (the vendor's own name for the cross-tool `cwd` field, #606) alongside `command`/`args`/`env`, and confirmed the Streamable HTTP or SSE Server (URL) table has only `url`/`headers` (no discriminant, #592). It documents no `description`, `disabled`, or `roots` field anywhere; this adapter still emits all three, inherited from the shared builder it used before #606 moved it to its own. Worth a future pass: either Warp's schema grows a home for them, or this adapter should stop sending fields the vendor page never names.

## opencode

- docs: https://opencode.ai/docs/agents/ · /docs/skills/ · /docs/mcp-servers/ · /docs/commands/ · /docs/rules/
- changelog: https://github.com/anomalyco/opencode/releases (the repo moved from `sst`; the redirect still works, and the default branch is `dev`)
- watch: `agents/` (plural) dir, which foreign skill trees it also scans, `opencode.json` `mcp` block shape. The rules lookup is an upward walk for files named exactly `AGENTS.md`, confirmed in the vendor's own source at `packages/core/src/instruction-context.ts` on branch `dev` (`fs.up({ targets: ["AGENTS.md"] })`); no doc or code path names `.opencode/AGENTS.md`, which is where this repo wrote opencode's entry point until #623 moved it to the root `AGENTS.md` (target-audit 2026-08-27). Re-check that the upward walk still targets `AGENTS.md` only, and that the global `~/.config/opencode/AGENTS.md` slot stays out of project scope. Agent `tools` is now vendor-deprecated in favour of `permission`, which our frontmatter filter already emits, so that choice is confirmed rather than a gap.

## antigravity

- docs: https://antigravity.google/docs/ide/rules · /docs/ide/workflows · /docs/ide/skills · /docs/ide/mcp · /docs/ide/hooks · https://antigravity.google/docs/subagents (the `/docs/ide/subagents` variant 404s; this is the real path) · https://codelabs.developers.google.com/getting-started-with-antigravity-skills
- changelog: https://antigravity.google/changelog (live, updates frequently; splits into four product tabs, Hub/2.0, IDE, CLI, and SDK, each on its own version track, confirmed 2026-08-09, #590; filter to the IDE tab, since that is what this adapter targets)
- watch: MCP schema fields beyond the confirmed set (stdio's `command`/`args`/`env`/`cwd`, remote's `serverUrl`/`headers`, both transports' `disabled`, confirmed #556). `description`, `roots`, and a `type` discriminant remain unconfirmed and are omitted from the adapter. The doc site now splits into `/docs/*` (general) and `/docs/ide/*`/`/docs/cli/*` (per-product); this adapter targets the IDE, and the `/docs/ide/*` pages above render the same content as their bare `/docs/*` predecessors (verified 2026-08-08, #563), so prefer the `/docs/ide/*` citations going forward. Hooks now have a documented schema (`/docs/ide/hooks`: `.agents/hooks.json`, five events, `PreToolUse`/`PostToolUse`/`PreInvocation`/`PostInvocation`/`Stop`), and **the IDE-executes-them question is settled**: the hook payload's `transcriptPath` resolves under `~/.gemini/antigravity-ide`, the IDE's own app-data directory, so the IDE is the process running them (target-audit 2026-08-27). Commands support remains fully unconfirmed in the public-preview docs. Appending `.md` to any antigravity docs path serves a clean markdown mirror past the client-side rendering. Subagent frontmatter is documented at `/docs/subagents`, and its `tools` list is Antigravity's own vocabulary (`view_file`, `replace_file_content`, `grep_search`, `run_command`, ...) with **zero** overlap with Claude-style names; the page warns that "Specifying an unmapped or misspelled tool name in the `tools` list may cause the subagent process to hang", so never pass `tools` through verbatim here. The Antigravity 2.0 / Hub track shipped `inheritCustomizations` (2.9.1) and a `rules:` frontmatter key plus `skills.json`/`agents.json`/`rules.json` subdirectory discovery (2.11.0) in August 2026; none is in the docs yet and none is IDE-attributed, so watch for the docs to catch up rather than modelling them.

## junie

- docs: https://junie.jetbrains.com/docs/get-started-with-junie.html · /docs/agent-skills.html · /docs/junie-ide-plugin.html (guidelines lookup order) · /docs/guidelines-and-memory.html (CLI guidelines lookup order, confirms the same order) · /docs/junie-cli-subagents.html (CLI-only native subagents, `.junie/agents/` or `.agents/`, live since 2026-03-10) · /docs/custom-slash-commands.html (CLI-only native slash commands, `.junie/commands/`, live since 2026-04-13) · /docs/junie-cli-hooks.html · /docs/junie-cli-mcp-configuration.html · /docs/junie-cli-extensions.html · /docs/junie-cli-configuration.html · /docs/environment-variables.html (the fullest statement of the guidelines lookup)
- changelog: https://plugins.jetbrains.com/plugin/26104-junie-the-ai-coding-agent-by-jetbrains/versions · https://junie.jetbrains.com/blog/
- watch: the guidelines lookup order is strict precedence, first match wins, not a merge (#552): `.junie/AGENTS.md`, then root `AGENTS.md`, then legacy `.junie/guidelines.md` / `.junie/guidelines/`. `sync` always writes `.junie/AGENTS.md`, so re-check whether that still holds on every audit: a vendor change to step 1 would make everything below it live again. Also watch `.junie/mcp/mcp.json` schema. Three consecutive audits (#552, #590, and whatever landed between) missed junie-cli-subagents.html and custom-slash-commands.html even though both pages predate junie's own adapter issue (#474, 2026-07-19), because each audit checked "does what we emit still land" rather than "did the vendor grow a surface we never modelled" (#604, #605). Re-run that second question explicitly for junie every audit: check `get-started-with-junie.html`'s own nav/sidebar for any page not already listed above, since that is how both were found. The subagent frontmatter table (`name`, `description`, `tools`, `disallowedTools`, `mcpServers`, `model`, `reasoningLevel`, `maxTurns`, `skills`, `allowPromptArgument`) and the "Configure subagent usage" auto model-selection toggle (Early Access, `/settings` only, not a file format) are two different things; only the latter is caveated. The full page list comes from `https://junie.jetbrains.com/docs/HelpTOC.json` in one call, which is how the uncited pages above were found; run that read every audit. Two updates from 2026-08-27: the lookup order's step 2 now reads "`AGENTS.md` file in the project root, **combined with `.junie/playbook.md` and every `.junie/rules/*.md` file, if present**" on both the CLI and IDE pages, so `.junie/rules/` is shadowed by our `.junie/AGENTS.md` rather than unread as this adapter's doc claims; and the subagent table gained `permissionMode` plus an `effort` alias for `reasoningLevel`. Both are docs-only, since `DocumentStyled` passes every key through. Junie hooks stay correctly declined on two independent grounds, Early Access and "Project-local hooks from `<project-root>/.junie/config.json` are ignored by default for safety".

## kiro

- docs: https://kiro.dev/docs/steering/ · /docs/mcp/ · /docs/mcp/configuration/ · /docs/hooks/ · /docs/skills/ · /docs/custom-agents/ · /docs/custom-agents/configuration-reference/ · /docs/tools/
- changelog: https://kiro.dev/changelog
- watch: steering `inclusion:` values (`always` / `fileMatch`); the agent `tools` field's category vocabulary, confirmed 2026-08-08 (`read`/`write`/`shell`/`web`/`subagent`/`knowledge`/`todo_list`, plus `@server_name`/`@mcp`/`@builtin`/`*`). Re-check whenever Kiro adds, renames, or splits a category, since `kiroToolCategory` in kiro.go maps agnostic-ai's Read/Write/Edit/Bash/Grep/Glob/WebFetch/WebSearch onto it. Also watch whether Kiro documents further hook `trigger` values beyond the 10 confirmed. The old `/docs/cli/custom-agents/configuration-reference/` path now serves a client-side meta-refresh and JS redirect (HTTP 200, not a real 3xx) to the URL above; curl with `-D -` to inspect headers and body before concluding a vendor page is gone or undocumented, since WebFetch does not follow a meta-refresh and an earlier audit misread this exact stub as an unconfirmed vocabulary. The docs are Next.js server-rendered, so plain curl plus a tag-strip gives full text; WebFetch truncates the 280 KB pages. Three things confirmed 2026-08-27: the hook `version` field is the **string** `"v1"`, not the integer this adapter emits; `/docs/mcp/configuration/` documents `oauth`, `oauthScopes`, `autoApprove` and `disabledTools`, none reachable because kiro routes through the shared builder with no `x-kiro` merge; and `/docs/skills/` documents `.kiro/skills/` with the standard `SKILL.md` folder layout, which this adapter flattens into `.kiro/steering/skill-<name>.md`. Also watch whether Kiro ever names a `type` discriminant on a remote MCP entry: its remote table does not list one and we emit `"type": "http"`.

## crush

- docs: https://github.com/charmbracelet/crush (README is the reference) · https://raw.githubusercontent.com/charmbracelet/crush/main/schema.json (the vendor's published JSON schema, and the only place the MCP property set appears closed) · https://github.com/charmbracelet/crush/blob/main/docs/hooks/README.md
- changelog: https://github.com/charmbracelet/crush/releases
- watch: `crush.json` `mcp` block, whether agents/commands surfaces landed. Read `schema.json` rather than the README for MCP: it carries `disabled` (which this adapter drops silently) and `sessionless` (new in v0.91.2, 2026-08-26), and it sets `"additionalProperties": false` on `MCPConfig` — so a generic `x-crush` passthrough on MCP entries would let a typo produce a config Crush rejects outright. Prefer explicit field mapping there, unlike skill frontmatter where the generic merge is safe. Hooks are project-scoped in `crush.json` and support `PreToolUse` only; the matcher vocabulary is lowercase in the vendor's own example (`^bash$`).

## trae

- docs: https://docs.trae.ai/ide/rules · https://docs.trae.ai/ide/model-context-protocol · https://docs.trae.ai/ide/add-mcp-servers · https://docs.trae.ai/ide/skills · https://docs.trae.ai/ide/subagents · https://docs.trae.ai/ide/slash-commands
- changelog: https://www.trae.ai/api/changelog (returns JSON; the docs site publishes no changelog page)
- watch: `docs.trae.ai/ide/mcp`, the old URL for the MCP page, now 302s
  to a marketing page; model-context-protocol (the MCP overview) and
  add-mcp-servers (the config how-to) are the two live ones. Confirm
  both stay live and watch whether Trae ever adds a `type` discriminant
  or a `disabled` key: the current adapter (`.trae/mcp.json`)
  deliberately omits both because neither appears in the doc's two
  worked examples. Fetching: neither WebFetch nor a `.md` suffix works here
  (`ide/rules.md` returns 200 with 1.8 MB of SPA shell). The content is
  server-side inside `window._ROUTER_DATA` as a Quill delta: curl to disk,
  brace-match the object, then collect `ops[].insert` per `zoneId`, since
  table cells arrive as separate zones. Confirmed 2026-08-27: `/ide/subagents`
  documents `{project_folder}/.trae/agents/{my_agent}.md` with `name`,
  `description`, `model`, `tools`, `disallowedTools`, `mcpServers`, gated
  behind a Settings > Beta toggle whose default is unstated; `/ide/rules`
  documents `scene: git_message` and caps rule nesting at three levels; and
  `/ide/slash-commands` documents `.trae/commands` with a `Name`/`Description`
  table, which retires this repo's claim that the command shape was only
  reverse-engineered. Trae's tool vocabulary is Claude-style and matches
  agnostic-ai's exactly, unlike kiro, factory, and antigravity.

## jules

- docs: https://jules.google/docs
- changelog: https://jules.google/docs (changelog section)
- watch: today AGENTS.md only; any per-file surface is new.

## goose

- docs: https://github.com/aaif-goose/goose/blob/main/documentation/docs/guides/context-engineering/using-goosehints.md · .../using-skills.md · .../custom-agents.md · .../hooks.md (the GitHub `documentation/docs/` tree is the working source; the rendered site at goose-docs.ai is not fetchable and its `llms.txt` lists no skills, agents, or hooks page)
- changelog: https://github.com/aaif-goose/goose/releases
- watch: `.goosehints` vs AGENTS.md precedence, whether recipes/extensions became project-scoped files. `goose-docs.ai/docs/guides/context-engineering/using-goosehints/` documents nested discovery: goose loads context files from the working directory up to the repo root, then discovers additional hint files in nested subdirectories as it reads or modifies files there, checking each for one of `CONTEXT_FILE_NAMES` (default `AGENTS.md`, `.goosehints`). This adapter now routes a scoped rule into a nested `<scope>/.goosehints` off that mechanism (#608); re-check `CONTEXT_FILE_NAMES` on future audits in case the vendor adds a third name or changes the default pair. `github.com/block/goose` 301s here (moved to the Agentic AI Foundation, 2026-04-07); the redirect still works, so this was a low-priority citation refresh, not a docs-moved finding (target-audit 2026-08-08, #563). Three surfaces confirmed 2026-08-27 that this adapter declares none of: `.agents/skills/` ("the recommended standard"), `<project>/.agents/agents/` with `name`/`description`/`model` frontmatter, and project hooks at `.agents/plugins/<name>/hooks/hooks.json` with twelve documented events. The hooks one is the costly one: it needs a wrapping plugin directory plus a manifest, and goose's `matcher` is a regex rather than a glob with `timeout` in seconds defaulting to 30. `goose.go`'s "Goose has no per-agent or per-skill file surface" is now false on both halves. Slash commands stay correctly declined: they are user-tier only, under `slash_commands:` in `~/.config/goose/config.yaml`.

## augment

- docs: https://docs.augmentcode.com/setup-augment/guidelines · https://docs.augmentcode.com/cli/subagents · /cli/config · /cli/rules · /cli/skills · /cli/hooks · /cli/custom-commands · /cli/integrations (append `.md` to any path for a markdown mirror)
- changelog: https://www.augmentcode.com/changelog
- watch: `.augment-guidelines` vs AGENTS.md, whether `.augment/rules/` exists. **The project-tier MCP question is settled and the answer is yes** (target-audit 2026-08-27): `/cli/config` names `<workspace>/.augment/settings.json` as "Best for team-shared project configuration, **such as shared MCP servers** or tool permissions", with `auggie mcp add --project` writing there, so `spec-format.md`'s "Augment has no project-scoped MCP file" is false. That same file also carries project hooks and `<workspace>/.augment/commands/<name>.md` exists too, so one merged emitter covers all three — and it must **merge**, since the file also holds `shell`, `startupScript`, `theme`, plugin keys and tool permissions. Scope it to Auggie CLI: the settings table's "Supported By" column reads CLI for both workspace tiers, and CLI+IDE only for the user and system tiers. Hook `timeout` here is **milliseconds** (default 60000), and the matcher vocabulary is Augment's own (`launch-process`, `str-replace-editor`, `save-file`, `remove-files`), so a Claude matcher matches nothing.

## qoder

- docs: https://docs.qoder.com/user-guide/rules · https://docs.qoder.com/extensions/subagent · https://docs.qoder.com/extensions/skills · https://docs.qoder.com/cli/subagent · https://docs.qoder.com/cli/mcp-servers · https://docs.qoder.com/cli/Skills · https://docs.qoder.com/user-guide/chat/model-context-protocol · https://docs.qoder.com/cli/mcp-reference · https://docs.qoder.com/cli/hooks-reference · https://docs.qoder.com/cli/hooks · https://docs.qoder.com/user-guide/commands
- changelog: https://docs.qoder.com/release-notes/qoder-cli.md and /release-notes/desktop.md (`qoder.com/changelog` is client-rendered and defeats both WebFetch and payload extraction; the docs release notes are plain markdown and split CLI from IDE)
- watch: `.qoder/rules/`, `.qoder/agents/`, and `.qoder/skills/<name>/SKILL.md` (project scope; user scope is `~/.qoder/skills/{skill-name}/SKILL.md`, out of this adapter's reach) today, plus `.mcp.json`. Re-check on every audit whether `.agents/skills/` gains listing as a compatible path (#558 confirmed it does not yet, unlike kilo/augment/openhands). `docs.qoder.com/cli/subagent` confirms `tools` and `disallowedTools` both accept a comma-separated string or a string array (inline `[Read, Grep]` or YAML block-list), so the adapter's comma-separated emission is a choice, not a vendor limitation (target-audit 2026-08-08, #563); the CLI (`/cli/*`) and IDE (`/extensions/*`, `/user-guide/*`) doc trees cover the same features from two products, so both stay listed here. `docs.qoder.com/llms.txt` needs `curl --compressed` or the body is not valid UTF-8. Read `/cli/mcp-reference` rather than `/cli/mcp-servers` for MCP: it is the only page with the field tables, and it documents nine keys we reach by no route (`disabled`, `timeout`, `description`, `trust`, `includeTools`, `excludeTools`, `alwaysAllow`, `oauth`, and stdio's `cwd`), which retires this repo's "not vendor-confirmed either way" claim about `disabled` (target-audit 2026-08-27). The *other* half of that reason still stands and constrains the fix: `.mcp.json` is the same file Claude Code reads, so emitting `disabled` there breaks the dedupe. `<project>/.qoder/settings.json → mcpServers` is a second documented project-level location Claude Code never reads, and it is also the hooks file. Hooks are 23 events, not the six a prior audit recorded, and hook `timeout` is **seconds defaulting to 600**.

## openhands

- docs: https://docs.openhands.dev/overview/skills · /overview/skills/path · https://docs.openhands.dev/openhands/usage/settings/mcp-settings · /openhands/usage/customization/hooks · /openhands/usage/customization/repository · /sdk/guides/agent-file-based (append `.md` to any path for a markdown mirror)
- changelog: https://github.com/All-Hands-AI/OpenHands/releases
- watch: `.agents/skills/` shared tree. The microagents item is **resolved**: the vendor now heads that section "Skills (formerly Microagents)" and says "use `.agents/skills/` for new skills", which is our default. Three live surfaces this adapter declares none of (target-audit 2026-08-27): `.openhands/hooks.json`, whose format the vendor states is Claude-compatible ("PascalCase event keys ... and the `{\"hooks\": {...}}` wrapper are both supported"), making it the cheapest hook fix in the registry; `{project}/.agents/agents/*.md` file-based agents, auto-registered from `_ensure_agent_ready()` in `local_conversation.py` with no user Python, though flat only since "subdirectories are skipped", and confirmed for `LocalConversation` rather than Cloud; and path-triggered rules, which need `paths:` frontmatter and can use the **folder** form `.agents/skills/<name>/SKILL.md`, sidestepping the loose-`.md`-in-a-shared-tree hazard a prior audit blocked on. OpenHands' tool names are its own (`file_editor`, `terminal`), so `tools` needs a coverage note rather than passthrough, and its matcher says `terminal` where Claude says `Bash`.

## factory

- docs: https://docs.factory.ai/harness/subagents · /harness/skills · /harness/hooks · /harness/custom-slash-commands · https://docs.factory.ai/ (append `.md` to any path for a markdown mirror)
- changelog: https://docs.factory.ai/changelog/release-notes.md (not linked from the docs nav, but listed in `docs.factory.ai/llms.txt` under `## Changelog`)
- watch: `.factory/droids/` frontmatter keys, whether MCP config became project-scoped. `docs.factory.ai/cli/configuration/custom-droids`, the prior URL here, 308s to the page above (Harness rebrand, target-audit 2026-08-08, #563). The `tools` array is **not** a Claude-style pass-through, whatever `spec-format.md` says: the vendor's closed table is `Read`, `LS`, `Grep`, `Glob`, `Create`, `Edit`, `ApplyPatch`, `Execute`, `WebSearch`, `FetchUrl`, and "Unknown IDs cause a validation error", so `Bash`/`Write`/`WebFetch` need translating to `Execute`/`Create`/`FetchUrl` (target-audit 2026-08-27). Factory ships its own Claude-name mapper for exactly this reason, and documents three more load-time rules: `TodoWrite` and `Skill` are implicit and must not be listed, `ExitSpecMode`/`GenerateDroid` are validation errors, and `tools: all` is rejected. Two surfaces also stay undeclared: `.agents/skills/**/SKILL.md` (a tree ten other targets already write, so the dedupe absorbs it) and `.factory/hooks.json`, which is keyed directly by event with no `hooks` wrapper and whose `timeout` is seconds. `.factory/commands` still loads but the vendor steers to skills, so do that one last or record the steer.

## kilo

- docs: https://kilo.ai/docs (client-rendered; use WebFetch) · /docs/customize/custom-rules · /docs/customize/agents-md · /docs/customize/custom-subagents (raw source also mirrored at `Kilo-Org/kilocode`'s `packages/kilo-docs/pages/customize/custom-subagents.md`, useful when the rendered site defeats fetching; appending `.md` to a kilo.ai docs path used to serve that raw source directly but now 404s there, confirmed 2026-08-09, #590, so use the GitHub mirror instead)
- changelog: https://github.com/Kilo-Org/kilocode/releases (GitHub mirror paths worth citing directly, since kilo.ai defeats fetching: `packages/kilo-docs/pages/getting-started/settings/index.md`, `/customize/workflows.md`, `/automate/mcp/using-in-kilo-code.md`)
- watch: `kilo.jsonc` `mcp` shape (already confirmed distinct from the deprecated `mcpServers`), `.kilo/agents/` vs custom modes, whether `.kilocode/rules/` backward compatibility is dropped. `instructions` array entries: this adapter lists one explicit `.kilo/rules/<name>.md` path per rule rather than a `.kilo/rules/*.md` glob (unconfirmed whether a `**` recursive glob would also cover a scoped rule); revisit if the vendor doc ever confirms glob recursion. Agent Configuration Options table (target-audit 2026-08-08, #562): `color`/`mode` now read from the generic top-level field; `disable`/`hidden`/`steps`/`temperature`/`top_p` are a deliberate `x-kilo`-only decision, not a gap, unless a future revisit finds a cross-tool counterpart worth promoting one of them to. **A priority sentence appeared for the project config path** (target-audit 2026-08-27): `getting-started/settings/index.md` now reads "`kilo.jsonc` in your project root, or `.kilo/kilo.jsonc` for a cleaner setup. **The `.kilo/` version takes priority if both exist.**" This adapter writes the root file, which is the loser of that sentence, and that one file carries both the `mcp` map and the `instructions` array. Not breaking today, since a user with no `.kilo/kilo.jsonc` is unaffected — but the vendor now recommends creating one. Report it as "our target is the loser of a new priority sentence", not as "the preference flipped". Resolved (target-audit 2026-09-03, #644): the same page has since grown a "Config File Precedence" section naming an explicit 8-level system (Legacy Kilocode < Remote well-known < Global < Custom < Project `kilo.jsonc` < `.kilo/` directory < Inline environment < Managed/Enterprise) and states "Higher-priority levels override lower ones", confirming a merge across named sources, not an exclusive first-match read of one whole file. Fixed with a doc caveat on the adapter and in targets.md rather than a path change, since the merge model means an untouched key on the root file still reaches Kilo Code even when `.kilo/kilo.jsonc` exists; only a key `.kilo/kilo.jsonc` itself redeclares gets shadowed. `.kilo/commands/<name>.md` is also documented and undeclared, with frontmatter near-identical to OpenCode's.

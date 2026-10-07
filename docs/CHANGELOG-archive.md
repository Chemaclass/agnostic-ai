# Changelog archive

Releases before v0.81.0. Everything from v0.81.0 on lives in [CHANGELOG.md](../CHANGELOG.md). Releases up to v0.72.0 group entries as Added, Changed, Fixed, and Removed.

## v0.80.0 - 2026-10-06

### General

- Opt-in `handoff` built-ins carry a task between tools, save approved rules, and add Git snapshots with resume notices (#1829, #1830, #1831).
- Built-in text can change with a release and show as `sync --check` drift. Run `sync` after upgrading (#1829).
- With no changed files, `sync` and `sync --check` run 2 to 5x faster; `sync --check --against` is about 5x faster at 20k+ files (#1819, #1821).
- `@path` lines inside a code fence stay as written, and a one-line code span no longer hides the lines after it (#1823, #1828).

### By tool

#### Kiro

- Commands sync to and import from `.kiro/prompts/` for CLI V3 slash commands, with native argument templates intact (#1822).

### Site

- The Amp page drops the services `agent` key, which Amp no longer documents (#1835, #1839).

## v0.79.0 - 2026-10-05

### General

- Agents, skills, and permissions take neutral capabilities; bare allow/ask warns on broad access (#1754, #1796, #1803, #1807, #1809, #1814, #1815).
- Hooks share events, matchers, args, and JSON decisions; runs show failures (#1728, #1752, #1755, #1768, #1780, #1788, #1790, #1797, #1806, #1810, #1811).
- MCP imports protect secrets and doctor checks references; literals need `!literal`; global migration works (#1729, #1736, #1742, #1753, #1804, #1805).
- Bodies render native calls and shared paths; agents can sync as skills on tools without subagents (#1772, #1773, #1779, #1787, #1789, #1800).
- Demo hooks guard force pushes, lint and drift; scaffolds omit models and flag TODOs (#1732, #1738, #1739, #1774, #1785).

### By tool

#### Cursor

- Portable hooks cover session events, prompts, and `before-tool`; unmapped events raise coverage notes (#1806).
- A portable hook synced to Claude Code and Cursor runs once on Cursor; global hooks get wrappers under `~/.cursor/hooks/` (#1806).

#### Cline

- Hooks land in `.clinerules/hooks/` for the CLI and VS Code extension, with exit 2 blocking and portable tool-kind matching (#1722, #1723, #1752, #1783).
- `hook run` runs scripts with bash and checks stdout for `{"cancel": true}`, as the Cline CLI does (#1678, #1726).

#### Codex

- Exact allows that would grant extra arguments are skipped; to allow a prefix, use `Bash(git push:*)` (#1808, #1815).
- Windows hook commands keep project-root paths, grouped matchers work, and hooks sharing a command merge correctly (#1743, #1744).

#### JetBrains

- Projects using `agnostic-ai.yaml` get root detection, schema validation, and drift status (#1757).
- Render pickers honor `agnostic-ai.local.yaml` targets and YAML flow lists (#1761).

#### Kiro

- Agent MCP tools map to `@server` or `@server/tool`; access widening raises a note or fails with `on-unsupported: error` (#1815).
- `hook run` uses an assumed shell and refuses `--bash` and `--edit` (#1566, #1725).

#### Claude Code

- `hook run` uses Git Bash for Claude Code hooks on Windows (#1746, #1751).

#### Copilot

- Portable `before-tool` hooks block on exit 2 with stderr as the reason; exit 1 still denies the call (#1783).

#### Kilo Code

- Edit deny/ask rules also restrict writes, and overlapping permission patterns let deny and ask win over allow (#1808, #1815).

#### VS Code

- Render pickers honor `agnostic-ai.local.yaml` targets and YAML flow lists (#1761).

#### Windsurf

- `hook run` uses an assumed shell for Devin CLI hooks and refuses `--edit` (#1724).

### Site

- MCP recipes and pack guides use practical examples and public repositories (#1740, #1741).
- The Why page distinguishes betagouv/agnostic-ai, and Factory links point to its current docs (#1737, #1763).

## v0.78.0 - 2026-10-03

### General

- `hook run` adds Crush, Cursor, Copilot, Factory, Qoder, and Antigravity; assumed results count only with `--include-assumed` (#1566, #1678, #1694).
- `init` pre-ticks the tools the project uses, else the CLIs on `PATH`, and in a terminal offers to import existing tool config (#1610).
- MCP `url` and `args` take `${NAME}` in each tool's own form; `$${NAME}` stays literal, and `lint` warns (LINT028) on a wrong form (#1633, #1667, #1666).
- `--json` adds warnings, notes and `/` paths; shared files show once; `explain` covers lint codes (#1607, #1608, #1648, #1675, #1680, #1683).
- `sync.allow-global-names` or `global-name-clash: ignore` silence shared-name warnings; watch keeps late edits (#1707, #1708, #1641, #1644, #1650, #1651).

### By tool

#### Gemini CLI

- `lint` warns (LINT030) on a hook command holding a bare `$GEMINI_PROJECT_DIR` or another variable Gemini replaces; write `"${NAME}"` instead (#1697).
- MCP `headers` keep `${NAME}` references and every field keeps `${NAME:-default}`, since Gemini expands all settings strings (#1668).
- Project import reads `httpUrl` and SSE `url` MCP servers back with their transport (#1665).

#### Kiro

- **Breaking:** `sync` removes steering copies of `AGENTS.md` rules kept for `x-kiro.resources`; with inheritance off, list `file://AGENTS.md` (#1643).
- Import skips `manual` and `auto` steering files with a note instead of making them always-on rules (#1656).

#### Qoder

- `hook paths` reads Qoder `Write` payloads, and `hook run` treats `asyncRewake` hooks as async, since Qoder runs them in the background (#1716, #1717).

#### Claude Code

- `hook run` treats `asyncRewake` hooks as async, since Claude Code runs them in the background (#1716).

#### Copilot

- A hook with `cwd` gets its synced script path written relative to that directory, so it starts; import restores the repository path (#1699, #1706).

#### Crush

- Synced hook scripts start with `./`, so Crush runs them and a guard hook blocks on every OS (#1695, #1698).

### Site

- The last five release briefings give concise upgrade and feature guidance, with readable code blocks, copy controls and tables (#1652, #1653).

## v0.77.0 - 2026-10-02

### General

- **Breaking:** Narrow `globs` load rules on matching files and bad config stops sync; set `alwaysApply: true` and fix named keys (#1597, #1589).
- **Breaking:** Release binaries need macOS 13+; on macOS 12, run `go install` with Go 1.26.8+ and `GOTOOLCHAIN=local`.
- Import, sync, watch and validation honor absolute sources; previews name them; sync keeps edits (#1611, #1590, #1595, #1630, #1640, #1639, #1645).
- `use` starts tools; init shows output costs; sync lists read files; imports guard specs; MCPs use env refs (#1613, #1615, #1614, #1594, #1619, #1620, #1622).
- Writes take a lock; checks catch drift and typos; models get notes; JSON and profiles keep error data (#1618, #1592, #1593, #1591, #1571, #1574, #1567).

### By tool

#### Claude Code

- MCP import and sync keep explicit `alwaysLoad` and `bareElicitationCapability` values (#1642, #1647).

#### Codex

- `sol`, `luna` and `astra` resolve to current model ids; `explain` shows them and sync notes alias changes after upgrades (#1572, #1578).

#### Continue

- MCP import replaces credentials with references; sync writes `${{ secrets.NAME }}` for Continue's `.env` files (#1619).

#### Kiro

- Hook validation accepts `SessionEnd` and names `SessionStart` for the `AgentSpawn` or `agentSpawn` alias (#1576, #1580).

### Site

- README, landing and getting started share one install, import, preview and sync quickstart (#1609).
- Errors, hook coverage, imports and Codex options match the CLI and vendor docs (#1628).
- [Why agnostic-ai](https://agnostic-ai.org/docs/why-agnostic-ai/) compares copies, one `AGENTS.md` and scripts; the old symlinks URL redirects.
- Cursor and Kiro docs explain shared skill roots and custom agent resource inheritance (#1635, #1643, #1646).

## v0.76.0 - 2026-10-01

### General

- **Breaking:** `scope`, `globs` and `paths` form a union; folders scope existing directories. For filters only, remove both scopes (#1472, #1477, #1481).
- Sync keeps user keys and files on failures, restores merged outputs and drops stale keys (#1500, #1503, #1509, #1551, #1557, #1558, #1559, #1560, #1563).
- Name `models:` tiers and protect paths across tools; accept coverage notes and see global name clashes (#1514, #1520, #1526, #1528, #1537, #1543, #1544).
- Hooks share scripts, `hook paths` reads edited files, and `hook run` tests seven tools' payloads and matchers (#1512, #1515, #1531, #1546, #1553, #1568).
- Init seeds editable text; imports keep whole files; doctor checks lint and packaging; invalid MCP JSON fails (#1442, #1448, #1468, #1492, #1501, #1564).

### By tool

#### Claude Code

- An environment spec's `setup` runs once in each new Claude Code worktree, unless `x-claude.setup: false`. Delete a hand-written bootstrap hook (#1521).
- Sync removes permission rules it no longer writes and keeps yours, and `sync --dry-run` previews the settings overlay (#1524, #1540).
- `.worktreeinclude` skips Claude runtime paths, and `gitignore.ignore-worktree-include: true` keeps the file out of Git (#1473, #1484).
- Frontmatter drops translated `readonly` and `scope`, comma `globs` become `paths` entries, and `CLAUDE.md` keeps `@AGENTS.md` with only Claude (#1470, #1488).
- `import claude` turns nested `CLAUDE.md` into scoped rules and agent models into `model.claude`, and drops `hooks: null` (#1455, #1459, #1471, #1504).

#### Codex

- Exact subtree rules write nested `AGENTS.md`; filename filters stay inline with a note. Set `nested-glob-rules: false` to opt out (#1490).
- `exec-policies-from-permissions: true` turns Bash permissions into exec policies, and `sync --dry-run` previews them (#1506, #1525).
- A project `config.toml` skips keys Codex ignores there, such as `notify`, with a note, and `environment.toml` always has `[setup]` (#1513).
- Notes name Bash `allow` rules Codex widens and agent `readonly` or `sandbox_mode` it ignores, and stop once moot (#1437, #1507, #1516, #1522).
- Sync names inactive hooks, `doctor` shows hook trust, and `import codex` strips skill headers and keeps agent models (#1482, #1485, #1486, #1502).

#### Copilot

- Agents and skills are written back where they live, such as `.github/agents/<name>.md` or `.agents/skills/`, instead of as duplicates.
- A rule's `description` goes to `.instructions.md` frontmatter, and a rule with `alwaysApply: false` and no globs stays on demand.

#### Gemini CLI

- Agents with inline MCP servers load: sync writes `mcp_servers` and renames `x-gemini.mcpServers` with a note (#1538).
- `import gemini` decodes command TOML strings, so an escaped `\\(` in a prompt no longer doubles; other keys import under `x-gemini`.

#### Cursor

- Portable `allow` and `deny` rules become Cursor CLI rules in `.cursor/cli.json`; multi-word commands, `ask`, and unmapped rules get a note (#1547).

#### Aider

- A `rules-file` change takes the old path's `read:` entry sync added out of `.aider.conf.yml`; an entry you listed stays (#1562, #1565).

#### Continue

- A comma-separated `globs` string becomes one `globs` entry per pattern, so the rule loads on those files (#1470).

### Site

- The [spec format](https://agnostic-ai.org/docs/spec-format/) reference has one page per `.agnostic-ai/` folder; old links redirect.
- The [CI page](https://agnostic-ai.org/docs/ci/) installs with npm or a pinned `install.sh`; Targets covers tools with no target, such as pi (#1467, #1480).
- Target pages note Kiro CLI V3 blocking `.kiroignore` reads and VS Code 1.140 deprecating `.vscode/mcp.json` (#1539, #1545).

## v0.75.0 - 2026-09-29

### General

- Release binaries build with Go 1.26.8 instead of 1.26.0, which fixes 18 Go standard-library vulnerabilities `govulncheck` found reachable in 0.74.0 and earlier, mostly in `crypto/x509`, which `upgrade` uses over HTTPS. CI now fails on any reachable vulnerability (#1415).
- Releases carry a signed build provenance attestation and an SPDX SBOM for every archive: `gh attestation verify <archive> --repo Chemaclass/agnostic-ai` checks one, and `agnostic-ai.intoto.jsonl` on the release page holds the same attestation. `install.sh` and `install.ps1` stop instead of installing when they cannot verify the checksum, and check provenance too with `AGNOSTIC_AI_VERIFY_ATTESTATION=1` or `-VerifyAttestation` (#1414, #1417).
- `gitignore.commit` takes `<target>:<kind>`, such as `cursor:environments`, to commit a kind for one target only; `lint` warns when that target is not configured (LINT017) (#1418).
- `sync --check --against` fails on hand-written config Git tracks inside a folder the managed block ignores, such as a skill an old branch adds under `.cursor/skills/`, and names the `import` that adopts it (#1420, #1421).
- `import` scopes each environment spec to its tool with `targets: [<tool>]` when several tools keep their own environment file, so the next sync reproduces each file instead of merging one tool's dev commands or setup into the others (#1419).
- Skills keep their Agent Skills `license` on every target that writes the standard `SKILL.md` (#1419).
- The "files to commit" hint no longer lists an output you are untracking with `git rm --cached` (#1418).

### By tool

#### Claude Code

- The root `CLAUDE.md` is `@AGENTS.md` plus only your `::target claude` blocks when another target writes the root `AGENTS.md`, so Cursor, which loads both files, reads the instructions once (#1423).
- With `gitignore.enabled`, sync keeps the managed block in `.worktreeinclude` too, so a CLI, subagent, or Desktop worktree starts with the generated files and the local layer; `gitignore.worktree-include: false` opts out (#1418).

#### Cursor

- A skill's `workspaces: [apps/platforma]` also writes it under `apps/platforma/.cursor/skills/`, since Cursor loads skills only from the workspace it opens; `import cursor` sets it when a root skill links into a project directory. `lint` warns on a `scope:` key in a skill, which has no effect (LINT018) (#1423).

### Site

- A [Verify a release](https://agnostic-ai.org/docs/verify-a-release/) page shows how to check checksums, provenance, SBOMs, npm provenance, and signed tags (#1414).
- The git hooks guide covers the first pull in a Node monorepo, before any checkout hook is installed (#1421).

## v0.74.0 - 2026-09-29

### General

- `requires` takes an exact release (`"0.73.0"`) or a range (`">=0.73.0 <0.74.0"`), so a newer binary stops with AAI-005 before it rewrites committed outputs. AAI-005 names the package manager's install, such as `pnpm install`, when the binary sits in the project's `node_modules`, and a candidate built with `-X github.com/chemaclass/agnostic-ai/internal/cli.candidateVersion=X.Y.Z` is checked as that release (#1399, #1411).
- `sync --check --against index` or `--against HEAD` compares the staged specs or the last commit with the outputs Git tracks, for pre-commit hooks and CI. It also fails on a tracked output no spec produces anymore, header or not, such as a deleted environment spec's `.claude/launch.json`; in CI, check out with `fetch-depth: 2` (#1398, #1411).
- `explain --inputs` lists every file whose change can change an output, including files a review inlines with `@path`, for a hook's trigger glob (#1411).
- With no `.sync-state`, `doctor` and `sync --check` find a headerless leftover such as `.claude/launch.json` when it still holds what the last commit's specs rendered, or what the commit that last changed it rendered. `doctor` names a leftover it cannot remove and says to delete it by hand or list it under `sync.unmanaged` (#1392).
- `sync.output-manifest: true` writes `.agnostic-ai/outputs.lock`, the committed list of generated paths and their sums. With no `.sync-state`, a tracked file it lists that no spec produces, unedited since, is a leftover `doctor --fix` removes.
- `sync --keep-edits` keeps an uncommitted edit to a tracked output when there is no `.sync-state`, such as in a new linked worktree (#1397).
- A review spec line holding only `@path` inlines that file, so a folder's `README.md` can be its review without a symlinked `BUGBOT.md` (#1395).
- One environment spec shared by several tools prints a no-effect note only for a field no enabled tool reads (#1411).

### By tool

#### Claude Code

- `import claude` turns a `launch.json` `cwd` of `${workspaceFolder}/apps/docs` into the portable `cwd: apps/docs`, and drops a bare `${workspaceFolder}` (#1400).
- `outputs.claude.settings.taskOutputMaxChars` is no longer written, since every release channel dropped it; sync removes it and says so (#1381).

#### Codex

- Environment specs write `.codex/environments/environment.toml`: `setup`, the new `cleanup`, and `setup-windows` become worktree scripts, and `dev-commands` become action buttons with an optional `icon`. A dev command's `cwd` becomes a `cd <cwd> &&` before its command, since Codex runs actions from the project root. `import codex` reads the file back (#1393, #1411).
- `lint` warns when the `AGENTS.md` chain Codex reads in one scope passes 32 KiB; `lint.codex-chain-bytes` moves the limit (#1396).
- `import codex` keeps a hand-written `## Conventions`, `## Rules`, `## Skills`, or `## Agents` section as a rule, and skips only the listings sync writes (#1391, #1411).

#### Cursor

- `import cursor` reads `.cursor/environment.json` into `environments/cursor.yaml` and keeps its `//` comments as YAML comments (#1394).

### Site

- The Claude Code page notes that Bedrock and telemetry-disabled sessions need v2.1.281 for the `AGENTS.md` fallback (#1382).

## v0.73.0 - 2026-09-28

### General

- With no `.sync-state`, `sync`, `sync --check`, and `doctor` name each leftover output and how to remove it (#1334, #1354, #1362).
- `gitignore.commit: [instructions, hooks]` keeps those kinds in Git for every target, and the block marks `allow` lines committed (#1332, #1335).
- `sync --untrack` untracks ignored outputs, `install-hook --post-checkout` syncs after checkout, and `sync --keep-edits -q` reports kept files (#1330, #1333).
- `init` pins its schema URL per release, skips default `sources:`, and creates only seeded folders; `lint` accepts bodyless environments (#1331, #1339).
- Imports point at `.agnostic-ai/` sources and add no rules on rerun; `doctor --check-references` resolves repo-root links (#1326, #1338, #1342, #1349).

### By tool

#### Claude Code

- Environment `dev-commands` write `.claude/launch.json` preview servers, and `import claude` reads them back; `lint` checks each entry (#1340).
- A fresh `import claude` syncs and lints cleanly, keeps Codex-ready hooks unpinned, and names `.claude/` files to delete (#1327, #1328, #1329, #1336).

#### Codex

- Review specs reach the root and scoped `AGENTS.md` as a `## Code Review Rules` section, and `import codex` reads it back (#1341).
- `import codex` names nested section rules after their scope, such as `api-tests.md`, not `tests-2.md` (#1337).

#### Copilot

- Hook notes say what each camelCase event's `matcher` tests, such as the agent name on `subagentStart`, and flag an invalid regex (#1377).

#### Cursor

- Environment specs take `setup` and `setup-windows` for new worktrees, written to `.cursor/worktrees.json`; `import cursor` reads it back (#1339).

#### Gemini CLI

- `import gemini` names nested section rules after their scope, such as `api-tests.md`, not `tests-2.md` (#1337).

### Site

- A compare page shows two or three targets side by side: each spec kind's state, file format, and the paths a real sync writes, in a shareable URL (#1355).
- The Factory page notes that its three command lists are deprecated in favor of `permissionRules`, which `x-factory.permissionRules` reaches (#1376).

## v0.72.0 - 2026-09-28

### Added

- Global settings `permissions.default-mode` sets Claude Code's user permission mode for matching spec targets (#1245).
- `sync --keep-edits` preserves and reports outputs edited since the last sync, including when run from a git hook (#1271).

### Changed

- `import` skips all `node_modules` directories, including scoped rules and skills for Windsurf and Antigravity (#1307).
- OpenHands `sync --global` carries remote MCP `api_key` as a Bearer header and reports unsupported `timeout` (#1306).
- Imported ignore specs target their source tool, avoiding unsupported warnings from other targets (#1274).

### Fixed

- Scoped imports keep Codex and Gemini text without duplicates, and sync adopts nested instructions (#1267, #1268, #1269, #1292, #1302, #1314).
- Import skips excluded dirs and reads in-project skill links; validation rejects `node_modules` scopes (#1265, #1266, #1273, #1321).
- Scoped output keeps its parent directories visible in Git, and ignore patterns survive Prettier (#1264, #1275, #1304).
- Previews list files, sync honors JSON, drift reports separate edits, and Cursor reviews import and appear in doctor (#1270, #1272, #1276, #1280, #1305).
- Codex settings stay outside overlay tables, and Gemini copies hook scripts stored under other tools (#1303).

### Removed

- Project sync removes legacy OpenHands `config.toml` MCP output, which current releases ignore. Use `sync --global` (#1259).

### Site

- The landing explorer pairs source specs with native Claude Code, Codex, and Gemini files; shared links show a new preview image.
- Guides cover OS-specific installation, upgrades, version pinning, and git hooks for Node monorepos and worktrees.
- Docs code blocks use the full content width and larger text.

## v0.71.0 - 2026-09-28

### Added

- `import --global` turns the model, effort, and MCP servers your tools hold into home specs that the next `sync --global` adopts unchanged (#1243).
- `sync --global` installs MCP servers from `mcps/` for eight tools, and hooks and `x-augment` keys in `~/.augment/settings.json` (#1242, #1246, #1252, #1253).
- `sync --global` sets the default `model` and `effort` for five tools from `settings/`; `lint --global` flags a rejected effort (LINT014) (#1240, #1242).
- An `x-<target>` block in a global settings spec sets that tool's own user keys, such as `x-codex.model_reasoning_summary` (#1242).
- `sync --global` takes `--plan` and `--json`, and `explain --global` shows where any global spec lands, down to the key (#1240, #1242, #1244).

### Changed

- OpenHands `config.toml` `[mcp]` output is deprecated: current releases ignore it and the next release drops it. Use `sync --global` (#1252, #1259).
- Settings `model` takes a per-target map with an optional `default`, like agent `model`, so Codex and Claude can get different models (#1240).
- `sync --global` writes through a symlinked user file such as a `CLAUDE.md` kept in dotfiles, and keeps the link (#1242).

### Fixed

- `sync --global` keeps the key order of hooks you wrote when it rewrites a hooks file such as `~/.claude/settings.json` (#1260).
- `sync --global --check` no longer fails right after a sync that removed a target's last file, and a user file sync created goes once empty (#1248, #1256).
- `lint` no longer warns that every settings spec is empty (LINT001) (#1240).

## v0.70.0 - 2026-09-27

### Added

- Hooks see `AGNOSTIC_AI_TARGET`, so one shared script knows which tool ran it (#1226).
- `requires: ">=0.70.0"` in `agnostic-ai.yaml` stops an older binary before it writes anything (AAI-005) (#1214).
- The global home reads `targets` from its own `agnostic-ai.yaml`, and `install-hook --global` gates its commits (#1210, #1213, #1223).
- `lint` warns when a target loads too many words per session or a description runs long (LINT011, LINT012), and on typos like `glob:` (LINT007) (#1205, #1225).
- `readonly: true` restricts Claude Code agents, and `disable-model-invocation` keeps a Codex skill manual-only (#1211, #1212).

### Changed

- Cline, Kiro, Qoder, Kilo Code and Augment no longer load always-on rules twice via `AGENTS.md`. Run `agnostic-ai sync` to drop old copies (#1235).
- `sync` ends with what it changed, and warnings repeated from the last run collapse into one line; `-v` shows them (#1194, #1196).
- Errors print their fix, and `why` traces every entry point to `.agnostic-ai/AGNOSTIC_AI.md` (#1206, #1207).
- `sync --global` stops on a hand-written hook with a source hook's matcher and command but other settings. Remove it or match the source (#1215).
- A Codex skill whose bundled `agents/openai.yaml` is not a YAML mapping fails `sync`. Fix that file (#1212).

### Fixed

- Exec-form hooks on Codex, Gemini, and Cursor ran a bare interpreter that read the event JSON as code. `args` now go into the command, quoted (#1229, #1230).
- A `globs` list now scopes a rule on every target, and a brace set like `*.{ts,tsx}` stays one pattern (#1238).
- `sync --global` keeps hook `args` for Claude Code and Qoder, and writes Gemini hook timeouts in milliseconds (#1229, #1231).
- `sync --global` recovers from a lost state file or a deleted hooks file, and refuses state recorded under another `HOME` (#1208, #1215).
- `install-hook --shared`, a mistyped `sync -t`, and shared skill folders with bundled files work as documented (#1195, #1197, #1223).

### Site

- The landing page plays the one-minute explainer video on click, and the header has the new hub logo and a GitHub button (#1221, #1233, #1237).
- The `why` guide moved to `/docs/trace/`; the old URL redirects (#1191).

---

## v0.69.0 - 2026-09-26

### Added

- Project specs in `.agnostic-ai/local/` override shared ones, its `AGNOSTIC_AI.md` extends every entry point, and it stays gitignored (#1172).
- A local spec edits one field of a shared spec and keeps the rest; a `::parent` line in its body extends the shared body (#1177).
- Agents map `readonly: true` to Factory's `tools: read-only` and no inherited MCP servers, overriding a portable tools list (#1162).
- OpenHands: a shttp MCP server's `oauth` field (or `auth: oauth`) emits `auth = "oauth"` in `config.toml`, and import reads it back (#1157).

### Changed

- Specs in `~/.agnostic-ai/local/` merge into shared ones instead of replacing them. Set a field to `null` to drop it. (#1177)

### Fixed

- `import` keeps `.agnostic-ai/local/` specs, hooks, and scripts out of the shared source, and leaves shared specs and settings untouched (#1174, #1185).
- `sync --watch` sees config, overlays, `AGNOSTIC_AI.md`, and source dirs created or recreated mid-session, and polls when events drop (#1179, #1184, #1188).
- `.junie/AGENTS.md` honors `sync.resolve-imports`, legacy `rules-file` entry points get local instructions, and `sync --watch` sees a new local layer (#1172).
- Global sync writes the Codex skill policy file and warns when a manual-only skill stays model-invocable on a target (#1156).

## v0.68.1 - 2026-09-25

### Fixed

- Global sync respects target filters on hooks and skills, and removes managed hooks when their target is excluded (#1148).
- Global sync renders skill metadata per target, keeps shared skill directories neutral, and preserves bundled assets (#1150).

### Added

- Agents map `readonly: true` to Codex's read-only sandbox and report a coverage note on targets that drop `readonly` (#1149).
- Global sync loads personal overrides from `local/`; `list --global` shows each effective spec's layer (#1153).

### Site

- Target reference pages explain current behavior more directly and correct the Factory settings merge key list.

## v0.68.0 - 2026-09-25

### Changed

- Noninteractive `init` enables detected tools or the default set. Pass `--all` to enable every target (#1136).
- Unsupported-kind warnings suggest removing unused `targets:` before suppressing them with `on-unsupported: silent` (#1135).

### Added

- Cursor: `sync` warns when emitted `BUGBOT.md` files exceed Bugbot's 30,000-character file cap or 100,000-character review budget (#1125, #1126).

### Fixed

- `init` and `import all` detect a root `CLAUDE.md` or `GEMINI.md` as an existing project (#1137).
- `import all` and `--dry-run` skip entry files linked outside the project and report them as skipped (#1138, #1139, #1141, #1142).

### Project

- Vendor watch filters page chrome and labels meaningful vendor docs changes in its daily issue (#1127).

### Site

- The README leads with setup and daily commands; the hook guide has a runnable Claude and Codex example.

## v0.67.0 - 2026-09-24

### Added

- Antigravity: a `scope`d rule emits to `<scope>/.agents/rules/<name>.md` instead of being skipped (#1114).
- Kilo: hook specs emit as `.kilo/plugin/<name>.ts` modules Kilo loads at startup (#1105).
- A daily `vendor-watch` workflow opens one issue listing targets whose vendor docs moved, with no AI or API key (#1119).

### Changed

- Antigravity: the entry point moves to `.agents/AGENTS.md`; the old `.agent/AGENTS.md` is kept as `.bak`. Run `agnostic-ai sync` and commit the move (#1114).

### Fixed

- Antigravity: every rule carries the `trigger` frontmatter Antigravity requires. Without it, Antigravity discarded every rule sync wrote (#1113).
- Antigravity: `manual` and unknown triggers survive `import` and sync, and the size note uses the documented 24,000-byte cap (#1113, #1114).
- OpenCode, Kilo: plugin hooks run commands unchanged, match tool names exactly, block only on exit 2, and skip disabled hooks (#1110).
- Import: Windsurf finds scoped rules under any directory and honors `outputs.windsurf.rules-dir`; an unreadable folder no longer half-imports (#1123).
- Target audits hash vendor pages the same on macOS and Linux, ignore fetch noise and site chrome, and read cross-target notes again (#1119, #1122).

### Site

- The playground is simpler: edit a spec and watch five tools' output as you type. Settings specs render, and samples show per-target model and effort.
- Docs: the sidebar lists every target, the matrix drops two rarely read columns, and the Windsurf page calls `.windsurfignore` legacy (#1116).
- The landing hero keeps its rail labels inside the hero on wide screens and on one row on narrow ones.

## v0.66.0 - 2026-09-23

### Added

- Settings `effort`: one repository default reasoning effort for Claude, Copilot, Codex, and Factory, filled by `import` (#1069, #1089).
- Global sync writes native agents for 18 targets, including a Copilot agent's `effort` (#1033, #1073).
- Previews: `sync --global --check --diff`, `import --dry-run --diff`, `compare <a> <b>`, and `explain --file` (#1034, #1035, #1036, #1083).
- Import: `import openhands` and `import factory` read native files back (#1055).
- Debugging: `doctor --check-references` finds broken skill links, and VS Code opens a file's source spec (#1037, #1038).

### Changed

- Continue skills move to `.continue/skills/<name>/`. Run `agnostic-ai sync` and commit the move (#1043).
- `import` writes each tool's settings to `settings/<target>.yaml`. Delete an old `settings/imported.yaml` after re-importing (#1096).
- Global sync honors root variables such as `CLAUDE_CONFIG_DIR` and skips a target it cannot write. Delete files under the old root (#1033).
- Target audits hash JSON with sorted keys and read Kiro's Powers pages (#1092, #1093).

### Fixed

- Global sync stops before it overwrites a hand edit or drops `~/.copilot/settings.json` comments; `--backup` keeps a copy (#1082, #1098).
- Global sync leaves unchanged settings files alone, keeps key order, and names files it would remove (#1084, #1090, #1091).
- Import: `--dry-run` writes nothing, `import all` skips tools without an importer, and a single-file `.clinerules` imports (#1046, #1052, #1057, #1060, #1064).
- Coverage notes: sync reports every dropped agent `effort` or `mcpServers`, and a hook `once` that Claude or Qoder ignore (#1066, #1072, #1078).
- Adapters: Qoder rules keep activation, Factory keeps `mcpServers: []`, `why` follows symlinks, VS Code reads `agnostic-ai.yaml` (#1029, #1047, #1049, #1075).

### Site

- Target pages for Amp, Kiro, Devin, Cursor, and Copilot document new vendor behavior (#1030, #1067, #1068, #1079).
- Docs code blocks have a Copy button, and the agent setup prompt is readable in light mode.
- The home diagram is shorter and shows `effort`; the updates archive shows six editions per page.

## v0.65.0 - 2026-09-22

### Added

- Codex agents map the portable `effort` to `model_reasoning_effort`; an integer budget raises a coverage note (#1016).

### Changed

- The Amp target page and adapter doc note that Amp also loads skills from Claude Code's directories by default, and name the settings that change it (#1023).
- Target audits fetch each vendor page once and hash it against `scripts/target-audit/sources.lock`, so auditors read only pages that moved (#1017).

### Fixed

- Windsurf (Devin) agents and permission rules granting only `Write` now emit `write`, not `edit`, so they no longer also grant edit capability (#1022).
- A sync then `import` no longer deletes frontmatter a target cannot store, such as `argument-hint`, `allowed-tools`, `tools`, or `effort`.
- `sync` prints the Devin shared `.agents/agents/` coverage note once and stops repeating it while it is unchanged (#1014).
- The Amp docs drop a settings-schema property count that goes stale; the Kilo and Copilot citations point at pages that still carry those sections (#1024).
- npm releases allow about 20 minutes for publish-time scanning before reporting a package missing (#1013).

### Site

- The home page uses its full width: the source spec runs across the top, with each target's generated file beside the target list.
- Code blocks, generated files, and the closing call to action share one dark slab, so a listing is distinguishable from the prose around it at a glance.
- The header reads Home, Docs, Targets, Updates, Playground, underlines the current section, and moves the version badge and GitHub button to the footer.
- The docs sidebar marks the current guide, the guide index reads as cards, and an `x-<target>` heading no longer breaks the table of contents (#1015).
- The targets page leads with the capability matrix, the README links each install channel, and releases before v0.50.0 move to `docs/CHANGELOG-archive.md`.

## v0.64.1 - 2026-09-21

### Changed

- npm platform packages use the project's `@agnostic-ai/<os>-<cpu>` organization scope instead of the maintainer's personal scope.

## v0.64.0 - 2026-09-21

### Changed

- `npm install agnostic-ai` downloads nothing, so it works offline and under `--ignore-scripts`. Pin with `agnostic-ai@0.62.0`; repair a global copy with `npm install -g agnostic-ai --force --include=optional` (#942).
- A release publishes the six platform packages before their parent, so a partial publish fails instead of tagging a prerelease as npm's `latest` (#942).
- The Homebrew tap pushes the cask itself every half hour, so this repository no longer needs a tap secret (#920, #943).
- Target audits reuse one issue index, load vendor references by target, and fit available worker slots.

### Added

- `agnostic-ai lint` warns (LINT009) on an allowed `Bash(...)` rule with a mid-command `*`, such as `Bash(git * main)`, which also approves inserted options.
- Gemini emits and imports default-model settings, and Qoder emits HTTP and prompt hooks (#998, #1005).
- This repository dogfoods the three spec kinds it never used: `ignore`, `review`, and `environment` (#980).

### Fixed

- Claude rejects disabled project MCP servers and preserves manual rejection entries when re-enabled (#997).
- Import preserves Cline and Continue rule conditions, both Cline rule roots, and compatible Junie, Warp, and OpenCode skills (#999, #1000, #1001, #1002).
- Factory keeps scoped skills in their project areas, and Trae rejects invalid native agent names (#1004, #1006).

### Site

- The site builds on Zola 0.23.6: templates moved to Tera 2 components, and three site tests that silently skipped on a mismatched local Zola now run (#970).
- The ten spec kinds stop being a bare list: the playground's kind picker, the landing, and the README each say what a kind is and link its definition (#980).
- The landing says the same things in fewer words and drops two installers it never rendered (#977, #991).
- The targets index records Cursor's default-on compatibility hooks and required `type: stdio` field (#1007).

## v0.63.0 - 2026-09-20

### Added

- An agent's `effort` takes a per-target map like `model` does: `effort: {claude: xhigh, qoder: 8000, default: high}` (#968).
- Settings specs take an `x-<target>` block, so a key such as `x-factory.sandbox` reaches that target's settings file (#949).
- Amp accepts settings specs: `x-amp` keys merge into `.amp/settings.json`, and each portable field reports why Amp has nowhere to put it (#950).
- Factory gets the portable permission policy: `ask` still prompts, and `deny` cannot be approved (#948).
- Qoder gets an agent's `memory` scope, `import kiro` reads `.kiro/hooks/*.json`, and the npm package ships a provenance attestation (#937, #952, #953).

### Changed

- The release mints its Homebrew token per run from a GitHub App scoped to the tap, falling back to the stored `HOMEBREW_TAP_TOKEN` until the App exists (#943).
- The npm package points its homepage at agnostic-ai.org and widens its keywords from nine to twenty (#937).

### Fixed

- An `x-<target>` settings key merges with the translated policy instead of deleting it; an unmergeable shape prints a coverage note (#966, #974).
- An `x-<target>` hooks block merges with the generated one event by event, and a wrong-shaped permission key raises a coverage note (#976).
- OpenCode and Devin permission rules use vendor names: `mcp__<server>__<tool>` becomes `<server>_<tool>`, and `WebSearch` becomes `web_search` (#947, #951).
- Installing the latest release no longer fails with HTTP 403 on a shared IP: the install scripts and `upgrade` skip the rate-limited GitHub API (#940).
- The npm wrapper survives the four ways its download used to fail, and exits `128 + signal` when killed (#936).
- An integer `effort` raises Factory's coverage note, `brew` drops a cask deprecation, and `update` reports only the PATH copy that wins (#933, #937, #968).

### Site

- The hero shows one agent spec and the five native files a real `sync` produces from it, and stays still under `prefers-reduced-motion` (#967).
- The landing reads lighter on a phone: the six-column matrix collapses to five chips, and the nav keeps only this site's pages.
- The spec-format page covers per-target `model` and `effort` in one section, and `spec.schema.json` types `model` as a map or a string (#968).
- Four target pages drop claims their vendors do not make, on Copilot, Claude, Cursor, and MCP `roots` (#955, #956, #957, #959).
- Installation lists the two package managers that work today, the talk demo moves to `/docs/#demo`, and the release shows beside the wordmark (#920).

## v0.62.0 - 2026-09-19

### Added

- OpenCode and Kilo Code get your portable permission policy, Kilo also gets agent `tools` lists, and `import` reads them back (#890, #922).
- Hook specs reach OpenCode as plugin modules in `.opencode/plugins/` and Cline as one script per event in `.cline/hooks/` (#889, #892).
- `import goose` reads everything sync writes, `import trae` reads `.trae/hooks.json`, and `import cline` reads `.agents/skills/` (#889, #894).
- Copilot gets `disabledMcpServers`, hook `cwd` and `env`, and a per-server MCP `tools` allowlist on Copilot CLI (#888).
- A settings `model` reaches Factory at `<project>/.factory/settings.json`, the documented project tier (#891).
- `agnostic-ai import all` detects a Goose project from `.agents/plugins/` or `.agents/REVIEW.md`, not only `.goosehints` (#906).

### Fixed

- Cline agents load again: they emit as `.cline/agents/<name>.yml` with the frontmatter Cline requires, and `import` reads both that and the old `.md` (#886).
- Sync always writes `CLAUDE.md`, so Claude Code never falls back to codex's `AGENTS.md`; `import claude` reads the files Claude Code loads (#885, #893).
- An exact `Bash(cmd)` in a `deny` list now blocks the command on Windsurf instead of being dropped and leaving it unblocked (#887).
- `import augment` reads your permission policy again instead of skipping every rule (#912).
- `sync` no longer fails on Windows when two targets write the same shared directory (#918).
- Copilot, Junie, and Codex report a coverage note naming where each keeps permission rules, instead of dropping the policy in silence (#917, #923).
- Cursor stdio MCP entries carry the required `"type": "stdio"`, and `validate` flags the undocumented copilot hook `SubagentStart` (#888, #895).
- Antigravity: `sync --global` writes skills to `~/.gemini/config/skills/`, and a rule over the 12,000-character cap reports a coverage note (#896).
- `agnostic-ai import antigravity codex` no longer rejects a source the error message calls supported (#905).
- `validate` flags a kiro hook spelled `AgentSpawn`, which Kiro CLI 3.0 documents nowhere, and accepts `Manual`, which it does (#907).
- A release checks that Homebrew and npm serve the new tag, instead of reporting success when a missing token skipped the push (#920).

### Site

- Site search ranks a page ahead of its own sections, matches text anywhere on a page, folds plurals, and caps one page at three of ten results.
- Target pages drop stale claims about Cline, Trae, Junie, aider, and goose (#897).
- The Claude Code plugin has a README, and the settings spec page names the right targets for `permissions` and `model`.

## v0.61.0 - 2026-09-18

### Added

- Portable permission lists reach Windsurf's `.devin/config.json` and Augment's `.augment/settings.json`, and `import` reads them back (#856, #872).
- Point a per-kind output key into `.agents/plugins/<name>/` and Antigravity ships a workspace plugin, manifest included (#810).

### Fixed

- Cline rules reach the model again: they emit to `.clinerules/`, the only path Cline reads, not the unread `.cline/rules/` (#853).
- A moved Claude Code directory round-trips: `import`, `doctor`, and the per-kind `agents-dir` and `skills-dir` keys all follow `outputs.claude.dir` (#852).
- Copilot's `{{rules_dir}}` resolves from `outputs.copilot.instructions-dir`, the key it reads, so a spec body names the directory sync writes to.
- Copilot and Cursor import every project skill directory their vendors document, so a repo on the shared `.agents/skills` layout keeps its skills (#854).
- MCP entries match vendor docs: no `ws` server on Augment or Qoder, no `disabled` on Junie, and `args` on every Warp stdio entry (#855, #858, #859).
- Qoder and Factory get per-agent `effort` and `mcpServers`, and Qoder also gets `permissionMode` and scoped `hooks` (#812, #824, #825, #826).
- `outputs.kilo.skills-dir` is registered in `skills.paths`, and a Goose skills-only bundle gets its manifest (#861, #862).
- Windsurf ignore specs also write `.windsurfignore` for agent file access, and sync reports the agent profile Devin would load twice (#863).
- `disable-model-invocation`, `color`, and Antigravity rule activation report a coverage note where a target cannot honor them (#811, #864, #865).
- Sync refuses an OpenCode skill or Junie agent name the vendor's regex rejects, instead of writing a file the tool never loads (#857).
- Cursor hooks filtering `beforeTabFileRead` or `afterTabFileEdit` no longer raise LINT005, since the vendor's matcher table documents both (#860).
- Two fork pull requests on branches with the same name no longer cancel each other's CI run (#850).

### Site

- Cursor runs `.claude/settings.json` hooks by default now that both opt-in gates are gone, so a repo syncing claude and cursor runs every hook twice (#865).
- The `/updates/` target filter closes on an outside click, on Escape, and when focus leaves it.
- The docs drop throat-clearing and repeated facts, and the Antigravity page moves to the vendor's current URLs (#865).

## v0.60.0 - 2026-09-18

### Added

- `agnostic-ai init --demo` seeds a `memory-curator` skill for Claude Code and Qoder that proposes memory merges and deletions for you to confirm (#842, #843).

### Fixed

- The managed `.gitignore` block ignores Claude Code's machine-local `agent-memory-local/`, not the shareable `agent-memory/` (#841).
- A nested `outputs.<target>.dir` such as `vendor/.claude` no longer ignores that whole directory in the managed `.gitignore` block (#846).
- `outputs.claude.dir` now moves rules, commands, and the `{{rules_dir}}` path variables with the rest of the tool directory (#849).
- `import crush` no longer writes a hook spec outside the hooks directory when a hook is named `../escape` (#831).

### Site

- Search across every guide, target page, and update, on Cmd/Ctrl+K or `/`.
- The targets reference is one page per target at `/docs/targets/<id>/`, with the capability matrix on the index and old deep links redirecting.
- The spec format, CLI, and configuration references are half their former length, and agent memory and the memory boundary are documented.
- The landing page states the four principles, answers "why not just symlink one file?", and plays the talk demo behind a click-to-play poster.
- Layout fixes: a readable measure, no sideways scroll from long inline code, 32px tap targets, the full sitemap, and search against a local `zola serve`.

## v0.59.0 - 2026-09-16

### Added

- `agnostic-ai verify` fails CI on stale generated output and fingerprints the harness for a project-owned verifier (#834).
- Portable model settings round-trip through Codex, Copilot, OpenCode, Junie, Qoder, and Kilo, plus allow, deny, and ask permissions on Qoder (#806, #827).
- Goose and OpenHands emit project agents to `.agents/agents/<name>.md`, and Goose hooks emit an Open Plugins package across 12 events (#631, #629).
- Amp environment specs emit `.agents/setup` scripts and supervised `.amp/services.yaml` services (#637).
- Augment and Factory emit commands, Augment writes `.augmentignore`, and scoped skills stay scoped on Codex, Cursor, Warp, and OpenCode (#630, #808, #805).

### Changed

- Antigravity agents move to the documented `.agents/agents/<name>/agent.md` layout, and sync migrates managed legacy flat files (#717).

### Fixed

- Codex imports package-style MCP server names containing `/` without splitting them across files (#711).
- Amp, Cline, and Windsurf import every documented skill path, with bundled assets, file modes, and Windsurf `triggers` preserved (#821, #823).
- Hooks keep their native handlers: Claude `continueOnBlock`, Copilot HTTP and `sessionStart`, and Zed `create_worktree` (#804, #629, #817).
- Factory and Windsurf skip unsupported WebSocket MCP entries instead of writing invalid native configuration (#809, #816).
- Kiro keeps an `x-kiro.name` display name alongside the canonical filename, and Amp and Copilot audits track their current upstream sources (#807, #822).

### Site

- `agnostic-ai.org` is the canonical site, built with Zola 0.22.0 from shared templates, with legacy URLs and feed identifiers stable.
- User guides live at `/docs/` from the same Markdown as the repository and `llms-full.txt`, and `/agent-setup.txt` gives a coding agent a safe setup path.
- The landing page leads with a copy-ready install command, a three-step activation path, and a ten-target comparison.
- The capability matrix filters and shares comparisons by URL; the playground covers all ten spec kinds; CI checks both against adapter declarations.
- Each release ships a briefing kept apart from verified upstream news, and the updates archive filters editions by target and search term.

## v0.58.0 - 2026-09-14

### Added

- `agnostic-ai upgrade --version v0.56.1` installs one named release instead of the latest, downgrades included, for standalone binary installs (#800).
- Hook specs reach Antigravity's `.agents/hooks.json`, keyed by definition name, and `validate` checks its five event names (#629).
- `agnostic-ai update` is an alias for `upgrade`, with the same `--run` and `--check` flags.
- The `agent-context` skill works globally without project agents and guides root setup and context reviews (#793).

### Changed

- `make tools` installs the golangci-lint version CI runs, and `make lint` names a stale binary instead of failing with a decode error (#749).

### Fixed

- `agnostic-ai upgrade` and `update` install the latest release by default, checksum-verified and replaced in place; `--check` stays read-only.

## v0.57.0 - 2026-09-14

### Added

- `sync.unmanaged` lists user-owned output paths that sync, `doctor`, the ledger, the `.gitignore` block, and `revert` all leave alone (#781).
- `::target` fences work in `.agnostic-ai/AGNOSTIC_AI.md`, so a fenced paragraph reaches only the entry-point files a listed target reads (#781).
- Claude emits and imports HTTP, MCP-tool, and prompt hooks; Cursor emits native prompt hooks (#767, #768).
- Ignore specs reach Crush's `.crushignore` and Kilo's `.kilocodeignore`, and review specs reach Goose's `.agents/REVIEW.md` (#770, #773, #771).
- Continue imports JSONC MCP maps, and Zed, Warp, and Antigravity import native skill folders with bundled assets (#764, #765).

### Fixed

- Hook scripts from `.agnostic-ai/scripts/` go through the sync session, so a failed sync rolls them back and deleting the hook sweeps the script (#789).
- Deleting a skill removes its bundled files from every target, and sync stops leaving stale output or flagging every prior output as an orphan (#781, #785).
- `::target` fences survive `import`, pass `validate` with external adapters, and leave no blank line when dropped (#781, #790).
- Sync keeps what it does not own: VS Code `inputs` and `sandbox`, extra MCP options, and hand-authored ignore file order (#757, #769, #763, #774, #775, #761).
- Native output matches what each tool loads, on Gemini hooks, Zed skill names, and Kiro timeouts and actions (#762, #766, #772).

## v0.56.1 - 2026-09-12

### Fixed

- `curl -fsSL ... | bash` installs again, and a run piped into `/bin/sh` no longer exits 0 having installed nothing.

## v0.56.0 - 2026-09-12

### Upgrade notes

Three changes make a previously green repo fail. All three are deliberate.

- `lint` now errors on an MCP spec missing `command:` or `url:` (LINT008). The entry was dead on every target and nothing said so.
- `sync` now fails with `AAI-103` rather than overwriting a hand-authored ignore file. Run `agnostic-ai import <target>`, then sync again.
- Gemini agents move to `.gemini/agents/`. Set `outputs.gemini.emit-agents-as-commands: true` to keep typing `/name`.

### Added

- Hook specs reach five more targets: Copilot (`.github/hooks/agnostic-ai.json`, 14 events), Factory, Trae, Crush (`PreToolUse` only), and Augment (#629).
- Hook specs accept `args` to run in exec form, so a path with a space or `$` runs as written on Claude Code, Qoder, and Copilot (#732, #746, #755).
- Ignore specs reach Kiro (`.kiroignore`), Trae (`.trae/.ignore`), and Junie (`.aiignore`) instead of skipping with a warning (#728).
- Gemini agents emit as native subagents at `.gemini/agents/<name>.md`, invokable with `@name` and listed by `/agents` (#733).
- `lint` reports an MCP server missing the field its transport requires, which targets wrote as a dead entry or dropped (LINT008, #747).

### Changed

- Gemini agents stop emitting `.gemini/commands/<name>.toml` and sweep old copies; set `outputs.gemini.emit-agents-as-commands: true` to keep `/name` (#733).
- Qoder hook events sort in the vendor's own order, so `.qoder/settings.json` changes bytes once for a project mixing documented and custom event names (#744).

### Fixed

- `sync` fails with `AAI-103` instead of overwriting a hand-authored ignore file on seven targets; run `import <target>` to read it into an ignore spec (#754).
- `sync` no longer deletes every user-authored key in a JSONC config such as `kilo.jsonc`, credentials and permission settings included (#725).
- `import gemini` reads `.gemini/commands/*.toml` again, after #748 made one subagent in a project hide every command (#750).
- Continue MCP servers match its schema: `streamable-http` for `type: http`, `headers` under `requestOptions`, and a coverage note for `ws` (#726, #730, #739).
- Warp skips an MCP entry with nothing to launch, Crush accepts each documented event spelling, and Codex round-trips `startup_timeout_ms` (#731, #735, #753).
- `sync -t amp` stops writing Agent specs to the directory Amp's own migration tells users to delete, and sweeps what a previous sync left there (#727).
- `validate` accepts Qoder's documented hook events and Cursor's matchers, and a Kilo command named `goal` raises a coverage note (#734, #736, #744).
- Docs fix stale vendor claims and warn that `claude` plus `copilot` can run a hook twice through `.claude/settings.json` (#737, #745, #755, #756).

## v0.55.0 - 2026-09-10

### Added

- Windsurf hooks emit to `.devin/hooks.v1.json` across eight events, `type: prompt` included, and `import windsurf` reads them back (#629).
- Qoder hooks emit to `.qoder/settings.json` across 23 events, matching Claude Code's tool-name matcher vocabulary (#629).
- Kilo Code and Qoder commands emit natively, with the `description` frontmatter each vendor requires; both previously skipped with a warning (#630).
- Trae rule frontmatter merges `x-trae` custom keys such as `scene: git_message`, which never reached a rule file before (#635).

### Fixed

- Kiro skills emit natively to `.kiro/skills/<name>/SKILL.md` with bundled assets, and a stale flattened copy in steering is swept (#642).
- `import codex` reads inline hooks in the vendor's documented nested shape, which previously imported as zero hooks (#669).
- Docs list every target that emits hooks and commands natively, after both enumerations drifted behind the adapters (#629, #630).
- Amp, Junie, Trae, Kiro, Warp, and Cursor docs drop stale vendor citations and add vendor-confirmed fields (#647).

## v0.54.0 - 2026-09-10

### Added

- `new rule --scope` adds directory-specific rules with native scoped output for 19 targets, kept out of root instructions (#704).
- The site and playground move onto a single amber accent on ink, with every foreground/background pair checked against WCAG AA.

### Changed

- Docs start with a first-rule tutorial and dedicated installation, migration, and troubleshooting guides, plus a runnable scoped-context walkthrough.

### Fixed

- Codex: an MCP server name with a package-style character no longer writes a TOML header that breaks every server in the file (#706).
- Windsurf: `outputs.windsurf.workflows-dir` warns instead of writing files nothing reads, since Devin removed the only agent that ever read a Workflow (#707).
- Junie docs name `allowPromptArgument` as vendor-documented, and the Kiro MCP comment drops an implied confirmation it never had (#708).

## v0.53.0 - 2026-09-09

### Added

- OpenHands hooks emit to `.openhands/hooks.json` across six events; a matcher copied from a Claude spec raises a coverage note (#629).
- Codex and Copilot MCP entries keep documented keys they dropped before, including Codex `oauth` and VS Code `envFile` and `sandboxEnabled` (#692).
- Codex hooks gain `type: mcp_tool`, which calls a connected server's tool instead of running a shell command (#693).
- The site is findable: canonical URL, Open Graph and Twitter cards, structured data, `llms.txt`, `sitemap.xml`, and `robots.txt`.

### Fixed

- `sync --jobs` no longer fails intermittently with `invalid argument` on macOS when two targets share a directory (#701).
- Warp: sync warns when a hand-authored `WARP.md` sits beside the generated `AGENTS.md`, since Warp reads `WARP.md` first and ignores synced rules (#691).
- Cursor skills promote the documented `icon` and `color` to first-class keys, which previously reached the file only through `x-cursor` (#694).
- The generated entry-point body offers `.geminiignore` instead of the `.aiexclude` this tool stopped writing in #625 (#651).

## v0.52.1 - 2026-09-07

### Changed

- The npm publish gates on the release archives, not on every package-manager push, and skips a version already on the registry, so a re-run is safe.

## v0.52.0 - 2026-09-07

### Added

- `sync --global` installs user-level instructions, rules, hooks, and skills from `$AGNOSTIC_AI_HOME` at the documented path of 22 of 25 targets (#680).
- Codex MCP servers emit per-tool sub-tables from a `tools` map, covering `output_token_limit` and the per-tool approval override (#678).
- `outputs.claude.settings.bashOutputMaxChars` and `.taskOutputMaxChars` raise how much output Claude Code keeps inline, up to 128K characters (#679).

### Changed

- Ordinary project sync no longer loads `~/.agnostic-ai/` as a low-precedence spec layer; it is now the explicit global source for `sync --global` (#680).
- Crush docs record that `crush.json` is the vendor's deprecated legacy format, merged with `crushrc`; emission is unchanged (#674).

### Fixed

- `sync --global` removes a hooks file left with nothing in it instead of writing `{"hooks": {}}`; a file holding other keys is kept (#680).
- `sync --jobs` no longer fails intermittently on a directory two adapters share.
- Kiro and Antigravity docs record a conflict between two vendor pages and drop a stale "stays unconfirmed" note (#675).

## v0.51.0 - 2026-09-04

### Added

- MCP servers stop dropping vendor-documented fields on ten targets, such as Claude Code `timeout` and Kiro `oauth` (#634, #641, #661).
- Augment MCP servers merge into `.augment/settings.json` under `mcpServers`, keeping the file's other settings (#633).
- Copilot MCP servers also emit to `.github/mcp.json`, the file Copilot CLI reads; override with `outputs.copilot.cli-mcp-file` (#646).
- Agents reach the subagent loader on Windsurf, Trae, and Antigravity with `model` and `tools`; stale `agent-<name>.md` rules are swept (#638).
- Windsurf maps an agent's `tools` onto Devin's own names under `allowed-tools`; on Antigravity, set `x-antigravity.tools` (#638).
- Windsurf writes the shared root `AGENTS.md` that Devin CLI reads, so unscoped rules reach a windsurf-only repo (#645).
- Factory and Goose emit skills to the shared `.agents/skills/` tree; neither had a skill surface before (#632).
- OpenHands emits a scoped rule as a path-triggered `.agents/skills/<name>/SKILL.md`, so it loads only for matching files, not always from `AGENTS.md` (#643).
- OpenHands emits an environment spec's `install` field as `.openhands/setup.sh`, the vendor's repository bootstrap script (#662).
- Codex hooks accept `async: true` to run in the background, and `import codex` reads it back (#636).

### Changed

- Qoder MCP servers move from `.mcp.json` to `.qoder/settings.json`. In a Qoder-only project, delete the leftover `.mcp.json`, which still outranks it (#641).
- Warp MCP servers stop emitting `description`, `disabled`, and `roots`, which Warp ignores; use `x-warp` to keep one (#641).
- Codex's sweep of its old `.agents/agents/` tree only removes `.toml`, so it no longer deletes Antigravity subagents in the same directory (#638).

### Fixed

- Scoped Cline and Continue rules stop loading on every request, using Cline's `paths` and Continue's `globs`; `x-continue.regex` reaches the file too (#639).
- `validate` accepts the `PreModelSwitch`, `PostModelSwitch`, `Interrupt`, and `AgentSpawn` hook events, which already emitted correctly but failed CI (#660).
- Warp's skills doc names `WARP_SKILL_DIRS` for Cloud agents, not the `SKILLS_DIRS` it claimed before, which did nothing (#663).
- Kilo Code's docs note that `.kilo/kilo.jsonc` outranks the root `kilo.jsonc` this adapter writes when both exist (#644).

## v0.50.0 - 2026-08-28

### Added

- `outputs.copilot.root-mcp-file` opts in to writing Copilot MCP servers to a root `.mcp.json`, the file Copilot CLI and VS Code's Agent Host read (#610, #622).
- Spec bodies expand path variables such as `{{$SKILLS_DIR}}` and `{{$MCP_FILE}}` to each target's own location, honoring `outputs.<target>` overrides (#616).
- `lint` warns on frontmatter keys that near-miss one agnostic-ai owns, such as `allowed_tools`, which parse but leave the agent unrestricted (#617).

### Changed

- `validate` exits 1 when it reports any issue, and `doctor --check-globs` exits 1 when a rule's globs match no files, so CI can gate on both (#617).

### Fixed

- Zed rules reach Zed again: sync writes its entry point to `.rules`, so enabling copilot beside zed no longer hides every rule (#624).
- OpenCode's entry point moves from `.opencode/AGENTS.md` to the root `AGENTS.md`, the file OpenCode reads; a managed file at the old path is swept (#623).
- Windsurf scoped rules land at `<scope>/.devin/rules/<name>.md`, where Devin finds them, and `alwaysApply: false` rules carry a matching `trigger` (#628).
- Gemini ignore specs emit as `.geminiignore`, the file Gemini CLI reads, and a managed `.aiexclude` is removed (#625).
- Kiro hook files write `"version": "v1"` as the schema documents instead of the number `1`, which a validating parser rejects (#626).
- Factory droids translate `tools` onto Droid CLI's own IDs, so a droid no longer fails to load on Claude-style names (#627).

## v0.49.0 - 2026-08-13

### Added

- Junie subagents emit at `.junie/agents/<name>.md`, so `tools`, `model`, and `mcpServers` reach Junie. Override with `outputs.junie.agents-dir` (#604).
- Junie slash commands emit at `.junie/commands/<name>.md`. Command specs targeting junie previously had no emission path at all (#605).
- Windsurf MCP servers emit to `.devin/mcp_config.json`, the project file Devin Local reads, instead of skipping every MCP spec (#587).
- Trae rules and agents carry `description`, `globs`, and `alwaysApply` frontmatter, so a glob-scoped or manual rule works on Trae (#607).
- With the `outputs.goose.rules-file` opt-in, a Goose rule scoped to a subdirectory writes a nested `<scope>/.goosehints` (#608).
- Antigravity and OpenCode MCP servers pass through extra `x-antigravity` and `x-opencode` fields, such as their `oauth` settings (#588).
- `import antigravity` reads `.agents/mcp_config.json` back into MCP specs, so a synced setup round-trips (#589).
- OpenHands streamable-HTTP MCP servers emit `timeout`; an `sse_servers` entry that sets it gets a coverage note (#588).
- Qoder agents emit `color`, matching Augment and Kilo Code (#588).

### Fixed

- Crush emits a `type: sse` MCP entry as `"type": "sse"` instead of `"http"`, so an SSE-only server connects (#586).
- Warp MCP stdio entries emit `working_directory` from a spec's `cwd` instead of dropping it, and no longer carry the undocumented `type` field (#592, #606).
- The `sync --watch` tests no longer flake on Windows CI: they wait for the watcher to arm before editing a file (#585).
- Capability tables drop duplicate `trae` and `qoder` rows, a test rejects duplicates, and the MCP and `disabled` target lists are corrected (#597).
- `docs/user/spec-format.md` adds a `color` table by target: Augment, Kilo Code, and Qoder share the key but not its values (#609).
- Targets docs cover `x-zed.disable-model-invocation`, mark windsurf's local MCP file won't-fix, and merge two Skills bullets (#557, #558, #590, #609).
- Amp skills scope their own MCP servers through `x-amp.mcpServers`, which the passthrough already emitted; this is now documented (#591).
- Warp's workflow doc link, its skill-scan directory count, Junie's lookup-order note, and five dead `sources.md` citations are corrected (#590).

### Changed

- The `target-audit` skill records four lessons from the 2026-08-09 run, such as addressing agents by spawn ID.

## v0.48.1 - 2026-08-09

### Changed

- `golang.org/x/sync` moves to 0.19.0, not Dependabot's 0.22.0 (#514), which would raise the minimum Go for `go install` to 1.25.

### Fixed

- The install docs, landing page, and contributing guide say Go 1.24+, matching `go.mod`.

## v0.48.0 - 2026-08-09

### Added

- `lint` fails on a spec whose frontmatter opens with `---` and never closes (LINT006), which every target otherwise wrote out as a broken file.
- `scripts/e2e_test.sh` drives the built binary through scaffold, sync, import, and revert on a throwaway project, and runs in CI via `make test-shell`.

### Fixed

- `lint` reports two specs of the same kind with the same `name` (LINT003), which the loader silently collapsed into one (#582).
- `lint` no longer flags hooks that share an event and matcher, since they all run; LINT002 is retired and CI now runs `lint` (#171, #582).
- The LSP reports the same lint findings as `lint`.
- The first `sync` in a fresh project no longer gitignores `.agnostic-ai/AGNOSTIC_AI.md`, so the first commit includes the shared instruction body (#580).
- The install docs mark `npx`, `winget`, and `scoop` as unpublished, and the landing's Windows tab points at the PowerShell install script.

## v0.47.1 - 2026-08-09

### Fixed

- A `tools` list on a Cursor agent raises a coverage note instead of vanishing, and `docs/user/spec-format.md` gains a per-target `tools` table.

## v0.47.0 - 2026-08-08

### Added

- Kilo Code agents emit `color` and `mode`; `disable`, `hidden`, `steps`, `temperature`, and `top_p` stay reachable through `x-kilo` (#562).
- Qoder skills emit to `.qoder/skills/<name>/SKILL.md` instead of skipping with a warning, and `import qoder` reads them back (#558).
- Kiro agents translate a spec's `tools` list onto Kiro's `read`, `write`, `shell`, and `web` tags; other names raise a coverage note (#559).
- `sync -t warp` writes skills to `.agents/skills/<name>/SKILL.md`, the shared tree Warp's docs recommend, instead of a coverage note (#557).

### Changed

- The `target-audit` skill and auditor agent record two weeks of lessons, such as trusting `gh pr checks` over the stale `statusCheckRollup`.

### Fixed

- The capability-map parity test also catches a target that drops a kind but stays in the map.
- The release workflow now skips optional npm publishing when `NPM_TOKEN` is absent instead of attempting to publish with `setup-node`'s placeholder credential.
- `import zed` reads every MCP server shape the `.zed/settings.json` it writes can hold, including string `command` and remote `url` entries (#546).
- `sync -t junie` inlines rule and agent bodies into `.junie/AGENTS.md`, the file Junie reads, instead of `.junie/rules/`; `import junie` reads both (#552).
- OpenCode MCP servers write `"enabled": false` for a `disabled: true` spec, so OpenCode no longer runs a disabled server (#555).
- `sync -t amp` no longer writes Command specs to `.agents/commands/`, which Amp never reads; they skip with a warning (#553).
- Antigravity MCP servers emit `headers` on remote entries, `cwd` on stdio entries, and `disabled` on both (#556).
- `sync -t openhands` writes `{ url, api_key }` for an sse or shttp MCP entry with `api_key`; a `headers`-only credential raises a coverage note (#554).
- `sync -t factory` skips an agent spec with an empty body with a coverage note instead of writing a file Droid rejects (#561).
- `sync -t trae` emits MCP servers to `.trae/mcp.json`, the project registry Trae documents, instead of skipping every MCP spec (#560).
- Vendor doc citations and stale target claims are refreshed from the 2026-08-08 audit, including Codex links moving to `learn.chatgpt.com` (#563).

## v0.46.0 - 2026-08-07

### Added

- `scripts/install.sh` (macOS, Linux) and `scripts/install.ps1` (Windows) install the latest release without a package manager and verify its checksum.
- Windows package managers: `winget install Chemaclass.agnostic-ai` and `scoop install agnostic-ai` from the `Chemaclass/scoop-bucket` bucket.
- npm wrapper: `npx agnostic-ai <command>` with no install, or `npm install -g agnostic-ai`; it downloads and verifies the platform binary.
- Claude Code plugin with four skills: `/plugin marketplace add Chemaclass/agnostic-ai`, then `/plugin install agnostic-ai@chemaclass`.
- `upgrade` recognizes Scoop, winget, and npm installs and prints their update command instead of falling back to a manual download.

### Fixed

- `sync` no longer fails at random with `mkdir <dir>: invalid argument` when two targets create the first children of a shared directory (#526).

## v0.45.0 - 2026-08-02

### Added

- MCP servers reach `factory` (`.factory/mcp.json`), `qoder` (`.mcp.json`, shared with Claude Code), and `openhands` (`config.toml`), now 17 of 25 targets.
- The `target-audit` skill audits every adapter against vendor docs and files issues; `--fix` spawns `adapter-fixer` agents that open PRs but never merge.
- MCP servers accept `type: ws`, emitted with the remote shape (`url`, `headers`, `type`) instead of a malformed entry.
- Seven new targets (#479): `qoder`, `openhands`, `factory`, and `kilo` join the default set, while `jules`, `goose`, and `augment` stay opt-in.
- Antigravity MCP servers land in `.agents/mcp_config.json`, with `serverUrl` for remote entries as Antigravity requires.
- Augment gets native rules, agents, and skills under `.augment/rules/`, `.augment/agents/`, and `.agents/skills/`; restrict tools with `x-augment.tools`.
- Trae skills emit as folders that keep bundled assets, commands emit at `.trae/commands/<name>.md`, and `import trae` reads both.
- Kiro agents emit at `.kiro/agents/<name>.md`, where Kiro's agent picker reads them; set tools with `x-kiro.tools`, since a generic `tools` list raises a note.
- Kiro hooks land in `.kiro/hooks/<name>.json`, one file per hook; `disabled: true` writes `"enabled": false`.
- Kilo Code skills emit into the shared `.agents/skills/<name>/SKILL.md` tree, which Kilo loads by default, instead of a second copy under `.kilo/skills/`.
- Qoder agents emit at `.qoder/agents/<name>.md` with `model`, `tools`, `skills`, and `mcpServers`, and `import qoder` reads them back (#529).
- Windsurf ignore specs merge into `.devinignore`, gitignore syntax under a provenance header, matching the shape Aider, Cursor, and Gemini already use (#530).
- Crush MCP http and sse entries accept `oauth` settings (#531), and `x-crush.user-invocable: true` adds a skill to the command palette (#540).
- Codex and Gemini MCP stdio servers accept `cwd`, Codex http servers accept `auth` (#532), and Codex hooks accept `additionalContextLimit` (#533).
- Warp workflows and Zed Tasks accept `x-warp` and `x-zed` passthrough for their documented fields, and `import` reads them back (#538, #539).
- Kilo Code rules also emit at `.kilo/rules/<name>.md`, listed in `kilo.jsonc` `instructions`, which Kilo ranks above AGENTS.md (#535).

### Fixed

- Claude hooks accept `DirectoryAdded`, and Claude settings support `attribution`, `statusLine.refreshInterval`, and `statusLine.hideVimModeIndicator`.
- Codex MCP servers emit and import `env_vars` and `env_http_headers`, and `import codex` keeps hook `commandWindows` and prefers `.codex/agents` (#532, #547).
- Codex agents no longer emit a Claude-style `tools` array that Codex rejects; it raises a coverage note, and `x-codex.tools` emits native `[tools]`.
- Concurrent sync no longer fails at random with `mkdir .agents/...` errors when Codex legacy cleanup races other targets.
- Kilo Code MCP servers write the `mcp` key Kilo reads, with `type`, one `command` array, `environment`, and `"enabled": false` for a disabled spec.
- Kilo Code agents drop the `name` and `tools` frontmatter Kilo ignores; a `tools` allowlist raises a coverage note.
- `docs/user/targets.md` corrects Warp's MCP `type` field, Codex's `SessionEnd` event and Starlark policies, Gemini's hook list, and Amp's commands path.
- Ten stale vendor doc URLs in the target-audit sources are refreshed, including Codex's move to `learn.chatgpt.com` and Kilo's to `kilo.ai`.
- Codex exec-policy `decision` takes `allow`, `forbidden`, or `prompt`; the old `ask` wrote a rule Codex ignored.
- `outputs.claude.settings.enabledPlugins` is a map of `plugin-id@marketplace-id` to boolean, matching Claude Code, so a plugin actually enables.
- MCP `disabled: true` maps to Codex's `enabled = false`; Claude Code, Cursor, and Copilot get a coverage note, since they cannot pre-disable a project server.
- Cline and Windsurf skills emit as a folder per skill that each tool loads, and `import cline` and `import windsurf` read both the new and old forms.
- Cline rules and agents default to `.cline/rules` and `.cline/agents`; keep `.clinerules` with `outputs.cline.rules-dir` (#534).
- Antigravity rules and skills default to `.agents/rules` and `.agents/skills`, where skills dedupe with other targets; stale `.agent/` copies are swept.
- `validate` and `sync --watch` no longer call specs dead weight or skip `antigravity`, `factory`, `qoder`, `openhands`, and `augment`.
- Junie skills emit as `.junie/skills/<name>/SKILL.md` with their assets, and `import junie` reads both the new and old flat forms.
- Junie also writes `.junie/AGENTS.md`, which it reads before the root `AGENTS.md`.
- Cursor rules with `alwaysApply: false` and no `globs` stay relevance-selected or manual instead of attaching to every file (#536).
- `validate` recognizes all 11 documented Gemini hook events instead of 4 (#537).
## v0.44.0 - 2026-07-24

### Added

- `new --dry-run` previews a spec scaffold without writing it, and `init` points at `agnostic-ai completion <shell>` for tab completion (#495).
- `import kiro` and `import crush` read their rules, skills, and MCP back, so all 18 emitted targets now import (#494).
- `--profile <file>` writes a CPU profile, and `sync --verbose` shows per-target wall time, so a slow sync points at one adapter (#491).
- `sync --check --diff` prints a diff per drifted file, `--format=github` emits PR annotations, and a failing check prints the fix command (#488).
- `sync --jobs <n>` emits targets in parallel, one worker per CPU by default; `1` forces serial (#487).

### Changed

- `sync --watch` re-syncs only the targets that emit the changed spec's kind; config edits, deletes, and renames still re-sync everything (#490).

## v0.43.0 - 2026-07-20

### Changed

- Gemini skills emit natively as `.gemini/skills/<name>/SKILL.md` folders with bundled assets; `emit-skills-as-commands` now only adds the command form (#439).
- Zed reads the shared `AGENTS.md` and gets native skills under `.agents/skills/`; the merged `.rules` file is opt-in via `outputs.zed.rules-file` (#439).
- OpenCode agents emit natively as subagents under `.opencode/agents/` and skills under `.opencode/skills/`, no longer flattened to commands (#439).
- Copilot agents emit natively as `.github/agents/<name>.agent.md` and skills under `.github/skills/`, replacing flattened `.instructions.md` copies (#439).
- Cline joins the shared root `AGENTS.md` entry-point, which current Cline reads as cross-tool project instructions (#439).
- All SKILL.md emitters share one renderer, so shared `.agents/skills` trees stay byte-identical; Antigravity SKILL.md gains `x-antigravity` passthrough (#439).
- Windsurf rules move to `.devin/rules/`, where Devin Desktop looks; keep the old path with `outputs.windsurf.rules-dir: .windsurf/rules` (#473).

- Codex skills move from `.codex/skills`, which Codex never read, to `.agents/skills`, the directory it scans; Amp writes the same tree.
- Codex reads prompts only from `~/.codex/prompts`, so commands no longer emit to `.codex/prompts`; set `outputs.codex.commands-dir` to keep them.
- Cursor emits natively: skills under `.cursor/skills/`, subagents under `.cursor/agents/`, and commands to `.cursor/commands/` by default (#430, #439).
- Cursor Bugbot files move to where Bugbot reads them: `.cursor/BUGBOT.md` at the root and `<scope>/.cursor/BUGBOT.md` per scope.
- Claude rules drop the Cursor-only `alwaysApply` frontmatter on emit; Claude's rule schema defines only `paths` and an unscoped rule is always-on already.
- Targets writing byte-identical content to one path dedupe instead of erroring; only divergent content collides.
- Docs stop calling `.claude/rules/` inert: current Claude Code loads it, so `outputs.claude.rules-mode: import` only matters on older versions.

### Added

- Four new default targets, all reading the root `AGENTS.md`: `junie`, `kiro`, `crush`, and `trae`; `import junie` and `import trae` read their rules (#474).
- Claude rules take `globs`: it emits as `paths:` in `.claude/rules/*.md`, and `paths` maps back to Cursor `globs`, so one spec scopes both tools.
- Hook specs pass through current per-tool fields, such as Claude's `async` and `if`, Codex's `commandWindows`, and Cursor's `failClosed`.
- `validate` knows every documented Claude Code hook event and Codex's `SubagentStart`, `SubagentStop`, and `PermissionRequest`.
- `import cursor` captures native `.cursor/agents/*.md`, `.cursor/skills/<name>/` folders (full tree), and `.cursor/commands/*.md` alongside rules.
- `sync.shared-skills` opt-in symlinks byte-identical skill folders across targets to one copy, preferring `.agents/skills/<name>` (#437).
- Import reads the new native trees: `.gemini/skills/`, `.opencode/agents/` and `.opencode/skills/`, `.github/agents/*.agent.md`, and `.github/skills/` (#439).

## v0.42.0 - 2026-07-03

### Changed

- The managed `.gitignore` header warns that listed paths are not committed, so a fresh clone or worktree needs `agnostic-ai sync`.

### Added

- "Why not symlinks" doc explaining how agnostic-ai compares to symlinks, manual copies, and shared-file `@`-includes, plus when a simpler option suffices.
- The managed `.gitignore` block ignores Claude Code's `/.claude/agent-memory/` and `/.claude/settings.local.json` when `claude` is enabled. (#469)

## v0.41.0 - 2026-06-19

### Added

- `sync.dropped-summary` opt-in prints, per target, what it could not fully emit and the key that would carry it. (#441)

### Fixed

- `doctor --fix` keeps your own keys in merged files such as `opencode.json`, and `doctor` and `sync --check` stop reporting false drift there. (#215, #465)
- An `x-<target>` block adding several new frontmatter keys emits them in stable order, so `sync --check` stops flagging false drift.
- A spec `name:` with path separators or `..` is rejected, and `sync` refuses any path outside the project root, closing a path traversal.
- Gemini TOML honors an `x-gemini.description` override instead of always using the top-level description.

## v0.40.0 - 2026-06-17

### Added

- `settings` kind (`.agnostic-ai/settings/*.yaml`): single-source `permissions` and default `model`. Claude maps them into `.claude/settings.json`. (#432)
- `review` kind (`.agnostic-ai/reviews/*.md`): Cursor emits scope-located `BUGBOT.md` files. (#433)
- `environment` kind (`.agnostic-ai/environments/*.yaml`): Cursor emits `.cursor/environment.json`. (#434)
- `ignore` kind (`.agnostic-ai/ignore/*.md`): one list emits Cursor `.cursorignore`, Gemini `.aiexclude`, Aider `.aiderignore`. (#435)
- `command` kind emits natively to Gemini (`.gemini/commands/*.toml`), OpenCode, and Amp, and to Cursor when `commands-dir` is set. (#436)
- Cursor hooks: `hook` kind emits `.cursor/hooks.json`. Override via `outputs.cursor.hooks-file`. (#438)
- `doctor`: "Unmanaged config" block lists agentic files on disk not generated from `.agnostic-ai/`, each with the `import` to adopt it. (#440)
- `validate`: warns for each declared `sources.<kind>` whose directory is missing. (#444)

### Fixed

- Cursor `.mdc` emit preserves your frontmatter, so adopting agnostic-ai on existing Cursor rules diffs clean. (#443)
- `validate` / `lint` stop flagging `command` specs as orphaned when only Gemini, OpenCode, Amp, or Cursor are enabled. (#436)
- `import` strips the provenance header when seeding `.agnostic-ai/AGNOSTIC_AI.md`, so import→sync stays byte-stable. (#429)
- `import cursor` drops the catch-all `globs` and empty `description`, so always-apply rules round-trip stable. (#429)
- `sync` warns when a skill bundles files Cursor cannot represent (flattened to one `.mdc`). Previously dropped silently. (#430)
- Extra markdown inside a folder skill (e.g. `skills/<name>/examples.md`) is a bundled asset, not a phantom skill. (#431)

## v0.39.0 - 2026-06-16

### Added

- `outputs.claude.rules-mode: import` adds `@`-imports for `.claude/rules/*.md` to the `CLAUDE.md` pointer, so Claude Code loads them. (#424)
- `sync.resolve-imports` (`passthrough`, `strip`, `inline`) sets how `@path` import lines reach targets that cannot resolve them. (#425)

### Changed

- `scripts/release.sh` drops empty `### ` subsections from `[Unreleased]` before promoting it, so a release never ships scaffolding headings with no entries.

## v0.38.0 - 2026-06-16

### Fixed

- `import` walks rule directories recursively for cursor, claude, and copilot, preserving nested subdirectories instead of dropping them. (#411)
- Nested rule, agent, and skill specs emit under the tool's rules directory (`.cursor/rules/backend/auth.mdc`), not a stray `<scope>/` tree at the root. (#412)
- `import` quotes `description` frontmatter containing `: `, quotes, or surrounding whitespace, so imported specs pass `validate`. (#413)
- Managed `.gitignore` ignores generated subdirectories such as `/.claude/rules/` instead of all of `/.claude/`, so hand-written siblings stay tracked. (#414)
- `import` warns when another target's entry-point holds unique content the next `sync` would overwrite. (#415)

## v0.37.0 - 2026-06-14

### Added

- `sync` prints a `note:` line when a target supports a kind but emits it only behind an opt-in key, so skipped content is visible. (#404)

### Fixed

- `init` now offers Antigravity in the target prompt and enables it with `--all`. It was absent before.
- `lint --help` now states the real exit code (`1`, not `2`).
- `explain` and `why` now credit rules inlined into entry-point files (`AGENTS.md`, `GEMINI.md`, ...) instead of omitting them. (#405)
- `copilot` always-on rules now emit as `applyTo:"**"` instruction files instead of being dropped. (#403)

## v0.36.0 - 2026-06-12

### Changed

- `.gitignore`: the local-override config, sync-state file, and packs dir move into the managed block, deduplicated and root-anchored. (#401)

### Fixed

- `codex`, `amp`, `warp`, `gemini`, `aider`, and `opencode` now inline rule bodies into their entry-point file instead of dropping them. (#399)

## v0.35.0 - 2026-06-09

### Added

- `sync.target-overview` opt-in appends to each entry-point file a list of where that tool's generated files live; `import` strips it again. (#397)

## v0.34.0 - 2026-06-06

### Changed

- The `sources:` block is optional; an omitted kind defaults to `.agnostic-ai/<kind>`, the tree `init` scaffolds.

### Fixed

- Docs state that `sync` enables 12 targets by default, with Amp and Warp opt-in, instead of implying all 14.

## v0.33.0 - 2026-06-05

### Added

- `gitignore.allow` re-allows patterns at the end of the managed `.gitignore` block, so a tracked file such as a fixture `AGENTS.md` stays tracked. Closes #388.

### Changed

- `init` writes `gitignore.enabled: true` by default, so generated outputs stay out of git; pass `init --gitignore=false` to commit them.

### Fixed

- `cleanup` removes only the `.bak` backups `sync --backup` wrote, not every `*.bak` in the project. Closes #390.
- `revert` restores entry-point files such as `CLAUDE.md` and `AGENTS.md` from `.bak`, and `revert --force` deletes the generated ones. Closes #389.
- Flat-file skills no longer copy sibling skills' bodies into each emitted skill folder on claude, codex, amp, and antigravity. Closes #387.

## v0.32.1 - 2026-06-04

### Changed

- Docs: README capability matrix shows per-kind support depth (native / bundled / opt-in / none) instead of a 3-state glyph.

### Fixed

- Docs: README, targets.md, configuration.md, and spec-format.md match adapter behavior, including hook paths, `AGENTS.md` dedupe, and every `outputs.*` key.

## v0.32.0 - 2026-06-03

### Added

- Antigravity emits skills natively as `.agent/skills/<name>/SKILL.md`. Configurable via `outputs.antigravity.skills-dir`. Closes #371.
- Amp emits skills natively as `.agents/skills/<name>/SKILL.md`. Configurable via `outputs.amp.skills-dir`. (#377)

### Changed

- Amp skills move from `.agents/commands/` to native `.agents/skills/<name>/SKILL.md`; `outputs.amp.emit-skills-as-commands` no longer affects Amp. (#377)
- Docs: Codex hooks emit to `.codex/hooks.json`, not `config.toml`. (#377)

### Fixed

- Antigravity skills no longer warn as unsupported. (#371)
- Zed MCP uses the current `context_servers` schema: flat `command`/`args`/`env` for stdio, native `url`/`headers` for remote. Closes #373.
- Continue MCP files use the required `mcpServers:` block wrapper; flat single-server files did not load. Closes #374.
- Remote (HTTP/SSE) MCP servers carry an explicit `type` in `.mcp.json`, `.cursor/mcp.json`, and `.warp/.mcp.json`. Closes #375.
- Gemini SSE MCP servers use `url`, not `httpUrl`. Closes #376.

## v0.31.0 - 2026-06-03

### Added

- `x-<target>` keys reach every target with frontmatter or TOML to hold them, stay scoped to that target, and emit in sorted order. Closes #367 (#368, #369).

### Changed

- Docs: `README.md` and every `docs/` page say the same facts in fewer words; `CHANGELOG.md` gains an entry-style line.

## v0.30.0 - 2026-06-02

### Changed

- The managed `.gitignore` block ignores each generated directory with one rule such as `/.claude/` instead of listing every file in it.

## v0.29.0 - 2026-06-02

### Added

- Docs: recommend gitignoring generated outputs via `gitignore.enabled`.

### Changed

- Docs: `configuration.md` fixes the default target set (12 of 14; omits `amp`, `warp`) and links `targets.md` instead of duplicating the entry-point list.

### Fixed

- Restored overlay helper files such as `.claude/README.md` land in the managed `.gitignore` block and the output ledger instead of showing up untracked.
- `sync --only` and `--except` no longer delete the files of targets they skip; cleanup waits for the next full sync.
- Managed `.gitignore` entries are root-anchored (`/AGENTS.md`), so a generated path no longer ignores a same-named file nested elsewhere. (#362)
- `import claude` propagates a nested `.claude/CLAUDE.md`'s instructions to every target, not just the claude overlay. (#361)
- Playground: optional source reads treat a missing `js/wasm` filesystem as absent instead of failing with `ENOSYS`.
- Playground renders each target's root entry-point file (CLAUDE.md, AGENTS.md, ...) alongside per-spec artifacts.

## v0.28.0 - 2026-06-01

### Added

- Per-target models: `model:` accepts a map such as `{claude: opus, codex: gpt-5.5}`, with an optional `default`; a bare string applies to every target.

## v0.27.0 - 2026-05-29

### Added

- `import antigravity` and `import continue` (rules + MCP yaml) close the sync -> import -> sync round-trip for both targets.
- `.agnostic-ai/AGNOSTIC_AI.md` is now the canonical entry-point body, tracked in git.
- `sync` ledger at `.agnostic-ai/.sync-state` sweeps orphan generated files on the next run.

### Changed

- Every generated file carries the provenance header with a "do not edit" hint.

### Fixed

- Every adapter passes a full audit: golden output, capability parity, provenance headers, and byte-equal sync -> import -> sync where supported.
- Cursor `.mdc` frontmatter double-quotes `globs:` so `**/*` parses as a YAML string instead of an anchor reference.
- Import unwraps `## Rules`, `## Agents`, and `## Skills` wrappers and provenance comments, so legacy concatenated rules files round-trip byte-equal.
- `import codex` reads `outputs.codex.rules-file` and more MCP fields; sync removes an empty `.codex/config.toml` and legacy `.agents/` trees.
- Shared MCP JSON importer synthesizes `type: http` from url-bearing entries so claude/amp/opencode pick the http transport branch on emit.
- Per-adapter `testdata/kitsink/` trees ship in git even when the underlying patterns are gitignored.

## v0.26.1 - 2026-05-27

### Changed

- `agnostic-ai upgrade` and the install docs print `brew upgrade --cask Chemaclass/tap/agnostic-ai`, which avoids ambiguity with the formula namespace.

### Fixed

- `upgrade --run` stops before a doomed `brew upgrade --cask` when `<brew>/bin/agnostic-ai` is a stray file, and prints a `rm + brew install --cask` hint.

## v0.26.0 - 2026-05-25

### Added

- `import.codex.shred: false` keeps each `AGENTS.md` as a single rule spec instead of splitting by `##` heading. Closes #248.
- Hook frontmatter accepts `target: <name>` or `targets: [a, b]` to scope a hook to specific CLIs. Closes #249.
- `agnostic-ai doctor` flags divergent hook script bodies across tools and suggests consolidating to `.agnostic-ai/scripts/<basename>`. Closes #251.
- `outputs.codex.exec-policies` / `outputs.codex.exec-policies-file` render Codex CLI's `prefix_rule(...)` DSL into `.codex/rules/default.rules`. Closes #254.
- Codex hooks emit into `.codex/hooks.json` with matcher-aware dedupe, `timeout`, and `statusMessage`; override with `outputs.codex.hooks-file`. Closes #255.
- `target:`, `targets:`, and their `-exclude` forms scope every spec kind, not only hooks. Closes #292.
- Per-target body fences: wrap prose in `::target codex` / `::end` markers to emit that block only to the named target. Closes #293.
- `import claude` / `import codex` auto-set `target: <tool>` on agents/skills present in only one tool's tree. Closes #299.
- `import codex` auto-fences divergent agent and skill bodies: shared prose stays plain, each tool's own section goes in a `::target` block. Closes #300.

### Changed

- `import <tool>` auto-sets `target: <tool>` on imported hooks so codex-specific scripts no longer leak into claude `settings.json`. Closes #257.
- Codex `agents-dir` default changed from `.agents/agents` to `.codex/agents`. Override via `outputs.codex.agents-dir`. Closes #252.
- Codex `skills-dir` defaults to `.codex/skills`, and `shared-subagents` defaults to `true` whether or not claude is enabled. Closes #253.

### Fixed

- Hook path rewriting now covers shell-expansion forms like `"$(git rev-parse --show-toplevel)/.codex/hooks/x.sh"`. Closes #250.
- `import codex` captures `.codex/rules/default.rules` into an overlay so exec-policies survive import → sync round-trips. Closes #265.
- `import claude/codex` captures helper files (CLAUDE.md, README.md, scripts) into overlays with file mode preserved. Closes #267.
- `.claude/settings.json` round-trip preserves source indent and hook event order. Closes #268.
- `outputs.<target>.provenance-header: false` suppresses the `Generated by agnostic-ai` comment. Closes #269, #276.
- `import codex` captures inline `justification` and `match` kwargs from `prefix_rule(...)` calls so they survive round-trips. Closes #277.
- `import codex` lifts a file-level comment block above the first `prefix_rule(...)` into a sidecar overlay so it re-renders on sync. Closes #283.
- `import codex` preserves claude-conventional frontmatter key order when merging agent specs. Closes #282, #284.
- Codex exec-policy emitter writes `justification` and `match` as inline kwargs for byte-stable import → sync round-trips. Closes #280, #281.
- `import codex` after `import claude` no longer overwrites claude-specific skill frontmatter (`argument-hint`, `allowed-tools`, etc.). Closes #286, #287.
- Claude skill emit skips `agents/openai.yaml` (codex-only metadata) from the source skill folder. Closes #288, #289.
- `.codex/hooks.json` emits events in lifecycle order (`PreToolUse` before `PostToolUse`). Closes #290, #291.
- Codex agent TOML emits keys in codex-docs convention order for byte-stable round-trips. Closes #294, #295.
- Routing keys (`target`, `targets`, and their `-exclude` forms) no longer leak into emitted frontmatter. Closes #303.
- Auto-fenced spec bodies no longer contain spurious blank lines between sections. Closes #306.
- `import codex` quotes plain-scalar SKILL.md frontmatter values containing `#` so strict YAML no longer truncates descriptions at the first hash. Closes #317.

## v0.25.0 - 2026-05-23

### Added

- `init --gitignore` flag, plus TTY prompt to enable the managed gitignore block.
- `.agnostic-ai/scripts/<tool>/<basename>` stashes hook script bodies; sync rebuilds `.<target>/hooks/` on every run so `.<tool>/` can stay gitignored.
- Codex import reads `.codex/hooks.json` alongside `config.toml`; previously skipped.
- Sync warns before overwriting hand-authored entry-point files (no agnostic-ai header).

### Changed

- `sync` footer counts only files that actually changed.
- Hook command paths rewrite `.<sibling>/hooks/` → `.<target>/hooks/` per target.
- Codex agent filenames canonicalise to dash-case; underscore form preserved via `x-codex.name`. Claude + codex variants merge onto one spec.
- Codex overlay capture preserves multi-line TOML literals and key order (text-level strip instead of decode/encode).

### Fixed

- Codex emit: multi-command hook arrays now emit one `[[hooks.<event>]]` block per command (was dropping `command` entirely).
- Claude settings.json: `timeout` and `statusMessage` round-trip.
- Claude import: spurious "AGNOSTIC_AI.md seeded" log only prints when the mirror actually wrote.
- Codex AGENTS.md shred skips rules that already exist from a prior import.

## v0.24.0 - 2026-05-19

### Added

- `sync` prints success footer: `✓ synced N targets · M files · Xms`. Suppressed by `--quiet` and `--json`.

### Changed

- Capability warnings group by kind across targets in one line, such as `! 5 hooks unsupported by cursor, copilot, aider, ...`, with one suppression hint.
- `sync -v` prints per-target `created / updated / unchanged` counts, and the footer counts only files that changed.
- Status symbols are colorized on tty (`✓` green, `!` yellow, `✗` red). Honors `NO_COLOR=1`. Pipes and redirects stay plain.
- The `sync --watch` banner shows the path count and backend, and each re-sync starts with a timestamped `[HH:MM:SS] change · <path>` line.
- Unchanged capability warnings shrink to a one-line reminder on repeat runs. Delete `.agnostic-ai/.sync-state` to show the full block again.

## v0.23.0 - 2026-05-17

### Added

- `agnostic-ai upgrade` prints the upgrade command for your install method; `--run` runs it and `--check` flags `PATH`-shadowed copies.
- Integration tests: `sync --check` and `doctor --fix` are zero-drift no-ops after `import claude` / `import codex` / both. Closes #232.
- README: `brew upgrade` line + releases page link.
- Docs cover the v0.22 import changes: `import codex` reads `.codex/prompts/` and `.codex/config.toml`, and `import claude` reads `.mcp.json`.
- Import summary prints `→ <overlay> seeded from <native>` for `import claude` and `import codex` when overlay file written. Closes #231.
- `sync --watch` re-emits when you edit `.agnostic-ai/overlays/`, such as `claude.settings.json` or `codex.config.toml`. Closes #234.
- Integration tests cover six round-trip edge cases, such as MCP `env` and `headers`, folded YAML scalars, and skill assets keeping exec bits. Closes #233.

## v0.22.0 - 2026-05-17

### Added

- Integration tests chain `claude → import → sync codex → import → sync claude` and the inverse, checking that every shared kind survives.
- `import codex` reads `.codex/prompts/*.md` into commands, so Codex slash prompts are no longer dropped and overwritten on the next sync.
- `import codex` saves other `.codex/config.toml` keys, such as `model`, to `.agnostic-ai/overlays/codex.config.toml`, and sync writes them back.
- `import claude` reads `.mcp.json`, one MCP spec per server, so Claude Code MCP servers reach other targets instead of being dropped.

### Changed

- Codex agent TOML keeps nested tables under `x-codex`, such as an agent-scoped `[mcp_servers.<name>]`, instead of dropping them on the next sync.
- `.claude/settings.json` writes hooks in lifecycle order with `{matcher, hooks}` and `{type, command}` key order, even on a first sync.

### Fixed

- Frontmatter keeps each scalar's source quoting: a plain `argument-hint: <ver>` stays plain and a double-quoted value stays double-quoted.

## v0.21.0 - 2026-05-16

### Changed

- Releases publish a Homebrew cask (`Casks/agnostic-ai.rb`) that clears macOS quarantine; `Formula/agnostic-ai.rb` stops getting updates. Closes #225.

### Removed

- `autoSync`, `sync --auto-sync`, the first-run auto-sync prompt, and the `auto-sync` rule. Existing `autoSync:` keys are ignored.

## v0.20.0 - 2026-05-16

### Fixed

- Frontmatter emit no longer force-quotes plain `description:` scalars, so long descriptions stay on one line. Closes #226.

## v0.19.0 - 2026-05-16

### Added

- `revert --force`: delete adapter-emitted files without a `.bak` (restores pre-#217 behavior). Closes #217.

### Changed

- `cleanup` defaults to .bak removal; `--backups` kept as alias. Closes #219.
- `revert` preserves files without a `.bak` (helpers next to `SKILL.md`, propagated templates). Pass `--force` to delete. Closes #217.
- `outputs.codex.shared-subagents` defaults to `false` when `claude` is enabled (avoids duplicating `.claude/skills/`), `true` when codex is alone. Closes #216.

### Fixed

- `doctor --fix` keeps `enabledPlugins` and `statusLine`, a clean sync reports no drift, and import keeps overlay key order. Closes #215.
- Frontmatter scalars containing `<`/`>` keep their quotes; long descriptions no longer wrap at 80 cols. Closes #218.

## v0.18.0 - 2026-05-16

### Added

- `doctor --check-globs`: opt-in flag rules whose `globs:` matches no path in the working tree. Closes #208.
- `cleanup --backups`: removes `*.bak` left by `sync --backup`. Closes #197.
- `outputs.codex.shared-subagents` (default `true`): set `false` in claude+codex setups to drop the duplicate `.agents/skills/` tree. Closes #194.
- `spec.Entry.MetaKeys` exposes source frontmatter key order; external adapters receive it as `meta_keys` (additive, no protocol bump).

### Changed

- Frontmatter emit preserves source key order, uses 2-space sequence indent, prefers double quotes. Closes #190, #191, #193.
- `.claude/settings.json` keeps overlay key order and writes hook keys and events in documented order; codex and opencode JSON match. Closes #192.
- Capability warnings collapse to one line per target and kind, with a count and an `on-unsupported: silent` hint. Closes #204.
- `import --dry-run` lists paths + count instead of dumping file bodies. Matches `sync --plan`. Closes #205.
- `doctor` drift splits into "missing" vs "stale (edited locally since last sync)". Closes #207.

### Fixed

- emit: normalize trailing newlines to exactly one `\n`. Closes #195.
- `doctor` no longer false-positives drift on `.claude/settings.json` after sync (OrderedJSON round-trip is now byte-stable). Closes #200.

### Docs

- README: rewrite "byte-identical" round-trip claim to match reality (content-preserving; marker + canonical formatting applied). Closes #201.
- README: target table separates native from convention paths. Closes #202.
- README + getting-started: recommended adoption workflow (split `import` and `sync` into two commits). Closes #196.
- AGNOSTIC_AI.md template: new "Target-specific overlays" section. Closes #203.
- spec-format hooks: render-to-target schema mapping (Claude / Codex / Gemini). Closes #206.
- spec-format skills: nested helper files round-trip verbatim. Closes #211.
- getting-started: import scope boundary + `.agnostic-ai/.sync-state` reference. Closes #209, #210.

### Tests

- claude import: provenance-marker round-trip stays marker-free across sync ↔ import cycles. Closes #198.

## v0.17.0 - 2026-05-15

### Added

- `agnostic-ai lsp`: LSP server on stdio; pushes lint diagnostics on open/change/save. Closes #168.
- `packs add` / `init`: auto-add `.agnostic-ai/packs/` to `.gitignore`. Closes #170.
- `doctor` / `sync --check`: now detect drift in `AGNOSTIC_AI.md` and target entry-point files (`CLAUDE.md`, `AGENTS.md`, etc.).

## v0.16.0 - 2026-05-15

### Changed

- `sync` distributes `AGNOSTIC_AI.md` to `CLAUDE.md`, `AGENTS.md`, and other entry points, seeds it when absent, and keeps content written by `import <target>`.
- `AGNOSTIC_AI.md` is no longer auto-added to `.gitignore`; commit it as a source file.
- `outputs` key in `agnostic-ai.yaml` is fully optional; omitted when empty in the JSON envelope sent to external adapters.

## v0.15.1 - 2026-05-15

### Changed

- `scripts/release.sh` refuses to cut a release with no new commits or an empty `[Unreleased]` section.

## v0.14.2 - 2026-05-15

### Added

- `import`: accept multiple sources (`agnostic-ai import claude codex`). `AGNOSTIC_AI.md` mirrors last source (last-wins). `all` stays exclusive.

## v0.14.1 - 2026-05-15

### Added

- `init`: scaffold `commands/` source folder and `sources.commands` entry.

## v0.14.0 - 2026-05-15

### Added

- `sync --plan`: per-target diff summary without writing (#161).
- `sync`: atomic transaction; partial writes roll back on failure (#162).
- `sync.collision-policy`: `prompt`, `prefer-spec`, `fail` (#167).
- `import all`: detect and import every installed CLI (#160).
- `init --from <cli>`, `init --dry-run`, `import --dry-run` (#158, #160).
- `agnostic-ai lint`: LINT001 to LINT005 semantic checks, `--strict` (#171, #177).
- `agnostic-ai doctor`: `mcp` / `install` / `config` subcommands, `--json` (#159).
- `agnostic-ai install-hook`: pre-commit `sync --check`, `--shared` for `.githooks/` (#169).
- MCP spec: `description`, `disabled`, `roots` (#177).
- Claude import: `.claude/commands/*.md` round-trips frontmatter (#177).
- Codex `outputs.codex.config`: first-class `notify`, `profiles.<name>`, `model-providers.<id>` (#177).
- Codex `[mcp_servers.<name>]`: `description`, `disabled`, `roots` (#177).
- Codex agent: top-level `tools:` first-classed in agent TOML (#177).
- Golden snapshot tests per adapter (#166).

## v0.13.0 - 2026-05-15

### Added

- `AAI-NNN` error codes; `agnostic-ai explain <code>` (#163).
- `agnostic-ai why <file>` traces a file back to adapter/specs/config/timestamp, `--format json` (#164).
- `agnostic-ai graph` spec → target → file. Formats: text, mermaid, dot, json. Filters: `--target`, `--spec`, `--kind` (#172).
- `claude`: `outputs.claude.settings.*` sets keys such as `model` and `outputStyle` over the captured overlay; spec hooks still win for `hooks` (#177).

### Changed

- **BREAKING**: every entry point gets one pointer body, plus `.agnostic-ai/AGNOSTIC_AI.md`. Set `outputs.<target>.rules-file` for the old concat (#153).
- `import` next-steps suggest `sync` + hint other detected CLIs.
- `init` next-steps adapt to context.
- `init` adds `.agnostic-ai/.sync-state` to `.gitignore` (#151).
- Provenance header shortened to `Generated by agnostic-ai`.

## v0.12.0 - 2026-05-14

### Added

- `antigravity` adapter (Google Antigravity IDE). Supports `rule` + `agent`. Emits `.agent/rules/*.md` and `.agent/AGENTS.md`.

### Changed

- `import` writes `AGNOSTIC_AI.md` to `.agnostic-ai/` instead of project root. Delete old root file after upgrading.
- `init` two-step next-steps: `import <target>` then `sync`.
- Adapter outputs now start with `Generated by agnostic-ai. Do not edit by hand.` header. Importers strip on round-trip. Closes #140.

### Fixed

- `spec`: derive skill name from parent directory when frontmatter absent.
- `claude`: no leading blank line when frontmatter empty (#137).
- `claude`: per-file rules round-trip frontmatter, drop synthetic `# <name>` heading (#138).

## v0.11.0 - 2026-05-14

### Added

- New `command` spec kind. `commands/*.md` emits per-target slash commands.
- `claude`: `.claude/commands/<name>.md`. Override via `outputs.claude.commands-dir`.
- `codex`: `.codex/prompts/<name>.md`. Override via `outputs.codex.commands-dir`.
- `sources.commands` config key (default `commands`).
- `emit.CopyTree` for mirroring file trees (honors capture/dry-run/recording/backup).

### Changed

- Spec loader skips kinds whose `sources.<kind>` is empty rather than walking the layer root.
- `init` pre-ticks targets whose marker exists (`.claude/`, `.codex/`, etc.).

### claude

- `import claude` mirrors full skill dirs (helper scripts, fixtures, subdirs) byte-for-byte.
- `sync` propagates skill sibling files into `.claude/skills/<name>/`. Mode bits preserved.

### codex

- `import codex` mirrors full `.agents/skills/<name>/` (including `agents/openai.yaml` + assets).
- `sync` propagates skill sibling files. `x-codex`-derived `agents/openai.yaml` wins over source copy at same path.

## v0.10.0 - 2026-05-14

### claude

- Rules emit per-file under `.claude/rules/<name>.md`. `CLAUDE.md` untouched. Set `outputs.claude.rules-file: CLAUDE.md` for legacy concat.
- `.claude/settings.json` keeps user keys: `import claude` saves them to `.agnostic-ai/overlays/claude.settings.json`, and sync adds spec hooks on top.
- Hooks merge by `event` + `matcher`; `command:` accepts string or list.

### codex

- Agents emit at `.agents/agents/<name>.toml` (was `.codex/agents/`). Override via `outputs.codex.agents-dir`.
- `import codex` round-trips agents/skills/hooks/MCPs. Unknown TOML keys land under `x-codex`.

### gemini

- `command:` accepts list; each entry emits as separate `{matcher, command}` pair.
- Fix: emit no longer drops `command` when spec uses a list.

### all

- Hook spec filenames derive from content hash (`<event>[-<matcher>]-<hash8>.yaml`). Re-imports converge.
- JSON outputs no longer HTML-escape `&`, `<`, `>`.

### Dependencies

- `github.com/BurntSushi/toml` v1.6.0 (codex TOML parsing).

## v0.9.0 - 2026-05-14

### Added

- Playground UX: target chips, per-target file selector, theme toggle, mobile layout.
- Source URL in `--help` / `--version`.
- `import` for **aider, amp, warp, gemini, copilot, opencode, zed** (full kind coverage where supported).
- Every importer mirrors target's main file to `AGNOSTIC_AI.md`. Last import wins.
- `import claude` prefers `.claude/rules/*.md` over slicing `CLAUDE.md`.
- `import copilot` lifts leading italic into `description:`; routes `agent-*` / `skill-*` files to matching source dir.

### Changed

- `init` prompts for targets by default on TTY. Pipe a list, or `--all` / `-a` to skip. `-i` / `--interactive` removed.
- `DefaultTargets()` no longer includes `amp` and `warp` (collide with codex on root `AGENTS.md`).

### Fixed

- `import claude` keeps preamble before first `##`; ignores `##` inside fenced blocks.
- `agnostic-ai.yaml`: only `version` required.
- `sync` / `sync --check` fail fast with `output collision` when two targets write the same path.

## v0.8.0 - 2026-05-13

### Added

- `agnostic-ai.local.yaml` per-machine overrides; deep-merged over base. Auto-gitignored by `init` (#128).

### Changed

- Renamed `agnostic.config.yaml` → `agnostic-ai.yaml`. Legacy still loads with deprecation warning (#128).

## v0.7.0 - 2026-05-13

### Added

- `new <kind> <name>` scaffolds a single spec (#31).
- `render <spec> [--target <t>...]` prints emission to stdout (#31).
- `explain <spec>` lists every output a spec contributes to, `--json` (#40).
- `init --preset <go|ts-react|python>` (#47).
- `sync --watch-poll` forces polling backend (#36).
- VS Code extension `editors/vscode/` (#44); JetBrains plugin `editors/jetbrains/` (#101). Schema validation, render commands, drift status indicator.
- Pre-commit recipes (pre-commit/lefthook/husky) at `docs/user/git-hooks.md` (#39).
- WASM playground at `docs/playground/` (#43).
- `validate` rejects unknown hook `event:` with supported list (#112); warns on hook/MCP with no consumer (#114).
- `doctor` resolves stdio MCP `command:` on PATH; install hints for missing npx/uvx/python/docker (#113).
- Opt-in native surfaces: Copilot chat modes, Cursor commands, Cline, Windsurf, and Warp workflows, Continue assistants, Zed tasks (#104, #105, #106, #107, #108, #109, #110).

### Changed

- `sync --watch`: fsnotify + 50 ms debounce (sub-100 ms re-sync, zero idle CPU). Polling kept as fallback (#36).
- Code of Conduct contact: `conduct@chemaclass.dev` → `agnostic-ai@chemaclass.es`.

## v0.6.0 - 2026-05-12

### Changed

- **BREAKING**: Amp default `AGENT.md` → `AGENTS.md` (per Sourcegraph spec). Legacy auto-renamed to `AGENT.md.bak` (#67).
- **BREAKING**: Warp default `WARP.md` → `AGENTS.md` (per Warp Rules / AGENTS.md standard). Legacy auto-renamed to `WARP.md.bak` (#68).
- Go toolchain bumped to 1.24 (#76).

### Added

- Native multi-file emission for ◐ adapters:
  - **Copilot** (#64): `.github/instructions/<name>.instructions.md` per scoped rule.
  - **Gemini** (#65): hierarchical `GEMINI.md` + `.gemini/commands/<name>.toml`.
  - **OpenCode** (#66): `.opencode/commands/<name>.md` per agent.
  - **Amp** (#67): hierarchical `AGENTS.md` + `.agents/commands/<name>.md`.
  - **Warp** (#68): hierarchical `AGENTS.md` (agents inlined).
- Hooks and MCP reach Codex (#78), Gemini (#79), Continue (#80), Amp (#81), Zed (#82), Warp (#83), and OpenCode (#84) config files.
- New `outputs` fields: `instructions-dir`, `commands-dir`, `mcp-dir`, `emit-skills-as-commands`.
- `sync --json`, `sync --check --json`, `revert --json`, `doctor --json` (schema v1).
- `agnostic-ai status`: project name, layers, spec counts, targets, last sync, drift count. `--json`.
- First-sync target picker (#92): interactive multi-select when config still lists every target. Selection persisted.

## v0.5.0 - 2026-05-07

### Added

- `sync --only <targets>` / `sync --except <targets>` filters (mutually exclusive). `revert` gains same flags.
- `agnostic-ai validate --fix` rewrites autofixable specs, such as backfilling a missing `name:`; plain `validate` marks fixable issues with `*`.
- Plugin protocol v1 for external adapters (`agnostic-ai-adapter-<target>` on PATH; JSON over stdin/stdout). Docs at `docs/internal/plugin-protocol.md`.
- `adapters.Resolve(name)`: lookup site with built-in → external fallback.
- `agnostic-ai packs add|remove|update|list`: shareable spec packs from Git URLs. Pinned in `agnostic.packs.lock`. Load as a layer before project specs.
- `docs/user/ci.md`: dedicated CI page + `chemaclass/agnostic-ai-action@v1`.
- `completion bash|zsh|fish|powershell`.
- JSON schemas at `docs/schemas/config.schema.json` (generated) and `spec.schema.json` (hand-authored).
- `init` injects `yaml-language-server` schema hint into generated config.

### Changed

- GitHub Release body built from `CHANGELOG.md` via `scripts/release-notes.sh`. No post-hoc `gh release edit`.
- `promote_changelog` writes `## vX.Y.Z - YYYY-MM-DD` (no brackets). `## [Unreleased]` keeps brackets.
- CI lint upgraded to `golangci-lint v2.6`.

### Fixed

- Spec frontmatter parse errors report `path:line:col`. Malformed YAML no longer silently treated as body.
- `extract_changelog_section` accepts both `## [vX.Y.Z]` and `## vX.Y.Z`.

## v0.4.0 - 2026-05-05

### Added

- `sync --watch`: re-emit on changes (200 ms poll; Ctrl+C exits). Incompatible with `--check`.
- `sync --auto-sync=yes|no`: first-sync TTY prompt; writes auto-sync rule spec; persists answer to config.
- `init --demo`: seed one example spec per source folder.
- `init -i` / `--interactive`: multi-select target picker.
- `import <source>`: translate existing CLI config into specs. Sources: `claude`, `codex`, `cursor`, `cline`, `windsurf`, `continue`.
- Codex subagents: `.codex/agents/<name>.toml`. `x-codex` frontmatter passes through.
- Codex skills: `.agents/skills/<name>/SKILL.md`. `x-codex.interface`/`policy`/`dependencies` add `agents/openai.yaml`.
- `--help` examples on every command.

### Changed

- `init` scaffolds under `.agnostic-ai/` by default. `init .` for legacy root layout.
- `sync` skips empty stub files.
- `list` / `validate` print hint when no specs loaded.
- Codex `AGENTS.md` lists agents as pointers, not inlined bodies.

### Removed

- `init --from <source>` flag. Use `init` then `import <source>`.

## v0.3.0 - 2026-05-04

### Added

- MCP servers as first-class spec kind (`mcps/*.yaml`). Propagates to Claude (`.mcp.json`), Cursor, Copilot (VS Code shape). Other targets warn.
- `sync --backup`: copy each existing file to `<path>.bak`. Opt-in. Skipped on no-op writes.
- `agnostic-ai revert`: restore `.bak` if present, otherwise remove. `--dry-run` to preview.
- Auto-managed `.gitignore` block. `gitignore.enabled: true` (or `sync --gitignore on`); lines outside block preserved.
- Provenance markers: each `###` section in merged docs opens with `<!-- source: <relpath> -->`.
- Nested rule scoping. Subdir specs (`rules/backend/auth.md`) carry implicit scope; per-dir adapters route accordingly.
- New adapters: `amp` (`AGENT.md`), `zed` (`.rules`), `warp` (`WARP.md`), `opencode` (`.opencode/AGENTS.md`).
- `init --from cursor`: import `.cursor/rules/*.mdc` (frontmatter passes through).
- `init --from codex`: walk for `AGENTS.md` at any depth; translate each `## section` into a rule with inferred `globs:`.

## v0.2.0 - 2026-05-04

### Added

- Skill emission: rules-dir adapters write per skill file; merged-doc adapters list under `## Skills`.
- Generated-by header on merged-document outputs.
- `internal/testutil` with `Chdir` / `TempCwd`.
- `.github/dependabot.yml` weekly updates.
- `SECURITY.md`.
- Roadmap doc.

### Changed

- README simplified; details moved into `docs/`.
- README aligned with AGENTS.md open standard.
- `internal/cli/import_claude.go` split into per-concern files. No behavior change.
- CI writes coverage profile; uploads to Codecov.
- `make build` adds `-trimpath -ldflags="-s -w"`.
- Makefile gains `lint`, `fmt`, `vet`, `cover`.

### Fixed

- CI Windows runner pinned to `bash`.

## v0.1.0 - 2026-05-04

### Added

- Initial release: adapters for Claude Code, Codex, Gemini CLI, Cursor, GitHub Copilot, Aider, Cline, Windsurf, Continue.
- Commands: `init`, `sync`, `validate`, `list`.
- `init --from claude`: import existing `CLAUDE.md` + `.claude/` config.
- `sync --check` and `doctor` for drift detection.
- `x-<target>` frontmatter namespace.
- Docs, examples, integration tests, dogfood specs.
- OSS scaffolding: CONTRIBUTING, COC, GOVERNANCE, issue/PR templates, CI, GoReleaser, golangci-lint, Dockerfile, lefthook, Taskfile, dependabot/renovate.

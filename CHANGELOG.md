# Changelog

Each release lists general changes first, then changes by tool, then site work. Releases up to v0.72.0 group entries as Added, Changed, Fixed, and Removed. Versioning: [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Entry style, section order, and what belongs here instead of the issue or the docs: `.agnostic-ai/agents/changelog-curator.md`.

## [Unreleased]

### General

- Opt-in `memory-hook` built-in loads the memory index at session start on Codex, Copilot, Cursor, Gemini CLI, Qoder, and Factory (#1850, #1851).
- Opt-in `memory` built-in keeps one project memory in `.agnostic-ai/memory/` that every tool reads and writes; Claude Code imports its index (#1844).
- The `memory` built-in adds personal memory in `.agnostic-ai/local/memory/`, saved without asking and ignored by Git (#1852).
- With `memory`, OpenCode and Kilo Code list both memory indexes in `instructions` and keep the user's own entries (#1845).
- A built-in spec inlined into a shared instructions file names its source as `builtin:<name>` instead of a cache path (#1844).

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

Releases before v0.70.0 are in [docs/CHANGELOG-archive.md](docs/CHANGELOG-archive.md).

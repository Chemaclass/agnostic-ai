# Changelog

Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Versioning: [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Entry style, section order, and what belongs here instead of the issue or the docs: `.agnostic-ai/agents/changelog-curator.md`.

## [Unreleased]

### Added

- OpenHands: a shttp MCP server's `oauth` field (or an explicit `auth: oauth`) emits `auth = "oauth"` in `config.toml`, and import reads it back (#1157).
- Agents map `readonly: true` to Factory's `tools: read-only` and no inherited MCP servers, overriding a portable tools list (#1162).
- Project specs in `.agnostic-ai/local/` override shared ones, its `AGNOSTIC_AI.md` extends every entry point, and it stays gitignored (#1172).
- A local spec edits one field of a shared spec and keeps the rest; a `::parent` line in its body extends the shared body. (#1177)

### Changed

- Specs in `~/.agnostic-ai/local/` merge into shared ones instead of replacing them. Set a field to `null` to drop it. (#1177)

### Fixed

- Global sync writes the Codex skill policy file and warns when a manual-only skill stays model-invocable on a target (#1156).
- `.junie/AGENTS.md` honors `sync.resolve-imports`; a legacy `rules-file` entry point gets local instructions; `sync --watch` sees a new local layer (#1172).
- `import` keeps `.agnostic-ai/local/` specs, hooks, and scripts out of the shared source, and leaves shared specs and settings untouched (#1174).
- `sync --watch` sees `agnostic-ai.local.yaml`, `.agnostic-ai/overlays/`, and source dirs created mid-session, and now watches commands (#1179).
- `sync --watch` re-syncs when `.agnostic-ai/AGNOSTIC_AI.md` changes, and falls back to polling when the OS drops file events (#1184).
- `import codex` keeps a local hook out of the shared source when Codex reorders its matcher or joins it with another hook's that runs the same command (#1185).

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

---

Releases before v0.60.0 are in [docs/CHANGELOG-archive.md](docs/CHANGELOG-archive.md).

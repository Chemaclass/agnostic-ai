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
- `import` no longer copies specs from `.agnostic-ai/local/` into the shared source or overwrites a shared spec with its local version, hooks and their scripts included; shared settings, reviews, and environments stay as they were when the local layer feeds them (#1174).
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

- Codex agents map the portable `effort` onto `model_reasoning_effort`; an integer budget raises a coverage note, and `x-codex.model_reasoning_effort` still wins (#1016).

### Changed

- The Amp target page and adapter doc note that Amp also loads skills from Claude Code's directories by default, and name the settings that change it (#1023).
- Target audits fetch each vendor page once through `scripts/docfetch.sh` and hash it against `scripts/target-audit/sources.lock`, so auditors read only the pages that moved (#1017).

### Fixed

- Windsurf (Devin) agents and permission rules granting only `Write` now emit `write`, not `edit`, so they no longer also grant edit capability (#1022).
- `import` keeps the skill and agent frontmatter a target has nowhere to put, so a sync then import no longer deletes `argument-hint`, `allowed-tools`, `tools`, or `effort` from a spec; deleting a key the target does write still reaches the spec.
- `sync` prints the Devin shared `.agents/agents/` coverage note once and stops repeating it while it is unchanged (#1014).
- The Amp docs no longer cite an exact settings-schema property count that goes stale with every new key; the Kilo config-precedence and Copilot chat-mode citations point at pages that still carry those sections (#1024).
- npm releases allow about 20 minutes for publish-time scanning before reporting a package missing (#1013).

### Site

- The home page uses its full width: the source spec runs across the top with each target's generated file beside the target list, and a section title pairs with the text or links beside it above 1000px.
- Code blocks, generated files, and the closing call to action share one dark slab, so a listing is distinguishable from the prose around it at a glance.
- The header reads Home, Docs, Targets, Updates, Playground, marks the current section with an underline on its own edge, stacks the nav under the brand from 820px, and leaves the version badge and GitHub button to the footer.
- The docs sidebar marks the current guide on a hairline rail, the guide index reads as cards rather than underlined rows, and the table of contents escapes heading titles so `x-<target>` no longer pulls the footer into the right column (#1015).
- The targets page leads with the capability matrix, defining sync states and spec kinds and moving shared details to a reference; the README links npm, the Homebrew tap, and GitHub Releases directly; the changelog keeps v0.50.0 and later, with earlier releases in `docs/CHANGELOG-archive.md`, which `release-notes.sh` falls back to.

## v0.64.1 - 2026-09-21

### Changed

- npm platform packages use the project's `@agnostic-ai/<os>-<cpu>` organization scope instead of the maintainer's personal scope.

## v0.64.0 - 2026-09-21

### Changed

- `npm install agnostic-ai` downloads nothing: the binary ships in a platform package npm picks by `os` and `cpu`, so it installs under `--ignore-scripts`, offline, and behind a proxy. Pin a version with `agnostic-ai@0.62.0`; `AGNOSTIC_AI_VERSION` no longer applies on npm, and a global copy missing its platform package wants `npm install -g agnostic-ai --force --include=optional` (#942).
- A release publishes the six platform packages before the parent that pins them and derives a dist-tag from the version, so a partial publish fails instead of handing a prerelease npm's `latest` (#942).
- The Homebrew cask is pushed by `Chemaclass/homebrew-tap` itself every half hour, so this repository no longer needs a tap secret; both existing token routes still work and write the same file (#920, #943).
- Target audits reuse one issue index, load vendor references by target, and fit available worker slots.

### Added

- `agnostic-ai lint` warns (LINT009) on an allowed `Bash(...)` rule with a `*` before the end of the command, such as `Bash(git * main)`, which also approves options inserted at that spot.
- Gemini emits and imports default-model settings, and Qoder emits HTTP and prompt hooks (#998, #1005).
- This repository dogfoods the three spec kinds it never used: `ignore`, `review`, and `environment` (#980).

### Fixed

- Claude rejects disabled project MCP servers and preserves manual rejection entries when re-enabled (#997).
- Import preserves Cline and Continue rule conditions, both Cline rule roots, and compatible Junie, Warp, and OpenCode skills (#999, #1000, #1001, #1002).
- Factory keeps scoped skills in their project areas, and Trae rejects invalid native agent names (#1004, #1006).

### Site

- The site builds on Zola 0.23.6: templates moved to Tera 2 components, and three site tests that silently skipped on a mismatched local Zola now run (#970).
- The ten spec kinds stop being a bare list: the playground's kind picker, the landing, and the README each say what a kind is and link its definition (#980).
- The landing says the same things in fewer words, ships its generated-file tablist inert until `landing.js` wires it, and drops two installers it never rendered (#977, #991).
- The targets index records Cursor's default-on compatibility hooks and required `type: stdio` field (#1007).

## v0.63.0 - 2026-09-20

### Added

- An agent's `effort` takes a per-target map the way `model` does: `effort: {claude: xhigh, qoder: 8000, default: high}` resolves per target instead of writing a broken nested mapping (#968).
- Settings specs take an `x-<target>` block, so a key such as `x-factory.sandbox` reaches that target's settings file instead of being dropped without a word (#949).
- Amp accepts settings specs: `x-amp` keys merge into `.amp/settings.json`, and each portable field reports why Amp has nowhere to put it (#950).
- Factory writes the portable permission policy to `commandAllowlist`, `commandDenylist`, and `commandBlocklist`; portable `ask` prompts, portable `deny` cannot be approved (#948).
- An agent's `memory` scope reaches Qoder, `import kiro` reads `.kiro/hooks/*.json` back, and the npm package publishes with a provenance attestation (#937, #952, #953).

### Changed

- The release mints its Homebrew token per run from a GitHub App scoped to the tap, falling back to the stored `HOMEBREW_TAP_TOKEN` until the App exists (#943).
- The npm package points its homepage at agnostic-ai.org and widens its keywords from nine to twenty (#937).

### Fixed

- An `x-<target>` settings key no longer deletes the translated policy it collides with: lists union, objects merge, and an unmergeable shape prints a coverage note. A map of whole records is the exception: an `x-qoder.mcpServers` entry naming a server the MCP specs emitted replaces it whole (#966, #974).
- An `x-<target>` hooks block merges with the generated one event by event, and a wrong-shaped permission key raises a note naming the key and both shapes instead of shipping the translated policy in silence (#976).
- Permission rules reach the keys their vendors document: `mcp__<server>__<tool>` lands in `opencode.json` as `<server>_<tool>`, and `WebSearch` reaches `.devin/config.json` as `web_search` (#947, #951).
- Installing the latest release no longer dies on a shared IP: the install scripts and `upgrade` follow the `releases/latest` redirect instead of calling `api.github.com`, whose per-IP limit failed installs with HTTP 403 (#940).
- The npm wrapper survives the four ways its download used to fail, names the archive when `tar` fails, and exits `128 + signal` so a killed run is distinguishable (#936).
- An integer `effort` raises Factory's coverage note instead of vanishing, `brew` stops printing a deprecation for our cask, the release retries a lagging registry replica, and `update` only reports a PATH copy that actually wins (#933, #937, #968).

### Site

- The hero shows one agent spec and the five native files it really produces, pickable, with every path and body copied from a real `sync`, and rests as a finished fan under `prefers-reduced-motion` (#967).
- The landing reads lighter on a phone: the six-column matrix collapses to five chips, the nav holds only pages of this site, and every outbound link opens in a new tab with `rel="noopener noreferrer"`.
- The spec-format page merges per-target models and `effort` into one section with a worked example, and `spec.schema.json` finally types `model` as a map as well as a string (#968).
- Four target pages drop a claim the vendor does not make, covering Copilot's `deniedUrls` and settings read, Claude's deprecated `taskOutputMaxChars`, Cursor's three skill roots, and MCP `roots` (#955, #956, #957, #959).
- Installation lists the two package managers that work today and names the two that do not, the talk demo moves to `/docs/#demo`, and the current release shows beside the wordmark (#920).

## v0.62.0 - 2026-09-19

### Added

- OpenCode reads your permission policy: the portable `allow`, `deny`, and `ask` lists translate into the `permission` map in `opencode.json`, and `import opencode` reads them back (#922).
- Kilo Code reads your permission policy: portable `allow`, `deny`, and `ask` lists reach `kilo.jsonc`'s `permission` map, an agent's `tools` list translates into the same per-tool shape, and `import kilo` reads both back (#890).
- Hook specs reach two more targets: OpenCode as plugin modules at `.opencode/plugins/<name>.ts`, and Cline as one executable script per event under `.cline/hooks/` (#889, #892).
- Import catches up with what sync writes: `import goose` reads Goose's agents, skills, plugin hooks, reviews and rules, `import trae` reads `.trae/hooks.json`, and `import cline` reads `.agents/skills/` (#889, #894).
- Copilot reaches three more documented keys: `disabledMcpServers` in `.github/copilot/settings.json`, `cwd` and `env` on hooks, and a per-server `tools` allowlist on Copilot CLI MCP entries (#888).
- A settings `model` reaches Factory at `<project>/.factory/settings.json`, the documented project tier (#891).
- `agnostic-ai import all` detects a Goose project from `.agents/plugins/` or `.agents/REVIEW.md`, not just the opt-in `.goosehints`, so a default Goose sync is no longer skipped (#906).

### Fixed

- Cline agents load again: they emit as `.cline/agents/<name>.yml` with the `name` and `description` frontmatter the loader requires, and `import` reads both that and the old `.md` (#886).
- Claude Code's AGENTS.md fallback no longer leaks routing: `CLAUDE.md` is always written, so an `outputs.claude.rules-file` override cannot hand Claude Code codex's `AGENTS.md`, and `import claude` reads the same fallback files the tool itself loads (#885, #893).
- An exact `Bash(cmd)` in a `deny` list now blocks the command on Windsurf instead of being dropped and leaving it unblocked (#887).
- `import augment` reads your permission policy again: it looked for `tool-name` where Augment and our own emitter write `toolName`, so every rule was skipped (#912).
- `sync` no longer fails on Windows when two targets write the same shared directory: pruning a legacy tree treated the platform's "directory is not empty" refusal as an error instead of a lost race (#918).
- Copilot, Junie, and Codex say what a permission policy does there instead of dropping it in silence: each reports a coverage note naming where that vendor keeps its rules, and Codex's points at `outputs.codex.exec-policies` (#917, #923).
- Emitted config matches what each vendor documents: Cursor stdio MCP entries carry the required `"type": "stdio"`, and `validate` flags a copilot hook spelled `SubagentStart`, an event no Copilot page names (#888, #895).
- Antigravity stops writing to a legacy path and past a documented limit: `sync --global` writes skills to `~/.gemini/config/skills/`, and a rule over the 12,000-character cap reports a coverage note (#896).
- `agnostic-ai import antigravity codex` works: multi-source validation reads the same list as the help text, so a source the error message calls supported is no longer rejected (#905).
- `validate` flags a kiro hook spelled `AgentSpawn`, which Kiro CLI 3.0 documents nowhere, and accepts `Manual`, which it does (#907).
- A release now checks that Homebrew and npm actually serve the tag it just cut, instead of reporting success when a missing token silently skipped the push (#920).

### Site

- Site search ranks the page a query names ahead of that page's own sections, finds a page by text anywhere in it, folds plurals, and caps one page at three of the ten result slots.
- Target pages drop stale claims: Cline reads both `.clinerules/` and `.cline/rules/`, Trae emits scoped rules, Junie also loads `.agents/skills/`, aider writes `.aiderignore`, and goose's `.goosehints` needs the Developer extension (#897).
- The Claude Code plugin carries a README covering both install commands, its four skills, and updating; the settings spec page names the right targets for `permissions` and `model` and states the rule grammar it uses in every example.

## v0.61.0 - 2026-09-18

### Added

- Portable permission lists reach Windsurf's `.devin/config.json` and Augment's `.augment/settings.json`, and `import` reads them back (#856, #872).
- Point a per-kind output key into `.agents/plugins/<name>/` and Antigravity ships a workspace plugin, manifest included (#810).

### Fixed

- Cline rules reach the model again: they emit to `.clinerules/`, the only path Cline reads, not the unread `.cline/rules/` (#853).
- A moved Claude Code directory round-trips: `import`, `doctor`, and the per-kind `agents-dir` and `skills-dir` keys all follow `outputs.claude.dir` (#852).
- Copilot's `{{rules_dir}}` resolves from `outputs.copilot.instructions-dir`, the key it reads, so a spec body names the directory sync writes to.
- Copilot and Cursor import every project skill directory their vendors document, so a repo on the shared `.agents/skills` layout keeps its skills (#854).
- MCP entries match what each vendor documents: no `ws` server on Augment or Qoder, no `disabled` on Junie, and `args` on every Warp stdio entry (#855, #858, #859).
- Per-agent fields reach the targets documenting them: `effort` and `mcpServers` on Qoder and Factory, `permissionMode` and scoped `hooks` on Qoder (#812, #824, #825, #826).
- A configured directory reaches the tool: `outputs.kilo.skills-dir` is registered in `skills.paths`, and a Goose skills-only bundle gets its manifest (#861, #862).
- Windsurf ignore specs also write `.windsurfignore` for agent file access, and sync reports the agent profile Devin would load twice (#863).
- A field a target cannot honor reports a coverage note instead of vanishing: `disable-model-invocation`, `color`, and Antigravity rule activation (#811, #864, #865).
- Sync refuses an OpenCode skill or Junie agent name the vendor's regex rejects, instead of writing a file the tool never loads (#857).
- Cursor hooks filtering `beforeTabFileRead` or `afterTabFileEdit` no longer raise LINT005, since the vendor's matcher table documents both (#860).
- Two fork pull requests on branches with the same name no longer cancel each other's CI run (#850).

### Site

- Cursor runs `.claude/settings.json` hooks by default now that both opt-in gates are gone, so a repo syncing claude and cursor runs every hook twice (#865).
- The `/updates/` target filter closes on an outside click, on Escape, and when focus leaves it.
- The docs drop throat-clearing and repeated facts, and the Antigravity page moves to the vendor's current URLs (#865).

## v0.60.0 - 2026-09-18

### Added

- `agnostic-ai init --demo` seeds a `memory-curator` skill for Claude Code and Qoder, which audits an agent memory store and proposes merges and deletions, applying nothing until you confirm (#842, #843).

### Fixed

- The managed `.gitignore` block ignores Claude Code's machine-local `agent-memory-local/` instead of the shareable `agent-memory/`, so a hidden `agent-memory/` reappears in `git status` (#841).
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

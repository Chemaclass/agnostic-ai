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

## v0.59.0 - 2026-09-16

### Added

- `agnostic-ai verify` fails CI on stale generated output and fingerprints the harness for a project-owned verifier (#834).
- Portable model settings round-trip through Codex, Copilot, OpenCode, Junie, Qoder, and Kilo, with shared allow, deny, and ask permissions on Qoder (#806, #827).
- Goose and OpenHands emit project agents to `.agents/agents/<name>.md`, and Goose hook specs emit a full Open Plugins package across 12 lifecycle events (#631, #629).
- Amp environment specs emit `.agents/setup` scripts and supervised `.amp/services.yaml` services (#637).
- Augment and Factory emit native commands, Augment ignore specs emit to `.augmentignore`, and directory-scoped skills stay scoped on Codex, Cursor, Warp, and OpenCode (#630, #808, #805).

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
- User guides live at `/docs/`, sharing one Markdown source with the repository and `llms-full.txt`, and `/agent-setup.txt` gives a coding agent a safe setup path.
- The landing page leads with a copy-ready install command, a three-step activation path, and a ten-target comparison.
- The capability matrix filters and shares comparisons by URL, and the playground covers all ten spec kinds with CI checking both against adapter declarations.
- Release briefings ship with each release, keeping the changelog separate from verified upstream news, and the updates archive filters editions by target and search term.

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

- `sync.unmanaged` lists user-owned output paths that sync never writes, copies, or removes, and that `doctor`, the ledger, the `.gitignore` block, and `revert` all leave alone (#781).
- `::target` fences work in `.agnostic-ai/AGNOSTIC_AI.md`, so a fenced paragraph reaches only the entry-point files a listed target reads (#781).
- Claude emits and imports HTTP, MCP-tool, and prompt hooks; Cursor emits native prompt hooks (#767, #768).
- Ignore specs reach Crush's `.crushignore` and Kilo's `.kilocodeignore`, and review specs reach Goose's `.agents/REVIEW.md` (#770, #773, #771).
- Continue imports JSONC MCP maps, and Zed, Warp, and Antigravity import native skill folders with bundled assets (#764, #765).

### Fixed

- Hook scripts from `.agnostic-ai/scripts/` go through the sync session, so a failed sync rolls them back and deleting the hook sweeps the script (#789).
- Deleting a skill removes its bundled files from every target, and sync no longer leaves stale output behind or reports every prior output as an orphan (#781, #785).
- `::target` fences survive a round trip: `import` keeps a fenced entry point, `validate` accepts external adapters, and a dropped fence leaves no blank line (#781, #790).
- Sync preserves what it does not own: VS Code `inputs` and `sandbox`, Continue, Factory, and Kilo MCP options, and hand-authored ignore file order (#757, #769, #763, #774, #775, #761).
- Native output matches what each tool loads, on Gemini hooks, Zed skill names, and Kiro timeouts and actions (#762, #766, #772).

## v0.56.1 - 2026-09-12

### Fixed

- `curl -fsSL ... | bash` installs again: `latest_version` piped curl into `grep -m1`, so curl took EPIPE and `pipefail` propagated it, and the tag resolved while the install died. Piped into `/bin/sh` it previously exited 0 having installed nothing.

## v0.56.0 - 2026-09-12

### Upgrade notes

Three changes make a previously green repo fail. All three are deliberate.

- `lint` now errors on an MCP spec missing `command:` or `url:` (LINT008). The entry was dead on every target and nothing said so.
- `sync` now fails with `AAI-103` rather than overwriting a hand-authored ignore file. Run `agnostic-ai import <target>`, then sync again.
- Gemini agents move to `.gemini/agents/`. Set `outputs.gemini.emit-agents-as-commands: true` to keep typing `/name`.

### Added

- Hook specs reach five more targets: Copilot at `.github/hooks/agnostic-ai.json` across 14 events, Factory across nine, Trae across six, Crush on `PreToolUse` only, and Augment across five with timeouts in milliseconds (#629).
- Hook specs accept `args`, which switches a Claude Code hook to exec form so a path carrying a space or `$` runs as written; Qoder emits the same form, and Copilot maps it onto its own `exec` key with a coverage note naming the CLI-only trade (#732, #746, #755).
- Ignore specs reach Kiro (`.kiroignore`), Trae (`.trae/.ignore`), and Junie (`.aiignore`), which all three document; until now every ignore spec skipped on them with a warning (#728).
- Gemini agents emit as native subagents at `.gemini/agents/<name>.md`, so an agent is delegatable, invokable with `@name`, and listed by `/agents`, with `tools` translated onto Gemini's own names (#733).
- `lint` reports an MCP server missing the field its transport requires (LINT008): most targets wrote a server object with nothing to start or connect to, and the rest dropped it in silence (#747).

### Changed

- Gemini agents no longer write `.gemini/commands/<name>.toml` by default and a managed file left there is swept; set `outputs.gemini.emit-agents-as-commands: true` to keep the `/name` prompt (#733).
- Qoder hook events sort in the vendor's own order, so `.qoder/settings.json` changes bytes once for a project mixing documented and custom event names (#744).

### Fixed

- `sync` no longer silently destroys a hand-authored ignore file on seven targets: it fails with `AAI-103`, names the patterns that would be lost, and writes nothing. `import <target>` reads the file into an ignore spec (#754).
- `sync` no longer deletes every user-authored key in a JSONC config file. `kilo.jsonc` and two siblings document `//` comments that `encoding/json` rejects, and the swallowed parse error took credentials and permission settings with it (#725).
- `import gemini` reads `.gemini/commands/*.toml` again and writes them to `<commands>/`, after #748 turned that read into a fallback and any project with one subagent lost every command (#750).
- Continue MCP servers reach the shapes its schema accepts: `streamable-http` for `type: http`, `headers` under `requestOptions`, `env` on stdio only, and a coverage note for `ws` or a missing required field (#726, #730, #739).
- Warp skips an MCP entry missing `command` or `url` instead of writing a server with nothing to launch, Crush accepts every hook-event spelling it documents, and Codex keeps `startup_timeout_ms` through a round trip (#731, #735, #753).
- `sync -t amp` stops writing Agent specs to the directory Amp's own migration tells users to delete, and sweeps what a previous sync left there (#727).
- `validate` accepts Qoder's four documented hook events, Cursor's five matcher-consuming ones stop raising LINT005, and a Kilo command named `goal` surfaces a coverage note for the name Kilo reserves (#734, #736, #744).
- Docs correct stale vendor claims across Copilot, Trae, Zed, Qoder, Factory, Cursor, and the spec-format skills list, and record that `.claude/settings.json` has become a cross-tool hook file, so syncing `claude` beside `copilot` can run a hook twice (#737, #745, #755, #756).

## v0.55.0 - 2026-09-10

### Added

- Windsurf hooks emit to `.devin/hooks.v1.json` across eight events, with `type` accepting `prompt` as well as `command`, and `import windsurf` reads them back (#629).
- Qoder hooks emit to `.qoder/settings.json` across 23 events, matching Claude Code's tool-name matcher vocabulary (#629).
- Kilo Code and Qoder commands emit natively, with the `description` frontmatter each vendor requires; both previously skipped with a warning (#630).
- Trae rule frontmatter merges `x-trae` custom keys, most notably `scene: git_message`: the merge helper commands already used was never called, so no custom key reached a rule file (#635).

### Fixed

- Kiro skills emit natively to `.kiro/skills/<name>/SKILL.md` instead of flattening into steering, keeping bundled assets and reaching Kiro's own skill picker; a stale flattened copy is swept (#642).
- `import codex` reads inline hooks in the vendor's documented nested shape, which every vendor example uses and which previously imported as zero hooks with no warning (#669).
- Docs list every target that emits hooks and commands natively, after both enumerations drifted behind the adapters (#629, #630).
- Amp, Junie, Trae, Kiro, Warp, and Cursor docs drop stale vendor citations and add vendor-confirmed fields (#647).

## v0.54.0 - 2026-09-10

### Added

- Directory-specific rules via `new rule --scope`, with native scoped output for 19 targets: scoped bodies stay out of root instructions, and preflight rejects unsafe shared readers and destination conflicts (#704).
- The site and playground move onto a single amber accent on ink, with every foreground/background pair checked against WCAG AA.

### Changed

- Documentation starts with a focused first-rule tutorial and dedicated installation, migration, and troubleshooting guides; scoped-context docs add a runnable walkthrough.

### Fixed

- Codex: an MCP server name with a package-style character, now allowed by Codex CLI 0.152.0, wrote a TOML table header its own parser rejects, breaking every server in the file (#706).
- Windsurf: `outputs.windsurf.workflows-dir` warns instead of writing files nothing reads, since Devin removed the only agent that ever read a Workflow (#707).
- Junie docs name `allowPromptArgument` as vendor-documented, and the Kiro MCP comment drops an implied confirmation it never had (#708).

## v0.53.0 - 2026-09-09

### Added

- OpenHands hooks emit to `.openhands/hooks.json` across six events; a matcher copied from a Claude spec raises a coverage note, since OpenHands names its own tools (#629).
- MCP passthrough gaps close on codex and copilot: eight documented Codex keys plus an `oauth` sub-table, and VS Code's `cwd`, `envFile`, `dev`, and `sandboxEnabled`, all previously dropped in silence (#692).
- Codex hooks gain `type: mcp_tool`, which calls a connected server's tool instead of running a shell command and was silently dropped before (#693).
- The published site is findable: canonical URL, Open Graph and Twitter cards, structured data, `llms.txt`, `sitemap.xml`, and `robots.txt` with real commit dates.

### Fixed

- `sync --jobs` no longer fails intermittently with `invalid argument` when two targets share a directory: macOS raises `EINVAL` where the retry only handled `ENOENT` (#701).
- Warp: sync warns when a hand-authored `WARP.md` sits beside the `AGENTS.md` it just wrote, since Warp reads `WARP.md` first and every synced rule was reaching nowhere (#691).
- Cursor skills promote the documented `icon` and `color` to first-class keys, which previously reached the file only through `x-cursor` (#694).
- The generated entry-point body offers `.geminiignore` instead of the `.aiexclude` this tool stopped writing in #625 (#651).

## v0.52.1 - 2026-09-07

### Changed

- The npm publish no longer rides on every package-manager push succeeding: it gates on the release archives being present and skips cleanly when that version is already on the registry, so a re-run is safe.

## v0.52.0 - 2026-09-07

### Added

- `agnostic-ai sync --global` installs shared user-level instructions, rules, hooks, and skills from `$AGNOSTIC_AI_HOME` as native configuration for 22 of the 25 targets, each at its documented user-level path, preserving unrelated native content (#680).
- Codex MCP servers emit the vendor's per-tool sub-tables from a `tools` map, covering `output_token_limit` and the per-tool approval override, both dropped in silence before (#678).
- `outputs.claude.settings.bashOutputMaxChars` and `.taskOutputMaxChars` raise how much output Claude Code takes inline before spilling to a file, up to 128K characters (#679).

### Changed

- Ordinary project sync no longer loads `~/.agnostic-ai/` as a low-precedence spec layer; it is now the explicit global source for `sync --global` (#680).
- Crush docs record that `crush.json` is the vendor's deprecated legacy format, frozen from new fields and merged with `crushrc`; emission is unchanged (#674).

### Fixed

- `sync --global` removes a hooks file it has nothing left to put in, instead of leaving a `{"hooks": {}}` shell; a file holding any other key is kept (#680).
- `sync --jobs` no longer fails intermittently on a directory two adapters share: a write now recreates a parent the prune removed, and the prune accepts a directory another target just wrote into.
- Kiro and Antigravity adapter docs record a vocabulary conflict between two vendor pages and drop a stale "stays unconfirmed" note the vendor's own payload settles; behavior is unchanged (#675).

## v0.51.0 - 2026-09-04

### Added

- MCP servers stop dropping documented vendor fields on ten targets. Claude Code gains `timeout` and `alwaysLoad`, plus `headersHelper` and an `oauth` object on a remote server; Gemini `timeout`, `trust`, `description`, `includeTools`, `excludeTools`; Codex `http_headers_helper`, `enabled_tools`, `disabled_tools`; Cursor a stdio server's `envFile` and a remote server's static-OAuth `auth`; Kiro `autoApprove`, `disabledTools`, and a remote server's `oauth` and `oauthScopes`; Crush `disabled`, `sessionless`, `enabled_tools`, `disabled_tools`; OpenCode a local server's `cwd` and either transport's `timeout`; Qoder the nine fields on the vendor's "Common Optional Fields" table plus a stdio server's `cwd`. Amp and Zed reach theirs through `x-amp` and `x-zed`. Every field was vendor-documented and dropped in silence before, with no coverage note (#634, #641, #661).
- Augment MCP servers merge into `<workspace>/.augment/settings.json` under `mcpServers`, alongside `shell`, `theme`, and other Auggie CLI settings the file already holds. MCP specs previously reached no Augment surface at all (#633).
- Copilot MCP servers also emit to `.github/mcp.json`, the file Copilot CLI reads. The CLI does not read `.vscode/mcp.json` and calls its `servers` key unsupported, so a Copilot CLI user with no VS Code in the loop previously got no MCP server at all. Override with `outputs.copilot.cli-mcp-file` (#646).
- Agents reach the subagent loader on Windsurf, Trae, and Antigravity. Each writes a native profile file (`.devin/agents/<name>.md`, `.trae/agents/<name>.md`, `.agents/agents/<name>.md`) instead of flattening into an always-on rule file, so `model`, `tools`, and the other documented frontmatter fields stop being dropped. A stale `agent-<name>.md` in the rules directory is swept on the next sync (#638).
- Windsurf translates an agent's `tools` onto Devin's own `read`/`edit`/`grep`/`glob`/`exec` vocabulary under the key `allowed-tools`. Antigravity never writes a generic `tools` list, because its vocabulary shares no name with agnostic-ai's and the vendor warns an unmapped name can hang the subagent; set `x-antigravity.tools` instead (#638).
- Windsurf writes the shared root `AGENTS.md`, the file Devin CLI reads automatically and its docs call the recommended way to give a project rules. A windsurf-only repo had no root entry point at all before, so unscoped rules reached Devin through no path (#645).
- Factory and Goose emit skills to the shared `.agents/skills/` tree. Neither had a skill surface before, so a skill spec reached either target only by accident, through another enabled target's write (#632).
- OpenHands emits a rule carrying `globs`/`paths` or a source-layout scope as a native path-triggered rule (`.agents/skills/<name>/SKILL.md`, `paths:` frontmatter), the vendor's deterministic per-file mechanism. The rule now loads only for the files it scopes, at zero context cost until touched, instead of an always-on block in `AGENTS.md` (#643).
- OpenHands emits an environment spec's `install` field as `.openhands/setup.sh`, the vendor's documented repository bootstrap script. Environment specs previously reached no OpenHands surface at all (#662).
- Codex hooks accept `async: true` to run a command hook in the background instead of blocking the session on it. `import codex` reads it back from `.codex/hooks.json` (#636).

### Changed

- Qoder MCP servers move from the project-root `.mcp.json` to `.qoder/settings.json`, Qoder's own documented project-level location. The old path is Claude Code's, and the two targets' documented per-server fields have diverged far enough that sharing it would trip sync's collision check. Delete a leftover `.mcp.json` by hand in a Qoder-only project: it still loads and outranks the new file. In a project that also targets Claude Code, that file is Claude Code's and stays, so Qoder resolves a same-named server from it and the fields above do not take effect (#641).
- Warp MCP servers no longer emit `description`, `disabled`, or `roots`. Warp publishes two closed property tables and names none of the three, so the keys did nothing. `disabled` now raises a coverage note, and the other two stay reachable through `x-warp` (#641).
- Codex's sweep of its pre-v0.26 `.agents/agents/` agents tree is scoped to `.toml`, the extension Codex agents use. Antigravity's subagents live in that same vendor-documented directory as `.md`, so the wholesale sweep deleted them and whichever adapter ran last decided whether the project had subagents at all (#638).

### Fixed

- Cline rules that set `alwaysApply: false` carry a `paths` glob array, Cline's one documented conditional, so a rule scoped to `src/components/**` stops loading on every request (#639).
- Continue rules carry `name`, `globs`, `alwaysApply`, and `description` frontmatter, with `globs` falling back to the rule's source-layout scope. No rule file carried frontmatter before, so every scoped rule was always-on. `x-continue.regex` reaches the file too (#639).
- `validate` accepts `PreModelSwitch`/`PostModelSwitch` (Claude), `Interrupt` (Codex), and `AgentSpawn` (Kiro) hook events instead of rejecting them as unknown. All three already reached their target's emitted file correctly; only `validate`'s allowlist was stale, which broke CI for a project with a correct spec (#660).
- Warp's skills doc names `WARP_SKILL_DIRS`, scoped to Cloud agents indexing skills outside the repo, not the general `SKILLS_DIRS` this repo's own docs and adapter comment claimed. A user who exported `SKILLS_DIRS` got nothing (#663).
- Kilo Code's docs note `.kilo/kilo.jsonc` outranks the root `kilo.jsonc` this adapter writes when both files exist. Documented as a caveat: the vendor's config precedence is a merge across named sources, not an exclusive first-match read, so this stays a doc clarification rather than a behavior change (#644).

## v0.50.0 - 2026-08-28

### Added

- `outputs.copilot.root-mcp-file` writes Copilot's MCP servers to a workspace-root `.mcp.json` under `mcpServers`, the key Copilot CLI and VS Code's Agent Host accept (neither reads `.vscode/mcp.json`, whose `servers` key the vendor calls unsupported). Opt-in, so no project gains a root file it did not ask for (#610, #622).
- Path variables in spec bodies: `{{$SKILLS_DIR}}`, `{{$AGENTS_DIR}}`, `{{$COMMANDS_DIR}}`, `{{$RULES_DIR}}`, and `{{$MCP_FILE}}` expand to each target's own location, so one spec can say where files go without hardcoding one tool's layout. An `outputs.<target>.<field>` override wins, and a variable a target has no surface for is left verbatim with a coverage note rather than blanked (#616).
- `lint` flags frontmatter keys that near-miss a key agnostic-ai owns (`allowed_tools`, `allowedTools`, `allowed-tools`, `disallowed_tools`, `max_turns`, `model_name`). Such a key parses, emits, and does nothing, so a tool restriction can look set while the agent runs unrestricted. Warn severity, so `lint --strict` gates it; the message suggests moving a genuinely target-native key under `x-<target>` (#617).

### Changed

- `validate` exits 1 when it reports any issue, and `doctor --check-globs` exits 1 when a rule's globs match no files. Both printed the problem and exited 0 before, so a CI step could not gate on either: one project sat 24 versions behind with 23 invalid specs and a green pipeline for months (#617).

### Fixed

- Zed rules reach Zed again. `sync` writes zed's entry-point to `.rules`, which Zed reads before `.github/copilot-instructions.md`; enabling copilot and zed together handed Zed copilot's pointer-only file, and every rule silently stopped applying (#624).
- OpenCode's entry point moved from `.opencode/AGENTS.md` to the root `AGENTS.md`, the only file OpenCode's upward lookup opens, so rule bodies now reach a project that syncs opencode alone. A managed file at the old path is swept on sync, a hand-authored one is kept, and `import opencode` still reads it when the root file is absent (#623).
- Windsurf scoped rules and agents now land at `<scope>/.devin/rules/<name>.md`, a location Devin discovers, instead of `.devin/rules/<scope>/<name>.md`, which no documented discovery path reached. A windsurf rule that sets `alwaysApply: false` also carries the matching `trigger` frontmatter now, so it stops being silently promoted to always-on (#628).
- Gemini ignore specs emit as `.geminiignore`, the file Gemini CLI actually reads. They went to `.aiexclude` before, which belongs to Gemini Code Assist, so every excluded path stayed visible to `@` file references and file search. A managed `.aiexclude` is removed on the next sync (#625).
- Kiro hook files write `"version": "v1"`, the string value the vendor schema documents, instead of the number `1`. A parser that validates the field would reject the file and none of the project's hooks would fire (#626).
- Factory droids translate `tools` onto Droid CLI's own IDs (`Bash` to `Execute`, `Write` to `Create`, `WebFetch` to `FetchUrl`) instead of writing Claude-style names Droid CLI rejects as unknown, which failed the whole droid at load time and kept it out of the picker (#627).

---

Releases before v0.50.0 are in [docs/CHANGELOG-archive.md](docs/CHANGELOG-archive.md).

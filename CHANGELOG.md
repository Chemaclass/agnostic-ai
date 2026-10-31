# Changelog

Changes by release, following [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Releases before v0.90.0 are in [docs/CHANGELOG-archive.md](docs/CHANGELOG-archive.md).

## [Unreleased]

### General

#### Sync and configuration

- Global voice settings round-trip native keys; Qwen, Gemini, Grok and Vibe join support, with notes for unsupported fields (#2833, #2834).
- Global instruction updates keep private modes and stop if the file changes during sync (#2857).
- `explain --global` lists native hook files and copied scripts for Cline, Grok and Kiro, including custom tool roots (#2863).
- Import keeps an imported `effort` when the only other settings spec that sets one targets a different tool (#2796).
- `init` sets minimums; `upgrade --requires` keeps minimum/exact pins or makes ranges minimums; `init`, `use` and AAI-005 explain exact pins (#2841, #2849).

#### Security

- Concurrent updates keep all private Git exclusion entries (#2876).

- Editor and vendor dependencies receive security patches; the scoped audit exception expires on November 14 (#2844).

#### Web interface

- Select controls use a shared, theme-aware chevron with room for labels and keyboard navigation.
- Context size keeps the report area in place while a different coding tool loads, with placeholders instead of stale totals.
- Phone headers keep configuration and editor controls together, and Settings tabs fit their labels and counts.
- Section warnings fold behind Review while errors stay visible; Config files shows merged values once below source links.
- Authority, Settings, and native server imports keep a compact Shared or Personal choice with the content it controls.
- Section headings link to the relevant spec or reference docs in a new tab, including when nothing is configured.
- File links show Open, keep the editor choice visible in the header, and explain how to recover when no editor opens.
- Context size groups tool and file controls, highlights load totals, and makes estimates and ranked sources easier to scan.
- Global Settings edits shared or personal voice values; project settings stay read-only (#2835).
- Authority previews and saves shared or personal permission rules, compares personal overrides with Shared, and leaves applying them to sync.
- Built-in previews group notes by tool, fold long lists, and keep Apply and Cancel at the top.
- All five pages fit narrow screens, source groups use tabs, and Settings shows the selected shared or personal values.
- Tools previews and saves a native-only MCP server as a shared or personal spec for the next sync (#2634).

### By tool

#### Antigravity

- MCP import accepts Windows project links that already work in previews (#2850).
- Import previews follow project links and Windows short paths, rejecting outside MCP files even through source aliases (#2831).
- Plugin MCP files `mcp.json` and legacy `mcp_config.json` get manifests; legacy output notes the move to `mcp.json` (#2826).
- Import reads the configured MCP file, including absolute in-project paths, so plugin servers round-trip (#2829).
- Sync keeps existing plugin manifests it did not write and prints a note.

#### Kiro

- Global sync and import use `~/.kiro` when `KIRO_HOME` is relative, a filesystem root, the home directory, or a home ancestor (#2860).
- Global sync sets voice language in `settings/cli.json`, keeps other settings, and restores the prior language on removal (#2837).

#### Crush

- Sync and import preserve all 13 native hook events and reject matchers on events without tools (#2807).
- Import keeps personal lifecycle commands out of shared specs and preserves handlers with distinct native names or timeouts (#2807, #2827).

#### Zed

- Global instructions use the native macOS and Flatpak roots, moving recorded blocks without changing personal text (#2866).
- Windows global instructions use `%APPDATA%\Zed\AGENTS.md`, keeping personal text while retiring recorded old output (#2857).

#### Goose

- Detection no longer treats Antigravity plugins in `.agents/plugins/` as a Goose project (#2830).
- Sync keeps existing plugin manifests it did not write and prints a note.

#### Kimi Code CLI

- Sync and import reject MCP executors other than `local`, including removed `kaos`; `runtime_id` remains supported (#2808).

#### Mistral Vibe

- Global MCP sync keeps different sampling values; missing means false, and removing owned true needs `x-vibe.sampling_enabled` or sync stops (#2817).

#### Copilot

- Import keeps `enabledPlugins` and `extraKnownMarketplaces` when only another target's settings spec sets those keys (#2799).

#### Gemini CLI

- Import and sync keep MCP OAuth options, including private-network opt-in, without putting literal credentials in specs (#2809).

#### Warp

- Global sync writes voice enabled and language choices to the platform settings file, preserving the activation shortcut (#2836).

#### Claude Code

- Home voice settings enable dictation, select hold or tap, and restore previous values when removed.

### Site

- Hooks docs explain the session flow and common shared hooks, with an optional step-through example.
- Tool docs update retirement dates, Codex history values, Antigravity plugin MCP names, and Qoder and Zed skill paths (#2810, #2811, #2812, #2814, #2818).
- Tool docs explain Continue's unmaintained client and output, Claude scope and settings, and OpenHands MCP timeouts (#2858, #2815, #2813).
- The home page leads with your OS install command, tool count and playground link, then setup steps before the source explorer.
- The docs landing page lists guides once in Start here; explorer model aliases and capability names link to their matching docs.
- Explorer shows config beside specs, fast-tier tool models, and a user-only skill with Codex metadata and Gemini's dropped flag.

## v0.90.0 - 2026-10-29

### General

#### Sync and configuration

- **Breaking:** Sync rejects `.git` specs and output paths outside the project or inside `.git`, except `.git/info/exclude` (#2701).
- `sync` keeps files and shared skill links when a target's external adapter is missing from PATH (#2657, #2702).
- Sync backs up edited files before removal, keeps backups private and out of Git, and preserves case-only renames (#2639, #2640, #2654, #2704).
- Sync recovers invalid state with a warning, skips unsafe entries, and writes state without partial files (#2665, #2701).
- Config errors show the file, line, key, and expected format; unsupported versions and line breaks in `sync.unmanaged` fail (#2648, #2664, #2701).

#### Security

- **Breaking:** `packs update` uses locked commits and pack commands reject unsafe names and sources. Use `--latest` to update (#2638, #2690).
- **Breaking:** Claude Code and Codex imports omit literal credentials; Grok import accepts safe paths, booleans, and token references (#2716).
- Credential checks catch passphrases and commands inside permission rules and hook filters (#2731, #2737).
- Sync blocks writes and removals through links outside the project or into `.git`; `compare` hides credentials (#2706, #2650).
- Installers and `upgrade` verify build provenance when `gh` is logged in; installers reject versions that are not releases (#2655).

#### Windows

- Import rejects spec names containing colons; global import skips them with a warning naming the source file (#2748).
- `sync --check --against` ignores checkout line endings; plain checks explain line-ending drift and links checked out as files (#2773).
- `init --dry-run` avoids listing an existing global note as new when the project path uses a short name or link (#2760).

#### Performance

- Sync runs about 25% faster on a 500-spec project, with fewer repeated renders and external adapter warnings (#2667).

#### Global sync and settings

- `sync --global` preserves your text outside its instructions block and comments in JSON settings with hooks (#2658, #2644).
- `sync --global --check` passes when only ownership records are missing or stale (#2647).
- Personal per-tool `model` and `effort` maps keep shared defaults for tools they leave out (#2602).

#### Commands

- Version mismatch errors offer fixes or a tool session; `upgrade` explains any remaining project version pin (#2726).
- `doctor` reports unknown targets and JSON findings, rejects `--json --fix`, and checks config like sync (#2659, #2660).
- `lint` and editors flag specs a selected tool cannot sync and hooks with nothing to run (#2642, #2645).
- Commands reject unexpected arguments; `new --dry-run` rejects existing specs; import previews handle aliased folders (#2661, #2662, #2675).
- `sync --json` seeds personal memory and lists all project notes; `why` ignores errors from unselected tools (#2663, #2641).

#### Web interface

- **Deprecated:** Edit requests use `audience: shared` or `personal`; `scope` works until the next minor release (#2602, #2615).
- Set per-tool `model` and `effort` and preview sync; **Tools** lists native-only MCP servers to import (#2614, #2619, #2620, #2622, #2624, #2636, #2633).
- Global settings open in their own tab with a link back to the project; `serve` stops promptly on Ctrl-C (#2607, #2616, #2627).
- Lifecycle stages start collapsed with counts and search; the authority view loads faster (#2610, #2631, #2632).
- The sidebar drops the configured tools card and supports page scrolling; search, focus, mobile fields, and empty states improve (#2733, #2729, #2740).

### By tool

#### Claude Code

- `import claude` omits credentials in settings and hooks and rejects old overlays containing them. Use `--overwrite` to replace them (#2649, #2668, #2669).
- With `blockReadsOutsideWorkingDirectories` on, tracked case or look-alike settings cannot enable repository auto memory (#2769, #2772).
- With `blockReadsOutsideWorkingDirectories` on, personal auto memory keeps loading on short or linked project paths (#2760).
- URL-only `dev-commands` attach previews to running servers through `.claude/launch.json` (#2579).
- Sync warns about ignored `x-claude` keys, hooks that cannot run, and models unaffected by global `effort` (#2582, #2583, #2581).

#### OpenCode

- `import opencode` preserves provider settings through sync and warns instead of importing a provider with literal credentials (#2755).
- Agents with V2 fields keep their `temperature` as `request.body.temperature` (#2584).
- `import all` and `init` recognize projects with only a root `opencode.json` (#2765).

#### CodeBuddy Code

- Sync and import support prompt hooks on `Stop`, `UserPromptSubmit`, and `PreToolUse` (#2578).
- Sync preserves `x-codebuddy.permissions` rules when no portable permission sets the same list (#2667).
- `import codebuddy` keeps native `reasoningEffort` when another settings spec sets `effort` (#2789).

#### Mistral Vibe

- Sync rejects the reserved agent name `main` in any letter case; import warns about it (#2575).
- `post_agent` hooks accept `match` and `matcher` to run after a specific agent (#2577).
- Hook errors and docs name Vibe 3.0.3 as the required version for subagent `active_model` (#2585).

#### Copilot

- Import preserves `enabledPlugins` and `extraKnownMarketplaces` through sync for Copilot CLI and cloud (#2756).
- `import all` and `init` recognize projects with only `.github/copilot/settings.json` (#2765).

#### Grok Build

- `sync --check` checks Git ignore rules once, so adding rules no longer slows it down (#2646).
- `init` with a managed `.gitignore` sets `gitignore.commit: [grok:instructions]` (#2642).

#### OpenHands

- **Breaking:** Global sync removes servers from `~/.openhands/mcp.json`. Add them in Agent Canvas, **Customize > MCP Servers** (#2576).

#### Codex

- `import codex` omits credentials from committed overlays and rejects old overlays containing them. Use `--overwrite` to replace them (#2669).

#### Cursor

- Import keeps rules without activation keys off. Set `alwaysApply: false` in previously imported rules without those keys (#2643).

#### Qwen Code

- Commands such as `git:commit` load and sync on Windows; sync rejects output filenames containing colons (#2744).

#### Gemini CLI

- Commands such as `git:commit` sync as nested TOML files and import with their original names, including on Windows (#2747).

#### Junie

- Sync writes `effort` to `.junie/config.json`, replacing any hand-written value; import reads it back (#2782).

### Site

- The Warp guide explains how to copy global MCP servers from the app to the Warp Agent CLI (#2580).
- The Antigravity guide explains that the CLI cannot load `manual` rules through mentions (#2783).

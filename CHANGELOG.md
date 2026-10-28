# Changelog

Changes by release, following [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### General

- `packs update` installs each Git pack at the commit recorded in `agnostic.packs.lock`. Before, it fetched the current tip of the recorded tag or branch and overwrote the recorded commit, so a moved tag or a force push changed the pack with no error. It now fails and leaves the lockfile unchanged when the remote no longer has that commit. `packs update --latest` moves a pack to the newest commit of its ref and prints the old and new commit (#2638).
- `doctor config` fails as `sync` does on an unknown builtin or tier target, an unmet `requires`, or a bad `outputs.<target>.agents`, now AAI-004 (#2660).
- `sync --json` prepares and seeds personal memory like `sync`. Its `notes` now list every project note a plain `sync` prints, such as an overlay file no tool reads, the facts copied into personal memory, and the entry point and worktree notes. Before, a project synced only with `--json` never got its personal memory seeded (#2663).
- Case-only rule, skill, or agent renames keep generated files and protect hand edits with backups or `--keep-edits` (#2640).
- `why` skips errors from tools outside `targets`, so an unselected Grok no longer blocks `CLAUDE.md` provenance (#2641).
- Sync saves edited outputs as `.bak` before removing them when a spec or target leaves, and `revert` restores them (#2639).
- `install.sh`, `install.ps1`, and `upgrade` verify each archive's build provenance when `gh` is logged in, and stop on a mismatch (#2655).
- The installers reject a version that is not a release, such as `v1.0.0/../x`, before downloading anything (#2655).

- `serve` stops at once on Ctrl-C when a browser or client holds a connection open without sending a request. Before, it could wait 5 seconds and fail with "stop the server: context deadline exceeded" (#2627).
- `validate` and `lint` report a hook spec with no `command` and nothing else to run (LINT052), and name a key such as `run:` as the likely misspelling. Before, both passed it, lint warned that it had no body (LINT001), and the sync summary and `explain --global` listed the hook although no tool got it. Lint no longer reports LINT001 for hook specs (#2645).
- A personal per-tool `model` or `effort` map in a settings spec no longer drops the shared single value for the tools it leaves out. Shared `model: sonnet` with personal `model: {codex: gpt-5.5}` now syncs `sonnet` to every other tool, where before they got no model (#2602).
- `sync --global` keeps comments in a JSON settings file that also holds hooks, such as `~/.gemini/settings.json` or `~/.claude/settings.json`. Before, one comment stopped the sync with `invalid character '/'`, even with no hook specs, so model, MCP, and settings changes did not reach that tool (#2644).
- `doctor` fails on an unknown `-t` name, a config `doctor mcp` cannot load, and the glob, nested copy, and hook script findings `--json` skipped (#2659).
- `new <kind> <name> --dry-run` fails with "already exists" for a spec file that exists, like the real run. Before, it printed "would create" and exited 0 (#2661).
- `sync`, `status`, `list`, `validate`, `cleanup`, `graph`, and `upgrade` fail on a positional argument instead of ignoring it. `upgrade v0.56.1` points at `--version v0.56.1`; before, it installed the latest release. `graph --kind` and `graph --target` fail on a value that is not a spec kind or a configured target, with a suggestion when one is close (#2662).

#### Web interface

- Generated-file status and **Refresh** move to **Configured tools** in the sidebar, with a disclosure beside the section links on narrow screens (#2632).
- Lifecycle stages start closed, with counts and descriptions visible; expand one or all, search hooks, or follow a link to open its stage (#2631).
- A project page opens the global settings in a new tab, starting `serve --global` on the next free port when none runs; a global page started in a project links back to it. The header labels each page **Project** or **Global** (#2602).
- Set `model` and `effort` for one tool under **What each tool gets**, saved as `model.<tool>` or `effort.<tool>`. Each tool shows where its value comes from, such as `personal · codex`, and when an `agnostic-ai.yaml` output key replaces it; **Use the value for every tool** removes the tool's entry. `/api/edit/field` and setting previews take `target` (#2602).
- **Settings** names where an edit goes **Shared (committed)** or **Personal (not committed)**, and edit and preview requests take `audience: shared` or `audience: personal`. The older `scope: team` or `scope: local` is accepted until the next minor release; a request that sends both with different meanings returns 400. Change plans carry `audience` beside `scope` until then, and a saved choice on the page carries over (#2602).
- When two project pages open the global settings at the same moment, each opens a global server that links back to its own project. Before, one could open the other project's server, which stopped when that project's server stopped (#2607).
- The authority view checks each distinct text for credentials once per request, so large projects load it several times faster (#2610).
- **Settings** marks a value from a personal file with a **personal** badge instead of **local**, and its notes and built-in previews say personal and shared instead of local and team (#2615).
- Going back from a global page the project page opened returns to the project server that opened it, also when that server runs on `--addr` outside ports 4310 to 4319. Before, it started a second project server on another port (#2616).
- Editing one tool's `model` or `effort` on **Settings** saves when a later spec sets that field only for other tools. Before, a later `model: {claude: opus}` blocked a codex edit in the earlier spec, though codex took its model from it (#2614).
- Editing the `model` of a tool such as codex on **Settings** saves when a later spec sets a Claude model name such as `model: claude-opus-4`, which that tool leaves out. Before, the edit returned 400 although **What each tool gets** showed the tool keeping the earlier value (#2619). This applies to project settings only: `sync --global` writes that name, so the global page still returns 400 for that edit.
- On the global **Settings** page, **What each tool gets** shows a later Claude model name such as `model: claude-opus-4` as the value codex gets, as `sync --global` writes it. Before, it showed codex keeping the earlier spec's value (#2624).
- When a later spec blocks a **Settings** edit, the error names the file that holds the value: the shared spec when your personal file on top of it does not change that value for the edited tool. Before, it named the personal file (#2620).
- Editing `effort` for one tool on the global **Settings** page saves. Before, the request failed with a network error (#2622).

### By tool

#### Claude Code

- `sync --global` notes that a global `effort` reaches Opus 5, Fable 5.1, and earlier models only (#2581).
- Sync notes `x-claude` settings keys that Claude Code ignores in project settings, such as `autoMode` and `deniedModels`, and says to set them in user or managed settings, as each key allows. `sync --global` notes the keys Claude Code reads from managed settings only (#2583).
- A `dev-commands` entry with a `url` and no `command` writes `{name, url}` to `.claude/launch.json`, so the preview attaches to a server you already run; import reads it back as a spec. With Codex enabled, lint reports the entry (LINT016) and sync notes that Codex skips it (#2579).
- Sync notes a prompt, HTTP, or MCP tool hook on an event that cannot run it, and an `if` filter on a non-tool event, where the hook never runs (#2582).

#### CodeBuddy Code

- Sync and import prompt hooks on `Stop`, `UserPromptSubmit`, and `PreToolUse` instead of stopping sync (#2578).

#### Cursor

- `import cursor` writes `alwaysApply: false` for a rule with neither `alwaysApply` nor `globs`, so sync no longer makes it always on (#2643).
- A rule imported and synced before keeps the `alwaysApply: true` sync wrote; set `alwaysApply: false` in its spec to turn it off (#2643).

#### Mistral Vibe

- `post_agent` hooks accept `match` and `matcher`, so a hook runs only after one agent, as Vibe 3.0.3 allows (#2577).
- Sync stops on an agent named `main` in any case, which Vibe 3.0.3 reserves and does not load; import notes it (#2575).
- Docs and the unsupported-hook-field error now cite Vibe v3.0.3, and say a subagent's `active_model` needs v3.0.3 (#2585).

#### OpenCode

- Agents with V2 fields keep their `temperature`, written as `request.body.temperature` (#2584).

#### OpenHands

- **Breaking:** `sync --global` no longer writes `~/.openhands/mcp.json`, which only the unmaintained OpenHands-CLI reads. Add MCP servers in Agent Canvas under **Customize > MCP Servers**, or pass `mcp_config` to the SDK. The next sync removes the servers it recorded as its own there, including a hand-written one an earlier sync adopted because it matched a spec exactly; `--backup` keeps a copy, and `import --global openhands` still reads the file (#2576).

### Site

- The Warp guide explains that global MCP servers reach the Warp app but not the Warp Agent CLI, and how to copy them (#2580).

## v0.89.0 - 2026-10-27

### General

- **Breaking:** Web edit and built-in preview requests must send `scope` (`team` or `local`).

#### Web interface

- Find duplicate startup passages and copy a prompt to review global instructions (#2572, #2567).
- Edit and preview team or personal settings, and see which values each tool receives (#2511, #2553, #2476, #2526, #2512, #2527).
- Search, recheck, and open findings in an editor; see which generated files match their source (#2514, #2515, #2513, #2510, #2519).
- Toggle built-ins in unsynced projects; stale saves keep drafts and cancelled previews stay cancelled (#2484, #2486, #2518, #2542, #2520).
- `ui` reuses servers; lifecycle stages show when they run; `serve --no-installed` skips installed comparisons (#2477, #2492, #2568, #2528).

#### Sync and configuration

- **Breaking:** Global ownership state moves to version 9; older releases cannot read it (#2529).
- Global sync preserves empty objects, guards changed symlink targets, and backs up removed files with `--backup` (#2529, #2517, #2516).
- `sync -v` highlights changes, `sync -vv` shows every target, and parallel sync keeps shared Copilot commands (#2570, #2561).
- Global settings honor target filters, and global rule validation rejects scoped rules in target blocks (#2525, #2508).
- Personal built-ins stay out of the committed `.gitignore`; `init` creates a shared placeholder note (#2494, #2534).

#### Built-ins

- The `caveman` built-in and example pack ship only `SKILL.md`; sync removes generated extras and keeps edited files.
- Edited `caveman` extras remain unowned. Delete them, run `doctor --fix`, or list them in `sync.unmanaged`.

#### Performance and benchmarks

- `compare` finishes in about a second at 500 specs instead of several minutes, with the same report (#2498).
- Command benchmarks can retain paired raw samples and compare fixture, binary, and machine metadata (#2459).

### By tool

#### Codex

- The `terra` model alias now resolves to `gpt-6.1-terra`.

#### Qwen Code

- Rules with `globs` go to `.qwen/rules/<name>.md` with `paths:`, so Qwen loads them only for matching files; import reads them back (#2502).

### Site

- The README and guides give new users a clearer path from installation to project setup (#2522).

## v0.88.0 - 2026-10-25

### General

- **Removed:** `view authority` and `view overview`; use `view` for project or user configuration, or `view --json` for the overview (#2466).
- CLI help groups tasks; the browser adds search and built-in apply/undo (#2319, #2453, #2454, #2460, #2461, #2462, #2465, #2467, #2468).
- `verify --record` saves private command outcomes; `verify compare` reports outcome and elapsed-time changes for matching evaluations (#2455, #2473).
- Vendor watch filters counter-only changes, labels archive additions, and detects edited release notes (#2456, #2472).
- Global import respects existing MCP names regardless of case, and project checks skip built-in hooks for unselected tools (#2434, #2438).

### By tool

#### Grok Build

- Sync and import native project and user configuration with opt-in `grok` (#2434). Set `gitignore.commit: [grok:instructions]`.
- Agent restrictions block unrequested MCP bridge tools; import keeps plugins and rejects credentials in agent metadata and hooks (#2434).
- Sync validates skill and command names and rejects Windows hook paths that need an explicit shell wrapper (#2434).

#### Kimi Code CLI

- Sync and import instructions, agents, skills, MCP, user hooks, and settings with the opt-in `kimi` target (#2436).
- Import rejects credentials in project and user hooks, including command wrappers (#2436).

#### Qwen Code

- Sync and import instructions, agents, skills, commands, MCP, hooks, settings, and ignore files (#2431).
- Import rejects credentials in native metadata and preserves supported settings references (#2431).

#### CodeBuddy Code

- Sync and import native configuration with the opt-in `codebuddy` target (#2433).

#### Mistral Vibe

- Sync and import project and user instructions, subagents, skills, model settings, MCP servers, and hooks (#2429).

#### Pi

- Sync and import instructions, skills, prompt commands, MCP servers, and model settings (#2425).

#### OpenCode

- V2 agent fields keep tool restrictions and ordered permission rules through sync and import (#2432).

#### Codex

- Validation accepts `PreModelRequest` and `PostModelRequest` observer hooks (#2415).

#### Junie

- MCP request timeouts survive sync and import in milliseconds (#2415).

#### Crush

- Skills shared with Goose sync without collisions; commands require a separate Crush skill folder (#2415).

#### Goose

- Skills shared with Crush keep invocation fields that both tools accept and sync without collisions (#2415).

#### Kilo

- Global sync accepts inline shell placeholders in trusted user skills and checks argument placeholders and Claude code blocks (#2415).

### Site

- Target guides clarify global hooks, OpenCode timeouts, Antigravity MCP URLs, Trae context names, and Amp tool names (#2416, #2435, #2444).
- Landing examples show the selected source's output and label excerpts from shared files (#2408).
- Guides explain shared configuration and Composer setup for its Claude Code and Codex providers (#2409, #2426).
- Caveman maintenance research records pinning, ownership, and global setup limits (#2458, #2471).
- Docs add a browser walkthrough, derive tool counts, and correct installation claims, audit dates, and the Go plugin example (#2451, #2464).

## v0.87.0 - 2026-10-22

### General

- Builds use Go 1.27.3 to fix certificate verification vulnerability GO-2026-6809 (#2354).
- `sync` and `lint` flag overlay files no tool reads; `lint --strict` fails on them (#2367, #2368).
- Commands become user-run skills on Codex, Copilot, Zed, Windsurf, and Crush; custom Codex command paths stay in use (#2382, #2386, #2389, #2400).

### By tool

#### Claude Code

- `doctor` finds skills hidden or disabled by `skillOverrides` across settings files; `doctor --fix` offers to re-enable them (#2357, #2359, #2370, #2374).
- `explain <skill>` shows `skillOverrides` by settings file, and `doctor` reports invalid JSON and rejected values (#2371, #2381).
- `doctor --fix` explains when a rejected value leaves a lower settings file's skill override uncertain (#2383, #2384).
- MCP server permissions keep their scope when a server name contains `__` or ends in `_` (#2391, #2397).
- `lint` accepts a matcher on `SessionEnd` hooks (#2392, #2396).

#### Windsurf

- Skills keep user-only invocation in a dedicated skill folder (#2389, #2400).
- Importing a dedicated skill folder after removing `triggers` lifts the restriction for Windsurf (#2402, #2404).

#### Copilot

- Skills keep `argument-hint` and `disable-model-invocation` for VS Code (#2382, #2386).

#### Codex

- `lint` accepts a matcher on `SessionEnd` hooks (#2392, #2396).

#### Zed

- Skills keep user-only invocation unless another tool shares the skill folder (#2389, #2400).

#### Crush

- Skills keep user-only invocation unless another tool shares the skill folder (#2389, #2400).

#### Ellipsis

- Reviews support Gemini, and environment files reject reserved `GEMINI_` and `GOOGLE_` names (#2385, #2398).

### Site

- The home page shows agent setup, native model choices, and consistent tool examples (#2361, #2362, #2365, #2366, #2369, #2372, #2373, #2378).
- Opt-in matrix cells open the setting to configure, and target pages name agent output keys (#2390, #2399).
- Reference pages clarify configuration, specs, hooks, MCP, errors, and user-only sandbox settings (#2375, #2376, #2377, #2379, #2395).
- Memory docs clarify Kiro defaults, and Amp docs explain workspace MCP approval (#2353, #2354, #2356, #2358).
- The playground serves locally and keeps its footer clear; update titles and release examples are readable (#2364, #2380).

## v0.86.0 - 2026-10-20

### General

- `import` keeps tool-specific keys and comments under `x-<target>` across spec kinds (#2340, #2341, #2342, #2343).
- Agents support `maxTurns`, `background`, and agent-only MCP servers (#2327, #2339, #2328, #2334).
- `lint` flags MCP secrets, sync notes suggest fixes, and upgrade prompts are shorter (#2335, #2337, #2329, #2331, #2265, #2266).
- `explain --layers` shows which source files replace or extend a project spec, in text or JSON (#2350, #2352).
- Target audits resume research and verify coverage, claims and metrics (#2280, #2287, #2278, #2286, #2275, #2282, #2277, #2285, #2279, #2284, #2276, #2283).

### By tool

#### Claude Code

- **Breaking:** scoped agent tools become plain tools, so command limits broaden (#2333, #2336); restrict commands with `PreToolUse` or settings deny rules.
- Agents get required names and descriptions; agents with no usable tools are skipped with a note (#2332).
- `lint` names native agent fields and warns when combined descriptions exceed Claude Code's startup budget (#2326, #2330).
- `import --global claude` copies user permission rules into the home, and `sync --global` merges home rules into user settings (#2259).

#### OpenCode

- Native agent permission rules keep their order through import and sync, so overlapping `deny` and `ask` rules stay effective (#2269, #2270).
- Restricted agents keep native V2 project permission limits from `opencode.json`, including ordered deny and ask rules (#2267, #2273).

#### Factory

- MCP `x-factory.enabledTools` survives sync and native import, including empty lists and names also listed in `disabledTools` (#2291, #2297).

#### Ellipsis

- `x-ellipsis.environment.hooks.validate` is written after `before_start` and imported back, so an environment that sets it is kept (#2320, #2323).

### Site

- The homepage shows how one source syncs to each tool, with a comparison to hand-edited files and an explanation of native rule placement (#2317).
- Setup and reference docs use focused pages, shorter navigation, and a safer agent setup checklist (#2289, #2296).
- Claude Code docs explain loaded context, agent triggers, inline MCP, permissions, and memory requirements (#2326, #2330).
- Docs clarify Kiro global ignores and memory defaults, plus shared skill-name precedence (#2349, #2351, #2321, #2292, #2298).
- OpenCode docs distinguish V1 and V2 outside-project access defaults and show a native V2 rule that requests approval (#2293, #2299).

## v0.85.0 - 2026-10-15

### General

- A rule's `paths` scopes it like `globs` on every tool, so Copilot and Cursor stop loading it everywhere; `validate` rejects a mismatch (#2243).
- A rule with `globs` that sync inlines into a root file now says `Applies to:`; run `sync` and commit the one-time diff (#2244).
- `sync --keep-edits` no longer keeps a skill file shared by Codex and Amp as a hand edit after a branch switch, so `sync --check` stays clean (#2245).
- With `sync.collision-policy: prefer-spec`, a file several tools write differently gets the last target's version, and `sync --check` expects it (#2257).
- With `sync.collision-policy: prefer-spec`, sync notes which tool's version wins each shared file and which tools it overrides (#2263).

- Root `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, and `copilot-instructions.md` skip the header when every reader sets `provenance-header: false` (#2256).
- `project --bootstrap` installs bun projects with `bun install --frozen-lockfile`, and names the install to run when it cannot help (#2246).

### By tool

#### Copilot

- Set `chat.useAgentsMdFile: false` in `.vscode/settings.json` when another target writes `AGENTS.md`, so VS Code's local chat loads them once (#2247).
- Write a stdio server's `sandbox` (write paths, denied paths, network domains) to `.vscode/mcp.json`, and keep it through `import copilot` (#2212).
- Read an MCP server's VS Code fields (`cwd`, `envFile`, `sandboxEnabled`, `sandbox`, `oauth`, `dev`) from `x-copilot` too (#2224).

#### OpenCode

- A `deny` or `ask` now wins over a narrower `allow`, as on Claude Code, so a broad `ask` also asks for the commands it allows (#2211).
- An agent's `can` or `tools` list becomes its `permission` map and denies unlisted tools; names with no OpenCode key print a note (#2236).

#### Kilo

- A restricted agent gets the stricter of the project's rules and its own tool list, so a project `ask` or `deny` is never loosened (#2242).
- A bare `Bash` in an agent's tool list now asks, and outside paths and `doom_loop` stay at `ask` instead of being denied (#2239, #2242).
- `Write` rules land under `edit`, the permission Kilo's write tool checks (#2242).

#### Claude Code

- Remove the handoff grant from `.claude/settings.local.json` in one sync after turning handoff off, and the file when it held nothing else (#2222).
- `lint` warns (LINT047) when a skill or command name holds a character Claude Code refuses, such as a zero-width space or `<`, so it would never load (#2217).

#### Factory

- Allow writes to the shared handoff store from linked worktrees when Factory's sandbox is on (#2215).
- The dropped `disable-model-invocation` note names `outputs.factory.skills-dir: .factory/skills`, needed for `x-factory` beside `.agents/skills` (#2213).

#### Windsurf / Devin

- Accept `PreCompact` and `PostCompact` hooks with `manual` and `auto` matchers, as in Devin CLI v3000.11.92; `hook run` takes them from `--payload` (#2237).

#### Codex

- Report an agent `service_tier` that Codex ignores; Codex follows the root session's tier (#2216).

#### Ellipsis

- `import ellipsis` keeps a reviewer without an `environment` in the basic sandbox; `x-ellipsis.reviewer.environment: null` does it by hand (#2214).

#### Crush

- The dropped `disable-model-invocation` note names `outputs.crush.skills-dir: .crush/skills`, needed for `x-crush` beside `.agents/skills` (#2213).

#### Qoder

- Remove the handoff grant from `.qoder/settings.json` in one sync after turning handoff off, and the file when it held nothing else (#2222).

## v0.84.0 - 2026-10-14

### General

- Share personal memory across worktrees (`init --memory`), keep facts Claude Code saved, and flag hidden characters in memory (#2104, #2109, #2100, #2105).
- Keep private handoffs and Git snapshots across worktrees, and list every saved task with `handoff list` (#2117, #2203).
- The first sync in a fresh clone is several times faster, and the change summary counts only the targets each change reaches (#2205, #2183, #2204).
- `import --dry-run` matches the real import, `import` keeps frontmatter that parses the same, and `lint` flags a repeated key (#2181, #2182, #2202).
- Set one personal session age limit for Claude Code, Gemini CLI, and Qoder CLI with `session-retention-days` (#2127).

### By tool

#### Codex

- Keep `x-codex.features` switches and `skills.config` entries in agent files, and name any `x-codex` value TOML cannot hold (#2165).
- Report agent approval policies, model providers, and MCP servers that Codex ignores; configure them in the parent session (#2156).
- `import codex` leaves the personal memory and shared handoff stores out of `codex.config.toml` (#2186).
- The "hook is untrusted" note is short and names each hook the way `doctor --json` does (#2183).
- Accept `fork` as a session-start source in hook diagnostics (#2158).

#### Windsurf / Devin

- Rules you add by hand to `allow`, `deny`, or `ask` in `.devin/config.json` stay next to the rules your specs write (#2184).
- Load shared memory through a Devin CLI session-start hook, and allow writes to the shared handoff store from linked worktrees (#2106, #2155).
- Handoff grants you add by hand stay, and `import windsurf` leaves the memory and handoff rules sync wrote out of the spec (#2176, #2186).
- A personal memory folder path with pattern characters gets no `Write` rule, and sync says so (#2171).
- Explain how Devin discovers shared agent profiles without promising that target scoping prevents it (#2084).

#### OpenCode

- Keep `permission` keys you wrote by hand next to spec keys; a spec that starts setting one of yours saves `opencode.json.bak` first (#2175).
- Allow writes to the shared handoff store from linked worktrees (#2155).
- A `deny` or `ask` on the memory folder or handoff store stays, and handoff grants you add by hand survive (#2155, #2176).
- A personal memory folder path with `*` or `?` gets no `external_directory` entry, and sync says so (#2171).

#### Ellipsis

- Add the opt-in `ellipsis` target to write `.ellipsis/code-review.yaml` from review specs and settings, and import a hand-written file (#2196).
- Write the selected Ellipsis environment from environment and MCP specs, and have the review use it; `import ellipsis` reads it back (#2188).

#### Antigravity

- Load project and checkout personal memory through an always-on include rule, and warn when it passes 24,000 bytes once expanded (#2108, #2157).
- Sync writes `SessionStart` hooks, and `hook run` accepts `auto_approve` replies and skips policy-dependent `allow` ones (#2124, #2125).

#### Cline

- Add a project memory TaskStart hook for VS Code, native Windows scripts, and explicit Unix VS Code diagnostics (#2106).
- Write global hook event scripts for the SDK and CLI under `~/.cline/hooks/`; the VS Code extension keeps its own folder (#2126).

#### Claude Code

- Claude Code loads personal memory natively; subagents read the same store, with an import fallback when native loading is off (#2116).
- On Windows, hooks find Git Bash when Git for Windows keeps `git` under `ucrt64` instead of `mingw64` (#2199).

#### Cursor

- A personal memory folder path with pattern characters gets no `Write` rule in `cli.json`, and sync says so (#2171).
- `import cursor` keeps a skill's `workspaces` when it matches the native copies; for a key 0.83.0 repeated, remove the extra line (#2202).

#### Copilot

- Send the shared memory index under the reply key VS Code agent sessions read, as well as the one Copilot CLI reads (#2101).

#### Gemini CLI

- With the `memory` built-in, Gemini CLI saves team facts to `.agnostic-ai/memory/` instead of the generated `GEMINI.md` (#2103).

#### Kiro

- Kiro loads project and checkout personal memory through a steering include, so you no longer add it by hand (#2108).

#### Augment

- Auggie CLI on Unix loads shared memory through an executable session-start script; the IDE keeps the memory rule (#2106).

#### Qoder

- Import project MCP servers from `.qoder/settings.json`, including JSONC, so fresh checkouts can recreate them (#2087).

#### Trae

- Load the shared memory index when a project session starts and project hooks are enabled (#2106).

#### Crush

- Preserve MCP channel opt-in and automatic direct and group replies through import and sync (#2086).

### Site

- Use the current Antigravity app name and explain that OpenCode V2 no longer reads the Claude instructions fallback (#2159).
- The memory docs list each tool's own memory and default, and the shared rule says a tool's own memory can stay on (#2102).
- Clarify setup, daily use, tool support, and contributor guides; include every user guide in the plain-text docs for coding agents (#2083).

## v0.83.0 - 2026-10-11

### General

- Rank startup context and compare hooks, MCP servers and permissions across tools (#2017, #1985, #1986, #1987).
- Run hooks with the project binary, use locked installs, and explain failed checks and timeouts (#2012, #2046, #2018).
- Scaffold all ten spec kinds, plan imported skill changes, and check project targets and text skill links (#1990, #2013, #2011, #2060).
- Hide credentials in previews and permissions; verify build downloads and npm releases, and update unsafe dependencies (#2021, #2044, #2022, #2016).
- Keep sync JSON valid, skip unchanged skill links, and reduce staged-check time and typo-check memory (#2079, #2080, #2020, #2055).

### By tool

#### Claude Code

- Handoff hooks use the active checkout; refresh setup in linked worktrees and safely remove clean ones (#2068, #2014, #2054).
- Preview RTK command rewrites and declared approval changes with `doctor rtk --command` (#2015).
- Explain instructions for a file with `explain --file <path> --target claude`, including scoped rules and session limits (#1988).
- Explain when a home skill takes precedence over a project skill with the same name, even when sync warnings are silenced (#2069).

#### OpenCode

- Import V2 MCP servers, disabled states and separate timeouts without losing servers added by hand (#2063).
- Run opt-in V2 tool hooks on macOS and Linux; Windows requires V1; set `x-opencode.plugin-api: v2` per hook (#2062).
- Preserve manual-only skills and native invocation metadata through sync and import; V2 enforces the policy (#2064).

#### Copilot

- Sync and import user hooks across projects; keep handwritten handlers without duplicate execution (#2065).

#### Codex

- Explain instructions for a file with `explain --file <path> --target codex`, including planned outputs and launch directory limits (#1989).

#### Amp

- Explain when a home skill takes precedence over a project skill with the same name, even when sync warnings are silenced (#2069).

#### VS Code

- Editors report source and load errors, clear fixed diagnostics, explain failed checks, and keep checks current (#1981, #1982, #1983, #1984).

#### Cline

- Keep tool names in hook overrides as literal data, including quotes and newlines, so they cannot add shell commands (#2019).

### Site

- Check binaries, generated output and skill links before starting a cached environment with the project readiness checklist (#2052).

## v0.82.0 - 2026-10-10

### General

- Interactive commands offer upgrades with Yes as the default, release notes, project migrations, and sync checks (#1952).
- Enable RTK and Caveman independently with `builtins`; both stay off by default and no programs are installed (#1964).
- Sync preserves JSON, hooks and ignore files; import avoids text loss; memory notes shrink and warnings name tools (#1889, #1923, #1943, #1934, #1935, #1937).
- Commands run faster at 500 specs, `graph --target` shows only that tool, and `doctor` lists tools in name order (#1905, #1913, #1914, #1953, #1956).
- Release builds avoid an HTTP/2 crash and omit the build machine's source paths (#1912, #1926).

### By tool

#### Claude Code

- Block supported hook failures with `failClosed: true`; sync names stop, completion, and background hooks where it has no effect (#1918, #1975).
- `import claude` preserves each hook's settings and `onFailure` values, so sync no longer adds duplicate hook groups (#1921, #1929).
- `doctor` names hook specs that cause duplicate runs; delete the named spec, then run `sync` and `import claude` (#1924, #1925).

#### Codex

- **Breaking:** Codex applies and validates `outputs.codex.config.sandbox`; set `read-only`, `workspace-write`, or `danger-full-access` (#1884).
- Sync shows how to give Codex access to repo memory when you keep a hand-written config (#1949).
- `import codex` keeps manual skill sources intact when their generated files are imported again (#1948).

#### Kiro

- `sync --global` writes CLI V3 hooks to `~/.kiro/hooks/`; `import --global kiro` adopts hook files a spec can preserve (#1976).
- Portable `on: before-tool` hooks select built-in tool IDs or one MCP server; other portable events remain unmapped (#1980).
- Test CLI V3 prompt and Stop exits, category tags, wildcards, and supported MCP selectors with `hook run` (#1974).

#### OpenCode

- OpenCode saves personal memory without asking each time with `memory.personal: repo` (#1884).
- Moving `AGNOSTIC_AI_HOME` removes the old personal memory path from OpenCode's instructions (#1888).

#### Trae

- Empty agent tool lists disable all tools, including after import and sync (#1954).
- `import trae` reads shared `.agents/skills/` folders and assets; `.trae/skills/` takes priority for matching names (#1955).

#### Windsurf / Devin CLI

- Let Devin CLI save personal memory without asking each time with `memory.personal: repo` (#1884).

#### Kilo Code

- A command named `goal` is accepted with a warning that Kilo offers it as `/goal:command` (#1917).

#### Warp

- `sync --global` writes MCP servers to `~/.warp/.mcp.json`; `import --global warp` reads them back (#1978).

#### Antigravity

- `sync --global` writes MCP servers to `~/.gemini/config/mcp_config.json`; `import --global antigravity` reads them back (#1977).

### Site

- RTK and Caveman guides separate cost, speed and style; setup, approvals and measured results retain failed checks (#1963, #1959, #1960, #1961).
- Memory and project setup guides use plain words and explain when tools load shared memory (#1933, #1951).
- Kiro docs name current tool categories and mark `keyboardShortcut` as CLI 2.x only (#1979).

## v0.81.0 - 2026-10-07

### General

- **Breaking:** `lint` and `sync` stop on a plain MCP `env` or `headers` value not marked `!literal`; run `agnostic-ai migrate --only secrets` (#1794).
- New `memory` built-in: one memory every tool loads at session start and saves to, for the team and for you (#1844, #1845, #1850, #1851, #1852).
- `memory.personal: repo` shares one personal memory across every worktree, also when `~/.agnostic-ai` is a link (#1859, #1869, #1877).
- New `memory lint`, `index`, `list`, and `path` commands check, rebuild, list, and locate memory; `doctor` flags broken links and secrets (#1847, #1877).
- Sync keeps hooks you wrote by hand when it adds its own, on Claude Code, Codex, Cursor, Gemini CLI, Qoder, and Factory (#1858).

### By tool

#### Claude Code

- **Breaking:** sync stops on an agent name that starts with `-` or contains `:`, which Claude Code would skip; rename the agent (#1870).
- With `memory`, Claude Code's own memory saves into the shared personal memory, so other tools see it (#1846).
- `lint` counts the `AGENTS.md` that `CLAUDE.md` imports in Claude Code's word total, which it used to leave out.

#### Cursor

- `.cursor/cli.json` always has both `allow` and `deny`; the Cursor CLI refused a file with only one, so its rules never loaded.
- In repo mode, the Cursor CLI can save personal memory: sync allows writes to its folder in `.cursor/cli.json`.
- Turning off the last Cursor hook removes `.cursor/hooks.json` instead of leaving `{"version": 1}` behind.

#### Copilot

- A hook with a Windows command now runs on Windows: sync writes separate `bash` and `powershell` commands (#1856).

#### Qoder

- In repo mode, Qoder saves personal memory without asking for approval each time (#1872).

#### Kilo Code

- In repo mode, Kilo finds personal memory through the rule; sync no longer lists a file Kilo ignores (#1871).

#### Augment

- `hook run` now shows that a `SessionStart` hook's output reaches the model (#1853).

#### OpenHands

- `hook run` no longer claims a `SessionStart` hook's output reaches the model; OpenHands only logs it (#1853).

### Site

- Docs rewritten in plain words and trimmed, with a new shared memory guide.

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

Releases before v0.80.0 are in [docs/CHANGELOG-archive.md](docs/CHANGELOG-archive.md).

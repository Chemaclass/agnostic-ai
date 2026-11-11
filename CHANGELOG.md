# Changelog

Changes by release, following [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Releases before v0.90.0 are in [docs/CHANGELOG-archive.md](docs/CHANGELOG-archive.md).

## [Unreleased]

### General

- `sync` no longer loses a target's rules when its `rules-file` is an instructions file another enabled target reads, such as `outputs.codex.rules-file: AGENTS.md` with Cline. It stops with AAI-102, and `validate` reports it: point the `rules-file` elsewhere, remove it, or set `outputs.<target>.file` for the other tool. Where the other tool's file already carries those rules, as with Amp, sync writes what it wrote before and `lint` warns with LINT063 (#3302).
- **Mark all as read** on the companion's Needs you header takes every chat it shows out of the group, after the search and the All, Local or Cloud choice. Rows no longer repeat their group as "New reply" or "Working", and a Codex thread opened and never typed in no longer shows as "Untitled conversation" (#3300).
- Hide in the companion's ⋯ menu is now Pause: the circle stays at 10% and reads nothing until you click it or press the shortcut (#3298).
- `outputs.<target>.rules-file` on a target that writes no merged rules file, such as Cline or Kiro, no longer stops `sync` from writing that target's instructions file and rules. `lint` warns about the key with LINT062 (#3295).
- A conversation started with `agnostic-ai conversations start` now sits indented under the one it came from, in the web interface and the companion, while both are in the same group. Otherwise it is a normal row (#3285).
- Quit the companion from its ⋯ menu or the circle's right-click menu (#3297).
- `agnostic-ai conversations start` starts a new conversation from inside a Claude Code or Codex session, in a new terminal tab in the same folder, with a brief as its first prompt. Its first line, `Continues <tool>:<id>`, gives the row a `parent` in the conversations API. The new `side-conversation` built-in adds a skill that tells the agent when to run it and how to write the brief (#3285).
- The companion and the web interface show the agent's latest reply under each chat's title: the question it asks, or the start of its last message. Local Claude Code and Codex chats show it; Cursor chats and cloud sessions do not yet. `reply` holds it in the API, and a search matches it (#3283).
- The new `wrap-up` built-in adds a skill that ends a session with what changed, what is still open, and a note to share; `handoff` now points to it (#3284).
- Delete an archived Claude Code chat for good from the companion's right-click menu, after a confirmation: its transcript, subagent transcripts, tool output, file backups and pasted images leave the disk. Other tools say why they cannot be deleted (#3275).
- A Claude Code chat that went quiet on its own under Archived can be deleted too, unless something was written in it since you last looked (#3288).
- A model tier takes an optional `description` saying when to use it. `sync` and `sync --global` list the described tiers in each tool's instructions file (#3286).
- The companion and the web interface show a `cursor-agent` chat that still runs as open. Opening it switches to its terminal instead of starting a second copy (#3274).
- The web interface no longer offers Copy command on a session that still runs; running it would start a second copy (#3272).
- `lint` no longer raises LINT005 for a `matcher` on a `FileChanged` hook, which names the files to watch (#3262).
- The companion's row button and "Archive all" now read "Mark complete" and "Mark all complete"; marking one chat complete turns its tick green for a moment.
- When the companion cannot open a chat, the reason stays until you dismiss it, and a chat that could not be resumed offers **Copy resume command**. Opening a Claude Code or Codex chat that still runs in a terminal no longer replies with a resume command (#3256).
- Mark a chat as read from the companion's right-click menu, without opening it; the next reply brings it back (#3250).
- The companion tells same-named folders apart, such as `shop/api` and `billing/api`, and shows the full path when you hover a row (#3254).
- A local Codex chat that went silent shows "No activity for 7m" instead of "Waiting for you" in the companion and the web interface; `quietSince` marks such a row in the API (#3251).
- In the web interface's Built-ins tab, Details now says what each built-in does when on and links RTK and Caveman to their official pages (#3261).
- `upgrade` also updates an installed companion and restarts it if it was running (#3247).
- Right-click the companion's circle to stop its animation or dim it when nothing needs you (#3237, #3246).
- `memory import --skip <file>` and `--skip-rest` decline facts you do not want, and LINT043 stops counting them (#3235).
- Rename a chat from the companion's right-click menu; the web interface shows the new name (#3236).
- Renaming a running chat also names its Terminal or Ghostty tab.
- An archived or hidden chat that starts working again moves back to Working (#3239).
- On macOS, `companion install` makes an app with the agnostic-ai icon that still starts at login (#3238).
- Cursor chat titles show what you typed, without the tags Cursor adds (#3241).
- Clicking a running session opens its Terminal or Ghostty tab; an ended session resumes in a new tab (#3242).
- Clicking a running session in the companion leaves its terminal ready for typing.
- Cursor and Windsurf load each always-on rule once, from `AGENTS.md`; Trae opts in with `outputs.trae.rules-mode: entry-point` (#3243).
- LINT061 warns when a target ignores its `outputs.<target>.rules-mode` value (#3243).
- Builds use Go 1.27.4 to fix the `net/http` vulnerability [GO-2026-6859](https://pkg.go.dev/vuln/GO-2026-6859).

### By tool

#### Augment

- The target guide calls Auggie 1.x stable, from 1.2.0, instead of a prerelease. The 0.x and 1.x differences it lists are unchanged (#3310).
- Settings `permissions.ask` writes `ask-user` rules to `.augment/settings.json`, between deny and allow rules; `import augment` reads them back (#3266).
- Settings `model` and `effort` write Auggie 1.x's `defaultModel` and `defaultThinkingLevel` in project and global settings; Auggie 0.x ignores both keys. `import augment` reads them back (#3265).
- Hooks with `args` reach `.augment/settings.json` and `~/.augment/settings.json` with their arguments, instead of being skipped. An inline command no longer raises a "never executes" note, and `hook run` runs it (#3264).

### Site

- The Qwen page says a `FileChanged` hook can also replace or clear the paths registered through `watchPaths` (#3263).
- `serve`, the local API, `companion`, `doctor`, `import`, and the `sync` and `gitignore` settings have their own reference pages; old links redirect.
- The companion page says Windows and Linux are not supported and links the issues.

## v0.93.0 - 2026-11-10

### Highlights

- The desktop companion shows chats that need you and opens them where they live (#3213).
- Conversations can list Claude Code and Codex cloud sessions and search your full local history (#3213).
- Mistral Vibe sync supports all seven native hook events from Vibe 3.1.1 (#3219).

### General

- `agnostic-ai companion` adds a floating chat list with unread counts, archive actions and a shortcut (#3213).
- `companion stop` ends the companion, `companion uninstall` removes it, and `companion install` restarts one that runs (#3223).
- `serve --global --cloud-sessions` lists Claude Code and Codex cloud chats using your existing sign-ins (#3213).
- Conversations searches all local chats, shows new replies and archives, and refreshes faster (#3213).
- Automatic upgrades use the new CLI to update `requires` and report stopped steps, starting with this release (#3196, #3198).
- A new grid logo appears across the site, web interface, playground, READMEs and editor extensions (#3212).

### By tool

#### Claude Code

- Handoff hooks call generated scripts, keeping completion messages short after compaction (#3205).
- `lint` and `explain --context` exclude manual-only skill descriptions from Claude's startup word count (#3199).

#### Mistral Vibe

- Project and global sync support all seven Vibe 3.1.1 hook events and reject strict mode where Vibe does not support it (#3217, #3219).

#### Windsurf / Devin Desktop

- MCP sync keeps `disabledTools` on every server and `oauthScopes` on remote servers (#3210).

#### Grok

- An ignored-rule error names the ignore file and pattern and explains how to remove Grok from the targets (#3198).

### Site

- Guides use shorter explanations, current examples and model aliases, and name the `ui` command (#3200, #3195).
- The Amp guides explain that setup repeats on snapshot refresh and has a shared 20-minute limit (#3208, #3209).
- The Windsurf guide separates Devin Local's lazy rule discovery from legacy Cascade (#3219).

## v0.92.0 - 2026-11-08

### Highlights

- **Breaking:** Sync stops on an MCP `roots` item that is not a `{uri, name}` mapping, and Pi MCP `timeout` is now milliseconds (#3016, #3002).
- The web interface takes the Mineral look, with a rail that names each page over its live counts and a phone top bar (#3120, #3121).
- The web interface opens on **Conversations**, with recent Claude Code, Codex and Cursor chats that reopen where they live (#3109).
- **Setup health** lists gaps with fix prompts, rechecks everything, and copies one prompt for ticked findings (#3070, #3071, #3127).
- `sync --dry-run`, `--plan` and `--check` now match what a real sync does, with `--diff` support for project and global sync (#2991, #2996, #3000).

### General

#### Sync and configuration

- **Breaking:** Sync stops on an MCP `roots` item without a `uri` mapping; `lint` (LINT060) flags it. Write `- uri: file:///path` (#3016).
- `sync --dry-run` and `--plan` accept `--diff`, also with `--global`, and `sync --global --plan --check` fails on drift (#2991, #2996, #2986).
- Sync previews name the files `--allow-empty-targets` removes and show a settings file that keeps your keys as an update (#2995, #3000, #3142).
- Sync notes when a target drops an agent's `hooks` or `permissionMode`, and `sync --global` notes dropped fields such as `maxTurns` (#3052, #3073).
- Built-in memory hooks write a shorter command, and `agnostic-ai project` finds the project and its pinned version (#3173).

#### Checks and reports

- `lint` warns (LINT059) when an MCP `timeout` under 1000 reaches Crush, Pi or Junie, because it most likely holds seconds (#3008).
- `graph` and `hook run` accept `--json`, the same as `--format json`; `--json` with a different `--format` fails (#2993).
- `hook run` rejects a `--payload` file that is not a JSON object, and `--format json` still prints its result when a target cannot be set up (#2994).
- `doctor` describes shared settings updates as removing unchanged managed keys and keeping yours, with JSON action `stale` (#3009).
- Previews withhold prose that labels a credential with a value, such as a `password:` line or a `token` table cell (#3062).

#### Commands and install

- `agnostic-ai --help` ends with "Made by Chemaclass", `--version` only in a terminal; the web tooltip and console greeting say "by Chemaclass" (#3185, #3186).
- Check, warning and cross marks print no color codes when `TERM=dumb` (#3190).
- The install scripts retry the latest release lookup twice when GitHub times out or answers 429 or a server error (#3188).

#### Web interface

- **Removed:** Context drops **Bytes**, **Show the largest source**, **Copy all prompts** and **Next** steps; Conversations drops **Not found** (#3180, #3181).
- The web interface takes the Mineral look: bone paper, a slate rail, Fraunces headings and ink pill buttons, also in search and the inspector (#3120, #3132).
- The rail names each page over its live counts, lists **Setup health** first, and says "Update needed" only for a newer required version (#3120, #3034, #3167).
- Phones get a top bar with the finding count, Search and **Menu** over one row of page links; search and the inspector open full screen (#3120).
- Each page opens with a short verdict and a facts line ending in **Tool support** and **Docs**; findings are cards (#3120, #3121, #3143, #3177).

#### Web interface details

- Labels use plain words, such as **Preferences**; Project or Global and Local replace Shared and Personal, and API values stay the same (#3033, #3123).
- Dropdowns open as a panel in every browser, and row actions such as **Open** show on hover or focus, behind a ⋯ menu on phones (#3168, #3170, #3177).
- **Open global settings** and **Open project** sit at the foot of the rail, with links to the agnostic-ai and project repositories (#3035, #3111).
- Following a link to an entry you already opened scrolls to it and focuses it again (#3076, #3080).
- `/api/health` adds `version`, `requires`, `requiresMet`; the overview adds `repository`, `health.gaps` and `health.improvements` (#2974, #3034, #3070, #3111).

#### Conversations

- The web interface opens on **Conversations**: recent Claude Code, Codex and Cursor chats, open ones first, each reopened where it lives (#3109, #3120, #3121).
- The list shows at once when you return and refreshes in the background, three times faster (#3166).
- Rows are calmer: one meta line, a one-line last prompt, a status chip only while the agent works or waits, and **Open in Terminal** on hover (#3163, #3181).
- It lists Codex and Cursor chats on Windows, handles a symlinked `~/.codex`, and withholds a prompt with `curl -u user:password` (#3119, #3133).
- Opening a Codex thread that a running `codex` holds switches to its terminal; an editor chat brings forward its workspace window (#3134, #3147).

#### Setup health

- **Setup health** opens with one sentence; errors list under **Fix first** and the rest under **Worth a look** (#3120, #3183).
- **Recheck** runs the checks for every section and says how many findings remain (#3071, #3086).
- It shows **7 of 9 improvements applied** and lists unused features your tools support under **You could add** (#3079, #3070, #3087, #3088).
- Tick findings and suggestions, then **Copy prompt** at the foot copies one prompt; **Copy all fixes** shows for two or more (#3127, #3032, #3160, #3183).
- In a project it includes `lint --global` findings labelled **global**; `view --json` marks them `"layer": "global"` (#3069, #3081).

#### Context and Lifecycle

- **Context** says how many words agents read at session start, with a budget bar and tables of what loads at start and on demand (#3120, #3121, #3128, #3180).
- **Show content** on a Context row shows the file's frontmatter as chips, and Context size no longer scrolls sideways on a phone (#3025, #3059, #3063, #3110).
- **Lifecycle** shows a timeline of stages that have hooks, with **Show all stages** for the rest, and a turned-off hook as off (#3120, #3145, #3154, #3179).
- Each hook says what it does and where it comes from; a built-in shows only its agnostic-ai call (#3038, #3062, #3065, #3172, #3179).

#### Tools, Settings and Authority

- **Authority** shows every allow, ask and block rule in one table, blocked first, with Edit and Remove on each row (#3120, #3151, #3182).
- **Tools** shows each server found in your tools as one row with a button that makes it a spec, and turned-off servers as off (#3120, #3146, #3148, #3178).
- **Settings** shows each built-in as a name, one line and a switch, and previews changes as a diff card (#3120, #3121, #3156, #3178).

#### Editing in the web interface

- **Add hook** on Lifecycle previews and writes a new hook spec, then asks you to run `agnostic-ai sync` (#3037, #3061).
- **Copy to global** (**Copy to project** in global view) moves a spec across levels; **Replace it** overwrites (#3027, #3090).
- **View and edit** (Project or Local) shows once, beside the filter, on **Authority** and **Settings** (#3121, #3162).
- Authority edits protected paths and, globally, the default permission mode; **Reset to project paths** clears a local `protected: null` (#3039, #3072, #3084).

### By tool

#### Kiro

- `import kiro` keeps an agent's inline `mcpServers`: ones in `.kiro/settings/mcp.json` become spec names, others move to `x-kiro.mcpServers` (#3051, #3075).
- `import kiro` replaces literal credentials with `${NAME}` references and leaves out a server whose `oauth` holds a credential, with a note (#3067, #3074).
- Agent files write an MCP server's `x-kiro.timeout` and `x-kiro.requestTimeout`; a portable `timeout` is left out with a note naming both keys (#3041).
- `validate` accepts the `PreCompact` and `PostCompact` hook triggers from Kiro CLI 3.1.0 (#3136).

#### Qwen Code

- Skills keep `disable-model-invocation`, `user-invocable` and `paths`, so a manual-only skill stays out of the model's reach; import reads them back (#3042).
- Agents keep `background` and `hooks`, and a shared `Stop` hook becomes `SubagentStop`; agent hooks used to be dropped with no note (#3043).
- Hooks with `event: FileChanged` reach `.qwen/settings.json` and run when a watched file such as `.envrc` changes, on Qwen Code 0.27.0 (#3189, #3193).

#### Codex

- **Deprecated:** MCP `experimental_environment`, which Codex ignores; sync and import write `environment_id`. Run `agnostic-ai migrate --only mcps` (#2976).
- MCP servers no longer get `description` or `roots` in `.codex/config.toml` or `~/.codex/config.toml`, which Codex ignored; sync and import note it (#2999).

#### Pi

- **Breaking:** MCP `timeout` is milliseconds for Pi; sync and import convert it. Multiply a `timeout` in seconds by 1000, so `1800` becomes `1800000` (#3002).

#### Crush

- MCP `timeout` reaches `crush.json` in seconds, and import converts it to milliseconds (#2997). Multiply a `timeout` held in seconds by 1000.

#### Junie

- `lint` (LINT059) and sync warn when an MCP `timeout` or `x-junie.timeout` above 3600000 reaches Junie, which ignores it and uses 60 seconds (#3014).

#### CodeBuddy Code

- An agent with `x-codebuddy.tools: []` gets `tools: []`, read as no tools by CodeBuddy 2.179.0 and later; sync used to write `tools: ""`, every tool (#2983).

#### Kimi Code CLI

- Sync and import keep a stdio MCP server's `x-kimi.environment_id`, which Kimi Code 2.5.0 reads in place of `runtime_id` (#2980).

#### Grok

- Project and global import keep MCP servers with an ignored `description`; global sync preserves the native file (#3006).

#### Ellipsis

- Sync and import accept `compute.idle` in an environment; it used to stop sync from writing the environment file (#2998).

#### Copilot

- The sync note for an agent's `hooks` names `x-copilot.hooks`, which Copilot CLI reads (#3137).

### Site

- Target pages note Qwen Code hook folder trust, Qoder IDE rule order, and the Trae Build step for clients before October 22, 2026 (#2978, #3015, #2984, #3044).

## v0.91.0 - 2026-11-01

### Highlights

- **Breaking:** An empty `targets` list keeps generated files. Run `agnostic-ai sync --allow-empty-targets` to remove them (#2963).
- Context size is the first tab: it shows what each tool loads at startup and on demand, with a fix and a copyable agent prompt per source (#2924).
- Settings turns coding tools on and off per project and picks the editor that file links open (#2930, #2933).
- Authority edits and removes permission rules with a preview; Tools saves a native-only MCP server as a spec (#2922, #2634).
- Sync never overwrites unreadable files, keeps `.gitignore` and Claude settings entries, and fixes remote MCP servers (#2887, #2884, #2877, #2886).

### General

#### Sync and configuration

- **Breaking:** An empty `targets` list no longer removes generated files. Run `agnostic-ai sync --allow-empty-targets` to remove them (#2963).
- `sync --watch` without `--target`, `--only` or `--except` follows `targets`, and an empty list keeps files from the first pass (#2932, #2963).
- A repeated key in a nested map, such as `permissions.deny` or an MCP `env` entry, is a parse error with its line, not a silently dropped field (#2890).
- An empty local key or model tier keeps the shared value, `{}` clears a group but not a tier, and lint names what an empty key replaces (#2891).
- `init` sets minimums; `upgrade --requires` keeps minimum/exact pins or makes ranges minimums; `init`, `use` and AAI-005 explain exact pins (#2841, #2849).

#### Checks and reports

- `explain <spec>` and `graph` skip tools outside `targets` that cannot render the project, so an unselected Grok no longer fails them (#2934).
- `compare` leaves out specs neither tool reads and points a built-in spec at `builtins` in `agnostic-ai.yaml`, not a cache file (#2938).
- Lint, doctor and sync warn (LINT057), naming the file that holds it, when a `sync.unmanaged` entry uses `**`, which acts like `*` (#2937, #2956).
- A `sync.unmanaged` glob that ends in `/`, such as `.claude/skills/legacy-*/`, now keeps sync out of every folder it matches (#2957).

#### Specs, import and editors

- The unknown capability error no longer shows a stray colon after the list name, as in `for permissions.allow` and `for can` (#2922).
- Markdown specs keep an opening `---` block of prose or a list as body text, and `lint` reports it as LINT056 (#2892).
- A kept opening block of only headings stays in the body; generated files write an empty frontmatter block before it, with the header below (#2892).
- The language server answers malformed JSON with a parse error and keeps running, and rejects message sizes above 64 MiB instead of crashing (#2889, #2908).
- Import keeps an imported `effort` when the only other settings spec that sets one targets a different tool (#2796).

#### Safe writes

- Sync writes `.gitignore`, `.git/info/exclude` and `.worktreeinclude` whole, keeps `.gitignore` entries, and asks for a cut block's end marker (#2884, #2875).
- Sync stops with an error instead of overwriting a JSON settings file it cannot read, or when it cannot record written files in its state file (#2887, #2883).
- Git queries use the requested repository when sync runs from a Git hook (#2882).
- Failed pack fetches keep installed content, and partial updates record each successful commit (#2880).
- Claude settings keep user entries in configured objects and warn before replacing an entry (#2877).

#### MCP and hooks

- MCP tool listing stops at its timeout when a detached child keeps stderr open (#2888).
- Hook commands keep home, variable and absolute paths such as `~/.claude/hooks/x.sh`; only project hook paths move to the target tool's folder (#2885).
- MCP `type: local`, `streamable-http` and `remote` now write a working server instead of an empty one, and `lint` reports any other unknown `type` (#2886).
- Hook runs report Windows timeouts correctly, so Copilot lets the action proceed (#2878).

#### Global settings and memory

- Memory indexing keeps nested and external links and avoids duplicate entries for filenames with spaces (#2881).
- Global voice settings round-trip native keys; Qwen, Gemini, Grok and Vibe join support, with notes for unsupported fields (#2833, #2834).
- Global instruction updates keep private modes and stop if the file changes during sync (#2857).
- `explain --global` lists native hook files and copied scripts for Cline, Grok and Kiro, including custom tool roots (#2863).
- **Open global settings** offers to create the global folder, with a short explanation and an optional `git init` (#2953).

#### Security

- Global sync ignores relative platform roots and excludes relative paths from ownership roots (#2879).
- Concurrent updates keep all private Git exclusion entries (#2876).
- Import rejects writes outside the project and configured source directories; invalid Codex and Warp agent names use their file names (#2873).
- `packs add` refuses packs folders that link outside the project (#2874).
- Editor and vendor dependencies receive security patches; the scoped audit exception expires on November 14 (#2844).

#### Context size

- Context opens on Context size, now its first tab, for a project with a coding tool, with a startup check against `lint.instructions-words` (#2924, #2929).
- Context size explains each source with what to change and a copyable agent prompt, and keeps the tool and file in the page address (#2924).
- Context size keeps its report in place with placeholders while a tool loads, groups tool and file controls, and highlights load totals (#2894, #2872).
- On wide screens, Context size puts the Startup and On demand figures beside the startup check instead of below it (#2955).

#### Settings

- Settings has a Coding tools tab: a switch per tool saves shared or personal targets for the next sync, with a docs link and a found in project badge (#2930).
- Settings has an Editor tab: Cursor, VS Code, VS Code Insiders, VSCodium, Zed, or a custom link with `{path}` and `{line}`; the header drops it (#2933).
- File links show Open and explain how to recover when no editor opens.
- Global Settings edits shared or personal voice values; project settings stay read-only (#2835).
- Config files shows merged values once below the source links.

#### Tools and previews

- Tools previews and saves a native-only MCP server as a shared or personal spec for the next sync (#2634).
- Built-in previews group notes by tool, fold long lists, and keep Apply and Cancel at the top.
- Authority, Settings and native server imports keep a compact Shared or Personal choice, and Settings shows the selected values.

#### Authority

- Authority lists each saved permission rule with Edit, which opens the editor on that line, and Remove, which shows a preview before saving (#2922).
- The permission rules box is four lines tall, shows example rules, and puts a readable message under it when a rule is rejected (#2922).
- Authority previews and saves shared or personal permission rules, compares personal overrides with Shared, and leaves applying them to sync.

#### Page layout

- The header shows the project name, its folder path and a Project label, as for Global, even when the project is named agnostic-ai (#2923).
- Section warnings show once, as a count under the page title with Review, instead of a banner on every tab; errors stay above the tabs.
- Section headings link to the matching docs in a new tab, even when nothing is configured; the Lifecycle Docs link opens the session timeline section.
- All five pages fit narrow screens: phone headers keep the configuration control on its own row and Settings tabs fit their labels and counts.
- Select controls use a shared, theme-aware chevron with room for labels and keyboard navigation (#2894).

### By tool

#### Antigravity

- MCP import accepts Windows project links that already work in previews (#2850).
- Import previews follow project links and Windows short paths, rejecting outside MCP files even through source aliases (#2831).
- Plugin MCP files get manifests; `mcp.json` gets a note naming the documented `mcp_config.json` path (#2826, #2967).
- Import reads the configured MCP file, including absolute in-project paths, so plugin servers round-trip (#2829).
- Sync keeps existing plugin manifests it did not write and prints a note.

#### Claude Code

- Home voice settings enable dictation, select hold or tap, and restore previous values when removed.
- `import claude` no longer warns that `CLAUDE.md` differs right after a clean sync when `AGNOSTIC_AI.md` has a `::target claude` block (#2935).
- `sync --check` and `status` stop reporting a Claude-only project's imported `AGENTS.md` as a leftover, and `doctor --fix` no longer deletes it (#2959).
- `import claude` names the edited file (`AGENTS.md`, `CLAUDE.md` or both) in its warning and says "seeded from AGENTS.md and CLAUDE.md" for a pair (#2958).
- `revert` restores and `cleanup` removes `AGENTS.md.bak` in a Claude-only project whose `CLAUDE.md` imports `AGENTS.md` (#2965).

#### Kiro

- Global sync and import use `~/.kiro` when `KIRO_HOME` is relative, a filesystem root, the home directory, or a home ancestor (#2860).
- Global sync sets voice language in `settings/cli.json`, keeps other settings, and restores the prior language on removal (#2837).
- Sync writes a settings spec `model` to `chat.defaultModel` in `.kiro/settings/cli.json` and keeps the file's other keys (#2960).

#### Crush

- Sync and import preserve all 13 native hook events and reject matchers on events without tools (#2807).
- Import keeps personal lifecycle commands out of shared specs and preserves handlers with distinct native names or timeouts (#2807, #2827).

#### Zed

- Global instructions use the native macOS and Flatpak roots, moving recorded blocks without changing personal text (#2866).
- Windows global instructions use `%APPDATA%\Zed\AGENTS.md`, keeping personal text while retiring recorded old output (#2857).

#### Goose

- Detection no longer treats Antigravity plugins in `.agents/plugins/` as a Goose project (#2830).
- Sync keeps existing plugin manifests it did not write and prints a note.

#### Qwen Code

- Import reads `.agents/skills/` as well as `.qwen/skills/`, and sync keeps agents-as-skills off that shared folder (#2931).
- Agents written as skills by Amp, Crush, Warp or Zed stop when Qwen is on; set `outputs.<tool>.skills-dir` to a separate folder to keep them (#2931).

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

#### Cline

- Import keeps native agent settings through sync (#2967).

#### OpenHands

- Import keeps native agent skills through sync (#2967).

#### Cursor

- `explain --context`, the Context size view and LINT011 count the root `AGENTS.md` that Cursor reads when another tool writes it (#2936).

### Site

- Tool docs update retirement dates, Codex history values, Antigravity plugin MCP names, and Qoder and Zed skill paths (#2810, #2811, #2812, #2814, #2818).
- Docs cover hook session flow and target guidance for Continue, Claude, and OpenHands (#2858, #2815, #2813, #2967).
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

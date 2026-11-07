# Changelog

Changes by release, following [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Releases before v0.90.0 are in [docs/CHANGELOG-archive.md](docs/CHANGELOG-archive.md).

## [Unreleased]

### General

- The web interface takes the Mineral look: bone paper, a slate rail that asks each section's question over a live summary, Fraunces headings (replacing Archivo Narrow), ink pill buttons and verdigris links. Phones get a top bar with the finding count, Search and **Menu** over one row of page links. The search dialog and the JSON inspector take the same look, as a surface panel with a soft ring and a full-screen sheet on phones (#3120).

- In the web interface, each section page opens with a verdict sentence, such as "Your rules allow 11 actions, ask before 3, and block 4.", and one facts line with the finding count and any tool that ignores what is configured. Then one filter row with **Docs**, the tabs and the content; coding tool support folds closed below it with a summary such as "3 of 3 tools support rules, agents, and skills". Findings are cards with **Copy prompt** and **Details**, an empty type says what it is and which tools support it, and counts show only above zero. **Conversations** opens with a sentence that says how many chats are open and how many wait for you, or when the last one ran; each row names its tool beside the logo, and on phones its open button spans the row (#3120, #3121).

- In the web interface, **Setup health** opens with one sentence, such as "1 thing needs a fix, and 3 more are worth a look.", a facts line that says whether the data was checked just now or comes from a saved snapshot, and a row of bars for the features your tools support, filled for those in use. Findings sit under **Needs a fix** and **Worth a look**, suggestions are plain rows that name tools by name, and **Recheck all** reports in a notice. On every page, recheck results say "configuration check" and "setup check" instead of "lint" and "doctor" (#3120).

- In the web interface, each page shows one **Docs** link, beside the filter, for the open tab; each coding tool name links to its docs. Finding cards put **Copy prompt** in a column on the right on wide screens and name the section's question beside the severity. The rail lists **Setup health** first, as "Needs attention" or "All clear" over "1 to fix, 12 you could add", and **Copy all fixes** sits beside the first findings heading (#3120).

- The Lifecycle page shows its stages as a timeline with a filled dot for each stage that runs a hook, names each tool next to its logo, and shows a count only above zero instead of "Hooks 0" and "No hooks" on every stage. With no hooks it uses the shared empty state with the tools that support them. A turned-off hook shows as off, and its stage counts it as "turned off". The Tools page says what your agents can use, counts only MCP servers that are turned on, notes how many are turned off, and shows each turned-off server as off. Saving a hook or a server as a spec reports through the same notice as the other pages (#3120, #3145).

- In the web interface, **Context size** shows its totals in the Mineral style, with a share bar beside each percentage. Source types use plain names (instructions file, tool wrapper text, skill name and description) under a **Type** column, **Reread specs** is now **Read again**, and copying or moving a file to the other level reports success or failure in one notice with **Dismiss** (#3120).

- In the web interface, **Settings** shows preferences and coding tools as plain rows, and a built-in or model change preview as a card whose diff marks added and removed lines. The permission and protected path editors use the same preview and diff. Actions use one label each: **Preview**, **Apply**, **Cancel** and **Reload**, and saved, failed and overridden states show as one notice with **Dismiss**. Switches and edit buttons stay off until the page has the saved values, and the editors wait for the refreshed rules after **Apply** or **Reload**, so a quick second click no longer sends a stale change or fails as changed after editing began (#3120, #3121).

- In the web interface, **Setup health** has a checkbox on each finding and suggestion. A bar at the foot of the page counts the ticked items and copies one prompt for all of them with **Copy prompt**, errors first, then suggestions, with the shared guidance once (#3127).

- `agnostic-ai view --json` gives each MCP server item its `disabled` state in `fields`, the way hooks carry it (#3145).

- The web interface opens on **Conversations**: recent Claude Code, Codex and Cursor chats, open ones first, each reopened where it lives (#3109).

- In the web interface, **Context size** starts with the startup and on-demand word counts, then one line on the startup budget with **Change budget**, then three **Next** steps. The ranked sources are always shown and replace **Start here**; duplicated passages show only when there are any; the load explanation and the model line moved into **How this is counted**; coding tools show by name (#3121).

- In the web interface, a snapshot or the mock fixture says once, in the notice at the top of each page, that it is read-only, instead of a sentence on every finding and path, and a snapshot that does not load shows **Could not load the snapshot** with the raw error under **Details** and **Retry**.

- In the web interface, labels use plain words: Settings asks "How are your tools set up?", its kinds are **Preferences** and **Config files** as their tabs say, on Tools the configured servers tab reads **MCP servers** and the native servers heading reads **Found in tools**, each matching its tab or heading, and Lifecycle stage descriptions say "A hook here can block the prompt" instead of naming exit codes. `view --json` carries the new labels and descriptions; ids and keys are unchanged.

- The web interface links to the agnostic-ai repository at the foot of the sidebar, and to the project's own GitHub repository in the top bar when the project's `origin` remote is on GitHub; `/api/views/overview` adds `repository` with `url` and `name` (#3111).

- In the web interface, **Context size** no longer scrolls sideways on a phone while its report loads.

- In the web interface, each **Reach the best setup** row on **Setup health** is shorter: what is missing, the first three tools that support it, and **Copy prompt**.

- In the web interface, Lifecycle numbers its stages in monospace beside square markers on a ruled line, filled for stages with hooks, and the Lifecycle, Settings and change preview disclosures draw their own chevron (#3026).

- In the web interface, **Setup health** lists what the enabled tools support and the project does not use yet (a spec kind, a hook stage, the `handoff` built-in), each with a fix prompt; `/api/views/overview` adds `health.gaps` (#3070).

- **Setup health** in the web interface has **Recheck all**, which runs the checks once for every section and says how many findings remain (#3071).

- In a project, the web interface's finding counts and **Setup health** include the global setup's `lint --global` findings, labelled **global**; `view --json` marks them `"layer": "global"` (#3069).

- In the web interface, following a link to an entry you already opened scrolls to it and focuses it again (#3076).

- `sync --global` notes agent fields a tool's agent file cannot hold, such as `maxTurns` or `background`, once each, as project sync does, and an agent's `mcpServers` on Kiro and OpenHands, whose user-level agent files get no inline servers. It used to drop them with no note (#3073).

- In the web interface, each hook on Lifecycle has **Summary**, **Key fields** (event, match, type, command, timeout, disabled) and **Full spec**, which shows every field of the hook file, or hides the file when it looks like it holds a credential; search matches a hook's command (#3038).
- The Authority page shows when a local settings file sets `protected: null`, which turns the project's protected paths off for you, and **Reset to project paths** removes it so sync protects them again (#3072).

- Dry-run, render, and other previews of Markdown and prose withhold text that labels a credential with a value (a `password:` line, a `token` table cell or tag, "the password is ..."). The web interface's file content view also withholds a file with such a label, besides any file that names a strong credential word, as before, and a Lifecycle hook's key fields redact the text that would hide its full spec (#3062).

- In the web interface, **Setup health** says how many improvements the enabled tools support are in place, such as **7 of 9 improvements applied**; `/api/views/overview` adds `health.improvements` with `applied` and `total` (#3079).

- In the web interface, **Show content** on a Context row shows the file's frontmatter as chips above its text (#3025).

- The web interface sets body text in Atkinson Hyperlegible Next at 14px and page headings in Archivo Narrow, both bundled in the binary, on a warmer palette with smaller corners (#3026).

- Sync prints a note when a target drops an agent's `hooks` or `permissionMode`, as it does for `maxTurns` and `background`. Gemini CLI, Cursor, Codex, OpenHands, and other targets without these keys used to drop them with no note (#3052).

- In the web interface, item lists become ruled rows with a colored left edge for findings, errors and linked rows instead of boxed cards and tinted rows; badges and chips get square corners, notices a colored left edge, and disclosures their own chevron (#3026).

- In the web interface, each page opens with its section number, a larger condensed title over a ruled line, and monospace counts; tabs and the page search follow the same style (#3026).

- In the web interface, **Copy to global** on a rule, agent, skill, hook, or MCP spec (**Copy to project** in the global view) copies or moves it to the other level, a skill with its whole folder, and names the sync commands to run. It replaces an existing spec only when you tick **Replace it**. `POST /api/edit/spec/move` does the same (#3027).

- In the web interface, tables get a plain monospace head over a ruled line and right-aligned monospace numbers; Context size shows its startup total as a large figure and each source's share as a thin bar (#3026).

- In the web interface, form fields, segmented choices and switches on Settings and Authority follow the new style: plain fields with a rust focus border, an ink-filled selected choice, rust switches, and dotted leaders between a setting and its value (#3026).

- In the web interface, the top bar becomes a dark band in both themes with a rust line under it that moves while the page reloads, and the sidebar numbers its sections 01 to 05 (#3026).

- In the web interface, **Add hook** on the Lifecycle page previews and writes a new hook spec, then asks you to run `agnostic-ai sync` (#3037).

- The web interface counts findings by severity in the top bar and lists them on one **Setup health** page with **Copy all fixes** (#3032).

- The web interface edits protected paths from Authority in project or local scope, and the default permission mode in the global view (#3039).

- The sidebar shows the config's `requires` under the version, in the warning color when unmet; `/api/health` adds `requires` and `requiresMet` (#3034).

- **Open global settings** and **Open project** move to the foot of the sidebar, and the top bar shows the project path beside its name (#3035).

- The JSON button moves to **Overview JSON** at the foot of the sidebar, and the inspector names `agnostic-ai view --json` (#3036).

- The web interface says Project or Global and Local instead of Shared and Personal; API audience values stay `shared` and `personal` (#3033).

- `doctor` describes shared settings updates as removing unchanged managed keys and keeping your keys, with JSON action `stale` (#3009).

- **Breaking:** sync stops on an MCP `roots` item that is not a `{uri, name}` mapping with a `uri`, such as a plain string; `lint` (LINT060) and `validate` report it and name the server. Every tool used to drop it with no note. Write `- uri: file:///path` (#3016).

- `hook run` rejects a `--payload` file that does not hold a JSON object instead of passing with no hook run, and `--format json` still prints its result object when a target cannot be set up (#2994).

- `sync --dry-run` (plain, `--diff`, or `--json`), `sync --plan`, and `sync --check` show a shared settings file that keeps your own keys as an update, not a removal, or as kept when sync leaves it as is, matching what sync does (#3000).

- `sync --allow-empty-targets --dry-run` names each generated file the real run removes, with or without `--diff` or `--json`. It used to print no removals (#2995).

- `sync --plan --diff` prints the plan, then a diff of each changed file, for project and `--global` sync. It used to ignore `--diff` or reject it (#2996).

- `sync --global --plan --check` previews user-level changes and fails on drift, matching project sync (#2986).

- `graph` and `hook run` accept `--json`, the same as `--format json`, like other commands that print JSON. `--json` with a different `--format` fails on all three; `why` used to print JSON (#2993).

- `sync --dry-run --diff` prints a diff of each file sync would change and names generated files it would remove, for project and `--global` sync. Project sync used to ignore `--diff` here, and global sync rejected it (#2991).

- In the web interface, **Copy all advice** on the Context size tab copies one prompt with the advice for every source (#3030).

- In the web interface, the Context tab row no longer scrolls the page sideways on a narrow phone screen when items have findings (#3059).

- In the web interface, **Coding tool support** moves from the bottom of each page to just under the search field, above the tabs. It folds, starts folded on a phone, and remembers your choice per page (#3028).

- In the web interface, each kind in **Coding tool support** links to the tab that holds it and its heading, and hovering a tab lists the kinds it holds, such as review instructions and ignore files under Guidance (#3029).

- In the web interface, the note after **Open** shows on its own line and no longer moves the Open and Copy path buttons or the other labels on that row (#3024).

- The web interface shows the agnostic-ai version at the foot of the sidebar, linked to its release notes; `/api/health` now returns `version` (#2974).

- `lint` warns (LINT059) when an MCP `timeout` under 1000 reaches Crush, Pi or Junie; it most likely holds seconds.

- **Conversations** lists Codex and Cursor chats on Windows, marks a Codex thread open when `~/.codex` or `CODEX_HOME` is a symlink, marks a Cursor chat open when its folder name has a character such as `@`, rereads a cursor-agent chat only when it changes, and withholds a prompt that passes `curl -u user:password` or `--user` (#3119).

- In **Conversations**, opening a Codex CLI thread that a running `codex` holds switches to its terminal instead of resuming it a second time, and opening a chat from an editor brings forward the window on its enclosing workspace instead of opening a new window on its subfolder (#3134).

- On Windows, `sync -t <tool> --allow-empty-targets --dry-run` no longer names that tool's own files and skill links as removals, and `sync --check` with an empty targets list passes once every generated file is gone (#3142).

### By tool

#### Codex

- **Deprecated:** MCP `experimental_environment`, which Codex ignores; sync and import now write `environment_id`. Run `agnostic-ai migrate --only mcps` (#2976).
- MCP servers no longer get `description` or `roots` in `.codex/config.toml` or `~/.codex/config.toml`. Codex has neither key and ignored both. Sync and import say so in a note, and import leaves both out of the spec (#2999).

#### Crush

- MCP `timeout` reaches `crush.json` in seconds, and import converts it to milliseconds (#2997). Multiply a `timeout` held in seconds by 1000.

#### Ellipsis

- Sync and import accept `compute.idle` in an environment; it used to stop sync from writing the environment file (#2998).

#### Grok

- Project and global import keep MCP servers with an ignored `description`; global sync preserves the native file (#3006).

#### Junie

- `lint` (LINT059) and sync warn when an MCP `timeout` or `x-junie.timeout` above 3600000 (one hour) reaches Junie. Junie ignores such a value and uses 60 seconds (#3014).

#### Kiro

- `validate` accepts the `PreCompact` and `PostCompact` hook triggers from Kiro CLI 3.1.0 (#3136).
- Agent files write an MCP server's `x-kiro.timeout` and `x-kiro.requestTimeout`. A portable `timeout` is left out with a note naming both keys (#3041).
- `import kiro` keeps an agent's inline `mcpServers` through the next sync. Servers that match `.kiro/settings/mcp.json` become a list of MCP spec names; any other map moves to `x-kiro.mcpServers` (#3051).
- `import kiro` turns literal credentials in an agent map it moves to `x-kiro.mcpServers` into `${NAME}` references, with the same note as MCP file import, and leaves out a server whose `oauth` holds a credential (#3067).
- `import kiro` leaves out a `.kiro/settings/mcp.json` server whose `oauth` holds a literal credential, with a note, instead of writing the secret into its MCP spec (#3074).

#### Pi

- **Breaking:** MCP `timeout` is milliseconds for Pi too; sync and import convert it (#3002). Multiply a Pi `timeout` written in seconds by 1000. Lint and sync flag only values under 1000, so check larger ones yourself: an old `timeout: 1800` (30 minutes) now means 1.8 seconds; write `1800000`.

#### Qoder

- The target page says only the Qoder IDE documents that rule files win over `AGENTS.md`; the CLI documents no order (#2984).

#### Qwen Code

- Skills keep `disable-model-invocation`, `user-invocable`, and `paths` in `.qwen/skills/`, so a manual-only skill stays out of the model's reach, and import reads them back as shared fields (#3042).

- Agents keep `background` and their `hooks` in `.qwen/agents/`; a shared `Stop` hook becomes `SubagentStop`. Agent hooks used to be dropped with no note. Import reads `background` back as a shared field (#3043).

- The target page says project hooks need folder trust or hook approval: from 0.26.7 in interactive sessions, from 0.26.8 in headless runs too (#2978, #3015).

#### Trae

- The Trae page says the Build step under Settings > Indexing & Docs applies only to clients released before October 22, 2026; newer clients no longer have that panel (#3044).

#### Kimi Code CLI

- Sync and import keep a stdio MCP server's `x-kimi.environment_id`, which Kimi Code 2.5.0 reads in place of `runtime_id` (#2980).

#### CodeBuddy Code

- An agent with `x-codebuddy.tools: []` gets `tools: []`, which CodeBuddy 2.179.0 and later read as no tools; sync used to write `tools: ""`, every tool (#2983).

#### Copilot

- The sync note for an agent's `hooks` names `x-copilot.hooks`, which Copilot CLI reads, instead of saying the agent file has no hook key (#3137).

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

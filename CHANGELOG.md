# Changelog

Changes by release, following [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Releases before v0.90.0 are in [docs/CHANGELOG-archive.md](docs/CHANGELOG-archive.md).

## [Unreleased]

### Highlights

- **Breaking:** layer names in `list`, `status`, `explain --layers`, the web interface and adapter input changed, and `--global` is now `--scope user` (#3856).
- Send questions, reviews and file handoffs between local Claude Code and Codex sessions from the companion or from the agents themselves (#3832, #3836).
- Approve or deny a Claude Code or Codex permission request from its companion row (#3906, #3908, #3909, #3911).
- Companion updates check the signer, show progress, and restore the earlier app when the new one fails to start (#3850, #3888).
- The companion opens and resumes chats on Windows and Linux, and switches to the exact tab in iTerm2, WezTerm, kitty, tmux and Konsole (#3221, #3222, #3843).

### General

- **Breaking:** layer names changed: `project-user` is `local`, `global` is `user`, `global-local` is `user-local`. Update scripts that match them (#3856).
- **Deprecated:** `--global` is now `--scope user`, `new rule --scope <dir>` is `--dir`, and `doctor --scope` is `--checks`. The old spellings warn (#3856).
- The session mailbox lets Claude Code and Codex sessions send, review and hand off files, with agent messages held for you. Only the companion can approve them or act as you (#3832-#3837, #3872, #3891, #3898, #3915).
- Typing into a chat or resuming one in a terminal through the local server needs the companion or a tab `view` opened. Scripts that sent through the API stop working; an open without proof returns the command to run (#3930).
- Companion updates check the signer, show each step, keep the earlier app until the new one starts, and keep Accessibility access (#3601, #3850, #3874, #3888). On macOS, grant Accessibility once more after this first update.
- The companion opens chats on Windows and Linux and switches to the exact iTerm2, WezTerm, kitty, tmux or Konsole tab (#3221, #3222, #3843-#3848, #3870, #3900, #3904).
- Each companion row under Needs you says why, stuck shells say how many still run and which subagent left each, with passwords masked, and the approvals mode is a small chip (#3912, #3917, #3921, #3923).
- Click a chat's name in the companion to rename it. The Rename button is gone, so row text uses the full width, and the other buttons show over the title only on hover (#3924).
- The companion's ⋯ menu and a new **Settings > Approvals and companion** tab in `view --scope user` set the approvals mode and the Approve button. **Apply to my tools** lists the files and every hook or MCP command before it syncs. Only the companion or a tab `view` opened can loosen approvals (#3913, #3916, #3922).
- File claims show only while one exists, as a chip on the chat that holds them, with a "?" to the docs. Gone sessions' claims release in one step, and a path two sessions want puts both under Needs you until it is released or dismissed (#3914, #3919, #3920).

### By tool

#### Claude Code

- With `approvals.companion: true`, the companion shows a request and its folder with **Approve** and **Deny**. The hook timeout is now 310 seconds (#3906, #3908, #3911).
- The companion lists agent team teammates as subagents in a tree, and a click on one in tmux brings its pane forward (#3905, #3907, #3911).
- An `x-<target>` block on a hook spec now sets `timeout` and `statusMessage` for that target. A spec with such a block gets them on the next `sync` (#3906, #3909).

#### Codex

- With `approvals.companion: true`, the companion shows a Codex request with **Approve**, **Deny** and **Answer in terminal**, for up to 60 seconds (#3909, #3911).
- After `sync --scope user`, Codex asks once to review the changed approvals hook. Trust it again in `/hooks` to keep hook approvals (#3909).

#### Kiro

- With approvals in `review`, `sync --scope user` adds the `approve-for-me` preset to `~/.kiro/settings/permissions.yaml` and keeps your rules (#3809).

#### Qwen Code

- With `yolo`, `approvals status` accepts the `auto-edit` value of `tools.approvalMode` and the other spellings Qwen Code reads (#3861).

#### Pi

- `approvals status` says Pi has no local permission prompt, instead of saying its hooks can only block or observe (#3862).

### Site

- The companion docs say where a companion started from Finder looks for Homebrew and npm, and what ran for updates (#3850).
- New [Scopes](https://agnostic-ai.org/docs/scopes/) page: project, local and user in one table, which wins, and what each command takes (#3856).

## v0.96.0 - 2026-11-20

### Highlights

- Set `approvals` to keep prompts, use a tool's reviewer, or let trusted hooks answer routine prompts in five tools on macOS and Linux (#3700).
- The companion lists cached Claude app chats and Cowork tasks with coding sessions, plus cached ChatGPT chats when `sqlite3` is available (#3511).
- On macOS, Ctrl+Alt+V opens the send picker or pastes into the only eligible Claude Code or Codex chat shown in Terminal or Ghostty (#3522, #3550).
- On macOS 13+, `companion install` can download the matching app when the release includes a prebuilt archive (#3516).
- `sync` stops when a locked pack is missing or an output path points into a source folder (#3712, #3740).

The v0.96.0 macOS companion archives were unavailable at publication. `companion install` builds this version locally using Go and Xcode Command Line Tools.

### General

- `import` updates existing single-file skills by name without creating a duplicate after `sync` (#3797).
- Fresh `init --demo` projects pass `lint` and need no spec migration (#3798).
- The web interface explains how to open it when a link from another site is refused (#3799).

#### Approvals

- Set `approvals` modes and tool or folder overrides in home config, with `always-ask` rules for `yolo` and only `ask` allowed in project config (#3700).
- When settings allow, `sync --global` enables `review` for Claude Code, Codex, `cursor-agent`, Devin CLI, CodeBuddy, Vibe, Qoder and Qwen Code (#3700).
- On macOS and Linux, use `sync --global` with the `approvals` built-in and trust its hooks to use `yolo` in Claude Code or Codex (#3700).
- `approvals status` shows how each configured tool handles prompts, and `approvals log` prints the hook's decisions with `--since` and `--json` (#3700, #3808).
- The companion counts prompts approved by the hook, shows known reasons for waits, and sets home approvals to `ask` with **Ask me** (#3700).

#### Approval safeguards

- `approvals ask` stops hook answers. Run `sync --global` to clear reviewer settings it wrote, then switch modes in running sessions (#3700).
- In `yolo`, edits to agent specs, moved agent, hook, MCP or settings folders, and legacy `agnostic.config.yaml` ask for approval (#3700, #3790).
- In `yolo`, a `Read` or `Edit` ask rule with a path covers shell commands, `rtk` matches `Bash` ask rules, and path rules follow links and ignore case (#3808).
- In `yolo`, home-folder sessions, edits to tool, login-agent and home `bin` folders, reads of `.git` or Git config, and links to credential files ask (#3808).
- With `AGNOSTIC_AI_APPROVALS=ask`, Claude Code logs `asked` for each prompt, so the companion shows it at once, not after 10 seconds (#3808).

#### Approvals in the interface

- The chat list shows the approvals mode in use and keeps the **Needs you** count consistent during searches (#3799).
- The companion keeps keyboard focus on toggles and shows expanded approvals inside their chat row (#3799).
- Permission previews show which shared ask or deny rules a personal list leaves out (#3799).
- The web interface suggests a settings file when the defaults file already holds permission rules (#3799).
- `approvals ask` and **Ask me** explain how to change approvals by hand when YAML aliases or merges prevent an edit (#3799).

#### Sync safety

- `sync` stops on an invalid `agnostic.packs.lock` or missing locked packs, keeping their generated files (#3712).
- `sync`, `status` and `validate` reject any `outputs` path that points into `.agnostic-ai/` or another spec folder (#3740).
- `doctor --fix` backs up edits to whole generated files, and both it and `sync` back up edits through links into the project (#3707, #3736).
- `revert --force` keeps files in `.agnostic-ai/` and configured spec folders, and `revert` reports how many files it restores or removes (#3701).
- When tools write different content to one file, `sync` applies the strictest of their `outputs.<target>.collision-policy` settings (#3631).

#### Sync checks and previews

- `sync` refuses to replace a hand-written MCP file if settings would be lost. Use `--backup` when the `.bak` path is free (#3702).
- `sync --plan` previews `.gitignore` and `.worktreeinclude` changes, and `init --dry-run` previews `.gitignore` (#3714).
- `sync --check`, `doctor --fix`, `use` and `upgrade --requires` stop on missing locked packs or an invalid `agnostic.packs.lock` (#3712).
- `sync` spends less time writing output, including Kilo Code projects with many wildcard permissions (#3634).
- `sync` starts fewer Git processes when you run it outside a Git repository (#3694).

#### Import

- `import` keeps tool-specific model choices on their own targets, so another tool no longer gets a model it cannot use (#3703).
- Gemini CLI and Qwen Code keep command names like `git:commit`, while other tools get `git-commit` with a number if needed (#3704).
- `import` and `doctor` flag legacy OpenCode agent and command files or Copilot prompts that can load twice after sync (#3749).
- After `sync`, `import` keeps an unchanged literal such as `$${NAME}` escaped in the MCP spec when the tool writes it as `${NAME}` (#3741).
- `import` says when no specs changed and shows next steps once for multiple sources, with fewer directory scans on large projects (#3619, #3663, #3704).

#### Import details

- `import claude` uses Codex-specific files to detect Codex, so `.agents/` from Goose or OpenHands no longer limits shared imports to Claude (#3749).
- `import codex` puts a shared settings model in the portable `model.codex` field (#3749).
- Re-importing an edited `git-commit` command keeps its `git:commit` spec name and the Gemini CLI or Qwen Code command it feeds (#3704).
- `import` shows project-relative paths for spec folders whose names start with two dots (#3621).

#### Checks and commands

- `doctor` names hand-written instructions or MCP files that block `sync`, and `doctor --fix` stops before writing when it finds one (#3749).
- `validate` rejects duplicate spec names of the same kind in one layer and outputs that differ only by case on case-insensitive disks (#3705, #3733).
- From a subfolder, `status`, `doctor`, `import`, `packs list` and `packs update` name the project root, and `init` refuses to nest a project (#3706).
- With `AGENTS.md` current or user-owned, `sync -t claude` keeps the same `CLAUDE.md` as a full sync (#3714).
- `explain` finds specs by path, `sync` rejects negative `--jobs`, and `sync --watch` prints one startup summary (#3709, #3749).

#### Command output

- `new` warns when no enabled tool reads a spec kind, and `validate` offers `--fix` only for issues it can fix (#3705).
- `validate` notes unused ignore specs without failing, and `packs` rejects unknown subcommands (#3749).
- `migrate` names kept aliases, `memory lint` names the repo's personal store, and shell completion suggests sources after `import` (#3709).
- `packs add` says when it reinstalls a pack, and `packs list` omits `@HEAD` for a local folder (#3709).
- `status`, `sync --watch` and the web editor handle project folders whose names start with two dots (#3676).

#### Diagnostics

- `explain --context` and `explain --file` run faster in projects with many rules (#3663).
- `doctor` skips Codex hook trust checks when the project has no Codex hooks file (#3707).
- The first sync tells you where tools read rules and skills even when they have no dedicated folder (#3709).

#### Hooks and built-ins

- `hook run` follows Claude Code 2.1.328 for `Bash(...)` matching for quotes, substitutions and wrappers such as `timeout` (#3681, #3698, #3742).
- Enable `cloud-handoff` to push work to a branch and continue it with the signed-in Claude Code or Codex CLI (#3666).
- Automatic Claude Code cloud launch uses `script`, while starting a Codex cloud task requires an environment ID (#3666).
- Built-in session skills point to `cloud-handoff`, which stops if it cannot commit or push your work (#3690, #3692).

#### Web interface

- On the Conversations page, working Claude Code subagents open their parent chat and working Codex subagents open their own thread (#3667).
- Copying a hook between project and global setups brings its shared scripts, and **Add hook** skips tools missing the chosen stage (#3719).
- The web interface blocks detected credentials in new hooks, and pages from `agnostic-ai serve` reject other sites in `?source=` (#3710).
- **Undo last change** groups review notes, and the sidebar shows **Setup health** errors and warnings separately (#3728).
- **Collapse all** on Lifecycle hides the stages without hooks again (#3529).

#### Web editing and API

- The web editor keeps existing permission rules on separate lines when the list stays nonempty and shows hooks with an `=` in the Windows command (#3719).
- `/api/switch` rejects unknown fields, `/api/global/create` requires `git`, and applying edits requires the preview value (#3728).
- The web editor points to the right YAML error line when a list or map is left open (#3730).
- Turn on the `cloud-handoff` built-in from the web interface and see its skill on the Context page (#3691).
- The conversations API lists `chatgpt` among the accepted tools when a request names an unknown tool (#3622).

#### Companion app

- Companion downloads check the checksum and verify build provenance with supported, signed-in `gh` when verification is enabled (#3516, #3520).
- On macOS, the disk image app comes with the CLI and can update itself, and the installed companion uses its own permissions (#3516, #3601).
- `companion install` stops on macOS before 13, `companion uninstall` removes unrunnable builds, and `upgrade` names lagging install sources (#3591).
- The companion restarts `serve` after three list timeouts if its health check fails or a scan has run for two minutes (#3591, #3601).
- For experimental builds, `companion uninstall` clears Windows login and web data, and `companion stop` reports missing Linux D-Bus (#3221, #3222).

#### Companion installation and updates

- On macOS, **Install command line tool** links the disk image app's bundled CLI when you have no existing install (#3611).
- On Apple silicon, `companion install` picks the native download even when the CLI runs under Rosetta (#3594).
- Canceling `companion install` clears its temporary build folder and leaves an existing app alone before replacement starts (#3594).
- `upgrade` refreshes a companion built for an older release even if the CLI is already current (#3594).
- `companion update-app` explains its restart and version rules, and it and `companion install-command` show usage examples (#3716).

#### Chat list

- Unread replies in cached Claude chats, Cowork tasks and ChatGPT chats appear under **Needs you**, with `sqlite3` required for ChatGPT (#3511).
- Rename Claude Code, local Codex and Cursor chats in their own tools, plus ChatGPT chats with a ChatGPT sign-in in Codex (#3512).
- Expand a chat to see working Claude Code and Codex subagents, with each Claude Code subagent's latest available message (#3518, #3667).
- **Needs you** puts permission requests first and shows details when available, and the circle shows a lock when a chat is blocked (#3526, #3533).
- On macOS and Linux, terminal chats from Claude Code, Codex and `cursor-agent` move to **Archived** once known closed, even unread (#3527, #3596).

#### Chat menu

- The companion chat menu lets you mark a chat complete, undo completion, or hide an archived chat (#3800, #3804).
- **Stop chat…** in the macOS companion asks before ending a live Claude Code terminal session and keeps its transcript (#3805, #3806).
- The companion and web dashboard flag forgotten Claude Code background shells and let you choose one to stop (#3792).
- The list marks a chat that waits for your permission, an answer or a forgotten shell with the circle's lock, apart from unread replies (#3807).

#### Chat history and actions

- Codex, Cursor editor and Cowork archives show under **Archived**, along with Claude app chats the running server saw leave the sidebar (#3524).
- You can delete an eligible closed Claude Code chat without reading its last reply first (#3596).
- Claude chats and Cowork tasks open in the Claude app, and ChatGPT chats open on chatgpt.com (#3511).
- A Claude Code subagent row opens its parent chat and names the subagent, while a Codex row opens its own thread (#3667).
- After you mark, rename or delete a chat, the companion waits for the updated list before the action finishes (#3603).

#### Companion fixes

- Companion search shows recent matches first, skips refreshes while closed, and limits pasted queries to 200 characters (#3606, #3607, #3708).
- Enter on a focused companion button only runs that button, and shortcut setup rejects common shortcuts such as Cmd+C (#3708).
- **Mark all complete** skips chats that changed in the latest list, and screen readers announce the selected chat (#3591).
- A closed Claude Code Remote Control session and its cloud copy stay under **Shared** while the running server still links them (#3541).
- On macOS, dragging keeps the companion list open, resizing keeps its side, and the circle shows its whole shadow (#3513, #3597).

#### Companion reliability

- The companion starts with defaults after invalid JSON in `companion.json` and suggests an update when an older CLI cannot start its server (#3591).
- A failed shortcut change keeps the old keys, and the companion can start before macOS detects a screen (#3593).
- Companion error notices have a red border and are announced by screen readers (#3531).
- The companion list skips named pipes in Claude Code transcripts, live session files and background job files (#3601).
- Working chats and subagents have animated markers that keep moving smoothly when the list redraws (#3513, #3544).

#### Saved chat marks

- Deleted Claude Code, Codex and `cursor-agent` chats lose saved marks once full reads have missed them regularly for at least 30 days (#3540).

#### Sending copied text

- On macOS, Ctrl+Alt+V opens the picker or pastes into the only eligible chat shown, while Cmd+Shift+V in the list opens the picker (#3522, #3550).
- On macOS, the picker fills eligible live Claude Code or idle Codex inputs in Terminal or Ghostty and leaves Enter to you (#3521).
- On macOS, copied web text can paste as Markdown when HTML adds structure, matches the copied words, and fits within the send limit (#3517).
- The send picker hides archived chats, explains what Enter does, and sends copied text to the parent chat when you click a subagent (#3550, #3591).
- If a send loses its connection or times out after 60 seconds, the companion tells you to check the chat before sending again (#3591).

#### Send fallback and permissions

- Pasting into Terminal needs macOS Accessibility access, and copied plain text ending in whitespace stays for manual paste (#3517, #3525).
- The companion tries to open other chats for manual paste and asks you to resume an ended terminal chat first (#3517).
- The companion warns when it cannot read the clipboard, and Enter waits for a picker search to finish before it can send (#3550).

#### Opening chats

- On macOS, `claude --bg` rows use `claude attach` in Terminal or Ghostty, with a copyable command when available (#3515).
- With a recent list, running terminal or cloud chats can open during a refresh, and the panel stays hidden on success or after 60 seconds (#3530, #3548).
- Concurrent clicks share one chat open, and late failures show why, with a resume command when available (#3537, #3685).
- Closed terminal chats offer **Resume in Terminal** or **Resume in Ghostty**, and a detached `tmux` chat can offer a command to run after it ends (#3527).
- When the companion cannot reach `agnostic-ai`, it says so in plain words instead of a Go error (#3530).

### By tool

#### Goose

- **Removed:** Goose review output after Goose 1.60 dropped `goose review`, with the old root `.agents/REVIEW.md` still importable (#3403).

#### Mistral Vibe

- `sync` and `sync --global` write Vibe 3.1.3 `pre_llm` and `post_llm` hooks and keep `x-vibe.strict` when set (#3766).
- `lint` accepts `matcher` on Vibe tool, agent and session hooks (#3558).
- Vibe skill descriptions can use up to 1,024 Unicode characters, counted as code points (#3570).

#### Copilot

- `import copilot` reads `.github/prompts/*.prompt.md` as commands when no matching spec exists (#3702).
- `import copilot` keeps the first copy of a project skill from `.github/skills`, `.agents/skills` or `.claude/skills`, in CLI order (#3557).
- For Copilot CLI agents, `sync` points to `x-copilot.reasoningEffort` when portable `effort` cannot set it (#3563).

#### OpenCode

- `import opencode` reads project files in `.opencode/agent/` and `.opencode/command/` and keeps MCP servers' explicit `enabled: true` on sync (#3702).
- For the same scope, `import opencode` keeps the first copy from `.opencode/skills`, `.agents/skills` or `.claude/skills`, in that order (#3567).

#### Codex

- For Codex, `sync --global` points `commandWindows` at the copied script when its path starts with `$CLAUDE_PROJECT_DIR/.agnostic-ai/scripts/` (#3749).
- `import codex` uses a free full-folder name for a hand-written nested rule if the short name belongs to another folder, and names skipped files (#3704).

#### Factory

- A Factory hook matcher of exactly `Bash`, `Write` or `WebFetch` no longer gets a false note that it does nothing (#3571).
- For Factory, `hook run` explains its Windows and shell limits and says it uses the project root (#3573, #3592).

#### Kiro

- Project import and sync keep remote MCP `oauth.mode` for Kiro IDE 1.4.236+ and CLI 3.5.0+ V3 sessions (#3767).
- `validate` and `sync` accept `PermissionRequest` command hooks for V3 sessions in Kiro CLI 3.4.0 and later (#3554).
- With the `approvals` built-in, `sync --global` adds a `yolo` hook for routine prompts in Kiro CLI 3.4.0+ V3 sessions (#3700, #3788).

#### OpenHands

- `sync` writes OpenHands path rules as flat Markdown files in its skills folder, keeping their triggers and avoiding Codex skill warnings (#3768).
- `import openhands` keeps a generated path rule and a same-named skill as separate specs (#3768).

#### Cursor

- `import cursor` reads `.cursor/mcp.json` and `.cursorrules`, and `doctor` reports hand-written files without matching specs (#3702).

#### Windsurf / Devin Desktop

- With Devin enabled and default rule paths, `import windsurf` prints a cleanup notice for hand-written `.windsurf/rules/` files (#3704).

#### Qwen Code

- Project and user settings round-trip supported `effort` values through `model.reasoningEffort` in Qwen Code 0.28.0 and later (#3556).
- In `yolo`, `sync --global` adds a hook for safe shell, read, edit and write calls. Set `tools.approvalMode` to `default` or `auto_edit` (#3700, #3789).
- A `permissions.ask` rule or a `.env` or `env` block that moves the hook's policy turns the Qwen Code hook off; `approvals status` says so (#3700, #3808).

#### Junie

- A Junie agent name may start with a digit, such as `7review` (#3555).

#### Qoder

- Project and user import and sync keep a Qoder MCP server's explicit `protocolVersion` (#3769).
- In `yolo`, `sync --global` adds a Qoder hook for safe `Bash`, `Read`, `Edit` and `Write` calls in `default` and `accept_edits` mode (#3700, #3791).
- A `permissions.ask` rule in Qoder user or project settings turns its hook off, and `approvals status` says so (#3700, #3791).

#### Kilo Code

- `sync` drops invalid MCP timeouts from Kilo Code's project config so Kilo uses its default and still loads the servers (#3765).

#### Cline

- `sync` warns when a hook in `.clinerules/hooks` uses `TaskResume`, `TaskError` or `SessionShutdown`, which Cline for VS Code skips (#3723, #3749).

#### Trae

- `hook run` marks successful Trae hook output as model context on supported events, including plain text at session start or prompt submission (#3569).

#### Grok Build

- Project and user import and sync keep `x-grok.headers_helper` for dynamic MCP headers in Grok Build 1.0.88 and later (#3568).

### Site

- The docs lead with common tasks and move long tables to reference pages, and the home page is shorter on desktop and phones (#3552).
- New guides explain approvals modes and how to hand off work to Claude Code or Codex in the cloud (#3689, #3700).
- Authority notes and the tool comparison table link directly to reference sections for settings and opt-ins (#3553).

#### Target guides

- Guides cover Claude MCP hooks, Kiro Memories, OpenCode V1 and skill names, Crush prompts and Junie ACP versions (#3770, #3771, #3772, #3773, #3774, #3775).
- Guides clarify Goose hook data, Auggie 1.x memory, Cline VS Code hook replies and Factory hook output and launchers (#3572, #3573, #3574, #3577, #3578).
- Guides explain Kiro OAuth scopes, protected paths, Devin ignores, OpenCode V2 Claude flags and Junie ACP support (#3559, #3560, #3561, #3575, #3576).
- The Grok and Augment guides link to vendor documentation for `StopCancelled` and terminal shell rule matching (#3745, #3746).

## v0.95.0 - 2026-11-15

### Highlights

- `import` keeps native settings under `x-<target>` for seven tools, so `sync` no longer drops them (#3380, #3390, #3391, #3393, #3394, #3395, #3479).
- `import kiro` reads `.json` agent profiles and agents in subfolders of `.kiro/agents/`, which it used to skip (#3462).
- `hook run` reads Factory 1.11 and Crush 0.101 replies the way those tools do, and sets Crush's `CLAUDE_PROJECT_DIR` (#3477, #3478, #3489).
- Re-importing an agent with a per-target `effort` or `model` map changes only the imported tool's value (#3466, #3470).
- A chat closed in its terminal moves to Archived as soon as you have seen its last reply, and the companion offers a smaller circle (#3447, #3457).

### General

- Re-importing an agent whose `effort` or `model` is a map per target changes only that tool's entry and keeps the others (#3466, #3470).
- A Claude Code, Codex or `cursor-agent` chat closed in its terminal moves to Archived once you have seen its last reply, not a day later (#3457).
- Both companion menus offer Small companion, a 40px circle in a smaller window instead of the 60px one (#3447).
- Agent-as-skill output uses neutral inline instructions, and import still recognizes agent skills generated before (#3436).
- Target audits can run in an isolated worktree, save each result in one checked write, and keep idle reviewers busy (#3369, #3370, #3371, #3372).

### By tool

#### OpenHands

- Agent import keeps native tools, permissions, color, hooks, MCP servers and run settings, including MCP deletions (#3380, #3408, #3443).
- Agent MCP import refuses literal credentials and keeps environment references (#3450).
- Fresh hook imports keep each command's timeout, async mode and order through sync (#3496).
- `hook run` reports added context from SDK 1.62.0 `SessionStart` replies that hold `additionalContext` (#3382).
- Folder skill import keeps native keyword triggers through sync (#3381).

#### OpenCode

- `import opencode` keeps MCP keys such as `oauth: false` and `{env:NAME}` OAuth secrets under `x-opencode`; literal secrets are refused (#3479, #3493).
- The web editor keeps `oauth: false` under `x-opencode` when it adds an MCP server, so sync keeps OAuth off (#3491).
- `import opencode` keeps a command's native `subagent` flag, including `false`, so `sync` no longer drops it (#3383).
- `explain` reports the permission keys `sync` writes for an agent's `tools`, and reads an `x-opencode.permission` override (#3407, #3425).

#### Factory

- `hook run` reads Factory 1.11 replies: `ask` allows, top-level `decision: "block"` denies, and JSON applies on any exit but 2 (#3477).
- Hooks write `onFailure: "block"` from `failClosed` and `preserveRiskLevel` from `x-factory`, and import reads both back (#3476).
- `import factory` keeps each hook command's own timeout and the command order through sync (#3490, #3492).
- `import factory` keeps a group's `commandRegex` as `x-factory.commandRegex`, and `sync` writes it back on that group (#3397).

#### Antigravity

- `import antigravity` keeps an agent's `tools`, `mcpServers`, `skills` and other native keys under `x-antigravity`; inline credentials are left out (#3393).
- `hook run --target antigravity` adds `lastUserInput`, `hookEventName`, `cwd` and `toolCall.id` to the payload (#3453).
- `hook run` reads a successful Stop reply of `{}` as `allow`, matching the released backend (#3402).
- `sync` keeps the `flash_lite` model tier in Antigravity plugin agents (#3429).

#### Copilot

- `import copilot` keeps an agent's `tools: []` and moves `user-invocable`, `disable-model-invocation` and `hooks` under `x-copilot` (#3391).
- `sync` and global import keep Copilot-only MCP keys, such as `oauthClientId`, `cwd` and `oidc`, out of `.vscode/mcp.json` (#3430, #3442).
- `import copilot` reads a hook's `timeout` alias and prefers an explicit `bash` entry over `command` (#3392).

#### Crush

- `hook run` reads Crush 0.101 replies: `decision: "block"` blocks, `continue: false` halts, and JSON applies on any exit but 2 and 49 (#3478).
- `hook run --target crush` sets `CLAUDE_PROJECT_DIR` to the project root, matching Crush 0.101.0 (#3489).
- `lint` no longer tells you to drop the matcher from a `PermissionResult` hook, since Crush filters it by tool name (#3385).

#### Goose

- `sync --global` writes hooks to the user plugin `~/.agents/plugins/agnostic-ai-global/`, which Goose loads for every project (#3389).
- `import goose` skips plugins in `disabledPlugins`, so `sync` no longer turns their hooks and skills on (#3398).
- `import goose` keeps each hook's own `timeout` and `on_failure`, and warns about a `$PLUGIN_ROOT` command (#3399).

#### Kiro

- `import kiro` reads `.json` agent profiles and agents in subfolders, and notes the original to remove after sync (#3454, #3462).
- `import kiro` keeps an agent's `excludedTools`, `resources`, `permissions`, `hooks` and other native keys under `x-kiro` (#3394).
- `lint` no longer warns (LINT005) about a `matcher` on the `PostFileCreate`, `PostFileSave` and `PostFileDelete` events (#3455).

#### Mistral Vibe

- A portable `Grep` tool maps to Vibe's native `grep`, so a `Grep`-only agent no longer fails `sync` (#3379).
- `import vibe` keeps native skill `allowed-tools` under `x-vibe`, so names such as `read_file` no longer block sync or lint (#3434).
- `explain` reports a Vibe agent's `x-vibe.enabled_tools` override instead of the portable mapping it replaces (#3407).

#### Augment

- An agent's `effort` becomes the Auggie 1.x `thinking` key, and `import augment` reads `.augment/agents/` back into `effort` (#3451).
- `import augment` reads `.augment/settings.json` with comments and trailing commas, as Auggie does (#3384).
- The Augment guide and the agent `tools` sync note name both the 0.x and 1.x tool names (#3387).

#### Codex

- `hook run` uses Codex's 1s default and 3s maximum for `SessionEnd` and `Interrupt` (#3432).
- `hook run` treats Codex `SessionEnd` handlers as synchronous, even with `async: true` (#3433).

#### Amp

- The note for a portable `model` and the target page point to Amp's model selector, which replaced the Dial (#3386).
- The target page says a shared orb snapshot keeps only the `PATH` additions setup makes to shell profiles (#3444).

#### Windsurf / Devin Desktop

- `import windsurf` keeps nested rules' native globs and placement, so `sync` no longer adds a wider root rule (#3396).
- `import windsurf` keeps an agent's native `max-nesting` under `x-windsurf`, so `sync` no longer drops it (#3395).

#### Zed

- `import zed` keeps `enabled: false` and moves `timeout`, `oauth` and `remote` under `x-zed`; an `oauth` client secret is left out (#3390).
- The target page says a `WorktreeCreate` task runs only in a trusted project; Zed skips it in restricted mode (#3485).

#### Qwen Code

- `readonly: true` writes `disallowedTools: [write_file, edit, notebook_edit]` instead of a sync note, and import reads it back (#3452).

#### CodeBuddy Code

- Sub-agents can restrict tools to one MCP server with `mcp__docs` or `mcp__docs__*` on CodeBuddy 2.181.0 or later (#3486).

#### Cline

- `sync` and `doctor` warn when a global skill, including one in `~/.agents/skills`, hides a project skill of the same name (#3400, #3424).

#### Pi

- `sync --global` and `import --global pi` keep MCP `auth.provider` without turning it on in project files (#3431).

#### Grok

- `import grok` keeps a `{{session_id}}` template in an MCP header or env value, which Grok fills in, instead of a variable (#3480).

#### Trae

- Fresh hook imports keep each command's timeout and order through sync, with the group's loop limit (#3496).

#### Kilo

- The agent `effort` note points to `x-kilo.variant`, and setting that override hides the note (#3484).

#### Jules

- The target page says Jules is read-only since November 12, 2026, and drops the step that needs a new Jules task (#3388).

#### Continue

- The `assistants-dir` note says Continue loads it as a local profile at `.continue/assistants`, `agents` or `configs` (#3401).

### Site

- The Factory guide shows how to adopt a changed native hook order through a clean import and a reviewed spec swap (#3499).
- Junie lists `WebFetch` as supported, and Zed's guidance describes automatic worktree tasks (#3435, #3437).

## v0.94.0 - 2026-11-12

### Highlights

- **Breaking:** `validate` fails a hook scoped with `targets:` to a tool that does not know its event; `sync` used to drop it silently (#3354).
- `sync` no longer loses a target's rules because of `outputs.<target>.rules-file`, and stops with AAI-102 where it would (#3301, #3318, #3343).
- Delete an archived Claude Code chat for good from the companion, including one that went quiet on its own (#3289, #3294).
- `agnostic-ai conversations start` opens a linked conversation in a new terminal tab, from inside Claude Code, Codex or `cursor-agent` (#3304, #3312, #3342).
- The companion and the web interface show the agent's latest reply or question under each chat's title (#3296, #3333, #3349).

### General

#### Settings and hooks

- **Breaking:** `validate` fails a hook scoped with `targets:` to a tool without its event. Use that tool's event name or an `x-<target>` event (#3354).
- The new `wrap-up` built-in adds a skill that ends a session with what changed, what is still open and a note to share; `handoff` points to it (#3293).
- A model tier takes an optional `description` of when to use it; `sync` and `sync --global` list the described tiers in each tool's instructions file (#3291).
- `memory import --skip <file>` and `--skip-rest` decline facts you do not want, and LINT043 stops counting them (#3235).
- The web interface's Built-ins tab says what each built-in does when on, and links RTK and Caveman to their pages (#3261).

#### The rules-file setting

- `sync` ignores `outputs.<target>.rules-file` on a target that writes no merged rules file, such as Cline or Kiro, and `lint` warns with LINT062 (#3301).
- `sync` stops with AAI-102, which `validate` reports, when a `rules-file` would lose another target's rules; point it elsewhere (#3318).
- When the other tool's file already has the rules, as with Amp, `sync` keeps it; `lint` warns with LINT063 (#3318, #3343).
- A `rules-file` of `./AGENTS.md` beside Amp no longer saves a `.bak` each sync; a project stuck on `AGENTS.md` heals on the next `sync` (#3337, #3345).
- Cursor and Windsurf load each always-on rule once, from `AGENTS.md`; Trae opts in with `rules-mode: entry-point`, and LINT061 flags an ignored value (#3243).

#### Sync safety

- `sync` and `status` stop with AAI-102 when a file output sits on a `.bak` that sync saved, ignoring letter case (#3345, #3359).
- `sync`, `status` and `validate` stop with AAI-102 when two outputs differ only in letter case, such as `agents.md` and `AGENTS.md` (#3359).
- `sync`, `doctor --fix` and `revert` stop on a `.sync-state` from a newer agnostic-ai; upgrade every copy that syncs a project (#3345).
- `revert` takes the project lock, and `revert --force` no longer deletes a shared rules-file it just restored (#3359).

#### Companion menus and app

- Both companion menus offer Animate, Dim when idle, Start at login, the shortcut, Hide and Quit (#3237, #3246, #3303, #3317).
- Rename a chat from the pencil button on its row or with F2, not only from the right-click menu (#3368).
- Hide takes the companion off the screen until the shortcut or a waiting chat brings it back, and names `agnostic-ai companion` while the shortcut does not work; Quit asks first and offers Hide (#3306, #3351, #3356, #3366).
- `upgrade` also updates an installed companion and restarts it if it was running (#3247).
- On macOS, `companion install` makes an app with the agnostic-ai icon that still starts at login (#3238).

#### Chat list

- Mark a chat as read from its right-click menu; **Mark all as read** on Needs you clears every unread or waiting chat, of any age (#3270, #3305, #3341).
- The companion's buttons read "Mark complete" and "Mark all complete", and a chat marked complete shows a green tick for a moment (#3273).
- Rows no longer repeat their group as "New reply" or "Working", and same-named folders such as `shop/api` and `billing/api` are told apart (#3305, #3269).
- A silent local Codex chat shows "No activity for 7m", not "Waiting for you", and the API sets `quietSince` (#3267).
- The companion and the web interface show a Claude Code session with Remote Control on as one row, marked Shared, instead of a local row and a Cloud row. A click opens it here; Open on claude.ai opens its web page (#3363).

#### Chat names, replies and states

- Rename a chat from the companion's right-click menu; the web interface and a running chat's Terminal or Ghostty tab show the new name (#3236, #3258).
- Chat titles show what you typed, without the tags tools add, and skip a background task's report as a first prompt (#3241, #3335, #3339, #3352).
- Local Claude Code, Codex and Cursor chats show the agent's question or last message as a reply line, held in the API's `reply` field (#3296, #3333, #3349).
- An archived or hidden chat that starts working again moves back to Working (#3239).

#### Opening and resuming chats

- A running session opens in its Terminal or Ghostty tab; an ended session resumes in a new tab (#3242, #3249).
- When the companion cannot open a chat, the reason stays until you dismiss it, and a chat that could not resume offers **Copy resume command** (#3271).
- No resume command is offered for a chat that still runs, since it would start a second copy (#3271, #3276).

#### Linked conversations

- `agnostic-ai conversations start`, run inside Claude Code or Codex, opens a chat in a new tab of the same folder, with a brief as its first prompt (#3304).
- The new chat shows indented under the one it came from in the web interface and the companion, and its row has a `parent` in the API (#3304, #3312).
- The new `side-conversation` built-in adds a skill that tells the agent when to start one and how to write the brief (#3304).

#### Deleting archived chats

- Delete an archived Claude Code chat for good from the companion's right-click menu, with its transcripts, tool output and backups (#3289).
- A Claude Code chat that went quiet on its own under Archived can be deleted too, unless it was written to since you last looked (#3294).
- Other tools say why their chats cannot be deleted; close a Codex thread before running `codex delete <id>` (#3289, #3292).

#### Security

- A folder or program path with control characters can no longer type a command into a terminal tab; start and resume refuse it (#3304).
- Builds use Go 1.27.4 to fix the `net/http` vulnerability [GO-2026-6859](https://pkg.go.dev/vuln/GO-2026-6859) (#3244).

### By tool

#### Augment

- `hook run` expands `${VAR}` and `${VAR:-default}` in a hook's command and `args` as Auggie 1.x does, so a default that blocks now reports block (#3358).
- `hook run` builds `PromptSubmit`, `Stop`, `SessionEnd` and `Notification` payloads without `--payload`, and reads a background block as allow (#3338, #3350).
- `hook run --bash` and `--edit` send Auggie 1.x tool names (`bash`, `edit`, `write`), and the next `sync` rewrites portable hook matchers to fit (#3353).
- `sync` writes hooks with `args` and `PromptSubmit` or `Notification` events, and skips one on an event Auggie never fires, with a note (#3281, #3314, #3354).
- Settings `permissions.ask` writes `ask-user` rules, and `model` and `effort` set Auggie's defaults; `import` reads them (#3280, #3282).

#### Cursor

- `cursor-agent` chats are listed with their folder even without an editor chat index; a running one shows as open and opens its terminal (#3279, #3342).
- `conversations start` works from Cursor's terminal agent, and `--tool cursor` starts a `cursor-agent` chat; Cursor editor chats are refused (#3342).
- Cursor chats show what you typed as the title and the right prompt and reply, and a chat written twice in one second is reread (#3241, #3349, #3357).
- A `cursor-agent` chat whose folder is unknown offers no resume command, since `cursor-agent` resumes a chat only from its own folder (#3349).

#### Qwen Code

- A command hook `timeout` of 1000 seconds or more is written in milliseconds; `timeout: 1200` gave a 1.2 second limit (#3316).
- `lint` no longer raises LINT005 for a `matcher` on a `FileChanged` hook, which names the files to watch (#3277).

#### Copilot

- Project and global settings accept the effort values `none`, `minimal` and `max`; per-agent effort keeps `low`, `medium`, `high` and `xhigh` (#3315).

### Site

- `serve`, the local API, `companion`, `doctor`, `import`, and the `sync` and `gitignore` settings have their own reference pages; old links redirect.
- The companion page says Windows and Linux are not supported and links the issues.
- The Augment guide calls Auggie 1.x stable from 1.2.0, not a prerelease (#3313).
- The Qwen page says a `FileChanged` hook can also replace or clear the paths registered through `watchPaths` (#3278).

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

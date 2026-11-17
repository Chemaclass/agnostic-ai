+++
title = "Web interface"
description = "Open the local web interface with view, or serve it with its JSON API."
weight = 42

[extra]
group = "Reference"
+++

# Web interface

`view` opens the web interface for the configuration you are in. `serve` keeps it running and adds a local JSON API. Its routes are in the [local API](@/docs/cli-reference/serve-api.md).

## view

Open the bundled web interface in your browser. `ui` is an alias.

- From a project folder or any folder inside it, `view` reads that project.
- Outside a project, it reads the user configuration in `$AGNOSTIC_AI_HOME` (default `~/.agnostic-ai`).
- Keep the terminal running. Ctrl-C stops it.

See the [browser walkthrough](@/docs/inspect-configuration.md).

```bash
agnostic-ai ui
agnostic-ai view --json
agnostic-ai view --global
```

| Flag | Description |
|------|-------------|
| `--json` | Print the overview as JSON (schema `overview/v1`) without starting a server or opening a browser. |
| `--dir` | Read this project directory instead of discovering one. |
| `--addr` | Explicit loopback IP and port. Default `127.0.0.1:4310`. When set, `view` stays on this port and fails if another configuration serves it. |
| `--global` | Read the user configuration instead of a project. Cannot combine with `--dir`. |

A second run for the same project and scope opens the running server and exits. When the default port serves another project or scope, an `--api-only` server, or another program, `view` takes the next free loopback port up to `127.0.0.1:4319` and prints it. Later runs find that server. An invalid project configuration reports an error.

- The overview lists context, authority, tools, lifecycle, and settings, with findings for the selected targets. The JSON overview also compares generated files.
- Specs scoped only to other targets are left out of lists and counts.
- Search within a section to find a source file or value. Press **Search** (`Cmd+K` or `Ctrl+K`) to search every section, findings included.
- Opening a result puts a link to that entry in the address bar. The link survives a reload.
- The browser has light and dark themes and works on narrow screens.

Other browser features are under [serve](#serve).

## serve

Serve the bundled web interface and a local JSON API. It works offline without a source checkout or frontend build. It reloads configuration on every request.

```bash
agnostic-ai serve --global
agnostic-ai serve --api-only
```

| Flag | Description |
|------|-------------|
| `--addr` | Explicit loopback IP and port. Default `127.0.0.1:4310`. |
| `--allow-origin` | Additional exact browser origin, such as `http://127.0.0.1:5173` for Vite. Otherwise the server accepts loopback hosts and same-origin requests. |
| `--dir` | Project directory to read. Default: the current directory. |
| `--ui` | Serve a built client directory, such as `ui/dist`, instead of the bundled one. |
| `--api-only` | Serve only the JSON API. Cannot combine with `--ui` or `--open`. |
| `--open` | Open the served URL in a browser. |
| `--global` | Serve the user configuration. Its overview shows `lint --global` findings and drift against what `sync --global` would write. Doctor checks are skipped. |
| `--no-installed` | Skip comparing generated files with disk. |
| `--cloud-sessions` | Also list Claude Code cloud sessions, the ones at claude.ai/code, and Codex cloud threads. Needs `--global`. Off by default: without it nothing reads your sign-in or leaves this machine. See [cloud sessions](@/docs/cli-reference/companion.md#claude-code-cloud-sessions). |

## Pages in the browser {#pages}

| Page | What it is for |
|------|----------------|
| Conversations (`/`) | Recent Claude Code, Codex and Cursor chats. Reopen one where it lives. |
| Context | Size of what each coding tool reads at startup, ranked by source. |
| Authority | Permission rules and protected paths, with a preview before every save. |
| Tools | MCP server specs, and servers found only in a tool's own file. |
| Lifecycle | Hooks by stage. **Add hook** opens a form. |
| Settings | Built-in switches, coding tools, model, effort and editor. |

Every edit shows a preview first. **Apply** writes only the source file. Then run the `agnostic-ai sync` command the page names.

## Conversations page {#conversations-page}

The page the interface opens on. It lists your recent chats and reopens one where it lives. A static snapshot has no server to read chats, so it opens on **Context** instead.

| You want to | Do this |
|-------------|---------|
| Reopen a chat | Use the row's action. It opens the Terminal or Ghostty tab, a new tab, the editor panel, the Cursor chat or the desktop app. |
| Resume an ended chat by hand | **Copy command**. |
| Find a chat | Search covers every conversation, loaded or not. The tool filter beside it shows one tool's chats. |
| See older chats | Scroll to the end, or **Load the week before**. |

{% <details summary="Details: order, loading, tabs, requirements"> %}
  - It lists recent Claude Code, Codex and Cursor chats from each tool's own store.
  - Open chats come first under **Open now**, with **Working** or **Waiting for you** when the tool reports it. A local Codex chat that wrote nothing for 5 minutes shows **No activity for 7m** instead, since Codex does not say when it waits. The rest follow by day, newest last message first.
  - It loads the past week and every blocked or unread chat of any age. It loads the week before each time you scroll to the end, or with **Load the week before**.
  - The tool filter beside the search shows one tool's chats. Search covers every conversation, loaded or not.
  - A chat started with [`conversations start`](@/docs/cli-reference/conversations.md#the-link-line) sits indented under the chat it came from while both are in the same group.
  - Each row's action reopens the chat where it lives: the Terminal or Ghostty tab that runs it, a new tab in the app it last ran in, the editor panel (after opening its folder there), the Cursor chat, or the desktop app.
  - **Copy command** copies the command that resumes an ended chat in a terminal. A chat that still runs, or that you just opened from the page, has none, because that command would start a second copy.
  - The first time, macOS asks to let agnostic-ai control each terminal app.
  - Codex and Cursor chats need the `sqlite3` command.
  - See [`/api/views/conversations`](@/docs/cli-reference/conversations-api.md#get-api-views-conversations).
{% </details> %}

## Context size {#context-size}

Shows how much text each coding tool reads. Choose a tool to see its [context report](@/docs/cli-reference/inspect.md#rank-context-contributions) in words and bytes.

| Table | What it holds |
|-------|---------------|
| **Read at every start** | Startup sources. A words-at-startup bar and **Change budget** sit beside it. |
| **Read for the named file** | File-scoped sources. **Check against a file** updates it as you type. |
| **Read on demand** | Sources loaded only when needed. |

The page compares startup words with [`lint.instructions-words`](@/docs/configuration.md#lint). **Read again** reads your setup files again and shows the change under **Before**. The tool and file stay in the page address.

{% <details summary="Details: sources, duplicates, prompts"> %}
- **How this is counted** explains when text loads and links to the tool's model and effort in Settings.
- Sources are ranked by words, with each source's share of its group. Duplicate startup passages follow.
- **What can I do?** on a source says what it is, why it loads when it does, and what to change.
- **Copy prompt** copies a prompt that asks a coding agent to propose the smallest safe cut and its tradeoffs before editing.
- The page lists **Read at every start**, **Read for the named file** and **Read on demand** tables, with a words-at-startup bar and **Change budget**, which opens the budget docs.
- **Show content** on a row shows the file's frontmatter as chips and its text.
- A source from a built-in names the built-in and links to **Settings**, Built-ins.
- Each duplicate has edit links to its sources and **Copy prompt**.
- **Read again** reads your setup files again and shows the change against the previous counts under **Before**. The tool and file stay in the page address.
{% </details> %}

## Setup health {#setup-health}

Lists what to fix. **Setup health** in the rail shows findings by severity, such as **1 error, 3 warnings**, or **No findings**. On phones, the top bar shows a chip such as **1E 3W**. Either opens the page (`?view=health`).

1. Read **Fix first** and **Worth a look**.
2. Open a finding. **Details** holds **Open** in your editor, **Copy path** and **Recheck**, then the suggested fix, explanation and full prompt.
3. Fix the source, then press **Recheck**.

| Control | What it does |
|---------|--------------|
| **Copy prompt** | Copies a fix prompt for one finding. |
| **Copy all fixes** | With two or more findings, one prompt, errors first. |
| Checkboxes | Select findings or suggestions. A bar shows the count, **Copy prompt** and **Clear**. The prompt lists errors, warnings, then suggestions. |
| **Recheck** beside **Checked just now** | Reruns every section's checks and reports what disappeared, appeared or remains. |

{% <details summary="Details: recheck, global findings, suggestions"> %}
- The page starts with what needs fixing and what needs review. Findings appear by section and item, with the same rows and actions as elsewhere. Click an item or section name to open it.
- **Recheck** on a finding keeps your page, search and unsaved input. It says whether the finding still appears and marks new findings from the same check on the same file.
- A failed refresh or a lint run that stopped early shows as unknown, never as fixed.
- A setup check (`doctor`) finding that disappears shows as not confirmed, because the check skips files it cannot read.
- Snapshots cannot recheck.
- With warnings only, a line under the page title counts them and **Review** opens the folded list at the end of the page. Errors stay visible above the tabs.
- In a project, the counts and **Setup health** include what `lint --global` reports for the global setup (`~/.agnostic-ai`), which reaches every project.
  - Each such finding is labelled **global** and sits under Settings as **Global setup**. Its fix prompt names the file in the global setup.
  - Doctor checks read project files, so the global setup has none.
  - Without a global setup there are none.
  - The JSON overview marks them `"layer": "global"`, with `path` relative to the global source.
- **You could add** lists unused features your enabled tools support: a context kind with no project spec, a hook stage with no matching hook (built-in hooks count), or the disabled `handoff` built-in.
- Each suggestion shows its tool count and **Copy prompt**.
- A row of bars shows supported features and which are in use, such as **7 of 9 features your tools support are in use**. When all are in use, no suggestions appear.
- Suggestions never count as findings, and findings do not lower feature usage.
- User configuration shows no suggestions.
{% </details> %}

## Authority {#authority}

Lists permission files and rules. **Add rules** or **Edit rules** opens an editor for Allow, Ask first, or Always blocked. Preview the source-file change before saving. You can also edit protected paths there, and the default permission mode in the global view. See [permission editing](#permission-editing).

## Tools and MCP servers {#tools}

Lists the MCP server specs, and servers a tool keeps in its own file.

| Item | What it is |
|------|------------|
| Spec rows | MCP server specs of your setup. |
| **Found in tools** | Servers a configured tool keeps in its own MCP file that no spec of the same name covers, such as one you signed in to from inside Claude Code. |
| **Add to your setup** | Previews one spec file, its contents, the variables you must set, and the tools it will reach after sync. **Apply** saves it. |
| **Save new servers for** | Chooses Project or Local for **Add to your setup**. |

{% <details summary="Details: native rows, where saves go"> %}
  - In `view --json`, an MCP item whose spec sets `disabled` carries it in `fields`, and a local override's value wins. A turned-off server is left out of the verdict and rail counts and shown as off.
  - Under **Found in tools**, it also lists servers a configured tool keeps in its own MCP file that no spec of the same name covers, such as one you signed in to from inside Claude Code.
  - Each native row names the server, the tool or tools that hold it, the file (`~/.claude.json`, `.mcp.json`), and the transport. It also shows the `agnostic-ai import <tool>` command that turns the server into a spec (`import --global <tool>` for a user file).
  - A project page reads each tool's project and user files, plus Claude Code's servers for this project under `projects` in `~/.claude.json`. `--global` reads user files only.
  - A native row has no command when no import reads its file, such as Claude Code's per-project servers or an `outputs` MCP file path the tool's import does not follow.
  - Native rows hide the server's own command, arguments, URL, environment, and header values. A file the page cannot read adds no row.
  - The JSON overview carries these rows as `native` on the Tools section.
- **Apply** saves the spec using the [MCP import rules](@/docs/spec-format/mcp-import.md), then asks you to run `agnostic-ai sync` (or `sync --global`).
- Project and Global saves use the project's configured MCP source folder (`.agnostic-ai/mcps/` by default), or `mcps/` in the global home. Local saves use `.agnostic-ai/local/mcps/`, or `local/mcps/` in the home.
- The save stops when the spec name already exists, or when import cannot rewrite a credential.
- Tool files stay as they are, including the original server. Each tool signs in on its own. Snapshots have no **Add to your setup** action.
{% </details> %}

## Lifecycle and hooks {#lifecycle}

Lists hooks by stage. Each stage starts closed, with its timing and hook count. **Show all stages** adds stages with no hooks. **Add hook**, at the end of the toolbar, opens a form.

1. Fill name, stage, tool match (Before and After a tool runs only), command, timeout in seconds and description.
2. Choose **Save for**: Project or Global, or Local.
3. **Preview** shows the one hook file it writes, in the [portable form](@/docs/spec-format/hook-reference.md#portable-events) with only the fields you filled. **Apply** saves it, then run `agnostic-ai sync` (or `sync --global`).

| Control | What it does |
|---------|--------------|
| **Expand all**, **Collapse all** | Control the visible stages. |
| **Command** | Shows the command, hook name and key fields (`match` or `matcher`, `type`, `timeout`, `disabled`). |
| **Full file** | Running server only: every source field. |
| **Copy command** | Copies the command. |

{% <details summary="Details: add-hook saves, redaction, search"> %}
  - Project and Global saves use the project's configured hook source folder (`.agnostic-ai/hooks/` by default), or `hooks/` in the global home. Local saves use `.agnostic-ai/local/hooks/`, or `local/hooks/` in the home, with the same Git-ignore check as other local saves.
  - The save stops when the name is taken, in any layer and in any letter case. It also stops for a hook that `validate` would report, such as a tool match a configured tool lacks.
  - The form cannot edit or delete hooks, or write a tool's own event name. Snapshots and the mock have no **Add hook** action.
- Tool lists use logos, with the full name on hover. Source files appear in compact lists with paths and edit links.
- **Lifecycle** lists stages with hooks. **Show all stages** adds the rest. Events specific to one tool, such as `SubagentStart`, have their own heading and a description of what their hooks do.
- Hooks with `disabled: true` are marked **Turned off** and counted separately.
- Open a stage to see its tools and hooks. Hooks show their purpose and source. Built-ins link to **Change in Settings**.
- Built-in launchers show the `agnostic-ai` call, with **Full command** for the script.
- **Copy command** is hidden when the command appears to contain a credential.
- **Full file** is withheld when the file appears to contain a credential. A sensitive key field shows `<redacted>`.
- Built-in hooks have no source file and show only key fields.
- Search shows and opens the stages with matching hooks. Clearing it closes them. A linked hook or finding opens its stage and receives focus. Open states are not saved.
{% </details> %}

## Settings, model and effort {#settings}

Shows configured fields first and the rest under **Unconfigured fields**, including a default model or reasoning effort. Native settings are grouped by tool, with the `x-<tool>` prefix shown once.

| Field | Where you edit it |
|-------|-------------------|
| Global scalar fields | **Apply** and **Clear**. Session retention and voice dictation work only with `--global`. Voice has an on/off switch, a hold or tap menu and a spoken language field. |
| Default model, reasoning effort | **What each tool gets**: the value sync writes for each configured tool, and the key it comes from. Each tool has its own edit control. |

{% <details summary="Details: what each tool gets"> %}
- Default model and reasoning effort have **What each tool gets**: the value sync writes for each configured tool, and the file or `agnostic-ai.yaml` key it comes from (Claude Code `outputs.claude.settings.model`, Codex `outputs.codex.config`).
  - It also says why each other value loses: a later spec, a local override or clear, a spec scoped to other tools, or a per-tool map with no entry.
  - It names the model tier and alias, says when a tool has no place for the value, and says when sync writes none, without guessing the tool's own default.
  - It reports whether installed files match, not what a running tool uses.
  - It points out an `x-<tool>.model` native key. Other `x-<tool>` keys and imported native settings files are not covered.
  - The JSON overview carries it as `resolved` on those fields.
- Under **What each tool gets**, each tool has its own edit control. It saves `model.<tool>` or `effort.<tool>` in the chosen file, which wins over the value for every tool.
  - The origin line says where the value comes from, such as `local · codex` or `project · all tools` (`global · all tools` in the global view), and names the `agnostic-ai.yaml` key that replaces it when one does.
  - **Use the value for every tool** removes that tool's entry.
  - Effort has a control only for tools that take an effort setting. A spec whose `model` names a model tier stays read-only here, and so does one tool's effort when the tier gives the effort.
{% </details> %}

## Built-in switches and undo {#builtins}

Turn built-ins on or off. In a project, each switch previews the config edit, generated files and findings for every target.

1. Flip the switch. A preview opens.
2. Read it. **Apply** and **Cancel** stay at the top.
3. **Apply** writes the change. **Cancel** discards the preview, so a reload does not bring it back.
4. **Undo last change** restores the files and permissions from before the last apply.

{% <details summary="Details: preview, apply and undo limits"> %}
- **Apply** and **Cancel** stay at the top. Tools, files, and review notes start collapsed. Notes group by tool in a bounded list, with **Copy notes** for the full messages. Pending source changes remain visible.
- **Apply** writes the change. **Cancel** discards the preview, so a reload does not bring it back.
- **Undo last change** restores the files and permissions from before the last apply. Undo survives a browser reload, expires when the server stops, and is replaced by the next apply.
- Turning on `memory` in a Git checkout also adds a `.git/info/exclude` line so Git ignores Claude Code's private `settings.local.json`.
- In a project, a default model or reasoning effort uses **Preview**, **Preview removal**, and **Preview local reset**. The preview names the settings file, the targets, and every source and generated file that changes.
- **Apply** writes those files and checks the generated ones. That does not mean a running tool has reloaded them. Built-ins and settings share one **Undo last change**, which names the change.
- Global built-ins and settings edits save source files without syncing. Run `agnostic-ai sync` (or `sync --global`) to apply them.
- In the global view, if a boolean or built-in changed since the page loaded, the save stops and offers **Reload**.
- Apply needs no earlier sync: the preview lists the generated files, sync state, and lock `.gitignore` entries it creates.
- The preview includes pending source changes when files are missing or match sync's recorded checksums, and undo can restore them.
- These need the CLI: hand edits, files without ownership records, external adapters, paths outside the project, linked skill folders, and memory changes that need fact migration or external storage.
- A changed input or output blocks apply or undo until a fresh preview.
- A failed write restores the files the operation owns. A recovery error names any it kept and points to `agnostic-ai doctor`.
{% </details> %}

## Coding tools {#coding-tools}

**Coding tools** in **Settings** lists tools with a switch, a **Docs** link, and a **found in project** badge when a disabled tool has native files. Snapshots and the mock show the list without switches.

{% <details summary="Details: how a switch saves, turning a tool off"> %}
- A switch saves the complete `targets` list to `agnostic-ai.yaml` in Project or `agnostic-ai.local.yaml` in Local.
- The local list replaces the project list, as built-ins do.
- With `--global`, it saves `agnostic-ai.yaml` or `local/agnostic-ai.yaml` in the home folder and lists only tools global sync supports.
- When no file lists `targets`, the first project change writes the default list with your change.
- Switching the tool back on removes that list only in the same `serve` session, with no intervening file edit. Otherwise, the file keeps the full list.
- Turning a tool off asks for confirmation and explains the next sync.
- In a project, sync removes files it wrote for that tool, backs up hand edits as `.bak`, and keeps unrelated keys in shared settings files.
- With `--global`, files stay until you remove them.
- The last supported tool cannot be turned off. External adapter names do not count.
- After saving, run the displayed `agnostic-ai sync` or `sync --global` command. The page does not sync tool selection.
- A running `sync --watch` applies it unless started with `--target`, `--only`, or `--except`.
- A stale switch stops with **Reload**.
{% </details> %}

## Copy to global {#copy-to-global}

Move or copy a spec between the project and the global setup. Use **Copy to global** on a project rule, agent, skill, hook or MCP spec (**Copy to project** in a global view started inside a project).

1. Open the action. It shows the destination file and whether a spec is already there.
2. Choose **Copy**, which keeps the original, or **Move**, which also deletes it.
3. Save. The page reloads and names the sync commands to run, such as `agnostic-ai sync` and `agnostic-ai sync --global` after a move.

{% <details summary="Details: what cannot be copied"> %}
  - A skill goes with its whole folder.
  - When a spec is already there, both stay off until you tick **Replace it**, which replaces the whole spec.
  - After saving, the page reloads and names the sync commands to run, such as `agnostic-ai sync` and `agnostic-ai sync --global` after a move.
  - Built-ins, pack specs, and local specs have no such action. Commands, review instructions, and settings stay too, since the global setup does not read them as specs.
  - A rule with a scope, paths, or a target condition cannot go to the global setup, where rules apply to every file.
  - When the global setup does not exist yet, create it with **Open global settings** first.
{% </details> %}

## Rail, tabs and editor links {#rail-and-tabs}

The rail names the page and holds the footer links. On phones, the top bar shows the name and **Menu** holds the rail.

| Item | What it does |
|------|--------------|
| **Docs** beside a section heading | Opens the matching spec or reference page in a new tab, including empty sections. |
| **Project** or **Global** label | Shows the page's path under the name on wide screens. On phones, **Menu** shows the path. |
| Version line | Names the agnostic-ai version that serves the page. |
| **Overview JSON** | Opens the overview document behind the page. `Esc` closes it. |
| **agnostic-ai on GitHub** | Opens the agnostic-ai repository in a new tab. |
| GitHub button | Opens the project's repository, when `origin` points at GitHub. |
| **Open global settings** | On a project page, opens the user configuration in a new tab. |
| **Open project** | On a global page that started in a project, goes back. |
| **Settings > Editor** | Picks which editor Open links use: Cursor, VS Code, VS Code Insiders, VSCodium, Zed, or a custom link template with `{path}` and `{line}`. |
| **View and edit** | Beside the filter on Authority and Settings: Project (Global in the global view) or Local values. |

{% <details summary="Details: version range, global setup, tabs, editor links"> %}
- The version line shows the `requires` range when the config sets [`requires`](@/docs/configuration.md#requires). When the running release is outside it, the line uses the warning color and the rail shows **Update needed** beside the version.
- **Overview JSON** shows its endpoint, schema version, revision and the command that prints it (`agnostic-ai view --json`).
- The GitHub button reads SSH and HTTPS remotes and drops any user name or token in the remote URL. Projects with no remote or another host, and the user configuration, show no button.
- **Open global settings** reuses a `serve --global` already running on `127.0.0.1:4310` to `4319` that started in this project, or starts one on the next free port.
- When the user configuration folder does not exist or holds no agnostic-ai files (`agnostic-ai.yaml`, `AGNOSTIC_AI.md`, a spec folder, `local/` or `state/`), the button shows a short panel instead. **Create global setup** then writes an empty `AGNOSTIC_AI.md` and `agnostic-ai.yaml`, makes sure `.gitignore` lists `/local/` (an existing one is kept), runs `git init` when the box is checked, git is installed and the folder is not already in a repository, and opens the global page.
- **Create global setup** never touches a folder that already holds a setup.
- **Open project** shows the project's name. A server the button starts stops with the server that started it. The way back returns to that server, also when it runs on another `--addr`.
- Each browser tab edits only the configuration it serves.
- The editor choice is saved in the browser, not in config files.
- Allow the browser to launch the editor when prompted. If nothing opens, change the editor or copy the path.
- A finding on a file opens at its line when known.
- **Copy path** sits beside each link, because the browser cannot tell whether the editor opened.
- All page links stay visible on narrow screens.
- A page that cannot load shows **Could not load the overview**, or **Could not load the snapshot** for a snapshot, with **Retry**.
- **Context**, **Tools**, and **Settings** use tabs for their source groups. Tools separates MCP servers, servers found in tools, and environments. Settings separates built-ins, coding tools, preferences, config files, and the editor. Config files lists the source files, then shows their combined effective values once.
- Tabs support arrow, Home, and End keys. On phones, Context and Tools tabs scroll horizontally. Settings keeps its tabs visible: five on a running server, four in a snapshot or the mock, which have no Editor tab.
- The selected tab survives refresh and browser history. Switching tabs keeps unsaved input and previews.
- Tab search covers all source tabs and opens a matching group. Linked results open their owning tab.
- Authority keeps permissions together, and Lifecycle keeps its session timeline.
- With `--global`, the `AGNOSTIC_AI.md` row in **Context** has **Copy refine prompt**. The prompt asks a coding agent to review the file against current guidance for instruction files, propose a full rewrite with a change table, recommend which rules belong in an existing or new skill or agent, and leave the file unchanged until you approve.
- **Context size** opens first in **Context** for a project served by a running server. Static snapshots, user configuration, and projects with no coding tool open the first tab with content instead.
- **Save new servers for**, the **View and edit** switch and tool lists: tool lists use logos with the full name on hover, and source files appear in compact lists with paths and edit links.
{% </details> %}

## Save project or local changes {#edit-scope}

**View and edit**, beside the filter at the top of **Settings** and **Authority**, picks where every editor on the page saves: Project (Global in the global view) writes files Git tracks, Local writes git-ignored [local overrides](@/docs/local-overrides.md). The browser remembers your choice.

{% <details summary="Project and local saves in detail"> %}
- It reads **Project | Local** in a project and **Global | Local** in the global view, matching the chip beside the project name.
- **Project**, the default, shows the project's built-in states and settings, and writes files Git tracks.
- **Local** shows your overrides, with values inherited from the project labeled. It writes git-ignored [local overrides](@/docs/local-overrides.md):
  - Built-ins go to `agnostic-ai.local.yaml`.
  - A settings field goes to `.agnostic-ai/local/settings/<name>.yaml`, which overlays the project spec of that name.
- In the global view, **Global** writes `~/.agnostic-ai/`, and **Local** writes `local/agnostic-ai.yaml` and `local/settings/<name>.yaml` there.
- Outputs a local built-in adds go to `.git/info/exclude`, not the project's `.gitignore` block.
- The config snapshot and **What each tool gets** still describe the combined configuration used for sync.

A value from a local file shows a **local** badge in Local. An item from either level's `local/` folder shows the layer `local`. Local values win for you. Rows where they differ show both values, such as **Project: Off · Local: On**. Built-in counts follow the selected view.

A project control blocked by a local override shows its project value, stays disabled, and points to **View local settings**. In Local, **Reset to the project value**, or **Preview local reset** for a project model or effort, removes your override and the local file once it is empty.

Your first local built-in change copies the project list into `agnostic-ai.local.yaml`, since that list replaces the project one whole. **Reset built-ins to the project list** drops it. A local list set through a YAML merge (`<<`) is refused. Edit it by hand.

A local save is refused when its file resolves, through links, outside the local folder, into a project config or source folder, or to a file Git tracks.

Before a local save writes its file, the server checks that Git ignores it.

- When no rule does yet, as in a project that never ran `sync`, it adds the local folder (or `agnostic-ai.local.yaml`) to `.git/info/exclude`, which linked worktrees share. Your other lines in that file stay.
- If Git still would not ignore the file, the save is refused. Add the path to `.gitignore` and save again.
- A new `.git/info/exclude` rule remains if it was added before a refusal or stale conflict, to protect local files.
- Outside a Git repository there is nothing to ignore, so the save writes the file directly.

In a linked worktree, a built-in only you turn on needs `agnostic-ai sync` in the CLI. The first local change keeps the `memory.personal` mode inherited from the main checkout. A reset leaves `memory.personal: checkout` behind rather than switch to the main checkout's mode.
{% </details> %}

## Edit permission rules {#permission-editing}

In **Authority**, pick Project or Local, then **Add rules** or **Edit rules** on Allow, Ask first, or Always blocked. Enter one [permission rule](@/docs/spec-format/settings.md#permission-rules) per line, such as `shell(git status)` or `read(src/**)`. **Preview** shows the exact diff, then **Apply** saves only that source file. Run the displayed `agnostic-ai sync` command to apply it.

{% <details summary="Permission editing in detail"> %}
Blank lines are ignored.

Each saved rule also has **Edit**, which opens the editor on that line, and **Remove**, which goes straight to the preview without that rule.

**Preview** shows the file, scope, and exact diff, with added and removed lines marked. **Apply** saves only that source file. Run the displayed `agnostic-ai sync` or `agnostic-ai sync --global` command to apply it.

The support table names which tools accept permissions.

**Default permission mode** is editable in the global view only, because only `sync --global` writes Claude Code's `defaultMode`. There it offers the mode names Claude Code accepts, and **Save** writes `permissions.default-mode` to the same settings file as the rule lists. A project shows it read-only with **Open global settings**.

**Protected paths** opens an editor with one path per line, a decision (`ask` or `deny`), and a reason.

- **Preview** shows the diff. **Apply** writes the whole [protected block](@/docs/spec-format/settings.md#protected-paths) to that file, replacing the block already there.
- **Remove protected paths** previews removing it.
- A local block starts from the project block merged under it, as sync reads them. It cannot lower a project `deny` to `ask`.
- A local file that sets `protected: null` turns the project paths off for you. Local says so, and **Reset to project paths** removes the key, so the next sync protects them again.
- The global view keeps it read-only, because `sync --global` does not write protected paths.

A local edit starts with the project rules and replaces only the selected list for that spec. Other settings keep inheriting.

- A changed local list offers **Compare project and local**.
- **Reset to the project rules** previews removing the override.
- An empty list is an explicit override with no rules. Resetting removes it and inherits Project again.
- Project editing stays blocked while a local list replaces it.

If a local spec clears the whole group with `permissions: null`, individual lists stay read-only in both views. Project still shows its baseline. Local shows the clear and names the file to edit. Remove that whole-group clear in the local file before editing one list.

When several settings files are editable, choose one explicitly. Lists from other specs still contribute their rules. With no settings spec, the editor creates `settings/permissions.yaml` under the configured source root, or `local/settings/permissions.yaml` for Local. Pack-only sources remain read-only.

Invalid rules, unsafe file paths, and credentials are refused. A changed file stops saving and offers **Reload**, which keeps your draft for a fresh preview. Permission saves have no browser undo.
{% </details> %}

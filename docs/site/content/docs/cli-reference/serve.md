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

In the browser:

- **Docs** beside each section heading opens the matching spec or reference page in a new tab, including empty sections.
- **Authority** lists permission files and rules. **Add rules** or **Edit rules** opens an editor for Allow, Ask first, or Always blocked. Preview the source-file change before saving. You can also edit protected paths there, and the default permission mode in the global view. See [permission editing](#permission-editing).
- **Tools** lists the MCP server specs.
  - In `view --json`, an MCP item whose spec sets `disabled` carries it in `fields`, and a local override's value wins. A turned-off server is left out of the verdict and rail counts and shown as off.
  - Under **Found in tools**, it also lists servers a configured tool keeps in its own MCP file that no spec of the same name covers, such as one you signed in to from inside Claude Code.
  - Each native row names the server, the tool or tools that hold it, the file (`~/.claude.json`, `.mcp.json`), and the transport. It also shows the `agnostic-ai import <tool>` command that turns the server into a spec (`import --global <tool>` for a user file).
  - A project page reads each tool's project and user files, plus Claude Code's servers for this project under `projects` in `~/.claude.json`. `--global` reads user files only.
  - A native row has no command when no import reads its file, such as Claude Code's per-project servers or an `outputs` MCP file path the tool's import does not follow.
  - Native rows hide the server's own command, arguments, URL, environment, and header values. A file the page cannot read adds no row.
  - The JSON overview carries these rows as `native` on the Tools section.
- **View and edit**, shown once beside the filter on Authority and Settings, switches between Project (Global in the global view) and Local values. **Save new servers for** above the native server list chooses Project or Local for **Add to your setup**.
- **Add to your setup** on a native-only server previews one spec file, its contents, the variables you must set, and the tools it will reach after sync.
- **Apply** saves the spec using the [MCP import rules](@/docs/spec-format/mcp-import.md), then asks you to run `agnostic-ai sync` (or `sync --global`).
- Project and Global saves use the project's configured MCP source folder (`.agnostic-ai/mcps/` by default), or `mcps/` in the global home. Local saves use `.agnostic-ai/local/mcps/`, or `local/mcps/` in the home.
- The save stops when the spec name already exists, or when import cannot rewrite a credential.
- Tool files stay as they are, including the original server. Each tool signs in on its own. Snapshots have no **Add to your setup** action.
- **Add hook**, at the end of the Lifecycle toolbar, opens a form: name, stage, tool match (for Before a tool runs and After a tool runs only), command, timeout in seconds, description, and **Save for** (Project or Global, or Local). **Preview** shows the one hook file it writes, in the [portable form](@/docs/spec-format/hook-reference.md#portable-events) with only the fields you filled. **Apply** saves it and asks you to run `agnostic-ai sync` (or `sync --global`).
  - Project and Global saves use the project's configured hook source folder (`.agnostic-ai/hooks/` by default), or `hooks/` in the global home. Local saves use `.agnostic-ai/local/hooks/`, or `local/hooks/` in the home, with the same Git-ignore check as other local saves.
  - The save stops when the name is taken, in any layer and in any letter case. It also stops for a hook that `validate` would report, such as a tool match a configured tool lacks.
  - The form cannot edit or delete hooks, or write a tool's own event name. Snapshots and the mock have no **Add hook** action.
- **Copy to global** on a project rule, agent, skill, hook, or MCP spec (**Copy to project** in a global view started inside a project) shows the destination file and whether a spec is already there.
  - **Copy** keeps the original. **Move** also deletes it.
  - A skill goes with its whole folder.
  - When a spec is already there, both stay off until you tick **Replace it**, which replaces the whole spec.
  - After saving, the page reloads and names the sync commands to run, such as `agnostic-ai sync` and `agnostic-ai sync --global` after a move.
  - Built-ins, pack specs, and local specs have no such action. Commands, review instructions, and settings stay too, since the global setup does not read them as specs.
  - A rule with a scope, paths, or a target condition cannot go to the global setup, where rules apply to every file.
  - When the global setup does not exist yet, create it with **Open global settings** first.
- **Conversations** (`/`) is the page the interface opens on. A static snapshot has no server to read chats, so it opens on **Context** instead.
  - It lists recent Claude Code, Codex and Cursor chats from each tool's own store.
  - Open chats come first under **Open now**, with **Working** or **Waiting for you** when Claude Code reports it. The rest follow by day, newest last message first.
  - It loads the past week. It loads the week before each time you scroll to the end, or with **Load the week before**.
  - The tool filter beside the search shows one tool's chats. Search covers every conversation, loaded or not.
  - Each row's action reopens the chat where it lives: the Terminal or Ghostty tab that runs it, a new tab in the app it last ran in, the editor panel (after opening its folder there), the Cursor chat, or the desktop app.
  - The first time, macOS asks to let agnostic-ai control each terminal app.
  - Codex and Cursor chats need the `sqlite3` command.
  - See [`/api/views/conversations`](@/docs/cli-reference/serve-api.md).
- On wide screens, the rail labels the page **Project** or **Global** and shows its path under the name. On phones, the top bar shows the name and **Menu** shows the path.
- The foot of the rail (in **Menu** on phones) names the agnostic-ai version that serves the page. When the config sets [`requires`](@/docs/configuration.md#requires), the line under it names that range. When the running release is outside that range, the line uses the warning color and the rail shows **Update needed** beside the version.
- **Overview JSON** at the foot of the rail (in **Menu** on phones) opens the overview document behind the page, with its endpoint, schema version, revision, and the command that prints it (`agnostic-ai view --json`). `Esc` closes it.
- **agnostic-ai on GitHub** at the foot of the rail (in **Menu** on phones) opens the agnostic-ai repository in a new tab.
- When the project's `origin` remote points at GitHub, a GitHub button at the foot of the rail (in **Menu** on phones) opens that repository in a new tab. It reads SSH and HTTPS remotes and drops any user name or token in the remote URL. Projects with no remote or another host, and the user configuration, show no button.
- On a project page, **Open global settings** at the foot of the rail (in **Menu** on phones) opens the user configuration in a new tab. It reuses a `serve --global` already running on `127.0.0.1:4310` to `4319` that started in this project, or starts one on the next free port.
- When the user configuration folder does not exist or holds no agnostic-ai files (`agnostic-ai.yaml`, `AGNOSTIC_AI.md`, a spec folder, `local/` or `state/`), the button shows a short panel instead. **Create global setup** then:
  - Writes an empty `AGNOSTIC_AI.md` and `agnostic-ai.yaml`.
  - Makes sure `.gitignore` lists `/local/` (an existing one is kept).
  - Runs `git init` when the box is checked, git is installed and the folder is not already in a repository.
  - Opens the global page.

  It never touches a folder that already holds a setup.
- A global page that started in a project (from that button, or `ui --global` or `serve --global` run in a project folder) shows **Open project** with the project's name to go back.
- A server the button starts stops with the server that started it. The way back from that global page returns to that server, also when it runs on another `--addr`.
- Each browser tab edits only the configuration it serves.
- **Settings > Editor** picks which editor Open links use: Cursor, VS Code, VS Code Insiders, VSCodium, Zed, or a custom link template with `{path}` and `{line}`.
  - The choice is saved in the browser, not in config files.
  - Allow the browser to launch the editor when prompted. If nothing opens, change the editor or copy the path.
  - A finding on a file opens at its line when known.
  - **Copy path** sits beside each link, because the browser cannot tell whether the editor opened.
- All page links stay visible on narrow screens.
- A page that cannot load shows **Could not load the overview**, or **Could not load the snapshot** for a snapshot, with **Retry**.
- **Context**, **Tools**, and **Settings** use tabs for their source groups. Tools separates MCP servers, servers found in tools, and environments. Settings separates built-ins, coding tools, preferences, config files, and the editor. Config files lists the source files, then shows their combined effective values once.
- Tabs support arrow, Home, and End keys. On phones, Context and Tools tabs scroll horizontally. Settings keeps its tabs visible: five on a running server, four in a snapshot or the mock, which have no Editor tab.
- The selected tab survives refresh and browser history. Switching tabs keeps unsaved input and previews.
- Search covers all source tabs and opens a matching group. Linked results open their owning tab.
- Authority keeps permissions together, and Lifecycle keeps its session timeline.
- With `--global`, the `AGNOSTIC_AI.md` row in **Context** has **Copy refine prompt**. The prompt asks a coding agent to review the file against current guidance for instruction files, propose a full rewrite with a change table, recommend which rules belong in an existing or new skill or agent, and leave the file unchanged until you approve.
- **Context size** opens first in **Context** for a project served by a running server. Static snapshots, user configuration, and projects with no coding tool open the first tab with content instead.

Choose a coding tool to see its [context report](@/docs/cli-reference/inspect.md#rank-context-contributions): startup, file-scoped, and on-demand sizes in words and bytes. **Check against a file** updates the file scope as you type. The page compares startup words with [`lint.instructions-words`](@/docs/configuration.md#lint).

**How this is counted** explains when text loads and links to the tool's model and effort in Settings. Sources are ranked by words, with each source's share of its group. Duplicate startup passages follow. The tool and file stay in the page address.

- **What can I do?** on a source says what it is, why it loads when it does, and what to change.
- **Copy prompt** copies a prompt that asks a coding agent to propose the smallest safe cut and its tradeoffs before editing.
- The page lists **Read at every start**, **Read for the named file** and **Read on demand** tables, with a words-at-startup bar and **Change budget**, which opens the budget docs.
- **Show content** on a row shows the file's frontmatter as chips and its text.
- A source from a built-in names the built-in and links to **Settings**, Built-ins.
- Each duplicate has edit links to its sources and **Copy prompt**.
- **Read again** reads your setup files again and shows the change against the previous counts under **Before**. The tool and file stay in the page address.

- Each finding has **Copy prompt**. **Details** holds **Open** in your editor, **Copy path** and **Recheck**, then the suggested fix, explanation, and full prompt.
- After you fix the source, **Recheck** runs the checks again with the same refresh and keeps your page, search, and unsaved input. It says whether that finding still appears and marks new findings from the same check on the same file. A failed refresh or a lint run that stopped early shows as unknown, never as fixed.
- A setup check finding (from `doctor`) that disappears is shown as not confirmed, because the setup check skips files it cannot read. Snapshots cannot recheck.
- **Setup health** in the rail shows findings by severity, such as **1 error, 3 warnings**, or **No findings**. On phones, the top bar shows a chip such as **1E 3W**. Either opens **Setup health** (`?view=health`).

The health page starts with what needs fixing and what needs review. Findings appear under **Fix first** and **Worth a look**, then by section and item, with the same rows and actions as elsewhere. Click an item or section name to open it. With two or more findings, **Copy all fixes** copies one prompt, errors first.

Select findings or suggestions with their checkboxes. A bar at the foot of the page shows the count, **Copy prompt**, and **Clear**. The copied prompt lists errors, warnings, then suggestions. **Recheck**, beside **Checked just now**, reruns every section's checks and reports findings that disappeared, appeared, or remain. Doctor and incomplete lint results use the limits described above.

**You could add** lists unused features your enabled tools support: a context kind with no project spec, a hook stage with no matching hook (built-in hooks count), or the disabled `handoff` built-in. Each suggestion shows its tool count and **Copy prompt**. A row of bars below the opening sentence shows supported features and which are in use, such as **7 of 9 features your tools support are in use**. When all are in use, no suggestions appear. Suggestions never count as findings, and findings do not lower feature usage. User configuration shows no suggestions.

- In a project, the counts and **Setup health** include what `lint --global` reports for the global setup (`~/.agnostic-ai`), which reaches every project.
  - Each such finding is labelled **global** and sits under Settings as **Global setup**. Its fix prompt names the file in the global setup.
  - Doctor checks read project files, so the global setup has none.
  - Without a global setup there are none.
  - The JSON overview marks them `"layer": "global"`, with `path` relative to the global source.
- With warnings only, a line under the page title counts them and **Review** opens the folded list at the end of the page. Errors stay visible above the tabs with their findings.
- Tool lists use logos, with the full name on hover. Source files appear in compact lists with paths and edit links.
- **Lifecycle** lists stages with hooks. **Show all stages** adds the rest. Events specific to one tool, such as `SubagentStart`, have their own heading and a description of what their hooks do.

Each stage starts closed, with its timing and hook count. Hooks with `disabled: true` are marked **Turned off** and counted separately. Open a stage to see its tools and hooks. **Expand all** and **Collapse all** control the visible stages. Hooks show their purpose and source. Built-ins link to **Change in Settings**.

**Command** shows the command, hook name, and key fields (`match` or `matcher`, `type`, `timeout`, and `disabled`). Built-in launchers show the `agnostic-ai` call, with **Full command** for the script. **Copy command** is hidden when the command appears to contain a credential.

On a running server, **Full file** shows every source field, unless the file appears to contain a credential. A sensitive key field shows `<redacted>`. Built-in hooks have no source file and show only key fields.

- Search shows and opens the stages with matching hooks. Clearing it closes them. A linked hook or finding opens its stage and receives focus. Open states are not saved.
- Native settings are grouped by tool, with the `x-<tool>` prefix shown once.
- **Settings** shows configured fields first and the rest under **Unconfigured fields**, including a default model or reasoning effort.
- Global scalar fields offer **Apply** and **Clear**. Session retention and voice dictation are editable only with `--global`. Voice has an on/off switch, a hold or tap menu, and a spoken language field.
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
- In a project, each built-in switch previews the config edit, generated files, and findings for every target.
- **Apply** and **Cancel** stay at the top. Tools, files, and review notes start collapsed. Notes group by tool in a bounded list, with **Copy notes** for the full messages. Pending source changes remain visible.
- **Apply** writes the change. **Cancel** discards the preview, so a reload does not bring it back.
- **Undo last change** restores the files and permissions from before the last apply. Undo survives a browser reload, expires when the server stops, and is replaced by the next apply.
- Turning on `memory` in a Git checkout also adds a `.git/info/exclude` line so Git ignores Claude Code's private `settings.local.json`.
- In a project, a default model or reasoning effort uses **Preview**, **Preview removal**, and **Preview local reset**. The preview names the settings file, the targets, and every source and generated file that changes.
- **Apply** writes those files and checks the generated ones. That does not mean a running tool has reloaded them. Built-ins and settings share one **Undo last change**, which names the change.
- **Coding tools** in **Settings** lists tools with a switch, a **Docs** link, and a **found in project** badge when a disabled tool has native files. Snapshots and the mock show the list without switches.

A switch saves the complete `targets` list to `agnostic-ai.yaml` in Project or `agnostic-ai.local.yaml` in Local. The local list replaces the project list, as built-ins do. With `--global`, it saves `agnostic-ai.yaml` or `local/agnostic-ai.yaml` in the home folder and lists only tools global sync supports.

When no file lists `targets`, the first project change writes the default list with your change. Switching the tool back on removes that list only in the same `serve` session, with no intervening file edit. Otherwise, the file keeps the full list.

Turning a tool off asks for confirmation and explains the next sync. In a project, sync removes files it wrote for that tool, backs up hand edits as `.bak`, and keeps unrelated keys in shared settings files. With `--global`, files stay until you remove them. The last supported tool cannot be turned off. External adapter names do not count.

After saving, run the displayed `agnostic-ai sync` or `sync --global` command. The page does not sync tool selection. A running `sync --watch` applies it unless started with `--target`, `--only`, or `--except`. A stale switch stops with **Reload**.

- Global built-ins and settings edits save source files without syncing. Run `agnostic-ai sync` (or `sync --global`) to apply them.
- In the global view, if a boolean or built-in changed since the page loaded, the save stops and offers **Reload**.

Apply and its preview:

- Apply needs no earlier sync: the preview lists the generated files, sync state, and lock `.gitignore` entries it creates.
- The preview includes pending source changes when files are missing or match sync's recorded checksums, and undo can restore them.
- These need the CLI:
  - hand edits
  - files without ownership records
  - external adapters
  - paths outside the project
  - linked skill folders
  - memory changes that need fact migration or external storage
- A changed input or output blocks apply or undo until a fresh preview.
- A failed write restores the files the operation owns. A recovery error names any it kept and points to `agnostic-ai doctor`.

Edits keep unrelated text and private file permissions. A validation failure puts the file back as it was, unless another process changed the file. Then the save fails with an error that says the file could not be restored.

### Save project or local changes {#edit-scope}

- **View and edit**, beside the filter at the top of **Settings** and **Authority**, selects the values shown and where every editor on the page saves.
- It reads **Project | Local** in a project and **Global | Local** in the global view, matching the chip beside the project name.
- The browser remembers your choice.
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

### Edit permission rules {#permission-editing}

In **Authority**, select Project or Local, then **Add rules** or **Edit rules** on Allow, Ask first, or Always blocked. Enter one [permission rule](@/docs/spec-format/settings.md#permission-rules) per line, such as `shell(git status)` or `read(src/**)`. Blank lines are ignored.

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

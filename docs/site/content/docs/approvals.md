+++
title = "Approvals"
description = "Choose who answers permission prompts in your coding tools."
weight = 73

[extra]
group = "Workflows"
+++

# Approvals

Choose how your tools handle permission prompts. `ask` keeps the tool's prompts, `review` uses its own reviewer, and `yolo` lets an agnostic-ai hook answer one call at a time. Check [which modes your tool supports](#per-tool).

## What the gate does not catch

Use `yolo` only in a sandbox, container, or throwaway worktree. The hook checks the tool name and input. It cannot see what an allowed command does after it starts.

- A script the agent just wrote can do anything when it runs as `./script.sh`.
- `make test`, `npm test`, and `go test` run the project's own scripts or tests.
- A recursive search such as `grep -r` can print a credential file inside the project.

## Turn it on

Add the built-in and mode to your [home config](@/docs/global-configuration.md), `$AGNOSTIC_AI_HOME/agnostic-ai.yaml`. This example enables the hook for Claude Code and Codex.

```yaml
targets: [claude, codex]
builtins: [approvals]
approvals:
  mode: yolo
```

```bash
agnostic-ai sync --scope user
agnostic-ai approvals status
```

A user-level sync installs the `PermissionRequest` hook. Each tool's [setup and limits](#per-tool) still apply. The hook reads the mode on every request. Changing it needs no sync for the hook to respond.

### Change it from the companion {#change-it-from-the-companion}

Open the companion list's ⋯ menu and choose a mode under **When a tool asks permission**: **Ask me**, **Let the tool's reviewer decide** or **Approve routine calls for me**. The two looser modes ask once. The menu writes the mode in the file the [settings reference](@/docs/global-configuration-reference.md#global-approvals) names, and adds `approvals` to `builtins` when it is missing.

- A status line under the rows says why a tool still asks, how many tools and folders have their own mode, and which shared keys a local block hides.
- **Not in effect yet** means a sync would put the setting in effect, such as on a machine without the hook. **Apply to my tools** lists every file `agnostic-ai sync --scope user` would change, including changes that are not about approvals, and every hook and MCP server command your tools run after it (with its environment and folder, or the server's address), then runs it after you confirm. Check the commands: a hook another program added shows there before it installs. After it writes the Codex hooks file, open `/hooks` in Codex once and trust it.
- A change made in the home config or another page shows in the open menu within 2 seconds. When the server cannot be read, the approvals rows hide and the rest of the menu works.

### Change it from the web page {#change-it-from-the-web-page}

Run `agnostic-ai view --scope user` and open **Settings**, then **Approvals and companion**. It shows the same modes, switches, status lines and **Apply to my tools** as the companion menu, plus each tool's mode and the mailbox switch. A tab reads the settings every 2 seconds while it is visible, so a change made in the companion shows there too.

Its **Companion** group holds the companion's Animate, Dim when idle, Small companion and Check for updates switches. A running companion applies a change within 5 seconds. Turning off Check for updates needs an unlocked tab. While the companion runs, the group also shows both shortcuts and Start at login, read only; change those from the companion's menu. A shortcut another program writes into `companion.json` is put back.

Only a tab that `view` opened can loosen approvals. `view` opens it through a file only your account can read, not through the address. The companion's **Dashboard** opens an unlocked tab the same way. A tab you open by typing the address or a bookmark can make approvals stricter but shows the looser choices locked. To unlock one, run `agnostic-ai view --scope user` again. It reuses the running server, the companion's included, and opens a new unlocked tab. If your system opens `.html` files in an editor, open the address `view` prints instead; that tab stays locked. On Windows no unlock file is written, so every tab stays locked: loosen approvals from the companion. See [who can change this](#who-can-change-this).

## Modes

- **`ask`** is the default. Your tool shows its own prompts.
- **`review`** uses your tool's own reviewer and its model calls. `sync --scope user` turns it on in the tool's user settings. The hook stays silent in this mode, so the tool's reviewer can answer. See [settings for each reviewer](@/docs/global-configuration-reference.md#global-approvals-review).
- **`yolo`** lets the hook approve one call at a time. Each approval applies only to that call. The hook leaves the tool's mode, rules, and call input as they are. It adds about 11 ms per prompt in a macOS test.

## Settings

```yaml
approvals:
  mode: ask
  tools:
    codex: ask
  projects:
    ~/Code/sandbox: yolo
  always-ask:
    - "Bash(make deploy*)"
```

- To stop hook approvals, set `AGNOSTIC_AI_APPROVALS=ask` for the session or `approvals.mode: ask` in the project's `agnostic-ai.yaml` or `agnostic-ai.local.yaml`. Otherwise, the home config uses `projects`, `tools`, and `mode` in that order. See [how to turn off a native reviewer](#turn-it-off-fast).
- A project may only set `approvals.mode: ask`. Project sync installs no approvals hook, and `builtins: [approvals]` in a project has no effect. `validate` reports ignored approvals config and built-in entries.
- `always-ask` adds patterns to the fixed rules below. Use Claude Code's rule form for every tool, as in `Bash(make deploy*)`.

See the [approvals settings reference](@/docs/global-configuration-reference.md#global-approvals).

## What always asks

Publishing, network access, destructive commands, credentials, and edits to tool policy still ask. The tool's deny rules take precedence. A match in `always-ask` can only add a prompt.

{% <details summary="Commands, files, and rules that always ask"> %}
| Request | Why |
|---|---|
| A shell command outside the known programs and options | The hook cannot tell what it does |
| Command substitution, `eval`, `bash -c`, here-documents, `xargs`, or inline code such as `python -c` | The hook cannot read the command in full |
| Deleting or overwriting with `rm`, `git reset --hard`, `git clean`, `git checkout --`, `git restore`, `git branch -D`, or redirection outside the project | Hard to undo |
| Publishing with `git push`, `gh` writes, `npm publish`, `docker push`, or release scripts | Sends changes elsewhere |
| Network or install commands such as `curl`, `wget`, `ssh`, `npm install <name>`, `pip install`, and `go install` | Reaches other hosts |
| Access to `~/.ssh`, `~/.aws`, `.env*`, `*.pem`, `*.key`, or a link to a credential file. Commands such as `env`, `printenv`, and `gh auth token` also ask | Can expose a credential |
| `sudo`, `chmod -R`, `chown`, `kill`, `launchctl`, and `crontab` | Changes the system |
| Reads or edits outside the project, or any call in a project rooted at or above your home folder. Outside Git, the session folder is the project root | Outside the folder you work in |
| Reads of `.git`, `.gitconfig`, or `.config/git/` | Git config can hold a token in a remote URL |
| Edits to Git config, `.git/`, `.github/workflows/`, shell profiles, tool settings and hooks, `agnostic-ai.yaml`, or anything under `$AGNOSTIC_AI_HOME` | Could change what the hook trusts |
| Edits to tool, login agent, or `bin` folders under your home, even when the session runs there, such as `~/.claude`, `~/Library/LaunchAgents`, and `~/go/bin` | Could change how tools run |
| Edits to agent, hook, MCP, or settings specs under `.agnostic-ai/` or in folders set by `sources` in a config above the edited file. An unreadable config or one that names its own folder as a source makes every edit ask | Sync can turn these specs into hooks, servers, or permission modes, including an agent's `x-claude` `hooks` or `permissionMode` |
| Commands that name `AGNOSTIC_AI_` settings | Could change the policy |
| MCP tools, web fetch and search, or tools such as `AskUserQuestion` | The hook has no check for these tools |
| A match in `always-ask` | Your own rule |

Denied calls never reach the hook. Project sync also enforces a settings spec's [protected paths](@/docs/spec-format/settings.md#protected-paths). Claude Code gets permission rules the hook reads as `ask` rules. Codex gets a hook that blocks the edit.
{% </details> %}

## What you see in the companion

After the first approval, the [chat row shows its approval count](@/docs/cli-reference/companion.md#approvals). Codex skips the prompt for an approved call. Claude Code can show it briefly, then closes it when the hook approves. Calls that still need you show **Needs permission**.

## Answer from the companion {#answer-from-the-companion}

Turn this on to answer a Claude Code or Codex prompt from the companion row, without switching to the terminal. It is off by default.

```yaml
# $AGNOSTIC_AI_HOME/agnostic-ai.yaml or its local/ file
builtins: [approvals]
approvals:
  mode: ask
  companion: true
```

Run `agnostic-ai sync --scope user` once, then open the companion. Or turn on **Approve and deny from this list** in the companion's ⋯ menu, which offers the sync for you. When Claude Code asks to run `touch a.txt`, the row shows the whole command with **Approve** and **Deny**. **Approve** runs that one call. **Deny** blocks it, and Claude Code reads "Denied in the agnostic-ai companion". The terminal prompt stays open the whole time, so you can still answer there.

- The hook waits for a click only when the request is left to you: mode `ask`, a match in the always-ask rules in `yolo`, or a prompt in `review`. A request the hook approves never waits.
- It waits only while the companion is open, and only for calls it can show in full: `Bash`, `Read`, `Write`, `Edit` and `MultiEdit`, and Codex `apply_patch` with the whole patch, under 8 KB, with nothing that looks like a credential. A call with an option the row does not show, such as a command run outside the sandbox or an edit that replaces every match, keeps the terminal prompt alone, and so do MCP tools, web tools and questions.
- The row shows the request from the hook's own copy, never from the chat. A click answers at once; no key does. When the request changed before the click, the row says "This request changed. Check it again." and shows the new one.
- Claude Code: the hook waits up to 300 seconds, then leaves the request to the terminal prompt. An answer in the terminal removes the buttons by the next list read. When Claude Code then asks again, the row offers only the new request, and a click on the earlier one answers "This request changed". When two held requests could be the one that waits, the row shows no buttons and the terminal prompt answers.
- Codex shows its prompt only after the hook ends. So the hook waits at most 60 seconds, the terminal shows "Waiting for your answer in the agnostic-ai companion, then asks here", and the row adds **Answer in terminal**, which shows the Codex prompt at once. Esc in the terminal stops the turn, and the row drops the request. The row counts under **Needs you** from the moment the hook holds the request.
- After this update and `sync --scope user`, Codex shows **Hooks need review** once, because the approvals hook changed, whether or not you use the companion. Trust it to keep hook approvals.
- Every request shows the folder it runs in, where its relative paths start. A command also shows whether it runs in the background and its time limit. A request without a folder, or with an option of a shape the row cannot show, keeps the terminal prompt alone.
- Only Claude Code and Codex on macOS and Linux support it. Codex in `review` never waits, so its reviewer is not delayed. Kiro, Qwen Code and Qoder keep their prompts as they are.
- Only the home config can turn it on. A project file that sets `approvals.companion` is ignored and reported.
- The companion never approves on its own. A click sends the request's id and a digest of what the row showed; the hook acts only when both match the request it holds, and the server checks again that the row still shows that request.
- Only the companion that started the server can answer a request or see it. The companion makes a new secret each launch and hands it to the server in its environment, never on the command line, and the server drops it so no program it starts inherits it. Any program on the machine, under any account, can reach the server's local port; without the secret it gets 403 for an answer, and the list it reads carries no held request. A server started without the companion has no answer route. [What the secret does not stop](#who-can-change-this) applies here too.
- A held request is saved for the wait in `$AGNOSTIC_AI_HOME/local/approvals/pending/`, which only you can read. A program that runs as you can already write there, or change your tool settings; this feature does not change that limit.
- Each held request leaves one log line with `by: companion`: `approved`, `denied`, or `asked` with rule `companion` when no click came. These lines do not count in **N approved for you**.

## Turn it off fast

Any of these stops hook approvals from the next request, without syncing or restarting.

1. Run `agnostic-ai approvals ask`, or select **Ask me** on the approvals chip or in the ⋯ menu of the companion list. Each writes `approvals.mode: ask` to `$AGNOSTIC_AI_HOME/local/agnostic-ai.yaml`.
2. Set `AGNOSTIC_AI_APPROVALS=ask` for a session or `approvals.mode: ask` in the project's config.

To remove the hooks, drop `approvals` from `builtins` and run `sync --scope user`. A user-level sync also removes the native reviewer settings it wrote when you turn off `review`. Switch modes separately in any running reviewer session, for example with Shift+Tab in Claude Code.

## Who can change this {#who-can-change-this}

Any program on the machine can reach the server's local port, including an agent that runs with automatic approvals. So a change that makes approvals less strict needs proof that you asked for it. A change that makes them stricter does not.

- Stricter, from any page of the server's own origin: `ask`, turning off **Approve and Deny** in the list, and turning approval of agent messages back on.
- Less strict, only from the companion or a tab that `view` opened: `review`, `yolo`, turning on **Approve and Deny** in the list, turning off approval of agent messages, and running a sync from a page. So does any change other than `ask` to an approvals block that does not load: fixing it could turn on the `yolo` it holds. Any other request gets 403 and no file changes.
- The companion proves it with the secret it hands its server at launch. A web tab proves it with a key the server draws at start and keeps in memory. `view` writes a file only your account can read, in an `agnostic-ai/unlock` folder under your user config folder, and opens it. The file sends the browser to the server with a one-time code, and the page trades the code for the key and keeps it for that tab only. Each code works once and its file goes away after use or after 60 seconds. Up to 8 codes wait at once, so a second `view` run does not end the first.
- This proves the request comes from your account, not that you clicked. A program running as you can read the file too, as it can edit your home config directly.
- On Windows a file mode does not keep other accounts out, so no unlock file is written there. A page left on `127.0.0.1` by an earlier program can install a service worker that watches later pages on the same address. A tab under one removes it and stays locked, but a worker that serves its own pages can still read the code, including the code of a tab the companion's **Dashboard** opens. If another program served pages on these ports, clear the site data for `127.0.0.1` in your browser. A browser extension that can read pages on `127.0.0.1` can read the tab's key, as it can read anything else on the page.
- Typing into a chat with `POST /api/conversations/send` needs the companion's secret or a tab's key too, since it types into another session's terminal as you. Without it the answer is 403 and nothing is typed. An open that would run a command in a terminal, such as resuming an ended terminal session, needs it too; without it no command runs and the reply gives the command to run by hand. Bringing a live tab to the front, or opening a chat in an app, an editor or a browser, needs no proof.
- Deleting an archived conversation with `POST /api/conversations/delete`, stopping a session or a stuck shell with `POST /api/conversations/stop-session` or `POST /api/conversations/stop-shell`, and renaming a Claude Code cloud session or a ChatGPT chat with `POST /api/conversations/rename` need the same proof, since each removes data, ends a process or uses your sign-in. Without it the answer is 403 and nothing changes. Renaming a conversation kept on this computer needs no proof.
- Adding a user-level hook or MCP server with the page's add forms needs the same proof, since it runs commands as you after the next sync. A hook or server copied from a project is not checked when copied; its command shows in the next **Apply to my tools**.
- The companion's server, the web page's server and `agnostic-ai approvals ask` take turns on one lock file, `local/approvals/write.lock`, so a check of the shown value and its write happen together.
- Each less strict change and each sync from a page adds a log line with `decision: changed` and `by: companion` or `by: web`.
- The same secret guards each [mailbox](@/docs/mailbox.md#approve-agent-messages) action recorded as yours, such as approving a held agent message. Those are recorded in the mailbox, not this log.

The secret stops web pages, other accounts on the machine, and agents that only use the local approval and mailbox routes. It does not stop code that runs as you and sets out to forge your answers: such code can start its own server with its own secret, or edit the files under `$AGNOSTIC_AI_HOME` directly.

The home config stays the place to change anything by hand. See the [settings routes](@/docs/cli-reference/serve-api.md#post-api-settings).

## The decision log

In `yolo`, the hook logs each request to `$AGNOSTIC_AI_HOME/local/approvals/decisions.jsonl`. It also logs `asked` with rule `mode` when a session sets `AGNOSTIC_AI_APPROVALS=ask` while home config allows automatic answers. The companion needs the record because it cannot read the session's environment.

`agnostic-ai approvals log` prints oldest first. Use `--since 1h` for the last hour or `--json` for an array.

{% <details summary="Log fields and rotation"> %}
Each line has `time`, `tool`, `session`, `cwd`, `call` (tool name), `detail`, `mode`, `decision` (`approved`, `asked`, `denied`, or `changed` for a settings change made from the companion), `rule` (what made it ask), and `by` (`companion` for a request held for the companion). The detail leaves out text that might hold a credential. For Codex `apply_patch`, it lists the affected files.

At 5 MB the log rotates to `decisions.jsonl.1`. It keeps one older file and asks if it cannot write.
{% </details> %}

## What can go wrong

- A repository can supply its own hooks once you trust it in Claude Code. Only home config can enable agnostic-ai approvals. If project environment settings stop the hook, see your tool below.
- Edits to the policy or log and commands that name them ask. A script can still write to them indirectly.
- On failure the hook exits 0 and stays silent, so the tool prompts. A hung hook can delay the Codex prompt by up to 70 seconds, the time Codex gives the hook before it shows its prompt anyway. The hook can approve a call or leave it to the tool. It denies a call only after a click on **Deny** in the companion.
- The hook reads only the tool name and arguments. Conversation text, including files and web pages, has no say in its decision.

## Per tool {#per-tool}

| Tool | `review` | `yolo` | Needs |
|---|---|---|---|
| Claude Code (`claude`) | Claude Code auto mode | PermissionRequest hook | Claude Code 2.0.45 or later, with workspace trust |
| CodeBuddy (`codebuddy`) | CodeBuddy auto mode | asks | |
| Codex (`codex`) | Codex Auto-review | PermissionRequest hook | Trust the hook in `/hooks`. Auto-review needs an `approval_policy` that prompts. |
| Cursor (`cursor`) | cursor-agent Auto-review | asks | A classifier-capable model and team permission for cursor-agent. Set Auto-review in the desktop app by hand. |
| Devin CLI (`windsurf`) | Devin smart mode | asks | Your `deny` and `ask` rules come first. `--permission-mode` and `DEVIN_PERMISSION_MODE` replace the mode for one launch. |
| Kiro (`kiro`) | Kiro Approve for me | PermissionRequest hook | For `review`, IDE or interactive CLI V3 2.36 or later where offered. For `yolo`, CLI V3 3.4.0 or later or IDE 1.4.236 or later. |
| Mistral Vibe (`vibe`) | Vibe smart-approve | asks | `bypass_tool_permissions` off |
| Qoder (`qoder`) | Qoder auto mode | PermissionRequest hook | With `yolo`, no `permissions.ask` rule |
| Qwen Code (`qwen`) | Qwen Code auto mode | PermissionRequest hook | With `yolo`, the hook needs `default` or `auto-edit` and no `permissions.ask` rule. |
| Every other tool | asks | asks | |

On Windows, `yolo` always asks because the hook has no Windows command. `agnostic-ai approvals status` reports how each tool handles prompts and why.

For `yolo`, accept Claude Code's workspace trust prompt and trust Codex's hook in `/hooks`. Check the limits below for your tool.

{% <details summary="Claude Code hook settings and ask rules"> %}
- Sync writes `~/.claude/settings.json`. Accept the workspace trust dialog before Claude Code will run hooks in that folder.
- The hook answers in manual and accept-edits modes. It stays silent in auto and plan, when the reviewer or a rule already chose to ask. Other modes were untested.
- Native `ask` rules still apply. The hook checks the written path and its resolved links, ignoring letter case. A `Read` rule with a path makes Grep, Glob, and LS ask. A `Read` or `Edit` rule with a path makes every shell command ask. For `rtk`, the hook checks the wrapped command.
- It reads `~/.claude/settings.json` (or the one under `$CLAUDE_CONFIG_DIR`), `.claude/settings.json` and `.claude/settings.local.json` in the session folder and its parents, the main checkout of a linked worktree, and the managed settings file and `managed-settings.d/`. A parse error makes every call ask. Copy patterns supplied through `--settings`, `--allowedTools`, a device profile, or the claude.ai console to `always-ask`, since the hook cannot read those sources.
- A project settings `env` block that sets `AGNOSTIC_AI_HOME`, `HOME`, `CLAUDE_CONFIG_DIR`, or `USERPROFILE` makes the hook ask because those variables can redirect its policy.
{% </details> %}

{% <details summary="Codex hook trust and native approval rules"> %}
- Sync writes `~/.codex/hooks.json`. When the hook is new or changed, startup shows **Hooks need review**. Open `/hooks` and trust it. Until then Codex shows its prompts, and `approvals status` reports the pending trust.
- Copy Codex `prefix_rule` patterns and other native approval rules you want the hook to honor into `always-ask`. The hook can approve a call those native rules would prompt for.
{% </details> %}

{% <details summary="Kiro hook support, policies, and workspace trust"> %}
- Sync writes `~/.kiro/hooks/approve-permission-request.json`. The hook can approve shell calls (`execute_bash`) and file reads (`read_file`). Writes, searches, and other tools ask.
- A rule with `effect: ask` or `effect: evaluate`, or the `approve-for-me` preset, turns the hook off. A hook allow could skip such a prompt and the hook cannot match Kiro rules. It checks `~/.kiro/settings/permissions.yaml`, workspace permission files under `~/.kiro/workspace-roots/`, and agent profiles. It follows `KIRO_HOME` when set and leaves the native prompt in place if it cannot read the policy.
- Kiro prompts for each shell call in untrusted workspaces. Its docs leave open whether the hook can answer that prompt. Trust only workspaces where you would use `yolo`.
{% </details> %}

{% <details summary="Qoder modes and ask rules"> %}
- Sync writes `~/.qoder/settings.json`. In `default` and `accept_edits` modes it can approve `Bash`, `Read`, and `Edit` or `Write` outside dot files and dot folders. Other tools and modes ask.
- A `permissions.ask` rule in the user settings or a project's `.qoder/settings.json` or `.qoder/settings.local.json` turns the hook off because it could skip that prompt. The hook cannot read rules passed with `qoder --settings` or managed by your organization. Put patterns the hook should honor in `always-ask`.
- If Qoder reports plan state as `default`, the hook can approve a call while you plan.
{% </details> %}

{% <details summary="Qwen Code modes, ask rules, and environment"> %}
- Sync writes `~/.qwen/settings.json` or `$QWEN_HOME/settings.json`. This is a user hook. Qwen's approval prompt for project hooks does not apply.
- Set `tools.approvalMode` to `default` or `auto-edit`. The hook can approve `run_shell_command` calls without a `directory`, file reads, edits, and writes. Other tools and modes ask. If the mode is unset, trusted folders use auto mode.
- `disableAllHooks`, `--safe-mode`, and `--bare` turn the hook off. So does a `permissions.ask` rule in user, project, or system settings. A hook allow could skip that prompt and the hook cannot match Qwen's rules.
- A `.env` or `.qwen/.env` in the session folder or a parent, or a settings `env` block, also turns the hook off when it sets `HOME`, `AGNOSTIC_AI_HOME`, `CLAUDE_CONFIG_DIR`, or `USERPROFILE`. Qwen passes those values to the hook, and a repository could redirect the policy the hook reads.
{% </details> %}

### What we verified

We checked Kiro and Qoder hooks and Kiro's reviewer against vendor docs. We ran the Qwen Code hook in version 0.28.0 with a local test model.

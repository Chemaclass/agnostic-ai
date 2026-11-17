+++
title = "Companion"
description = "A floating list of the AI conversations that need you, and the cloud sessions it can include."
weight = 44

[extra]
group = "Reference"
+++

# Companion

## companion

Show a small circle that floats over every app. It counts the Claude Code, Codex and Cursor conversations that wait for you. Click it, or press its shortcut, for a searchable list. The list opens each conversation where it lives and marks it as read or done. Right-click a conversation and choose **Rename…** to give it your own name. The web interface shows the same name. On a conversation that needs you, the same menu offers **Mark as read**, which clears it without opening it.

```bash
agnostic-ai companion install
agnostic-ai companion
agnostic-ai companion stop
agnostic-ai companion uninstall
```

The companion is a separate program. Run `companion install` first. `agnostic-ai upgrade` then replaces an installed companion when its version differs.

On macOS, `companion install` downloads the companion that the release built for your Mac (Apple silicon or Intel). It checks the download against the release's `checksums.txt` and, when the GitHub CLI (`gh`) is installed and logged in, against the release's build provenance, the same way `agnostic-ai upgrade` checks the CLI. It needs no compiler. A download that does not verify stops the install and leaves the installed companion as it was. A release that ships no built companion, an `agnostic-ai` built from any commit but a release tag (such as `make build` on `main`), and `--source` build it on your machine instead.

Everywhere else, and in those cases, it builds the companion from the same commit as the `agnostic-ai` you run and prints each step. A build needs:

| System | Status | Needs |
|--------|--------|-------|
| macOS | **Supported.** Built and run on macOS 27 (arm64). The released companion needs macOS 13 or later. | Nothing to download it. To build it: Go and the Xcode Command Line Tools. |
| Windows | **Not supported.** It compiles, but nobody has run it on a Windows machine. | Go. |
| Linux | **Not supported.** Nobody has built or run it on a Linux machine. Wayland compositors decide what stays on top, so the circle may not float there. | Go, a C compiler, and the GTK 4, WebKitGTK 6.0 and X11 development packages. |

Want it on your system? Help is welcome: [Windows (#3221)](https://github.com/Chemaclass/agnostic-ai/issues/3221) and [Linux (#3222)](https://github.com/Chemaclass/agnostic-ai/issues/3222) list what to run and verify.

- When something is missing, the command names it, says how to get it, and builds nothing.
- On macOS it installs an app, `agnostic-ai companion.app`, with the agnostic-ai icon. It signs the app on your machine with `codesign`, without a developer identity. The app stays out of the Dock.
- Start at login on macOS registers the app with the system. It shows under System Settings > General > Login Items, in Open at Login, with the app's name and icon.
- An install over a companion that an earlier release built as a bare program removes that program and its launch agent. When it started at login, the app does now.
- A build that fails leaves the installed companion as it was.
- A running companion is stopped for the install and started again after it.

| Flag | Description |
|------|-------------|
| `install --source` | Build from this folder instead of fetching the source. It is the `companion` folder of an agnostic-ai checkout. |

`agnostic-ai companion` starts the companion and returns. Run it again to bring back a hidden companion and open its list. It tells you when the companion was built for another release.

| Command | What it does |
|---------|--------------|
| `companion stop` | Ends the companion and the server it started. When none runs, it succeeds and says so. Start at login stays as you set it. **Quit Agnostic AI companion** in the companion's menus does the same. |
| `companion uninstall` | Ends the companion and turns off start at login. Deletes the built program, `companion.json` (its place, shortcut, animation and dim choices) and `companion-command` (where the CLI is). Keeps `conversation-marks.json`, which `serve` uses too. On macOS and Linux it also removes a login entry that a deleted program left behind. On Windows it names the registry value to delete. |

- The companion reads no chat itself. It starts its own `agnostic-ai serve --global --api-only --cloud-sessions` on a free loopback port.
- Every 5 seconds it asks that server whether the list changed, and sends the list's `ETag`. While no chat changed, the server answers without reading anything.
- Clicking a running chat selects the Terminal or Ghostty tab that runs it, through the same server route as the web interface. When only the app can come forward, the list stays open and says where the chat runs. macOS asks once to let the companion control each of those apps.
- **Send copied text**: with the list open, Cmd+Shift+V or the clipboard button beside the search shows a banner with the start of the copied text. Pick a chat with the arrow keys and Enter, or a click. A live chat in a Terminal or Ghostty tab gets the text typed into its input, and nothing is submitted. Text copied from a web page goes as Markdown when it holds links, tables, code blocks, headings or lists, and the banner says so. Any other chat comes forward and the list says to paste with Cmd+V. Pasting into Terminal also needs macOS Accessibility access, granted once in System Settings › Privacy & Security › Accessibility to the companion or the terminal that runs `agnostic-ai serve`. See [`POST /api/conversations/send`](@/docs/cli-reference/serve-api.md#post-api-conversations-send) for what is refused.
- A Claude Code session started with `claude --bg` runs in Claude's own background service, not in a terminal app. Clicking it selects a Terminal or Ghostty tab that already runs `claude attach` on it, named by its short id, such as `e3469759`, or its exact full name. The companion does not recognize a tab attached with the start of an id or part of a name, since that may name another session. With none, it opens a new tab that runs `claude attach <id>` in the session's folder, in the app the session last ran in when it remembers one; the short id comes from `claude agents --json`. The companion looks for `claude` on `PATH`, then in `~/.local/bin`, `~/.claude/local`, `/opt/homebrew/bin` and `/usr/local/bin`. Leaving that tab does not stop the session. When switching to a tab fails, or `claude agents` cannot run or does not list the session, the list offers the `claude attach` command to copy, or says to find its id with `claude agents` and run `claude attach <id>`.
- When a chat does not open, the reason stays above the list until you dismiss it or open another chat. When a chat could not be resumed, **Copy resume command** copies the command to run in a terminal. A Claude Code, Codex or `cursor-agent` chat still running in a terminal never offers it: that command would start a second copy.
- A row names the conversation's folder when it has one; a cloud session has none. When two folders in the list end in the same name, the row adds the parent folders that tell them apart, such as `shop/api` and `billing/api`. The row's tooltip and screen-reader description give the full path.
- Under its title, a row shows the agent's latest reply in one line: the question it asks you, or the start of its last message. Local Claude Code, Codex and Cursor chats show it. Cloud sessions do not yet. A reply older than your latest prompt is not shown, and a reply that may hold a password or key is left out. See [`reply`](@/docs/cli-reference/serve-api.md#get-api-views-conversations).
- **Rename…** renames the conversation in its own tool first, so the tool's list shows the same name when you jump to the chat. Only once the tool has the name does that server save it in `conversation-marks.json`, so every `serve` on the machine shows it and finds it in a search. The pencil button on the hovered or selected row, and F2 on the selected row, start the same rename without opening the chat. A click on the name still opens it. Enter saves it, and Escape keeps the old one. Right-clicking does not open the conversation or mark it as read. A new message keeps the name. A running conversation's Terminal or Ghostty tab takes the name as its title, and keeps it after the session ends. How each tool takes the name:

  - **Claude Code**: adds the same `custom-title` line Claude Code's own `/rename` writes to the session's transcript. A session that is still running may write its old name back.
  - **Claude Code cloud sessions**: changes the session's title on claude.ai/code, signed in with the login Claude Code keeps in the macOS keychain.
  - **Codex**: asks `codex app-server` to rename the thread (`thread/name/set`), so `codex` must be installed.
  - **Cursor**: an editor chat's name changes in Cursor's state database, in the chat list and in the chat's own data, and nothing else. Cursor may write the old name back if it has the chat loaded. A cursor-agent chat's name changes in its `store.db`, but only when a look at the open files just before the write finds the chat not running, since a running chat writes its old name back. The rename keeps the chat's done and read marks. Both need the `sqlite3` command.
  - **ChatGPT**: asks chatgpt.com to change the chat's title, signed in with the ChatGPT login the Codex command keeps. The ChatGPT app shows it once it syncs.
  - **Claude app chats, Cowork tasks and Codex cloud threads** cannot be renamed yet.

  When the tool does not take the name, the rename field stays open with the reason under it, such as "Codex is not installed" or "Sign in to ChatGPT in the Codex app", and the row keeps its old name. The server logs what the tool said on its standard error, with the tool and the conversation id.
- **Mark as read** saves the same seen mark that opening a conversation saves, through that server. The conversation leaves Needs you in the companion and the web interface without being opened, and moves where an opened one would: Active, or Archived when it is closed. See [`state`](@/docs/cli-reference/serve-api.md#get-api-views-conversations) for when a closed chat moves at once and when it waits a day. Its next write brings it back. If the save fails, the row stays and the list shows why.
- **Mark all as read**, beside Needs you, saves that mark for each conversation the group shows, after the search and the All, Local or Cloud choice, one after the other. A conversation written in since the list was drawn is skipped and stays. The first failed save stops the run and says why. The list holds every blocked and unread conversation of any age. The circle counts them all, whatever the search or place, so with **All** chosen and no search it shows zero after the run until something new is written.
- A chat started with [`conversations start`](@/docs/cli-reference/conversations.md#the-link-line) sits indented under the chat it came from while both are in the same group. A child that waits for you stays under Needs you, even when its parent is active or archived.
- Under Needs you, a row says why only when the agent waits: "Waiting for you", or "No activity for 7m" for local Codex. A new reply needs no words, and a row under Working does not repeat "Working".
- **Delete permanently…** removes a chat under Archived from this computer, after a confirmation. See [Delete an archived chat](#delete-an-archived-chat).
- An archived or hidden chat that starts working moves to Working as soon as its tool reports the work, before it writes a new message. For local Claude Code that is the next poll; a cloud session can take up to a minute. The archive is cleared in `conversation-marks.json`, so the web interface agrees and the chat does not go back to Archived when the work ends.
- The list's ⋯ menu and the circle's right-click menu hold the same settings, in the same order. A change in one shows in the other:
  - **Animate companion** turns its movement on and off. Working rows show a five-bulb gyroscope at constant brightness, with bulbs at different orbital radii and depths. Each working native Claude Code or Codex sub-agent adds one blue miniature of the same five-dot fractal. A number and agent icon show the count, with a tooltip and accessible label; unavailable counts add none. Working is named once in the blue section header. Working sub-agents keep an idle parent in Working, with its own still dot. A parent asking you a question stays in Needs you with a plain orange dot and no orbiting bulbs of its own; its working satellites keep their blue cores and animation. Cloud and other tools' sub-agents are not tracked. A chat with working subagents shows "▸ 3 subagents" in its meta line in place of the count. The list starts folded. Click it to fold them out under the chat, newest first, five at most, then a "+N more" button. Only working subagents are listed and counted: one that finished leaves the list, and the toggle goes away once none works. Each shows the working gyroscope, its title, its tool, its type when it has one, and its age. Its Codex runs started with `codex exec` count among them while they run. A Codex subagent opens its own thread, or switches to the terminal that runs it. A Claude Code subagent opens its chat. Which chats are unfolded is kept until the companion quits. Turning animation off or enabling the system's Reduce Motion keeps the indicator still with its section heading and worker count visible. The circle shows a still ring and its count keeps updating. The choice is kept across restarts.
  - **Dim when idle** fades the circle while nothing needs you. Pointing at it, or any conversation that needs you, brings back full color. The choice is kept across restarts.
  - **Small companion** shows a 40px circle instead of the 60px one. Its mark, working ring and count shrink with it, and so does its window, so clicks just outside the small circle reach the app behind it. It keeps the screen edges it sits nearest. The choice is kept across restarts.
  - **Start at login** starts the companion when you log in.
  - **Open from anywhere** shows the shortcut. In the right-click menu, choosing it opens the list on its ⋯ menu to change the keys.
  - **Hide Agnostic AI companion** takes the circle and the list off the screen. The companion keeps running and reading. While the shortcut is not active, the entry names `agnostic-ai companion` as the way back. The shortcut, `agnostic-ai companion` or opening the app again shows the circle where it was and opens the list.
  - **Quit Agnostic AI companion** asks first: the program stops, so the shortcut will not bring it back, and `agnostic-ai companion` (or your AI tool running it) starts it again. The question offers **Quit**, **Hide instead** and **Cancel**; Escape or a click elsewhere cancels. Quit ends the companion and its server, as `companion stop` does. `stop`, `install`, `upgrade`, `uninstall` and logging out never ask. Start at login stays as you set it.
- The companion is shown, hidden, or back dimmed. While hidden, a conversation that starts waiting for you brings the circle back on its own at 25%, with its count, without opening the list. On macOS it does not take the keys. Conversations that already waited when you hid it do not bring it back. A click on the circle or the shortcut brings back full color. **Dim when idle** does not change a dimmed circle, and **Animate companion** still applies to it. None of the three states is saved: a restart starts shown.
- A local Codex chat that wrote nothing for 5 minutes moves to Needs you as "No activity for 7m", not "Waiting for you": Codex does not say when it waits, so the silence is all the server knows. A tool that reports its wait shows "Waiting for you". The web interface uses the same words.

## Delete an archived chat

Right-click a chat under Archived and choose **Delete permanently…**. The confirmation names the chat and what goes. Nothing is sent until you confirm, and **Cancel** or Escape keeps it.

- It is offered on any chat under Archived that no process holds: one you marked complete, or one that is closed: right away for a Claude Code, Codex or `cursor-agent` chat run in a terminal, after a day for the others.
- A chat that went quiet by itself is checked again when you confirm. If anything was written in it since you last looked at the list, or while the delete runs, it is not deleted.
- Afterwards the list says how much space it freed.
- A refused or partial delete keeps the row. The reason stays above the list until you dismiss it, with any folder still on disk.

| Chat | Can be deleted |
|------|----------------|
| Claude Code, on this computer | Yes |
| Codex | No. Codex deletes a thread even while it is open. Close it first, then run `codex delete <id>` in a terminal. |
| Cursor editor chat | No. Cursor keeps every chat in one shared database. Delete it in Cursor. |
| `cursor-agent` | Not yet. |
| Any cloud session | No. Delete it where it runs. |
| Any chat on Windows | No. The server cannot tell whether a session still runs. |

A chat that cannot be deleted says why when you choose the item, and sends nothing. [`POST /api/conversations/delete`](@/docs/cli-reference/serve-api.md#post-api-conversations-delete) lists what is removed and what stays, such as Claude Code's own prompt history.

Two limits are accepted. A session that starts in the moment between the last check and the removal is not seen. A forked Claude Code session may still point at saved tool output in its parent's folder, and deleting the parent removes that output.

## Upgrade the companion

`agnostic-ai upgrade` uses the updated CLI to replace an installed companion, downloaded or built as `companion install` does. It prints progress and restarts the companion only if it was running. Settings and Start at login stay as they were. It does not install a companion if none is installed. `upgrade --check` and `upgrade --requires` do not replace it.

A companion update failure leaves the CLI upgraded and the old companion usable. The error names the failed step or missing build tool. Fix it, then run `agnostic-ai companion install` to retry.

After a direct Homebrew or npm upgrade, run `agnostic-ai companion install`. A fresh companion/server start shows a panel notice when their versions differ, including the command to run. Starting through the CLI also prints a warning. A companion and server already running before an external upgrade can keep their old versions until restarted.

## Cloud sessions {#claude-code-cloud-sessions}

`agnostic-ai serve --global --cloud-sessions` adds the Claude Code sessions that run at claude.ai/code to the conversations list. They count among Claude Code's conversations, with `app` `cloud`. They take the same states and marks as the others. A project server lists none, because a cloud session names no folder.

- **What it reads.** It asks `GET https://api.anthropic.com/v1/code/sessions?limit=100`, which answers at most 100 sessions. It does not page past them yet. It uses their id, title, status and times. It never reads a session's messages.
- **The sign-in.** For each call it reads the claude.ai sign-in that Claude Code keeps in the macOS keychain (the `Claude Code-credentials` entry). It sends it only to `api.anthropic.com`. It never keeps, prints or logs it, and never uses the refresh token. Run `claude` to sign in or renew it. Windows and Linux are not supported yet.
- **How often.** At most once a minute, with a 5-second timeout and no retry. A cloud change reaches the list at most a minute late. The web interface shows it when you open or reload the page. The companion asks every 5 seconds, so it shows there at most 65 seconds late.
- **When it fails.** Local conversations still list. A refusal, no answer in time, or an answer this version cannot read passes by itself: the last good list stands in for up to 10 minutes and `cloudNote` stays empty. After that, no cloud row shows and `cloudNote` says why. A missing or expired sign-in does the same at once. A system with no supported sign-in store lists none and says nothing.
- **Opening one** opens the session's own page, `https://claude.ai/code/session_<id>`, in the browser. It is the only kind of web page the server opens.
- **Status.** `working` shows as working, only while the session's worker runs. `blocked` shows as waiting for you. Other statuses are not live. No process on this machine can be checked for a cloud conversation, so either cloud's status is believed for a day after the conversation's last write.
- **Remote Control.** A Claude Code session that runs here with Remote Control on is also listed at claude.ai. It shows as one row: the local one, marked **Shared** on its meta line ("Also open on claude.ai" on hover and for screen readers). It counts once, takes its state from the local session, and shows under both Local and Cloud. Clicking it opens the local session as before. Its second action, **Open on claude.ai**, also in its right-click menu, opens the session's page. The match uses only the session's ids: the running session's `bridgeSessionId`, `session_<id>`, names the cloud session `cse_<id>`. This relation is undocumented and was measured on one account on 2026-11-12. When it does not match, or the local session has ended, both rows show as before. A seen mark on the cloud row carries over to the shared row. Its completed and hidden marks and its name do not. Marking the shared row read or complete, opening it, or taking a completion back marks the cloud row the same way, so it does not come back unread when the local session ends.

The same flag adds the threads that run in the Codex cloud. They count among Codex's conversations, with `app` `cloud`.

- **What it reads.** It asks `GET https://codex-cloud-backend.chatgpt.com/v1/threads?limit=100`, the list the Codex app reads, which answers at most 100 threads. It does not page past them yet. It uses their id, name, status and times. It never reads a thread's messages.
- **The sign-in.** For each call it reads the ChatGPT login that the Codex command keeps in `auth.json` (in `$CODEX_HOME`, default `~/.codex`). It sends it only to that host. It never keeps, prints or logs it, and never reads the refresh token or an API key. Without that file it asks nothing and says nothing, because you do not use Codex. A login with an API key has no cloud threads, and `cloudNote` says so.
- **How often, and failures:** as for Claude Code above. `cloudNote` holds one sentence for each cloud that could not be read.
- **Opening one** opens `codex://threads/<id>?hostId=durable`, which the Codex app handles.
- **Status.** An `active` thread shows as working. It shows as waiting for you when it waits on an approval or on your input. Other statuses are not live.

Both endpoints are undocumented and can change with any release of either tool.

Your Claude chats and Cowork tasks show too, with no flag: they are read from what the Claude desktop app keeps on this computer. A row reads "Claude · Chat" or "Claude · Cowork" and opens in the Claude app. A chat the assistant answered counts as waiting for you until you see it. Chats sit under **Cloud**, Cowork tasks under **Local**.

Your ChatGPT chats show too, with no flag, when the ChatGPT desktop app is installed: they are read from the chat list it keeps on this computer, in `codex.db` under `$CODEX_HOME/sqlite` (default `~/.codex`). Nothing is asked over the network, and a new reply shows within the companion's 5-second check. A row reads "ChatGPT · Chat" and opens on chatgpt.com. A chat you started on chatgpt.com or your phone shows once the app has synced it. That list is undocumented and can change with any release of the app.

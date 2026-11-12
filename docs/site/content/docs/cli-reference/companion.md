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

The companion is a separate program that is built on your machine. Run `companion install` first. `agnostic-ai upgrade` then rebuilds an installed companion when its version differs. It builds the companion from the same commit as the `agnostic-ai` you run and prints each step. It needs:

| System | Status | Needs |
|--------|--------|-------|
| macOS | **Supported.** Built and run on macOS 27 (arm64). | Go and the Xcode Command Line Tools. |
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

`agnostic-ai companion` starts the companion and returns. Run it again to wake a paused companion and open its list. It tells you when the companion was built for another release.

| Command | What it does |
|---------|--------------|
| `companion stop` | Ends the companion and the server it started. When none runs, it succeeds and says so. Start at login stays as you set it. **Quit Agnostic AI companion** in the companion's menus does the same. |
| `companion uninstall` | Ends the companion and turns off start at login. Deletes the built program, `companion.json` (its place, shortcut, animation and dim choices) and `companion-command` (where the CLI is). Keeps `conversation-marks.json`, which `serve` uses too. On macOS and Linux it also removes a login entry that a deleted program left behind. On Windows it names the registry value to delete. |

- The companion reads no chat itself. It starts its own `agnostic-ai serve --global --api-only --cloud-sessions` on a free loopback port.
- Every 5 seconds it asks that server whether the list changed, and sends the list's `ETag`. While no chat changed, the server answers without reading anything.
- Clicking a running chat selects the Terminal or Ghostty tab that runs it, through the same server route as the web interface. When only the app can come forward, the list stays open and says where the chat runs. macOS asks once to let the companion control each of those apps.
- When a chat does not open, the reason stays above the list until you dismiss it or open another chat. When a chat could not be resumed, **Copy resume command** copies the command to run in a terminal. A Claude Code, Codex or `cursor-agent` chat still running in a terminal never offers it: that command would start a second copy.
- A row names the conversation's folder when it has one; a cloud session has none. When two folders in the list end in the same name, the row adds the parent folders that tell them apart, such as `shop/api` and `billing/api`. The row's tooltip and screen-reader description give the full path.
- Under its title, a row shows the agent's latest reply in one line: the question it asks you, or the start of its last message. Local Claude Code and Codex chats show it. Cursor chats and cloud sessions do not yet. A reply older than your latest prompt is not shown, and a reply that may hold a password or key is left out. See [`reply`](@/docs/cli-reference/serve-api.md#get-api-views-conversations).
- **Rename…** saves the name in `conversation-marks.json` through that server, so every `serve` on the machine shows it and finds it in a search. Enter saves it, and Escape keeps the old one. Right-clicking does not open the conversation or mark it as read. A new message keeps the name. A running conversation's Terminal or Ghostty tab takes the name as its title, and keeps it after the session ends.
- **Mark as read** saves the same seen mark that opening a conversation saves, through that server. The conversation leaves Needs you in the companion and the web interface without being opened, and moves where an opened one would: Active, or Archived when it is not running and was last written more than a day ago. Its next write brings it back. If the save fails, the row stays and the list shows why.
- **Mark all as read**, beside Needs you, saves that mark for each conversation the group shows, after the search and the All, Local or Cloud choice, one after the other. A conversation written in since the list was drawn is skipped and stays. The first failed save stops the run and says why. The list holds every blocked and unread conversation of any age, so after the run the circle shows zero until something new is written.
- A chat started with [`conversations start`](@/docs/cli-reference/conversations.md#the-link-line) sits indented under the chat it came from while both are in the same group. A child that waits for you stays under Needs you, even when its parent is active or archived.
- Under Needs you, a row says why only when the agent waits: "Waiting for you", or "No activity for 7m" for local Codex. A new reply needs no words, and a row under Working does not repeat "Working".
- **Delete permanently…** removes a chat under Archived from this computer, after a confirmation. See [Delete an archived chat](#delete-an-archived-chat).
- An archived or hidden chat that starts working moves to Working as soon as its tool reports the work, before it writes a new message. For local Claude Code that is the next poll; a cloud session can take up to a minute. The archive is cleared in `conversation-marks.json`, so the web interface agrees and the chat does not go back to Archived when the work ends.
- The list's ⋯ menu and the circle's right-click menu hold the same settings, in the same order. A change in one shows in the other:
  - **Animate companion** turns its movement on and off. When it is off, a still ring shows a working agent and the count keeps updating. The choice is kept across restarts.
  - **Dim when idle** fades the circle while nothing needs you. Pointing at it, or any conversation that needs you, brings back full color. The choice is kept across restarts.
  - **Start at login** starts the companion when you log in.
  - **Open from anywhere** shows the shortcut. In the right-click menu, choosing it opens the list on its ⋯ menu to change the keys.
  - **Pause Agnostic AI companion** fades the circle to 10% and stops that server. The circle shows no count and can still be dragged. Nothing is read or asked until a click on the circle, the shortcut or `agnostic-ai companion` wakes it with a new list. A restart starts awake. Unlike **Dim when idle**, a paused companion does not keep watching.
  - **Quit Agnostic AI companion** ends the companion and its server, as `companion stop` does. Start at login stays as you set it. `agnostic-ai companion` starts it again.
- A local Codex chat that wrote nothing for 5 minutes moves to Needs you as "No activity for 7m", not "Waiting for you": Codex does not say when it waits, so the silence is all the server knows. A tool that reports its wait shows "Waiting for you". The web interface uses the same words.

## Delete an archived chat

Right-click a chat under Archived and choose **Delete permanently…**. The confirmation names the chat and what goes. Nothing is sent until you confirm, and **Cancel** or Escape keeps it.

- It is offered on any chat under Archived that no process holds: one you marked complete, or one that went quiet by itself more than a day ago.
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

`agnostic-ai upgrade` uses the updated CLI to rebuild an installed companion. It prints progress and restarts the companion only if it was running. Settings and Start at login stay as they were. It does not build a companion if none is installed. `upgrade --check` and `upgrade --requires` do not rebuild it.

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

The same flag adds the threads that run in the Codex cloud. They count among Codex's conversations, with `app` `cloud`.

- **What it reads.** It asks `GET https://codex-cloud-backend.chatgpt.com/v1/threads?limit=100`, the list the Codex app reads, which answers at most 100 threads. It does not page past them yet. It uses their id, name, status and times. It never reads a thread's messages.
- **The sign-in.** For each call it reads the ChatGPT login that the Codex command keeps in `auth.json` (in `$CODEX_HOME`, default `~/.codex`). It sends it only to that host. It never keeps, prints or logs it, and never reads the refresh token or an API key. Without that file it asks nothing and says nothing, because you do not use Codex. A login with an API key has no cloud threads, and `cloudNote` says so.
- **How often, and failures:** as for Claude Code above. `cloudNote` holds one sentence for each cloud that could not be read.
- **Opening one** opens `codex://threads/<id>?hostId=durable`, which the Codex app handles.
- **Status.** An `active` thread shows as working. It shows as waiting for you when it waits on an approval or on your input. Other statuses are not live.

Both endpoints are undocumented and can change with any release of either tool.

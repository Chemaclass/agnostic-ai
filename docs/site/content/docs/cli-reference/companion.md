+++
title = "Companion"
description = "A floating list of the AI conversations that need you, and the cloud sessions it can include."
weight = 44

[extra]
group = "Reference"

[extra.moved]
waiting = "@/docs/cli-reference/companion-list.md"
chats-archived-in-their-app = "@/docs/cli-reference/companion-list.md"
delete-an-archived-chat = "@/docs/cli-reference/companion-list.md"
+++

# Companion

The companion is a small circle that floats over every app. It counts the Claude Code, Codex and Cursor conversations that wait for you. Click it for a searchable list: see [Companion list](@/docs/cli-reference/companion-list.md).

<span id="companion" hidden></span>

## Install and start {#install}

Put the circle on your screen.

1. Run `agnostic-ai companion install`. On macOS it downloads the build for your Mac. Elsewhere it builds the companion.
2. Run `agnostic-ai companion`. It starts the companion and returns.
3. Run it again to bring back a hidden companion and open its list. It tells you when the companion was built for another release.

| System | Status |
|--------|--------|
| macOS | Supported. Built and run on macOS 27 (arm64). The released companion needs macOS 13 or later. |
| Windows | Not supported. It compiles, but nobody has run it. [Help with #3221](https://github.com/Chemaclass/agnostic-ai/issues/3221). |
| Linux | Not supported. Nobody has built or run it. [Help with #3222](https://github.com/Chemaclass/agnostic-ai/issues/3222). |

{% <details summary="Details: download, build, signing, login item"> %}
- Download: on macOS the companion that the release built for your Mac (Apple silicon or Intel). It needs no compiler.
- Checks: the download is checked against the release's `checksums.txt`. When `gh` is installed and logged in, it is also checked against the release's build provenance, as `agnostic-ai upgrade` checks the CLI.
- A download that does not verify stops the install and leaves the installed companion as it was.
- Build instead of download: a release that ships no built companion, an `agnostic-ai` built from any commit but a release tag (such as `make build` on `main`), and `--source`.
- A build uses the same commit as the `agnostic-ai` you run, and prints each step.
- Build needs on macOS: Go and the Xcode Command Line Tools.
- Build needs on Windows: Go.
- Build needs on Linux: Go, a C compiler, and the GTK 4, WebKitGTK 6.0 and X11 development packages. Wayland compositors decide what stays on top, so the circle may not float there.
- When something is missing, the command names it, says how to get it, and builds nothing.
- A build that fails leaves the installed companion as it was.
- A running companion is stopped for the install and started again after it.
- On macOS it installs an app, `agnostic-ai companion.app`, with the agnostic-ai icon. The app stays out of the Dock.
- The app is signed on your machine with `codesign`, without a developer identity.
- Start at login registers the app with the system. It shows under System Settings > General > Login Items, in Open at Login, with the app's name and icon.
- An install over a companion that an earlier release built as a bare program removes that program and its launch agent. When it started at login, the app does now.
{% </details> %}

## Commands {#commands}

| Command | What it does |
|---------|--------------|
| `companion install` | Installs it. `--source` builds from this folder instead of fetching the source: the `companion` folder of an agnostic-ai checkout. |
| `companion` | Starts it, or brings back a hidden one and opens its list. |
| `companion stop` | Ends the companion and the server it started. Start at login stays as you set it. |
| `companion uninstall` | Ends the companion, turns off start at login, and deletes what install made. |

{% <details summary="Details: stop and uninstall"> %}
- `stop` when nothing runs: it succeeds and says so.
- **Quit Agnostic AI companion** in the menus does the same as `stop`.
- `uninstall` deletes the built program, `companion.json` (its place, shortcuts, animation and dim choices) and `companion-command` (where the CLI is).
- `uninstall` keeps `conversation-marks.json`, which `serve` uses too.
- On macOS and Linux it also removes a login entry that a deleted program left behind.
- On Windows it names the registry value to delete.
{% </details> %}

## The circle and its menus {#menus}

The circle's count turns solid orange, with a lock and a halo, while a chat waits for a permission or an answer. Unread replies alone keep the softer count. A screen reader hears both numbers, such as "2 waiting for you, 4 unread".

Right-click the circle, or open the list's ⋯ menu. Both hold the same items in the same order, and a change in one shows in the other.

| Menu item | What it does |
|-----------|--------------|
| **Animate companion** | Movement on or off. Off, or with Reduce Motion on, the circle shows a still ring and its count keeps updating. |
| **Dim when idle** | Fades the circle while nothing needs you. |
| **Small companion** | A 40px circle instead of 60px. |
| **Start at login** | Starts the companion when you log in. |
| **Open from anywhere** | Shows the shortcut that opens the list, and changes it. |
| **Send copied text** | Shows the send shortcut, Ctrl+Alt+V until you change it. See [Send copied text](@/docs/cli-reference/companion-list.md#send-copied-text). |
| **Hide Agnostic AI companion** | Takes the circle and the list off the screen. It keeps running. |
| **Quit Agnostic AI companion** | Asks first, then ends the companion and its server. |

Animate companion, Dim when idle and Small companion are kept across restarts. For what the list shows while animated, see [Working rows and subagents](@/docs/cli-reference/companion-list.md#working-rows).

{% <details summary="Details: dim, small, shortcuts"> %}
- Dim: pointing at the circle, or any conversation that needs you, brings back full color.
- Small: its mark, working ring, count and window shrink with it, so clicks just outside the small circle reach the app behind it. It keeps the screen edges it sits nearest.
- Open from anywhere: in the right-click menu, choosing it opens the list on its ⋯ menu to change the keys.
- The open and send shortcuts cannot share keys.
- Keys the system refuses are kept, and the menu says why the shortcut is not active.
{% </details> %}

## Hide and quit {#hide-and-quit}

**Hide** takes the circle and the list off the screen. The shortcut, `agnostic-ai companion` or opening the app again brings them back where they were and opens the list. **Quit** stops the program.

| State | What happens |
|-------|--------------|
| Hidden | A chat that starts waiting for you brings the circle back on its own at 25%, with its count, without opening the list. |
| Dimmed | A click on the circle or the shortcut brings back full color. |
| Quit | The shortcut cannot bring it back. Run `agnostic-ai companion` to start it again. |

{% <details summary="Details: hidden, dimmed and quit states"> %}
- While the shortcut is not active, the Hide entry names `agnostic-ai companion` as the way back.
- A hidden companion keeps running and reading.
- A returned circle on macOS does not take the keys.
- Chats that already waited when you hid it do not bring it back.
- **Dim when idle** does not change a dimmed circle. **Animate companion** still applies to it.
- None of the shown, hidden and dimmed states is saved. A restart starts shown.
- Quit asks first. The question offers **Quit**, **Hide instead** and **Cancel**. Escape or a click elsewhere cancels.
- Quit ends the companion and its server, as `companion stop` does. Start at login stays as you set it.
- `stop`, `install`, `upgrade`, `uninstall` and logging out never ask.
- Your AI tool running `agnostic-ai companion` also starts it again.
{% </details> %}

## How it reads conversations {#how-it-reads}

The companion reads no chat itself. It starts its own `agnostic-ai serve --global --api-only --cloud-sessions` on a free loopback port. Every 5 seconds it asks that server whether the list changed.

{% <details summary="Details: polling"> %}
- The request sends the list's `ETag`.
- While no chat changed, the server answers without reading anything.
{% </details> %}

## Upgrade the companion

`agnostic-ai upgrade` replaces an installed companion, downloaded or built as `companion install` does.

- It prints progress.
- It restarts the companion only if it was running.
- Settings and Start at login stay as they were.
- It installs none if none is installed.
- `upgrade --check` and `upgrade --requires` do not replace it.

After a direct Homebrew or npm upgrade, run `agnostic-ai companion install`.

{% <details summary="Details: failures and version mismatch"> %}
- A companion update failure leaves the CLI upgraded and the old companion usable.
- The error names the failed step or missing build tool. Fix it, then run `agnostic-ai companion install` to retry.
- A fresh companion/server start shows a panel notice when their versions differ, with the command to run. Starting through the CLI also prints a warning.
- A companion and server already running before an external upgrade can keep their old versions until restarted.
{% </details> %}

## Cloud sessions {#claude-code-cloud-sessions}

Add the sessions that run in the cloud to the list.

1. Run `agnostic-ai serve --global --cloud-sessions`. The companion starts its server with this flag already.
2. Claude Code sessions from claude.ai/code and Codex cloud threads join the conversations, with `app` `cloud`.
3. They take the same states and marks as the others.

A project server lists none, because a cloud session names no folder. Both endpoints are undocumented and can change with any release of either tool.

| | Claude Code cloud | Codex cloud |
|---|---|---|
| Reads | `GET https://api.anthropic.com/v1/code/sessions?limit=100` | `GET https://codex-cloud-backend.chatgpt.com/v1/threads?limit=100`, the list the Codex app reads |
| Sign-in | The claude.ai login in the macOS keychain (`Claude Code-credentials`). Run `claude` to renew it. | The ChatGPT login in `auth.json` (in `$CODEX_HOME`, default `~/.codex`) |
| Opens | `https://claude.ai/code/session_<id>` in the browser | `codex://threads/<id>?hostId=durable`, handled by the Codex app |
| Working | Status `working`, only while the worker runs | Status `active` |
| Waiting for you | Status `blocked` | Waits on an approval or on your input |

{% <details summary="Details: privacy, refresh, failures, status"> %}
- Limit: at most 100 sessions or threads. It does not page past them yet.
- It uses their id, title or name, status and times. It never reads their messages.
- The sign-in is sent only to its own host. It is never kept, printed or logged, and the refresh token is never used.
- Claude Code sign-in: Windows and Linux are not supported yet.
- Codex sign-in: without `auth.json` it asks nothing and says nothing, because you do not use Codex. A login with an API key has no cloud threads, and `cloudNote` says so.
- Flag off: without `--cloud-sessions`, nothing reads your sign-in or leaves this machine.
- Refresh: at most once a minute, with a 5-second timeout and no retry.
- A cloud change reaches the list at most a minute late. The web interface shows it when you open or reload the page. The companion asks every 5 seconds, so it shows there at most 65 seconds late.
- Failures: local conversations still list.
- A refusal, no answer in time, or an answer this version cannot read passes by itself. The last good list stands in for up to 10 minutes and `cloudNote` stays empty.
- After that, no cloud row shows and `cloudNote` says why. A missing or expired sign-in does the same at once.
- A system with no supported sign-in store lists none and says nothing.
- `cloudNote` holds one sentence for each cloud that could not be read.
- The Claude Code session page is the only kind of web page the server opens.
- Other statuses are not live.
- No process on this machine can be checked for a cloud conversation, so either cloud's status is believed for a day after the conversation's last write.
- After that day, a Claude Code cloud session is active, or unread, under Active. It is never archived by its age.
{% </details> %}

## Remote Control twin {#remote-control}

A Claude Code session that runs here with Remote Control on is also listed at claude.ai. It shows as one row: the local one, marked **Shared** ("Also open on claude.ai" on hover and for screen readers).

- It counts once and takes its state from the local session.
- It shows under both Local and Cloud.
- Clicking it opens the local session. **Open on claude.ai**, also in its right-click menu, opens the cloud page.

{% <details summary="Details: how the match works, marks"> %}
- The match uses only ids: the running session's `bridgeSessionId`, `session_<id>`, names the cloud session `cse_<id>`.
- This relation is undocumented and was measured on one account on 2026-11-12.
- When it does not match, both rows show as before.
- A closed session stays shared while the server that saw it shared runs.
- A seen mark on the cloud row carries over to the shared row. Its completed and hidden marks and its name do not.
- Marking the shared row read or complete, opening it, or taking a completion back marks the cloud row the same way. It then does not come back unread when a restarted server lists it apart.
- Opening the cloud row on claude.ai marks the local row read too.
- After it closes: see [Shared Remote Control rows](@/docs/cli-reference/companion-list.md#remote-control).
{% </details> %}

## Claude chats, Cowork tasks and ChatGPT chats {#other-chats}

These show with no flag. The companion reads what the desktop apps keep on this computer.

| Row | Reads from | Opens in | Sits under |
|-----|------------|----------|------------|
| "Claude · Chat" | The Claude desktop app | The Claude app | Cloud |
| "Claude · Cowork" | The Claude desktop app | The Claude app | Local |
| "ChatGPT · Chat" | `codex.db` under `$CODEX_HOME/sqlite` (default `~/.codex`), when the ChatGPT desktop app is installed | chatgpt.com | |

{% <details summary="Details: waiting, sync, stability"> %}
- A Claude chat the assistant answered counts as waiting for you until you see it.
- ChatGPT reads nothing over the network. A new reply shows within the companion's 5-second check.
- A ChatGPT chat you started on chatgpt.com or your phone shows once the app has synced it.
- The ChatGPT chat list is undocumented and can change with any release of the app.
{% </details> %}

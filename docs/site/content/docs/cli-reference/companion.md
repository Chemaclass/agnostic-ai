+++
title = "Companion"
description = "A floating list of the AI conversations that need you, and the cloud sessions it can include."
weight = 44

[extra]
group = "Reference"
+++

# Companion

## companion

Show a small circle that floats over every app and counts the Claude Code, Codex and Cursor conversations that wait for you. Click it, or press its shortcut, for a searchable list that opens each conversation where it lives and marks it as read or done.

```bash
agnostic-ai companion install
agnostic-ai companion
agnostic-ai companion stop
agnostic-ai companion uninstall
```

The companion is a separate program that is built on your machine, so `companion install` comes first, and again after each upgrade of agnostic-ai. It builds the companion from the same commit as the `agnostic-ai` you run and prints each step as it goes. It needs:

| System | Needs |
|--------|-------|
| macOS | Go and the Xcode Command Line Tools. Built and run on macOS 27 (arm64). |
| Windows | Go. Compiles; not yet run on a Windows machine. |
| Linux | Go, a C compiler, and the GTK 4, WebKitGTK 6.0 and X11 development packages. Not yet built or run on a Linux machine. Wayland compositors decide what stays on top, so the circle may not float there. |

When something is missing, the command names it and how to get it, and builds nothing. A build that fails leaves the installed companion as it was. A companion that runs is stopped for the install and started again after it.

| Flag | Description |
|------|-------------|
| `install --source` | Build from this folder, the `companion` folder of an agnostic-ai checkout, instead of fetching the source. |

`agnostic-ai companion` starts the companion and returns. Running it again brings a hidden companion back and opens its list. It says when the companion was built for another release.

| Command | What it does |
|---------|--------------|
| `companion stop` | Ends the companion and the server it started. It succeeds and says so when none runs. Start at login stays as you set it. |
| `companion uninstall` | Ends the companion, turns off start at login, and deletes the built program, `companion.json` (its place and shortcut) and `companion-command` (where the CLI is). It keeps `conversation-marks.json`, which `serve` uses too. On macOS and Linux it also removes a login entry that a deleted program left behind; on Windows it names the registry value to delete. |

The companion reads no chat itself. It starts its own `agnostic-ai serve --global --api-only --cloud-sessions` on a free loopback port and asks it every 5 seconds whether the list changed, with the list's `ETag`; the server answers without reading anything while no chat changed. **Hide Agnostic AI companion**, in the list's ⋯ menu, takes the circle off the screen and stops that server: nothing is read or asked until the shortcut brings the companion back.

## Claude Code cloud sessions

`agnostic-ai serve --global --cloud-sessions` adds the Claude Code sessions that run at claude.ai/code to the conversations list. They count among Claude Code's conversations, with `app` `cloud`, and take the same states and marks as the others. A project server lists none: a cloud session names no folder.

- **What it reads.** It asks `GET https://api.anthropic.com/v1/code/sessions?limit=100`, which answers at most 100 sessions; it does not page past them yet. It uses their id, title, status and times. It never reads a session's messages.
- **The sign-in.** It reads the claude.ai sign-in Claude Code keeps in the macOS keychain (the `Claude Code-credentials` entry) for each call, and sends it only to `api.anthropic.com`. It never keeps, prints or logs it, and never uses the refresh token. Run `claude` to sign in or renew it. Windows and Linux are not supported yet.
- **How often.** At most once a minute, with a 5-second timeout and no retry. A cloud change shows in the list at most a minute late (65 seconds in the companion, which asks every 5 seconds).
- **When it fails.** Local conversations still list. A refusal, no answer in time, or an answer this version cannot read passes by itself: the last good list stands in for up to 10 minutes and `cloudNote` stays empty. After that, or at once for a missing or expired sign-in, no cloud row shows and `cloudNote` says why. A system with no supported sign-in store lists none and says nothing.
- **Opening one** opens the session's own page, `https://claude.ai/code/session_<id>`, in the browser. It is the only kind of web page the server opens.
- **Status.** `working` shows as working, only while the session's worker runs, and `blocked` as waiting for you; other statuses are not live. No process on this machine can be checked for a cloud conversation, so either cloud's status is believed for a day after the conversation's last write.

The same flag adds the threads that run in the Codex cloud. They count among Codex's conversations, with `app` `cloud`.

- **What it reads.** It asks `GET https://codex-cloud-backend.chatgpt.com/v1/threads?limit=100`, the list the Codex app reads, which answers at most 100 threads; it does not page past them yet. It uses their id, name, status and times, and never reads a thread's messages. The endpoint is undocumented and can change with any Codex release.
- **The sign-in.** It reads the ChatGPT login the Codex command keeps in `auth.json` (in `$CODEX_HOME`, default `~/.codex`) for each call, and sends it only to that host. It never keeps, prints or logs it, and never reads the refresh token or an API key. Without that file nothing is asked and nothing is said: the person does not use Codex. A login with an API key has no cloud threads, and `cloudNote` says so.
- **How often, and failures:** as for Claude Code above. `cloudNote` holds one sentence for each cloud that could not be read.
- **Opening one** opens `codex://threads/<id>?hostId=durable`, which the Codex app handles.
- **Status.** An `active` thread shows as working, or as waiting for you when it waits on an approval or on your input; other statuses are not live.

Both endpoints are undocumented and can change with any release of either tool.

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

A small circle that floats over every app and counts the Claude Code, Codex and Cursor conversations that wait for you. Click it for a searchable list: see [Companion list](@/docs/cli-reference/companion-list.md).

Use the row menu to [stop a forgotten Claude Code shell](@/docs/cli-reference/companion-list.md#stop-a-forgotten-shell).

<span id="companion" hidden></span>

## Install and start {#install}

The disk image works on macOS 13 Ventura or later and includes the companion, the matching CLI, and the full web dashboard. You can open the app without setting up agnostic-ai or installing a separate CLI.

1. Open **Apple menu > About This Mac**. An Apple chip uses the **Apple silicon** (`arm64`) download. An Intel processor uses **Intel** (`amd64`).
2. Open the [latest stable release](https://github.com/Chemaclass/agnostic-ai/releases/latest) and download the matching `.dmg`. If the release has no disk image for your Mac, [install from the terminal](#install-from-terminal).
3. Open the disk image, drag **agnostic-ai companion** into **Applications**, and eject the disk image. Open the app from **Applications** in Finder.
4. Click the floating circle to see your conversations. Use **Open the dashboard** in the list header to open the full web interface in your browser.

The companion and dashboard use your [global settings](@/docs/global-configuration.md) in `~/.agnostic-ai` by default. Existing sessions from supported tools appear as they are discovered. If the list is empty, start a conversation in a supported tool such as Claude Code or Codex. See the [Companion list](@/docs/cli-reference/companion-list.md) for the conversations it can show.

{% <details summary="First-launch checks and macOS permissions"> %}
The app is signed without an Apple Developer ID and is not notarized. Gatekeeper may say it cannot verify the developer or check the app. [Verify the downloaded disk image](@/docs/verify-a-release.md) and see [Apple's guide to macOS alerts](https://support.apple.com/en-us/102445).

macOS may ask for Automation when you [open a terminal chat](@/docs/cli-reference/companion-list.md#open-a-chat), or for Accessibility when you [paste into a terminal](@/docs/cli-reference/companion-list.md#send-copied-text). These permissions are for those actions.
{% </details> %}

{% <details summary="Use a different configuration folder or install the command line tool"> %}
The app uses `AGNOSTIC_AI_HOME` when that variable is set in its environment. A Finder launch normally uses the default folder. An override in a shell startup file such as `.zshrc` applies to that shell and processes that inherit its environment.

To use the command in a terminal, right-click the circle and choose **Install command line tool**. The app links the bundled CLI into `/usr/local/bin` when that folder is writable and on `PATH`, or into `~/.local/bin`. Add the reported folder to your shell's `PATH` if needed. Existing commands, including npm and Homebrew installs, stay in place. Keep the app in Applications so the link still works.
{% </details> %}

{% <details summary="Update the disk-image app"> %}
The app checks for updates on start and every 24 hours while it runs. A check it makes on its own opens no window: when a newer stable release has a disk image for this Mac, the update entry in both menus reads **Update to vX.Y.Z** and a dot appears on the list's ⋯ button. Choose **Check for updates** in either menu to check now; the list says what it found. Choose **Update to vX.Y.Z** to update: the list asks once, then **Update and restart** replaces the app and its bundled CLI together. The list shows each step: downloading and checking the checksum, checking the downloaded app, restarting. The download uses the same checksum and build provenance checks as the CLI installer.

Separate npm or Homebrew installs keep their own update path. Running `agnostic-ai upgrade` from the bundled CLI link directs you to the app menu. If an update fails before the restart, your current version keeps running. The list names the reason, such as a download that did not match its checksum, and the log, `~/Library/Application Support/agnostic-ai/companion-update.log`. **Retry** starts the update again.
{% </details> %}

### Install from the terminal {#install-from-terminal}

Use an existing CLI to install and start the companion:

1. Run `agnostic-ai companion install`. On macOS it downloads the build for your Mac. Elsewhere it builds the companion.
2. Run `agnostic-ai companion`. It starts the companion and returns.
3. Run it again to bring back a hidden companion and open its list.

If a release ships without the companion archive, the installer builds the companion from source. That needs Go and the Xcode Command Line Tools on macOS.

| System | Status |
|--------|--------|
| macOS | Supported (macOS 13 or later). |
| Windows | Builds and runs on a GitHub Actions Windows Server 2025 runner. Not yet tried on a Windows 11 desktop. [Help with #3221](https://github.com/Chemaclass/agnostic-ai/issues/3221). |
| Linux | Builds and runs in containers, on X11 and Wayland. Not yet tried on a desktop with a real screen. [Help with #3222](https://github.com/Chemaclass/agnostic-ai/issues/3222). |

{% <details summary="Details: download, build, signing, login item"> %}
- Built and run on macOS 27 (arm64). The released companion needs macOS 13 or later.
- CI builds and tests it on Windows and Linux. On Windows it ran only on a GitHub Actions runner: see [What ran on Windows](#windows-runs). On Linux it ran only in containers: see [What ran on Linux](#linux-runs).
- Running `agnostic-ai companion` again tells you when the companion was built for another release, or installed by an `agnostic-ai` built from another commit. When the companion cannot run, it says so and names its path.
- On a Mac older than macOS 13, install stops and says why.
- Download: on macOS the companion that the release built for your Mac (Apple silicon or Intel). An Intel `agnostic-ai` that runs through Rosetta gets the Apple silicon one. It needs no compiler.
- Checks: the download is checked against the release's `checksums.txt`. When `gh` is installed and logged in, it is also checked against the release's build provenance, as `agnostic-ai upgrade` checks the CLI.
- A download that does not verify stops the install and leaves the installed companion as it was. When `gh` itself fails, `AGNOSTIC_AI_VERIFY_ATTESTATION=0` installs on the checksum alone.
- Ctrl-C stops the install and removes its build folder. The next install also removes a build folder an earlier one left over an hour ago.
- When neither the new app nor the old one can take its place, the old one stays in the build folder, and the error names both paths.
- Build instead of download: a release that ships no built companion, an `agnostic-ai` built from any commit but a release tag (such as `make build` on `main`), and `--source`.
- A build uses the same commit as the `agnostic-ai` you run, and prints each step.
- Build needs on macOS: Go and the Xcode Command Line Tools.
- Build needs on Windows: Go.
- Build needs on Linux: Go, a C compiler, `pkg-config`, and the GTK 4, WebKitGTK 6.0 and X11 development packages. The command names every missing one at once, with one install line for `apt`, `dnf` or `pacman`, whichever the machine has. Without `pkg-config` it names that first, since the libraries are checked with it.
- When something is missing, the command names it, says how to get it, and builds nothing.
- A build that fails leaves the installed companion as it was.
- A running companion is stopped for the install and started again after it.
- On macOS it installs an app, `agnostic-ai companion.app`, with the agnostic-ai icon. The app stays out of the Dock.
- The terminal installer signs the app on your machine with `codesign`, without a developer identity. It makes a certificate for this Mac on the first install and keeps it in the companion folder, so each later install keeps the app's Accessibility access. Uninstalling removes the certificate.
- The disk image carries an app signed during the release build with the project's own certificate, so updating from one disk image to the next keeps Accessibility access too.
- Switching between the disk image and the terminal installer changes the certificate, and so does the first install with this signing. Grant Accessibility access once more after either.
- Start at login registers the app with the system. It shows under System Settings > General > Login Items, in Open at Login, with the app's name and icon.
- An install over a companion that an earlier release built as a bare program removes that program and its launch agent. When it started at login, the app does now.
{% </details> %}

### What ran on Linux {#linux-runs}

Every run below used containers on an Apple silicon Mac (Docker Desktop, `linux/arm64`), with no monitor, keyboard or login. WebKitGTK needs its sandbox, so the containers ran with `--privileged`.

| Check | Where | Result |
|-------|-------|--------|
| `companion install` names each missing package | Ubuntu 24.04, Fedora 43 | Pass: one `sudo apt install ...` or `sudo dnf install ...` line |
| `companion install` builds with exactly those packages | Ubuntu 24.04, Fedora 43 | Pass |
| The Arch package names exist and hold the `.pc` files | Arch Linux (`pacman -Si`, `pacman -F`), no build | Pass |
| The circle is round with a see-through background | Ubuntu 24.04: Xvfb with Openbox and picom (X11); Sway headless (Wayland) | Pass |
| The circle stays over other windows | Xvfb with Openbox | Pass |
| `Ctrl+Alt+Space` brings back a hidden companion with its list | Xvfb with Openbox, keys sent by `xdotool` | Pass |
| A second start brings back the running one | X11 and Wayland | Pass: same process |
| **Hide** from the right-click menu | Xvfb with Openbox | Pass: windows off the screen, server keeps reading |
| The list opens beside the circle | Xvfb with Openbox | Pass |
| `companion stop`, and `stop` when nothing runs | X11 and Wayland | Pass |
| `companion install` over a running companion | X11 and Wayland | Pass: new file, new process |
| **Start at login** writes `~/.config/autostart/agnostic-ai-companion.desktop` | X11 and Wayland | Pass |
| `companion uninstall` leaves no process, autostart entry or program | X11 and Wayland | Pass |
| `stop` and `uninstall` with no session bus | X11 and Wayland | Pass: as described under [Commands](#commands) |
| A Claude Code chat lists | X11 and Wayland, one session file | Pass |
| A click on an ended chat resumes it in a new terminal window | Xvfb with Openbox, `x-terminal-emulator` set to `xterm` | Pass |
| The Claude Code cloud sign-in is read from `~/.claude/.credentials.json` | Ubuntu 24.04, a fake token | Pass: the API refused it, as it should |

{% <details summary="Details: what did not run on Linux, and Wayland"> %}
- Not run: GNOME, KDE or any desktop with a real screen; a Wayland shortcut, which the desktop asks you to approve; a new shortcut; **Start at login** across a real login; a monitor unplugged; Codex and Cursor chats; working and waiting states; a real Claude Code sign-in; a cloud session opened in the browser.
- On Wayland the desktop places windows, not the program. Under Sway the circle opened in the middle of the screen as a floating window with a title bar, and its saved place was not used. This Sway rule puts it back in a corner without the title bar: `for_window [app_id="org.wails.agnostic-ai_companion"] border none, sticky enable, move position 1170 690`.
- Wayland has no standard way to keep a window over the others. Under Sway a floating window stays over tiled ones.
- A chat that still runs in a terminal cannot be switched to on Linux. The list says where it runs.
{% </details> %}

### What ran on Windows {#windows-runs}

Every run below used a GitHub Actions `windows-latest` runner: Windows Server 2025 (x64) with WebView2 157, signed in as an administrator, with a 1024x768 screen and no person at it. Clicks and keys came from a script.

| Check | Result |
|-------|--------|
| `companion install` without Go names it and builds nothing | Pass |
| `companion install` builds with Go alone | Pass |
| The circle shows, round, over other windows, with no console window of its own | Pass |
| A second start brings back the running one | Pass: same process |
| The right-click menu, and **Hide** from it | Pass: windows hidden, server keeps reading |
| `Ctrl+Alt+Space` brings back a hidden companion with its list | Pass |
| A Claude Code chat lists | Pass |
| The list says Codex chats are missing without `sqlite3` | Pass |
| `winget install SQLite.SQLite` puts `sqlite3` on `PATH` | Pass: SQLite 3.54.1 |
| A click on an ended chat resumes it in a new PowerShell window, in the chat's folder | Pass |
| A click on a chat written in the last day offers its command instead | Pass |
| A `codex://` link opens the app registered for it | Pass, with a stand-in app |
| **Start at login** writes its `Run` value, and `companion stop` | Pass |
| `companion install` over a running companion | Pass: new file, new process |
| `companion uninstall` leaves no process, `Run` value, program or companion folder | Pass |

{% <details summary="Details: what did not run on Windows"> %}
- Not run: Windows 11 or any desktop a person uses; a standard (not administrator) account; **Start at login** across a real login; a new shortcut; more than one screen; Codex and Cursor chats; working and waiting states; a real Claude Code sign-in; a cloud session opened in the browser; a real `claude` or `codex` resumed (neither is installed on the runner).
- `agnostic-ai serve`, which the companion starts, has a console window of its own, hidden.
- Windows gives no way to list another program's processes without extra tools, so the server cannot tell whether a terminal chat still runs. A chat written in the last day is never resumed, so two copies never compete. Its command shows instead.
- A chat that still runs in a terminal cannot be switched to on Windows, and a chat cannot be deleted there.
{% </details> %}

## Commands {#commands}

| Command | What it does |
|---------|--------------|
| `companion install` | Installs it. `--source` builds from the `companion` folder of an agnostic-ai checkout. |
| `companion` | Starts it, or brings back a hidden one and opens its list. |
| `companion stop` | Ends the companion and its server. Start at login stays as set. |
| `companion uninstall` | Ends it, turns off start at login, deletes what install made. |
| `companion install-command --app /Applications/agnostic-ai\ companion.app` | Links the app's bundled CLI into a command folder on macOS. |
| `companion update-app --app /Applications/agnostic-ai\ companion.app --version X.Y.Z` | Downloads the named stable release, replaces that app and its bundled CLI, then restarts it. |

{% <details summary="Details: stop and uninstall"> %}
- `stop` when nothing runs: it succeeds and says so.
- **Quit Agnostic AI companion** in the menus does the same as `stop`.
- `uninstall` deletes the built program, `companion.json` (its place, shortcuts, animation and dim choices), `companion-command` (where the CLI is) and `companion-revision` (the commit of the CLI that installed it).
- When the companion cannot run at all, such as one built for another processor, `uninstall` warns and still deletes it. A companion that runs but does not stop or turn off start at login keeps `uninstall` from deleting anything.
- `uninstall` keeps `conversation-marks.json`, which `serve` uses too.
- `stop` and `uninstall` manage the terminal-installed companion. For the disk-image app, quit from its menu, turn off **Start at login**, remove any CLI link you installed, then move the app from Applications to Trash.
- It also removes a login entry that a deleted program left behind: a file on macOS and Linux, the `agnostic-ai-companion` value under `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` on Windows.
- On Windows the companion keeps its browser data in the `companion` folder, which `uninstall` deletes. It also deletes the `agnostic-ai-companion.exe` folder that earlier companions left in `%AppData%`.
- On Linux the CLI reaches the companion over the D-Bus session bus. In a shell without one, such as an SSH login, `stop` says so and stops nothing, and `uninstall` warns and still deletes the files.
{% </details> %}

## The circle and its menus {#menus}

Right-click the circle, or open the list's ⋯ menu. A settings change in one shows in the other. The disk-image app also offers the update entry in both menus, and **Install command line tool** under it in the circle's right-click menu.

| Menu item | What it does |
|-----------|--------------|
| **Animate companion** | Movement on or off. Off, the circle shows a still ring. |
| **Dim when idle** | Fades the circle while nothing needs you. |
| **Small companion** | A 40px circle instead of 60px. |
| **Start at login** | Starts it when you log in. |
| **Open from anywhere** | Shows and changes the shortcut that opens the list. |
| **Send copied text** | Shows and changes the send shortcut. See [Send copied text](@/docs/cli-reference/companion-list.md#send-copied-text). |
| **Check for updates** | Disk-image app only. Checks for a newer release now. Reads **Update to vX.Y.Z** when one is known; choosing it asks, then updates and restarts. See [Install and start](#install). |
| **Hide Agnostic AI companion** | Takes the circle and the list off the screen. It keeps running. |
| **Quit Agnostic AI companion** | Asks first, then ends it and its server. |

{% <details summary="Details: count, animation, dim, small, shortcuts"> %}
- The circle shows a solid orange count with a lock and a halo for chats waiting for permission or an answer. Unread replies alone keep the softer count. A screen reader hears both numbers, such as "2 waiting for you, 4 unread". A request the [approvals](#approvals) hook approved never counts.
- The circle and the list's ⋯ menu hold the same items in the same order.
- Animate companion, Dim when idle and Small companion are kept across restarts.
- With Animate off, or Reduce Motion on, the count keeps updating. For what the list shows while animated, see [Working rows and subagents](@/docs/cli-reference/companion-list.md#working-rows).
- Send copied text: the shortcut is Ctrl+Alt+V until you change it.
- Dim: pointing at the circle, or any conversation that needs you, brings back full color.
- Small: its mark, working ring, count and window shrink with it, so clicks just outside the small circle reach the app behind it. It keeps the screen edges it sits nearest.
- Open from anywhere: in the right-click menu, choosing it opens the list on its ⋯ menu to change the keys.
- The open and send shortcuts cannot share keys.
- Keys the system refuses are kept, and the menu says why the shortcut is not active.
{% </details> %}

## Hide and quit {#hide-and-quit}

**Hide** takes the circle and the list off the screen. The shortcut, `agnostic-ai companion` or opening the app again brings them back and opens the list. **Quit** stops the program; run `agnostic-ai companion` to start it again.

| State | What happens |
|-------|--------------|
| Hidden | A chat that starts waiting for you brings the circle back at 25%, without opening the list. |
| Dimmed | A click on the circle or the shortcut brings back full color. |

{% <details summary="Details: hidden, dimmed and quit states"> %}
- Once quit, the shortcut cannot bring it back.
- Bringing it back returns the circle where it was. A hidden circle that returns shows its count.
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

## Approvals {#approvals}

With an active reviewer or approvals hook, some requests no longer need your answer. Run `agnostic-ai approvals status` to check each tool. The companion shows the decisions the hook logged.

| Mode | Routine request | Request the gate sends to you | Tool with no route |
|---|---|---|---|
| `ask` | "Needs permission" under Needs you, counted on the circle | Same | Same |
| `review` | The tool's reviewer handles the request | Under Needs you as soon as the tool prompts | Under Needs you when the tool prompts |
| `yolo` | The approvals hook handles the request and adds it to **N approved for you** | Under Needs you with a reason, such as "Always asks: publishing" | Under Needs you with a reason, such as "Approvals not supported for Cursor" |

- **N approved for you** folds out the latest approved requests under the row, newest first. It opens nothing.
- The reason line sits under the approval pill. agnostic-ai words it from the always-ask rule that matched, or from why approvals did not answer. A session's own text never reaches it, so a file in a repository cannot fake it.
- The header names the loosest active mode when it is `review` or `yolo`.
- **Ask me** writes `approvals.mode: ask` to `$AGNOSTIC_AI_HOME/local/agnostic-ai.yaml`, as `agnostic-ai approvals ask` does. The agnostic-ai hook stops approving requests right away. Run `agnostic-ai sync --global` to remove native reviewer settings that agnostic-ai wrote. In a running session, switch modes in the tool.

{% <details summary="Details: the brief Claude Code prompt, counts, limits"> %}
- With the `yolo` hook active, Claude Code can show its prompt while the hook runs. The companion lists the permission wait when the log says `asked` or after 10 seconds with no decision. Until then the row shows as working.
- An `approved` line holds the wait for 10 seconds after it. A prompt still open after that reaches you, with "Still waits after the approvals hook approved a request".
- A wait with no decision after 10 seconds reaches you with "No answer from the approvals hook".
- A `review` prompt reaches you as soon as the tool asks. For Claude Code with its reviewer on, the row says "The tool's reviewer left this to you".
- Codex waits for the hook before it prompts, so a Codex wait is never held.
- A cloud session runs no local hook. When a live cloud session waits for permission and its approvals mode is `review` or `yolo`, the row says "Approvals do not reach cloud sessions".
- The count comes from `$AGNOSTIC_AI_HOME/local/approvals/decisions.jsonl` and its older file. The count covers hook decisions. Native reviewers do not add entries. A logged detail that may hold a credential is left out.
- The mode is resolved per row from its tool and folder, by the order `approvals status` uses. A session started with `AGNOSTIC_AI_APPROVALS=ask` in its own environment is not known to the companion: its prompt can still wait up to 10 seconds when the companion sees an active `yolo` hook.
{% </details> %}

## How it reads conversations {#how-it-reads}

The companion reads no chat itself. It starts its own `agnostic-ai serve --global --api-only --cloud-sessions` on a free loopback port. Every 5 seconds it asks that server whether the list changed.

{% <details summary="Details: polling"> %}
- The request sends the list's `ETag`.
- While no chat changed, the server answers without reading anything.
{% </details> %}

## Upgrade the companion

`agnostic-ai upgrade` replaces an installed companion, and restarts it only if it was running. When the CLI is already current, as after a direct Homebrew or npm upgrade, it still replaces a companion built for another release.

{% <details summary="Details: failures and version mismatch"> %}
- The companion is downloaded or built as `companion install` does, and progress is printed.
- Settings and Start at login stay as they were.
- It installs none if none is installed.
- `upgrade --check` and `upgrade --requires` do not replace it.
- A companion update failure leaves the CLI upgraded and the old companion usable.
- When Homebrew or npm does not serve the new release yet, the CLI stays as it was and the error says to run `agnostic-ai upgrade` again later.
- The error names the failed step or missing build tool. Fix it, then run `agnostic-ai companion install` to retry.
- A fresh companion/server start shows a panel notice when their versions differ, with the command to run. Starting through the CLI also prints a warning.
- A companion and server already running before an external upgrade can keep their old versions until restarted.
{% </details> %}

## Cloud sessions {#claude-code-cloud-sessions}

Add the sessions that run in the cloud to the list.

1. Run `agnostic-ai serve --global --cloud-sessions`. The companion starts its server with this flag already.
2. Claude Code sessions from claude.ai/code and Codex cloud threads join the conversations, with `app` `cloud`.
3. They take the same states and marks as the others.

A project server lists none, because a cloud session names no folder. To send the current task to one from a session, use a [cloud handoff](@/docs/cloud-handoff.md). Both endpoints are undocumented and can change with any release of either tool.

| | Claude Code cloud | Codex cloud |
|---|---|---|
| Sign-in | The claude.ai login in the macOS keychain, or in `.credentials.json` on Linux. Run `claude` to renew it. | The ChatGPT login in `auth.json` |
| Opens | In the browser | In the Codex app |
| Working | Status `working` | Status `active` |
| Waiting for you | Status `blocked` | An approval or your input |

{% <details summary="Details: endpoints, privacy, refresh, failures, status"> %}
- Claude Code reads `GET https://api.anthropic.com/v1/code/sessions?limit=100`. Codex reads `GET https://codex-cloud-backend.chatgpt.com/v1/threads?limit=100`, the list the Codex app reads.
- The Claude Code sign-in is the claude.ai login in the macOS keychain (`Claude Code-credentials`). On Linux and Windows it is `.credentials.json` in `$CLAUDE_CONFIG_DIR` (default `~/.claude`), where Claude Code 2.1.330 for Linux and for Windows writes it. No real Linux or Windows sign-in was tried yet. The Codex sign-in is the ChatGPT login in `auth.json`, in `$CODEX_HOME` (default `~/.codex`).
- Claude Code sessions open `https://claude.ai/code/session_<id>` in the browser. Codex threads open `codex://threads/<id>?hostId=durable`, handled by the Codex app.
- Claude Code `working` shows only while the worker runs. Codex waits on an approval or on your input.
- Limit: at most 100 sessions or threads. It does not page past them yet.
- It uses their id, title or name, status and times. It never reads their messages.
- The sign-in is sent only to its own host. It is never kept, printed or logged, and the refresh token is never used.
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

| Row | Reads from | Opens in |
|-----|------------|----------|
| "Claude · Chat" (under Cloud) | The Claude desktop app | The Claude app |
| "Claude · Cowork" (under Local) | The Claude desktop app | The Claude app |
| "ChatGPT · Chat" | The ChatGPT desktop app's data | chatgpt.com |

{% <details summary="Details: waiting, sync, stability"> %}
- ChatGPT chats come from `codex.db` under `$CODEX_HOME/sqlite` (default `~/.codex`), when the ChatGPT desktop app is installed.
- A Claude chat the assistant answered counts as waiting for you until you see it.
- ChatGPT reads nothing over the network. A new reply shows within the companion's 5-second check.
- A ChatGPT chat you started on chatgpt.com or your phone shows once the app has synced it.
- The ChatGPT chat list is undocumented and can change with any release of the app.
{% </details> %}

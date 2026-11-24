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

The companion floats over your apps and counts Claude Code, Codex and Cursor chats that need you. Click the circle to [search and open chats](@/docs/cli-reference/companion-list.md). Use a row's menu to [stop a forgotten Claude Code shell](@/docs/cli-reference/companion-list.md#stop-a-forgotten-shell).

<span id="companion" hidden></span>

## Install and start {#install}

The disk image needs macOS 13 Ventura or later. It holds the companion, the matching CLI and the web dashboard, so it works without a separate agnostic-ai install.

1. Open **Apple menu > About This Mac**. An Apple chip uses the **Apple silicon** (`arm64`) download. An Intel processor uses **Intel** (`amd64`).
2. Open the [latest stable release](https://github.com/Chemaclass/agnostic-ai/releases/latest) and download the matching `.dmg`. If the release has no disk image for your Mac, [install from the terminal](#install-from-terminal).
3. Open the disk image, drag **agnostic-ai companion** into **Applications**, and eject the disk image. Open the app from **Applications** in Finder. If macOS says Apple could not verify it, click **Done**, then **Open Anyway** under **System Settings > Privacy & Security**.
4. Click the floating circle to see your conversations. Use **Open the dashboard** in the list header to open the full web interface in your browser.

To skip that alert, [install from the terminal](#install-from-terminal) instead. macOS checks apps downloaded in a browser, not the one the CLI installs.

The companion and dashboard use your [user-level settings](@/docs/global-configuration.md) in `~/.agnostic-ai`. Sessions from supported tools show up as the companion finds them. If the list is empty, start a conversation in Claude Code or Codex. The [Companion list](@/docs/cli-reference/companion-list.md) says which conversations it can show.

{% <details summary="First launch: Apple could not verify the app, and macOS permissions"> %}
The app is signed without an Apple Developer ID and is not notarized, so the first launch says Apple could not verify it. [Verify the downloaded disk image](@/docs/verify-a-release.md), then open it once:

1. Click **Done** in the alert.
2. Open **System Settings > Privacy & Security** and scroll to **Security**.
3. Click **Open Anyway** next to "agnostic-ai companion". It shows for about an hour after the blocked launch.
4. Confirm with **Open** and your password. Later launches open normally.

On macOS 15 and later, right-click **Open** no longer skips the alert. See [Apple's guide to macOS alerts](https://support.apple.com/en-us/102445).

macOS may ask for Automation when you [open a terminal chat](@/docs/cli-reference/companion-list.md#open-a-chat), or for Accessibility when you [paste into a terminal](@/docs/cli-reference/companion-list.md#send-copied-text). Only those actions need them.
{% </details> %}

{% <details summary="Use a different configuration folder or install the command line tool"> %}
The app uses `AGNOSTIC_AI_HOME` when its environment sets it. A Finder launch normally does not, so it uses the default folder: a setting in `.zshrc` reaches only that shell and what it starts.

To use the command in a terminal, right-click the circle and choose **Install command line tool**. The app links the bundled CLI into `/usr/local/bin` when that folder is writable and on `PATH`, or into `~/.local/bin`. Add the reported folder to your shell's `PATH` if needed. Existing commands, including npm and Homebrew installs, stay in place. Keep the app in Applications so the link still works.
{% </details> %}

{% <details summary="Update the disk-image app"> %}
The bundled CLI checks for a release on start and every 24 hours. A good answer is reused for 24 hours, even across restarts; a failed check runs again at the next start. Offline, the app keeps the last verified offer. When a newer stable release has a disk image for this Mac, both menus show **Update to vX.Y.Z** and the ⋯ button shows a dot. **Check for updates** checks now and reports release or checksum errors. **Check for updates automatically**, in either menu, turns the automatic checks off.

Choose **Update to vX.Y.Z**, then **Update and restart**. The app and its bundled CLI update together, and the list shows download, verification and restart progress. The download gets the CLI installer's checksum and provenance checks. It works without `gh`: the new app must match the installed app's project certificate before it runs or replaces it. After a certificate change, install the next disk image by hand once. An app still on the disk image, or in a read-only folder, must move to Applications before it can update.

The updater keeps the earlier app until the new one has stayed open for a few seconds. If macOS refuses to open the new app, or it quits or never appears, the updater restores and opens the earlier one. If that fails, it saves the earlier app and logs its path. The list shows the failure and offers **Retry**, also after a restart, and says whether the new app could not take its place or could not start. A second install, update or uninstall of the same app waits for the first to end. The log is `~/Library/Application Support/agnostic-ai/companion-update.log`.

`agnostic-ai upgrade` run through the bundled CLI link points you to the app menu. The app also looks for a separate CLI, such as a Homebrew or npm install. If that one is older, the list shows both versions and a command to run in your terminal.
{% </details> %}

### Install from the terminal {#install-from-terminal}

Use an existing CLI to install and start the companion:

1. Run `agnostic-ai companion install`. On macOS it downloads the build for your Mac. Elsewhere it builds the companion.
2. Run `agnostic-ai companion`. It starts the companion and returns.
3. Run it again to bring back a hidden companion and open its list.

| System | Status |
|--------|--------|
| macOS | Supported (macOS 13 or later). |
| Windows | Builds and runs on a GitHub Actions Windows Server 2025 runner. Not yet tried on a Windows 11 desktop. [Help with #3221](https://github.com/Chemaclass/agnostic-ai/issues/3221). |
| Linux | Builds and runs in containers, on X11 and Wayland. Not yet tried on a desktop with a real screen. [Help with #3222](https://github.com/Chemaclass/agnostic-ai/issues/3222). |

{% <details summary="Details: download, build, signing, login item"> %}
- Built and run on macOS 27 (arm64). On a Mac older than macOS 13, install stops and says why.
- Running `agnostic-ai companion` again tells you when the companion was built for another release, or by an `agnostic-ai` from another commit. When the companion cannot run, it says so and names its path.
- Download: on macOS, the release's build for your Mac (Apple silicon or Intel). An Intel `agnostic-ai` running through Rosetta gets the Apple silicon one. No compiler needed.
- Checks: the download must match the release's `checksums.txt`. With `gh` installed and logged in, it must also match the release's build provenance, as `agnostic-ai upgrade` checks the CLI. A download that fails either check stops the install and leaves the installed companion as it was. When `gh` itself fails, `AGNOSTIC_AI_VERIFY_ATTESTATION=0` installs on the checksum alone.
- Ctrl-C stops the install and removes its build folder. The next install also removes build folders older than an hour.
- When neither the new app nor the old one can take its place, the old one stays in the build folder, and the error names both paths.
- It builds instead of downloading for a release that ships no built companion, for an `agnostic-ai` built from any commit but a release tag (such as `make build` on `main`), and with `--source`. A build uses the same commit as the `agnostic-ai` you run, and prints each step.
- Build needs: Go everywhere, plus the Xcode Command Line Tools on macOS. Linux also needs a C compiler, `pkg-config`, and the GTK 4, WebKitGTK 6.0 and X11 development packages.
- When something is missing, the command names all of it at once, with how to get it, and builds nothing. On Linux that is one install line for `apt`, `dnf` or `pacman`, whichever the machine has. Without `pkg-config` it names that first, since the other packages are checked with it.
- A failed build leaves the installed companion as it was.
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
- A running chat's tab is selected in WezTerm, kitty, and `tmux` inside them, on X11 and under Sway (kitty only: WezTerm 20240203 opened no window under headless Sway). Each step ran in a container with stand-in sessions, not a real `claude`: selecting the tab, naming it, resuming an ended chat in a new tab, and `conversations start`. Other terminals are named, as GNOME Terminal and Alacritty were. agnostic-ai does not bring the window forward on Linux, so the reply says it may be behind.
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
| A running Claude Code chat (`claude.exe` 2.1.330) and Codex thread (0.171.0) show as open, and a click starts no second copy | Pass |
| Once ended, each shows as closed, and a click starts its resume command in a new PowerShell window | Pass |
| A running chat cannot be deleted; an archived ended chat can | Pass |
| A `codex://` link opens the app registered for it | Pass, with a stand-in app |
| The Claude Code cloud sign-in is read from `~/.claude/.credentials.json` | Pass with a fake token: the API refused it, as it should |
| **Start at login** writes its `Run` value, and `companion stop` | Pass |
| `companion install` over a running companion | Pass: new file, new process |
| `companion uninstall` leaves no process, `Run` value, program or companion folder | Pass |

{% <details summary="Details: what did not run on Windows"> %}
- Not run: Windows 11 or any desktop a person uses; a standard (not administrator) account; **Start at login** across a real login; a new shortcut; more than one screen; Cursor chats; working and waiting states; a real Claude Code or Codex sign-in; a cloud session opened in the browser.
- `agnostic-ai serve`, which the companion starts, has a console window of its own, hidden.
- A chat that still runs in a terminal shows as open, but the server cannot bring its window forward on Windows.
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
| `companion update-status --json` | Prints what the companion needs to offer an update: installed and latest release, install method and upgrade command. `--refresh` asks GitHub now. |

{% <details summary="Details: update-status fields"> %}
- `oneClick`: `agnostic-ai upgrade` can run the upgrade command.
- `confirmsRelease`: the CLI can check an approved release before upgrading.
- `image`, macOS only: the newer release lists a disk image for this Mac. `imageError` says why that is unknown.
- The answer is cached for up to a day and shared with `agnostic-ai upgrade`. An automatic check can use the last verified answer offline. With `--refresh`, a failed check is reported.
{% </details> %}

{% <details summary="Details: stop and uninstall"> %}
- `stop` with nothing running succeeds and says so.
- **Quit Agnostic AI companion** in the menus does the same as `stop`, after asking.
- `uninstall` deletes the built program, `companion.json` (its place, shortcuts, animation and dim choices) with its lock, `companion-state.json`, `companion-command` (where the CLI is) and `companion-revision` (the commit of the CLI that installed it).
- When the companion cannot run, such as one built for another processor, `uninstall` warns and still deletes it. A companion that runs but does not stop or turn off start at login keeps `uninstall` from deleting anything.
- `uninstall` keeps `conversation-marks.json`, which `serve` uses too.
- `stop` and `uninstall` manage the terminal-installed companion. For the disk-image app, quit from its menu, turn off **Start at login**, remove any CLI link you installed, then move the app from Applications to Trash.
- It also removes a login entry that a deleted program left behind: a file on macOS and Linux, the `agnostic-ai-companion` value under `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` on Windows.
- On Windows the companion keeps its browser data in the `companion` folder, which `uninstall` deletes. It also deletes the `agnostic-ai-companion.exe` folder older companions left in `%AppData%`.
- On Linux the CLI reaches the companion over the D-Bus session bus. In a shell without one, such as an SSH login, `stop` says so and stops nothing, and `uninstall` warns and still deletes the files.
{% </details> %}

## The circle and its menus {#menus}

Right-click the circle, or open the list's ⋯ menu. A settings change in one shows in the other. Both menus also hold the update entry. The disk-image app adds **Install command line tool** under it in the circle's right-click menu.

| Menu item | What it does |
|-----------|--------------|
| **Animate companion** | Movement on or off. Off, the circle shows a still ring. |
| **Dim when idle** | Fades the circle while nothing needs you. |
| **Small companion** | A 40px circle instead of 60px. |
| **Start at login** | Starts it when you log in. |
| **When a tool asks permission** and **Answer permission requests from the companion** | [Approvals settings](@/docs/approvals.md#change-it-from-the-companion). List menu only. |
| **Ask me before an agent sends a message** | Holds each mailbox message an agent sends until you approve it. On by default. List menu only. See [Approve agent messages](@/docs/mailbox.md#approve-agent-messages). |
| **Open from anywhere** | Shows and changes the shortcut that opens the list. |
| **Send copied text** | Shows and changes the send shortcut. See [Send copied text](@/docs/cli-reference/companion-list.md#send-copied-text). |
| **Check for updates** | Checks for a newer release now. Reads **Update to vX.Y.Z** when one is known; choosing it asks, then updates and restarts. See [Install and start](#install) for the disk-image app and [Upgrade the companion](#upgrade-the-companion) for a terminal install. |
| **Hide Agnostic AI companion** | Takes the circle and the list off the screen. It keeps running. |
| **Quit Agnostic AI companion** | Asks first, then ends it and its server. |

{% <details summary="Details: count, animation, dim, small, shortcuts"> %}
- The circle shows a solid orange count with a lock and a halo for chats waiting for you (a terminal when all wait on a shell). Unread replies alone keep the softer count. A screen reader hears both numbers, such as "2 waiting for you, 4 unread". A request the [approvals](#approvals) hook approved never counts.
- Animate companion, Dim when idle and Small companion are kept across restarts.
- With Animate off, or Reduce Motion on, the count keeps updating. For what the list shows while animated, see [Working rows and subagents](@/docs/cli-reference/companion-list.md#working-rows).
- Send copied text: the shortcut is Ctrl+Alt+V until you change it.
- Dim: pointing at the circle, or any conversation that needs you, brings back full color.
- Small: its mark, working ring, count and window shrink with it, so clicks just outside the small circle reach the app behind it. It keeps the screen edges it sits nearest.
- Open from anywhere: choosing it in the right-click menu opens the list's ⋯ menu, where you change the keys.
- The open and send shortcuts cannot share keys.
- When the system refuses a shortcut, the keys are kept and the menu says why it is not active.
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
- The quit question offers **Quit**, **Hide instead** and **Cancel**. Escape or a click elsewhere cancels.
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
- The header chip names the loosest mode: `review` or `yolo`.
- With [`approvals.companion: true`](@/docs/approvals.md#answer-from-the-companion), a held Claude Code request shows in full under the pill, with **Approve** and **Deny**.
- **Ask me** writes `approvals.mode: ask` to `$AGNOSTIC_AI_HOME/local/agnostic-ai.yaml`, as `agnostic-ai approvals ask` does. The hook stops approving at once. Run `agnostic-ai sync --scope user` to remove native reviewer settings it wrote. In a running session, switch modes in the tool.

{% <details summary="Details: the brief Claude Code prompt, counts, limits"> %}
- With the `yolo` hook active, Claude Code can show its prompt while the hook runs. The companion lists the permission wait when the log says `asked` or after 10 seconds with no decision. Until then the row shows as working.
- An `approved` line holds the wait for 10 seconds after it. A prompt still open after that reaches you, with "Still waits after the approvals hook approved a request".
- A wait with no decision after 10 seconds reaches you with "No answer from the approvals hook".
- A `review` prompt reaches you as soon as the tool asks. For Claude Code with its reviewer on, the row says "The tool's reviewer left this to you".
- Codex waits for the hook before it prompts, so a Codex wait is never held.
- A cloud session runs no local hook. When a live cloud session waits for permission and its approvals mode is `review` or `yolo`, the row says "Approvals do not reach cloud sessions".
- The count comes from `$AGNOSTIC_AI_HOME/local/approvals/decisions.jsonl` and its older file, so it covers only hook decisions, not native reviewers. A logged detail that may hold a credential is left out.
- The mode is resolved per row from its tool and folder, by the order `approvals status` uses. A session started with `AGNOSTIC_AI_APPROVALS=ask` in its own environment is not known to the companion: its prompt can still wait up to 10 seconds when the companion sees an active `yolo` hook.
{% </details> %}

## How it reads conversations {#how-it-reads}

The companion reads no chat itself. It starts its own `agnostic-ai serve --scope user --api-only --cloud-sessions` on a free loopback port. Every 5 seconds it asks that server whether the list changed.

{% <details summary="Details: polling"> %}
- The request sends the list's `ETag`.
- While no chat changed, the server answers without reading anything.
{% </details> %}

## Upgrade the companion

`agnostic-ai upgrade` replaces an installed companion, and restarts it only if it was running. When the CLI is already current, as after a direct Homebrew or npm upgrade, it still replaces a companion built for another release.

A terminal install uses the same menus and quiet automatic checks. On macOS, **Update and restart** runs `agnostic-ai upgrade` after you confirm the offer. The CLI first checks that GitHub still shows that release, then uses its install method. Homebrew and npm publish on their own schedules. The updater verifies the installed CLI version before replacing the companion. If the package manager has yet to publish the release, choose **Retry** later. If it already serves a newer release, the updater keeps that CLI and installs the matching companion.

Older CLIs, unknown install methods, and other systems use **Copy command** for `agnostic-ai upgrade` and **Release page** for the GitHub release. Finder and login launches on macOS also search `/opt/homebrew/bin`, `/usr/local/bin`, `~/.local/bin` and `~/go/bin`. If the update fails because npm lives elsewhere, as with nvm, use **Copy command** in the failure message and paste it into a terminal where npm is available.

{% <details summary="Details: what ran for updates"> %}
- On macOS 27.2.1 (arm64), checksums, build provenance and signatures passed for v0.97.0 and v0.98.0. `companion update-app` upgraded between them. The restarted app and its CLI reported v0.98.0, and preferences survived.
- The published npm CLI upgraded from v0.97.0 to v0.98.0 in a scratch prefix. A Finder-like `PATH` found Homebrew once the folders above were added. With npm absent, the update was offered and then failed. The CLI still reported v0.97.0.
- The signature of a quarantined v0.98.0 app copy verified. Gatekeeper assessment rejected it, and the policy check reported a missing notarization ticket.
- Browser tests exercised **Copy command** and **Release page**. The remaining Mac desktop checks cover panel updates, the Finder copy fallback, Accessibility, the Gatekeeper prompt and **Open Anyway**, and manual certificate rotation. Native Linux and Windows still need clipboard and browser launch checks.
{% </details> %}

{% <details summary="Details: failures and version mismatch"> %}
- The companion is downloaded or built as `companion install` does, and progress is printed.
- Settings and Start at login stay as they were.
- It installs none if none is installed.
- `upgrade --check` and `upgrade --requires` do not replace it.
- If the CLI upgrade succeeds and the companion update fails, the CLI stays upgraded. On macOS, a new companion that does not start is replaced by the earlier one, which starts again. The list shows the failure and log path when it can recover the result on the next start.
- When Homebrew or npm does not serve the new release yet, the CLI stays as it was and the error says to run `agnostic-ai upgrade` again later.
- The error names the failed step or missing build tool. Fix it, then run `agnostic-ai companion install` to retry.
- A fresh companion/server start shows a panel notice when their versions differ, with the command to run. Starting through the CLI also prints a warning.
- A companion and server already running before an external upgrade can keep their old versions until restarted.
{% </details> %}

## Cloud sessions {#claude-code-cloud-sessions}

Add the sessions that run in the cloud to the list.

1. Run `agnostic-ai serve --scope user --cloud-sessions`. The companion starts its server with this flag already.
2. Claude Code sessions from claude.ai/code and Codex cloud threads join the conversations, with `app` `cloud`.
3. They take the same states and marks as the others.

To resume a Claude Code cloud session in a terminal, choose **Copy resume command** from its row menu and run the copied command. The main action still opens the session in the browser.

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
- Limit: at most 100 sessions or threads per cloud.
- It uses their id, title or name, status and times. It never reads their messages.
- The sign-in is sent only to its own host. It is never kept, printed or logged, and the refresh token is never used.
- Codex sign-in: without `auth.json` it asks nothing and says nothing, because you do not use Codex. A login with an API key has no cloud threads, and `cloudNote` says so.
- Flag off: without `--cloud-sessions`, nothing reads your sign-in or leaves this machine.
- Refresh: at most once a minute, with a 5-second timeout and no retry.
- A cloud change reaches the list at most a minute late. The web interface shows it when you open or reload the page. The companion asks every 5 seconds, so it shows there at most 65 seconds late.
- Failures never hide local conversations.
- After a refusal, a timeout, or an answer this version cannot read, the last good list stays for up to 10 minutes and `cloudNote` stays empty.
- After those 10 minutes, no cloud row shows and `cloudNote` says why. A missing or expired sign-in does this at once.
- A system with no supported sign-in store lists none and says nothing.
- `cloudNote` holds one sentence for each cloud that could not be read.
- The Claude Code session page is the only web page the server opens.
- Any other status counts as not live.
- No local process shows whether a cloud conversation still runs, so its status is trusted for a day after its last write.
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

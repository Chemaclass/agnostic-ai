+++
title = "Companion"
description = "Install, start, and update the floating list of AI conversations that need your attention."
weight = 44

[extra]
group = "Reference"

[extra.moved]
waiting = "@/docs/cli-reference/companion-list.md"
chats-archived-in-their-app = "@/docs/cli-reference/companion-list.md"
delete-an-archived-chat = "@/docs/cli-reference/companion-list.md"
+++

# Companion

Agnostic Companion is a floating circle that counts chats waiting for your input or permission. When none are waiting, it counts unread replies. Click the circle to [search and open conversations](@/docs/cli-reference/companion-list.md) from Claude Code, Codex, and Cursor. Choose **Open the dashboard** in the list to open the web interface.

## Install and start {#install}

### macOS disk image

The disk image requires macOS 13 Ventura or later and includes the app, CLI, and web dashboard.

1. In **Apple menu > About This Mac**, check whether you have an Apple chip or an Intel processor.
2. From the [latest stable release](https://github.com/Chemaclass/agnostic-ai/releases/latest), download `agnostic-companion_darwin_arm64.dmg` for Apple silicon or `agnostic-companion_darwin_amd64.dmg` for Intel.
3. Open the disk image, drag **Agnostic Companion** into **Applications**, and eject the image.
4. Open **Agnostic Companion** from **Applications**, then click the circle.

{% <details summary="First launch, macOS permissions, and the bundled CLI"> %}
If macOS says it cannot verify the app, [verify the download](@/docs/verify-a-release.md). Click **Done**, then open **System Settings > Privacy & Security** and choose **Open Anyway**. The app uses the project's signing certificate and is not notarized. Installing from the terminal avoids this alert.

macOS may ask for Automation when you [open a terminal chat](@/docs/cli-reference/companion-list.md#open-a-chat) or Accessibility when you [send copied text to one](@/docs/cli-reference/companion-list.md#send-copied-text).

To use the bundled CLI in a terminal, right-click the circle and choose **App > Install command line tool**. The app links its CLI into `/usr/local/bin` or `~/.local/bin` and leaves existing commands in place. Add the reported folder to your `PATH` if needed. Keep the app in Applications so the link works.
{% </details> %}

### Install from the terminal {#install-from-terminal}

With [agnostic-ai installed](@/docs/installation.md), run:

```sh
agnostic-ai companion install
agnostic-ai companion
```

On macOS, the CLI downloads and verifies the build for your Mac when the release provides one. Otherwise it builds from source. A build needs Go, plus Xcode Command Line Tools on macOS. On Linux it also needs a C compiler, `pkg-config`, and the GTK 4, WebKitGTK 6.0, and X11 development packages. The installer names missing tools and prints package commands for supported Linux distributions.

#### Linux {#linux-runs}

Tested in virtual X11 and Wayland sessions. Under Wayland, the desktop controls where the circle appears and whether it stays above other windows. [Native desktop verification](https://github.com/Chemaclass/agnostic-ai/issues/3222) is open.

#### Windows {#windows-runs}

Tested on a Windows Server 2025 CI runner. [Windows 11 desktop verification](https://github.com/Chemaclass/agnostic-ai/issues/3221) is open.

See the [platform test results](https://github.com/Chemaclass/agnostic-ai/blob/main/docs/internal/companion-platform-checks.md) for the environments tested. Local Codex and Cursor chats need [`sqlite3` on `PATH`](@/docs/cli-reference/serve.md#conversations-page).

## Commands {#commands}

| Command | What it does |
|---|---|
| `agnostic-ai companion install` | Installs or replaces the terminal-installed app. Use `--source <path>` to build from the `companion` folder of a checkout. |
| `agnostic-ai companion` | Starts the app or opens the running app's list. |
| `agnostic-ai companion stop` | Stops a terminal-installed companion and its server. For the disk-image app, choose **Quit Agnostic Companion**. |
| `agnostic-ai companion uninstall` | Stops and removes a terminal-installed companion, disables start at login, and removes the app settings. Keeps conversation marks. |
| `agnostic-ai companion update-status --json` | Prints the installed and latest stable releases, install method, and available upgrade command. Add `--refresh` to check GitHub now. |

{% <details summary="Commands for the macOS disk-image app"> %}
```sh
agnostic-ai companion install-command --app "/Applications/Agnostic Companion.app"
agnostic-ai companion update-app --app "/Applications/Agnostic Companion.app" --version X.Y.Z
```

The first command links the bundled CLI into a command folder. The second replaces the app and bundled CLI with the specified stable release, then restarts the app.

To remove a disk-image install, turn off **App > Start at login**, quit the app, remove any CLI link you installed, and move the app to Trash.
{% </details> %}

## Upgrade the companion {#upgrade-the-companion}

In either menu, choose **App > Updates > Check for updates**. Automatic checks run at startup and once a day while the app runs. You can check manually with automatic checks off.

| How you installed | How to update |
|---|---|
| macOS disk image | Choose **Update to vX.Y.Z**, then **Update and restart**. This updates the app and its bundled CLI together. |
| Terminal | Run [`agnostic-ai upgrade`](@/docs/cli-reference/upgrades.md), or use **Update and restart** when offered. |
| CLI updated directly through Homebrew or npm | Run `agnostic-ai companion install` with the updated CLI to match versions. |

If the menu offers a manual update, use **Release page** or **Copy command** where available. If npm is missing from the app's `PATH`, run the copied command in a terminal where npm works. If the app cannot verify the npm installation, update through your package manager, then run `agnostic-ai companion install`.

{% <details summary="Update failures and macOS signing"> %}
Move the disk-image app to a writable location such as **Applications** before updating. The updater verifies the release and requires the same signer as the installed app. If the signer changes, install the new disk image by hand. Grant Accessibility again after switching between a disk-image install and a terminal install.

If a macOS replacement fails to start, the updater tries to restore the previous app. The disk-image updater writes to `~/Library/Application Support/agnostic-ai/companion-update.log`. If a terminal update upgrades the CLI but fails to replace the companion, fix the reported error and run `agnostic-ai companion install` again.

Older disk images can update through v1.0.0 and for at least 30 days after the first renamed release, whichever is later. If an older app stops offering updates, replace it with the current disk image. Keep the app at the same path if you use its bundled CLI link.
{% </details> %}

## Menus and shortcuts {#menus}

Right-click the circle or open **⋯** in the list.

| Menu | What you can change |
|---|---|
| **Appearance** | Turn animation on or off, dim the circle when idle, or make it smaller. |
| **Shortcuts** | Change **Open from anywhere** (default **Ctrl+Alt+Space**, **⌃⌥Space** on macOS) or [**Send copied text**](@/docs/cli-reference/companion-list.md#send-copied-text) (default **Ctrl+Alt+V**). Find **Keyboard shortcuts** in the list menu. |
| **Permissions** | Change [tool approvals](#approvals) and [approval for agent messages](@/docs/mailbox.md#approve-agent-messages). Available in the list menu. |
| **App** | Start at login, check for updates, hide, or quit. |

### Hide and quit {#hide-and-quit}

**Hide Agnostic Companion** clears the screen while the app keeps watching for chats. The shortcut, reopening the app, or running `agnostic-ai companion` brings back the list. A newly waiting chat brings back just the faded circle.

**Quit Agnostic Companion** asks for confirmation and stops the app and its server. Open the app again or run `agnostic-ai companion` to use the shortcut again. The **Start at login** setting stays as you set it.

## Approvals {#approvals}

Requests that need a decision appear under **Needs you**. Expand **N approved for you** on a chat to see recent approvals made by the hook. Run `agnostic-ai approvals status` to check the mode for each tool.

To answer supported Claude Code and Codex requests in the list, enable [**Answer permission requests from the companion**](@/docs/approvals.md#answer-from-the-companion). To switch future requests back to manual approval, choose **Ask me**. See [turn approvals off](@/docs/approvals.md#turn-it-off-fast) for running sessions and reviewer settings.

## How it reads conversations {#how-it-reads}

The app starts a local [`agnostic-ai serve`](@/docs/cli-reference/serve.md) process with user scope and checks for list changes every five seconds. It uses your [user-level configuration](@/docs/global-configuration.md). A Finder launch uses the default configuration folder unless its environment sets `AGNOSTIC_AI_HOME`.

## Cloud sessions {#claude-code-cloud-sessions}

Companion includes Claude Code sessions from claude.ai/code and Codex cloud threads when you are signed in. For a standalone web dashboard, run `agnostic-ai serve --scope user --cloud-sessions`.

| Session | Opens in | Sign-in |
|---|---|---|
| Claude Code cloud | Browser. Use **Copy resume command** in the row menu to resume in a terminal. | Claude Code's claude.ai login, stored in the macOS Keychain or `.credentials.json` on Linux and Windows. |
| Codex cloud | Codex app | Codex's ChatGPT login, stored in `auth.json`. |

The server fetches titles, status, and timestamps from each cloud service. Local conversations stay available if a cloud service fails.

### Claude Code Remote Control {#remote-control}

A local Claude Code session shared with claude.ai appears once, marked **Shared**, under both Local and Cloud. Select the row to open it locally, or choose **Open on claude.ai** for the cloud page. See [how shared rows behave after the session closes](@/docs/cli-reference/companion-list.md#remote-control).

### Claude chats, Cowork tasks, and ChatGPT chats {#other-chats}

Companion reads chats and tasks stored by the Claude and ChatGPT desktop apps. Claude chats and Cowork tasks open in Claude. ChatGPT chats open on chatgpt.com once they have synced to the desktop app.

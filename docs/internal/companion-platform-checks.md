# Companion platform checks

These runs record what worked on virtual displays. Native desktop verification is tracked in [Linux #3222](https://github.com/Chemaclass/agnostic-ai/issues/3222) and [Windows #3221](https://github.com/Chemaclass/agnostic-ai/issues/3221). See [Companion signing and updates](companion-signing.md#first-live-update) for update checks.

## Linux

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
| Before the rename, **Start at login** wrote an entry under `~/.config/autostart/` | X11 and Wayland | Pass |
| `companion uninstall` leaves no process, autostart entry or program | X11 and Wayland | Pass |
| `stop` and `uninstall` with no session bus | X11 and Wayland | Pass: stop reports the missing bus. Uninstall warns and removes the files. |
| A Claude Code chat lists | X11 and Wayland, one session file | Pass |
| A click on an ended chat resumes it in a new terminal window | Xvfb with Openbox, `x-terminal-emulator` set to `xterm` | Pass |
| The Claude Code cloud sign-in is read from `~/.claude/.credentials.json` | Ubuntu 24.04, a fake token | Pass: the API refused it, as it should |

### Limits and desktop behavior

- Not run: GNOME, KDE or any desktop with a real screen; a Wayland shortcut, which the desktop asks you to approve; a new shortcut; **Start at login** across a real login; a monitor unplugged; Codex and Cursor chats; working and waiting states; a real Claude Code sign-in; a cloud session opened in the browser.
- On Wayland the desktop places windows, not the program. Under Sway the circle opened in the middle of the screen as a floating window with a title bar, and its saved place was not used. This Sway rule puts it back in a corner without the title bar: `for_window [app_id="org.wails.agnostic-ai_companion"] border none, sticky enable, move position 1170 690`.
- Wayland has no standard way to keep a window over the others. Under Sway a floating window stays over tiled ones.
- A running chat's tab is selected in WezTerm, kitty, and `tmux` inside them, on X11 and under Sway (kitty only: WezTerm 20240203 opened no window under headless Sway). Containers with stand-in sessions covered four actions: selecting the tab, naming it, resuming an ended chat in a new tab, and `conversations start`. Other terminals are named, as GNOME Terminal and Alacritty were. agnostic-ai does not bring the window forward on Linux, so the reply says it may be behind.

## Windows

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
| Deleting a running chat is blocked. Deleting an archived, ended chat works. | Pass |
| A `codex://` link opens the app registered for it | Pass, with a stand-in app |
| The Claude Code cloud sign-in is read from `~/.claude/.credentials.json` | Pass with a fake token: the API refused it, as it should |
| **Start at login** writes its `Run` value | Pass |
| `companion install` over a running companion | Pass: new file, new process |
| `companion uninstall` leaves no process, `Run` value, program or companion folder | Pass |

### Limits and desktop behavior

- Not run: Windows 11 or any desktop a person uses; a standard (not administrator) account; **Start at login** across a real login; a new shortcut; more than one screen; Cursor chats; working and waiting states; a real Claude Code or Codex sign-in; a cloud session opened in the browser.
- `agnostic-ai serve`, which the companion starts, has a console window of its own, hidden.
- A chat that still runs in a terminal shows as open, but the server cannot bring its window forward on Windows.

# agnostic-ai widget

A small circle that floats over every app and shows how many of your AI
conversations need you. Click it for a searchable list of your Claude Code,
Codex and Cursor chats, and open any of them where it lives.

It is a separate app from the `agnostic-ai` command. It reads the same stores
and keeps the same marks as `agnostic-ai serve`, in its own process, so no
server has to run.

## Run it

From the repository root:

```sh
make widget-run
```

`make widget` builds `bin/agnostic-ai-widget` without starting it. Releases do
not ship the widget yet, so building it is the only way to get it.

## Use it

- **Drag** the circle anywhere. It remembers where you left it, and comes back
  to the lower right when that place is on a monitor you unplugged.
- **Click** the circle to open or close the list. So does the shortcut,
  **Ctrl+Alt+Space** (⌃⌥Space on a Mac) until you change it.
- **Type** to search every conversation, of any age.
- **↑ ↓** move through the list and **Enter** opens the chat. So does a click.
- Hover a row to **mark it as read** or **mark it as done**. **Mark all read**
  beside Needs you clears every new reply at once; a chat that waits on a
  prompt stays.
- **Dashboard** opens the full agnostic-ai web interface.
- **Esc**, or a click anywhere else, closes the list.

The number on the circle counts the conversations that need you. An ice blue arc
turns around the circle while an agent is working.

Starting the widget a second time opens the list of the one that already runs.

## Settings

The ⋯ button at the foot of the list holds them:

- **Start at login** starts the widget when you log in. The system keeps this
  setting; it points at the widget where it is now, so turn it off and on
  again after you move the file.
- **Open from anywhere** shows the shortcut. Click it, then press the new keys:
  at least one of Ctrl, Alt or Cmd (the Windows or Super key elsewhere), and a
  letter, digit, Space, an arrow or a function key. A shortcut takes its keys
  away from every other app, so pick one nothing else uses. Keys another app
  already holds are refused and the old shortcut stays. macOS, Windows and X11
  ask for no permission; a Wayland desktop asks you once to approve it.
- **Quit widget**.

## What each group means

Each conversation is in the first group whose rule holds.

| Group | Rule |
| --- | --- |
| Needs you | Open, and its tool says it waits for you. Or something was written after you last saw it. |
| Working | Open, and its tool says the agent is busy. |
| Active | Open, or written in the last 24 hours. |
| Done | You marked it as done and nothing was written since. A new message moves it back to Needs you. |
| Dormant | Closed and quiet for more than 24 hours. |

Only Claude Code reports waiting and busy. Codex and Cursor chats reach Needs
you when something new is written in them.

You see a conversation when you open it from the widget or the web interface,
or mark it as read or done. A chat you read in its own terminal still counts
as unread until then.

## Files it keeps

Both are in the `agnostic-ai` folder of your user configuration folder
(`~/Library/Application Support` on macOS, `%AppData%` on Windows,
`~/.config` on Linux):

- `conversation-marks.json`: what you saw and what you marked as done, shared
  with `agnostic-ai serve`. Tool names, ids and times only.
- `widget.json`: where you left the circle, and your shortcut.

## Systems

| System | State |
| --- | --- |
| macOS | Built and run on macOS 27 (arm64). |
| Windows | Compiles; not yet run on a Windows machine. Build with `-ldflags="-s -w -H windowsgui"` so no console window opens. |
| Linux | Not yet built or run. Needs cgo with the GTK and WebKitGTK development packages. Wayland compositors decide what stays on top, so the circle may not float there. |

Codex and Cursor chats need the `sqlite3` command, as in the web interface.

## What it costs

Measured on an Apple silicon Mac with 146 Claude Code sessions, a Codex store
and a Cursor store, CPU time including the helper programs it starts:

| When | Cost |
| --- | --- |
| Nothing was written in any tool | 2 ms every 5 seconds, to check the files' sizes and times, and one full read a minute |
| A chat is being written | about 22 ms every 5 seconds |
| A full read, with Codex and Cursor read again | about 133 ms |

Codex and Cursor keep their chats in SQLite, read through the `sqlite3`
command. Their last result is kept until their files change, for at most a
minute, so a chat that changes shows within 5 seconds, and a Codex or Cursor
chat can show as open for up to a minute after its app quit. Memory is about
41 MB for the widget and about 20 MB for each of its two web views. The
download is about 10 MB.

## Layout

- `main.go`: the two windows (the circle and the list) and how they open and close.
- `inbox.go`: checks the stores every 5 seconds, reads them when one changed, and answers the pages.
- `preferences.go`: the shortcut and the start-at-login setting.
- `dashboard.go`: opens the full web interface.
- `place.go`: where the list opens beside the circle, and whether a saved place is still on a screen.
- `settings.go`: what the widget remembers between runs.
- `frontend/`: the pages, in plain HTML, CSS and JavaScript with no build step.
  `model.js` holds the logic and has no DOM in it.

This folder is its own Go module, so [Wails](https://wails.io), which draws the
windows, is not a dependency of the `agnostic-ai` command and adds nothing to
its binary.

## Test it

```sh
make widget-test
```

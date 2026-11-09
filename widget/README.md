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

- **Drag** the circle anywhere. It remembers where you left it.
- **Click** the circle to open or close the list.
- **Type** to search every conversation, of any age.
- **↑ ↓** move through the list and **Enter** opens the chat. So does a click.
- Hover a row to **mark it as read** or **mark it as done**.
- **Esc**, or a click anywhere else, closes the list.
- The power button at the foot of the list quits the widget.

The number on the circle counts the conversations that need you. An ice blue arc
turns around the circle while an agent is working.

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
- `widget.json`: where you left the circle.

## Systems

| System | State |
| --- | --- |
| macOS | Built and run on macOS 27 (arm64). |
| Windows | Compiles; not yet run on a Windows machine. Build with `-ldflags="-s -w -H windowsgui"` so no console window opens. |
| Linux | Not yet built or run. Needs cgo with the GTK and WebKitGTK development packages. Wayland compositors decide what stays on top, so the circle may not float there. |

Codex and Cursor chats need the `sqlite3` command, as in the web interface.

## Layout

- `main.go`: the two windows (the circle and the list) and how they open and close.
- `inbox.go`: reads the conversations every 5 seconds and answers the pages.
- `place.go`: where the list opens beside the circle.
- `settings.go`: the circle's saved place.
- `frontend/`: the pages, in plain HTML, CSS and JavaScript with no build step.
  `model.js` holds the logic and has no DOM in it.

This folder is its own Go module, so [Wails](https://wails.io), which draws the
windows, is not a dependency of the `agnostic-ai` command and adds nothing to
its binary.

## Test it

```sh
make widget-test
```

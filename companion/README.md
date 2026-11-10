# agnostic-ai companion

A small circle that floats over every app and shows how many of your AI
conversations need you. Click it for a searchable list of your Claude Code,
Codex and Cursor chats, and open any of them where it lives.

It is a window on the `agnostic-ai` command and reads no chat itself. It
starts its own `agnostic-ai serve --global --cloud-sessions`, and that server
reads, opens and marks the conversations. So the companion and the web interface always agree,
and an upgrade of agnostic-ai is an upgrade of what the companion shows.

## Get it

```sh
agnostic-ai companion install   # build it on this machine; again after each upgrade
agnostic-ai companion           # start it
```

`install` builds the companion from the same commit as your `agnostic-ai`,
with Go, and says how far it is. Releases ship no built companion, so nothing
has to be signed or published for each system. It names what your machine
lacks before it builds anything; see [Systems](#systems).

To work on the companion itself, from the repository root: `make companion-run`
builds it and the CLI and starts it on that CLI. `make companion` only builds
`bin/agnostic-ai-companion`.

## Use it

- **Drag** the circle anywhere. It remembers where you left it, and comes back
  to the lower right when that place is on a monitor you unplugged.
- **Click** the circle to open or close the list. So does the shortcut,
  **Ctrl+Alt+Space** (⌃⌥Space on a Mac) until you change it.
- **Type** to search every conversation, of any age.
- **↑ ↓** move through the list and **Enter** opens the chat. So does a click.
- Hover a row for its one button: **archive** a chat, or **hide** one that
  is already archived. **Archive all** beside Active archives every active
  chat at once.
- An archived chat comes back when you open it or when something new is
  written in it. A hidden chat leaves every list and search in agnostic-ai
  and comes back only with a new message. The tool that owns the chat keeps
  it either way.
- **Dashboard** opens the full agnostic-ai web interface.
- **Esc**, or a click anywhere else, closes the list.

The number on the circle counts the conversations that need you. An ice blue arc
turns around the circle while an agent is working.

Starting the companion a second time brings back the one that already runs and
opens its list.

## Settings

The ⋯ button at the foot of the list holds them:

- **Start at login** starts the companion when you log in. The system keeps this
  setting; it points at the companion where it is now, so turn it off and on
  again after you move the file.
- **Open from anywhere** shows the shortcut. Click it, then press the new keys:
  at least one of Ctrl, Alt or Cmd (the Windows or Super key elsewhere), and a
  letter, digit, Space, an arrow or a function key. A shortcut takes its keys
  away from every other app, so pick one nothing else uses. Keys another app
  already holds are refused and the old shortcut stays. macOS, Windows and X11
  ask for no permission; a Wayland desktop asks you once to approve it.
- **Hide Agnostic AI companion** takes the circle off the screen and stops
  the companion's server. Nothing reads or asks anything after that; only the
  shortcut listens. The shortcut, or `agnostic-ai companion`, brings it back
  with a new list.

There is no quit: a hidden companion does nothing but wait for the shortcut.

## What each group means

Each conversation is in the first group whose rule holds.

| Group | Rule |
| --- | --- |
| Needs you | Open, and its tool says it waits for you. Or something was written after you last saw it. |
| Working | Open, and its tool says the agent is busy. |
| Active | Open, or written in the last 24 hours. |
| Archived | You archived it and nothing was written since, or it is closed and was quiet for more than 24 hours. A new message moves it back to Needs you. |

Only Claude Code reports waiting and busy. Codex and Cursor chats reach Needs
you when something new is written in them.

## Claude Code cloud sessions

The list also shows the Claude Code sessions that run at claude.ai/code. They
sit in the same groups as the others, with **Cloud** in their second line.

- **What it reads.** The server asks `api.anthropic.com` for the list of your
  cloud sessions: id, title, status and times, at most 100 of them. It never
  reads a session's messages.
- **When.** At most once a minute, with a 5-second timeout. A cloud session that
  starts, stops or needs you shows in the list at most about a minute late
  (65 seconds).
- **The sign-in.** The server reads the claude.ai sign-in that Claude Code keeps
  in the macOS keychain, afresh for each call, and sends it only to
  `api.anthropic.com`. It never keeps, prints or logs it. Run `claude` to sign in or
  renew it. Windows and Linux are not supported yet: there the list shows no
  cloud sessions and says why.
- **When it fails.** Your local chats still show, no cloud session shows, and the
  note above the list says why. An old cloud list is never shown as current.
- **Opening one** opens claude.ai/code in your browser: a session's own page
  cannot be linked from what the list holds, so find it there by its title.
- **It can break.** The endpoint is undocumented, so any Claude Code release can
  change it. The list then shows your local chats and says the cloud answer could
  not be read.

You see a conversation when you open it from the companion or the web interface,
or archive it. A chat you read in its own terminal still counts
as unread until then.

## Files it keeps

Both are in the `agnostic-ai` folder of your user configuration folder
(`~/Library/Application Support` on macOS, `%AppData%` on Windows,
`~/.config` on Linux):

- `conversation-marks.json`: what you saw and what you archived, shared
  with `agnostic-ai serve`. Tool names, ids and times only.
- `companion.json`: where you left the circle, and your shortcut.

## Systems

| System | State |
| --- | --- |
| System | To build it | State |
| --- | --- | --- |
| macOS | Go, Xcode Command Line Tools | Built and run on macOS 27 (arm64). |
| Windows | Go | Compiles; not yet run on a Windows machine. |
| Linux | Go, a C compiler, the GTK 4, WebKitGTK 6.0 and X11 development packages | Not yet built or run. Wayland compositors decide what stays on top, so the circle may not float there. |

Codex and Cursor chats need the `sqlite3` command, as in the web interface.

## When it reads

| The companion is | What runs |
| --- | --- |
| Hidden | The shortcut listener. No server, no timer, no reads. |
| On screen | Every 5 seconds it asks its server whether the list changed. The server compares the files' sizes and times and answers "unchanged" without reading a chat. It reads only when a chat or a mark changed, and once when a new minute begins. |
| Searching | One read for each search, and again when a chat changes while the search shows. |

What you see is never older than this:

- A new message, a new chat, or a mark: 5 seconds.
- A Claude Code chat that starts or stops working or waiting: 5 seconds.
- An app or process that ended without writing anything, a chat that went
  dormant, and any change to a cloud session: the turn of the minute, so at
  most 65 seconds.
- A list that could not be read is not shown. The circle loses its number and
  the list says why, instead of showing the last one as if it were current.
- After the companion was hidden, the first list is a new read.

COSTS
## Layout

- `main.go`: the two windows (the circle and the list) and how they open and close.
- `server.go`: starts and stops the companion's `agnostic-ai serve`, and calls it.
- `inbox.go`: asks the server every 5 seconds whether the list changed, keeps the latest one, and hands the pages' opens and marks to the server.
- `preferences.go`: the shortcut and the start-at-login setting.
- `dashboard.go`: opens the full web interface.
- `place.go`: where the list opens beside the circle, and whether a saved place is still on a screen.
- `settings.go`: what the companion remembers between runs.
- `frontend/`: the pages, in plain HTML, CSS and JavaScript with no build step.
  `model.js` holds the logic and has no DOM in it.

This folder is its own Go module, so [Wails](https://wails.io), which draws the
windows, is not a dependency of the `agnostic-ai` command and adds nothing to
its binary. It imports nothing from the `agnostic-ai` module: what it knows
about conversations comes over the server's API, documented under
[serve](../docs/site/content/docs/cli-reference/inspect.md).

## Test it

```sh
make companion-test
```

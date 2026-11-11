# agnostic-ai companion

A small circle that floats over every app and shows how many of your AI
conversations need you. Click it for a searchable list of your Claude Code,
Codex and Cursor chats, and open any of them where it lives.

It is a window on the `agnostic-ai` command and reads no chat itself. It
starts its own `agnostic-ai serve --global --api-only --cloud-sessions`, and
that server reads, opens and marks the conversations. So the companion and the
web interface always agree,
and an upgrade of agnostic-ai is an upgrade of what the companion shows.

## Get it

```sh
agnostic-ai companion install     # build it on this machine
agnostic-ai companion             # start it
agnostic-ai companion stop        # end it
agnostic-ai companion uninstall   # remove it from this machine
```

`install` builds the companion from the same commit as your `agnostic-ai`,
with Go, and prints each step as it goes. Releases ship no built companion, so nothing
has to be signed or published for each system. It names what your machine
lacks before it builds anything. [companion](../docs/site/content/docs/cli-reference/companion.md#companion)
lists what each system needs. On macOS it installs the companion as an app,
`agnostic-ai companion.app`, with the agnostic-ai icon, signed on your machine.
Only macOS is supported; Windows
([#3221](https://github.com/Chemaclass/agnostic-ai/issues/3221)) and Linux
([#3222](https://github.com/Chemaclass/agnostic-ai/issues/3222)) need someone to run and verify it.

Codex and Cursor chats need the `sqlite3` command, as in the web interface.

To work on the companion itself, from the repository root: `make companion-run`
builds it and the CLI and starts it on that CLI. `make companion` only builds
`bin/agnostic-ai-companion`.

To work on its pages, `make companion-preview` serves the real bubble and list
in your browser with made-up conversations, and prints the address. Pick a
fixture at the top: an empty inbox, many sessions, long names, duplicate folder
names, failures. Saving a file under `frontend/` reloads the pages; no rebuild,
Wails, server or sign-in is needed. Calls to open, mark or rename change only
the fixture in that process. The preview runs in a browser, not in the app's
web view, so check focus, window placement, the shortcut and changes to Go code
with `make companion-run`.

## Upgrade

`agnostic-ai upgrade` also rebuilds an installed companion from the updated CLI's commit. It prints each build step and restarts the companion if it was running. A stopped companion stays stopped. Your settings and Start at login choice stay as they were. If no companion is installed, the upgrade does not build one.

If the companion update fails, the CLI stays upgraded and the old companion stays usable. The command explains the failed step or missing build tool. Fix it, then run `agnostic-ai companion install` to retry.

After upgrading directly through Homebrew or npm, run `agnostic-ai companion install`. On the next companion/server start, the panel shows a notice if their versions differ, with that command. Starting through `agnostic-ai companion` also prints a warning. A companion and server already running before an external upgrade can keep their old versions until restarted.

## Use it

- **Drag** the circle anywhere. It remembers where you left it, and comes back
  to the lower right when that place is on a monitor you unplugged.
- **Click** the circle to open or close the list. So does the shortcut,
  **Ctrl+Alt+Space** (⌃⌥Space on a Mac) until you change it.
- **Type** to search every conversation, of any age.
- **All, Local, Cloud** at the foot narrows the list to the chats on this
  machine or to the ones that run in a tool's cloud. It remembers your choice.
  The number on the circle always counts both.
- A row names the chat's folder when it has one. When two folders in the list end in the
  same name, the row adds the parent folders that tell them apart, such as
  `shop/api` and `billing/api`. Hover a row for its full folder path.
- Under its title, a row shows the agent's latest reply in one line: the
  question it asks you, or the start of its last message. Local Claude Code
  and Codex chats show it. Cursor chats and cloud sessions do not yet.
- A chat started with `agnostic-ai conversations start` names the chat it
  came from. The list reads that link, but does not yet show the new chat
  under its parent.
- **↑ ↓** move through the list and **Enter** opens the chat. So does a click.
  A running chat comes up in the Terminal or Ghostty tab that runs it, ready
  for typing. When only the app can come forward, the list comes back and
  says where the chat runs. macOS asks once to let the companion control each app.
  The reason stays above the list until you dismiss it or open another chat.
  When a chat could not be resumed, **Copy resume command** copies the command
  to run in a terminal. A Claude Code or Codex chat still running in a
  terminal never offers it, because that command would start a second copy.
- Hover a row for its one button: **mark complete** a chat, which archives
  it, or **hide** one that is already archived. A working chat has no button,
  since the work would bring it back at once. The tick turns green for a
  moment when you mark a chat complete. **Mark all complete** beside Active
  archives every active chat at once, with no tick.
- **Right-click** a row and choose **Rename…** to give the chat your own
  name. **Enter** saves it and **Esc** keeps the old one. An empty name is
  refused. The web interface shows the same name, and a search finds it.
  A running chat's Terminal or Ghostty tab takes the name too, in place of
  the title the tool sets.
- On a chat in Needs you, the same menu offers **Mark as read**. It takes the
  chat out of Needs you without opening it, for a reply you already read in
  its terminal. The next thing written in the chat puts it back.
  Right-clicking alone does not open the chat or mark it as read.
- On a chat under Archived, the same menu offers **Delete permanently…**. It
  asks first, then removes the chat from this computer. Only local Claude
  Code chats can be deleted for now; the others say why. See
  [what is removed and what stays](../docs/site/content/docs/cli-reference/companion.md#delete-an-archived-chat).
- An archived chat comes back when you open it, when something new is
  written in it, or when it starts working. A hidden chat leaves every list
  and search in agnostic-ai and comes back with a new message or when it
  starts working. The tool that owns the chat keeps
  it either way.
- The **home button** beside the search opens the full agnostic-ai web
  interface, the dashboard.
- **Esc**, or a click anywhere else, closes the list. While a menu or a
  rename is open, **Esc** closes that first.

The number on the circle counts the conversations that need you. An ice blue arc
turns around the circle while an agent is working. With animation off, a still
ice blue ring shows instead.

Starting the companion a second time brings back the one that already runs and
opens its list.

## Settings

The ⋯ button at the foot of the list holds them:

- **Start at login** starts the companion when you log in. The system keeps this
  setting. On macOS the app shows under System Settings > General > Login Items,
  in Open at Login, with its name and icon. Elsewhere it points at the
  companion where it is now, so turn it off and on again after you move the
  file.
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

The menu has no quit: a hidden companion does nothing but wait for the
shortcut. To end it, see below.

Right-click the circle (Ctrl+click on a Mac) for its own menu. The settings
are at the top:

- **Animate companion** turns movement on and off: the turning arc, the
  breathing dot in the list, the badge's pop and the list's opening. When it is
  off, a still ring shows that an agent is working, and the number keeps
  updating. Hover and press feedback stay. Your system's reduce motion setting
  still applies when it is on.
- **Dim when idle** fades the circle while nothing needs you, so it stays out
  of your way. It comes back to full color when you point at it, even with
  another app in front, and while any conversation needs you. A working agent
  alone does not bring it back. It stays easy to click and drag while faded.

## Stop it and remove it

- `agnostic-ai companion stop` ends the companion and its server. It says so
  when none runs. **Start at login** stays as you set it.
- `agnostic-ai companion uninstall` ends the companion, turns off **Start at
  login**, and deletes the built program, `companion.json` and
  `companion-command`. Your read and archive marks stay, because
  `agnostic-ai serve` uses them too. It also removes a login entry that a
  deleted program left behind.
- `agnostic-ai companion install` over a companion that runs stops it, replaces
  it, and starts the new one. On macOS it also replaces a companion that an
  earlier release installed as a bare program, with its launch agent. When
  that one started at login, the app does now.

## What each group means

Each conversation is in the first group whose rule holds.

| Group | Rule |
| --- | --- |
| Needs you | Something was written after you last saw it. Or it is open, its tool says it waits for you, and you have not looked at it since it asked. |
| Working | Open, and its tool says the agent is busy. |
| Active | Open, or written in the last 24 hours. |
| Archived | You archived it and nothing was written since, or it is closed and was quiet for more than 24 hours. A new message moves it back to Needs you. A chat that starts working moves to Working, and that archive does not come back when the work ends. |

What each tool reports:

| Tool | Working | Waiting for you |
| --- | --- | --- |
| Claude Code, local and cloud | yes | yes |
| Codex cloud | yes | yes |
| Codex local | yes, from the turn that runs in its session file | inferred: a turn that wrote nothing for 5 minutes, shown as No activity for 5m |
| Cursor | no | no |

A chat whose tool does not report waiting reaches Needs you when something new
is written in it. Cursor reports no working either, so an archived Cursor chat
comes back only with a new message.

Local Codex does not write down that it asks for an approval or for input, and
a turn that hangs never says so. All of those look the same: a turn that began
and a session file that holds still. So a turn that wrote nothing for 5 minutes
goes to Needs you, and its row says how long it has been silent, such as No
activity for 7m, instead of Waiting for you. A command that runs longer than
that reads the same way; open the chat and it leaves Needs you until it writes
again. The time updates about once a minute.

When a status counts:

- **Looking counts.** Opening or archiving a chat that waits for you takes it
  out of Needs you. Its next prompt is a new write, which puts it back.
- **Only looking here counts.** You see a conversation when you open it from
  the companion or the web interface, archive it, or choose **Mark as read**.
  A chat you read in its own terminal still counts as unread until then.
- **A local chat** is working or waiting only while its process runs on this
  machine.
- **A cloud chat** has no process here to check, so its status is believed for
  a day after its last write. A Claude Code cloud session also counts as working
  only while its worker runs: a session that failed can keep a working status
  for weeks.

## Cloud sessions

The list also shows the Claude Code sessions that run at claude.ai/code and the
threads that run in the Codex cloud. They sit in the same groups as the others,
with **Cloud** in their second line.

- **What it reads.** The id, title, status and times of at most 100 sessions
  for each tool. It never reads a session's messages.
- **The sign-in.** It uses the sign-in each tool already keeps on this machine
  and sends it only to that tool's own cloud. Claude Code cloud sessions need
  macOS for now; elsewhere the list shows none and says nothing. If you never
  signed in to Codex, nothing is asked and nothing is said.
- **When it fails.** Your local chats always show. When the cloud is slow or
  does not answer, the list keeps the last good cloud sessions for up to 10
  minutes and says nothing. After that, or when the sign-in is the problem,
  they leave the list and one quiet line under the rows says why.
- **Opening one** opens a Claude Code session on claude.ai in your browser, and
  a Codex thread in the Codex app.
- **It can break.** Both endpoints are undocumented, so a release of either
  tool can change them. The list then shows your local chats and says the cloud
  answer could not be read.

[Cloud sessions](../docs/site/content/docs/cli-reference/companion.md#claude-code-cloud-sessions)
has the hosts, the sign-in files and the timing.

## Files it keeps

Both are in the `agnostic-ai` folder of your user configuration folder
(`~/Library/Application Support` on macOS, `%AppData%` on Windows,
`~/.config` on Linux):

- `conversation-marks.json`: what you saw, what you archived and the names
  you gave chats, shared with `agnostic-ai serve`. Tool names, ids, times
  and those names only, never chat text.
- `companion.json`: where you left the circle, your shortcut, whether it
  animates, and whether it dims when idle.

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

## Layout

- `main.go`: the two windows (the circle and the list) and how they open and close.
- `server.go`: starts and stops the companion's `agnostic-ai serve`, and calls it.
- `inbox.go`: asks the server every 5 seconds whether the list changed, keeps the latest one, and hands the pages' opens and marks to the server.
- `preferences.go`: the shortcut and the start-at-login setting.
- `appearance.go`: the circle's right-click menu and the animation and dim settings.
- `hover_darwin.go`: tells the circle on macOS when the pointer is over it, even while another app is in front.
- `dashboard.go`: opens the full web interface.
- `place.go`: where the list opens beside the circle, and whether a saved place is still on a screen.
- `settings.go`: what the companion remembers between runs.
- `frontend/`: the pages, in plain HTML, CSS and JavaScript with no build step.
  `model.js` holds the logic and has no DOM in it. `look.js` turns your
  settings into how the pages look, and `motion.js` applies both choices to each page.
- `dev/`: the browser preview behind `make companion-preview` and `make companion-e2e`.
  It is outside `frontend/`, so no build of the companion holds it.

This folder is its own Go module, so [Wails](https://wails.io), which draws the
windows, is not a dependency of the `agnostic-ai` command and adds nothing to
its binary. It imports nothing from the `agnostic-ai` module: what it knows
about conversations comes over the server's API, documented under
[serve](../docs/site/content/docs/cli-reference/inspect.md).

## Test it

```sh
make companion-test   # Go vet, Go tests and the page logic tests
make companion-e2e    # the list's search, open, rename, mark as read and failure flows in headless Chromium
```

CI runs the page tests and the browser tests. It does not run the Go tests yet,
so run `make companion-test` before you push a change to Go code here.

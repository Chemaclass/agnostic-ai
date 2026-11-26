# agnostic-ai companion

A small circle that floats over every app and shows how many of your AI
conversations need you. Click it for a searchable list of your Claude Code,
Codex and Cursor chats, and open any of them where it lives.

It is a window on the `agnostic-ai` command and reads no chat itself. It
starts its own `agnostic-ai serve` at user scope, with cloud sessions, and
that server reads, opens and marks the conversations. So the companion and the
web interface always agree,
and an upgrade of agnostic-ai is an upgrade of what the companion shows.

## Get it

On macOS, you can also copy the app from the release disk image to Applications and open it in Finder. On a fresh install the panel opens while your conversations load. The app uses its bundled CLI. An explicit `AGNOSTIC_AI_BIN` in its environment overrides it.

```sh
agnostic-ai companion install     # put it on this machine
agnostic-ai companion             # start it
agnostic-ai companion stop        # end it
agnostic-ai companion uninstall   # remove it from this machine
```

On macOS, `install` downloads the companion the release built, checks it
against the release's checksums and build provenance, and needs no compiler.
A release without one, an `agnostic-ai` built from any commit but a release
tag, `--source`, and other systems build it from the same commit as your `agnostic-ai`, with Go, and
print each step as it goes. A build names what your machine lacks before it
starts. [companion](../docs/site/content/docs/cli-reference/companion.md#companion)
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

The companion checks for a newer release on start and every 24 hours without opening a window. An automatic check can keep the last verified offer offline. In either menu, open **App → Updates** to check now or change **Check for updates automatically**. You can check manually with automatic checks off. When an update is known, choose **Update to vX.Y.Z** under **Updates**, then **Update and restart** in the list. See the [update guide](https://agnostic-ai.org/docs/cli-reference/companion/#upgrade-the-companion) for the terminal install and copy fallback.

Disk image updates check the new app's signer before running it. The updater keeps the earlier app until the new one stays open for a few seconds. If macOS refuses to open the replacement, or it quits or never appears, the updater tries to put back and open the earlier app. If it cannot restore the app, it logs where the earlier bundle is saved. A terminal update on macOS also puts back and opens the earlier app when the new one does not start. The list shows the failure and log path after restart when it can recover the outcome.

`agnostic-ai upgrade` also replaces an installed companion through the updated CLI, downloaded or built as `install` does. It prints each step and restarts the companion if it was running. A stopped companion stays stopped. Your settings and Start at login choice stay as they were. If no companion is installed, the upgrade does not build one.

If the CLI upgrade succeeds and the companion update fails, the CLI stays upgraded. The command explains the failed step or missing build tool. Fix it, then run `agnostic-ai companion install` to retry.

After upgrading directly through Homebrew or npm, run `agnostic-ai companion install`. On the next companion/server start, the panel shows a notice if their versions differ, with that command. Starting through `agnostic-ai companion` also prints a warning. A companion and server already running before an external upgrade can keep their old versions until restarted.

## Use it

- **Drag** the circle anywhere. An open list follows it and keeps your search.
  It remembers where you left it, and comes back
  to the lower right when that place is on a monitor you unplugged.
- **Click** the circle to open or close the list. So does the shortcut,
  **Ctrl+Alt+Space** (⌃⌥Space on a Mac) until you change it.
- **Type** to search every conversation, of any age.
  The dashboard icon beside search opens the web interface.
- **All, Local, Cloud** at the foot narrows the list to the chats on this
  machine or to the ones that run in a tool's cloud. It remembers your choice.
  The number on the circle always counts both.
- A row names the chat's folder when it has one. When two folders in the list end in the
  same name, the row adds the parent folders that tell them apart, such as
  `shop/api` and `billing/api`. Hover a row for its full folder path.
- Under its title, a row shows the agent's latest reply in one line: the
  question it asks you, or the start of its last message. Local Claude Code,
  Codex and Cursor chats show it. Cloud sessions do not yet.
- A chat started with `agnostic-ai conversations start` sits indented under
  the chat it came from while both are in the same group. A child that waits
  for you stays under Needs you, even when its parent is active or archived.
- Open **Keyboard shortcuts** from the list’s ⋯ menu to see global and list keys.
  With a chat focused and an empty search, press **?** to open the same sheet.
- **↑ ↓** move through the list and **Enter** opens the chat. So does a click
  anywhere on the row but its name. **Tab** focuses a chat after opening the list or using search.
  **F2** renames the selected chat.
  A running chat comes up in the Terminal, Ghostty, iTerm, WezTerm, kitty or Konsole tab that runs it, ready
  for typing. Inside tmux, its pane is selected first. When only the app can come forward, the list comes back and
  says where the chat runs. macOS asks once to let the companion control each app.
  The reason stays above the list until you dismiss it or open another chat.
  kitty needs [remote control](../docs/site/content/docs/cli-reference/conversations.md#kitty-remote-control).
  When a chat could not be resumed, **Copy resume command** copies the command
  to run in a terminal. A Claude Code, Codex or `cursor-agent` chat still
  running in a terminal never offers it, because that command would start a
  second copy.
- Hover a row to reach **Mark complete**. It archives an eligible chat and leaves it searchable. A working chat has no
  mark complete button, since the work would bring it back at once. The tick turns green for a
  moment when you mark a chat complete. **Mark all complete** beside Active
  archives every active chat at once, with no tick. A chat that got a reply
  or started working since the list was drawn keeps its place.
- Click a chat's name, **right-click** the row and choose **Rename…**, or press
  **F2** to give the chat your own name. The name field opens without opening
  the chat. A chat that cannot be renamed keeps a plain name that opens it. **Enter** saves it and **Esc** keeps
  the old one. An empty name is
  refused. The web interface shows the same name, and a search finds it.
  A running chat's Terminal, Ghostty, WezTerm, kitty or Konsole tab takes the name too, in place of
  the title the tool sets. In iTerm the name goes on the chat's pane.
  Renaming a running chat's single-pane Ghostty tab on macOS renames the
  chat too, on the next refresh or the one after.
- Right-click a chat and choose **Copy session ID** to copy its exact ID.
  A cloud chat gives you its cloud ID. Use the tool that owns the chat to
  decide how to open it. Copying keeps the chat state and marks.
- Choose **Message another session…** on a local Claude Code or Codex chat to start
  a [mailbox thread](https://agnostic-ai.org/docs/mailbox/). Pick a receiver and send the default **Handoff** to hand a session over.
  Or drag the chat onto the receiver's row: the same form opens with both chats and **Handoff** chosen, and nothing is sent until **Send**.
  The thread appears under both chats and says what happens next. Choose **Review request** to share exact Git commits and changed paths after checking the base and preview.
  A message an agent sends shows as **Held for you** until you choose **Approve message** or **More → Reject message**.
  **More → Queue for delivery** hands a message you copied or pasted back to the
  [mailbox built-in](https://agnostic-ai.org/docs/mailbox/#deliver-with-the-next-prompt),
  which delivers it with the session's next prompt.
- Choose **Note files this chat is working on…** to record which paths a session is working on. A claim does not block edits. A row with claims shows a chip such as "2 paths claimed", and the footer's **File claims** button shows only while a claim exists. When two sessions want one path, both rows move under Needs you until the holder releases it or the requester dismisses it. Send a **Handoff** to offer those paths to another chat. The claim moves when the receiver accepts it.
- On a chat in Needs you, the same menu offers **Mark as read**. It takes the
  chat out of Needs you without opening it, for a reply you already read in
  its terminal. The next thing written in the chat puts it back.
  Right-clicking alone does not open the chat or mark it as read.
- The same menu offers **Mark complete** on an unmarked chat in Active or
  Needs you, and **Hide from agnostic-ai** on any archived chat, including one you could delete. A chat that
  waits only on a forgotten shell stays in Needs you until the shell stops.
  Once marked, it offers **Undo complete**. The
  [usual rules for Archived](../docs/site/content/docs/cli-reference/conversations-api.md#get-apiviewsconversations) still apply.
- On macOS, **Stop chat…** asks before ending a live Claude Code terminal
  session. The transcript stays, so you can resume the chat later. Separate
  shell processes may keep running.
- **Mark all as read** beside Needs you does the same for every chat that
  group shows, after the search and the All, Local or Cloud choice. It shows
  only while the group is unfolded. A chat
  that got a new message since the list was drawn keeps its place. If a mark
  fails, the run stops there, those rows stay, and the list says why. The
  list holds every unread or waiting chat of any age, so with All chosen and
  no search the circle shows zero after the run.
- On a chat under Archived, the same menu offers **Delete permanently…**. It
  asks first, then removes the chat from this computer. Cancel, Escape or a
  click outside the question keeps the chat. Only local Claude
  Code chats can be deleted for now; the others say why. See
  [what is removed and what stays](../docs/site/content/docs/cli-reference/companion-list.md#delete-an-archived-chat).
- In a live Claude Code chat, use **Stop shell…** from the row menu when it
  warns about a background shell. See how to
  [stop a forgotten shell](../docs/site/content/docs/cli-reference/companion-list.md#stop-a-forgotten-shell).
- An archived chat comes back when you open it, when something new is
  written in it, or when it starts working. A hidden chat leaves every list
  and search in agnostic-ai and comes back with a new message or when it
  starts working. The tool that owns the chat keeps
  it either way.
- **Send copied text** to a chat: copy text anywhere and press the send
  shortcut, **Ctrl+Alt+V** until you change it in the list's ⋯ menu. With
  the list open, **Cmd+Shift+V** (Ctrl+Shift+V off a Mac) does the same. A
  banner shows "Click a chat or press Enter to paste these 412 words there"
  with the start of the text. Pick a chat with **↑ ↓** and **Enter**, or a
  click. The text lands in that chat's input and is not sent: you press
  Enter there.
  - Text copied from a web page goes as Markdown when it holds links,
    tables, code blocks, headings or lists, and the reply after the send
    says so. Other copied text goes as plain text. On macOS only.
  - The list shows only this machine's chats, without Archived, also during
    a search.
  - The first chat the text can be typed into is selected. Nothing is sent
    until you press Enter or click.
  - A click on a subagent row sends the text to the chat it runs in.
  - A live chat in a Terminal, Ghostty, iTerm, WezTerm, kitty or Konsole tab gets the text typed in, and the
    list hides. For any other chat, the companion switches to it and says to
    paste with Cmd+V: the clipboard still holds the text. On Linux the window
    may stay behind, so switch to it before you press Enter. Konsole 26.04
    types text only with [one setting on](https://agnostic-ai.org/docs/cli-reference/conversations/#konsole-on-linux).
  - A chat that ended is never resumed and typed into. A Cursor chat or a
    working Codex chat is not typed into either. Nor is a chat waiting for
    your answer, where a typed letter could answer it, or one whose state
    is not known.
  - Pasting into Terminal needs macOS Accessibility access once. Plain text
    leaves the clipboard as you copied it, so text ending in a line break is
    not pasted there: paste it with Cmd+V.
  - **Esc** leaves pick mode. A second **Esc** closes the list.
  - From any app, press **Ctrl+Alt+V** (⌃⌥V on a Mac) until you change it.
    The list opens in pick mode on every chat, with the search cleared. When
    exactly one chat can take typed text, the text goes straight there: the
    banner names the chat, the text flies into its row, and the list hides.
    With none or several, pick one as above. Cmd+Shift+V and the clipboard
    button never send by themselves.
- **Dashboard** opens the full web interface when its local server is ready.
- **Settings** shows the global configuration folder the server reads, including an `AGNOSTIC_AI_HOME` set in the app's environment.
- **Esc**, or a click anywhere else, closes the list. While a menu or a
  rename is open, **Esc** closes that first.

The number on the circle counts the conversations that need you. An ice blue arc
turns around the circle while an agent is working. With animation off, a still
ice blue ring shows instead.

Rows under the blue **Working** section show a small gyroscope. Five bulbs
travel at different radii and depths around a shaded core. The assembly
turns slowly at constant brightness, with space around it. Each working
native Claude Code or Codex sub-agent adds one blue miniature of the same
five-dot fractal. A number beside an agent icon shows the count; its tooltip
and accessible label say how many sub-agents are working.
An idle parent stays under Working while its sub-agents work, with its own
plain, still dot. A parent that needs your answer stays under Needs you with
the orange lock dot and no orbiting bulbs of its own. Working satellites
keep their blue cores and animation independently of the parent. An unavailable
count adds no markers. Cloud and other tools' native sub-agents are not tracked.
With animation off or your system's Reduce Motion enabled, the whole indicator
stays still and its section heading and worker count remain visible.

Starting the companion a second time brings back the one that already runs and
opens its list, even when it is hidden.

The companion is in one of three states. None is saved: a restart starts shown.

| State | What you see |
| --- | --- |
| Shown | The circle at full color, as usual. |
| Hidden | Nothing. The companion keeps reading, so the shortcut and the count stay ready. |
| Back dimmed | The circle at 25%, with its count, after a conversation started waiting for you while it was hidden. The list stays closed, and on macOS the keys stay with the app you use. Click the circle or press the shortcut for full color. |

## Settings

Use the list's ⋯ button or right-click the circle (Ctrl+click on a Mac).
Both menus group their settings under **Appearance**, **Permissions**,
**Shortcuts** and **App**. Permission controls are in the list's menu. A
change to a shared setting shows in both menus.

- **Animate companion** turns movement on and off: the turning arc, the
  gyroscope in the list, the badge's pop and the list's opening. When it is
  off, a still ring shows that an agent is working, and the number keeps
  updating. Hover and press feedback stay. Your system's reduce motion setting
  still applies when it is on.
- **Dim when idle** fades the circle while nothing needs you, so it stays out
  of your way. It comes back to full color when you point at it, even with
  another app in front, and while any conversation needs you. A working agent
  alone does not bring it back. It stays easy to click and drag while faded.
- **Small companion** shows a 40-point circle instead of the 60-point one. The
  mark, the working arc and the count shrink with it, and so does its window,
  so clicks around the small circle reach the app behind it. The circle keeps
  the screen edges it sits nearest, and an open list moves to stay beside it.
- **Start at login** starts the companion when you log in. The system keeps this
  setting. On macOS the app shows under System Settings > General > Login Items,
  in Open at Login, with its name and icon. Elsewhere it points at the
  companion where it is now, so turn it off and on again after you move the
  file. A change made in System Settings shows in the right-click menu once
  the ⋯ menu was opened or another setting changed.
- **Ask me before an agent sends a message** holds each mailbox message an
  agent sends until you approve it. It is on by default, and only the list's
  ⋯ menu shows it, because the server keeps the setting.
- **Open from anywhere** shows the shortcut. In the ⋯ menu, click it, then
  press the new keys: at least one of Ctrl, Alt or Cmd (the Windows or Super
  key elsewhere), and a letter, digit, Space, Enter, Tab, Backspace, Delete,
  Home, End, an arrow or a function key. Keys every app uses to copy, paste,
  undo, quit, close, stop a terminal command or switch apps and tabs, such as
  Cmd+C, Cmd+Q, Ctrl+C or Ctrl+Tab, are refused. In the right-click menu it shows the keys; choosing it opens the list on its ⋯
  menu, ready to change them. A shortcut takes its keys away from every other
  app, so pick one nothing else uses. Keys another app already holds are
  refused and the old shortcut stays. macOS, Windows and X11 ask for no
  permission; a Wayland desktop asks you once to approve it.
- **Send copied text** shows the send shortcut and changes the same way. The
  two shortcuts cannot share keys: keys the other one uses are refused.
- **Hide Agnostic AI companion** takes the circle and the list off the
  screen. The program keeps running and reading, so the shortcut still
  works. While the shortcut is not active, the entry names the way back:
  `agnostic-ai companion`. Press the
  shortcut, run `agnostic-ai companion`, or open the app again: the circle
  comes back where it was, at full color, and the list opens. When a
  conversation starts waiting for you, the circle comes back on its own,
  dimmed. Conversations that already waited when you hid it do not bring it
  back; one that started waiting shortly before Hide, before the companion
  read it, can (up to 5 seconds, or about a minute for a change that shows
  when the minute turns). Hide also hides a dimmed circle again.
- **Quit Agnostic AI companion** asks first. The question says the program
  stops, so the shortcut will not bring it back, and that `agnostic-ai
  companion` (or your AI tool running it) starts it again. Answer **Quit** to
  end the companion and its server, as `agnostic-ai companion stop` does,
  **Hide instead** to hide it, or **Cancel**. Escape or a click elsewhere
  cancels. **Start at login** stays as you set it.

## Stop it and remove it

- **Quit Agnostic AI companion**, in the list's ⋯ menu or the circle's
  right-click menu, asks first, then ends the companion and its server.
- `agnostic-ai companion stop` does the same from a terminal, without asking.
  It says so when none runs. **Start at login** stays as you set it.
  `install`, `upgrade`, `uninstall` and logging out also end it without
  asking.
- `agnostic-ai companion uninstall` ends the companion, turns off **Start at
  login**, and deletes the built program, `companion.json`, its lock file and
  `companion-command`. Your read and archive marks stay, because
  `agnostic-ai serve` uses them too. It also removes a login entry that a
  deleted program left behind. On Linux, from a shell without a D-Bus session
  bus, such as an SSH login, `stop` says it cannot reach the companion and
  `uninstall` still deletes the files.
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
| Archived | You archived it and nothing was written since, you archived it in its own app (Codex, Cursor, Claude chats or Claude Cowork) and nothing was written since, or it is closed and was quiet for more than 24 hours. A new message moves it back to Needs you. A chat that starts working moves to Working, and that archive does not come back when the work ends. |

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

- **Newly listed app history starts quiet.** On the first full read of the
  available Claude chat, Cowork, or ChatGPT list, older chats without saved
  read marks start outside Needs you. If you already have a marks file from
  before this change, replies since v0.96.0 still count as usual.
- **Looking counts.** Opening or archiving a chat that waits for you takes it
  out of Needs you. Its next prompt is a new write, which puts it back.
- **Only looking here counts.** You see a conversation when you open it from
  the companion or the web interface, archive it, or choose **Mark as read**
  or **Mark all as read**.
  A chat you read in its own terminal still counts as unread until then.
  An archived chat has no read state, so it never shows as unread.
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
- `companion.json`: where you left the circle, your shortcuts, whether it
  animates, whether it dims when idle, and whether it is small. A file that is
  not JSON moves to `companion.json.bad`, and the companion starts with the
  defaults.
- `companion.json.lock`: held for each save of `companion.json`. A save reads
  the file again under this lock and changes only its own value, so a value
  another program wrote in the meantime stays.

## When it reads

| The companion is | What runs |
| --- | --- |
| Shown, hidden or back dimmed | Every 5 seconds it asks its server whether the list changed. The server compares the files' sizes and times and answers "unchanged" without reading a chat. It reads only when a chat or a mark changed, and once when a new minute begins. |
| Searching | One read for each search, and again when a chat changes while the search shows. |

What you see is never older than this:

- A new message, a new chat, or a mark: 5 seconds.
- A Claude Code chat that starts or stops working or waiting: 5 seconds.
- An app or process that ended without writing anything, a chat that went
  dormant, and any change to a cloud session: the turn of the minute, so at
  most 65 seconds.
- A list that could not be read is not shown. The circle loses its number and
  the list says why, instead of showing the last one as if it were current.

## Layout

- `main.go`: the two windows (the circle and the list) and how they open and close.
- `server.go`: starts and stops the companion's `agnostic-ai serve`, and calls it.
- `inbox.go`: asks the server every 5 seconds whether the list changed, keeps the latest one, and hands the pages' opens and marks to the server.
- `preferences.go`: the two shortcuts and the start-at-login setting.
- `appearance.go`: the animation and dim settings, and the companion's state.
- `presence.go`: the three states, and what counts as a new conversation waiting.
- `menu.go`: the circle's right-click menu, built from `frontend/menu.json`.
- `hover_darwin.go`: tells the circle on macOS when the pointer is over it, even while another app is in front.
- `dashboard.go`: opens the full web interface.
- `place.go`: where the list opens beside the circle, and whether a saved place is still on a screen.
- `settings.go`: what the companion remembers between runs.
- `frontend/panel-approvals.js`: the approvals rows' status line, the question before a looser mode, and Finish setup.
- `frontend/`: the pages, in plain HTML, CSS and JavaScript with no build step.
  `menu.json` lists the settings both menus show, in order, so they cannot drift
  apart. `model.js` holds the logic and has no DOM in it. `look.js` turns your
  settings and the companion's state into how the pages look, and
  `motion.js` applies them to each page.
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
make companion-e2e    # the list's search, open, rename, mark as read and failure flows, its ⋯ menu, the quit question, and the hidden and dimmed circle, in headless Chromium
```

CI runs Go vet, the Go tests, the page tests and the browser tests.

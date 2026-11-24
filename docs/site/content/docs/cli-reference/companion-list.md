+++
title = "Companion list"
description = "What the companion's list does when you click, send to, rename, mark or delete a conversation, and how it groups and archives them."
weight = 47

[extra]
group = "Reference"
+++

# Companion list

Select a conversation in the [companion](@/docs/cli-reference/companion.md) to open it in its original tool.

## The list {#the-list}

Click the companion circle, or press its shortcut, to open the list. Type to search.

| Group | What it holds |
|-------|---------------|
| Needs you | Chats waiting for a permission, then an answer or a path conflict, then unread replies. Newest first. |
| Working | The agent is busy. |
| Active | Open chats, and app or cloud chats until you archive them. |
| Archived | Closed chats, and chats you archived in their app. |

{% <details summary="Details: keys"> %}
| Key | What it does |
|-----|--------------|
| ⌃⌥V on a Mac (Ctrl+Alt+V elsewhere) | Open the send picker from any app. You can change it. |
| Cmd+Shift+V | Open the send picker in the open list, keeping your search. |
| Tab after opening the list or using search | Focus the selected chat or the first chat in the list. |
| Arrow keys, Enter | Pick a chat, then open it or send to it. In the right-click menu, move between its items. |
| Escape | Cancel the picker or rename first. In the list, clear the search before closing the panel. |
| F2 | Rename the selected row. |
{% </details> %}

## Open a chat {#open-a-chat}

Click a row to reach its chat.

| Chat | What a click does |
|------|-------------------|
| Running in Terminal, Ghostty, iTerm, WezTerm, kitty or Konsole | Selects its tab. kitty needs [remote control](@/docs/cli-reference/conversations.md#kitty-remote-control). |
| Running in tmux | Selects its pane, then the tab that shows its tmux session. |
| Ended terminal chat | Resumes it in a new tab. |
| Claude Code `--bg` session | Selects the tab that runs `claude attach`, or opens one. |
| Anything else | Opens its app, editor or browser. |

If a chat does not open, the reason stays above the list until dismissed or another chat opens.

{% <details summary="Details: tabs, background sessions, slow opens, resume command"> %}
- The first time, macOS asks to let the companion control each terminal app.
- An ended terminal chat resumes in a new tab of the app it last ran in.
- On Linux an ended terminal chat resumes in a new `x-terminal-emulator` window. It runs in your shell, so its startup files set `PATH`. Without that command, or with no display, the list offers **Copy resume command**. A chat that last ran in WezTerm, kitty or Konsole resumes in a new tab there instead.
- On Linux, WezTerm, kitty, Konsole and tmux inside them select the tab. The window can stay behind. For GNOME Terminal, Ptyxis, Alacritty, foot, xterm, Tilix and Xfce Terminal, the notice names the terminal and device.
- When only the app can come forward, the list stays open and says where the chat runs.
- Codex can find a tab after a delayed first prompt. After the earlier chat ends, a lone live chat and terminal in the project can give a **Possible Codex tab**. Check it before pasting.
- `claude --bg` runs in Claude's own background service, not in a terminal app.
- The tab is found by its short id, such as `e3469759`, or its exact full name. A tab attached with the start of an id or part of a name is not recognized, since it may be another session.
- With no such tab, a new tab runs `claude attach <id>` in the session's folder, in the app it last ran in when it remembers one.
- `claude` is looked up on `PATH`, then in `~/.local/bin`, `~/.claude/local`, `/opt/homebrew/bin` and `/usr/local/bin`.
- Leaving that tab does not stop the session.
- When switching fails, or `claude agents` cannot run or does not list the session, the list offers the `claude attach` command to copy, or says to find its id with `claude agents` and run `claude attach <id>`.
- An open over 60 seconds keeps the list hidden and goes on in the background.
- If it then fails, such as when macOS asks to allow Automation, the list shows the reason, the chat's name and **Copy resume command** as soon as it shows, whatever the search. Another message on screen comes first; the late one follows once you dismiss it.
- No answer at all says "agnostic-ai did not answer in time. The chat may still have opened."
- Clicking the same chat again while it opens starts no second copy.
- **Copy resume command** copies the command to run in a terminal.
- It is also offered for a chat still running where no tab shows it, such as in a detached `tmux` session or after its tab closed. It comes with a note to end that process first, since two copies of one session would compete.
- A chat whose tab was found, and a Codex thread Codex's background service keeps, never offer it.
- On Windows a click opens editor chats, Codex threads and cloud sessions, and resumes an ended terminal chat in a new PowerShell window. A chat that still runs shows as open and is never started twice, but its window is not brought forward. Commands on Windows are for PowerShell.
{% </details> %}

## Send copied text {#send-copied-text}

Send text you copied to any chat without switching apps.

1. Copy text anywhere.
2. Press **⌃⌥V** on a Mac (Ctrl+Alt+V elsewhere). The list opens with a banner such as "Click a chat or press Enter to paste these 14 words there".
3. Click a chat, or pick one with the arrow keys and Enter. Escape cancels.

| Chat | What happens |
|------|--------------|
| Live Terminal, Ghostty, iTerm, WezTerm, kitty or Konsole tab | It comes forward with the text in its input. Press Enter. On Linux, switch to its window first. Konsole 26.04 needs [one setting](@/docs/cli-reference/conversations.md#konsole-on-linux). |
| Any other chat | It comes forward. Paste with Cmd+V. |

Only one chat can take text? It goes straight there. Change the shortcut under **Send copied text** in the list's ⋯ menu or the circle's right-click menu.

{% <details summary="Details: which chats, search, Markdown, permissions, errors"> %}
- Offered chats: this machine's chats under Needs you, Working and Active. Archived chats are never offered, even in a search.
- With the list already open, Cmd+Shift+V opens the same picker but keeps your search, and never sends by itself.
- While a search is on its way, Enter does nothing. Its answer keeps the selected chat when it still matches, else selects the first.
- "Straight there" means: exactly one chat can take typed text and its row is visible (not inside a folded group). The banner names the chat, the text flies into its row, and the list hides.
- Text copied from a web page goes as Markdown when it holds links, tables, code blocks, headings or lists, and the reply says so.
- Nothing copied shows "Nothing to send: copy some text first." A clipboard the companion cannot read says so. Neither sends anything.
- A send waits up to 60 seconds. With no answer by then, the list says the text may have been typed: check the chat before you send again.
- Pasting into Terminal needs macOS Accessibility access, granted once in System Settings › Privacy & Security › Accessibility to the companion or to the terminal that runs `agnostic-ai serve`.
- What the server refuses: [`POST /api/conversations/send`](@/docs/cli-reference/conversations-api.md#post-api-conversations-send).
{% </details> %}

## Copy a session ID {#copy-a-session-id}

Right-click a chat to **Copy session ID**. This copies its exact ID, including cloud IDs, and keeps its state and marks.

## Message another session {#message-another-session}

Right-click a local Claude Code or Codex chat and choose **Message another session…**. [New threads](@/docs/mailbox.md#hand-a-session-over) start on **Handoff**, replies on **Question**. The line under the chat says what happens next.

[**Note files this chat is working on…**](@/docs/mailbox.md#hand-off-files) shares which files this chat works on. Anyone in the worktree can still edit them.

## Rename a chat {#rename-a-chat}

Give a chat your own name. The web interface shows the same name.

1. Click the chat's name (or right-click the row and choose **Rename…**, or press F2).
2. Type the name. Enter saves, Escape keeps the old name.

The chat's own tool takes the name first. Only then is it saved, so every `serve` on the machine shows it and finds it in a search. If the tool refuses, the field stays open with the reason, such as "Codex is not installed". Claude app chats, Cowork tasks and Codex cloud threads cannot be renamed yet.

{% <details summary="Details: how each tool is renamed, where the name lives, errors"> %}
| Chat | How it is renamed |
|------|-------------------|
| Claude Code | Adds the `custom-title` line Claude Code's own `/rename` writes. |
| Claude Code cloud session | Changes the title on claude.ai/code, using the login Claude Code keeps in the macOS keychain, or in `.credentials.json` on Linux. |
| Codex | Asks `codex app-server` to rename the thread (`thread/name/set`). Needs `codex`. |
| Cursor | Changes the name in Cursor's own database. Needs `sqlite3`. |
| ChatGPT | Asks chatgpt.com to change the title, using the ChatGPT login the Codex command keeps. |
| Claude app chat, Cowork task, Codex cloud thread | Cannot be renamed yet. |

- Other reasons shown: "Sign in to ChatGPT in the Codex app".
- Marks file: the name is saved in `conversation-marks.json`, only once the tool has it.
- A new message keeps the name.
- Right-clicking does not open the chat or mark it as read.
- A click elsewhere on the row opens the chat.
- A running chat's Terminal, Ghostty, WezTerm, kitty or Konsole tab takes the name as its title, and keeps it after the session ends. In iTerm the name goes on the chat's pane.
- Claude Code: a session that is still running may write its old name back.
- Cursor editor chat: the name changes in the chat list and in the chat's own data, and nowhere else. Cursor may write the old name back if it has the chat loaded.
- cursor-agent chat: the name changes in its `store.db`, only when a look at the open files just before the write finds the chat not running.
- Cursor renames keep the chat's done and read marks.
- ChatGPT: the app shows the new name once it syncs.
- On failure the row keeps its old name. The server logs what the tool said on its standard error, with the tool and the conversation id.
{% </details> %}

## Archive, hide or stop a chat from the row menu {#row-menu}

Right-click a row to reach what its buttons do, and a little more:

- **Mark complete** moves an unmarked chat from Active or Needs you to Archived. A chat waiting only on a forgotten shell stays in Needs you until the shell stops or you [keep it running](#stop-a-forgotten-shell).
- **Undo complete** clears the mark you set. The [usual rules for Archived](@/docs/cli-reference/conversations-api.md#get-api-views-conversations) still apply.
- **Hide from agnostic-ai**: takes an archived chat out of every list and search. The tool keeps the chat.
- **Stop chat…** in the macOS companion asks before ending a live Claude Code terminal session. You can resume from its transcript. Separate shell processes may keep running. The [Stop chat API](@/docs/cli-reference/conversations-shell-api.md#post-api-conversations-stop-session) is available on macOS and Linux.

A new message, or the tool starting work, brings a completed or hidden chat back.

## Mark chats as read {#mark-as-read}

Clear a chat that needs you without opening it.

- **Mark as read**: right-click the chat.
- **Mark all as read**: the button beside Needs you, while the group is unfolded. It marks each chat the group shows, after the search and the All, Local or Cloud choice.

The chat leaves Needs you in the companion and the web interface, and moves where an opened one would: Active, or Archived when closed.

{% <details summary="Details: same mark as opening, failures, the circle count and click"> %}
- The mark is the seen mark that opening a chat saves, stored through the server.
- When a closed chat moves at once and when it waits a day: see [`state`](@/docs/cli-reference/conversations-api.md#get-api-views-conversations).
- On a chat resumed by hand with a reply from before, it clears the orange ring. The next write brings it to Needs you.
- If the save fails, the row stays and the list shows why.
- **Mark all as read** saves one chat after another.
- A chat written in since the list was drawn is skipped and stays.
- The first failed save stops the run and says why.
- The list holds every blocked and unread chat of any age.
- The circle's count ignores the search and the place. With **All** chosen and no search, it shows zero after the run until something new is written.
- The circle's number is the rows of the kind its icon names that the list shows under Needs you with **All** chosen and no search: waiting chats when one waits, else unread replies.
- While a chat needs you, a click on the circle opens the list on the first row under Needs you, selected and in view. It clears the search, switches to **All** when no chat in the chosen place needs you, and unfolds Needs you.
{% </details> %}

## Rows and order {#rows}

A row shows the title, the agent's latest reply in one line, and a meta line. A row under Needs you also says why it is there.

| Part | What it shows |
|------|---------------|
| Reason line | Under the title in Needs you: why the chat needs you. See the reasons below. |
| Meta line | Tool, where it runs when not here (Cloud, Chat, Cowork or Shared), folder, age, paths claimed. |
| Reply line | The agent's question, or the start of its last message. |
| Approval pill | Orange, replaces the reply line while a call waits for approval. |
| Approval reason | Under the pill, why [approvals](@/docs/cli-reference/companion.md#approvals) left the request to you. |
| Approved for you | "▸ 3 approved for you" under the meta line, while approvals answer prompts. |

| Reason | When |
|--------|------|
| The approval pill | A tool call waits for your approval. |
| Waiting on 2 shells still running | A [forgotten shell](#stop-a-forgotten-shell) is the only work left. |
| a.go is claimed by Fix the parser | A [path conflict](@/docs/mailbox.md#hand-off-files), on both rows. |
| Asks you a question | The agent asked you something. |
| No activity for 7m | The server read the wait from silence. |
| Waiting for you | The tool says it waits, but not for what. |
| New reply | Something was written since you last looked. "Hidden because it may hold a password or key" is added when the reply is left out. |

The [approvals chip](@/docs/cli-reference/companion.md#approvals) shows **Auto-approve on** (yolo) or **Reviewers answer** (review). Hover for details or choose **Ask me**. In ⋯, use [**Answer permission requests from the companion**](@/docs/approvals.md#answer-from-the-companion). **Finish setup…** previews changes to your tools’ settings.

Hover a row, shell line or approval pill, or select a row with the arrow keys, for a tooltip. It never covers the row, and closes on a key, scroll or click.

A chat started with [`conversations start`](@/docs/cli-reference/conversations.md#the-link-line) sits under its parent in the same group.

See [what a chat waits for](#waiting) for where each tool's wait comes from.

{% <details summary="Details: pill kinds, folders, replies, pill limits, child chats"> %}
| Pill | Waiting on |
|------|------------|
| Approve `git push origin…` | A command (its first words). |
| Approve edit `dashboard.tsx` | An edit or a write. |
| Approve read `.env.example` | A read. |
| Approve `github.com` | A fetch. |
| Approve Notion fetch | An MCP tool. |
| Needs permission | A call that is not known, as for a Codex cloud thread. |

- A row names the conversation's folder when it has one. A cloud session has none.
- When two folders end in the same name, the row adds the parent folders that tell them apart, such as `shop/api` and `billing/api`.
- The tooltip and screen-reader description give the full path.
- Local Claude Code, Codex and Cursor chats show the reply line. Cloud sessions do not yet.
- A reply older than your latest prompt is not shown.
- A reply that may hold a password or key is left out. See [`reply`](@/docs/cli-reference/conversations-api.md#get-api-views-conversations).
- Needs you already says a chat waits, so the meta line does not repeat it.
- A detail that may hold a password or key is left out of the pill, and the pill names the tool only.
- The agent's reply moves to the row's tooltip while the pill shows.
- A question, or a wait whose kind the tool does not say, keeps the reply line.
- A child chat that waits for you stays under Needs you, even when its parent is active or archived.
- A chat started from another stays under it. The two move up together to the place of the more urgent one.
{% </details> %}

## Working rows and subagents {#working-rows}

Working chats show an animated gyroscope. Each working Claude Code or Codex subagent, and each working Claude Code teammate, adds a small blue copy.

| Part | What it shows |
|------|---------------|
| "▸ 3 subagents" | A line under the meta line. Click it to unfold. With subagents inside others it says how deep, as in "▸ 4 subagents, 2 levels". |
| Unfolded list | A tree: each subagent under the one that started it, one step further in. Newest first on each level, five at the top at most, then "+N more". |
| "▾ 2 subagents" under a subagent | Folds the subagents it started. |
| Subagent row | Gyroscope, title, a Claude Code subagent's latest message when it wrote one, tool, type if any, age. A teammate's type is its name. |

A click on a subagent row:

| Subagent | What comes to the front | Notice |
|----------|-------------------------|--------|
| Claude Code teammate in a tmux pane | Its pane, and the terminal tab that shows it | None |
| Same, in a tmux session no terminal shows | The chat's tab | The `tmux -L <socket> attach` command |
| Claude Code teammate in the chat's process, or a subagent | The chat's tab | Press the down arrow to pick it below the prompt, then Enter |
| Subagent started by another subagent or a teammate | The pane of the nearest one with a pane, or the chat's tab | Which rows to open, in order |
| Subagent that finished since the list showed it | The chat's tab | It finished |
| Codex subagent | Its own thread, or the terminal that holds it | Type `/subagents` to switch to it |
| Subagent of a chat in the desktop app or an editor | The chat, as a click on its row does | Its name only |

On macOS the notice appears beside the circle as soon as the switch finishes, even after a slow open. The terminal keeps keyboard focus. On other systems the notice waits in the list. **Copy attach command** copies the tmux command when available. Only a teammate’s tmux pane opens directly. Pick other subagents yourself. A later failure replaces the notice. The send picker targets the chat that runs the subagent.

{% <details summary="Details: counts, parents, animation"> %}
- The five-bulb gyroscope keeps constant brightness, with bulbs at different orbital radii and depths.
- A number and agent icon show the count, with a tooltip and accessible label. Unavailable counts add none.
- Working is named once, in the blue section header.
- Working subagents keep an idle parent in Working, with its own still dot.
- A parent that asks you a question stays in Needs you with the orange lock dot and no orbiting bulbs of its own. Its working satellites keep their blue cores and animation.
- Cloud sessions and other tools' subagents are not tracked.
- When the server counts working subagents but cannot list them, the count sits on the subagents line instead.
- The list starts folded. Which chats are unfolded, and which subagents are folded shut, is kept until the companion quits.
- A subagent whose parent finished, or whose parents lead back to it, sits at the top of the tree.
- Only working subagents are listed and counted. A finished one leaves, and the toggle goes away once none works.
- When Claude Code removes a teammate from its team, the teammate and its subagents leave Working.
- `codex exec` runs count among them while they run.
- With animation off, or the system's Reduce Motion on, the indicator stays still with its heading and worker count visible. See [Animate companion](@/docs/cli-reference/companion.md#menus).
{% </details> %}

## Stop a forgotten shell {#stop-a-forgotten-shell}

On macOS and Linux, a local Claude Code row can warn about a background shell that is still running. Each warning shows the subagent that started it when known, how long it has run, then the command, cut in the middle so two commands that differ only at the end still read apart. The server masks passwords passed in [known ways](@/docs/cli-reference/conversations-shell-api.md), but may miss some. A command the server hid, or two that still read the same, add the process ID, as in "Command hidden · pid 14476". Hover the line for the whole command, the start time, the process ID and the number of processes. A shell that is the only work left puts the chat in **Needs you**. Elsewhere, as under **Working**, a line such as "2 shells are still running" comes first.

Each warning line has two buttons, for Claude Code on macOS and Linux:

- **Stop** asks before it stops that shell.
- **Keep running** says you meant to leave the shell running.

To stop a shell:

1. Choose **Stop** on its line. You can also right-click the row and choose **Stop shell…**, then pick the shell when there is more than one.
2. Review the command, run time and number of processes in its group. While the agent is still working, the question says it may need this shell. Choose **Stop shell** to stop that group. The Claude Code session stays open.
3. The line reads "Stopping the shell…", then says whether the shell stopped. If the shell changed, choose it again from the refreshed list. If processes left the group and are still running, the result names their process IDs.

**Keep running** takes the chat out of Needs you and out of the circle's count. Its line turns grey and says when it asks again, and **Stop** stays on it. The first wait is 30 minutes. Each further **Keep running** on the same shell doubles it, up to 4 hours: 30 minutes, 1 hour, 2 hours, then 4 hours each time. The companion saves the choice in its settings, so it lasts across a restart.

- The choice follows the shell, not the chat. If the agent works again and then stops, the shell still waits for its time.
- A shell with a new process ID or start time asks on its own, even when another shell of the same chat is kept.
- A kept shell does not bring a hidden circle back.
- To be asked now, right-click the row and choose **Ask about this shell again**.

Warnings start after 30 minutes by default. To change the time or turn off the warnings and action, set [`conversations.stuckShellAfter`](@/docs/global-configuration-reference.md#global-conversations).

## Archived chats {#archived-chats}

A chat moves to Archived when it is closed.

| Chat | When it archives |
|------|------------------|
| Terminal chat | As soon as no process holds it, even with an unread reply. |
| Editor panel, Claude Code background job | A day after its last write. |
| Desktop app session, Claude chat, Cowork task, ChatGPT chat, cloud session | Only when you archive it, here or in its app. |

Archiving here never changes the chat in its app. Click an archived chat to resume it.

{% <details summary="Details: unread replies, resuming, working chats, silent Codex chats"> %}
- A process that ends shows as archived within a minute. Quitting an app archives none of its chats.
- The row's tooltip on an archived chat says **Resume in Terminal**, or the app it last ran in.
- An archived chat has no read or unread state, even when it closed with a reply you have not read. It shows no ring, adds nothing to the Archived header, and does not count in the circle. You can delete it without reading it first.
- If you resume such a chat by hand, its old reply does not bring it to Needs you. It shows under Active with its mark ringed in the Needs you orange until you open it or mark it read. The tooltip says "Resumed with a reply from before that you have not read". A later reply does bring it to Needs you.
- An editor panel or background job archives after a day because nothing reliable says when it closes.
- An archived or hidden chat that starts working moves to Working as soon as its tool reports the work, before it writes a new message. For local Claude Code that is the next poll; a cloud session can take up to a minute.
- That clears the archive in `conversation-marks.json`, so the web interface agrees and the chat does not go back to Archived when the work ends.
- A local Codex chat that wrote nothing for 5 minutes moves to Needs you. Codex does not say when it waits, so the silence is all the server knows.
- Its reason line, its tooltip and the age on its meta line say "No activity for 7m". The web interface says that in place of "Waiting for you".
- **Delete permanently…** removes an archived chat. See [Delete an archived chat](#delete-an-archived-chat).
{% </details> %}

## Shared Remote Control rows {#remote-control}

A local Claude Code session shared with claude.ai through Remote Control is one row, marked **Shared**. It stays one row once it closes. Marking it read, or opening it on claude.ai, clears both.

| Event | What happens |
|-------|--------------|
| A claude.ai reply within two minutes of the session's last reply | It stays folded into the archived shared row, which shows no ring. |
| A later reply | The cloud row shows alone, under Needs you. |

The server restarting, or resuming without Remote Control, shows two rows again. Deleting the local session leaves the cloud row.

{% <details summary="Details: unconfirmed disconnect activity, restarts"> %}
- Once closed, the one row is the archived local row.
- After you delete the local session, the cloud row stays because claude.ai still keeps it.
- The link is kept in memory only.
- After a restart, the cloud row does not count a reply the local row already showed.
- One exception: the last reply reached claude.ai after the server last read the list while the session ran, and you did not mark it read. Then the cloud row counts that reply once. The archived local row shows no ring for it.
{% </details> %}

## What a chat waits for {#waiting}

In Needs you, a chat that waits for you has an orange dot with a lock, the same lock the circle shows next to its count. A chat whose only wait is a forgotten shell has a terminal icon in place of the lock, and when every waiting chat waits on a shell, the circle shows the terminal too. An unread reply has a plain orange dot, and so does a wait you already read while its subagents still work under Working.

A row that waits for you can carry [`waiting`](@/docs/cli-reference/conversations-api.md#get-api-views-conversations) with `{for, tool?, detail?}`. `for` is `permission` (approve a tool call), `answer` (a question), or `shell` (a [forgotten shell](#stop-a-forgotten-shell) is the only work left). It is omitted when unknown.

| Tool | Where the wait comes from |
|------|---------------------------|
| Local Claude Code | `waitingFor` in `sessions/<pid>.json`. |
| Codex cloud | `waitingOnApproval` is `permission`, `waitingOnUserInput` is `answer`. |
| Local Codex, Cursor, Claude app, Claude Code cloud | Not said, so no `waiting`. |

{% <details summary="Details: Claude Code values, tool and detail fields"> %}
- Codex cloud rows have no `tool`, since the cloud names no call.
- Local Codex does not record pending approvals in its rollout.
- `permission prompt` and `sandbox request` are `permission`. `input needed` is `answer`. Other values, such as `dialog open`, give no `waiting`.
- For a permission, `tool` and `detail` come from the oldest tool call at the end of the transcript that has no result yet.
- `tool` is the name Claude Code writes, such as `Bash`, `Edit` or `mcp__claude_ai_Notion__notion-fetch`.
- `detail` for `Bash`: the first words of the command, about 30 characters, with `…` when cut. A leading `cd <folder> &&` step is left out.
- `detail` for `Edit`, `MultiEdit`, `Write`, `Read` and `NotebookEdit`: the file name.
- `detail` for `WebFetch`: the domain.
- `detail` for an MCP tool: the server and action, such as `Notion fetch`. A server registered by id is named from its action, so the id never shows.
- Other tools have no `detail`.
- A call whose line is larger than the transcript end the server reads gives no `tool`.
- `detail` is left out when it, or the first 4 KB of the text it was read from, may hold a credential, by the rule `reply` uses.
{% </details> %}

## Chats archived in their app {#chats-archived-in-their-app}

A chat you archived in its own app shows under Archived, so un-archiving it in the app brings it back.

| Chat | Archived in its app |
|------|---------------------|
| Codex thread, Cursor editor chat, Claude Cowork task | Listed as archived. |
| Claude chat | Listed as archived when it leaves the app's list. |
| ChatGPT chat, `cursor-agent` chat | Not known: no archive flag. A normal row. |

A chat that waits for you or works still shows under Needs you or Working.

{% <details summary="Details: when an archive ends"> %}
- The [conversations API](@/docs/cli-reference/conversations-api.md#get-api-views-conversations) lists these chats as `dormant`, and the web interface lists them by date.
- The server reads the app's archive flag on each read and writes nothing to `conversation-marks.json`.
- Codex keeps the time it archived a thread. Cursor and Cowork keep no time for the archive.
- ChatGPT's chat list and `cursor-agent` keep no archive flag.
- A tool's `count` includes its archived chats.
- The archive holds until something is written in the chat after it, or its tool says the agent is working, as an archive mark of your own does. The chat then shows by its usual state, even while its app still says archived.
- Un-archiving and archiving again in the app starts a new archive.
- A write counts when it comes after the app's archive time, in whole seconds.
- When the app keeps no time, a write counts when it comes after the last write the server saw when it first read the chat as archived.
- The server keeps that first write, and whether a write or work ended the archive, in memory only. A restarted server starts again from what the app says.
{% </details> %}

{% <details summary="Details: Claude app chats and deleted chats"> %}
- The Claude app keeps no archive flag here, only its list of chats that are not archived, and only the pages of it you scrolled to. The server remembers the Claude chats it saw in that list, in memory.
- A chat that leaves the list shows under Archived, as last seen, when it is not older than the oldest chat still in the list. That shows it did not just fall off the pages the app keeps.
- A chat that leaves the list and is older than every chat still in it may sit on a page the app no longer keeps. It leaves the agnostic-ai list, and the server forgets it.
- A chat you deleted in the app cannot be told apart from one you archived. It shows under Archived until it is older than the oldest chat in the list.
- A chat back in the list, after you un-archive it, is a normal row again.
- A read of the app's storage that fails says so in the Claude Code tool's note, and nothing is read as archived from it.
- A restarted server remembers nothing, so a chat archived while no server ran is not listed.
- A chat deleted in its app leaves the list once its store no longer holds it.
- Its marks leave `conversation-marks.json` once full reads have missed it for 30 days. This holds only for Claude Code, Codex and `cursor-agent`, whose stores the server reads in full. See [Marks](@/docs/cli-reference/conversations-api.md#get-api-views-conversations).
{% </details> %}

## Delete an archived chat {#delete-an-archived-chat}

Remove a chat from this computer for good.

1. Right-click a chat under Archived and choose **Delete permanently…**.
2. Read the confirmation. It names the chat and what goes.
3. Confirm. **Cancel**, Escape or a click outside the question keeps the chat. The list then says how much space it freed.

| Chat | Can be deleted |
|------|----------------|
| Claude Code, on this computer | Yes |
| Codex | No. Close it, then run `codex delete <id>`. |
| Cursor editor chat, cloud session | No. Delete it where it lives. |
| `cursor-agent` | Not yet. |

A chat that cannot be deleted says why when you choose the item.

{% <details summary="Details: when it is offered, refusals, limits"> %}
- Codex deletes a thread even while it is open, so close it first.
- Delete a Cursor editor chat in Cursor, and a cloud session where it runs. `cursor-agent` chats cannot be deleted yet.
- Cursor keeps every chat in one shared database.
- Offered on any archived chat that no process holds and whose latest reply you have read: one you marked complete, or one that is closed.
- Closed means right away for a Claude Code, Codex or `cursor-agent` chat run in a terminal, and after a day for an editor panel or a background job.
- A chat that went quiet by itself is checked again when you confirm. If it closed with a reply you have not read, or anything was written in it while the delete runs, it is not deleted.
- A refused or partial delete keeps the row. The reason stays above the list until you dismiss it, with any folder still on disk.
- [`POST /api/conversations/delete`](@/docs/cli-reference/conversations-delete-api.md#post-api-conversations-delete) lists what is removed and what stays, such as Claude Code's own prompt history.
- Limit: a session that starts in the moment between the last check and the removal is not seen.
- Limit: a forked Claude Code session may still point at saved tool output in its parent's folder. Deleting the parent removes that output.
{% </details> %}

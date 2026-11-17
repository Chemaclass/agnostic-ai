+++
title = "Companion list"
description = "What the companion's list does when you click, send to, rename, mark or delete a conversation, and how it groups and archives them."
weight = 47

[extra]
group = "Reference"
+++

# Companion list

How the list of the [companion](@/docs/cli-reference/companion.md) opens, sends to, marks, archives and deletes conversations.

## The list {#the-list}

Click the companion circle, or press its shortcut, for a searchable list of your conversations. Each row opens its chat where it lives. The groups, in order, are Needs you, Working, Active and Archived.

| Key | What it does |
|-----|--------------|
| Ctrl+Alt+V (⌃⌥V on a Mac) | Send copied text. Opens the picker from any app. Change it under **Send copied text** in the list's ⋯ menu or the circle's right-click menu. |
| Cmd+Shift+V | With the list already open, opens the same picker and keeps your search. |
| Arrow keys, Enter | Pick a chat in the picker, or open the selected chat. |
| Escape | Leave the picker without sending, or dismiss a rename. |
| F2 | Rename the selected row. |

## Open a chat

Click a row. A running chat comes to its Terminal or Ghostty tab. An ended chat resumes in a new tab of the app it last ran in. The first time, macOS asks to let the companion control each of those apps.

When the chat does not open, the reason stays above the list until you dismiss it or open another chat. **Copy resume command** copies the command to run in a terminal.

{% <details summary="Open details: background sessions, slow opens, resume command"> %}
- When only the app can come forward, the list stays open and says where the chat runs.
- A Claude Code session started with `claude --bg` runs in Claude's own background service, not in a terminal app. Clicking it selects a Terminal or Ghostty tab that already runs `claude attach` on it, named by its short id, such as `e3469759`, or its exact full name. The companion does not recognize a tab attached with the start of an id or part of a name, since that may name another session.
- With no such tab, it opens a new tab that runs `claude attach <id>` in the session's folder, in the app the session last ran in when it remembers one. The short id comes from `claude agents --json`. The companion looks for `claude` on `PATH`, then in `~/.local/bin`, `~/.claude/local`, `/opt/homebrew/bin` and `/usr/local/bin`. Leaving that tab does not stop the session.
- When switching to a tab fails, or `claude agents` cannot run or does not list the session, the list offers the `claude attach` command to copy, or says to find its id with `claude agents` and run `claude attach <id>`.
- An open that takes over 60 seconds keeps the list hidden and goes on in the background. When it then fails, such as when macOS asks to allow Automation, the list shows the reason, the chat's name and **Copy resume command** as soon as it shows, whatever the search. The companion keeps that message until you dismiss it or open a chat. Another message on screen comes first, and the late one follows once you dismiss it.
- An open that gets no answer says "agnostic-ai did not answer in time. The chat may still have opened." Clicking the same chat again while it opens starts no second copy.
- **Copy resume command** is also offered for a chat still running where no tab shows it, such as in a detached `tmux` session or after its tab closed, with a note to end that process first: two copies of one session would compete. A chat whose tab was found, and a Codex thread Codex's background service keeps, never offer it.
{% </details> %}

## Send copied text

Copy text anywhere, then press the send shortcut. The list opens with the search cleared. Pick the chat that gets the text.

1. Press the shortcut. A banner shows the start of the text and says what a click or Enter does, such as "Click a chat or press Enter to paste these 14 words there".
2. Pick a chat with the arrow keys and Enter, or click it. Escape leaves without sending.
3. The chat comes to the front.

| Chat | What happens |
|------|--------------|
| Live chat in a Terminal or Ghostty tab | The text is in its input. Nothing is submitted: press Enter there. |
| Any other chat | The list says to paste with Cmd+V. |

When exactly one chat can take the text and its row shows, not inside a folded group, the text goes straight there: the banner names the chat, the text flies into its row, and the list hides. Text copied from a web page goes as Markdown when it holds links, tables, code blocks, headings or lists, and the reply says so.

Pasting into Terminal needs macOS Accessibility access, granted once in System Settings › Privacy & Security › Accessibility to the companion or the terminal that runs `agnostic-ai serve`.

{% <details summary="Send details: which chats are offered, refusals, other chats"> %}
- The picker offers this machine's chats under Needs you, Working and Active. Archived chats are never offered, even in a search; the search narrows the rest.
- While a search is on its way Enter does nothing. Its answer keeps the selected chat when it still matches, else selects the first.
- Cmd+Shift+V in the open list never sends by itself.
- Nothing copied shows "Nothing to send: copy some text first.", and a clipboard the companion cannot read says so. Neither sends anything.
- See [`POST /api/conversations/send`](@/docs/cli-reference/conversations-api.md#post-api-conversations-send) for what is refused.
{% </details> %}

## Rename a chat

Right-click a row and choose **Rename…**, press F2 on the selected row, or use the pencil button on the hovered row. Enter saves the name and Escape keeps the old one. A click on the name still opens the chat.

The chat's own tool takes the name first, so its list shows the same name when you jump to the chat. Then the server saves it in `conversation-marks.json`, so every `serve` on the machine shows it and finds it in a search. A new message keeps the name. A running chat's Terminal or Ghostty tab takes the name as its title, and keeps it after the session ends.

| Chat | How it is renamed |
|------|-------------------|
| Claude Code | Adds the `custom-title` line Claude Code's own `/rename` writes to the transcript. A running session may write its old name back. |
| Claude Code cloud session | Changes the title on claude.ai/code, signed in with the login Claude Code keeps in the macOS keychain. |
| Codex | Asks `codex app-server` to rename the thread (`thread/name/set`). `codex` must be installed. |
| Cursor | An editor chat's name changes in Cursor's state database, in the chat list and in the chat's own data, and nothing else. Cursor may write the old name back if it has the chat loaded. A cursor-agent chat's name changes in its `store.db`, only when a look at the open files just before the write finds the chat not running. The rename keeps the done and read marks. Both need `sqlite3`. |
| ChatGPT | Asks chatgpt.com to change the title, signed in with the ChatGPT login the Codex command keeps. The ChatGPT app shows it once it syncs. |
| Claude app chat, Cowork task, Codex cloud thread | Cannot be renamed yet. |

When the tool does not take the name, the field stays open with the reason under it, such as "Codex is not installed" or "Sign in to ChatGPT in the Codex app", and the row keeps its old name. The server logs what the tool said on its standard error, with the tool and the conversation id. Right-clicking does not open the chat or mark it as read.

## Mark chats as read

**Mark as read** (right-click a chat that needs you) clears it without opening it. It saves the same seen mark that opening saves. The chat leaves Needs you in the companion and the web interface, and moves where an opened one would: Active, or Archived when it is closed.

**Mark all as read**, beside Needs you, does the same for each conversation the group shows, after the search and the All, Local or Cloud choice.

{% <details summary="Mark details: failures, skipped chats, the circle count"> %}
- See [`state`](@/docs/cli-reference/conversations-api.md#get-api-views-conversations) for when a closed chat moves at once and when it waits a day.
- On an archived chat that closed with a reply you have not read, **Mark as read** clears the orange ring. Its next write brings it back.
- If the save fails, the row stays and the list shows why.
- **Mark all as read** saves one chat after the other. A conversation written in since the list was drawn is skipped and stays. The first failed save stops the run and says why.
- The list holds every blocked and unread conversation of any age. The circle counts them all, whatever the search or place, so with **All** chosen and no search it shows zero after the run until something new is written.
{% </details> %}

## Groups and rows

| Group | What it holds |
|-------|---------------|
| Needs you | Chats waiting for a permission first, then for an answer, then unread replies, each newest first. |
| Working | Chats whose tool says the agent is busy. |
| Active | Open chats, and desktop app sessions, Claude chats, Cowork tasks, ChatGPT chats and cloud sessions until you archive them. |
| Archived | Closed chats, and chats you archived in their app. |

A row shows the title, the agent's latest reply in one line, and a meta line: the tool, where it runs when not here (Cloud, Chat, Cowork or Shared), the folder and the age. Needs you already says that a chat waits, so the row does not repeat it. A chat started with [`conversations start`](@/docs/cli-reference/conversations.md#the-link-line) sits indented under the chat it came from while both are in the same group.

While a tool call waits for your approval, an orange pill replaces the reply line. For example "Approve `git push origin…`", "Approve edit `dashboard.tsx`" or "Approve `github.com`". See [what a chat waits for](#waiting).

{% <details summary="Row details: folder names, reply line, permission pill"> %}
- A row names the conversation's folder when it has one; a cloud session has none. When two folders in the list end in the same name, the row adds the parent folders that tell them apart, such as `shop/api` and `billing/api`. The row's tooltip and screen-reader description give the full path.
- Local Claude Code, Codex and Cursor chats show the agent's latest reply: the question it asks you, or the start of its last message. Cloud sessions do not yet. A reply older than your latest prompt is not shown, and a reply that may hold a password or key is left out. See [`reply`](@/docs/cli-reference/conversations-api.md#get-api-views-conversations).
- The pill reads a key and "Approve `git push origin…`" for a command (its first words), "Approve edit `dashboard.tsx`" for an edit or a write, "Approve read `.env.example`" for a read, "Approve `github.com`" for a fetch, and "Approve Notion fetch" for an MCP tool.
- When the call is not known, as for a Codex cloud thread, a lock and "Needs permission" show instead. A detail that may hold a password or key is left out, and the pill names the tool only.
- The agent's reply moves to the row's tooltip. A question, or a wait whose kind the tool does not say, keeps the reply line.
- A child chat that waits for you stays under Needs you, even when its parent is active or archived. A chat started from another one stays under it, and the two move up together to the place of the more urgent one.
{% </details> %}

{% <details summary="Archiving and moving between groups"> %}
- A terminal chat goes under Archived as soon as no process holds it, whatever its age, even with a reply you have not read. A process that ends shows within a minute.
- Such an archived chat with an unread reply has its mark ringed in the Needs you orange, and its tooltip says "Closed with a reply you have not read". It does not count in the circle. Its right-click menu offers **Mark as read**, and **Delete permanently…** only once it is read.
- While Archived is folded, its header says how many such chats it holds, such as "2 unread". Once you unfold it, its **Mark all as read** clears the markers of the chats it shows.
- Clicking it resumes it: its tooltip says **Resume in Terminal**, or the app it last ran in. When you resume it by hand instead, its old reply does not bring it to Needs you; it shows under Active with the same ring until you open it or mark it read, and its tooltip says "Resumed with a reply from before that you have not read". A later reply does bring it to Needs you.
- A desktop app session, Claude chat, Cowork task, ChatGPT chat or cloud session stays under Active until you archive it, here or in its app, however old it is. Quitting the app archives none of them. An editor panel or a Claude Code background job still goes under Archived a day after its last write: nothing reliable says when it closes. Archiving here never changes the chat in its app.
- An archived or hidden chat that starts working moves to Working as soon as its tool reports the work, before it writes a new message. For local Claude Code that is the next poll; a cloud session can take up to a minute. The archive is cleared in `conversation-marks.json`, so the web interface agrees and the chat does not go back to Archived when the work ends.
- A local Codex chat that wrote nothing for 5 minutes moves to Needs you, and its tooltip says "No activity for 7m": Codex does not say when it waits, so the silence is all the server knows. The age on its meta line shows the same. The web interface says "No activity for 7m" in place of "Waiting for you".
- A Codex thread, Cursor editor chat, Claude chat or Claude Cowork task you archived in its own app shows under Archived. See [Chats archived in their app](#chats-archived-in-their-app).
- **Delete permanently…** removes a chat under Archived from this computer, after a confirmation. See [Delete an archived chat](#delete-an-archived-chat).
{% </details> %}

{% <details summary="Remote Control: one row for a local and a cloud session"> %}
A local session shared with claude.ai through Remote Control stays one row once it closes: the archived local row, still **Shared**. Marking it read, or opening it on claude.ai, clears both.

- A reply written on claude.ai within two minutes of the session's last reply rings that row; a later one shows the cloud row on its own, under Needs you.
- If claude.ai records the disconnect itself as activity, which is not confirmed, a session closed within two minutes of a reply you already read can show the ring for no new reply; mark it read to delete it.
- The server keeps this link in memory: after a restart, or a resume without Remote Control, the two show as two rows again, but the cloud row does not count a reply the local row already showed. One exception: if the last reply reached claude.ai after the server last read the list while the session ran, and you did not mark it read, the cloud row counts that reply once and the local row keeps its ring for it too.
- Deleting the local session leaves the cloud row listed, since claude.ai still keeps it.
{% </details> %}

## What a chat waits for {#waiting}

A row that waits for you carries [`waiting`](@/docs/cli-reference/conversations-api.md#get-api-views-conversations) when its tool says what for: `{for, tool?, detail?}`, where `for` is `permission` (a tool call to approve) or `answer` (a question). It is omitted when the kind is unknown.

{% <details summary="Where each tool's wait comes from"> %}
- Local Claude Code: `for` comes from the `waitingFor` field the running process writes to `sessions/<pid>.json`. `permission prompt` and `sandbox request` are `permission`, `input needed` is `answer`. Other values, such as `dialog open`, give no `waiting`.
- For a permission, `tool` and `detail` come from the oldest tool call at the end of the transcript that has no result yet. `tool` is the tool's name as Claude Code writes it, such as `Bash`, `Edit` or `mcp__claude_ai_Notion__notion-fetch`. `detail` is short: the first words of a `Bash` command, about 30 characters, with `…` when words were cut and a leading `cd <folder> &&` step left out; the file name for `Edit`, `MultiEdit`, `Write`, `Read` and `NotebookEdit`; the domain for `WebFetch`; the server and action for an MCP tool, such as `Notion fetch`. A server registered by id is named from its action, so the id never shows. Other tools have no `detail`. A call whose line is larger than the transcript end the server reads gives no `tool`.
- `detail` is left out when it, or the first 4 KB of the text it was read from, may hold a credential, by the rule `reply` uses.
- Codex cloud: `waitingOnApproval` is `permission` and `waitingOnUserInput` is `answer`. The cloud names no tool call, so there is no `tool`.
- Local Codex does not record pending approvals in its rollout, and Cursor, the Claude app and Claude Code cloud sessions do not say what they wait for, so their rows have no `waiting`.
{% </details> %}

## Chats archived in their app

A chat you archived in its own app shows under Archived. The [conversations API](@/docs/cli-reference/conversations-api.md#get-api-views-conversations) lists it as `dormant`, and the web interface lists it by date, like any closed chat. The server reads the app's own archive flag on each read and writes nothing to `conversation-marks.json`, so un-archiving it in the app brings it back.

| Chat | Archived in its app |
|------|---------------------|
| Codex thread | Listed as archived. Codex keeps the time it archived the thread. |
| Cursor editor chat | Listed as archived. Cursor keeps no time for the archive. |
| Claude Cowork task | Listed as archived. The app keeps no time for the archive. |
| Claude chat | Listed as archived when it leaves the app's list. See below. |
| ChatGPT chat | Not known. The ChatGPT app's chat list keeps no archive flag. |
| `cursor-agent` chat | Not known. `cursor-agent` keeps no archive flag. |

{% <details summary="When an archive ends, and what stays"> %}
- The archive holds until something is written in the chat after it, or its tool says the agent is working, as an archive mark of your own does. The chat then shows by its usual state, even while its app still says archived. Un-archiving and archiving it again in the app starts a new archive.
- A write counts when it comes after the app's archive time, compared in whole seconds. When the app keeps no time, it counts when it comes after the last write the server saw when it first read the chat as archived.
- The server keeps that first write, and whether a write or work ended the archive, in memory only. A restarted server starts again from what the app says.
- A chat that waits for you or works still shows under Needs you or Working, as with your own archive.
- A tool's `count` includes its archived chats.
{% </details> %}

{% <details summary="How Claude app chats are archived"> %}
The Claude app keeps no archive flag on this computer, only its list of chats that are not archived, and only the pages of it you scrolled to. So the server remembers the Claude chats it saw in that list, in memory:

- A chat that leaves the list shows under Archived, as it was last seen, when it is not older than the oldest chat still in the list. That shows it did not just fall off the pages the app keeps.
- A chat that leaves the list and is older than every chat still in it may only sit on a page the app no longer keeps. It leaves the agnostic-ai list, and the server forgets it.
- A chat you deleted in the app cannot be told apart from one you archived. It shows under Archived until it is older than the oldest chat in the list.
- A chat back in the list, after you un-archive it, is a normal row again.
- A read of the app's storage that fails says so in the Claude Code tool's note, and nothing is read as archived from it.
- A restarted server remembers nothing, so a chat archived while no server ran is not listed.
{% </details> %}

The ChatGPT reader has no archive flag, so a chat archived in ChatGPT stays a normal row. A chat deleted in its app leaves the list once its store no longer holds it. Its marks leave `conversation-marks.json` once full reads have missed it for 30 days, only for Claude Code, Codex and `cursor-agent`, whose stores the server reads in full. See [Marks](@/docs/cli-reference/conversations-api.md#get-api-views-conversations).

## Delete an archived chat

Right-click a chat under Archived and choose **Delete permanently…**. The confirmation names the chat and what goes. Nothing is sent until you confirm, and **Cancel** or Escape keeps it.

{% <details summary="When delete is offered, and what happens"> %}
- It is offered on any chat under Archived that no process holds and whose latest reply you have read: one you marked complete, or one that is closed: right away for a Claude Code, Codex or `cursor-agent` chat run in a terminal, after a day for an editor panel or a background job.
- A chat that went quiet by itself is checked again when you confirm. If it closed with a reply you have not read, or anything was written in it while the delete runs, it is not deleted.
- Afterwards the list says how much space it freed.
- A refused or partial delete keeps the row. The reason stays above the list until you dismiss it, with any folder still on disk.
{% </details> %}

| Chat | Can be deleted |
|------|----------------|
| Claude Code, on this computer | Yes |
| Codex | No. Codex deletes a thread even while it is open. Close it first, then run `codex delete <id>` in a terminal. |
| Cursor editor chat | No. Cursor keeps every chat in one shared database. Delete it in Cursor. |
| `cursor-agent` | Not yet. |
| Any cloud session | No. Delete it where it runs. |
| Any chat on Windows | No. The server cannot tell whether a session still runs. |

A chat that cannot be deleted says why when you choose the item, and sends nothing. [`POST /api/conversations/delete`](@/docs/cli-reference/conversations-api.md#post-api-conversations-delete) lists what is removed and what stays, such as Claude Code's own prompt history.

{% <details summary="Accepted limits"> %}
Two limits are accepted. A session that starts in the moment between the last check and the removal is not seen. A forked Claude Code session may still point at saved tool output in its parent's folder, and deleting the parent removes that output.
{% </details> %}

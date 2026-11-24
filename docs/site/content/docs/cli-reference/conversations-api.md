+++
title = "Conversations API"
description = "Routes of the local JSON API that list, open, send to, mark, rename and delete conversations, and what each state means."
weight = 46

[extra]
group = "Reference"
+++

# Conversations API

These routes of the [local API](@/docs/cli-reference/serve-api.md) serve the Conversations list and the [companion](@/docs/cli-reference/companion.md). Use them to list your AI chats, reopen one where it lives, send it text, or mark it read.

The three most common calls, against `agnostic-ai serve --scope user` on the default port:

```bash
curl -s "http://127.0.0.1:4310/api/views/conversations?tool=claude"
curl -s -X POST http://127.0.0.1:4310/api/conversations/open \
  -H 'Content-Type: application/json' -d '{"tool":"claude","id":"<id>"}'
curl -s -X POST http://127.0.0.1:4310/api/conversations/mark \
  -H 'Content-Type: application/json' -d '{"tool":"claude","id":"<id>","as":"seen"}'
```

The first call lists chats and their `state`. The second brings a live one to the front; resuming an ended one [needs proof](#post-api-conversations-open). The third marks one read. POST routes must come from the server's own origin. See [Shell API](@/docs/cli-reference/conversations-shell-api.md) to stop a forgotten shell.

## `GET /api/views/conversations`

List conversations, newest first, one week per page.

| Query | Meaning |
|-------|---------|
| `editor=<id>` | The editor chosen in Settings, so editor links use its scheme. |
| `before=<time>` | The next page, which holds older conversations. |
| `q=<words>` | A search. |
| `tool=claude`, `codex`, `cursor` or `chatgpt` | List one tool. |

Each row has a `state`, the first of these that holds:

1. `blocked`: open, and its tool says it waits for you.
2. `working`: open, and its tool says the agent is busy.
3. `dormant`: [archived in its app](@/docs/cli-reference/companion-list.md#chats-archived-in-their-app).
4. `completed`: you marked it completed. It holds until something is written or its tool reports work.
5. `dormant`: a terminal session no process holds, even with a reply you have not read.
6. `unread`: written after you last saw it, the applicable history cutoff, and any last write the server recorded while the session was closed.
7. `active`: open; a desktop, chat, Cowork or cloud conversation of any age; or written within 24 hours when the server cannot tell whether it is closed.
8. `dormant`: otherwise.

{% <details summary="Search, paging and scope"> %}

A search keeps the conversations of every age where each word appears in the title, folder, branch, latest prompt, latest reply, tool or app, as the row shows them. Case is ignored.

- A withheld field does not match.
- Its pages hold only matches, and the page adds `matches`, the count across all pages.
- A `q` over 200 characters, or another `tool`, is a 400.

The route lists Claude Code, Codex and Cursor conversations, and ChatGPT chats when its desktop app is installed, from each tool's own store. The newest come first, by the last message from the person or the agent.

- Each page holds one week. The first page holds every open conversation, every blocked or unread one, and the past week, so, without a search or tool filter, it holds every conversation `needsYou` counts. A later page leaves those out.
- A page asked for with `before` holds the week before that time, and reaches past weeks with no conversation.
- The page returns `from` and `to`, and `next`, the `before` value of the following page, absent when no older conversation remains.
- A `before` that is not an RFC 3339 time is a 400.
- A project server lists the conversations started in the project or one of its Git worktrees. A user-level server lists them all.
- Runs a program started (Claude Code SDK sessions, `codex exec`) are left out. A `codex exec` run that a local Claude Code session started shows in that session's `helpers` instead while it runs.
- A Claude Code session or Codex thread that was opened and never typed in is left out: it has no title and no prompt to show. A Codex thread stays when the agent replied, or when you named it with `POST /api/conversations/rename`. A left-out one is not counted in `needsYou` or a tool's `count`.

Codex and Cursor stores are read with `sqlite3`.
{% </details> %}

{% <details summary="Response fields"> %}

Returns `tools` and `conversations`:

- `tools` is `{tool, label, found, count, shown, note?, needs?}`. `count` is every conversation the tool holds, `shown` those on this page, `note` why a store was not read, `needs` the missing command (`sqlite3`).
- Each of `conversations` is `{tool, app, id, title, folder?, branch?, startedAt?, updatedAt, opening?, prompt?, reply?, withheld?, live?, activity?, waiting?, quietSince?, workingSubagents?, helpers?, stuckShells?, stoppable?, deleteUnsupported?, parent?, cloud?, cloudResumeCommand?, approvals?, threads?, claimCount?, claimedPaths?, claimConflicts?, open}`.
- With the [mailbox](@/docs/mailbox.md#hand-off-files), a row that holds file claims has `claimCount` and `claimedPaths`, a row in an active path conflict has `claimConflicts`, each with an `id` for the dismiss route, and the view has `mailboxClaims: {paths, stale, conflicts}` while any claim or conflict exists.
- `approvalsMode` is the loosest active mode for the project or user list, `review` or `yolo`. It is omitted when inactive.
- [Mailbox routes](@/docs/mailbox.md#mailbox-api).

Conversation fields:

- `app` is `cli`, `ide`, `desktop`, `background`, `cloud`, `chat` or `cowork`.
- `folder` is where the conversation started and resumes.
- `live` is true while it is open: a running Claude Code process holds it, a Codex process has its thread loaded, a running `cursor-agent` has its chat open, a Cursor window shows it selected, or a cloud provider reports an active session.
- `activity` is `working`, `waiting` or `idle` when available. Claude Code reports local session status. Local Codex activity is inferred from its rollout. Cloud sessions use provider status.
- `waiting` is `{for, tool?, detail?}`. See [what a chat waits for](@/docs/cli-reference/companion-list.md#waiting).
- `approvals` is `{approved?, recent?, reason?}`: how many requests the approvals hook approved for this session, the latest 10 of them as `{time, call, detail?}` newest first, and why a permission wait reached you. With an active Claude Code `yolo` hook, a wait the hook may still answer appears as `working` and stays out of `needsYou` across all pages and filters. See [approvals](@/docs/cli-reference/companion.md#approvals).
- `quietSince` is set only with a `waiting` activity that was inferred from silence, not reported by the tool. It is the time of the last write, so a client can say how long nothing came. Today only local Codex sets it.
- `workingSubagents` is an optional native task count for local Claude Code and Codex. Zero means none; omission means unavailable. Claude uses task dispatch, launch, completion, stop and resume events from the current live process; Codex uses native parent relationships, live writer locks and unfinished turn events, including nested workers. It does not count linked child conversations or cloud workers. An unfinished native task may be waiting on a tool. Native logs that cannot be read leave the count unavailable. Worker event writes invalidate the cached list; process exits can take up to a minute to show. Positive counts keep an otherwise idle parent in `working`, while a parent's unread question still takes priority.
- [`helpers`](@/docs/cli-reference/conversations-helpers-api.md#helpers) lists the subagents, teammates and `codex exec` runs a chat started that still work.
- [`stuckShells`](@/docs/cli-reference/conversations-shell-api.md#stuck-shells) lists forgotten shells.
- [`stoppable`](@/docs/cli-reference/conversations-shell-api.md#stoppable) marks a chat the server can stop.
- `unread` is `true` on an `active` row when the server confirms the session is open and you have yet to see its latest write. This happens when you resume a terminal session by hand after the server recorded a reply while it was closed. It stays until you open the chat or mark it read. It does not count toward `needsYou`.
- `deleteUnsupported` is set only on a `completed` or `dormant` row that `POST /api/conversations/delete` cannot delete. It says why, such as for a Codex thread or a cloud session.
- `cloudResumeCommand`, on Claude Code cloud rows only, resumes the session in a terminal.
- `opening` holds up to three lines of the first prompt, and `prompt` holds the latest one.
- `parent` is `{tool, id}`, the conversation this one continues. It is read from the line `Continues <tool>:<id>` that starts the first prompt, which [`conversations start`](@/docs/cli-reference/conversations.md#conversations-start) writes. That line is cut from `title`, `opening` and `prompt`. The parent may be on no page: hidden, older than the page, of another tool than the filter, or deleted. Hiding, archiving or renaming a parent never changes its children.
- `cloud` is `{id, link}`, set on a local Claude Code session that Remote Control also shows on claude.ai: a live one, or one that closed while this server ran and whose cloud session wrote nothing more than two minutes after the session's last write. A restarted server sets it again only once the session runs; the cloud row it lists meanwhile is not `unread` for a write the local row showed, except a last reply that reached claude.ai after this server last read the list while the session ran and was never marked read. That cloud session is not listed as its own row. `POST /api/conversations/open` with the cloud `id` opens `link`. See [Remote Control](@/docs/cli-reference/companion.md#claude-code-cloud-sessions).
- `reply` is the agent's latest message since the latest prompt, as one line of at most 200 characters. When the agent asks a question, the line is the question. Otherwise it is the start of the message. It never holds tool calls, tool results or subagent output.
- Only local Claude Code, Codex and Cursor conversations have a `reply`.
- Any of `title`, `opening`, `prompt` and `reply` that may hold a credential is left out and named in `withheld`, by the rule `/api/views/source` uses. The check reads the whole title and prompts, and the first 4 KB of a reply.
- `title` is the name you chose with `POST /api/conversations/rename`, when you chose one. Otherwise it is the tool's own title, or the first line of the first prompt.
- `open` is `{kind, label, command?, link?}`:
  - `focus` switches to the tab (and pane) that runs it in a tab app: Terminal, Ghostty, iTerm, WezTerm or kitty on macOS, and WezTerm, kitty or Konsole on Linux. Inside tmux it selects the pane first. `label` then names the app, such as `Switch to Ghostty`. For a Codex thread that Codex's background service holds, the server looks for the Codex terminal that shows it. Launch times and the thread's folder can suggest a terminal even after a delayed first prompt. If they point to just one possible tab in that folder, the server tries to select it and marks it as a guess in the reply.
  - `terminal` runs `command` in a new tab of the app the session last ran in. It never types into the old tab: a shell waiting in `read` looks the same as one at its prompt. With no tab remembered, it opens a new Terminal window. `label` then says it resumes, such as `Resume in Ghostty`. A `cursor-agent` chat whose folder is not known has no `command`, because `cursor-agent` resumes a chat only from the folder it started in; opening it says so.
  - `link` opens the editor extension, Cursor chat or desktop app it came from, or a cloud session in its browser or app. For an editor extension, the server first opens the conversation's folder in that editor, so the link reaches the window that holds it.
{% </details> %}

{% <details summary="How closed sessions are detected"> %}

The server knows a terminal session is closed for local Claude Code and Codex sessions started in a terminal, and for `cursor-agent` chats, on macOS and Linux: a full read of `ps` or `lsof` finds no process that holds them, whatever their age. A session file Claude Code left behind after a crash, a kill or a reboot does not count when its process is gone or its pid now belongs to another process. A session stays open while any process holds it: in a detached `tmux`, `screen` or `zellij` session, stopped with Ctrl+Z, or after its tab closed. With two holders it closes when the last one ends.

A process that ends shows within a minute: the server reads processes again at most once a minute for a list that did not change.

Desktop app sessions, Claude chats, Cowork tasks, ChatGPT chats and cloud sessions stay `active` until you archive them here or in their app. Quitting the app archives none of them. A cloud session whose status says it works or waits, and that wrote nothing for 24 hours, no longer counts as working or waiting; it is `active` or `unread`, never archived by its age.

Editor panels and Claude Code background jobs keep the 24 hours: nothing reliable says when they close. So does every chat whose process read failed or cannot run: Codex and `cursor-agent` chats on a system without `lsof`, Codex threads from a version that keeps no `thread-writer-locks` folder, Claude Code sessions from a version that writes no `sessions` folder, and every chat in a scan where `ps` or `lsof` failed. So does a Codex lock or `cursor-agent` store that `lsof` cannot reach: under a folder it cannot read, or behind a link, since `lsof` skips both without saying so. So does every Claude Code session while a session file cannot be read and the process its name gives still runs; the Claude Code tool's `note` then names the file. A file whose process is gone, as a crash leaves it, is skipped. When a process read fails, whether the terminal session is closed remains unknown.

Process ids are read on this computer only. A conversation store shared with a container, WSL or another user account can name a process this computer cannot see, so that session may show closed while it runs elsewhere. The server does not detect another process namespace.

The server saves the time of the last write it read while a terminal session was confirmed closed, leaving your seen mark as it was. That write stays out of `needsYou` after a manual resume or a failed process read. After a confirmed resume, an unseen reply can show on an `active` row with `unread: true`. If the latest write is still the saved one and the process read fails, the terminal row falls back to `active` for writes less than 24 hours old and `dormant` for older writes, unless an earlier state in the list above applies. Later writes can count even if the process read still fails. If a session closes and resumes between reads, the server can show its old reply as new.

The saved write time affects the list. Deleting a session still requires a fresh check that no process or job holds it.

A Claude Code session that began in the desktop app or an editor, and was last used in a terminal, is a terminal session only while a terminal holds it. Once closed it opens where it began and follows that app's rule.

After `/clear`, an in-process `/resume` or a fork, the old session id is archived once no process holds it, and the new id stays hidden until it holds a prompt. This is tested only on the scanner's side: it assumes Claude Code points `sessions/<pid>.json` at the new id, which was not checked against Claude Code itself. A fork copies the old reply under the new id, so that reply can show under Needs you once more on the fork. What a real fork transcript looks like could not be confirmed, so nothing links the fork to its parent.

Codex thread locks and `cursor-agent` chat stores are checked with one `lsof` run over their folder, so a held thread of any age stays open and the cost does not grow with old threads.

The `activity` of a loaded local Codex thread is `working` during an unfinished turn. It is inferred to be `waiting` after five minutes without a rollout write, which makes the state `blocked` until you see it. A long-running command can also meet that threshold. Such a row carries `quietSince`, shown as "No activity for 7m".

{% </details> %}

{% <details summary="Marks, cloud sessions and refresh"> %}

Marks:

- `seenAt` and `completedAt` are your marks, when set.
- `needsYou` counts the blocked and unread conversations of every age and tool the server lists, whatever the page or the search. With the mailbox, it also counts each session in an active path conflict once, and the first page holds those sessions.
- The marks live in `conversation-marks.json` in the `agnostic-ai` folder of your user configuration folder. The file holds tool names, ids, times, the names you chose, the folder of each store and where each conversation's own file sits, never conversation text. A Claude Code project folder's name spells the folder the session ran in, and a Codex rollout outside `CODEX_HOME` is kept as a full path.
- When a scan creates the marks file, it uses the start of that scan as the initial history cutoff. For Claude chats, local Cowork tasks, remote Cowork tasks and ChatGPT chats, the first successful read of each source can move its own cutoff later. This uses the read of the app's local list before the server filters its rows. For example, a project scan reads the Claude app's list and can set its cutoffs. A project scan skips the ChatGPT query, so it leaves that cutoff unset if it did not already exist.
- An existing marks file without source cutoffs uses the later of the file's initial cutoff and the v0.96.0 release time for those four sources. Older history alone stays out of `needsYou`. Later writes can count, and explicit seen, completed and hidden marks still apply. A live session waiting for you can still count.
- An older `serve` that still runs after an upgrade drops the chosen names when it rewrites the file. Restart it, and the companion, after you upgrade.
- A read clears a completion or a hiding once a later message overtook it or the conversation started working. The entry keeps its seen time and chosen name. After a new message the conversation is unread. After work that wrote nothing it is active again when the work ends.
- `marksNote` says why the file could not be read or written. Rows then carry no marks.
- A chat its store no longer holds leaves the list on the next read. A Claude chat is the exception: it shows under Archived for a while, as the [companion list page](@/docs/cli-reference/companion-list.md#chats-archived-in-their-app) says.
- Its marks are removed once full reads of its store have missed it for 30 days, but only for a store the server reads in full: Claude Code transcripts, Codex threads and `cursor-agent` chats. The 30 days start at the first full read without the chat. A pause of more than 8 days between two such reads, such as a computer that was off, starts them again, so one read after a month away removes nothing. A pause of 7 days or less never does. A server that reads a store less often than every 8 days never removes its marks. The server saves the missing marks of one store together, at most once a day. A read that fails, is canceled, lists nothing, or cannot open a folder or a chat's store changes nothing. A chat that comes back starts the 30 days again.
- Each mark keeps which store listed it, by a short name for its folder with links followed, and where its own transcript, rollout or chat folder sits there. A server that reads another store, such as one started from a terminal with another `CLAUDE_CONFIG_DIR` or `CODEX_HOME` than the companion, never ages it. When the store's folder is gone and a full read of another store lists the chat, as after a home folder was renamed, the mark moves to that store. Two stores that both exist and hold the same chat, such as a copied folder, keep the first. Whether a store exists is judged at each read, so a store on a disk that is not mounted reads as gone: if another store holds a copy of the chat, the mark moves there and does not move back. A chat whose store moved and that is gone from the new one too keeps its marks for good, and so does a store whose folder still exists under its old path but that no server reads any more. Before a mark goes, the server checks that this file or folder is gone. When it is still there, as when Codex rebuilds its thread index, the mark stays and the 30 days start again. A mark whose file is not known stays.
- Marks of Claude chats, Cowork tasks, ChatGPT chats, Cursor editor chats and cloud sessions stay. The Claude app keeps only the pages of its list you scrolled to, a cloud list is cut at a limit, and the ChatGPT list and Cursor's editor index are not known to hold every chat. Marks saved before this release stay too, until a full read lists their chat.
- A chat that returns after its marks went, restored from a backup or synced back, loses its chosen name and its completed and hidden marks, and shows as unread.
- The file keeps fields this server does not know, so a newer server's fields survive its writes. An older server drops the store and missing times when it rewrites the file. For a listed chat that only starts the 30 days again. A chat already missing then can never be tied to its store again, so its marks stay for good.

Cloud sessions and refresh:

- On a server started with `--cloud-sessions`, Claude Code cloud sessions and Codex cloud threads are listed too, with `app` `cloud`. `cloudNote` says why they are missing when they could not be read. A machine where no one signed in to Codex has no Codex cloud threads, so that gets no note; a Codex sign-in with an API key instead of a ChatGPT login does.
- When the ChatGPT desktop app is installed, its chats are listed as the tool `chatgpt` with `app` `chat`, read from the chat list the app keeps in `$CODEX_HOME/sqlite/codex.db`, with no network call and no flag. A project server lists none. Without the app the `chatgpt` tool is left out.
- Claude chats (`app` `chat`) and Cowork tasks (`app` `cowork`) are listed under Claude Code, from what the Claude desktop app keeps on this computer. The server reads only the lists the app last fetched, never a chat's messages, and makes no network call. A chat you have not opened in the app since claude.ai changed it may be missing or out of date. These rows open in the Claude app and cannot be deleted from here. A project server lists no chats, because a chat names no folder. It lists a Cowork task whose folder is inside the project. When the app's lists cannot be read, the Claude Code tool's `note` says so and none of these rows are listed; the server reads them again after the app's next write.
- Codex and Cursor chats are read again only when their files change, for at most a minute. One of them can show as open for up to a minute after its app quit.
- The reply has an `ETag`. It changes when a chat or a mark changed, when the approvals settings or log changed, when a new minute began (a process that ended or a chat that went dormant shows no other way), or when the query differs.
- Sent back as `If-None-Match`, an unchanged list answers 304 with no body and reads no chat.

{% </details> %}

## `POST /api/conversations/open`

Body: `{tool, id, editor?, helper?}` with `Content-Type: application/json`, from the server's own origin.

- Reopens a conversation the list holds along its `open` route and returns `{opened, message, guessed?, command?, helper?, reached?}`.
- Resuming a terminal chat or attaching to `claude --bg` needs the companion's secret or the key of a tab `view` opened. Without proof, `command` gives the command to run by hand. Focusing a live tab or opening a chat in the Claude app, an editor or a browser needs no proof.
- `helper` names a subagent or teammate the conversation lists in `helpers`. See [open a helper](@/docs/cli-reference/conversations-helpers-api.md#open-a-helper) for what comes to the front, the note in `message`, and `reached`.

{% <details summary="Open details, results and status codes"> %}

- macOS selects tabs in Terminal, Ghostty, iTerm, WezTerm and kitty. Warp and editor terminals bring forward the app or editor window only. `guessed: true` identifies the only possible Codex tab.
- Linux selects tabs in WezTerm, kitty and Konsole. The window can stay behind, so the reply uses `opened: false`. Other terminals are named when known.
- macOS and Linux can select a tmux pane inside a supported terminal.
- Windows leaves existing tabs for you to select. With proof, a supported resume opens a new PowerShell window.
- `opened` is false when only the app came forward, when several Codex sessions could show the thread, when no terminal shows a running session, when the system cannot open a terminal, or when macOS refuses Automation access. `message` says where the session runs. `command`, when set, resumes the session by hand. A Claude Code or Codex session that still runs in a terminal gets none, because running it would start a second copy. When the server could not check whether a terminal session still runs, it does not resume it: `message` says so and `command` resumes it by hand once you know it has ended. That happens when the read of running processes or open files fails (`ps` or `lsof` is missing or times out on macOS and Linux, or the lookup fails on Windows), when a Claude Code session file of a running process cannot be read, or for a Codex lock in a folder the server cannot read or behind a link, or a Claude Code `sessions` folder it cannot list.
- kitty needs [remote control](@/docs/cli-reference/conversations.md#kitty-remote-control).
- The route comes from the server's own scan, never from the request.
- macOS asks once for Automation access to each app the server controls: Terminal, Ghostty and iTerm. The prompt names the companion app, or the terminal that runs `agnostic-ai serve`.
- The last confirmed tab is kept in `conversation-tabs.json`, beside the marks file: the app and the terminal device. It holds no conversation text.
- A confirmed open marks the conversation seen and clears its completion. A guessed open leaves both marks in place.
- A second open of the same conversation while one runs starts nothing: it waits for the first and returns the same reply. Opens of different conversations run side by side.
- While a list read runs, an open takes its row from the read before it, so it does not wait. A row that would resume a session waits for a fresh read instead: an ended terminal session, and any Claude Code session of the desktop app or an editor, which may run in a terminal since. Such an open can take as long as that read.
- An open may take up to 90 seconds to answer. When its caller left before that, the server logs whether it opened on its standard error, without the reason.

| Status | Meaning |
|--------|---------|
| 200 | `{opened, message, command?, helper?, reached?}`. `opened` says whether the chat came to the front. |
| 404 | A conversation outside the list. |
| 400 | Any other field, or a `helper` that is not an agent id: 1 to 128 letters, digits, `-`, `_` or `@`. |
{% </details> %}

## `POST /api/conversations/send`

Body: `{tool, id, text, html?}` with `Content-Type: application/json`, from the server's own origin, with the companion's secret or the key of a tab `view` opened ([who can change this](@/docs/approvals.md#who-can-change-this)).

Types `text` into a conversation's input, never pressing Enter, and returns `{sent, message}`. Sending from a script or any other headless caller is retired: only the companion or an unlocked `view` tab can send. A server with neither, such as a project server, one started with `--api-only`, or one on Windows, refuses every send and says it cannot type into sessions.

{% <details summary="Send details, which chats get no text, and status codes"> %}

- `sent` is true only when the text was typed into a live tab of a tab app.
- The conversation comes from the server's own scan, never from the request.
- Right before typing, the server checks that the session still runs and holds its terminal in front. An ended session is never resumed and typed into.
- `html`, the copied HTML, goes as Markdown instead when it holds links, tables, code blocks, headings or lists, and shows only words of `text`, in order. It keeps no attribute but short `http`, `https` and `mailto` link targets.
- The text keeps its newlines and tabs. Other control characters, escape sequences and bracketed paste markers are removed, and trailing newlines are trimmed.
- Every tab app but Terminal takes the text as one paste, so several lines never submit one by one. Terminal pastes the clipboard with Cmd+V. Only Markdown is put there first, and `message` says so; plain text that cleaning would change, such as a trailing newline, is not pasted.
- Never typed into: a Cursor session, which does not say when it waits; a working Codex session, which may drop the text; a session waiting for your answer, or in a state it does not report; a Codex thread shown by a client, which may have switched thread. Working Claude Code keeps the text.
- Anywhere else the server switches to the conversation as `POST /api/conversations/open` does, and `message` says to paste with Cmd+V, such as for editor terminals.
- macOS asks once for Automation access to each app. Terminal also needs Accessibility access.

| Status | Meaning |
|--------|---------|
| 200 `{sent, message}` | A send that typed, and one that did not. |
| 400 | An unknown tool or any other field. |
| 403 | No companion secret or session key, or another origin. Nothing is typed. |
| 404 | A conversation outside the list. |
| 413 | `text` is over 256 KB. |
{% </details> %}

## `POST /api/conversations/mark`

Body: `{tool, id, as}` with `Content-Type: application/json`, from the server's own origin.

| `as` | Meaning |
|------|---------|
| `seen` | Read up to now. |
| `completed` | Done until something new is written or it starts working. Also marks it seen. |
| `open` | Takes a completion back. |
| `hidden` | Left out of every list, search and count until something new is written or it starts working. The tool's own store keeps it. Also marks it seen. |

{% <details summary="Mark status codes"> %}

| Status | Meaning |
|--------|---------|
| 204 | Marked. |
| 404 | A conversation outside the list. |
| 400 | Another `as`, or any other field. |
{% </details> %}

## `POST /api/conversations/rename`

Body: `{tool, id, title}` with `Content-Type: application/json`, from the server's own origin.

Renames a conversation in its own tool first, then saves the name for every server that shares the marks file.

{% <details summary="Rename details and status codes"> %}

- Renames the conversation in its own tool first, before it replies. See [the companion's Rename](@/docs/cli-reference/companion.md) for how each tool takes the name. Claude app chats, Cowork tasks and Codex cloud threads cannot be renamed yet.
- Only once the tool has the name does it save `title` as the conversation's name in the marks file. Every server that shares the file shows it and finds it in a search. A name the tool did not take is not saved.
- The name stays after new messages. It leaves the seen, completed and hidden marks as they were.
- Spaces and line breaks are joined into one line.
- A running session's tab takes the name as its title: Terminal, Ghostty, WezTerm or kitty on macOS, and WezTerm, kitty or Konsole on Linux. The tab is named after the 204. A tab that cannot be named keeps its title. The name stays on the tab after the session ends, and names the whole tab when it holds split panes. iTerm names the pane.

| Status | Meaning |
|--------|---------|
| 204 | The tool and the marks file both have the name. |
| 404 | A conversation outside the list. |
| 400 | An empty name, a name longer than 120 characters, a name that may hold a credential, or any other field. |
| 409 | Another rename of the same conversation runs, on this server or another one that shares the marks file. Nothing changes. |
| 501 | This kind of conversation cannot be renamed yet. |
| 503 | The program the tool needs is not installed, such as `codex` or `sqlite3`. |
| 403 | The tool's sign-in is missing or expired. Or, for a Claude Code cloud session or a ChatGPT chat, the request carries neither the companion's secret nor a tab's [session key](@/docs/approvals.md#who-can-change-this). Nothing changes. |
| 409 | A running program holds the conversation and would write the old name back. |
| 502 | The tool refused the name, did not answer in time, or keeps the conversation in a way this version does not know. |

When the tool does not take the name, the reply is `{"error": "..."}` with a sentence for the person that names the tool and what to do. What the tool said goes to the server's standard error only.
{% </details> %}

## `POST /api/conversations/delete` {#post-api-conversations-delete}

See the [conversation deletion API](@/docs/cli-reference/conversations-delete-api.md#post-api-conversations-delete).

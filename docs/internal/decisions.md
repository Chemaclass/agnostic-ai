# Decision log

[Contributor docs](README.md)

Design choices and their reasons. Add new decisions without removing earlier ones.

## 001: Go over Phel/PHP

**Date:** 2026-05-03

**Context:** initial language choice. Considered Phel (Lisp on PHP) for faster development and to demonstrate the language.

**Decision:** Go.

**Why:** target users build coding tools across languages (JS, Python, Rust, Go). A PHP runtime cannot be assumed. Go ships a single static binary via Homebrew, `curl | sh`, or `go install`: no separate runtime dependencies, easy builds for other operating systems and processors (`GOOS GOARCH`). Demonstrating Phel did not justify limiting who could run the tool. Tradeoff: give up faster Lisp development to distribute the tool across language communities.

## 002: MD + YAML frontmatter for source format

**Date:** 2026-05-03

**Context:** file format for writing agents, skills, and rules. Considered YAML alone, Markdown alone, and JSON.

**Decision:** MD + YAML frontmatter.

**Why:** matches Claude Code's native format, so migration is copy-paste. Markdown body fits prompts. YAML header fields (frontmatter) are widely understood (Jekyll, Hugo, MDX). Hooks use pure YAML: they have no body, only fields.

## 003: Capability degradation over least-common-denominator

**Date:** 2026-05-03

**Context:** Codex/Gemini/Cursor lack hooks. Skills exist only on Claude. Should the spec omit unsupported features?

**Decision:** support features even when only some tools can use them. Adapters skip unsupported kinds with a warning.

**Why:** restricting specs to features every tool supports would limit Claude users because of other tools' limits. Skipping with a warning makes the gap visible without breaking sync. Future tools may close gaps (e.g. Cursor adding hooks).

## 004: Adapter packages do not import each other

**Date:** 2026-05-03

**Context:** could share more code between adapters (e.g. agent merging logic).

**Decision:** adapters share only via `internal/adapters/internal/emit`. No cross-adapter imports.

**Why:** one tool's special behavior must not affect another adapter. Adding an adapter must not require touching existing ones. Sharing adapter-specific behavior too early has caused problems in similar projects that generate output for different platforms or languages.

## 005: Jev triage for target audits, optional and measured

**Date:** 2026-09-28

**Context:** a daily `/target-audit` sends Sonnet auditors to read every changed vendor page. Most changes are harmless, and reading them all spends the maintainer's Claude or Codex usage window. TypeSafe's Jev returns structured answers (Choice, Noul) for a fraction of a cent. It needs an API key that not every machine or contributor has.

**Decision:** `scripts/jev-triage.sh` asks Jev whether each changed page contradicts any claim we make about its target, and whether it changes a project configuration feature. Jev's possible problems determine the reading order. A target whose changed pages Jev fully answers with no possible problem, a configuration-feature score under 0.1, and no `mentions:` label skips the auditor, and one such target per run is audited anyway. With no key, or on any API failure, the script writes possible problems based on matching words, and every changed page is read as before.

**Why:** On 33 labeled past findings Jev flagged every confirmed difference from vendor documentation in every run, where word matching alone flagged 12 of 18. A full audit of 25 targets with 19 changed pages cost 13 requests, 136,688 input tokens, and $0.0057, checked against the TypeSafe dashboard. The script is optional so audits still work without a key. A wrong `clear` skips a read, so skipping requires a much stricter threshold than requesting review. The random spot check tests that decision. See [Jev triage](jev-triage.md) for measurements.

## 006: Grok Build requires explicit selection

**Date:** 2026-10-23

**Context:** Grok skips ignored project instructions, while sync ignores generated output by default.

**Decision:** Register `grok` as a supported target and offer it in explicit selection, while keeping it outside `DefaultTargets`. Require `gitignore.commit: [grok:instructions]` or an intentionally disabled ignore manager, and check the native rule paths against Grok's root and global ignore inputs before emission.

**Why:** Default generation would otherwise create rule files Grok cannot read. Changing the user's ignore or trust policy silently would hide a real setup decision. This is an explicit exception to the adapter guide's default-target registration step. Setup and runtime limits live on the [Grok Build target page](../site/content/docs/targets/grok.md).

## 007: Caveman source maintenance investigation

**Date:** 2026-10-25

**Status:** Proposed, awaiting maintainer acceptance.

**Recommendation:** Retain the built-in and example pack with their current parity check. They duplicate seven pinned files; current packs cannot replace global built-in setup. No user setup or source asset changes.

The [comparison and local proof](experiments/caveman-source-decision.md) record the existing contract, three options, pinning and ownership limits, and migration approval requirements for [#2458](https://github.com/Chemaclass/agnostic-ai/issues/2458).

## 008: Check for a missing global setup on the switch, not on health

**Date:** 2026-11-01

**Context:** A project page needs to know that the global folder is missing before it opens a new tab.

**Decision:** `POST /api/switch` answers 409 with `missing` and the folder path, and starts no server. `/api/health` stays unchanged. A second endpoint, `POST /api/global/create`, exists on project servers only and takes no path.

**Why:** Health loads nothing and runs often; a folder check there adds disk work to every poll. The switch already runs on a click, and it is the one call that would otherwise start a server for an empty folder. A fixed path keeps the create call from writing anywhere else.

## 009: The macOS companion is an app that registers its own login item

**Date:** 2026-11-10

**Context:** macOS lists a bare program's launch agent with a blank terminal icon. It shows an icon and a name only for something that belongs to an app ([#3234](https://github.com/Chemaclass/agnostic-ai/issues/3234)).

**Decision:** On macOS, `companion install` puts the companion in `agnostic-ai companion.app` with the bundle ID `dev.agnostic-ai.companion`, the project icon as a one-image ICNS written in Go, and an ad-hoc signature from `codesign`. Inside an app the window library registers start at login with `SMAppService`, so the item moves to Open at Login. The install removes the bare program and its launch agent and turns start at login on for the app when it was on.

**Why:** A launch agent ties to an app only through a Team ID, which an ad-hoc build has none of. `SMAppService` registers the app itself and works for an ad-hoc app outside `/Applications`. A spike on macOS 27 showed that register and unregister both work, and that a rebuild with a new signature keeps the registration. Writing the icon in Go needs no `iconutil` or `sips` and runs in tests on any system.

## 010: Delete only what agnostic-ai can verify, transcript last

**Date:** 2026-11-11

**Context:** People asked to delete archived chats from the companion for good ([#3275](https://github.com/Chemaclass/agnostic-ai/issues/3275)). Each tool stores a conversation differently, and a delete cannot be undone.

**Decision:** `POST /api/conversations/delete` deletes only local Claude Code sessions in `completed` state. It removes `session-env/<id>`, `file-history/<id>`, `uploads/<id>` and `projects/<project>/<id>/`, each only when the name is exactly the session id, and the transcript `projects/<project>/<id>.jsonl` last. It holds the marks lock throughout, refuses when any of its files was written in or after the archive mark's second, refuses a location that is a link or holds one, and asks the process table again right before it removes anything. Shared files stay: `history.jsonl`, project memory, `tasks/session-<prefix>/` and `telemetry/`. Codex, Cursor, `cursor-agent`, cloud sessions and every session on Windows answer 422 with the reason.

**Why:** Claude Code keeps a session in files named by its full id, so ownership can be checked by name alone. Removing the transcript last means any failure leaves the row listed and archived, and the delete can be retried; the reply is 200 only once the transcript is gone. A file newer than the mark catches a resumed session that wrote only bookkeeping lines, which the list's own time ignores. Cursor keeps every chat in one shared database it holds open, with no delete command. `codex delete --force` removes a thread's row, history and rollout file, but it also deletes a thread a process has loaded, with exit 0 and no warning (tested 2026-11-11 on codex-cli 0.167.1, #3287). The removal is done by Codex, so agnostic-ai cannot check again right before it. On Windows the server cannot see running processes, so "cannot tell" would read as "not running". The process list must be readable before anything is removed, since a failed `ps` reads as "nothing runs". The session is checked again before its transcript, because the side folders can take a while.

**Accepted limits:** A session that starts in the moment between the last check and the removal is not seen. A forked Claude Code session may still point at its parent's saved tool output under `projects/<project>/<parent id>/tool-results/`, and deleting the parent removes it. On one machine 0 of 182 transcripts pointed at another session's folder, so no scan was added. A clock set back after the archive can make a later write look older than the mark, so the delete goes ahead. A resume that recreates an empty side folder, such as `session-env/<id>`, and exits without writing the transcript is not caught, because only file times are compared.

[Decision 012](#012-a-dormant-conversation-is-archived-at-the-moment-of-its-delete) extends the delete to `dormant` rows.

## 011: Model tier descriptions render after the instructions, sorted by name

**Date:** 2026-11-11

**Context:** A model tier's `description` says when to use it, and the main agent needs that text in its instructions ([#3286](https://github.com/Chemaclass/agnostic-ai/issues/3286)).

**Decision:** Sync appends one marked line after each entry point's instructions body, once the per-file view is built, and lists the described tiers sorted by name. `sync --global` puts the same line, without markers, in the block it manages. Import drops the marked line.

**Why:** Appending after the view keeps the text out of the `AGNOSTIC_AI.md` layer that `lint` and `explain --file` report, and keeps a `CLAUDE.md` that imports `AGENTS.md` as it is. Config file order is lost once tiers decode into a map; keeping it needs an order-aware decode in three loaders. Sorted order is stable at no cost.

## 012: A dormant conversation is archived at the moment of its delete

**Date:** 2026-11-11

**Context:** The companion shows `dormant` rows under Archived, beside `completed` ones, but only `completed` could be deleted ([#3288](https://github.com/Chemaclass/agnostic-ai/issues/3288)). The oldest chats often hold the most disk space, and the companion has no way to archive a row that already sits under Archived.

**Decision:** A row under Archived can be deleted when it is not running and none of its files was written at or after the moment it was archived. For `completed` that moment is the archive mark. For `dormant` it is the moment the server read the row in the fresh scan of the delete request, in whole seconds. Every other check of decision 010 applies unchanged: a fresh scan, the state read again on the server, the running check before the state, the baseline read before the measure, the process table read again before the removal and before the transcript, links refused, and the transcript last. No new state, mark, field or status code.

**Why:** Confirming the delete is the person's statement that they are done with the chat, as the archive mark is for `completed`. A message written before the server read the row makes the row `unread`, not `dormant`, so the server refuses it as not archived. A file written after the read is at or after the cutoff, so the server refuses it as changed. A later moment, such as when the marks lock is taken, would let a write between the read and that moment through. Two alternatives were rejected. Offering Archive on a dormant row costs a second step on a row already shown as archived, and checks nothing more, since the new mark would also be the moment of the click. Measuring against the last look instead would refuse every later delete when a resumed session wrote only bookkeeping, and the companion has no action to refresh that time.

**Accepted limits:** A dormant session that wrote only bookkeeping lines after the person last looked, and before the server read the row, is deleted. The list's time counts only messages from the person or the agent, so only a message makes the row `unread`.

**Update (2026-11-17, [#3527](https://github.com/Chemaclass/agnostic-ai/issues/3527)):** A terminal session that closed after a reply the person never saw is now `dormant` with `unread`, not `unread`. The delete refuses it with reason `unread` (409) until the reply is read, checked from the marks, whatever the state. Every other check above is unchanged. See [decision 018](#018-a-terminal-session-is-open-only-while-a-process-holds-it). A chat taken back out of the archive that is closed is `dormant` again: right away for a chat the server knows is closed ([#3457](https://github.com/Chemaclass/agnostic-ai/issues/3457)), otherwise once its last write is older than a day. It is still listed under Archived and can be deleted, also from a page loaded before it was taken back. Since #3457, a terminal Claude Code session can be deleted as soon as it closes, not a day later; the process table read again before the removal still refuses one that runs.

**Update (2026-11-18, [#3596](https://github.com/Chemaclass/agnostic-ai/issues/3596)):** An archived row has no read state, so a closed terminal session is `dormant` without `unread`. The delete no longer refuses it as unread, and the reason `unread` is gone. Every other check above is unchanged.

## 013: A conversation names its parent in its own first prompt

**Date:** 2026-11-11

**Context:** `agnostic-ai conversations start` opens a new conversation from inside a running one, and the list shows where it came from ([#3285](https://github.com/Chemaclass/agnostic-ai/issues/3285)).

**Decision:** The new conversation's first prompt starts with the whole line `Continues <tool>:<id>`. The scan reads it as the row's `parent` and cuts it from the title, opening and latest prompt. The brief goes to a one-time file, mode 0600 in a 0700 folder, under a random hex name. The new tab runs a fixed line, `cd '<folder>' && '<agnostic-ai>' conversations run <id>`, and the runner starts the tool with the brief as one argument. The parent is the nearest tool among the command's parent processes: a Claude Code session file whose start time matches, or a Codex process with `CODEX_THREAD_ID` or exactly one thread, or a `cursor-agent` process with `CURSOR_CONVERSATION_ID` or exactly one open chat `store.db`. On `cursor-agent` 2026.11.11-b912975 (tested 2026-11-12, #3327), that variable, the chat folder name and the `agentId` in its `store.db` are the same id, and a first prompt passed as an argument is stored inside `<user_query>`. A Cursor editor chat is refused.

**Why:** Every reader already exposes the first prompt, so the link works across tools with no shared state, and a person can type the line by hand. A link file kept by agnostic-ai would need the new conversation's id, and only Claude Code lets the caller choose it. The fixed line keeps the brief out of every shell and AppleScript string. Variables leak from an outer tool into a tool started inside it, so a tool's variable is trusted only under that tool's process.

**Accepted limits:** The tools' own history shows the link line at the start of a child. A child shows under its parent only when both sit in the same group, so a waiting child stays under Needs you. A child of a parent in another group, or one that is hidden, older than the page, filtered out or deleted, shows as a normal row. A Codex process that holds several threads, without `CODEX_THREAD_ID`, cannot be a parent. Neither can a `cursor-agent` process with several chats open and no `CURSOR_CONVERSATION_ID`.

## 014: A paused companion fades and reads nothing

**Date:** 2026-11-11

**Context:** Hide took the circle off the screen, and only the shortcut or a second `agnostic-ai companion` brought it back. A person who forgot the shortcut could not find it ([#3298](https://github.com/Chemaclass/agnostic-ai/issues/3298)).

**Decision:** Hide becomes **Pause Agnostic AI companion**. The circle stays where it is at 10% opacity, also under the pointer, with no count and no arc. The companion's server stops and nothing polls, as before. A click on the circle, the shortcut or a second start wakes it with a new read. The paused state is not saved, so a restart starts awake.

**Why:** Pause is kept apart from Dim when idle. Dim is a saved setting that fades to 55% while nothing needs you and keeps watching. Pause is the off mode you can still see and click: it costs no reads. "Pause" names a state that lasts until you resume it, and it carries the companion's name so nobody reads it as pausing the agents.

## 015: Hide replaces Pause, comes back dimmed, and Quit asks

**Date:** 2026-11-12

**Context:** Pause (014) left a faint circle on the screen, so it did not get the companion out of the way, and Quit ended the program, so the shortcut no longer brought it back ([#3344](https://github.com/Chemaclass/agnostic-ai/issues/3344)).

**Decision:** The companion has one presence state: shown, hidden or back dimmed. **Hide Agnostic AI companion** takes the circle and the list off the screen; the program keeps running and reading. Its label names the way back: the shortcut's keys, or `agnostic-ai companion` while the shortcut is not active. The shortcut, `agnostic-ai companion` or opening the app again shows the circle where it was, at full color, with the list. While hidden, a conversation that starts waiting brings the circle back at 25% with its count, without the list or the keys; a click or the shortcut brings back full color. The presence is never saved. **Quit** in either menu asks first, in the list, with Quit, Hide instead and Cancel; `stop`, install, upgrade, uninstall and logout never ask.

**Why:** A hidden companion that reads can call the person back when it matters, which 014's problem (a forgotten shortcut) needed. "Starts waiting" compares the waiting conversations of each list with the previous one, by tool and id; the list holds every waiting one of any age. A count alone would miss one conversation that starts waiting while another stops. Two lists are compared only when they could not read the same things (marks, cloud sessions, each tool's store): a list that read more becomes the new baseline instead of news, and a standing note, such as no claude.ai sign-in, still lets a new wait through. Quit asks because it is the only menu entry the shortcut cannot undo.

## 016: A Remote Control session is one row, matched by its bridge id

**Date:** 2026-11-12

**Context:** A local Claude Code session with Remote Control on is also listed among the cloud sessions, so the companion and the web interface showed it twice and counted it twice ([#3362](https://github.com/Chemaclass/agnostic-ai/issues/3362)).

**Decision:** After the scan reads local and cloud rows, a live local session whose `sessions/<pid>.json` holds `bridgeSessionId` `session_<id>` takes in the cloud row `cse_<id>`. The cloud row leaves the list; the local row keeps its state and route and gains `cloud: {id, link}`. The companion marks it **Shared** and offers **Open on claude.ai**, which sends the cloud id to the open endpoint, so the server still opens only links that match the cloud session pattern. The row shows under Local and Cloud. A cloud id that two live sessions claim, a malformed bridge id or a cloud list without that id changes nothing. A seen mark on the cloud id counts for the shared row; its completed and hidden marks and its name stay with the cloud id. Seen, completed, a completion taken back and an open on the shared row are written to both ids in one write; an open of the cloud id writes its seen mark to the local id too. A hide and a name stay on the local id. Only the open request takes the cloud id of a shared row; a mark or rename with it is refused like any id the list does not show.

**Update (2026-11-17, [#3541](https://github.com/Chemaclass/agnostic-ai/issues/3541)):** The server remembers, in memory, each link it saw while the session ran. Once the session ends, the cloud row stays folded into the archived local row, which shows the later of the two last writes. A cloud write more than two minutes after the later of the last cloud write the server saw while the session ran and the local session's last write is new work from the cloud side: the cloud row then shows on its own and counts under Needs you. The two minutes cover the cloud's copy of the last reply: the list is asked only when someone looks, so the last read that saw the session run can be long before its last reply. A cloud write inside them shows on the folded row, unread until seen. This compares a cloud time with a local file time: with the cloud's clock more than two minutes ahead of this computer's, the cloud's copy of the last reply shows the cloud row on its own and counts it twice, as before this change; with it behind, a reply from the cloud side can stay folded up to that gap longer, and while its time is older than the local row's last write it rings no row until the two show apart. A session that says it has no bridge id ends the link; a complete read that no longer lists either row drops it; a failed read keeps it. The marks file keeps, on the cloud id, the last cloud write a shared row showed (`twinShownAt`). A closed row saves a write past the one read while the session ran only once the person saw it, so a reply from the cloud side inside the window is never marked shown while only the borrowed time on the folded row rings it. The accepted cost: the cloud's copy of a last reply that the last live read missed can count once on the cloud row after a restart or a resume without Remote Control. Whenever the cloud row shows on its own, after a server restart, a resume without Remote Control or with a new bridge id, or a delete of the local session, it counts only a later write. The local row keeps its own unread marker for the old reply. An older version that rewrites the marks file drops `twinShownAt`, which brings back the double count for that reply.

**Why:** The ids are the only link the two lists share; a title, folder or time can match two different sessions. The session file is already read for the live check and the cloud list is already in memory, so the match costs one map lookup per row. A seen mark on the duplicate meant the person saw the conversation, so ignoring it would show it unread again. A hide or completion on the duplicate was often made to get rid of the second row, so it must not hide the local one.

**Accepted limits:** The relation between the two ids is undocumented and was measured on one account on 2026-11-12. If it changes, both rows show again, as before this change. Codex and Cursor cloud sessions are not matched. Whether claude.ai moves `last_event_at` when the bridge disconnects is not known: the repository holds no list response captured around a session end. If it does, a terminal closed more than two minutes after its last write shows the cloud row on its own at once, counted under Needs you; and a terminal closed inside those two minutes, after a reply already read, rings the archived row for an event that is no reply, and its delete is refused until Mark as read. Settling it needs two list responses for the same session id: one while a Remote Control session runs, one after it is closed.

## 017: Ghostty types sent text, Terminal pastes it

**Date:** 2026-11-17

**Context:** The companion sends copied text into a chat's input without submitting it ([#3517](https://github.com/Chemaclass/agnostic-ai/issues/3517)). Each terminal app offers a different way in.

**Decision:** Ghostty gets the text through `input text ... to t`. Terminal gets a Cmd+V from System Events, sent only once its tab is selected in the front window. Plain text is pasted only when the clipboard already holds it exactly, so the person's clipboard never changes; only Markdown is put there first. The server types only into a live session that holds its terminal in front, checked right before typing, and removes control characters and bracketed paste markers first.

**Why:** Ghostty's `input text` is a paste: measured on Ghostty 1.4.0, a program that asks for bracketed paste gets every line inside one paste, and the server adds no markers of its own, which would wrap the text twice. Terminal has no such command: `do script` always presses Enter, which would submit the text. A Cmd+V goes to whatever app is in front, so the script checks the front window first and refuses when Terminal did not come forward.

**Accepted limits:** Pasting into Terminal needs macOS Accessibility access. A program that does not ask for bracketed paste reads each newline as Enter in both apps; Claude Code and Codex ask for it. cursor-agent is never typed into: it was not checked, and it does not say when it waits for an answer. macOS may refuse to bring Terminal forward, and the send then says to paste by hand.

## 018: A terminal session is open only while a process holds it

**Date:** 2026-11-17

**Context:** Needs you, Working and Active should list only sessions the person can still use ([#3527](https://github.com/Chemaclass/agnostic-ai/issues/3527)). A terminal session that ended with an unread reply stayed under Needs you. Desktop, chat, Cowork and cloud conversations went under Archived after a quiet day, though they stay open in their app.

**Decision:** With a full process read, a terminal session (`app` `cli`: Claude Code, Codex and `cursor-agent`) is `blocked`, `working`, `unread` or `active` only while a process holds it. Once none holds it, it is `dormant`, or `completed` when the person archived it, and `unread: true` marks a reply not seen. It leaves `needsYou`, and a delete is refused until the reply is read. A scan that reads a closed session with an unseen reply saves that write's time as `closedWrittenAt` in its marks entry, without marking it seen. Once a process holds the session again, a write up to that time needs nobody; a later write is `unread` or `blocked` again. The resumed row keeps `unread: true`, `active` and not counted, until the person reads it, so one rule holds whether it runs or closes again. A failed read ignores `closedWrittenAt` and goes by the seen mark. The first page always holds a row with `unread`. A Claude Code session that began in the desktop app or an editor follows that app's rule once no terminal holds it. One last used in an editor or the desktop app keeps that app, since only a terminal writes `sessions/<pid>.json`. An unreadable `sessions/<pid>.json` blocks closure only while the process its name gives runs. Codex locks and `cursor-agent` stores are read with one `lsof +D` over their folder ([benchmark](benchmarks.md#which-process-holds-a-thread)). Desktop, chat, Cowork and cloud conversations are never archived by age: only an archive here or in their app archives them. A cloud status older than a day still stops counting as work, and the row falls back to `active` or `unread`. Editor panels (`ide`) and background jobs keep the one-day fallback. A live session that no tab shows gets its resume command with a note to end that process first. A closed terminal session's route says Resume. Archiving stays local: nothing is written to another app.

**Why:** A process that holds the session is the only signal every terminal tool gives, and the scan already reads it. An exit hook or a transcript write is not sent on a crash, a kill or a reboot. Archiving a closed unread session keeps Needs you for sessions the person can act on, and the marker keeps the reply in view. Saving the closed write, not a seen mark, keeps the reply unread on the archived row and still lets a later reply count after a resume. Quitting a desktop app closes none of its chats, so their age said nothing. An editor's selected tab and an attach terminal are no reliable signal for editor panels and background jobs. Starting a second copy of a running session would compete with it, so the server only offers the command.

**Accepted limits:** A process that ends shows within a minute, the bound the scan already keeps for a list that did not change. A failed or impossible process read (Windows, no `ps` or `lsof`, an unreadable session file) keeps the old fallback and never shows a session closed. So does a Codex lock or `cursor-agent` store under a folder `lsof` cannot read, or behind a link: `lsof +D` skips both without an error, so the scan checks each path it asked about with one `lstat` and each folder with one open. Process ids are read on this computer only: a store shared with a container, WSL or another account can name a process that runs elsewhere, and the server does not detect another process namespace. A session closed and resumed between two reads shows its old reply as new, since the scan saw no closure; handling that needs a resume time each tool does not give. A Remote Control session that ends keeps its cloud twin folded in while the server that saw them shared runs, as decision 016 says. After a server restart the twin shows again as its own row; it does not count a reply the local row already showed, except the unread last reply whose cloud copy came after the last live read, which counts once there (decision 016), and marks made after the restart no longer pass between the two rows. The `/clear`, in-process `/resume` and fork cases are tested on the scanner side only: they assume Claude Code points `sessions/<pid>.json` at the new id, which was not checked against Claude Code. A fork copies the old reply under a new id, so that reply can show under Needs you once more on the fork: what a real fork transcript looks like could not be confirmed, so nothing links the fork to its parent. A seen mark keeps whole seconds; every reader also writes whole seconds, so a mark in the reply's second covers it. The marks of a conversation deleted in its app are pruned as [decision 019](#019-a-mark-goes-after-30-days-of-full-reads-that-miss-its-conversation) says.

**Update (2026-11-18, [#3596](https://github.com/Chemaclass/agnostic-ai/issues/3596)):** Archived means archived: an archived row has no read or unread state. A closed terminal session with an unseen reply is `dormant` without `unread`, can be deleted without being read, and is no longer pinned to the first page. `closedWrittenAt` is still saved, so a session resumed by hand is `active` with `unread: true` until read, as above. A claude.ai reply that stays folded into a closed shared row is therefore not flagged until the cloud row shows on its own.

## 019: A mark goes after 30 days of full reads that miss its conversation

**Date:** 2026-11-17

**Context:** A conversation deleted in its app kept its entry in `conversation-marks.json` for good ([#3540](https://github.com/Chemaclass/agnostic-ai/issues/3540)). Only a read that lists every conversation a store holds can show one is gone. A read that failed, was canceled, stopped at a page or kept only a time window shows nothing. The marks file is shared by every server of the user, while each server reads the stores its own `CLAUDE_CONFIG_DIR` and `CODEX_HOME` name: the companion, started at login, sees neither variable of a shell.

**Decision:** Each reader says whether its read was complete, and then gives every id its store holds, whatever the folder filter of the scan, with where that conversation's own data sits. Three stores can: Claude Code transcripts, Codex threads and `cursor-agent` chats. The Claude Code and `cursor-agent` folders are listed with a check on each folder, since a glob skips one it cannot open without an error. A `cursor-agent` chat store whose metadata cannot be read makes the read incomplete, and so does a read that lists no conversation at all, since an empty folder can be a store that moved or a disk not mounted. The Claude app's cached sidebar, the ChatGPT catalog, Cursor's editor chat index and the cloud lists cannot. A mark records `source`, `store` (a short hash of the store's folder, absolute with links followed) and `file` (its transcript, rollout or chat folder: relative to the store, or a full path for a Codex rollout outside `CODEX_HOME`) from the complete read that listed it: when it is made, from the last read, or else at the next complete read. The file's top-level `stores` keeps each such folder by its hash. Only complete reads of that same store age it. When the recorded folder is gone and a complete read of another store lists the conversation, the mark takes that store and file. Two stores that both exist and list it keep the first, so two servers do not move it back and forth. The first complete miss sets `missingSince` and `lastMissAt`. When one missing mark of a store is due a save, a day after its `lastMissAt`, every missing mark of that store is saved in the same write. A miss more than 8 days after `lastMissAt` (7 days plus the day a save can lag), or an unreadable time, starts both again. A complete read that lists it clears both. Once `missingSince` is 30 days old, the scan checks with one `lstat` that `file` is gone, then drops the entry and the in-memory app archive it had. If `file` is still there, or not known, the mark stays, and in the first case the 30 days start again. A mark no complete read ever listed is never dropped. The pass runs inside the scan's hold on the marks lock and writes only when something changed.

**Why:** A missing transcript, thread row or chat store is a strong sign of deletion only when the whole store was read, and a complete read can still be wrong: Codex's `threads` table indexes the rollout files, and a new `state_<n>.sqlite` can start empty or part filled after an update. The check on the conversation's own file at the moment of deletion catches that, and costs one call per mark only then. Tying a mark to one store keeps a server that reads another store from aging it, and from rewriting the file on every scan. The gap rule keeps the 30 days a span of reads, so a computer off for a month removes nothing on its first read back. Saving the missing marks of a store together keeps the writes to one a day per store, whatever time of day each went missing. The file now keeps fields it does not know, so a newer version's fields survive this one's writes. A file with none is read in one strict pass, about a quarter slower than before ([benchmark](benchmarks.md#reading-and-pruning-the-marks)).

**Accepted limits:** Thirty days and seven days are choices, not measures. A server that reads a store less often than every 8 days never prunes it. A conversation whose store moved and that is gone from the new store too keeps its mark for good, and so does one whose old store folder still exists but that no server reads any more. Folder names that differ only in letter case name different stores. Whether a store's folder exists is judged at the read, so a folder on a volume that is not mounted reads as gone: when another store holds a copy of the same conversation, its mark moves there and does not move back. A store that is missing reads as incomplete, so a tool removed from this computer keeps its marks. Cursor's editor index filters subagent chats and unnamed ones, and what Cursor does with a deleted chat there was not checked, so its marks stay. So do marks from before this version whose conversation is already gone, and the marks of a chat already missing when an older version rewrote the file: that version drops `source`, `store` and `file`, and the chat can never be listed again to restore them. A Codex thread whose index row names no rollout keeps its mark. A broken link or unreadable folder under a store keeps every mark of that store until it is fixed. A chat that returns after its mark went, from a backup or a sync, loses its name and its completed and hidden marks, and shows unread. Remembered tabs already expire on their own after 30 days.

## 020: No safe key route selects a Claude Code subagent

**Date:** 2026-11-19

**Context:** Clicking a Claude Code subagent row in the companion only brings its parent's terminal tab to the front ([#3667](https://github.com/Chemaclass/agnostic-ai/issues/3667)). Sending keys could also switch the terminal to that subagent's transcript. Step 5 of the issue may do that only along a route this probe proves safe. Measured on Claude Code 2.1.328 with Haiku, driven by script in tmux 200 columns wide: one session in the fullscreen renderer (`CLAUDE_CODE_NO_FLICKER=1`, auto mode) with three background subagents and one nested one, and one in the classic renderer (`--settings '{"env":{"CLAUDE_CODE_NO_FLICKER":"0"}}'`, manual mode) with two background subagents.

**Decision:** No safe route. The server sends no keys to select a subagent. The open reply names the subagent and says how to reach it by hand: press Down until a row below the prompt is selected, move to the subagent with Up and Down, and press Enter; to go back, select `main` the same way and press Enter.

**Findings:**

- Rows. Below the prompt, `main` is always the first row, then one row per running top-level subagent, in spawn order. A row shows the agent type (or the teammate name), what it waits on, and its description. A subagent that finished leaves the list. A nested subagent is folded into its parent's row as `(+1)` and gets its own row, under a tree line, only while its parent's or its own transcript is shown. In a nested subagent's view, the list holds only `main`, its parents and itself, so the same position names a different agent in each view.
- Files. `subagents/agent-<id>.jsonl` names carry a random id (`agent-a<name>-<id>` for a named teammate), so their sort order is not the spawn order; in the fullscreen run the three top-level files were created in the same second. The spawn order is the order of the `Agent` calls in the parent transcript; `toolUseId`, `spawnDepth` and `parentAgentId` in `agent-<id>.meta.json` link each file to its call and parent. A meta file says `"finished": true` while its agent still shows as a row waiting on its own background shell.
- Keys into the rows. Down or Ctrl+N from the prompt. The number of presses depends on what the screen holds: one press reaches `main` when no background shell or monitor runs, two when one does (the first selects the footer's tasks item, where Enter opens the tasks dialog). A one-line draft needs the same count as an empty prompt. In a draft of several lines, or while browsing prompt history, each press first moves through the lines or the history. With a draft that starts with `/`, Down moves through the command menu, so a following Enter runs a command such as `/clear`. Down stops at the last row and does not wrap. Up from `main` goes back to the footer item and then the prompt.
- While the main agent works, the same keys behave the same. While a permission prompt is open (seen in the classic renderer, manual mode), the rows are hidden and Down moves the answer: two Downs and Enter answer "Yes, and switch to auto mode". The row keys, Enter and `x` behaved the same in both renderers; the nested rows and Escape were seen in the fullscreen one only.
- Stop keys. `x` on a selected row stops that subagent at once, with no question, and selection jumps to `main`. Ctrl+X Ctrl+K stops all agents. In a subagent's view, Escape with the prompt focused stops the subagent shown (the footer says "esc to stop"); from the rows, the first Escape only moves focus to the prompt. Left on an empty prompt asks to send the whole session to the background, where Enter confirms.
- Typed text. While a subagent's transcript is shown, the prompt reads "Message @<agent type>…" and its border shows the description. A message sent there goes to that subagent and wakes it. The draft is shared: text typed in a subagent's view is still in the prompt after going back to `main`, and Enter then sends it to `main`.
- `/tasks` lists background shells and monitors, teammates, and an "Earlier in this session" group of finished subagents. A running subagent that is not a teammate shows there only through a shell or monitor it started; one with neither is reachable only from the rows. Typing does not filter the list, and `x` there stops the selected shell. This differs from the subagents docs page, which says `/tasks` lists running subagents and Enter on one opens its transcript. What Enter does on a teammate row, and whether the order stays stable as tasks start and end, was not measured.
- Routes by id or name: none found. `claude --help` has no such flag; `--agent` picks the agent for the whole session. `claude attach <id>` with a subagent id answers "No session matching". `claude agents --json` lists sessions, not subagents. No file route is documented, and writing into the session folder was not tried. The keybindings name `footer:*` actions that move by position only. Hooks and the `subagentStatusLine` setting run commands whose documented output reports status; none of it changes the view. Remote Control documents no such action.

**Why:** The server sees neither the draft nor which view is shown. The count of Downs that reaches a row changes with a running shell, a draft of several lines and history browsing, and a draft that starts with `/` turns the final Enter into a command. The guards Step 5 planned (a waiting session, an unknown state, a process that does not hold the terminal, a tab that is not in front) catch the permission prompt but none of these. A Down or Enter that lands in the wrong place answers a prompt, opens the tasks dialog or runs a command. A route back to `main` cannot use Escape either: in a subagent's view, an Escape too many stops that subagent. A route that pressed Down until it stops at the last row and then counted Up would be safe only on a known empty prompt in the `main` view, and neither is visible from outside.

**macOS permissions:** Ghostty 1.4.0's scripting dictionary has `send key "<name>" to <terminal>` with modifiers, aimed at one terminal under the Automation consent that `input text` already uses (017); the launcher already sends `send key "enter"` to a new tab's shell. Terminal has no command for one key: `do script` presses Enter after its text (017), and an arrow key needs System Events `key code`, which needs Accessibility for the app that runs the script and reaches whatever app is in front. iTerm2 3.6's dictionary offers `write text`, which could carry an arrow key's escape code under Automation only; the launcher does not drive iTerm2, and this was not tried.

**Accepted limits:** Which permission dialog each terminal app shows, and whether it names the companion or the terminal that runs `agnostic-ai serve`, cannot be checked by script; that needs a hand check on macOS (the open checks in [#3601](https://github.com/Chemaclass/agnostic-ai/issues/3601)). Not checked in this probe: whether Claude Code reads Ghostty's `send key` arrow keys and Enter as typed ones; the behavior in Terminal, iTerm2, Ghostty and other apps outside tmux; user keybindings that move these actions; and the default keys of `chat:attentionUp` and `chat:attentionDown`, which appear in the binary and may jump between agents. The probe ran once per renderer on one account.

**Revisit when:** Claude Code adds a way to select a subagent by id or name (a command argument, a keybinding action, a Remote Control call), or a way to read the draft and the shown view.

## 021: A cloud handoff launches Claude Code through a terminal and Codex with an environment id

**Date:** 2026-11-19

**Context:** The `cloud-handoff` skill ([#3666](https://github.com/Chemaclass/agnostic-ai/issues/3666)) needs the exact launch lines and failure texts of `claude --cloud` and `codex cloud exec` before any skill text is written. Measured with Claude Code 2.1.328 and Codex 0.170.0 on macOS, on one account each, from a Claude Code session's shell tool. Claude Code got one real launch from a pushed throwaway branch. The Codex account has no cloud environment, so Codex got no launch. Each cell says where its fact comes from: measured, from source (Claude Code's bundled CLI code, or Codex's released source at tag `rust-v0.170.0`, `codex-rs/cloud-tasks/src/lib.rs`), or from the vendor documentation.

**Decision:** The skill uses these lines and expects these results.

| | Claude Code | Codex |
|---|---|---|
| Launch | `claude --cloud "<prompt>"`, with a terminal on standard output | `codex cloud exec --env <id> --branch <branch> "<prompt>"`, or `-` as the prompt to read it from standard input |
| From an agent's shell tool | Measured: refused, exit 1, `Error: --cloud requires an interactive terminal.` then `Non-interactive runs (piped stdout, --init-only, --sdk-url) run locally and would silently ignore --cloud. Drop --cloud, or run in an interactive terminal.` The same shell tool launched it when it gave the command a pseudo-terminal (a Python `pty.fork` helper). `script` was not measured. | Measured for the error paths only: they ran without a terminal and reached the cloud. Source: `exec` opens no terminal interface. A launch was not measured. |
| With `-p` | Measured: refused, exit 1, `Error: --cloud cannot be combined with --print.` then ``Starting a new cloud session with --cloud is interactive only: drop --print, or drop --cloud to run locally. To message an existing cloud session instead, pass its ID: `claude -p "message" --cloud <session-id>` (find IDs at claude.ai/code).`` | Not applicable |
| Prints | Measured: three lines, then it restores the terminal's modes but keeps running: `Created cloud session: <title>`, `View: https://claude.ai/code/session_<id>?from=cli&m=0`, `Resume with: claude --teleport session_<id>`. It did not exit within 90 seconds and was killed, so a wrapper must read those lines and then kill the process. | Source: one line on success, the task URL `https://chatgpt.com/codex/tasks/<task_id>`, exit 0; non-zero when submission fails. |
| Branch | Documentation: clones the GitHub remote at the current branch, so push first. If the branch is not on GitHub, it prints a line starting `Branch <name> is not on GitHub` and starts from a pushed commit or the default branch. Measured: the session works on a new branch of its own, `claude/<slug>` (here `claude/probe-only-1f5s8s`), pushed only when it has changes. The handed-off branch is only its starting point. | Source: `--branch`, else the current branch, else the default branch, else `main`. The repository is the one the environment names, not the local remote. Nothing checks that the branch is pushed. |
| No Git remote | Documentation, not run (a run would have created a second session): no error; it uploads a bundle of the local repository, uncommitted changes to tracked files included, except `.env` and key files on macOS and Linux. Needs at least one commit, under 100 MB, git 2.31 or later. | Measured: no effect, the launch depends only on the environment (`Error: no cloud environments are available for this workspace` here, the same as from a checkout with a remote). |
| Repository not connected | Documentation, not run: a github.com repository without the Claude GitHub App also uploads a bundle instead of cloning. | Not measured: an environment names its repository when it is created. |
| Not signed in | Measured with an empty configuration folder: ``Error: Cloud sessions need a claude.ai sign-in. Run `claude auth login` (or /login in a local session), then try again.`` The terminal interface stays open. | Measured with an empty `CODEX_HOME`: exit 1, `Not signed in. Please run 'codex login' to sign in with ChatGPT, then re-run 'codex cloud'.` |
| Environment id | Not needed. `--environment <id>` only targets a self-hosted environment (`ccpool_...`). | Measured: no `--env`, exit 2, `error: the following required arguments were not provided:` then `--env <ENV_ID>`. No environments at all, exit 1, `Error: no cloud environments are available for this workspace`. Blank, exit 1, `Error: environment id must not be empty`. Source: a wrong id gives ``environment '<id>' not found; run `codex cloud` to list available environments``, and `--env` also accepts an environment's label, matched without case. |
| Title | Measured: `Probe only`, for a prompt starting `Probe only. Reply with the word ok.` Source: claude.ai generates the title and the branch name from the prompt; without a generated title the CLI uses the prompt's first 75 characters. The prompt's first line is not the title as is. | Not measured. |
| Id and bring-back | Measured: `claude --teleport session_<id>` found the session, loaded its conversation and named its branch, then reported `Couldn't check out branch 'claude/probe-only-1f5s8s'` because the session pushed nothing, and resumed without it. Source: `cse_<id>` is accepted too. The companion lists the session as `cse_<id>`, with the same suffix. | Source: `codex cloud status`, `diff` and `apply` take the id or the whole task URL; the CLI keeps the last path segment. `status` exits 1 whenever the task is not ready, so non-zero does not mean a bad id. Measured: an unknown id fails with `get_task_details failed: ... 404 Not Found; ... {"detail":"Invalid task ID"}`. |
| Prompt length | Not found in the CLI; the longest prompt the server accepts was not measured. | Source: no limit in the CLI; an empty prompt on standard input fails with `no query provided via stdin (received empty input).` The server limit was not measured. |
| Companion | Measured: `agnostic-ai serve --global --cloud-sessions` listed it 21 seconds after launch (polled every 10 seconds; absent at 10 seconds) as `app: cloud`, title `Probe only`, state `unread` once the reply came, opening `https://claude.ai/code/session_<id>`. | Not measured. |

**How a user finds the Codex environment id:** environments are created and listed at [chatgpt.com/codex/settings/environments](https://chatgpt.com/codex/settings/environments). `codex cloud` with no subcommand opens a picker that browses them, and `codex cloud list --json` shows `environment_id` and `environment_label` for each past task. No CLI command lists the environments themselves, and none creates one.

**Why:** The skill runs inside an agent's shell tool, which has no terminal, so Claude Code needs a pseudo-terminal wrapper with a time limit, or the user runs the printed line in their own terminal. Both tools start from what the remote holds on the named branch, so the skill pushes first, and with no remote it stops: Claude Code would upload a bundle instead, but then the work exists only on this computer and in the cloud session. The cloud session commits to its own `claude/<slug>` branch (Claude Code), so the skill tells the user that the cloud work arrives on a new branch, brought back with `claude --teleport <id>` or `codex cloud apply <id>`, and that the handed-off branch stays theirs. The title is generated by the cloud, so the skill cannot rely on its first line becoming the row title.

**Accepted limits:** One Claude Code launch on one account, made through a pseudo-terminal from an agent's shell; no launch was typed in a plain user terminal, since an agent has no terminal of its own and the skill will run in the same pseudo-terminal setup that was measured. The exit code after a successful Claude Code launch is unknown, since the process stayed open after printing. The Claude Code no-remote and not-connected cases come from its documentation, since running them would create more sessions. Neither tool's server prompt limit was measured, since finding it means sending ever longer prompts, each one a new cloud session. The Codex launch, its title, its branch handling on the server and its companion row were not measured: the account has no cloud environment, and creating one was out of scope. The Claude Code session was left on claude.ai: the CLI has no command to stop or archive it. Cloud launches for other tools were not checked.

## 022: In Claude Code a permission hook's allow skips ask rules, and the prompt shows while the hook runs

**Date:** 2026-11-19

**Context:** Approval modes ([#3700](https://github.com/Chemaclass/agnostic-ai/issues/3700)) answer a permission prompt from a `PermissionRequest` hook. The plan rests on vendor docs; this probe runs the hook in real sessions. Measured on Claude Code 2.1.328 with Haiku and Codex 0.170.0 with its default model, on macOS, driven by script in tmux 200 columns wide. Each tool ran in a scratch project with one hand-written hook, a shell script that logged its input and answered by a mode read from a file. Claude Code read only project settings (`--setting-sources project,local`, manual mode, fullscreen renderer: `CLAUDE_CODE_NO_FLICKER=1` came from the shell environment), with the hook in the project's `.claude/settings.json` and `timeout: 5`. Codex used a temporary `CODEX_HOME` holding the hook in its `hooks.json` (the user-level location) with `timeout: 5`, and `approval_policy = "on-request"` and `sandbox_mode = "read-only"`, so every write asked. Claude Code ran as one session; Codex ran as four, one per configuration. Triggers were `touch` and `echo` in the scratch project. Times below start when the message was sent, with the screen polled every 0.25 s unless a row says otherwise.

**Decision:** Slice 3 keeps the reply `{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow"}}}` for both tools and keeps "print nothing, exit 0" as the way to leave the prompt. Three findings change the plan. First, in Claude Code an allow from the hook skips the user's `ask` rules, so the hook must also ask when the request matches one. That conflicts with slice 2's rule that the gate is a pure function with no I/O: slice 3's `hook approve` must read the `ask` lists from the settings files Claude Code reads (user, project, local, managed) and hand them to the gate as extra always-ask patterns. Rules passed with `--settings` on the command line cannot be read; the docs must say so. Second, a hook `allow` does not stop Claude Code from recording a wait: it draws its prompt and writes `"status": "waiting"`, `"waitingFor": "permission prompt"` to `~/.claude/sessions/<pid>.json`, the file the companion reads, for as long as the hook runs. Slice 4 must not count a Claude Code wait as "Needs permission" while the approvals hook may still answer it, for example by ignoring a `permission prompt` wait younger than the hook timeout when the mode is not `ask`, or by matching it against the decision log. Third, a hook `deny` blocks the call in both tools and shows no prompt, unlike the plan's rule that a denied request falls back to the tool's prompt; the planned hook never denies, so only the plan text changes.

**Findings:**

| Question | Claude Code | Codex |
|---|---|---|
| Hook replies `allow` | No prompt. The call ran and the transcript line read `Allowed by PermissionRequest hook`. | No prompt. The call ran. |
| Hook prints nothing, exit 0 | The prompt shows. | The prompt shows. |
| Hook exits 2 with text on standard error | The prompt shows. | The call is blocked: `Blocked by hook`, then the error text, and the agent is told it was rejected. Same as a `deny` reply. |
| Hook prints text that is not JSON | The prompt shows. | The prompt shows. |
| Hook replies `deny` with a message | Blocked: `Denied by PermissionRequest hook`, and the agent reads the message. | Blocked: `Blocked by hook`, and the agent reads the message. |
| Hook runs past its timeout (5 s, script sleeps 30 s) | The prompt was already on screen 1.1 s after the request, while the hook ran. The hook was cancelled at 5 s; the prompt stayed. | Nothing shows until the timeout. The prompt appeared 7.7 s after the request, under `Hook failed`, `hook timed out after 5s`. |
| Session status file the companion reads (`~/.claude/sessions/<pid>.json`, polled every 10 ms) | With a fast `allow` (four runs), `waiting` / `permission prompt` for 20 to 220 ms, then back to `busy`. With an `allow` after 3 s, `waiting` for 3.07 s. With the hook silent, `waiting` until answered. | Not applicable: the companion reads a Codex local wait from a rollout that has not changed for 5 minutes (`codexQuietSpan`, `internal/conversations/codex.go`), so a hook decision that takes seconds never shows as a wait. Not measured. |
| When the prompt is drawn | While the hook runs. A hook that answered `allow` after 3 s left the prompt on screen for 3 s, then it closed and the call ran. A hook that answered in 36 ms still showed the prompt for one 40 ms screen capture. | After the hook ends. A hook `allow` showed no prompt in 400 captures over 16 s. |
| A user `ask` rule matches, then the hook replies `allow` | The call ran with no prompt (one project rule, `Bash(echo askrule*)`, in manual and accept-edits mode). With the hook silent, the prompt named the rule: `Rule Bash(echo askrule*) from shared project settings says to ask.` An `echo` that matched no rule ran with no hook call. | Not tried: Codex `ask` rules were not set up. |
| A user `deny` rule matches | The hook was not called; the call was refused by the rule. | Not tried. |
| One approval covers one call | Yes. After an `allow`, the next `touch` reached the hook again. | Yes, same check. |
| `permission_mode` in the input | `default` in manual mode, `acceptEdits`, `plan`, `auto`. In auto mode the hook ran for an `ask` rule, whose prompt said `overrides auto mode`. | `default` in every run, with and without the reviewer. |
| Other prompts that reach the hook | `AskUserQuestion` and `ExitPlanMode` (plan mode), each with its own `tool_name`. | Not seen. |
| Input fields | `session_id`, `transcript_path`, `cwd`, `scratchpad_dir`, `prompt_id`, `permission_mode`, `effort`, `hook_event_name`, `tool_name`, `tool_input`, and `permission_suggestions` on some prompts. `Bash` input: `command`, `description`. `Write` input: `file_path`, `content`. | `session_id`, `turn_id`, `transcript_path`, `cwd`, `hook_event_name`, `model`, `permission_mode`, `tool_name`, `tool_input`. `Bash` input: `command`, `description` (the approval reason). An edit arrives as `tool_name: "apply_patch"` with the patch text in `command`. |
| Hook trust | No step. A project hook ran once the folder was trusted. | A new or changed hook stops startup with `Hooks need review`. "Continue without trusting" left the hook unrun and the prompt showed. Trust is saved in `CODEX_HOME/config.toml` under `[hooks.state."<hooks.json path>:permission_request:0:0"]` as `trusted_hash`. |

Codex with `approvals_reviewer = "auto_review"` (set with `-c`):

- The hook runs first, right after the tool call and before the reviewer.
- Hook silent: the reviewer decided, with no prompt. The command tool's own `Wall time` in the rollout, which covers approval and run, read 3.2 s.
- Hook `allow`: the same `Wall time` read 0.1 s. No sign of a reviewer call, so the hook's allow wins and the reviewer is skipped.
- Hook `deny`: the call was blocked with the hook's message.
- The input does not say which reviewer is active: `permission_mode` stayed `default`. The hook has to read the policy on its own.
- A project's `.codex/config.toml` with `approvals_reviewer = "auto_review"` was honored in a trusted project: the thread settings in the rollout recorded `auto_review` and a write ran with no prompt and a silent hook.

**Why:** Claude Code's precedence rule for `ask` rules is documented for `PreToolUse` only, and the run shows it does not hold for `PermissionRequest`. A user who wrote `ask` for a command in their settings would lose that prompt under `yolo` unless the hook reads those rules. The input carries no field that says a rule caused the prompt: in this run the `ask` rule prompts came without `permission_suggestions` and the others had them, but that is one sample and not documented, so the gate must read the rules from the settings files. Because Claude Code draws the prompt while the hook runs, a key the person presses in that window may answer it (not tried), and a slow hook is a visible flash; the hook must stay fast. Codex treats exit 2 as a deny, which matches the plan's rule that the hook never exits 2. Codex delays its prompt by the whole hook time on a timeout, so the planned `timeout: 10` costs a Codex user up to 10 seconds per prompt when the hook hangs. Codex lets a hook `allow` skip the reviewer, so in `review` mode the hook must print nothing, as planned.

**Accepted limits:** One run per case on one account, macOS only. The `ask` rule finding rests on one project-level rule on `Bash`; no user, local or managed rule and no user-level Claude Code hook were tried. Not measured: the classic Claude Code renderer (`CLAUDE_CODE_NO_FLICKER=0`), the `bypassPermissions` and `dontAsk` modes, a Claude Code auto mode prompt caused by its classifier rather than a rule (the probe ran only harmless commands, which the classifier allowed), Codex `ask` and `deny` rules (`execpolicy` or `prefix_rule`), Codex in its other sandbox modes and collaboration modes and the `permission_mode` values there, a hook file in a Codex project `.codex/hooks.json`, MCP tool calls, Claude Code sandboxed network requests, Windows and Linux, and the start time of `agnostic-ai hook approve`, which does not exist yet. The script hook ran in 30 to 260 ms. Whether a running companion scan lands in the short wait was not measured; the status file shows it can.

**Revisit when:** Claude Code documents how `PermissionRequest` hooks rank against `ask` rules, or adds a field that names the rule behind a prompt; or Codex adds the active reviewer to the hook input.

## 023: The macOS companion is signed with self-signed certificates

**Date:** 2026-11-21

**Context:** Ad hoc signing made the app's designated requirement its code hash, so each update lost the Accessibility grant the companion needs to paste ([#3601](https://github.com/Chemaclass/agnostic-ai/issues/3601)). Apple ties privacy grants to the designated requirement, and the signer in it prevents impersonation ([TN3127](https://developer.apple.com/documentation/technotes/tn3127-inside-code-signing-requirements/)). The project has no Apple Developer ID.

**Decision:** Sign with self-signed certificates and pin each one in the requirement: a certificate made on each Mac for `agnostic-ai companion install`, and one project certificate, kept in the `COMPANION_SIGNING_P12` secret, for the release disk image. [macOS companion signing](companion-signing.md) has the details and the manual steps.

**Options not taken:**

- **Apple Developer ID:** needs a paid Apple account. It would also allow notarization, but the maintainer chose not to have one.
- **Requirement on the bundle identifier alone:** keeps the grant, but any program that copies the identifier meets the rule. A local `codesign` check accepted two different programs with it.
- **One certificate for both paths:** the terminal installer signs on the user's Mac, so it cannot hold the project key.
- **Passwords on the keychain or the `.p12`:** the installer must sign without a prompt, so a per-Mac password would sit on the same Mac as the key. In CI it would be a second secret beside the first. The `.p12` has the fixed password `agnostic-ai` only because macOS cannot import one without a password.

**Accepted limits:** A program running as the same user can read the per-Mac key and sign a copy that macOS accepts. Switching between the disk image and the terminal installer resets the grant once. A new project certificate resets it for every disk image user. Gatekeeper still reports an unidentified developer, as it did with ad hoc signing.

**Revisit when:** the project gets an Apple Developer ID, or Gatekeeper starts treating untrusted self-signed apps differently from ad hoc ones.

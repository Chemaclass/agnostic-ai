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

**Decision:** After the scan reads local and cloud rows, a live local session whose `sessions/<pid>.json` holds `bridgeSessionId` `session_<id>` takes in the cloud row `cse_<id>`. The cloud row leaves the list; the local row keeps its state and route and gains `cloud: {id, link}`. The companion marks it **Shared** and offers **Open on claude.ai**, which sends the cloud id to the open endpoint, so the server still opens only links that match the cloud session pattern. The row shows under Local and Cloud. A cloud id that two live sessions claim, a malformed bridge id, an ended process or a cloud list without that id changes nothing. A seen mark on the cloud id counts for the shared row; its completed and hidden marks and its name stay with the cloud id. Seen, completed, a completion taken back and an open on the shared row are written to both ids in one write, so the cloud row that returns when the local session ends or Remote Control is turned off is not unread again. A hide and a name stay on the local id. Only the open request takes the cloud id of a shared row; a mark or rename with it is refused like any id the list does not show.

**Why:** The ids are the only link the two lists share; a title, folder or time can match two different sessions. The session file is already read for the live check and the cloud list is already in memory, so the match costs one map lookup per row. A seen mark on the duplicate meant the person saw the conversation, so ignoring it would show it unread again. A hide or completion on the duplicate was often made to get rid of the second row, so it must not hide the local one.

**Accepted limits:** The relation between the two ids is undocumented and was measured on one account on 2026-11-12. If it changes, both rows show again, as before this change. Codex and Cursor cloud sessions are not matched.

## 017: Ghostty types sent text, Terminal pastes it

**Date:** 2026-11-17

**Context:** The companion sends copied text into a chat's input without submitting it ([#3517](https://github.com/Chemaclass/agnostic-ai/issues/3517)). Each terminal app offers a different way in.

**Decision:** Ghostty gets the text through `input text ... to t`. Terminal gets a Cmd+V from System Events, sent only once its tab is selected in the front window. Plain text is pasted only when the clipboard already holds it exactly, so the person's clipboard never changes; only Markdown is put there first. The server types only into a live session that holds its terminal in front, checked right before typing, and removes control characters and bracketed paste markers first.

**Why:** Ghostty's `input text` is a paste: measured on Ghostty 1.4.0, a program that asks for bracketed paste gets every line inside one paste, and the server adds no markers of its own, which would wrap the text twice. Terminal has no such command: `do script` always presses Enter, which would submit the text. A Cmd+V goes to whatever app is in front, so the script checks the front window first and refuses when Terminal did not come forward.

**Accepted limits:** Pasting into Terminal needs macOS Accessibility access. A program that does not ask for bracketed paste reads each newline as Enter in both apps; Claude Code and Codex ask for it. cursor-agent is never typed into: it was not checked, and it does not say when it waits for an answer. macOS may refuse to bring Terminal forward, and the send then says to paste by hand.

## 018: A terminal session is open only while a process holds it

**Date:** 2026-11-17

**Context:** Needs you, Working and Active should list only sessions the person can still use ([#3527](https://github.com/Chemaclass/agnostic-ai/issues/3527)). A terminal session that ended with an unread reply stayed under Needs you. Desktop, chat, Cowork and cloud conversations went under Archived after a quiet day, though they stay open in their app.

**Decision:** With a full process read, a terminal session (`app` `cli`: Claude Code, Codex and `cursor-agent`) is `blocked`, `working`, `unread` or `active` only while a process holds it. Once none holds it, it is `dormant`, or `completed` when the person archived it, and `unread: true` marks a reply not seen. It leaves `needsYou`, and a delete is refused until the reply is read. A scan that reads a closed session with an unseen reply saves that write's time as `closedWrittenAt` in its marks entry, without marking it seen. Once a process holds the session again, a write up to that time needs nobody; a later write is `unread` or `blocked` again. The resumed row keeps `unread: true`, `active` and not counted, until the person reads it, so one rule holds whether it runs or closes again. A failed read ignores `closedWrittenAt` and goes by the seen mark. The first page always holds a row with `unread`. A Claude Code session that began in the desktop app or an editor follows that app's rule once no terminal holds it. An unreadable `sessions/<pid>.json` blocks closure only while the process its name gives runs. Codex locks and `cursor-agent` stores are read with one `lsof +D` over their folder ([benchmark](benchmarks.md#which-process-holds-a-thread)). Desktop, chat, Cowork and cloud conversations are never archived by age: only an archive here or in their app archives them. A cloud status older than a day still stops counting as work, and the row falls back to `active` or `unread`. Editor panels (`ide`) and background jobs keep the one-day fallback. A live session that no tab shows gets its resume command with a note to end that process first. A closed terminal session's route says Resume. Archiving stays local: nothing is written to another app.

**Why:** A process that holds the session is the only signal every terminal tool gives, and the scan already reads it. An exit hook or a transcript write is not sent on a crash, a kill or a reboot. Archiving a closed unread session keeps Needs you for sessions the person can act on, and the marker keeps the reply in view. Saving the closed write, not a seen mark, keeps the reply unread on the archived row and still lets a later reply count after a resume. Quitting a desktop app closes none of its chats, so their age said nothing. An editor's selected tab and an attach terminal are no reliable signal for editor panels and background jobs. Starting a second copy of a running session would compete with it, so the server only offers the command.

**Accepted limits:** A process that ends shows within a minute, the bound the scan already keeps for a list that did not change. A failed or impossible process read (Windows, no `ps` or `lsof`, an unreadable session file) keeps the old fallback and never shows a session closed. Process ids are read on this computer only: a store shared with a container, WSL or another account can name a process that runs elsewhere, and the server does not detect another process namespace. A session closed and resumed between two reads shows its old reply as new, since the scan saw no closure; handling that needs a resume time each tool does not give. A Remote Control session that ends lists its cloud twin again, as decision 016 says. That row follows the cloud rule, so an unseen reply there still counts under Needs you, and marks no longer pass between the two rows; a follow-up issue tracks it. The `/clear`, in-process `/resume` and fork cases are tested on the scanner side only: they assume Claude Code points `sessions/<pid>.json` at the new id, which was not checked against Claude Code. A seen mark keeps whole seconds; every reader also writes whole seconds, so a mark in the reply's second covers it. Sessions deleted in their app are not taken out of the list and their marks are not pruned: no reader proves a complete inventory yet. The Claude app's cached sidebar cannot tell an archive from a deletion, and the ChatGPT reader has no archive flag. That work, with a proposed 30-day retention after complete successful reads, is [#3540](https://github.com/Chemaclass/agnostic-ai/issues/3540).

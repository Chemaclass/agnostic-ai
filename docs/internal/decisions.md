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

**Accepted limits:** A dormant session that wrote only bookkeeping lines after the person last looked, and before the server read the row, is deleted. The list's time counts only messages from the person or the agent, so only a message makes the row `unread`. A chat taken back out of the archive whose last write is older than a day is `dormant` again: it is still listed under Archived and can be deleted, also from a page loaded before it was taken back.

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

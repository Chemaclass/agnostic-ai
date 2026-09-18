---
name: memory-curator
description: Curate this tool's auto memory. Use when the memory index is near its load limit or the stored notes look duplicated or stale.
targets: [claude, qoder]
---

# memory-curator

Curates the auto memory store of the tool you are running in. agnostic-ai never writes there, so the tool prunes its own store, during its own session, with the user watching.

## Store

::target claude
Auto memory usually lives in `~/.claude/projects/<project>/memory/`, but the location is configurable: `autoMemoryDirectory` in any settings scope moves it, `CLAUDE_CONFIG_DIR` moves the whole config root, and `CLAUDE_CODE_PROJECT_DIR_NAME` changes the project segment. Never assume the default path. Run `/memory`, pick the auto memory folder, and curate whatever store it opens. `MEMORY.md` is the index and every other file holds one topic.

Each session loads the first 200 lines or 25KB of `MEMORY.md`, whichever comes first. Everything past that is dropped on load.
::end

::target qoder
Auto memory lives in `~/.qoder/projects/<project>/memory/`, with a user-level store in `~/.qoder/memory/`. Run `/memory` for the overview and `/memory manage` to view, edit, or delete topic files. Ask which of the two stores to curate before reading anything, and curate one per run. `MEMORY.md` is the index and every other file holds one topic.

Qoder CLI reads the first 200 lines or about 25KB of each active `MEMORY.md`. Everything past that is dropped on load.
::end

## Steps

1. Resolve the store first. Open it through the tool's own `/memory` command rather than a hard-coded path, and confirm `MEMORY.md` exists there. If auto memory is turned off, say so and stop: there is nothing to curate until it is enabled.
2. Read `MEMORY.md` and list the topic files beside it.
3. Measure the index in lines and bytes against the load limit above, and report the headroom left.
4. Group entries that state the same fact in different words. Those are merge candidates.
5. Flag stale entries: a decision later work reversed, a date that has passed, a path or command the repository no longer has, a preference the user has since contradicted. Check each claim against the repository before calling it stale.
6. Flag entries that belong somewhere else: anything derivable from the code, and anything the instructions files already say. The session already carries those.
7. Report one table: file, entry, verdict (keep, merge, delete, move), and one line of evidence.
8. Propose the edits. Show the replacement line for a merge, and name the file and entry for a deletion.
9. Stop there. Apply nothing until the user confirms which proposals to take.
10. Apply only the confirmed items, then re-measure the index and report the new size.

## Conventions

- Keep one line per entry in the index and push detail into a topic file.
- Prefer a merge over a deletion when both entries are true.
- Never drop a topic file while the index still points at it. Fix the index line in the same edit.
- Treat an entry you cannot verify as keep, and say that it is unverified.
- Touch only the memory store of the tool you are running in. Leave other tools' stores and the project's own files alone.

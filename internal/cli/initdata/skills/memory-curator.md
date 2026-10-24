---
name: memory-curator
description: Curate this tool's auto memory. Use when the memory index is near its load limit or the stored notes look duplicated or stale.
targets: [claude, qoder]
---

# memory-curator

Curates the auto memory store of the tool you are running in. The tool prunes its own store, during its own session, with the user watching.

## Store

::target claude
Auto memory usually lives in `~/.claude/projects/<project>/memory/`, but the location is configurable: `autoMemoryDirectory` in any settings scope moves it, `CLAUDE_CONFIG_DIR` moves the whole config root, and `CLAUDE_CODE_PROJECT_DIR_NAME` changes the project segment. Never assume the default path. Run `/memory`, pick the auto memory folder, and curate whatever store it opens. With the `memory` built-in on, sync points `autoMemoryDirectory` at the resolved personal store. The default is `.agnostic-ai/local/memory/`; `memory.personal: repo` selects a shared store across worktrees. Use `agnostic-ai memory path` to resolve it. `MEMORY.md` is the index and every other file holds one topic.

Each session loads the first 200 lines or 25KB of `MEMORY.md`, whichever comes first. See the [Claude memory reference](https://code.claude.com/docs/en/memory#how-it-works).
::end

::target qoder
Auto memory lives in `~/.qoder/projects/<project>/memory/`, with a user-level store in `~/.qoder/memory/`. Run `/memory` for the overview and `/memory manage` to view, edit, or delete topic files. Use the store the user named. If neither is selected and both are active, ask which to curate; handle one per run. `MEMORY.md` is the index and every other file holds one topic.

Qoder CLI reads the first 200 lines or about 25KB of each active `MEMORY.md`. See the [Qoder memory reference](https://docs.qoder.com/cli/memory#auto-memory-storage-locations).
::end

## Steps

1. Find the store first. Resolve it through the tool's `/memory` command or effective settings, and confirm `MEMORY.md` exists there. If the session cannot invoke the interactive command, inspect the settings instead of pretending to run it. If auto memory is turned off, say so and stop: there is nothing to curate until it is enabled.
2. Read `MEMORY.md` and list the topic files beside it.
3. Measure the index in lines and bytes against the load limit above, and report the space left.
4. Group entries that state the same fact in different words. Those are merge candidates.
5. Flag stale entries: a decision later work reversed, a path or command the repository no longer has, a preference the user has since contradicted. Check each claim against the repository before calling it stale.
6. Flag entries that belong somewhere else: anything you can work out from the code, and anything the instructions files already say. The session already carries those.
7. Report one table: file, entry, verdict (keep, merge, delete, move), and one line of evidence.
8. Propose the edits. Show the replacement line for a merge, and name the file and entry for a deletion.
9. Apply only the proposals the user has approved. If that approval is already explicit in the request or session, continue within its scope; otherwise ask with the concrete proposed edits.
10. Apply only the confirmed items, then re-measure the index and report the new size.

## Conventions

- Keep one line per entry in the index and push detail into a topic file.
- Prefer a merge over a deletion when both entries are true.
- Never drop a topic file while the index still points at it. Fix the index line in the same edit.
- Treat an entry you cannot verify as keep, and say that it is unverified.
- Touch only the memory store of the tool you are running in. Leave other tools' stores and the project's own files alone.

---
name: shared-memory
description: Recall or curate the project memory that every AI coding tool shares.
---

# shared-memory

Recall or curate the shared project memory when the user asks. Choose the mode from the request: "what do we know about X" recalls; "clean up memory" curates. The `shared-memory-policy` rule covers when and how to save a fact.

Project memory is `.agnostic-ai/memory/` and personal memory is `.agnostic-ai/local/memory/`, both in the current project root, including when this skill is installed globally. For root lookup, use canonical paths for the current directory, candidate ancestors, and the effective global source root (`AGNOSTIC_AI_HOME` when nonempty, otherwise `~/.agnostic-ai`). Find the nearest ancestor containing `agnostic-ai.yaml` or the legacy `agnostic.config.yaml`, but exclude the effective global source root: its home config never identifies a project. The remaining nearest configured ancestor is the project root. If none exists, use `git rev-parse --show-toplevel` for the current checkout.

Each folder holds `MEMORY.md`, an index with one `- [Title](<slug>.md): <hook>` line per fact, and one topic file per fact. A topic file has frontmatter `name`, `description`, and `metadata.type` (`user`, `feedback`, `project`, or `reference`), then the fact, a `**Why:**` line, and a `**How to apply:**` line.

## Recall

1. Read both `MEMORY.md` indexes. If neither exists, tell the user the project has no shared memory yet.
2. Pick the index lines that match the request and read their topic files.
3. Before you report a fact that names a file, flag, or command, check that it still exists. Report a stale fact as stale.

## Curate

1. Read each index and every topic file. Curate each folder on its own, and move a fact between them only when the user asks.
2. Find duplicates to merge, facts the code or rules now state, facts that name something that no longer exists, topic files missing from the index, and index lines whose file is missing. Keep the index under 100 lines.
3. Show one diff per changed file and wait for the user to confirm. Apply only what they approve.
4. Never write secrets, and never read or write any tool's own memory store.

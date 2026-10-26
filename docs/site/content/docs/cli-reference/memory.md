+++
title = "Memory commands"
description = "Locate, inspect, and repair shared project and personal memory."
weight = 58

[extra]
group = "Reference"
+++

# Memory commands

## memory

Check, list, locate, and repair the [shared memory](@/docs/memory.md) in the working directory: project memory in `.agnostic-ai/memory/`, then personal memory in `.agnostic-ai/local/memory/` (or the [shared personal folder](@/docs/memory.md#one-store-per-repository) with `memory.personal: repo`). Missing folders are skipped.

```bash
agnostic-ai memory lint     # run only the memory checks
agnostic-ai memory index    # rebuild each MEMORY.md from its fact files
agnostic-ai memory list     # print each fact's scope, type, and title
agnostic-ai memory path     # print each memory folder's absolute path
agnostic-ai memory import --from claude   # copy facts from another memory store
```

- `memory lint` reports [LINT039 to LINT044](@/docs/cli-reference/check.md#lint), the memory findings `lint` and `doctor` also report. It exits 1 on an error, or on a warning with `--strict`, and exits 0 when no memory folder exists. `--json` prints the `lint --json` format with `command` set to `memory lint`.
- `memory index` drops merge conflict markers, links to missing files, and repeated links, keeping every other line. It then adds a `- [name](file.md): description` line, from the fact's YAML fields, for each fact without one.
- `memory list` prints facts in index order, titled by the index line's link text, or the fact's `name` when no line links it. The type comes from `metadata.type`, or a top-level `type` as Claude Code's auto memory writes it.
- `memory path` prints one `scope  folder` line per memory folder, project first, as absolute paths, even for a folder not created yet. It works from any folder in the project and from a linked worktree; with `memory.personal: repo`, every worktree gets the same personal folder. Tools without a session-start hook run it, as the [`shared-memory-policy` rule](@/docs/memory.md#how-each-tool-loads-it) asks.
- `memory import` copies fact files into personal memory, which every tool reads, and adds their index lines. `--from claude` reads Claude Code's store from before sync pointed it at personal memory; `--from checkout` and `--from repo` read the other personal folder after a `memory.personal` switch; `--path` reads any folder with a `MEMORY.md` and one file per fact.

  The source stays unchanged, and so does an existing destination file. Source index lines that link no fact become a fact of their own (`claude-index-notes.md` for `--from claude`). Symlinks and files with a line that looks like a secret are skipped and named. An identical fact without an index line gets its line back, so a rerun finishes an interrupted import.

  Import refuses a personal folder holding tracked files, and first adds the checkout folder to Git's exclude list if Git would track it. It records handled facts in `.imported` there, so LINT043 stops counting facts you later delete or merge.

| Flag | Description |
|------|-------------|
| `--strict` | With `memory lint`, exit 1 on warnings too. |
| `--json` | With `memory lint`, print `{version, command, findings}`. |
| `--from` | With `memory import`, the store to copy from: `claude`, `checkout`, or `repo`. |
| `--path` | With `memory import`, a folder to copy from instead of `--from`. |
| `--only` | With `memory import`, copy only this fact file; repeat the flag for more. |
| `--dry-run` | With `memory import`, list each fact and its description and write nothing. |

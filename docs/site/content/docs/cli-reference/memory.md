+++
title = "Memory commands"
description = "Locate, inspect, and repair shared project and personal memory."
weight = 58

[extra]
group = "Reference"
+++

# Memory commands

## memory

Check, list, locate, and repair the [shared memory](@/docs/memory.md) in the working directory. Each subcommand reads project memory in `.agnostic-ai/memory/`, then personal memory in `.agnostic-ai/local/memory/`, or in the [personal folder shared across worktrees](@/docs/memory.md#one-store-per-repository) with `memory.personal: repo`. Missing memory folders are skipped.

```bash
agnostic-ai memory lint     # run only the memory checks
agnostic-ai memory index    # rebuild each MEMORY.md from its fact files
agnostic-ai memory list     # print each fact's scope, type, and title
agnostic-ai memory path     # print each memory folder's absolute path
agnostic-ai memory import --from claude   # copy facts from another memory store
```

- `memory lint` reports [LINT039 to LINT044](@/docs/cli-reference/check.md#lint), the memory findings `lint` and `doctor` also report. It exits 1 on an error, or on a warning with `--strict`. `--json` prints the same format as `lint --json`, with `command` set to `memory lint`.
- `memory index` drops merge conflict markers, links to missing files, and repeated links. Every other line stays in place, headings and notes included. It then adds a `- [name](file.md): description` line from the YAML fields at the top of each fact without one. Run it after two tools edit the index at once.
- The type comes from `metadata.type`, or from a top-level `type` as Claude Code's auto memory writes it.
- With no memory folder, `memory lint` says so and exits 0.
- `memory list` prints facts in index order. The title is the index line's link text, or the fact's `name` when no line links it.
- `memory path` prints one `scope  folder` line per memory folder, project first, as absolute paths. It works from any folder inside the project and from a linked worktree, and prints a folder that does not exist yet. With `memory.personal: repo`, every worktree of the repository gets the same personal folder. The [`shared-memory-policy` rule](@/docs/memory.md#how-each-tool-loads-it) has tools without a session-start hook run it.
- `memory import` copies fact files from another memory store into personal memory and adds their index lines. `--from claude` reads Claude Code's own store, the folder it used before sync pointed it at personal memory. `--from checkout` and `--from repo` read the other personal folder after a `memory.personal` switch. `--path` reads any folder with a `MEMORY.md` and one file per fact. Every tool reads personal memory, so run `--dry-run` first: it lists each fact with its description, and `--only <file>` copies just the ones you name. The source never changes. Lines of the source index that link no fact come along as one more fact (`claude-index-notes.md` for `--from claude`). A fact whose file name personal memory already has stays as it is, and a symlink or a file with a line that looks like a secret is skipped; the report names each. A fact already there byte for byte gets its index line back if the index lacks it, so a rerun finishes an import that stopped. Import refuses a personal folder with files Git tracks, and first adds the checkout folder to Git's exclude list when Git would track it. It lists each fact it handled in `.imported` inside the personal folder, so LINT043 stops counting a fact you delete or merge afterwards. `--dry-run` prints the plan and writes nothing.

| Flag | Description |
|------|-------------|
| `--strict` | With `memory lint`, exit 1 on warnings too. |
| `--json` | With `memory lint`, print `{version, command, findings}` on stdout. |
| `--from` | With `memory import`, the store to copy from: `claude`, `checkout`, or `repo`. |
| `--path` | With `memory import`, a folder to copy from instead of `--from`. |
| `--only` | With `memory import`, copy only this fact file; repeat the flag for more. |
| `--dry-run` | With `memory import`, print what would be copied and write nothing. |

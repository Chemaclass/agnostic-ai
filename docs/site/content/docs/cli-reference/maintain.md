+++
title = "Maintain and set up"
description = "Revert or clean generated files, share packs, and set up hooks, completion, and updates."
weight = 50

[extra]
group = "Reference"

[extra.moved]
memory = "@/docs/cli-reference/memory.md#memory"
project = "@/docs/cli-reference/project.md#project"
project-readiness = "@/docs/cli-reference/project.md#project-readiness"
upgrade = "@/docs/cli-reference/upgrades.md#upgrade"
automatic-upgrade-offers = "@/docs/cli-reference/upgrades.md#automatic-upgrade-offers"
upgrade-options = "@/docs/cli-reference/upgrades.md#upgrade-options"
migrate = "@/docs/cli-reference/upgrades.md#migrate"
+++

# Maintain and set up

## Memory {#memory .legacy-anchor}

## Project {#project .legacy-anchor}

## Project readiness {#project-readiness .legacy-anchor}

## Upgrade {#upgrade .legacy-anchor}

## Automatic upgrade offers {#automatic-upgrade-offers .legacy-anchor}

## Upgrade options {#upgrade-options .legacy-anchor}

## Migrate {#migrate .legacy-anchor}


This page covers handoffs, generated-file recovery, packs, hook helpers, git hooks, completion, and the language server. Other maintenance commands have their own pages: [project CLI and readiness](@/docs/cli-reference/project.md), [upgrade and migrate](@/docs/cli-reference/upgrades.md), and [memory](@/docs/cli-reference/memory.md).

## handoff list

List saved [task handoffs](@/docs/handoff.md) across the repository's worktrees:

```bash
agnostic-ai handoff list
```

Each branch gets one row: live worktree (or `removed`), saved HEAD and date, working changes (`clean`, `dirty`, or `unknown`), task status, PR (`-` when missing), and handoff file. The newest manual note wins over automatic snapshots. It reads only note headers, from the stores the [handoff skill](@/docs/handoff.md#limits-and-overrides) uses.

It suggests removal only when local Git ancestry proves the branch merged into the main checkout's branch and no working changes are pending, so squash merges need your review. It never fetches or deletes files.

## revert

Undo a `sync --backup`: for every generated file and tool instruction file (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, `CONVENTIONS.md`, `.agnostic-ai/AGNOSTIC_AI.md`), restore `<path>.bak` and remove it. It also restores a nested `CLAUDE.md` that sync deleted. Paths under [`sync.unmanaged`](@/docs/configuration.md#syncunmanaged) are never touched.

| Flag | Description |
|------|-------------|
| `-t`, `--only`, `--except` | Select targets, as in [`sync`](@/docs/cli-reference/sync.md#sync). |
| `--dry-run` | Show what it would do and change nothing. |
| `--force` | Also delete generated files that have no `.bak`, including generated tool instruction files and your own files at the same paths. |
| `--json` | Same format as `sync --json`. Actions: `"restore"`, `"remove"`, `"preserve"` (no `.bak`, no `--force`), `"skip"` (already gone). |

## cleanup

Remove the `<path>.bak` backups that `sync --backup` wrote, and no other `.bak` files.

```bash
agnostic-ai cleanup
agnostic-ai cleanup --dry-run   # preview deletions
```

## packs

Add, update, and remove collections of shared specs. See the [packs](@/docs/packs.md) guide.

```bash
agnostic-ai packs add github.com/obra/superpowers@v6.4.2
agnostic-ai packs add ./path/to/pack
agnostic-ai packs list
agnostic-ai packs update [name]
agnostic-ai packs remove superpowers
```

## hook paths

Run inside an edit hook to print the files the edit leaves on disk, one per line. [Edited paths](@/docs/spec-format/hook-reference.md#edited-paths) explains the input, tool detection, and exit codes.

```bash
files=$(agnostic-ai hook paths) || exit 1
printf '%s\n' "$files" | grep '\.go$' | while IFS= read -r f; do gofmt -w "$f"; done
```

Capture the output first, as above: piped straight into the loop, a failure is lost and the hook exits 0.

| Flag | Description |
|------|-------------|
| `-t`, `--target <name>` | Tool that sent the event. Defaults to `AGNOSTIC_AI_TARGET`. |
| `--action` | Print each change as `<action><TAB><path>` (`add`, `update`, `delete`, `move`), including deleted files and move sources. |
| `--json` | Print each change as a JSON array of `{action, path, from}`. Not with `--action`. |

## hook worktree-remove

Remove one clean Git worktree from a Claude Code [`WorktreeRemove`](https://code.claude.com/docs/en/hooks#worktreeremove) event on stdin. Pair it with your `WorktreeCreate` hook; it does not change cleanup for worktrees the host creates without one.

```bash
agnostic-ai hook worktree-remove --repo "/path/to/repository" --allowed-root "/path/to/disposable worktrees"
```

| Flag | Description |
|------|-------------|
| `--repo <path>` | Required. A surviving checkout of the repository, outside the disposable worktrees. |
| `--allowed-root <path>` | Required. Only descendants of this directory may be removed. |

After checking Git's worktree registration and repository metadata, it runs `git worktree remove` without force, keeping the branch. It refuses the main checkout, the allowed root itself, other repositories, unregistered directories, traversal, symlinks below the allowed root, locked worktrees, and worktrees with changed, untracked, or ignored files (ignored dependencies included; remove them first). It also refuses indexes with `assume-unchanged` or `skip-worktree` entries, sparse checkouts included, because those can hide changed contents.

| Result | Output | Exit |
|--------|--------|------|
| Removed | `removed: <path>` | 0 |
| Already gone below the allowed root (also on a repeat call); nothing deleted or pruned | `already absent: <path>` | 0 |
| Unsafe input, event data over 1 MiB, or a failed Git operation | error | 1 |

Claude Code runs this event instead of its own cleanup, while the directory still exists. A non-zero exit keeps the directory, with no Git fallback.

## hook guard

Run inside an `after-edit` or `stop` hook, as the [spec guard hooks](@/docs/spec-format/hook-reference.md#spec-guard) do. It exits 2 with a short report for the agent on a problem, else 0 with no output.

```bash
command -v agnostic-ai >/dev/null 2>&1 || exit 0; agnostic-ai hook guard after-edit
command -v agnostic-ai >/dev/null 2>&1 || exit 0; agnostic-ai hook guard stop
```

- `after-edit` reads the event data on stdin, like [`hook paths`](#hook-paths), and reports lint errors in the specs the edit touched.
- `stop` reports specs changed without a sync. It passes when the agent already continued from a stop hook, so the notice cannot loop.
- It uses the nearest `agnostic-ai.yaml` at or above the current directory, and exits 0 when it cannot read the event data or config.

| Flag | Description |
|------|-------------|
| `-t`, `--target <name>` | Tool that sent an `after-edit` event. Defaults to `AGNOSTIC_AI_TARGET`. |

## hook memory

Print the `MEMORY.md` fact lists, personal first, for a `session-start` hook to add to the model's context. The [`memory` built-in](@/docs/memory.md#load-at-session-start) sets this up, and also runs it on Claude Code when a subagent starts.

```bash
agnostic-ai hook memory --target codex
```

| Tool | Output |
|------|--------|
| Codex, Factory | Plain text |
| Cursor | `{"additional_context": ...}` |
| Gemini CLI, Qoder | `{"hookSpecificOutput": {"hookEventName": "SessionStart", "additionalContext": ...}}` |
| Copilot | That object for VS Code, plus a top-level `additionalContext` for Copilot CLI |
| Claude Code | `{"hookSpecificOutput": {"hookEventName": "SubagentStart", "additionalContext": ...}}` with personal memory only, since `CLAUDE.md` gives subagents the project list. Nothing when `CLAUDE.md` imports the personal list too. |

- Output stays under 6,000 bytes, cut at a line end with a note naming the full index. Hidden characters are dropped first (see [session start](@/docs/memory.md#load-at-session-start)), except a zero-width joiner or non-joiner between two non-ASCII characters, as in an emoji or a Persian word.
- With no project, it prints nothing. With `memory.personal: repo`, it names the personal folder even before an index exists; in checkout mode, no index means no output. It finds the project as the `shared-memory` skill does.

| Flag | Description |
|------|-------------|
| `-t`, `--target <name>` | Tool that runs the hook. Defaults to `AGNOSTIC_AI_TARGET`. |

## hook worktree-refresh

Rerun Claude Code's configured environment setup in one registered linked worktree:

```bash
agnostic-ai hook worktree-refresh --repo /path/to/main --worktree "/path/to/linked worktree"
```

`--repo` is a checkout of the owning repository and `--worktree` the linked checkout. The helper checks that the worktree is registered to that repository and refuses the main checkout. Run `agnostic-ai sync` in that worktree first when setup changes: it resets the completion marker only when the generated setup script matches current source, including output-path overrides such as `outputs.claude.dir`. See [refresh an existing worktree](@/docs/spec-format/environments.md#refresh-an-existing-worktree) for how markers work.

- Setup runs from the worktree with a 600-second limit; output and failures pass through. Cancellation stops the setup processes it owns; a successful setup can leave background services running. Other worktrees keep their markers.
- On Windows it needs Git Bash, from `CLAUDE_CODE_GIT_BASH_PATH` or the Git for Windows install, never a `bash` launcher from PATH.
- It installs dependencies only if your setup command does. It does not check application health or change other tools' setup, and normal Claude setup hooks still run once per worktree.

## hook run

Test a synced hook spec without starting a session. See [test a hook](@/docs/cli-reference/hook-diagnostics.md#hook-run) for the sample events, results, and exit codes.

```bash
agnostic-ai hook run protect-files --edit .github/workflows/tests.yml --expect block
```

| Flag | Description |
|------|-------------|
| `-t`, `--target <names>` | Test only these tools. Defaults to every configured tool the hook applies to. |
| `--edit <path>` | Send a `PreToolUse` or `PostToolUse` event that edits this file. |
| `--bash <command>` | Send a `PreToolUse` or `PostToolUse` event that runs this shell command. |
| `--prompt <text>` | Prompt text for `UserPromptSubmit`. |
| `--runtime vscode` | With `--target cline`, model Cline VS Code TaskStart on Unix instead of the SDK. Windows launcher selection is not modeled. |
| `--payload <file>` | Send this JSON file to every tool, for events the flags above do not cover. |
| `--expect allow\|block` | Fail unless every tool gives this result. |
| `--include-assumed` | Count [assumed results](@/docs/cli-reference/hook-diagnostics.md#assumed-results) toward `--expect` and the comparison between tools. |
| `--format text\|json` | `json` prints one report with a `targets` array; see the [JSON fields](@/docs/cli-reference/hook-diagnostics.md#json-output). Defaults to `text`. |

## install-hook

Install git hooks; see the [git hooks](@/docs/git-hooks.md) guide. By default it installs a pre-commit hook that runs `project --check --against index`, so a commit fails if regenerated files are not staged. Project hooks prefer the installed local binary over `PATH`.

```bash
agnostic-ai install-hook            # writes .git/hooks/pre-commit (local)
agnostic-ai install-hook --shared   # writes .githooks/ and sets core.hooksPath
agnostic-ai install-hook --global   # checks global specs before committing

agnostic-ai install-hook --post-checkout            # writes .git/hooks/post-checkout and post-merge
agnostic-ai install-hook --post-checkout --shared   # writes both hooks in .githooks/
```

An existing hook keeps its content, and the checks go at its end. A hook that would stop before them (its first line does not select `sh` or `bash`, or it has an `exec` or an unindented `exit`) is left alone, and the command prints the lines to add by hand.

- `--shared` writes `.githooks/<hook>` at the root of the main working tree, even from a linked worktree. It stops if `core.hooksPath` already points elsewhere.
- `--global` is for the global specs folder, which must be the root of its own git repository. The hook runs `lint --global --strict`, `validate --global`, and `sync --global --check` (skipped in a linked worktree). Not with `--shared` or `--post-checkout`; it stops when `core.hooksPath` points elsewhere or the hook already runs the project `sync --check`.
- `--post-checkout` installs `post-checkout` and `post-merge`, which run `agnostic-ai project` from the worktree root after a branch or worktree checkout (not a single file) or a merge, pull included. See [regenerate on checkout](@/docs/git-hooks.md#regenerate-on-checkout).

## completion

Generate a shell completion script.

```bash
agnostic-ai completion bash > ~/.local/share/bash-completion/completions/agnostic-ai  # or /etc/bash_completion.d/
agnostic-ai completion zsh > "${fpath[1]}/_agnostic-ai"
agnostic-ai completion fish > ~/.config/fish/completions/agnostic-ai.fish
agnostic-ai completion powershell | Out-String | Invoke-Expression
```

Restart your shell or `source` the file. Completing `--target` reads `agnostic-ai.yaml` in the current directory, or offers every target if there is none.

## lsp

Start the language server on stdin and stdout. The [VS Code extension](https://github.com/Chemaclass/agnostic-ai/tree/main/editors/vscode) starts it for a configured workspace, using `agnostic-ai.binaryPath`; other editors can launch `agnostic-ai lsp` for Markdown and YAML files.

- It lints the project's configured sources, custom and absolute directories included, when you open or save a file. Unsaved edits are not linted.
- Config and spec load failures appear on the source file, with the parser location when known, or as a workspace error.
- A successful check clears resolved diagnostics from every affected file, deleted sources included. After a failed load, earlier diagnostics stay until you fix the problem and save a project file.

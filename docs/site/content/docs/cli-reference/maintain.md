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


Handoffs, generated-file recovery, packs, hook helpers, git hooks, completion, and the language server. See also [project CLI and readiness](@/docs/cli-reference/project.md), [upgrade and migrate](@/docs/cli-reference/upgrades.md), and [memory](@/docs/cli-reference/memory.md).

## handoff list

List saved [task handoffs](@/docs/handoff.md) across the repository's worktrees. Each branch gets one row.

```bash
agnostic-ai handoff list
```

| Column | Value |
|--------|-------|
| Worktree | Live, or `removed` |
| Saved HEAD | Commit and date |
| Working changes | `clean`, `dirty`, or `unknown` |
| Task status | From the handoff |
| PR | `-` when missing |
| Handoff file | Path |

{% <details summary="Details: which note wins, and removal hints"> %}
- The newest manual note wins over automatic snapshots.
- It reads only note headers, from the stores the [handoff skill](@/docs/handoff.md#limits-and-overrides) uses.
- It suggests removal only when local Git ancestry proves the branch merged and no working changes are pending. Squash merges need your review.
- It never fetches or deletes files.
{% </details> %}

## revert

Undo a `sync --backup`. For each generated file and tool instruction file (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, `CONVENTIONS.md`), it restores `<path>.bak` and removes the backup. It prints how many files it restored, removed, and kept for each target.

| Flag | Description |
|------|-------------|
| `-t`, `--only`, `--except` | Select targets, as in [`sync`](@/docs/cli-reference/sync.md#sync). |
| `--dry-run` | Show what it would do and change nothing. |
| `--force` | Also delete generated files that have no `.bak`, including generated tool instruction files and your own files at the same paths. |
| `--json` | Same format as `sync --json`. Actions: `"restore"`, `"remove"`, `"preserve"` (no `.bak`, no `--force`), `"keep"` (in the source tree), `"skip"` (already gone). |

{% <details summary="Details: what revert restores and skips"> %}
- Restored files keep their permissions, never wider than the file's.
- A nested `CLAUDE.md` that sync deleted comes back too.
- An unfiltered `revert` also restores intact recorded backups of removed outputs, including those from dropped targets.
- Paths under [`sync.unmanaged`](@/docs/sync-settings.md#syncunmanaged) are never touched.
- Files in `.agnostic-ai/` and in the spec folders `sources` sets, your note `AGNOSTIC_AI.md` included, are what you write, so `revert` never restores or deletes them, even with `--force` and even when an `outputs` path or a link points there. A spec folder set to the project root does not count.
- A shared `AGENTS.md` is handled once.
- `revert` takes the project lock, so it stops while another command writes. `--dry-run` does not.
{% </details> %}

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
agnostic-ai packs update [name]            # at the commit in agnostic.packs.lock
agnostic-ai packs update --latest [name]   # move to the newest commit of the ref
agnostic-ai packs remove superpowers
```

## hook paths

Run inside an edit hook to print the files the edit leaves on disk, one per line. [Edited paths](@/docs/spec-format/hook-reference.md#edited-paths) explains the input, tool detection, and exit codes.

```bash
files=$(agnostic-ai hook paths) || exit 1
printf '%s\n' "$files" | grep '\.go$' | while IFS= read -r f; do gofmt -w "$f"; done
```

Capture the output first, as above. Piping it straight into the loop hides failures and lets the hook exit 0.

| Flag | Description |
|------|-------------|
| `-t`, `--target <name>` | Tool that sent the event. Defaults to `AGNOSTIC_AI_TARGET`. |
| `--action` | Print each change as `<action><TAB><path>` (`add`, `update`, `delete`, `move`), including deleted files and move sources. |
| `--json` | Print each change as a JSON array of `{action, path, from}`. Not with `--action`. |

## hook worktree-remove

Remove one clean Git worktree from a Claude Code [`WorktreeRemove`](https://code.claude.com/docs/en/hooks#worktreeremove) event on stdin. Pair it with your `WorktreeCreate` hook. It does not change cleanup for worktrees the host creates without one.

```bash
agnostic-ai hook worktree-remove --repo "/path/to/repository" --allowed-root "/path/to/disposable worktrees"
```

| Flag | Description |
|------|-------------|
| `--repo <path>` | Required. A surviving checkout of the repository, outside the disposable worktrees. |
| `--allowed-root <path>` | Required. Only descendants of this directory may be removed. |

{% <details summary="Details: what worktree-remove refuses, and its results"> %}
- After checking Git's worktree registration and repository metadata, it runs `git worktree remove` without force, keeping the branch.
- It refuses the main checkout, the allowed root itself, other repositories, unregistered directories, traversal, symlinks below the allowed root, and locked worktrees.
- It refuses worktrees with changed, untracked, or ignored files. Ignored dependencies count; remove them first.
- It refuses indexes with `assume-unchanged` or `skip-worktree` entries, sparse checkouts included, because those can hide changed contents.

| Result | Output | Exit |
|--------|--------|------|
| Removed | `removed: <path>` | 0 |
| Already gone below the allowed root (also on a repeat call); nothing deleted or pruned | `already absent: <path>` | 0 |
| Unsafe input, event data over 1 MiB, or a failed Git operation | error | 1 |

Claude Code runs this event instead of its own cleanup, while the directory still exists. A non-zero exit keeps the directory, with no Git fallback.
{% </details> %}

## hook guard

Run inside an `after-edit` or `stop` hook, as the [spec guard hooks](@/docs/spec-format/hook-reference.md#spec-guard) do. It exits 2 with a short report for the agent on a problem, else 0 with no output.

```bash
command -v agnostic-ai >/dev/null 2>&1 || exit 0; agnostic-ai hook guard after-edit
command -v agnostic-ai >/dev/null 2>&1 || exit 0; agnostic-ai hook guard stop
```

| Event | What it reports |
|-------|-----------------|
| `after-edit` | Lint errors in the specs the edit touched. Reads event data on stdin, like [`hook paths`](#hook-paths). |
| `stop` | Specs changed without a sync. It passes when the agent already continued from a stop hook, so the notice cannot loop. |

It uses the nearest `agnostic-ai.yaml` at or above the current directory. It exits 0 when it cannot read the event data or config.

| Flag | Description |
|------|-------------|
| `-t`, `--target <name>` | Tool that sent an `after-edit` event. Defaults to `AGNOSTIC_AI_TARGET`. |

## hook memory

Print the `MEMORY.md` fact lists, personal first, for a `session-start` hook to add to the model's context. The [`memory` built-in](@/docs/memory.md#load-at-session-start) sets this up, and also runs it on Claude Code when a subagent starts.

```bash
agnostic-ai hook memory --target codex
```

| Flag | Description |
|------|-------------|
| `-t`, `--target <name>` | Tool that runs the hook. Defaults to `AGNOSTIC_AI_TARGET`. |

{% <details summary="Details: output format per tool, size limit, and project lookup"> %}
| Tool | Output |
|------|--------|
| Codex, Factory | Plain text |
| Cursor | `{"additional_context": ...}` |
| Gemini CLI, Qoder | `{"hookSpecificOutput": {"hookEventName": "SessionStart", "additionalContext": ...}}` |
| Copilot | That object for VS Code, plus a top-level `additionalContext` for Copilot CLI |
| Claude Code | `{"hookSpecificOutput": {"hookEventName": "SubagentStart", "additionalContext": ...}}` with personal memory only, since `CLAUDE.md` gives subagents the project list. Nothing when `CLAUDE.md` imports the personal list too. |

- Output stays under 6,000 bytes, cut at a line end with a note naming the full index.
- Hidden characters are dropped first (see [session start](@/docs/memory.md#load-at-session-start)), except a zero-width joiner or non-joiner between two non-ASCII characters, as in an emoji or a Persian word.
- With no project, it prints nothing.
- With `memory.personal: repo`, it names the personal folder even before an index exists. In checkout mode, no index means no output.
- It finds the project as the `shared-memory` skill does.
{% </details> %}

## hook approve

Answer a permission prompt from inside a `PermissionRequest` hook. The [`approvals` built-in](@/docs/approvals.md) sets this up for Claude Code, Codex, Kiro and Qwen Code through `sync --global`.

```bash
command -v agnostic-ai >/dev/null 2>&1 || exit 0; agnostic-ai hook approve --target claude
```

When the approvals mode for the tool and the session's folder is `yolo` and [no always-ask rule](@/docs/approvals.md#what-always-asks) matches, it prints the tool's allow reply, which allows that one call: `{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow"}}}` for Claude Code and Codex, and the same without `hookEventName` for Kiro and Qwen Code. In every other case it prints nothing and the tool shows its prompt. It always exits 0, never denies, and writes nothing to stderr. In `yolo` it adds a line per request to the [decision log](@/docs/approvals.md#the-decision-log); when that write fails, it asks.

| Flag | Description |
|------|-------------|
| `-t`, `--target <name>` | Tool that runs the hook: `claude`, `codex`, `kiro` or `qwen`. Defaults to `AGNOSTIC_AI_TARGET`. |

## hook worktree-refresh

Rerun Claude Code's configured environment setup in one registered linked worktree.

```bash
agnostic-ai hook worktree-refresh --repo /path/to/main --worktree "/path/to/linked worktree"
```

`--repo` is a checkout of the owning repository and `--worktree` the linked checkout. The helper checks that the worktree is registered to that repository and refuses the main checkout.

When setup changes, run `agnostic-ai sync` in that worktree first. Sync resets the completion marker only when the generated setup script matches current source, including output-path overrides such as `outputs.claude.dir`. See [refresh an existing worktree](@/docs/spec-format/environments.md#refresh-an-existing-worktree) for how markers work.

{% <details summary="Details: limits of worktree-refresh"> %}
- Setup runs from the worktree with a 600-second limit. Output and failures pass through.
- Cancellation stops the setup processes it owns. A successful setup can leave background services running.
- Other worktrees keep their markers.
- On Windows it needs Git Bash, from `CLAUDE_CODE_GIT_BASH_PATH` or the Git for Windows install, never a `bash` launcher from PATH.
- It installs dependencies only if your setup command does.
- It does not check application health or change other tools' setup. Normal Claude setup hooks still run once per worktree.
{% </details> %}

## hook run

Test a synced hook spec without starting a session. See [test a hook](@/docs/cli-reference/hook-diagnostics.md#hook-run) for the sample events, results, and exit codes.

```bash
agnostic-ai hook run protect-files --edit .github/workflows/tests.yml --expect block
```

{% <details summary="Flags: choose the event, the tools, and the expected result"> %}
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
{% </details> %}

## install-hook

Install git hooks; see the [git hooks](@/docs/git-hooks.md) guide. By default it installs a pre-commit hook that runs `project --check --against index`, so a commit fails if regenerated files are not staged. Project hooks prefer the installed local binary over `PATH`.

```bash
agnostic-ai install-hook            # writes .git/hooks/pre-commit (local)
agnostic-ai install-hook --shared   # writes .githooks/ and sets core.hooksPath
agnostic-ai install-hook --global   # checks global specs before committing

agnostic-ai install-hook --post-checkout            # writes .git/hooks/post-checkout and post-merge
agnostic-ai install-hook --post-checkout --shared   # writes both hooks in .githooks/
```

{% <details summary="Details: existing hooks and each option"> %}
- An existing hook keeps its content, and the checks go at its end.
- A hook that would stop before them is left alone, and the command prints the lines to add by hand. That means its first line does not select `sh` or `bash`, or it has an `exec` or an unindented `exit`.
- `--shared` writes `.githooks/<hook>` at the root of the main working tree, even from a linked worktree. It stops if `core.hooksPath` already points elsewhere.
- `--global` is for the global specs folder, which must be the root of its own git repository. The hook runs `lint --global --strict`, `validate --global`, and `sync --global --check` (skipped in a linked worktree).
- `--global` is not allowed with `--shared` or `--post-checkout`. It stops when `core.hooksPath` points elsewhere or the hook already runs the project `sync --check`.
- `--post-checkout` installs `post-checkout` and `post-merge`. They run `agnostic-ai project` from the worktree root after a branch or worktree checkout (not a single file) or a merge, pull included. See [regenerate on checkout](@/docs/git-hooks.md#regenerate-on-checkout).
{% </details> %}

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

Start the language server on stdin and stdout. The [VS Code extension](https://github.com/Chemaclass/agnostic-ai/tree/main/editors/vscode) starts it for a configured workspace, using `agnostic-ai.binaryPath`. Other editors can launch `agnostic-ai lsp` for Markdown and YAML files.

It lints the project's configured sources when you open or save a file.

{% <details summary="Details: what gets linted and when diagnostics clear"> %}
- Custom and absolute source directories are linted too. Unsaved edits are not.
- Config and spec load failures appear on the source file, with the parser location when known, or as a workspace error.
- A successful check clears resolved diagnostics from every affected file, deleted sources included.
- After a failed load, earlier diagnostics stay until you fix the problem and save a project file.
{% </details> %}

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


Choose the command for the change you need: `upgrade` installs a CLI release, `migrate` updates old spec formats, and `project` checks or prepares the project's installed CLI. Use `revert` to restore generated-file backups and `cleanup` to remove those backups after review.

- [Project CLI and readiness](@/docs/cli-reference/project.md): check the installed version, bootstrap dependencies, and gate startup.
- [Upgrade and migrate](@/docs/cli-reference/upgrades.md): update the binary or rewrite old spec forms.
- [Memory commands](@/docs/cli-reference/memory.md): inspect stores and repair indexes.
- Below: handoffs, generated-file recovery, packs, hooks, completion, and editor support.

## handoff list

List saved task handoffs across the repository's worktrees:

```bash
agnostic-ai handoff list
```

Each branch gets one row with its live worktree (or `removed`), saved HEAD and date, current working changes (`clean`, `dirty`, or `unknown`), task status, PR, and handoff file. The newest manual note takes precedence over automatic snapshots; a snapshot is shown when no manual note exists. Missing status or PR fields show `-`. Legacy `HANDOFF.md` and `HANDOFF.auto.md` remain readable.

It reads bounded headers from ignored stores and keeps configured subprojects separate. A bare main checkout or an unignored shared store uses the current checkout's ignored store. The command prints a removal suggestion only when local Git ancestry proves the branch is merged into the main checkout's branch and no live working changes are pending. Squash merges need your review because ancestry cannot prove them. It does not fetch or delete files.

## revert

Undo a `sync --backup`. For every generated file and tool instruction file (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, `CONVENTIONS.md`, `.agnostic-ai/AGNOSTIC_AI.md`), `revert` restores `<path>.bak` and removes it. It also restores a nested `CLAUDE.md` that sync deleted. Files without a `.bak` stay unless you pass `--force`.

| Flag | Description |
|------|-------------|
| `-t`, `--only`, `--except` | Select targets, as in [`sync`](@/docs/cli-reference/sync.md#sync). |
| `--dry-run` | Show what it would do and change nothing. |
| `--force` | Also delete generated files that have no `.bak`, including generated tool instruction files and your own files at the same paths. |
| `--json` | Same format as `sync --json`. Actions: `"restore"`, `"remove"`, `"preserve"` (no `.bak`, no `--force`), `"skip"` (already gone). |

Paths under [`sync.unmanaged`](@/docs/configuration.md#syncunmanaged) are never restored or removed.

## cleanup

Remove the `<path>.bak` backups that `sync --backup` wrote. Other `.bak` files are never touched.

```bash
agnostic-ai cleanup
agnostic-ai cleanup --dry-run   # preview deletions
```

## packs

Add, update, and remove collections of shared specs. A project spec wins over a pack spec with the same name. See the [packs](@/docs/packs.md) guide.

```bash
agnostic-ai packs add github.com/obra/superpowers@v6.4.2
agnostic-ai packs add ./path/to/pack
agnostic-ai packs list
agnostic-ai packs update [name]
agnostic-ai packs remove superpowers
```

## hook paths

Run inside an edit hook. It reads the hook's event data on stdin and prints the files the edit leaves on disk, one per line, relative to the current directory. A tool call that is not an edit prints nothing. See [edited paths](@/docs/spec-format/hook-reference.md#edited-paths).

```bash
files=$(agnostic-ai hook paths) || exit 1
printf '%s\n' "$files" | grep '\.go$' | while IFS= read -r f; do gofmt -w "$f"; done
```

`agnostic-ai` must be on the hook's `PATH`. Capture the output first, as above: piped straight into the loop, a failure is lost and the hook exits 0.

It exits 1 on invalid JSON, a `tool_input` that is not an object for an edit tool, or a missing or unsupported target.

| Flag | Description |
|------|-------------|
| `-t`, `--target <name>` | Tool that sent the event data. Defaults to `AGNOSTIC_AI_TARGET`. With neither, a Claude Code file-tool event reads as `claude`, and any other event fails. |
| `--action` | Print every change as `<action><TAB><path>`: `add`, `update`, `delete`, or `move`. Deleted files and move sources appear only here and in `--json`. |
| `--json` | Print every change as a JSON array of `{action, path, from}`; `from` is a move's source. Not with `--action`. |

## hook worktree-remove

Remove one clean Git worktree from a Claude Code `WorktreeRemove` event on stdin. Both flags are required: `--repo` selects a surviving checkout of the repository, and `--allowed-root` selects the directory whose descendants you permit the helper to remove. Quote paths that contain spaces.

```bash
agnostic-ai hook worktree-remove --repo "/path/to/repository" --allowed-root "/path/to/disposable worktrees"
```

The helper verifies Git's worktree registration and the checkout's repository metadata before running `git worktree remove` without force. It refuses the main checkout, the allowed root itself, other repositories, unregistered directories, traversal, symlinks below the allowed root, locked worktrees, and worktrees with changed, untracked, or ignored files. It also refuses indexes with `assume-unchanged` or `skip-worktree` entries, including sparse checkouts, because those flags can hide changed file contents. Ignored dependencies also prevent removal; remove them deliberately before retrying. It keeps the branch.

According to [Claude Code's WorktreeRemove contract](https://code.claude.com/docs/en/hooks#worktreeremove), this event replaces cleanup for hook-created worktrees and runs while the directory still exists. Exit 0 means cleanup succeeded; a non-zero exit preserves an existing directory without a Git fallback. Pair this helper with your `WorktreeCreate` hook, and keep `--repo` outside the disposable checkout. It does not change cleanup for worktrees the host creates without a custom creation hook.

Success prints `removed: <path>` and exits 0. A path already absent below the allowed root prints `already absent: <path>` and exits 0, including repeated calls after successful removal. Absence alone cannot prove prior ownership; this result performs no deletion or registration pruning. Unsafe inputs, event data larger than 1 MiB, and failed Git operations exit 1.

| Flag | Description |
|------|-------------|
| `--repo <path>` | Required. A surviving checkout whose registered worktree may be removed. |
| `--allowed-root <path>` | Required. Only descendants of this directory may be removed. |

## hook guard

Run inside an `after-edit` or `stop` hook. It exits 2 with a short report for the agent on a problem, and 0 with no output otherwise. See the [spec guard hook](@/docs/spec-format/hook-reference.md#spec-guard).

```bash
command -v agnostic-ai >/dev/null 2>&1 || exit 0; agnostic-ai hook guard after-edit
command -v agnostic-ai >/dev/null 2>&1 || exit 0; agnostic-ai hook guard stop
```

- `after-edit` reads the event data on stdin, like [`hook paths`](#hook-paths), and reports the lint errors in the specs the edit touched. An edit outside the spec sources prints nothing.
- `stop` reports when specs changed without a sync. A hand edit to a generated file is left to `sync --check`. It passes when the agent already continued from a stop hook, so the notice cannot loop. It never runs sync.
- It finds the project from the nearest `agnostic-ai.yaml` in the current directory or above. If it cannot read something, such as unknown event data or no config, it exits 0.

| Flag | Description |
|------|-------------|
| `-t`, `--target <name>` | Tool that sent an `after-edit` event. Defaults to `AGNOSTIC_AI_TARGET`, as for `hook paths`. |

## hook memory

Prints the `MEMORY.md` fact lists for a `session-start` hook, so the tool adds them to the model's context. Personal memory comes first, then project memory. The [`memory` built-in](@/docs/memory.md#load-at-session-start) sets this up for you, and runs it on Claude Code when a subagent starts.

```bash
agnostic-ai hook memory --target codex
```

- Codex and Factory get plain text. Cursor gets `{"additional_context": ...}`. Gemini CLI and Qoder get `{"hookSpecificOutput": {"hookEventName": "SessionStart", "additionalContext": ...}}`. Copilot gets that object plus a top-level `additionalContext`: VS Code reads the object, and Copilot CLI reads the top-level key.
- Claude Code gets `{"hookSpecificOutput": {"hookEventName": "SubagentStart", "additionalContext": ...}}` with personal memory only, since `CLAUDE.md` gives subagents the project list. It prints nothing when `CLAUDE.md` imports the personal list too.
- Output stays under 6,000 bytes. A longer index is cut at the end of a line, with a note saying where the full index is.
- Hidden characters, such as zero-width and bidirectional controls and Unicode tags, are left out before the cut, so they do not count toward the 6,000 bytes. `memory lint` reports them. A zero-width joiner or non-joiner between two non-ASCII characters, as in an emoji or a Persian word, stays.
- With no project, it prints nothing. With `memory.personal: repo`, it names the personal folder even before an index exists; in checkout mode, no index means no output. It finds the project the same way the `shared-memory` skill does.

| Flag | Description |
|------|-------------|
| `-t`, `--target <name>` | Tool that runs the hook. Defaults to `AGNOSTIC_AI_TARGET`. |

## hook worktree-refresh

Rerun Claude Code's configured environment setup in one registered linked worktree:

```bash
agnostic-ai hook worktree-refresh --repo /path/to/main --worktree "/path/to/linked worktree"
```

Pass a checkout of the owning repository with `--repo` and the linked checkout with `--worktree`. The helper refuses the main checkout and paths that are not registered to that repository. Run `agnostic-ai sync` in the selected worktree first when setup changes. Its generated setup script must match current source, including configured output-path overrides, before the helper resets the completion marker.

Setup runs from the selected worktree with a 600-second limit. Output and failures pass through to the caller. Cancellation stops setup processes the helper owns; successful setup can leave background services running. A failed run leaves no completion marker; run the command again after fixing the failure. Other worktrees keep their markers. Normal Claude setup hooks still run once per worktree. Windows requires Git Bash, resolved from `CLAUDE_CODE_GIT_BASH_PATH` or the Git for Windows installation; the helper does not use a Windows `bash` launcher from PATH.

This command runs your configured setup, which can install dependencies or perform other project setup. It does not check application health or change other tools' setup behavior. See [environment specs](@/docs/spec-format/environments.md#claude-code-worktree-setup).

## hook run

Test a hook spec without starting a session. Run `sync` first so the hook's scripts are in place. For each tool the hook applies to, `hook run` sends a sample event to the synced command and prints the result: allow or block, exit code, time, stdout, and stderr.

```bash
agnostic-ai hook run protect-files --edit .github/workflows/tests.yml --expect block
agnostic-ai hook run guard --target codex --bash "git push --force"
agnostic-ai hook run greet --prompt "ship it"
agnostic-ai hook run on-stop --payload stop.json
agnostic-ai hook run protect-files --edit .env --format json
```

See [test a hook](@/docs/cli-reference/hook-diagnostics.md#hook-run) for the sample events, results, exit codes, and stale tool file warnings.

| Flag | Description |
|------|-------------|
| `-t`, `--target <names>` | Test only these tools. Defaults to every configured tool the hook applies to. |
| `--edit <path>` | Send a `PreToolUse` or `PostToolUse` event that edits this file. |
| `--bash <command>` | Send a `PreToolUse` or `PostToolUse` event that runs this shell command. |
| `--prompt <text>` | Prompt text for `UserPromptSubmit`. |
| `--runtime vscode` | Model Cline VS Code TaskStart on Unix. Requires `--target cline`; default Cline diagnostics model the SDK. Windows launcher selection is not modeled. |
| `--payload <file>` | Send this JSON file to every tool, for events the flags above do not cover. |
| `--expect allow\|block` | Fail unless every tool gives this result. |
| `--include-assumed` | Count tools whose shell, timeout, or working directory is a guess toward `--expect` and the comparison between tools. See [assumed results](@/docs/cli-reference/hook-diagnostics.md#assumed-results). |
| `--format text\|json` | `json` prints one report with a `targets` array containing each tool's result, warnings, and command output. See the [JSON fields](@/docs/cli-reference/hook-diagnostics.md#json-output). Defaults to `text`. |

## install-hook

Install git hooks. By default it installs a pre-commit hook that runs `project --check --against index`, so a commit fails if regenerated files are not staged. With `--post-checkout`, it installs hooks that regenerate the tool files after a checkout or a pull. Project hooks prefer the installed local binary over `PATH`. See [git hooks](@/docs/git-hooks.md).

```bash
agnostic-ai install-hook            # writes .git/hooks/pre-commit (local)
agnostic-ai install-hook --shared   # writes .githooks/ and sets core.hooksPath
agnostic-ai install-hook --global   # checks global specs before committing

agnostic-ai install-hook --post-checkout            # writes .git/hooks/post-checkout and post-merge
agnostic-ai install-hook --post-checkout --shared   # writes both hooks in .githooks/
```

An existing hook keeps its content, and the checks go at its end. A hook that would stop before reaching them is left alone, and the command prints the lines to add by hand. A hook stops early when its first line does not select `sh` or `bash`, or it contains an `exec` or an unindented `exit`.

- `--shared` writes `.githooks/<hook>` at the root of the main working tree, even from a linked worktree. It stops if `core.hooksPath` already points elsewhere.
- `--global` is for the global specs folder, which must be the root of its own git repository. The hook runs `lint --global --strict`, `validate --global`, and `sync --global --check` (skipped in a linked worktree), and the commit fails if any fails. It cannot combine with `--shared` or `--post-checkout`, and it stops when `core.hooksPath` points elsewhere or the hook already runs the project `sync --check`.
- `--post-checkout` installs `post-checkout` and `post-merge`. They run `agnostic-ai project` from the worktree root after a branch or worktree checkout (not a single-file checkout) or a merge, including a pull. They preserve manual edits, skip a checkout without `agnostic-ai.yaml`, and report a missing binary with a recovery command. They never install dependencies.

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

Start the language server on stdin and stdout. The [VS Code extension](https://github.com/Chemaclass/agnostic-ai/tree/main/editors/vscode) starts it automatically for a configured workspace, using `agnostic-ai.binaryPath`. Other editors can launch `agnostic-ai lsp` for Markdown and YAML files. It reads saved files and reports lint problems when you open or save a file. Config and spec load failures appear on the source file, with the parser location when available. A failure with no known source appears as a workspace error.

After a successful check, resolved diagnostics clear from every affected file, including deleted sources. If loading fails, earlier diagnostics stay until a successful check replaces them. Fix the reported problem and save a project file to retry.

The server checks the selected project's configured sources, including custom and absolute directories. Unsaved buffer edits do not run lint. Files keep their ordinary Markdown or YAML editing support.

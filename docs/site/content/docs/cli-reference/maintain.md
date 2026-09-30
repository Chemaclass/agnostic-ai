+++
title = "Maintain and set up"
description = "Revert or clean generated files, share packs, and set up hooks, completion, and updates."
weight = 50

[extra]
group = "Reference"
+++

# Maintain and set up

## revert

Undo a `sync --backup`. For every emitted file and entry-point file (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, `CONVENTIONS.md`, `.agnostic-ai/AGNOSTIC_AI.md`), `revert` restores `<path>.bak` and removes the .bak. It also restores a nested `CLAUDE.md` that sync deleted for Claude's scoped rules. Files without a `.bak` stay unless you pass `--force`.

| Flag | Description |
|------|-------------|
| `-t`, `--only`, `--except` | Select targets, as in [`sync`](@/docs/cli-reference/sync.md#sync). |
| `--dry-run` | Report intended actions without touching disk |
| `--force` | Also delete emitted files that lack a `.bak`, including generated entry-point files and user files sharing their paths. |
| `--json` | Same schema as `sync --json`. Actions: `"restore"` (`.bak` applied), `"remove"` (deleted), `"preserve"` (no `.bak`, no `--force`), `"skip"` (already absent). |

Paths under [`sync.unmanaged`](@/docs/configuration.md#syncunmanaged) are never restored or removed.

## cleanup

Remove the `<path>.bak` backups `sync --backup` wrote for emitted paths. Unrelated `.bak` files are never touched.

```bash
agnostic-ai cleanup
agnostic-ai cleanup --dry-run   # preview deletions
```

## packs

Manage shareable spec packs. Packs load as a layer below the project, so a project spec overrides a pack entry with the same name. Full guide in [packs](@/docs/packs.md).

```bash
agnostic-ai packs add github.com/chemaclass/go-rules@v1.2.0
agnostic-ai packs add ./path/to/pack
agnostic-ai packs list
agnostic-ai packs update [name]
agnostic-ai packs remove go-rules
```

## hook paths

Run inside an edit hook. Reads the hook payload on stdin and prints the files the edit leaves on disk, one per line, relative to the current directory. A tool call that is no edit prints nothing. See [edited paths](@/docs/spec-format/hooks.md#edited-paths) for the payloads each target sends.

```bash
files=$(agnostic-ai hook paths) || exit 1
printf '%s\n' "$files" | grep '\.go$' | while IFS= read -r f; do gofmt -w "$f"; done
```

`agnostic-ai` must be on the hook's `PATH`. Capture the output first, as above: piped straight into the loop, a failure of `hook paths` ends with exit 0.

It exits 1 on invalid JSON, on an edit tool's `tool_input` that is not an object, and on a missing or unsupported target.

| Flag | Description |
|------|-------------|
| `-t`, `--target <name>` | Target that sent the payload. Defaults to `AGNOSTIC_AI_TARGET`. With neither, a Claude Code file tool payload reads as `claude`, and any other payload fails. |
| `--action` | Print every change as `<action><TAB><path>`, with `add`, `update`, `delete`, or `move`. Deleted files and move sources show only here and in `--json`. |
| `--json` | Print every change as a JSON array of `{action, path, from}`; `from` is a move's source. Not with `--action`. |

## hook run

Run one hook spec before a session fires it. For each target the hook reaches, it builds that target's payload, runs the command sync wrote with that target's env, shell, and timeout, from the project root, and prints the decision, exit code, time, stdout, and stderr. Run `sync` first, so the scripts sync copies are in place. See [test a hook](@/docs/spec-format/hooks.md#hook-run) for the payloads and decisions.

```bash
agnostic-ai hook run protect-files --edit .github/workflows/tests.yml --expect block
agnostic-ai hook run guard --target codex --bash "git push --force"
agnostic-ai hook run greet --prompt "ship it"
agnostic-ai hook run on-stop --payload stop.json
```

It exits 1 when a command times out, when two targets decide differently, or when a decision is not the one `--expect` names.

| Flag | Description |
|------|-------------|
| `-t`, `--target <names>` | Run only for these targets. Defaults to every configured target the hook reaches. |
| `--edit <path>` | Build a `PreToolUse` or `PostToolUse` event that edits this path. |
| `--bash <command>` | Build a `PreToolUse` or `PostToolUse` event that runs this shell command. |
| `--prompt <text>` | Prompt text for `UserPromptSubmit`. |
| `--payload <file>` | Send this JSON file to every target as the payload, for events with no builder. |
| `--expect allow\|block` | Fail unless every target decides this. |

## install-hook

Install a pre-commit hook that runs `sync --check`, or, with `--post-checkout`, hooks that regenerate tool files after a checkout or a pull that merges. See [git hooks](@/docs/git-hooks.md).

```bash
agnostic-ai install-hook            # writes .git/hooks/pre-commit (local)
agnostic-ai install-hook --shared   # writes .githooks/ and sets core.hooksPath
agnostic-ai install-hook --global   # gates commits to a global home kept in git

agnostic-ai install-hook --post-checkout            # writes .git/hooks/post-checkout and post-merge
agnostic-ai install-hook --post-checkout --shared   # writes both hooks in .githooks/
```

An existing hook keeps its content and the checks go at its end. A hook that already holds them stays as it is. A hook that would stop before reaching them (no `sh` or `bash` shebang, an `exec`, or an unindented `exit`) is left alone, and the command prints the lines to add by hand.

- `--shared` writes `.githooks/<hook>` at the root of the main working tree, from any linked worktree. It stops when `core.hooksPath` already points elsewhere.
- `--global` is for the global home, which must be the root of its own git repository. The hook runs `lint --global --strict`, `validate --global`, and `sync --global --check`; the commit fails when any fails. In a linked worktree it skips `sync --global --check`. It stops when run anywhere else, when `core.hooksPath` points elsewhere, or when the hook still runs the project `sync --check`. Not with `--shared` or `--post-checkout`.
- `--post-checkout` installs `post-checkout` and `post-merge`, which run `agnostic-ai sync -q` from the worktree root after a branch or worktree checkout (never a single-file checkout) or a merge, including a pull. Both skip when the binary or `agnostic-ai.yaml` is missing. The hooks directory is shared across linked worktrees. Reinstall an older checkout-only setup to add pull coverage.

## completion

Generate a shell completion script.

```bash
agnostic-ai completion bash > ~/.local/share/bash-completion/completions/agnostic-ai  # or /etc/bash_completion.d/
agnostic-ai completion zsh > "${fpath[1]}/_agnostic-ai"
agnostic-ai completion fish > ~/.config/fish/completions/agnostic-ai.fish
agnostic-ai completion powershell | Out-String | Invoke-Expression
```

Restart your shell or `source` the file. Completing `--target` reads `agnostic-ai.yaml` in the current directory, or offers every target. See `agnostic-ai completion <shell> --help`.

## upgrade

Upgrade the running binary to the latest release with the method it was installed with. `update` is an alias.

```bash
agnostic-ai upgrade --version v0.56.1
```

| Flag | Description |
|------|-------------|
| `--check` | Print install details and exit without changing anything. With `--version`, adds a `Requested:` line and downloads nothing. |
| `--version <tag>` | Install one release, downgrades included. The leading `v` is optional. Standalone binaries only; package-manager installs are told to pin through their manager. |
| `--run` | Accepted for compatibility; upgrading is the default. |

| Binary location | Upgrade |
|-----------------|---------|
| `*/Cellar/*`, `*/Caskroom/*`, `/opt/homebrew/*`, `/home/linuxbrew/.linuxbrew/*` | `brew update && brew upgrade --cask Chemaclass/tap/agnostic-ai` |
| `$GOBIN` or `$GOPATH/bin` (defaults to `$HOME/go/bin`) | `go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@latest` |
| `*\scoop\apps\*`, `*\scoop\shims\*` | `scoop update agnostic-ai` |
| `*\Microsoft\WinGet\*` | `winget upgrade Chemaclass.agnostic-ai` |
| `*/node_modules/*` | `npm install -g agnostic-ai@latest` |
| Standalone binary on macOS or Linux | Download the release, verify checksum and version, replace the binary atomically. |
| Standalone binary on Windows | Use the [PowerShell install script](@/docs/installation.md); Windows cannot replace a running executable. |

Scoop, WinGet, and `node_modules` markers match case-insensitively. `upgrade` also lists any other `agnostic-ai` on `PATH` that shadows the resolved executable.

## lsp

Start the Language Server on stdin/stdout. Point your editor at `agnostic-ai lsp` for spec files (`.agnostic-ai/**/*.md`, `*.mdc`). It pushes lint diagnostics on open and save.


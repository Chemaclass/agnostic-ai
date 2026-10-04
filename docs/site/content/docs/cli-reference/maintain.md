+++
title = "Maintain and set up"
description = "Revert or clean generated files, share packs, and set up hooks, completion, and updates."
weight = 50

[extra]
group = "Reference"
+++

# Maintain and set up

## revert

Undo a `sync --backup`. For every emitted file and entry-point file (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, `CONVENTIONS.md`, `.agnostic-ai/AGNOSTIC_AI.md`), `revert` restores `<path>.bak` and removes the `.bak`. It also restores a nested `CLAUDE.md` that sync deleted for Claude's scoped rules. Files without a `.bak` stay unless you pass `--force`.

| Flag | Description |
|------|-------------|
| `-t`, `--only`, `--except` | Select targets, as in [`sync`](@/docs/cli-reference/sync.md#sync). |
| `--dry-run` | Report what it would do without touching disk |
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

Manage shareable spec packs. Packs load as a layer below the project, so a project spec overrides a pack entry with the same name. See the [packs](@/docs/packs.md) guide.

```bash
agnostic-ai packs add github.com/obra/superpowers@v6.4.2
agnostic-ai packs add ./path/to/pack
agnostic-ai packs list
agnostic-ai packs update [name]
agnostic-ai packs remove superpowers
```

## hook paths

Run inside an edit hook. It reads the hook payload on stdin and prints the files the edit leaves on disk, one per line, relative to the current directory. A tool call that is not an edit prints nothing. See [edited paths](@/docs/spec-format/hooks.md#edited-paths) for the payloads each target sends.

```bash
files=$(agnostic-ai hook paths) || exit 1
printf '%s\n' "$files" | grep '\.go$' | while IFS= read -r f; do gofmt -w "$f"; done
```

`agnostic-ai` must be on the hook's `PATH`. Capture the output first, as above. If you pipe it straight into the loop, a failure of `hook paths` ends with exit 0.

It exits 1 on:

- invalid JSON
- an edit tool's `tool_input` that is not an object
- a missing or unsupported target

| Flag | Description |
|------|-------------|
| `-t`, `--target <name>` | Target that sent the payload. Defaults to `AGNOSTIC_AI_TARGET`. With neither, a Claude Code file tool payload reads as `claude`, and any other payload fails. |
| `--action` | Print every change as `<action><TAB><path>`, with `add`, `update`, `delete`, or `move`. Deleted files and move sources show only here and in `--json`. |
| `--json` | Print every change as a JSON array of `{action, path, from}`; `from` is a move's source. Not with `--action`. |

## hook run

Run one hook spec before a session fires it. Run `sync` first, so the scripts sync copies are in place. For each target the hook reaches, `hook run`:

1. Builds that target's payload.
2. Runs the command sync wrote, from the project root (or Copilot's `cwd`), with that target's env, shell, and timeout.
3. Prints the decision, exit code, time, stdout, and stderr.

See [test a hook](@/docs/spec-format/hooks.md#hook-run) for the payloads and decisions.

```bash
agnostic-ai hook run protect-files --edit .github/workflows/tests.yml --expect block
agnostic-ai hook run guard --target codex --bash "git push --force"
agnostic-ai hook run greet --prompt "ship it"
agnostic-ai hook run on-stop --payload stop.json
agnostic-ai hook run protect-files --edit .env --format json
```

It exits 1 when:

- a command times out or errors (such as a missing script)
- two targets decide differently
- a decision is not the one `--expect` names

It warns, without failing, when a target's synced native file (`.claude/settings.json` or `.codex/hooks.json`) does not run the command the spec produces. Run `sync` to fix it.

| Flag | Description |
|------|-------------|
| `-t`, `--target <names>` | Run only for these targets. Defaults to every configured target the hook reaches. |
| `--edit <path>` | Build a `PreToolUse` or `PostToolUse` event that edits this path. |
| `--bash <command>` | Build a `PreToolUse` or `PostToolUse` event that runs this shell command. |
| `--prompt <text>` | Prompt text for `UserPromptSubmit`. |
| `--payload <file>` | Send this JSON file to every target as the payload, for events with no builder. |
| `--expect allow\|block` | Fail unless every target decides this. |
| `--include-assumed` | Count results that rest on an assumed shell, timeout, or working directory, such as Cursor's, Copilot's, Factory's, Antigravity's, Cline's, Kiro's, and Windsurf's, in `--expect` and the comparison. See [assumed results](@/docs/spec-format/hooks.md#assumed-results). |
| `--format text\|json` | `json` prints one object per target, with its decision, warnings, and each command's exit code, time, stdout, and stderr. Defaults to `text`. |

## install-hook

Install git hooks. By default it installs a pre-commit hook that runs `sync --check --against index`, so a commit that leaves regenerated files unstaged fails. With `--post-checkout`, it installs hooks that regenerate tool files after a checkout or a pull that merges. See [git hooks](@/docs/git-hooks.md).

```bash
agnostic-ai install-hook            # writes .git/hooks/pre-commit (local)
agnostic-ai install-hook --shared   # writes .githooks/ and sets core.hooksPath
agnostic-ai install-hook --global   # gates commits to a global home kept in git

agnostic-ai install-hook --post-checkout            # writes .git/hooks/post-checkout and post-merge
agnostic-ai install-hook --post-checkout --shared   # writes both hooks in .githooks/
```

An existing hook keeps its content, and the checks go at its end. A hook that already holds them stays as it is. A hook that would stop before reaching them is left alone, and the command prints the lines to add by hand. A hook stops early when it has no `sh` or `bash` shebang, an `exec`, or an unindented `exit`.

- `--shared` writes `.githooks/<hook>` at the root of the main working tree, from any linked worktree. It stops when `core.hooksPath` already points elsewhere.
- `--global` is for the global home, which must be the root of its own git repository. The hook runs `lint --global --strict`, `validate --global`, and `sync --global --check`. The commit fails when any of them fails. In a linked worktree it skips `sync --global --check`. Not with `--shared` or `--post-checkout`.
- `--post-checkout` installs `post-checkout` and `post-merge`. They run `agnostic-ai sync -q` from the worktree root after a branch or worktree checkout (never a single-file checkout) or a merge, including a pull. Both skip when the binary or `agnostic-ai.yaml` is missing. The hooks directory is shared across linked worktrees.

{% <details summary="When install-hook --global stops, and older setups"> %}
`--global` stops when run anywhere but the root of the global home's repository, when `core.hooksPath` points elsewhere, or when the hook still runs the project `sync --check`.

Reinstall an older checkout-only `--post-checkout` setup to add pull coverage.
{% </details> %}

## completion

Generate a shell completion script.

```bash
agnostic-ai completion bash > ~/.local/share/bash-completion/completions/agnostic-ai  # or /etc/bash_completion.d/
agnostic-ai completion zsh > "${fpath[1]}/_agnostic-ai"
agnostic-ai completion fish > ~/.config/fish/completions/agnostic-ai.fish
agnostic-ai completion powershell | Out-String | Invoke-Expression
```

Restart your shell or `source` the file. Completing `--target` reads `agnostic-ai.yaml` in the current directory, or offers every target if there is none. See `agnostic-ai completion <shell> --help`.

## upgrade

Upgrade the running binary to the latest release, using the method it was installed with. `update` is an alias.

```bash
agnostic-ai upgrade --version v0.56.1
```

| Flag | Description |
|------|-------------|
| `--check` | Print install details and exit without changing anything. With `--version`, adds a `Requested:` line and downloads nothing. |
| `--version <tag>` | Install one release, downgrades included. The leading `v` is optional. Standalone binaries only: for a package-manager install, the command tells you to pin through that manager. |
| `--requires` | Set this project's `requires` and schema tag to the installed release, then sync. |
| `--run` | Accepted for compatibility; upgrading is the default. |

After upgrading through a package manager, use its installed CLI from the project root:

```bash
pnpm exec agnostic-ai upgrade --requires
```

`--requires` replaces a minimum, range, or older pin with the installed exact release. It updates the base config and any existing local `requires` override, including a null override. Comments and unrelated settings stay intact. The command holds the project lock through the config changes and sync. If sync fails, the new pins stay. Fix the reported output problem and run `sync` again.

This mode needs a stable release build. It works with `agnostic.config.yaml` too. It cannot combine with `--check`, `--version`, or `--run`, and it does not update global home config. Your package manager owns install and dependency files.

The config edit keeps unrelated YAML bytes intact. Before running it, convert flow-style root mappings, merged root keys, and anchored or multiline `requires` values to a plain block mapping and a single-line scalar.

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

## migrate

Rewrite old spec forms into their current replacements, such as a renamed field or file. A migration never changes what sync writes for the targets a spec already reaches, so `sync --check` stays clean after it. Old forms keep working, so you never have to run it before a sync.

```bash
agnostic-ai migrate --list      # which migrations apply here
agnostic-ai migrate --dry-run   # preview the rewrites
agnostic-ai migrate             # apply them
```

| Flag | Description |
|------|-------------|
| `--dry-run` | Print each rename and a diff of each rewrite, and write nothing. Values under credential-named keys, and values that look like a credential, print as `<redacted>`. |
| `--list` | List every migration with the release that added it and whether it applies here. |
| `--only <group>` | Run only these groups, comma-separated. A migration ID starts with its group, such as `config-file-name` in group `config`. |

- A migration rewrites only what maps one to one. Anything else stays as written, and the output says why.
- It writes each file atomically and keeps its mode. A rename writes the new file before it removes the old one.
- It refuses to run in the global home; edit those specs by hand.
- Running it twice changes nothing.

| Migration | Release | Rewrites |
|-----------|---------|----------|
| `config-file-name` | 0.79.0 | `agnostic.config.yaml` to `agnostic-ai.yaml`. When both exist with the same content, it removes the old file. Skipped when they differ, since `agnostic-ai.yaml` wins, and when Git ignores `agnostic-ai.yaml`. |

## lsp

Start the Language Server on stdin/stdout. Point your editor at `agnostic-ai lsp` for spec files (`.agnostic-ai/**/*.md`, `*.mdc`). It pushes lint diagnostics on open and save.

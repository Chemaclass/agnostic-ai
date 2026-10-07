+++
title = "Maintain and set up"
description = "Revert or clean generated files, share packs, and set up hooks, completion, and updates."
weight = 50

[extra]
group = "Reference"
+++

# Maintain and set up

## revert

Undo a `sync --backup`. For every generated file and entry-point file (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, `CONVENTIONS.md`, `.agnostic-ai/AGNOSTIC_AI.md`), `revert` restores `<path>.bak` and removes it. It also restores a nested `CLAUDE.md` that sync deleted. Files without a `.bak` stay unless you pass `--force`.

| Flag | Description |
|------|-------------|
| `-t`, `--only`, `--except` | Select targets, as in [`sync`](@/docs/cli-reference/sync.md#sync). |
| `--dry-run` | Show what it would do and change nothing. |
| `--force` | Also delete generated files that have no `.bak`, including generated entry-point files and your own files at the same paths. |
| `--json` | Same format as `sync --json`. Actions: `"restore"`, `"remove"`, `"preserve"` (no `.bak`, no `--force`), `"skip"` (already gone). |

Paths under [`sync.unmanaged`](@/docs/configuration.md#syncunmanaged) are never restored or removed.

## cleanup

Remove the `<path>.bak` backups that `sync --backup` wrote. Other `.bak` files are never touched.

```bash
agnostic-ai cleanup
agnostic-ai cleanup --dry-run   # preview deletions
```

## packs

Manage shareable spec packs. A project spec wins over a pack spec with the same name. See the [packs](@/docs/packs.md) guide.

```bash
agnostic-ai packs add github.com/obra/superpowers@v6.4.2
agnostic-ai packs add ./path/to/pack
agnostic-ai packs list
agnostic-ai packs update [name]
agnostic-ai packs remove superpowers
```

## memory

Check, list, locate, and repair the [shared memory](@/docs/memory.md) in the working directory. Each subcommand reads project memory in `.agnostic-ai/memory/`, then personal memory in `.agnostic-ai/local/memory/`, or in the [repository store](@/docs/memory.md#one-store-per-repository) with `memory.personal: repo`. A store whose folder is missing is skipped.

```bash
agnostic-ai memory lint     # run only the memory checks
agnostic-ai memory index    # rebuild each MEMORY.md from its fact files
agnostic-ai memory list     # print each fact's scope, type, and title
agnostic-ai memory path     # print each memory folder's absolute path
```

- `memory lint` reports [LINT039 to LINT042](@/docs/cli-reference/check.md#lint), the memory findings `lint` and `doctor` also report. It exits 1 on an error, or on a warning with `--strict`. `--json` prints the same format as `lint --json`, with `command` set to `memory lint`.
- `memory index` drops merge conflict markers, links to missing files, and repeated links. Every other line stays in place, headings and notes included. It then adds a `- [name](file.md): description` line from the frontmatter of each fact without one. Run it after two tools edit the index at once.
- The type comes from `metadata.type`, or from a top-level `type` as Claude Code's auto memory writes it.
- With no store, `memory lint` says so and exits 0.
- `memory list` prints facts in index order. The title is the index line's link text, or the fact's `name` when no line links it.
- `memory path` prints one `scope  folder` line per store, project first, as absolute paths. It works from any folder inside the project and from a linked worktree, and prints a folder that does not exist yet. With `memory.personal: repo`, every worktree of the repository gets the same personal folder. The [`shared-memory-policy` rule](@/docs/memory.md#how-each-tool-loads-it) has tools without a session-start hook run it.

| Flag | Description |
|------|-------------|
| `--strict` | With `memory lint`, exit 1 on warnings too. |
| `--json` | With `memory lint`, print `{version, command, findings}` on stdout. |

## hook paths

Run inside an edit hook. It reads the hook's event data on stdin and prints the files the edit leaves on disk, one per line, relative to the current directory. A tool call that is not an edit prints nothing. See [edited paths](@/docs/spec-format/hooks.md#edited-paths).

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

## hook guard

Run inside an `after-edit` or `stop` hook. It exits 2 with a short report for the agent on a problem, and 0 with no output otherwise. See the [spec guard hook](@/docs/spec-format/hooks.md#spec-guard).

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

Prints the memory indexes for a `session-start` hook, so the tool adds them to the model's context. Personal memory comes first, then project memory. The [`memory` built-in](@/docs/memory.md#load-at-session-start) sets this up for you.

```bash
agnostic-ai hook memory --target codex
```

- Codex, Qoder, and Factory get plain text. Cursor gets `{"additional_context": ...}`, Copilot `{"additionalContext": ...}`, and Gemini CLI `{"hookSpecificOutput": {"additionalContext": ...}}`.
- Output stays under 6,000 bytes. A longer index is cut at the end of a line, with a note saying where the full index is.
- With no project, it prints nothing. With `memory.personal: repo`, it names the personal store even before an index exists; in checkout mode, no index means no output. It finds the project the same way the `shared-memory` skill does.

| Flag | Description |
|------|-------------|
| `-t`, `--target <name>` | Tool that runs the hook. Defaults to `AGNOSTIC_AI_TARGET`. |

## hook run

Test a hook spec without starting a session. Run `sync` first so the hook's scripts are in place. For each tool the hook applies to, `hook run` sends a sample event to the synced command and prints the result: allow or block, exit code, time, stdout, and stderr.

See [test a hook](@/docs/spec-format/hooks.md#hook-run) for the sample events and results.

```bash
agnostic-ai hook run protect-files --edit .github/workflows/tests.yml --expect block
agnostic-ai hook run guard --target codex --bash "git push --force"
agnostic-ai hook run greet --prompt "ship it"
agnostic-ai hook run on-stop --payload stop.json
agnostic-ai hook run protect-files --edit .env --format json
```

It exits 1 when:

- a command times out or fails, for example a missing script
- two tools give different results
- a result does not match `--expect`

If `.claude/settings.json` or `.codex/hooks.json` is out of date with the spec, it warns and tells you to run `sync`.

| Flag | Description |
|------|-------------|
| `-t`, `--target <names>` | Test only these tools. Defaults to every configured tool the hook applies to. |
| `--edit <path>` | Send a `PreToolUse` or `PostToolUse` event that edits this file. |
| `--bash <command>` | Send a `PreToolUse` or `PostToolUse` event that runs this shell command. |
| `--prompt <text>` | Prompt text for `UserPromptSubmit`. |
| `--payload <file>` | Send this JSON file to every tool, for events the flags above do not cover. |
| `--expect allow\|block` | Fail unless every tool gives this result. |
| `--include-assumed` | Count tools whose shell, timeout, or working directory is a guess (Cursor, Copilot, Factory, Antigravity, Cline, Kiro, Windsurf) toward `--expect` and the comparison between tools. See [assumed results](@/docs/spec-format/hooks.md#assumed-results). |
| `--format text\|json` | `json` prints one object per tool with its result, warnings, and each command's exit code, time, stdout, and stderr. Defaults to `text`. |

## install-hook

Install git hooks. By default it installs a pre-commit hook that runs `sync --check --against index`, so a commit fails if regenerated files are not staged. With `--post-checkout`, it installs hooks that regenerate the tool files after a checkout or a pull. See [git hooks](@/docs/git-hooks.md).

```bash
agnostic-ai install-hook            # writes .git/hooks/pre-commit (local)
agnostic-ai install-hook --shared   # writes .githooks/ and sets core.hooksPath
agnostic-ai install-hook --global   # gates commits to a global home kept in git

agnostic-ai install-hook --post-checkout            # writes .git/hooks/post-checkout and post-merge
agnostic-ai install-hook --post-checkout --shared   # writes both hooks in .githooks/
```

An existing hook keeps its content, and the checks go at its end. A hook that would stop before reaching them is left alone, and the command prints the lines to add by hand. A hook stops early when it has no `sh` or `bash` shebang, an `exec`, or an unindented `exit`.

- `--shared` writes `.githooks/<hook>` at the root of the main working tree, even from a linked worktree. It stops if `core.hooksPath` already points elsewhere.
- `--global` is for the global home, which must be the root of its own git repository. The hook runs `lint --global --strict`, `validate --global`, and `sync --global --check` (skipped in a linked worktree), and the commit fails if any fails. It cannot combine with `--shared` or `--post-checkout`, and it stops when `core.hooksPath` points elsewhere or the hook already runs the project `sync --check`.
- `--post-checkout` installs `post-checkout` and `post-merge`. They run `agnostic-ai sync -q` from the worktree root after a branch or worktree checkout (not a single-file checkout) or a merge, including a pull. Both skip if the binary or `agnostic-ai.yaml` is missing.

## completion

Generate a shell completion script.

```bash
agnostic-ai completion bash > ~/.local/share/bash-completion/completions/agnostic-ai  # or /etc/bash_completion.d/
agnostic-ai completion zsh > "${fpath[1]}/_agnostic-ai"
agnostic-ai completion fish > ~/.config/fish/completions/agnostic-ai.fish
agnostic-ai completion powershell | Out-String | Invoke-Expression
```

Restart your shell or `source` the file. Completing `--target` reads `agnostic-ai.yaml` in the current directory, or offers every target if there is none.

## upgrade

Upgrade the running binary to the latest release, using the method you installed it with. `update` is an alias.

```bash
agnostic-ai upgrade --version v0.56.1
```

| Flag | Description |
|------|-------------|
| `--check` | Print install details and change nothing. With `--version`, adds a `Requested:` line. |
| `--version <tag>` | Install one release, including an older one. The leading `v` is optional. Standalone binaries only: for a package-manager install, pin the version through that manager. |
| `--requires` | Set this project's `requires` and schema tag to the installed release, then sync. |
| `--run` | Accepted for compatibility; upgrading is the default. |

After a package-manager upgrade, run its installed CLI from the project root:

```bash
pnpm exec agnostic-ai upgrade --requires
```

`--requires` replaces a minimum, range, or older pin with the installed exact release. It updates the base config and any existing local `requires` override, including a null override. Comments and other settings stay. If sync fails, the new pins stay: fix the problem it reports and run `sync` again.

This mode needs a stable release build. It works with `agnostic.config.yaml` too. It cannot combine with `--check`, `--version`, or `--run`, and it does not update the global home config. Your package manager owns install and dependency files.

Before you run it, rewrite flow-style root mappings, merged root keys, and anchored or multiline `requires` values as a plain block mapping and a single-line value.

| Binary location | Upgrade |
|-----------------|---------|
| `*/Cellar/*`, `*/Caskroom/*`, `/opt/homebrew/*`, `/home/linuxbrew/.linuxbrew/*` | `brew update && brew upgrade --cask Chemaclass/tap/agnostic-ai` |
| `$GOBIN` or `$GOPATH/bin` (defaults to `$HOME/go/bin`) | `go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@latest` |
| `*\scoop\apps\*`, `*\scoop\shims\*` | `scoop update agnostic-ai` |
| `*\Microsoft\WinGet\*` | `winget upgrade Chemaclass.agnostic-ai` |
| `*/node_modules/*` | `npm install -g agnostic-ai@latest` |
| Standalone binary on macOS or Linux | `upgrade` downloads the release, checks it, and replaces the binary. |
| Standalone binary on Windows | Use the [PowerShell install script](@/docs/installation.md). Windows cannot replace a running program. |

The Scoop, WinGet, and `node_modules` paths match in any letter case. `upgrade` also lists any other `agnostic-ai` on `PATH` that hides the one it found.

## migrate

Rewrite old spec forms into their current replacements, such as a renamed field or file. Sync writes the same files afterward, so `sync --check` stays clean. The one exception is a literal MCP credential, which becomes a `${NAME}` reference. Old forms keep working, so you never have to run it before a sync. `doctor` and `upgrade --requires` name the migrations that apply.

```bash
agnostic-ai migrate --list      # which migrations apply here
agnostic-ai migrate --dry-run   # preview the rewrites
agnostic-ai migrate             # apply them
agnostic-ai migrate --global    # rewrite the global specs
```

| Flag | Description |
|------|-------------|
| `--dry-run` | Print each rename and a diff of each rewrite, and write nothing. Every `env` and `headers` value, `args` item, URL, and credential-like value prints as `<redacted>`. A `${NAME}` reference and a `!literal` tag still show. |
| `--list` | List every migration with its release and whether it applies here. It names each pack whose author needs to update its specs. |
| `--only <group>` | Run only these groups, comma-separated. A migration ID starts with its group, such as `config-file-name` in group `config`. |
| `--global` | Rewrite the specs in `$AGNOSTIC_AI_HOME` (default `~/.agnostic-ai`) and its `local/` folder. |

- A migration rewrites only what maps one to one. Anything else stays as written, and the output says why.
- When `secrets-mcp-literals` turns a credential into a reference, its output names each variable to set, never the value.
- A symlinked spec keeps its symlink; the file it points to gets the rewrite.
- It never rewrites a pack. A spec in a pack, or a symlink into one, is skipped, and the output names the pack. So is any file outside the project, or outside the global home with `--global`.
- A migration that cannot plan, for example on a spec that does not parse, prints `cannot plan` with the reason. The others still run, and `migrate` exits 1.
- `doctor` names a migration only when it rewrites something or a skip needs your action, such as a spec that sets both the old and the new form. `--dry-run` and `--list` show every skip.

| Migration | Release | Rewrites |
|-----------|---------|----------|
| `config-file-name` | 0.79.0 | `agnostic.config.yaml` to `agnostic-ai.yaml`, in a project only. When both exist with the same content, it removes the old file. Skipped when they differ, and when Git ignores `agnostic-ai.yaml`. |
| `hooks-portable-events` | 0.79.0 | A hook's `event` and `matcher` to the [portable](@/docs/spec-format/hooks.md#portable-events) `on` and `match`, such as `PreToolUse` on `Bash` to `before-tool` on `shell`. Comments, quoting, and key order stay. Skipped when the portable form would give a tool another event or matcher (such as `matcher: Read` on Codex, or `matcher: Edit\|Write` on Claude Code, since `match: edit` also covers `MultiEdit` and `NotebookEdit`), when a `local/` spec extends the hook, and for a pack's hook. |
| `capabilities-agent-tools`, `capabilities-settings-permissions` | 0.79.0 | Agent `tools` to `can`, and Claude Code aliases in settings permissions to neutral capabilities. An adjacent `WebFetch, WebSearch` pair becomes `web`. Names with no neutral form stay as aliases. Run with `--only capabilities`. |
| `capabilities-skill-tools` | 0.79.0 | Claude Code aliases in skill `allowed-tools` to neutral capabilities, with the same `web` pair rule. Run with `--only capabilities`. |
| `secrets-mcp-literals` | 0.79.0 | Each MCP `env` and `headers` value that is neither a reference nor marked. A value import reads as a credential becomes a `${NAME}` reference, named as import names it. Every other value gets [`!literal`](@/docs/spec-format/mcps.md#plain-settings). Skipped for a key with a credential name whose value has no credential shape, a credential around a reference, and a pack's spec. Run with `--only secrets`. |

## lsp

Start the language server on stdin and stdout. Point your editor at `agnostic-ai lsp` for spec files (`.agnostic-ai/**/*.md`, `*.mdc`). It reports lint problems when you open or save a file.

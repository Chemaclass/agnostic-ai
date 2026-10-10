+++
title = "Maintain and set up"
description = "Revert or clean generated files, share packs, and set up hooks, completion, and updates."
weight = 50

[extra]
group = "Reference"
+++

# Maintain and set up

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

## memory

Check, list, locate, and repair the [shared memory](@/docs/memory.md) in the working directory. Each subcommand reads project memory in `.agnostic-ai/memory/`, then personal memory in `.agnostic-ai/local/memory/`, or in the [personal folder shared across worktrees](@/docs/memory.md#one-store-per-repository) with `memory.personal: repo`. Missing memory folders are skipped.

```bash
agnostic-ai memory lint     # run only the memory checks
agnostic-ai memory index    # rebuild each MEMORY.md from its fact files
agnostic-ai memory list     # print each fact's scope, type, and title
agnostic-ai memory path     # print each memory folder's absolute path
```

- `memory lint` reports [LINT039 to LINT042](@/docs/cli-reference/check.md#lint), the memory findings `lint` and `doctor` also report. It exits 1 on an error, or on a warning with `--strict`. `--json` prints the same format as `lint --json`, with `command` set to `memory lint`.
- `memory index` drops merge conflict markers, links to missing files, and repeated links. Every other line stays in place, headings and notes included. It then adds a `- [name](file.md): description` line from the YAML fields at the top of each fact without one. Run it after two tools edit the index at once.
- The type comes from `metadata.type`, or from a top-level `type` as Claude Code's auto memory writes it.
- With no memory folder, `memory lint` says so and exits 0.
- `memory list` prints facts in index order. The title is the index line's link text, or the fact's `name` when no line links it.
- `memory path` prints one `scope  folder` line per memory folder, project first, as absolute paths. It works from any folder inside the project and from a linked worktree, and prints a folder that does not exist yet. With `memory.personal: repo`, every worktree of the repository gets the same personal folder. The [`shared-memory-policy` rule](@/docs/memory.md#how-each-tool-loads-it) has tools without a session-start hook run it.

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

## hook worktree-remove

Remove one clean Git worktree from a Claude Code `WorktreeRemove` event on stdin. Both flags are required: `--repo` selects a surviving checkout of the repository, and `--allowed-root` selects the directory whose descendants you permit the helper to remove. Quote paths that contain spaces.

```bash
agnostic-ai hook worktree-remove --repo "/path/to/repository" --allowed-root "/path/to/disposable worktrees"
```

The helper verifies Git's worktree registration and the checkout's repository metadata before running `git worktree remove` without force. It refuses the main checkout, the allowed root itself, other repositories, unregistered directories, traversal, symlinks below the allowed root, locked worktrees, and worktrees with changed, untracked, or ignored files. It also refuses indexes with `assume-unchanged` or `skip-worktree` entries, including sparse checkouts, because those flags can hide changed file contents. Ignored dependencies also prevent removal; remove them deliberately before retrying. It keeps the branch.

According to [Claude Code's WorktreeRemove contract](https://code.claude.com/docs/en/hooks#worktreeremove), this event replaces cleanup for hook-created worktrees and runs while the directory still exists. Exit 0 means cleanup succeeded; a nonzero exit preserves an existing directory without a Git fallback. Pair this helper with your `WorktreeCreate` hook, and keep `--repo` outside the disposable checkout. It does not change cleanup for worktrees the host creates without a custom creation hook.

Success prints `removed: <path>` and exits 0. A path already absent below the allowed root prints `already absent: <path>` and exits 0, including repeated calls after successful removal. Absence alone cannot prove prior ownership; this result performs no deletion or registration pruning. Unsafe inputs, payloads larger than 1 MiB, and failed Git operations exit 1.

| Flag | Description |
|------|-------------|
| `--repo <path>` | Required. A surviving checkout whose registered worktree may be removed. |
| `--allowed-root <path>` | Required. Only descendants of this directory may be removed. |

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
- With no project, it prints nothing. With `memory.personal: repo`, it names the personal folder even before an index exists; in checkout mode, no index means no output. It finds the project the same way the `shared-memory` skill does.

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

## project

Run the project's installed binary with its `requires` contract. The helper prefers `node_modules/.bin/agnostic-ai` over `PATH`. A project declaring an agnostic-ai dependency must have its local binary installed; a global binary does not silently replace it. On Windows, the helper runs the installed Node shim directly.

```bash
agnostic-ai project                     # sync --keep-edits --quiet
agnostic-ai project --check             # sync --check, no install or writes
agnostic-ai project --check --against index
agnostic-ai project --bootstrap         # repair dependencies once, then sync
agnostic-ai project -- hook memory --target codex
```

The helper finds the nearest project config at or above the working directory. A built-in hook can use the host's project-directory environment variable instead. The version probe must satisfy `requires`; failures name the installed version, required version, and `agnostic-ai project --bootstrap` recovery command. Bootstrap does not modify the requirement or package manifest, so the committed lockfile must install an allowed version.

| Flag | Description |
|------|-------------|
| `--check` | Check the version and generated output without installing or writing. |
| `--against index\|HEAD` | With `--check`, use the existing staged or committed output gate. |
| `--bootstrap` | If the binary is missing or mismatched, run one locked npm or pnpm install, then sync preserving manual edits. |

Bootstrap uses `packageManager` from `package.json`, or an unambiguous root lockfile. It runs `npm ci` or `pnpm install --frozen-lockfile`. Both require a committed lockfile and a declared agnostic-ai dependency. Other package managers must install explicitly before invoking the helper. Normal package scripts run, including hook-manager setup and native builds. `AGNOSTIC_AI_PROJECT_BOOTSTRAP=1` prevents a recursive helper call from starting another install; postinstall can still regenerate output. Repeating bootstrap with the correct binary performs no install.

Existing hook managers can call this helper without changing their own configuration format. A bootstrap entry point must already be installed, globally or locally. Versions released before `project` existed must first be updated through their existing package-manager command.

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

## upgrade

Update agnostic-ai using the method you installed it with. `update` is an alias.

```bash
agnostic-ai upgrade
agnostic-ai upgrade --check              # show install details without updating
agnostic-ai upgrade --version v0.81.0    # install a specific release
```

### Automatic upgrade offers

When you run a command in a terminal, a stable release build checks for updates at most once a day. Release checks have a five-second network limit. The offer shows release notes and manual steps for each version you skipped. If docs are missing, it names them and shows changelog entries when available.

Press Enter or type `y` to accept. Type `n` to continue your original command; the offer will not repeat that day.

Before asking, the offer explains what it will change. In a project, accepting it:

1. Installs the new release and checks that the new executable has the expected version.
2. Sets the project's required version (`requires`) to that exact release and updates the editor schema link. This replaces a minimum or range too, including an existing value in `agnostic-ai.local.yaml`.
3. Previews and applies the built-in changes to old spec files.
4. Runs `sync`, then `sync --check`.

Outside a project, it updates the tool only. After accepting, rerun your original command with the updated tool. Review any skipped changes and release notes for steps you must do yourself. Release notes are shown for guidance; they are not run as commands.

If a config file, `.agnostic-ai`, or `.agnostic-ai/local` links to another project or the global specs folder, the offer is skipped and names that location. Links within the project are supported.

For project-local npm or pnpm installs, the offer prints the package manager's update command. For npx, it prints a command with the new version. Run that command yourself. Automatic npm updates are limited to confirmed global installs; that check has a two-second limit.

If installation or version checking fails, project changes do not start. If a later step fails, the tool stops and prints how to continue. Changes already completed stay in place.

Set `AGNOSTIC_AI_NO_UPDATE_CHECK=1` to disable these checks. Offline checks stay silent. Checks also skip CI, redirected input or output, JSON output, quiet mode, global commands, checks, previews, hooks, completion, editor services, and development builds. You can still run `upgrade` yourself.

### Upgrade options

| Flag | Description |
|------|-------------|
| `--check` | Show install details without changing anything. With `--version`, also show `Requested:`. |
| `--version <tag>` | Install a specific release, including an older one. The leading `v` is optional. For package-manager installs, choose the version through that manager instead. |
| `--requires` | Set this project's required version and editor schema link to the installed release, then sync. |
| `--run` | Accepted for compatibility; upgrading is already the default. |

After updating through a package manager, run its installed CLI from the project root:

```bash
pnpm exec agnostic-ai upgrade --requires
```

`--requires` replaces a minimum, range, or older version with the exact installed release. It updates the base config and any existing local `requires` value, including `null`. Comments and other settings stay. If sync fails, the version changes stay: fix the reported problem and run `sync` again.

This option needs a stable release build and also supports `agnostic.config.yaml`. It cannot combine with `--check`, `--version`, or `--run`. It leaves the global config, package-manager files, and dependency files to their own update commands.

The config must have one root setting per line and a single-line `requires` value. If it uses `{...}` at the root, YAML `<<` merges, or a `requires` value shared through `&`/`*` or spread across several lines, rewrite those parts first.

| Binary location | Upgrade |
|-----------------|---------|
| `*/Cellar/*`, `*/Caskroom/*`, `/opt/homebrew/*`, `/home/linuxbrew/.linuxbrew/*` | `brew update && brew upgrade --cask Chemaclass/tap/agnostic-ai` |
| `$GOBIN` or `$GOPATH/bin` (defaults to `$HOME/go/bin`) | `go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@latest` |
| `*\scoop\apps\*`, `*\scoop\shims\*` | `scoop update agnostic-ai` |
| `*\Microsoft\WinGet\*` | `winget upgrade Chemaclass.agnostic-ai` |
| `*/node_modules/*` | `npm install -g agnostic-ai@latest` |
| Standalone binary on macOS or Linux | `upgrade` downloads the release, checks it, and replaces the binary. |
| Standalone binary on Windows | Use the [PowerShell install script](@/docs/installation.md). Windows cannot replace a running program. |

The Scoop, WinGet, and `node_modules` paths match in any letter case. `upgrade` also reports other copies on `PATH` that would run instead of the one it found.

## migrate

Update spec files that use old field names or formats. Sync writes the same files afterward, so `sync --check` stays clean. The one exception is a literal MCP credential, which becomes a `${NAME}` reference. Old forms keep working, so you never have to run it before a sync. `doctor` and `upgrade --requires` name the migrations that apply.

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

- A migration rewrites only what it can replace without changing its meaning. Anything else stays as written, and the output says why.
- When `secrets-mcp-literals` turns a credential into a reference, its output names each variable to set, never the value.
- A symlinked spec keeps its symlink; the file it points to gets the rewrite.
- It never rewrites a pack or a spec linked into one. The output names the pack.
- Migrations stay within the project and its local specs folder. With `--global`, they stay within the global specs folder and its local folder. A linked local folder is included too.
- A migration that cannot plan, for example on a spec that does not parse, prints `cannot plan` with the reason. The others still run, and `migrate` exits 1.
- `doctor` names a migration only when it rewrites something or a skip needs your action, such as a spec that sets both the old and the new form. `--dry-run` and `--list` show every skip.

| Migration | Release | Rewrites |
|-----------|---------|----------|
| `config-file-name` | 0.79.0 | `agnostic.config.yaml` to `agnostic-ai.yaml`, in a project only. When both exist with the same content, it removes the old file. Skipped when they differ, and when Git ignores `agnostic-ai.yaml`. |
| `hooks-portable-events` | 0.79.0 | A hook's `event` and `matcher` to the [portable](@/docs/spec-format/hooks.md#portable-events) `on` and `match`, such as `PreToolUse` on `Bash` to `before-tool` on `shell`. Comments, quoting, and key order stay. Skipped when the portable form would give a tool another event or matcher (such as `matcher: Read` on Codex, or `matcher: Edit\|Write` on Claude Code, since `match: edit` also covers `MultiEdit` and `NotebookEdit`), when a `local/` spec extends the hook, and for a pack's hook. |
| `capabilities-agent-tools`, `capabilities-settings-permissions` | 0.79.0 | Agent `tools` to `can`, and Claude Code permission names to names shared across tools. An adjacent `WebFetch, WebSearch` pair becomes `web`. Names without a shared form stay as written. Run with `--only capabilities`. |
| `capabilities-skill-tools` | 0.79.0 | Claude Code names in skill `allowed-tools` to names shared across tools, with the same `web` pair rule. Run with `--only capabilities`. |
| `secrets-mcp-literals` | 0.79.0 | Plain MCP `env` and `headers` values. Values recognized as credentials become `${NAME}` references, using the same variable names as `import`. Other plain values get [`!literal`](@/docs/spec-format/mcps.md#plain-settings). Existing references and marked values stay. Skips keys with credential names whose values do not look like credentials, credentials mixed with a reference, and specs in packs. Run with `--only secrets`. |

## lsp

Start the language server on stdin and stdout. The [VS Code extension](https://github.com/Chemaclass/agnostic-ai/tree/main/editors/vscode) starts it automatically for a configured workspace, using `agnostic-ai.binaryPath`. Other editors can launch `agnostic-ai lsp` for Markdown and YAML files. It reads saved files and reports lint problems when you open or save a file. Config and spec load failures appear on the source file, with the parser location when available. A failure with no known source appears as a workspace error.

After a successful check, resolved diagnostics clear from every affected file, including deleted sources. If loading fails, earlier diagnostics stay until a successful check replaces them. Fix the reported problem and save a project file to retry.

The server checks the selected project's configured sources, including custom and absolute directories. Unsaved buffer edits do not run lint. Files keep their ordinary Markdown or YAML editing support.

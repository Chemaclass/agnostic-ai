+++
title = "Upgrade and migrate"
description = "Update the CLI release and rewrite older source formats."
weight = 57

[extra]
group = "Reference"
+++

# Upgrade and migrate

## upgrade

Update agnostic-ai using the method you installed it with. `update` is an alias.

```bash
agnostic-ai upgrade
agnostic-ai upgrade --check              # show install details without updating
agnostic-ai upgrade --version v0.81.0    # install a specific release
```

### Automatic upgrade offers

When you run a command in a terminal, a stable release build checks for updates at most once a day. Release checks have a five-second network limit. The offer is two lines, the version change and a link to the release page, then the prompt:

```text
Update available: 0.83.0 -> 0.85.0
Release guidance: https://github.com/Chemaclass/agnostic-ai/releases/tag/v0.85.0
Update now? [Y/n]
```

Press Enter or type `y` to accept. Type `n` to continue your original command; the offer will not repeat that day.

In a project, accepting it:

1. Installs the new release and checks that the new executable has the expected version.
2. Sets the project's required version (`requires`) to that exact release and updates the editor schema link. This replaces a minimum or range too, including an existing value in `agnostic-ai.local.yaml`.
3. Previews and applies the built-in changes to old spec files.
4. Runs `sync`, then `sync --check`.

Outside a project, it updates the tool only. After accepting, rerun your original command with the updated tool. Read the release page for steps you must do yourself.

If a config file, `.agnostic-ai`, or `.agnostic-ai/local` links to another project or the global specs folder, the offer is skipped and names that location. Links within the project are supported.

For project-local npm or pnpm installs, the offer prints the package manager's update command. For npx, it prints a command with the new version. Run that command yourself. Automatic npm updates are limited to confirmed global installs; that check has a two-second limit.

If installation or version checking fails, project changes do not start. If a later step fails, the tool stops and prints how to continue. Changes already completed stay in place.

Set `AGNOSTIC_AI_NO_UPDATE_CHECK=1` to disable these checks. Offline checks stay silent. Checks also skip CI, redirected input or output, JSON output, quiet mode, global commands, checks, previews, hooks, completion, editor services, and development builds. Manual `upgrade` remains available.

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

See [credential previews](@/docs/cli-reference/sync.md#credential-previews) for shared reference and display rules.

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
| `hooks-portable-events` | 0.79.0 | A hook's `event` and `matcher` to the [portable](@/docs/spec-format/hook-reference.md#portable-events) `on` and `match`, such as `PreToolUse` on `Bash` to `before-tool` on `shell`. Comments, quoting, and key order stay. Skipped when the portable form would give a tool another event or matcher (such as `matcher: Read` on Codex, or `matcher: Edit\|Write` on Claude Code, since `match: edit` also covers `MultiEdit` and `NotebookEdit`), when a `local/` spec extends the hook, and for a pack's hook. |
| `capabilities-agent-tools`, `capabilities-settings-permissions` | 0.79.0 | Agent `tools` to `can`, and Claude Code permission names to names shared across tools. An adjacent `WebFetch, WebSearch` pair becomes `web`. Names without a shared form stay as written. Run with `--only capabilities`. |
| `capabilities-skill-tools` | 0.79.0 | Claude Code names in skill `allowed-tools` to names shared across tools, with the same `web` pair rule. Run with `--only capabilities`. |
| `secrets-mcp-literals` | 0.79.0 | Plain MCP `env` and `headers` values. Values recognized as credentials become `${NAME}` references, using the same variable names as `import`. Other plain values get [`!literal`](@/docs/spec-format/mcps.md#plain-settings). Existing references and marked values stay. Skips keys with credential names whose values do not look like credentials, credentials mixed with a reference, and specs in packs. Run with `--only secrets`. |

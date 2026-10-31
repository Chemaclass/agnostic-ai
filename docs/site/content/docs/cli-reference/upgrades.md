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

`upgrade` updates the installed tool. If the latest release is already installed but the project still requires an older one, the message points to `agnostic-ai upgrade --requires`. That command updates the project requirement and schema URL, then runs sync.

### Automatic upgrade offers

In a terminal, a stable release build checks for updates at most once a day, with a five-second network limit, and offers:

```text
Update available: 0.83.0 -> 0.85.0
Release guidance: https://github.com/Chemaclass/agnostic-ai/releases/tag/v0.85.0
Update now? [Y/n]
```

Enter or `y` accepts. `n` runs your original command and silences the offer for the day. In a project, accepting:

1. Installs the new release and checks the new executable's version.
2. Raises `requires` to that release, keeping its form, and updates the editor schema link, as [`--requires`](#upgrade-options) does.
3. Previews and applies the [migrations](#migrate) to old spec files.
4. Runs `sync`, then `sync --check`.

Outside a project, it updates only the tool. Then rerun your command; the release page lists manual steps.

- A failed install or version check starts no project change. A later failed step stops, keeps what finished, and prints how to continue.
- If a config file, `.agnostic-ai`, or `.agnostic-ai/local` links to another project or the global specs folder, the offer is skipped and names it. Links within the project are fine.
- Project-local npm or pnpm installs, and npx, get a command to run yourself. Automatic npm updates apply only to confirmed global installs (a two-second check).
- `AGNOSTIC_AI_NO_UPDATE_CHECK=1` turns the checks off. They are silent offline and skip CI, redirected input or output, JSON output, quiet mode, global commands, checks, previews, hooks, completion, editor services, and development builds.

### Upgrade options

| Flag | Description |
|------|-------------|
| `--check` | Show install details without changing anything. With `--version`, also show `Requested:`. |
| `--version <tag>` | Install a specific release, including an older one. The leading `v` is optional. For package-manager installs, choose the version through that manager instead. |
| `--requires` | Raise this project's `requires` to the installed release, keeping its form, update the editor schema link, then sync. See below. |
| `--run` | Accepted for compatibility; upgrading is already the default. |

After updating through a package manager, run its installed CLI from the project root, as in `pnpm exec agnostic-ai upgrade --requires`.

`--requires` changes the base config and any existing local `requires`, keeping comments and other settings. With 0.91.0 installed:

| Current value | New value |
|---------------|-----------|
| missing, empty, or `null` | `">=0.91.0"` |
| `">=0.90.0"` | `">=0.91.0"` |
| `"0.90.0"` | `"0.91.0"` |
| `"=0.90.0"` | `"=0.91.0"` |
| a range or upper bound, such as `">=0.90.0 <0.91.0"` or `"<0.91.0"` | `">=0.91.0"` |

A missing key is added only to the base config. When a range becomes a minimum, the command prints the value it replaced. If sync fails, the version changes stay: fix the problem and run `sync` again. It needs a stable release build, also reads `agnostic.config.yaml`, and does not combine with `--check`, `--version`, or `--run`. The global config and package-manager files are left to their own commands. The config needs one root setting per line and a single-line `requires`; rewrite a root `{...}`, YAML `<<` merges, or a `requires` shared through `&`/`*` or split across lines first.

| Binary location | Upgrade |
|-----------------|---------|
| `*/Cellar/*`, `*/Caskroom/*`, `/opt/homebrew/*`, `/home/linuxbrew/.linuxbrew/*` | `brew update && brew upgrade --cask Chemaclass/tap/agnostic-ai` |
| `$GOBIN` or `$GOPATH/bin` (defaults to `$HOME/go/bin`) | `go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@latest` |
| `*\scoop\apps\*`, `*\scoop\shims\*` | `scoop update agnostic-ai` |
| `*\Microsoft\WinGet\*` | `winget upgrade Chemaclass.agnostic-ai` |
| `*/node_modules/*` | `npm install -g agnostic-ai@latest` |
| Standalone binary on macOS or Linux | `upgrade` downloads the release, checks its checksum and its [build provenance](@/docs/verify-a-release.md#build-provenance), and replaces the binary. |
| Standalone binary on Windows | Use the [PowerShell install script](@/docs/installation.md). Windows cannot replace a running program. |

The Scoop, WinGet, and `node_modules` paths match in any letter case. `upgrade` also reports other copies on `PATH` that would run instead of the one it found.

## migrate

Update spec files that use old field names or formats. Old forms keep working, and sync writes the same files afterward, except that a literal MCP credential becomes a `${NAME}` reference. `doctor` and `upgrade --requires` name the migrations that apply.

```bash
agnostic-ai migrate --list      # which migrations apply here
agnostic-ai migrate --dry-run   # preview the rewrites
agnostic-ai migrate             # apply them
agnostic-ai migrate --global    # rewrite the global specs
```

| Flag | Description |
|------|-------------|
| `--dry-run` | Print each rename and a diff of each rewrite, and write nothing. Every `env` and `headers` value, `args` item, URL, and credential-like value prints as `<redacted>`; a `${NAME}` reference and a `!literal` tag still show. See [credential previews](@/docs/cli-reference/sync.md#credential-previews). |
| `--list` | List every migration with its release and whether it applies here. It names each pack whose author needs to update its specs. |
| `--only <group>` | Run only these groups, comma-separated. A migration ID starts with its group, such as `config-file-name` in group `config`. |
| `--global` | Rewrite the specs in `$AGNOSTIC_AI_HOME` (default `~/.agnostic-ai`) and its `local/` folder. |

- A migration rewrites only what keeps its meaning; anything else stays, with the reason. `secrets-mcp-literals` names each variable to set, never the value.
- A symlinked spec keeps its link, and its target gets the rewrite. A pack, or a spec linked into one, is never rewritten; the output names the pack.
- Migrations stay within the project (or, with `--global`, the global specs folder) and its local folder, even a linked one.
- A migration that cannot plan, such as on a spec that does not parse, prints `cannot plan` with the reason; the others run, and `migrate` exits 1.
- `doctor` names a migration only when it rewrites something or a skip needs you, such as a spec setting both old and new forms. `--dry-run` and `--list` show every skip.

| Migration | Release | Rewrites |
|-----------|---------|----------|
| `config-file-name` | 0.79.0 | `agnostic.config.yaml` to `agnostic-ai.yaml`, in a project only. When both exist with the same content, it removes the old file; it skips when they differ or Git ignores `agnostic-ai.yaml`. |
| `hooks-portable-events` | 0.79.0 | A hook's `event` and `matcher` to the [portable](@/docs/spec-format/hook-reference.md#portable-events) `on` and `match`, such as `PreToolUse` on `Bash` to `before-tool` on `shell`. Comments, quoting, and key order stay. Skipped when the portable form would change a tool's event or matcher (such as `matcher: Read` on Codex, or `matcher: Edit\|Write` on Claude Code, since `match: edit` also covers `MultiEdit` and `NotebookEdit`), when a `local/` spec extends the hook, and for a pack's hook. |
| `capabilities-agent-tools`, `capabilities-settings-permissions`, `capabilities-skill-tools` | 0.79.0 | Agent `tools` to `can`, and Claude Code names in permissions and skill `allowed-tools` to names shared across tools. An adjacent `WebFetch, WebSearch` pair becomes `web`; names without a shared form stay. Run with `--only capabilities`. |
| `secrets-mcp-literals` | 0.79.0 | Plain MCP `env` and `headers` values: credentials become `${NAME}` references, named as `import` names them, and other values get [`!literal`](@/docs/spec-format/mcps.md#plain-settings). Skips keys with credential names whose values do not look like credentials, credentials mixed with a reference, and pack specs. Run with `--only secrets`. |

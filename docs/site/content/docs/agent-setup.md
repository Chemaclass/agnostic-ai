+++
title = "Set up agnostic-ai with a coding agent"
description = "Paste one prompt into your coding agent, and it sets agnostic-ai up in about two minutes."
weight = 15

[extra]
group = "Start"
+++

# Set up agnostic-ai with a coding agent

{{ <agent_setup_prompt /> }}

## What the agent does

Follow this when a user asks you to install or configure agnostic-ai in a repository. Goal: keep existing AI tool behavior, make `.agnostic-ai/` the source of truth, and finish with a clean `sync --check`.

### Safety contract

- Work from the repository root. Read its contributor and agent instructions first.
- Run `git status --short`. Never discard user changes.
- Look for existing instructions files such as `CLAUDE.md`, `AGENTS.md`, and `GEMINI.md`. Tell the user what they hold and ask whether to import them before any sync.
- Import existing native configuration before the first sync. Sync writes output; it does not migrate.
- Enable only the targets the project uses or the user names. Never select every target.
- Preview before writing. On an ownership or overwrite error, stop and name the path.
- After setup, edit `.agnostic-ai/` only. Native files are generated.

### 1. Install or verify the CLI

```bash
agnostic-ai --version
```

If missing, install it:

```bash
# macOS or Linux
curl -fsSL https://raw.githubusercontent.com/Chemaclass/agnostic-ai/main/scripts/install.sh | bash
```

```powershell
# Windows PowerShell
irm https://raw.githubusercontent.com/Chemaclass/agnostic-ai/main/scripts/install.ps1 | iex
```

Run `--version` again. If the command is not found, add the install directory to `PATH`. See [Installation](@/docs/installation.md) for other methods.

### 2. Detect the project state

| State | Signal | Action |
|---|---|---|
| Already configured | `agnostic-ai.yaml` and `.agnostic-ai/` exist | Skip `init`. Go to step 4. |
| Existing tool config | `CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, `.cursor/`, `.github/copilot-instructions.md` | Import during `init`. |
| Fresh | Neither | Plain `init`. |

If the target list is unclear, ask the user. A tool installed on the machine does not mean the repository uses it.

### 3. Initialize

Ask the user whether Git should track generated files. Pass `--gitignore=on` (ignore them; teammates run `sync` after cloning) or `--gitignore=off` (commit them; CI runs `sync --check`).

Replace `claude,codex` with the project's tools:

```bash
# existing tool config
printf '%s\n' 'claude,codex' | agnostic-ai init --from all --gitignore=on

# fresh project
printf '%s\n' 'claude,codex' | agnostic-ai init --gitignore=on
```

`--from all` adds each source's missing sections to `.agnostic-ai/AGNOSTIC_AI.md` and names them. Review that file for duplicates before you sync. Import tells you when a section needs a manual merge. Details: [Migration](@/docs/migration.md).

Do not use `--demo` unless the user asks. Add rules only from conventions the repository or user already states.

### 4. Validate and preview

```bash
agnostic-ai validate
agnostic-ai lint
agnostic-ai sync --dry-run
```

Check that targets, output paths, and kept instructions match the project. Fix errors in the specs. Understand an unsupported-capability warning before you silence it.

### 5. Sync and prove the result

```bash
agnostic-ai sync
agnostic-ai sync --check
git status --short
git diff -- .
```

Inspect generated files and `.gitignore`. If outputs are committed, keep them in the same change as their specs.

Report:

- agnostic-ai version and install method;
- targets and imported sources;
- what each tool now reads from `.agnostic-ai/`, as the first `agnostic-ai sync` lists it;
- files changed under `.agnostic-ai/`;
- generated outputs and whether Git tracks them;
- results of `validate`, `lint`, and `sync --check`;
- open decisions for the user.

Setup is done only when `agnostic-ai sync --check` exits 0.

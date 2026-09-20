+++
title = "Set up agnostic-ai with a coding agent"
description = "A safe workflow for a coding agent to set up agnostic-ai in any repository."
weight = 15

[extra]
group = "Start"
+++

# Set up agnostic-ai with a coding agent

Follow this when a user asks you to install or configure agnostic-ai in a repository. Preserve existing AI tool behavior, make `.agnostic-ai/` the source of truth, and leave the project with a clean validation result.

{{ <agent_setup_prompt /> }}

## Safety contract

- Work from the repository root. Read its contributor and agent instructions first.
- Check `git status --short`. Preserve unrelated work and never discard user changes.
- Inspect `agnostic-ai.yaml`, `.agnostic-ai/`, and the native configuration for tools the project already uses.
- Import existing native configuration before the first sync. Sync is an output operation, not a migration shortcut.
- Enable only the targets the project uses or the user requests. Never select every target by default.
- Preview changes before writing. Stop on an ownership or overwrite error and explain the conflicting path.
- Treat `.agnostic-ai/` as canonical after setup. Native target files are generated outputs.

## 1. Install or verify the CLI

Start with:

```bash
agnostic-ai --version
```

If the command is missing, use the platform installer.

macOS or Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/Chemaclass/agnostic-ai/main/scripts/install.sh | bash
```

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/Chemaclass/agnostic-ai/main/scripts/install.ps1 | iex
```

Run `agnostic-ai --version` again. If the shell cannot find the binary, add the installer destination to `PATH`. See [Installation](@/docs/installation.md) for pinned versions and other methods.

## 2. Detect the project state

Choose one path:

- **Already configured:** `agnostic-ai.yaml` and `.agnostic-ai/` exist. Do not run `init` again. Review the configured targets and go to validation.
- **Existing native AI configuration:** files such as `CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, `.cursor/`, or `.github/copilot-instructions.md` exist, but agnostic-ai is not configured. Import them during initialization.
- **Fresh setup:** no canonical or native AI configuration exists. Initialize only the targets the project will use.

When target choice is ambiguous, ask the user. Do not infer that every installed CLI belongs in this repository.

## 3. Initialize safely

For existing native configuration, replace the example target list with the tools the project uses:

```bash
printf '%s\n' 'claude,codex' | agnostic-ai init --from all
```

`--from all` imports every detected source. The last imported top-level file wins, so when several tools carry different top-level instructions, review `.agnostic-ai/AGNOSTIC_AI.md` and merge the useful content before syncing.

For a fresh project:

```bash
printf '%s\n' 'claude,codex' | agnostic-ai init
```

Do not use `--demo` in a real repository unless the user asks for example specs. Add project rules only from conventions already in the repository or supplied by the user. See [Getting started](@/docs/getting-started.md) for the spec workflow and [Migration](@/docs/migration.md) for import behavior.

## 4. Validate and preview

Run these commands in order:

```bash
agnostic-ai validate
agnostic-ai lint
agnostic-ai sync --dry-run
```

Confirm the preview's targets, output paths, and preserved instructions match the project. Fix validation errors in the canonical specs. Do not silence unsupported-capability warnings until you understand their effect.

## 5. Sync and prove the result

```bash
agnostic-ai sync
agnostic-ai sync --check
git status --short
git diff -- .
```

Inspect the generated files and `.gitignore` changes. The default setup ignores generated outputs, so a fresh clone must run `agnostic-ai sync`. If the project commits generated outputs instead, keep them in the same change as their source specs.

Finish by reporting:

- the agnostic-ai version and install method;
- selected targets and imported sources;
- canonical files created or changed under `.agnostic-ai/`;
- generated outputs and whether Git tracks them;
- the results of `validate`, `lint`, and `sync --check`;
- any unsupported capability or decision left for the user.

Setup is not complete until `agnostic-ai sync --check` exits successfully.

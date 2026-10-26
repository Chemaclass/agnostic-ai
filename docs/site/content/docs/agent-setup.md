+++
title = "Set up agnostic-ai with a coding agent"
description = "Paste one prompt into your coding agent and it sets up agnostic-ai for you."
weight = 20

[extra]
nav_title = "Set up with an agent"
group = "Start"
+++

# Set up agnostic-ai with a coding agent

{{ <agent_setup_prompt /> }}

Open your coding agent in the project folder. It asks which tools the project uses and whether Git should track the generated files. Setup is done when `agnostic-ai sync --check` passes. Then [try a first session](@/docs/getting-started.md#sync) and learn [the daily loop](@/docs/getting-started.md#daily-use). To set up by hand instead, follow the [manual walkthrough](@/docs/getting-started.md).

<details>
<summary>Read the full checklist your coding agent follows</summary>

## What the agent does

Use this checklist when a user asks you to install or set up agnostic-ai in a repository. Preserve the existing tool setup, store source instructions in `.agnostic-ai/`, and finish with a passing `sync --check`.

### Setup rules {#safety-contract}

- Work from the repository root. Read its contributor and agent instructions first.
- Run `git status --short`. Never discard user changes. If the tree is not clean, tell the user before writing so they can tell your changes from theirs.
- Look for existing instructions files such as `CLAUDE.md`, `AGENTS.md`, and `GEMINI.md`. Tell the user what they hold and ask whether to import them before any sync.
- Import existing tool files before the first sync. Sync writes files; it does not import them.
- Enable only the targets the project uses or the user names. Never enable every target.
- Preview before writing. If sync reports a file it does not own or would overwrite, stop and name the path.
- Never print token or key values found in imported settings or MCP config.
- Stop and ask when a step fails or the project state does not match this checklist.
- After setup, edit instructions and specs under `.agnostic-ai/`, and project settings in `agnostic-ai.yaml`. The tool files are generated.

### 1. Install or verify the CLI

```bash
agnostic-ai --version
```

If missing, ask the user before installing. The script downloads the latest release and checks it against the published checksums:

```bash
# macOS or Linux
curl -fsSL https://raw.githubusercontent.com/Chemaclass/agnostic-ai/main/scripts/install.sh | bash
```

```powershell
# Windows PowerShell
irm https://raw.githubusercontent.com/Chemaclass/agnostic-ai/main/scripts/install.ps1 | iex
```

Run `--version` again. If the command is not found, add the install directory to `PATH`. [Installation](@/docs/installation.md) covers other methods.

### 2. Detect the project state

| State | Signal | Action |
|---|---|---|
| Already set up | `agnostic-ai.yaml` and `.agnostic-ai/` exist | Keep its selected tools and Git policy. Skip `init`. Go to step 4. |
| Existing tool files | `CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, `.cursor/`, `.github/copilot-instructions.md` | Import during `init`. |
| Fresh | Neither | Plain `init`. |

If the repository has only one of `agnostic-ai.yaml` or `.agnostic-ai/`, inspect it before initializing; do not replace a partial setup. If you are unsure which tools to enable, ask the user. A tool installed on the machine may not be one the repository uses.

### 3. Initialize

Ask the user whether Git should track the generated files. Pass `--gitignore=on` to ignore them (teammates run `sync` after cloning) or `--gitignore=off` to commit them (CI runs `sync --check`).

Replace `claude,codex` with the project's tools:

```bash
# existing tool files
printf '%s\n' 'claude,codex' | agnostic-ai init --from all --gitignore=on

# fresh project
printf '%s\n' 'claude,codex' | agnostic-ai init --gitignore=on
```

`--from all` adds each source's missing sections to `.agnostic-ai/AGNOSTIC_AI.md` and names them. Check that file for duplicates before you sync. Import tells you when a section needs a manual merge. See [Migration](@/docs/migration.md).

Use `--demo` only if the user asks. Add rules only for conventions the repository or user already states.

### 4. Validate and preview

```bash
agnostic-ai validate
agnostic-ai lint
agnostic-ai sync --dry-run
```

Check that the targets, output paths, and kept instructions match the project. Fix errors in the specs. Read warnings about instructions or fields a tool cannot use. A passing sync check means files match their sources; it does not mean every tool can enforce every rule. See [Tool behavior](@/docs/target-behavior.md) before changing restrictions or hiding warnings.

### 5. Sync and prove the result

```bash
agnostic-ai sync
agnostic-ai sync --check
git status --short
git diff -- .
```

Check the generated files and `.gitignore`. Inspect ignored output files directly: `git diff` does not show them. If Git tracks the generated files, include them with their specs when committing.

Report:

- agnostic-ai version and install method;
- targets and imported sources;
- what each tool now reads from `.agnostic-ai/`, as the first `agnostic-ai sync` lists it;
- files changed under `.agnostic-ai/`;
- the generated files and whether Git tracks them;
- results of `validate`, `lint`, and `sync --check`;
- decisions left for the user.

Setup is done only when `agnostic-ai sync --check` exits 0.

</details>

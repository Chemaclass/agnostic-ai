# agnostic-ai

agnostic-ai is for developers and teams using more than one AI coding tool, or preparing to change tools. Write agents, skills, rules, hooks, and MCP configuration once; `agnostic-ai sync` turns those shared specifications (specs) into the files each tool reads. Skills provide task instructions, hooks run actions at chosen events, and MCP connects tools to external services. See [Why agnostic-ai](https://agnostic-ai.org/docs/why-agnostic-ai/).

[![CI](https://github.com/Chemaclass/agnostic-ai/actions/workflows/ci.yml/badge.svg)](https://github.com/Chemaclass/agnostic-ai/actions/workflows/ci.yml)
[![npm](https://img.shields.io/npm/v/agnostic-ai?logo=npm&label=npm)](https://www.npmjs.com/package/agnostic-ai)
[![Homebrew](https://img.shields.io/badge/Homebrew-Chemaclass%2Ftap-FBB040?logo=homebrew&logoColor=111)](https://github.com/Chemaclass/homebrew-tap/blob/master/Casks/agnostic-ai.rb)
[![Downloads](https://img.shields.io/github/downloads/Chemaclass/agnostic-ai/total)](https://github.com/Chemaclass/agnostic-ai/releases)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/Chemaclass/agnostic-ai/badge)](https://scorecard.dev/viewer/?uri=github.com/Chemaclass/agnostic-ai)
[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/15088/badge)](https://www.bestpractices.dev/projects/15088)

Each AI tool reads its instructions from different files. Kept by hand, those files fall out of step. agnostic-ai keeps one editable source in plain Markdown and YAML inside your repository. No account, no service.

## Set up with a coding agent

Paste this into Claude Code, Codex, Cursor, or another coding agent:

```text
Set up agnostic-ai in this repository. Follow https://agnostic-ai.org/agent-setup.txt exactly. Preserve existing AI tool behavior. Before any sync, check for existing CLAUDE.md, AGENTS.md, and similar files, and ask me what to do with their content. Finish with agnostic-ai sync --check. Summarize the targets selected and every file changed.
```

It takes about two minutes. The [agent setup guide](https://agnostic-ai.org/docs/agent-setup/) is the checklist the agent follows.

<a id="set-up-manually"></a>

## Quickstart

From your project root, with Node 18 or newer:

```bash
npm install -g agnostic-ai
agnostic-ai init --from all
agnostic-ai sync --plan
agnostic-ai sync
```

`init --from all` creates the project config and imports the tool files it finds. Pick your tools when prompted. `sync --plan` previews the changes. `sync` writes the files each tool reads.

Use `agnostic-ai sync --global` for your personal specs in `~/.agnostic-ai/`. It also writes [Kiro CLI V3 hooks](https://agnostic-ai.org/docs/targets/kiro/) and the user MCP files for [Antigravity](https://agnostic-ai.org/docs/targets/antigravity/) and [Warp](https://agnostic-ai.org/docs/targets/warp/). See [global configuration](https://agnostic-ai.org/docs/configuration/#global-configuration) for paths and supported tools.

From then on, edit sources under `.agnostic-ai/`, then sync again. `AGNOSTIC_AI.md` holds the shared project instructions. Files such as `CLAUDE.md`, `AGENTS.md`, and `.cursor/rules/` are generated outputs.

After installing a newer release, run `agnostic-ai upgrade --requires` from the project root to update an exact version pin and sync again. To keep a deliberate minimum or range, follow [guided upgrades](https://agnostic-ai.org/docs/cli-reference/maintain/#upgrade).

The [VS Code extension](editors/vscode/) shows lint findings for saved specs and project configuration in Problems. Open or save a source to check it, then fix and save to clear the finding.

Next: [Getting started](https://agnostic-ai.org/docs/getting-started/) to add your first rule, [Installation](https://agnostic-ai.org/docs/installation/) for other installers, and [Migration](https://agnostic-ai.org/docs/migration/) to review an existing setup.

## Daily commands

Interactive runs offer new releases with upgrade guidance and a default Yes prompt. Accept to update, migrate this project, and check its generated files. The prompt includes any project version and schema changes. Local package installations show their package manager's update command. Set `AGNOSTIC_AI_NO_UPDATE_CHECK=1` to disable the daily check. See [guided upgrades](https://agnostic-ai.org/docs/cli-reference/maintain/#upgrade).

```bash
agnostic-ai import claude codex --dry-run --diff  # preview existing tool config
agnostic-ai import claude --overwrite           # replace conflicting specs
agnostic-ai compare claude cursor                # compare fields, rule activation, hooks, and MCP connections
agnostic-ai new settings project-defaults --dry-run # preview a settings spec
agnostic-ai why AGENTS.md                        # trace an output to its source
agnostic-ai explain --file src/main.go --target claude # show configured instructions for a file
agnostic-ai project --check                      # check the installed project binary and generated output
agnostic-ai sync --check                         # find generated files that differ from the sources
agnostic-ai doctor --fix                         # repair generated files, choose which leftover files to remove
agnostic-ai migrate --dry-run                    # preview rewrites of old spec forms
agnostic-ai memory lint                          # check the shared memory index and facts
agnostic-ai memory path                          # print the memory folders, including the repo store
```

`agnostic-ai new` creates all ten spec kinds in their configured source directories. Edit the marked placeholders before rendering or syncing. See [new](https://agnostic-ai.org/docs/cli-reference/start/#new).

`explain --file` supports Claude Code and Cursor. It shows source specs, planned output paths, and why instructions apply. See [file instructions](https://agnostic-ai.org/docs/cli-reference/inspect/#explain-a-source-file).

On a fresh clone, `sync --check` uses Git history to identify old generated files. See [Leftover files](https://agnostic-ai.org/docs/cli-reference/sync/) for what it keeps and how to remove them.

## What you can share

Support spans [Claude Code, Codex, Cursor, Gemini CLI, Copilot, Kiro, and more](https://agnostic-ai.org/docs/targets/#capability-matrix). Each tool supports a different set of spec kinds; the [target reference](https://agnostic-ai.org/docs/targets/) shows the exact paths.

- **Rules, agents, skills, and commands** in one [spec format](https://agnostic-ai.org/docs/spec-format/). Kiro commands land in `.kiro/prompts/` for CLI V3. `import trae` also reads shared `.agents/skills/` folders, keeping `.trae/skills/` first for duplicate names.
- **Trae agents** preserve an explicitly empty list of allowed tools, including after import, so text-only agents keep tools disabled.
- **Capabilities** such as `read(src/**)`, `shell(git diff *)`, and `mcp:github` map to each tool's own names. `lint` warns when one covers a whole tool. See [Capabilities](https://agnostic-ai.org/docs/spec-format/agents/#capabilities).
- **Hooks** run checks before or after a tool acts. Shared `on` and `match` values work where tools handle results the same way. Kiro supports `on: before-tool` for its built-in tools and named MCP servers. See [Portable hooks](https://agnostic-ai.org/docs/spec-format/hooks/#portable-events).
- **Model roles** name a choice once for every tool. See [Models and aliases](https://agnostic-ai.org/docs/configuration/#models).
- **MCP servers** use environment references for credentials. Mark plain settings with `!literal`. See [MCP references](https://agnostic-ai.org/docs/spec-format/mcps/#environment-references).
- **Session handoffs** carry a task from one tool to another on the same machine. `builtins: [handoff]` adds the skill; `handoff-hook` adds Git snapshots and resume notices. See [Session handoffs](https://agnostic-ai.org/docs/handoff/).
- **Shared memory** keeps one project memory that every tool reads and writes. `builtins: [memory]` adds it. Personal memory can share one store across worktrees with `memory.personal: repo` in the local config. See [Shared memory](https://agnostic-ai.org/docs/memory/).
- **RTK and Caveman** are separate opt-ins: `builtins: [rtk]` adds a Claude hook that rewrites commands using an installed RTK; `builtins: [caveman]` adds the response skill from a fixed Caveman revision, invoked with `/caveman`, with no executable needed. Keep existing `builtins` names when adding either one. Both stay off in existing projects and `init`. Remove a name and sync to remove its owned output. See [RTK and Caveman](https://agnostic-ai.org/docs/rtk-and-caveman/) for supported coding tools and how to avoid duplicate installations.

## Develop agnostic-ai

```bash
make tools      # install pinned development tools once
make build
make preflight  # format, lint, and Go tests
```

This repository keeps its own agent setup in `.agnostic-ai/`. Edit those source specs, then run `./agnostic-ai sync`. Most native output is ignored by Git; `.openhands/setup.sh` is tracked to set up new checkouts. See [CONTRIBUTING.md](CONTRIBUTING.md) for checks by change type and the [architecture guide](docs/internal/architecture.md) for the Go packages.

[Getting started](https://agnostic-ai.org/docs/getting-started/) · [Playground](https://agnostic-ai.org/playground/) · [Editor extensions](editors/) · [Claude Code plugin](plugins/agnostic-ai/) · [CLI reference](https://agnostic-ai.org/docs/cli-reference/) · [Changelog](CHANGELOG.md)

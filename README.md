# agnostic-ai

agnostic-ai keeps one set of instructions for all your AI coding tools. You write agents, skills, rules, hooks, and MCP server settings once, and `agnostic-ai sync` writes the files each tool reads.

[![CI](https://github.com/Chemaclass/agnostic-ai/actions/workflows/ci.yml/badge.svg)](https://github.com/Chemaclass/agnostic-ai/actions/workflows/ci.yml)
[![npm](https://img.shields.io/npm/v/agnostic-ai?logo=npm&label=npm)](https://www.npmjs.com/package/agnostic-ai)
[![Homebrew](https://img.shields.io/badge/Homebrew-Chemaclass%2Ftap-FBB040?logo=homebrew&logoColor=111)](https://github.com/Chemaclass/homebrew-tap/blob/master/Casks/agnostic-ai.rb)
[![Downloads](https://img.shields.io/github/downloads/Chemaclass/agnostic-ai/total)](https://github.com/Chemaclass/agnostic-ai/releases)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/Chemaclass/agnostic-ai/badge)](https://scorecard.dev/viewer/?uri=github.com/Chemaclass/agnostic-ai)
[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/15088/badge)](https://www.bestpractices.dev/projects/15088)

Each AI tool reads its instructions from different files. Kept by hand, those files fall out of step. agnostic-ai keeps one source in plain Markdown and YAML inside your repository. It needs no account and no service. See [Why agnostic-ai](https://agnostic-ai.org/docs/why-agnostic-ai/).

## Set up with a coding agent

Paste this into Claude Code, Codex, Cursor, or another coding agent:

```text
Set up agnostic-ai in this repository. Follow https://agnostic-ai.org/agent-setup.txt exactly. Preserve existing AI tool behavior. Before any sync, check for existing CLAUDE.md, AGENTS.md, and similar files, and ask me what to do with their content. Finish with agnostic-ai sync --check. Summarize the targets selected and every file changed.
```

The [agent setup guide](https://agnostic-ai.org/docs/agent-setup/) is the checklist the agent follows.

<a id="set-up-manually"></a>
<a id="install"></a>

## Quickstart

From your project root, with Node 18 or newer:

```bash
npm install -g agnostic-ai
agnostic-ai init --from all
agnostic-ai sync --plan
```

Review the planned files. If they match the tools and instructions you want, run:

```bash
agnostic-ai sync
```

`init --from all` creates the project config and imports the tool files it finds. Pick your tools when prompted. `sync --plan` previews the changes. `sync` writes the files each tool reads.

Then edit your shared instructions in `.agnostic-ai/AGNOSTIC_AI.md` and your specs under `.agnostic-ai/`, and run `agnostic-ai sync` again. Files such as `CLAUDE.md`, `AGENTS.md`, and `.cursor/rules/` are generated. Do not edit them by hand.

Next steps:

- [Getting started](https://agnostic-ai.org/docs/getting-started/): add your first rule.
- [Installation](https://agnostic-ai.org/docs/installation/): other ways to install.
- [Migration](https://agnostic-ai.org/docs/migration/): review an existing setup before you sync.
- [Global configuration](https://agnostic-ai.org/docs/configuration/#global-configuration): personal specs in `~/.agnostic-ai/`, written with `agnostic-ai sync --global`.

## Daily commands

Run from the directory containing `agnostic-ai.yaml`:

```bash
agnostic-ai status       # see selected tools and out-of-date files
agnostic-ai sync --plan  # preview what will change
agnostic-ai sync         # write the tool files
agnostic-ai sync --check # confirm they match the sources
```

When something looks wrong:

```bash
agnostic-ai why AGENTS.md  # trace an output to its source
agnostic-ai doctor --fix   # repair generated files
```

The [command reference](https://agnostic-ai.org/docs/cli-reference/) covers import, compare, explain, upgrade, and the other commands.

## What you can share

Each tool supports a different set of spec kinds. The [capability matrix](https://agnostic-ai.org/docs/targets/#capability-matrix) shows which tools get what, and the [target reference](https://agnostic-ai.org/docs/targets/) shows the exact paths. Supported tools include Claude Code, Codex, Cursor, Gemini CLI, Copilot, and Kiro.

- **Rules, agents, skills, and commands**: instructions in one [spec format](https://agnostic-ai.org/docs/spec-format/).
- **Hooks**: checks that run before or after a tool acts. See [hooks](https://agnostic-ai.org/docs/spec-format/hooks/).
- **MCP servers**: connections to outside services, with credentials read from environment variables. See [MCP servers](https://agnostic-ai.org/docs/spec-format/mcps/).
- **Capabilities**: what an agent may read, run, or call, mapped to each tool's own names. See [capabilities](https://agnostic-ai.org/docs/spec-format/agents/#capabilities).
- **Model roles**: name a model choice once for every tool. See [models](https://agnostic-ai.org/docs/configuration/#models).
- **Code review guidance**: instructions for review bots. See [reviews](https://agnostic-ai.org/docs/spec-format/reviews/).
- **Shared memory**: one project memory that every tool reads and writes. See [shared memory](https://agnostic-ai.org/docs/memory/).
- **Session handoffs**: one private note per branch, so you can pick up work in another tool. See [session handoffs](https://agnostic-ai.org/docs/handoff/).
- **RTK and Caveman**: optional add-ons that shorten command output and responses. See [RTK and Caveman](https://agnostic-ai.org/docs/rtk-and-caveman/).

OpenCode agents with a `can` or `tools` list keep project permission limits from `opencode.json`, including native V2 rules. See [OpenCode permissions](https://agnostic-ai.org/docs/targets/opencode/).

## Develop agnostic-ai

```bash
make tools      # install pinned development tools once
make build
make preflight  # format, lint, and Go tests
```

This repository keeps its own agent setup in `.agnostic-ai/`. Edit those source specs, then run `./agnostic-ai sync`. See [CONTRIBUTING.md](CONTRIBUTING.md) for checks by change type and the [architecture guide](docs/internal/architecture.md) for the Go packages.

[Getting started](https://agnostic-ai.org/docs/getting-started/) · [Playground](https://agnostic-ai.org/playground/) · [Editor extensions](editors/) · [Claude Code plugin](plugins/agnostic-ai/) · [CLI reference](https://agnostic-ai.org/docs/cli-reference/) · [Changelog](CHANGELOG.md)

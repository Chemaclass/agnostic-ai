# agnostic-ai

agnostic-ai keeps your coding tools working from the same project instructions. Edit one source, then run `agnostic-ai sync` to write the files Claude Code, Codex, and your other tools read.

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

The [setup guide](https://agnostic-ai.org/docs/getting-started/) walks through agent-assisted or terminal setup, including keeping existing instructions.

<a id="set-up-manually"></a>
<a id="install"></a>

## Quickstart

From your project root, with Node 18 or newer:

```bash
npm install -g agnostic-ai
agnostic-ai init
mkdir -p .agnostic-ai
```

Select the tools you use. In a terminal, `init` offers to import existing tool files; keep their instructions before syncing. Create or edit `.agnostic-ai/AGNOSTIC_AI.md` with one useful project note, keeping imported content. See the [one-note walkthrough](https://agnostic-ai.org/docs/getting-started/#scaffold).

Preview the planned files, then generate and check them:

```bash
agnostic-ai sync --plan
agnostic-ai sync
agnostic-ai sync --check
```

Open the generated files and follow any instruction-file references to find your note. With Claude Code and Codex selected in a fresh setup, `CLAUDE.md` points to the shared `AGENTS.md`. Change the source note and sync again to update the instructions both tools read. Keep editing `.agnostic-ai/`, since sync replaces generated output.

Next steps:

- [Set up your project](https://agnostic-ai.org/docs/getting-started/): share one note and update your tools together.
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

`agnostic-ai explain <spec> --layers` shows which built-in, pack, project, and personal sources replace or extend a spec.

The [command reference](https://agnostic-ai.org/docs/cli-reference/) covers import, compare, explain, upgrade, and the other commands.

## What you can share

Each tool supports a different set of spec kinds. The [capability matrix](https://agnostic-ai.org/docs/targets/#capability-matrix) shows which tools get what, and the [target reference](https://agnostic-ai.org/docs/targets/) shows the exact paths. Supported tools include Claude Code, Codex, Cursor, Gemini CLI, Copilot, Kiro, and Qwen Code.

- **Rules, agents, skills, and commands**: instructions in one [spec format](https://agnostic-ai.org/docs/spec-format/).
- **Hooks**: checks and observers that run before or after a tool acts. See [hooks](https://agnostic-ai.org/docs/spec-format/hooks/).
- **MCP servers**: connections to outside services, with credentials read from environment variables. See [MCP servers](https://agnostic-ai.org/docs/spec-format/mcps/).
- **Capabilities**: what an agent may read, run, or call, mapped to each tool's own names. See [capabilities](https://agnostic-ai.org/docs/spec-format/agents/#capabilities). [OpenCode agents](https://agnostic-ai.org/docs/targets/opencode/) with a `can` or `tools` list keep project permission limits from `opencode.json`, including native V2 rules, and keep their rule order through import and sync.
- **Model roles**: name a model choice once for every tool. See [models](https://agnostic-ai.org/docs/configuration/#models).
- **Code review guidance**: instructions for review bots. See [reviews](https://agnostic-ai.org/docs/spec-format/reviews/).
- **Shared memory**: one project memory that every tool reads and writes. See [shared memory](https://agnostic-ai.org/docs/memory/).
- **Session handoffs**: one private note per branch, so you can pick up work in another tool. See [session handoffs](https://agnostic-ai.org/docs/handoff/).
- **RTK and Caveman**: optional add-ons that shorten command output and responses. See [RTK and Caveman](https://agnostic-ai.org/docs/rtk-and-caveman/).

## Develop agnostic-ai

```bash
make tools      # install pinned development tools once
make build
make preflight  # format, lint, and Go tests
```

This repository keeps its own agent setup in `.agnostic-ai/`. Edit those source specs, then run `./agnostic-ai sync`. See [CONTRIBUTING.md](CONTRIBUTING.md) for checks by change type and the [architecture guide](docs/internal/architecture.md) for the Go packages.

[Target audits](scripts/target-audit/manifest.md) record coverage before updating the vendor source lock and can resume completed research after fresh source verification. Manual updates that skip coverage use `scripts/docfetch.sh --update-unvalidated`.

[Getting started](https://agnostic-ai.org/docs/getting-started/) · [Playground](https://agnostic-ai.org/playground/) · [Editor extensions](editors/) · [Claude Code plugin](plugins/agnostic-ai/) · [CLI reference](https://agnostic-ai.org/docs/cli-reference/) · [Changelog](CHANGELOG.md)

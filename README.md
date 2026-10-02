# agnostic-ai

agnostic-ai is for developers and teams using more than one AI coding tool, or preparing to change tools. Write agents, skills, rules, hooks, and MCP configuration once; `agnostic-ai sync` turns those specs into each tool's native files. See [Why agnostic-ai](https://agnostic-ai.org/docs/why-agnostic-ai/).

[![CI](https://github.com/Chemaclass/agnostic-ai/actions/workflows/ci.yml/badge.svg)](https://github.com/Chemaclass/agnostic-ai/actions/workflows/ci.yml)
[![npm](https://img.shields.io/npm/v/agnostic-ai?logo=npm&label=npm)](https://www.npmjs.com/package/agnostic-ai)
[![Homebrew](https://img.shields.io/badge/Homebrew-Chemaclass%2Ftap-FBB040?logo=homebrew&logoColor=111)](https://github.com/Chemaclass/homebrew-tap/blob/master/Casks/agnostic-ai.rb)
[![Downloads](https://img.shields.io/github/downloads/Chemaclass/agnostic-ai/total)](https://github.com/Chemaclass/agnostic-ai/releases)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/Chemaclass/agnostic-ai/badge)](https://scorecard.dev/viewer/?uri=github.com/Chemaclass/agnostic-ai)
[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/15088/badge)](https://www.bestpractices.dev/projects/15088)

AI tools store instructions in different files. Keeping those files by hand makes them drift. agnostic-ai keeps the editable source in plain Markdown and YAML in your repository. It needs no account or service.

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

`init --from all` creates the project config and imports existing tool files when it finds them. Pick the tools you use when prompted. `sync --plan` previews the changes; `sync` writes the native files.

Edit sources under `.agnostic-ai/`, including `AGNOSTIC_AI.md` for shared project instructions, then sync again. Generated files such as `CLAUDE.md`, `AGENTS.md`, and `.cursor/rules/` are outputs.

`sync`, `import`, `use`, and `init` take a project lock. A second writer stops and names the running command; retry when it finishes. Read-only checks and previews can still run.

See [Getting started](https://agnostic-ai.org/docs/getting-started/) to add your first rule, [all install options](https://agnostic-ai.org/docs/installation/) for other installers, and [Migration](https://agnostic-ai.org/docs/migration/) to review an existing setup.

`import`, `sync`, and `validate` use the same `sources` paths, including absolute directories and linked source roots. Watch mode follows their edits too. Polling picks up edits made during a re-sync on the next tick. `import --dry-run` previews those destinations without writing them.

## Daily commands

```bash
agnostic-ai import claude codex --dry-run --diff  # preview existing tool config
agnostic-ai import claude --overwrite           # replace conflicting specs
agnostic-ai compare claude cursor                # compare agent and skill fields and rule activation
agnostic-ai why AGENTS.md                        # trace an output to its source
agnostic-ai sync --check                         # find local drift
agnostic-ai doctor --fix                         # repair drift, choose kept orphan removal
```

Support spans [Claude Code, Codex, Cursor, Gemini CLI, Copilot, and more](https://agnostic-ai.org/docs/targets/#capability-matrix). Each tool supports a different set of spec kinds. The [spec format](https://agnostic-ai.org/docs/spec-format/) and [target reference](https://agnostic-ai.org/docs/targets/) show the exact paths and fields.

MCP import replaces literal environment and header values with portable references. Continue sync writes its secret syntax for `.env` files. See [MCP references](https://agnostic-ai.org/docs/spec-format/mcps/#environment-references).

Claude MCP import and sync preserve `alwaysLoad` and `bareElicitationCapability`, including explicit `false` values. See [Claude MCP options](https://agnostic-ai.org/docs/targets/claude/#output) for tool loading and connection compatibility.

## Develop agnostic-ai

```bash
make tools      # install pinned development tools once
make build
make preflight  # format, lint, and Go tests
```

This repository keeps its own agent setup in `.agnostic-ai/`. Edit those source specs, then run `./agnostic-ai sync`. Most native output is ignored by Git; `.openhands/setup.sh` is tracked for bootstrap. See [CONTRIBUTING.md](CONTRIBUTING.md) for checks by change type and the [architecture guide](docs/internal/architecture.md) for the Go packages.

[Getting started](https://agnostic-ai.org/docs/getting-started/) · [Playground](https://agnostic-ai.org/playground/) · [Editor extensions](editors/) · [Claude Code plugin](plugins/agnostic-ai/) · [CLI reference](https://agnostic-ai.org/docs/cli-reference/) · [Changelog](CHANGELOG.md)

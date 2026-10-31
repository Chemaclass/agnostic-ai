# agnostic-ai

agnostic-ai keeps your coding tools working from the same project instructions. Edit one source, then run `agnostic-ai sync` to write the files Claude Code, Codex, and your other tools read.

[![CI](https://github.com/Chemaclass/agnostic-ai/actions/workflows/ci.yml/badge.svg)](https://github.com/Chemaclass/agnostic-ai/actions/workflows/ci.yml)
[![npm](https://img.shields.io/npm/v/agnostic-ai?logo=npm&label=npm)](https://www.npmjs.com/package/agnostic-ai)
[![Homebrew](https://img.shields.io/badge/Homebrew-Chemaclass%2Ftap-FBB040?logo=homebrew&logoColor=111)](https://github.com/Chemaclass/homebrew-tap/blob/master/Casks/agnostic-ai.rb)
[![Downloads](https://img.shields.io/github/downloads/Chemaclass/agnostic-ai/total)](https://github.com/Chemaclass/agnostic-ai/releases)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/Chemaclass/agnostic-ai/badge)](https://scorecard.dev/viewer/?uri=github.com/Chemaclass/agnostic-ai)
[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/15088/badge)](https://www.bestpractices.dev/projects/15088)

Each AI tool reads its instructions from different files. Kept by hand, those files fall out of step. agnostic-ai keeps one source in plain Markdown and YAML inside your repository. It needs no account and no service. See [Why agnostic-ai](https://agnostic-ai.org/docs/why-agnostic-ai/).

<a id="set-up-manually"></a>
<a id="install"></a>

## Quickstart

From your project root, with Node 18 or newer:

```bash
npm install -g agnostic-ai
agnostic-ai init
```

Select the tools you use, for example Claude Code and Codex. If the project already has files such as `CLAUDE.md` or `AGENTS.md`, accept the offer to import them so their instructions are kept. If the project already has `agnostic-ai.yaml`, skip `init` and run `agnostic-ai sync`.

`init` also creates `.agnostic-ai/AGNOSTIC_AI.md`, the note every selected tool reads. Open it and replace the placeholder comment with one project note, or add it below any imported instructions, for example:

```markdown
Run the project tests before finishing a change.
```

Preview the files, write them, and check that they match the source:

```bash
agnostic-ai sync --plan
agnostic-ai sync
agnostic-ai sync --check
```

Use `sync -v` for a concise report of changes and `sync -vv` for full details. Both commands write generated files. Sync saves hand edits as `.bak` before replacing or removing a generated file; `revert` restores them.

Changing only the capitalization of a rule, skill, or agent name needs one sync, including on macOS and Windows.

With Claude Code and Codex selected, your note is now in `AGENTS.md`, and `CLAUDE.md` points to it. To change what both tools read, edit `.agnostic-ai/AGNOSTIC_AI.md` and sync again. Do not edit the generated files: sync replaces them.

Homebrew, the install script, Windows, and Go are on the [installation page](https://agnostic-ai.org/docs/installation/).

When a project version requirement blocks a command, the terminal offers a fix, a configured AI CLI, or a prompt to copy. See [version mismatch help](https://agnostic-ai.org/docs/errors/#aai-005-installed-version-outside-requires).

## Set up with a coding agent

Or paste this into Claude Code, Codex, Cursor, or another coding agent:

```text
Set up agnostic-ai in this repository. Follow https://agnostic-ai.org/agent-setup.txt exactly. Preserve existing AI tool behavior. Before any sync, check for existing CLAUDE.md, AGENTS.md, and similar files, and ask me what to do with their content. Finish with agnostic-ai sync --check. Summarize the targets selected and every file changed.
```

## Next steps

- [Set up your project](https://agnostic-ai.org/docs/getting-started/): the full walkthrough, Git choices, and everyday commands.
- [Import an existing setup](https://agnostic-ai.org/docs/migration/): review your current tool files before the first sync.
- [What you can share](https://agnostic-ai.org/docs/why-agnostic-ai/#what-to-add-and-when): rules, skills, agents, hooks, MCP servers, settings, and memory.
- [Supported tools](https://agnostic-ai.org/docs/targets/): what each tool receives, and where.
- [Inspect lifecycle hooks](https://agnostic-ai.org/docs/cli-reference/inspect/#serve): scan stage counts, then expand a stage or all stages to see hooks.
- [Share an MCP server only one tool has](https://agnostic-ai.org/docs/cli-reference/inspect/#serve): **Tools** previews and saves one server as a spec, ready for your next sync.
- [Find a global hook's files](https://agnostic-ai.org/docs/cli-reference/inspect/#explain): `agnostic-ai explain --global hooks/guard.yaml` lists where sync places the hook; directory-based hooks also list copied scripts.
- [Trace a generated file](https://agnostic-ai.org/docs/cli-reference/inspect/#why): `agnostic-ai why CLAUDE.md` finds its source even when an unselected tool rejects your setup.
- [Voice dictation](https://agnostic-ai.org/docs/spec-format/settings/#voice-dictation): keep personal voice settings in your home specs, import existing values, and apply them with `sync --global`, including Warp dictation.
- [Edit settings in the browser](https://agnostic-ai.org/docs/cli-reference/inspect/#serve): browse source groups in tabs, compare Shared and Personal, preview permission rules, and edit global voice dictation settings.
- [Troubleshooting](https://agnostic-ai.org/docs/troubleshooting/): when a file is missing or a check fails.

## Contribute

See [CONTRIBUTING.md](CONTRIBUTING.md) to build the CLI and choose checks for your change.

[Documentation](https://agnostic-ai.org/docs/) · [Playground](https://agnostic-ai.org/playground/) · [Editor extensions](editors/) · [Claude Code plugin](plugins/agnostic-ai/) · [CLI reference](https://agnostic-ai.org/docs/cli-reference/) · [Changelog](CHANGELOG.md)

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/site/static/logo-dark.svg">
    <img src="docs/site/static/logo.svg" alt="agnostic-ai logo" width="96" height="96">
  </picture>
</p>

# agnostic-ai

Write project instructions once in Markdown and YAML. Run `agnostic-ai sync` to generate the files Claude Code, Codex, and your other tools read. Sync runs locally and needs no account.

[![CI](https://github.com/Chemaclass/agnostic-ai/actions/workflows/ci.yml/badge.svg)](https://github.com/Chemaclass/agnostic-ai/actions/workflows/ci.yml)
[![npm](https://img.shields.io/npm/v/agnostic-ai?logo=npm&label=npm)](https://www.npmjs.com/package/agnostic-ai)
[![Homebrew](https://img.shields.io/badge/Homebrew-Chemaclass%2Ftap-FBB040?logo=homebrew&logoColor=111)](https://github.com/Chemaclass/homebrew-tap/blob/master/Casks/agnostic-ai.rb)
[![Downloads](https://img.shields.io/github/downloads/Chemaclass/agnostic-ai/total)](https://github.com/Chemaclass/agnostic-ai/releases)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/Chemaclass/agnostic-ai/badge)](https://scorecard.dev/viewer/?uri=github.com/Chemaclass/agnostic-ai)
[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/15088/badge)](https://www.bestpractices.dev/projects/15088)

[Why share your coding tool setup?](https://agnostic-ai.org/docs/why-agnostic-ai/)

<a id="set-up-manually"></a>
<a id="install"></a>

## Quickstart

From your project root, with Node 18 or newer:

```bash
npm install -g agnostic-ai
agnostic-ai init
```

Select your tools. Accept the import offer to keep instructions from existing files such as `CLAUDE.md` and `AGENTS.md`. If the project already has `agnostic-ai.yaml`, run `agnostic-ai sync` directly.

`init` creates `.agnostic-ai/AGNOSTIC_AI.md`. Replace the placeholder comment with a project note, or add one below the imported instructions.

```markdown
Run the project tests before finishing a change.
```

Preview, write, and check the generated files.

```bash
agnostic-ai sync --plan
agnostic-ai sync
agnostic-ai sync --check
```

If you selected Claude Code and Codex, `AGENTS.md` contains your note and `CLAUDE.md` points to it. Edit `.agnostic-ai/AGNOSTIC_AI.md` and sync again to update both. Sync replaces generated files and saves hand edits in `.bak` files. Use `agnostic-ai revert` to restore them.

See the [installation guide](https://agnostic-ai.org/docs/installation/) for Homebrew, the install script, Windows, and Go.

## Set up with a coding agent

Paste this into your coding agent.

```text
Set up agnostic-ai in this repository. Follow https://agnostic-ai.org/agent-setup.txt exactly. Preserve existing AI tool behavior. Before any sync, check for existing CLAUDE.md, AGENTS.md, and similar files, and ask me what to do with their content. Finish with agnostic-ai sync --check. Summarize the targets selected and every file changed.
```

## Next steps

- [Set up your project](https://agnostic-ai.org/docs/getting-started/) and see what to commit and which commands to run after editing.
- [Import an existing setup](https://agnostic-ai.org/docs/migration/) before the first sync.
- [Choose what to share](https://agnostic-ai.org/docs/why-agnostic-ai/#what-to-add-and-when). Share rules, skills, agents, hooks, MCP servers, settings, and memory.
- [See what each tool reads](https://agnostic-ai.org/docs/targets/).
- [Share dependency setup for cloud agents](https://agnostic-ai.org/docs/spec-format/environments/#cloud-sessions) from one environment spec.
- [Check cloud environment setup](https://agnostic-ai.org/docs/cli-reference/doctor/#doctor-environment) with `agnostic-ai doctor environment`.
- [Install the floating companion](https://agnostic-ai.org/docs/cli-reference/companion/#install) from a Mac disk image that includes the CLI and dashboard, or run `agnostic-ai companion install` with an existing CLI. Browse coding sessions, Claude chats, Cowork tasks, and ChatGPT chats. Press Tab to focus a session and Enter to open it or switch to its running tab in a supported terminal. Open **Keyboard shortcuts** from the list’s ⋯ menu for the current key bindings. If the companion switches to a possible Codex tab, the list stays closed until you reopen it. You can also [stop a forgotten Claude Code shell](https://agnostic-ai.org/docs/cli-reference/companion-list/#stop-a-forgotten-shell). In either menu, open **App → Updates** to check now or change automatic checks. Updates keep saved read and archive marks. On macOS, updates signed by the same signer keep Accessibility access, and automatic checks can show the last verified update offer offline.
- [Open the web interface](https://agnostic-ai.org/docs/cli-reference/serve/#serve) with `agnostic-ai ui` to see context size and edit hooks, MCP servers, and settings. The Conversations page starts with chats that need you and explains why cloud chats are missing.
- [Save a session handoff](https://agnostic-ai.org/docs/handoff/) to switch tools mid-task or automate Git snapshots with Claude hooks.
- [Start a side conversation](https://agnostic-ai.org/docs/side-conversation/) with `agnostic-ai conversations start`. It opens a terminal tab linked to the original session.
- [Message another session](https://agnostic-ai.org/docs/mailbox/) to ask a question, request a review, or hand off work between local Claude Code and Codex sessions. The `mailbox` built-in delivers messages with the next prompt after any required approval. Each thread shows its next action and puts other controls under **More**. `agnostic-ai mailbox stats` reports counts without showing message text.
- [Answer routine permission prompts](https://agnostic-ai.org/docs/approvals/) with `approvals.mode: yolo` in your home config. A hook handles safe calls one at a time for Claude Code, Codex, Kiro, Qwen Code, and Qoder. Riskier calls still ask.
- [Hand off to the cloud](https://agnostic-ai.org/docs/cloud-handoff/) by saying "send this to a remote agent" in Claude Code or Codex. The skill pushes the branch and starts a cloud session. Copy a Claude Code cloud session's resume command from the companion or web interface.
- [Trace a generated file](https://agnostic-ai.org/docs/cli-reference/inspect/#why) with `agnostic-ai why CLAUDE.md`.
- [Keep personal settings](https://agnostic-ai.org/docs/global-configuration/) in `~/.agnostic-ai` and apply them with `agnostic-ai sync --scope user`.
- [Find a missing file or a failed check](https://agnostic-ai.org/docs/troubleshooting/) with `agnostic-ai doctor`.

## Contribute

See [CONTRIBUTING.md](CONTRIBUTING.md) to build the CLI and choose checks for your change.

[Documentation](https://agnostic-ai.org/docs/) · [Playground](https://agnostic-ai.org/playground/) · [Editor extensions](editors/) · [Claude Code plugin](plugins/agnostic-ai/) · [CLI reference](https://agnostic-ai.org/docs/cli-reference/) · [Changelog](CHANGELOG.md)

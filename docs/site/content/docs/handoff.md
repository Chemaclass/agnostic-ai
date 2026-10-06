+++
title = "Session handoffs"
description = "Write a portable session handoff and resume work in another AI tool on the same machine."
weight = 78

[extra]
group = "Workflows"
+++

# Session handoffs

Enable the built-in `handoff` skill to carry a task between AI tools. The leaving tool writes portable Markdown. The arriving tool reads it, compares it with git, and continues from the recorded next steps.

## Enable the skill

Add this to the project's `agnostic-ai.yaml`, then run `agnostic-ai sync`:

```yaml
requires: ">=0.80.0"
builtins: [handoff]
```

`init` enables `handoff` for new projects. Existing projects opt in. To use it across projects, put the same key in `~/.agnostic-ai/agnostic-ai.yaml` and run `agnostic-ai sync --global`.

Targets with native skills receive it through their normal skill path. Targets configured with `emit-skills-as-commands` also receive a command. Aider inlines the instructions when `outputs.aider.rules-file` is configured. Jules has no skill surface. The browser playground does not show built-ins.

## Leave a session

Ask the current tool to "write a handoff". It records the goal, acceptance criteria, completed work, next steps, open decisions, and verification commands in `.agnostic-ai/local/HANDOFF.md` at the project root. The header records the tool, date, branch, and HEAD. The file has a soft cap of 100 lines.

```markdown
# Handoff
tool: codex | date: 2026-10-05T14:02Z | branch: feat/x | head: abc1234

## Goal
## Acceptance
## Done
## In progress
## Next steps
## Open decisions
## Gotchas
## Verify
```

Each write replaces the previous handoff in that checkout. It excludes secrets and tool-owned memory stores. Git records the code's history. `sync --watch` ignores both `HANDOFF.md` and `HANDOFF.auto.md`, including temporary snapshot files, in this local folder.

The skill uses the nearest ancestor with `agnostic-ai.yaml` or the legacy `agnostic.config.yaml` as the project root. It compares canonical paths and skips the effective global source root (`AGNOSTIC_AI_HOME` when nonempty, otherwise `~/.agnostic-ai`), whose home config never identifies a project. Separately configured subprojects keep separate handoffs, including requests from a nested directory. When no project config remains, it falls back to the current Git worktree root.

## Promote durable learnings

After saving the handoff, the skill proposes learnings from the session. It skips facts already in loaded rules or obvious from the code. Each proposal has a scope tag and a diff for its destination file. Confirm both the content and scope before the skill writes it.

| Scope | Default destination | Sync after approval |
| --- | --- | --- |
| [project] | `.agnostic-ai/rules/learnings.md` | `agnostic-ai sync` |
| [personal] | `.agnostic-ai/local/rules/learnings-local.md` | `agnostic-ai sync` |
| [global] | `~/.agnostic-ai/rules/learnings.md` | `agnostic-ai sync --global` |

Project `learnings.md` follows the effective `sources.rules` from the project config and any `agnostic-ai.local.yaml` override. Relative paths start at the project root; absolute paths are used directly, without `~` expansion. The personal path stays fixed. A nonempty `AGNOSTIC_AI_HOME` moves the global file to that directory's `rules/learnings.md`; global `sources` config does not change the `rules/` folder. Proposals and diffs show each resolved destination before confirmation.

Approved items append to existing entries. New files receive valid rule frontmatter. The personal rule is named `learnings-local` so it keeps the project rule's body. Global writes require an explicit global choice for each item; global rules have no `scope`, `globs`, or `paths`.

Project learnings can be committed for teammates. Personal learnings stay in the checkout's gitignored local folder. Global learnings apply across projects on the machine. Claude Code loads project and personal learnings through its native rule files by default. Set `outputs.claude.rules-file: CLAUDE.md` to include both bodies in `CLAUDE.md`.

If `agnostic-ai` is not on PATH, the skill prints the command to run. After approved writes and sync attempts, it refreshes the handoff's git header, completed work, and verification notes. Failed or pending syncs stay recorded as unfinished work.

## Add automatic Git snapshots

Enable the separate `handoff-hook` built-in alongside the skill, then run `agnostic-ai sync`:

```yaml
requires: ">=0.80.0"
builtins: [handoff, handoff-hook]
```

`init` enables only the skill. The hooks are opt-in and support these targets:

| Target | Snapshot events | Arrival event and notice |
| --- | --- | --- |
| [Claude Code](https://code.claude.com/docs/en/hooks) | `PreCompact`, `SessionEnd` | `SessionStart`, JSON `systemMessage` |
| [Codex](https://learn.chatgpt.com/docs/hooks) | `PreCompact`, `SessionEnd` | `SessionStart`, JSON `systemMessage` |
| [Gemini CLI](https://geminicli.com/docs/hooks/reference/) | `PreCompress`, `SessionEnd` | `SessionStart`, JSON `systemMessage` |
| [Qoder CLI](https://docs.qoder.com/cli/hooks-reference.md) | `PreCompact`, `SessionEnd` | `SessionStart`, JSON `systemMessage` |
| [Factory / Droid](https://docs.factory.com/harness/hooks) | `PreCompact`, `SessionEnd` | `SessionStart`, one text line with `showHookOutput: true` |

The snapshot records the tool, UTC date, branch, full HEAD, `git status --short`, and the last five commit subjects in `.agnostic-ai/local/HANDOFF.auto.md`. It replaces that file atomically and leaves the last complete snapshot in place on a failed update. Detached HEAD is recorded as `detached`. Non-Git, bare, and unborn repositories produce no new snapshot. The hooks never open `HANDOFF.md`, diffs, transcripts, ignored file contents, or tool-owned memory stores.

The hooks use the same nearest-config root as the skill, starting from the hook process's working directory. They exclude the global source directory (`AGNOSTIC_AI_HOME`, or `~/.agnostic-ai` by default) from project-root discovery, including an ancestor directory or a symlink to it. With no project config, they fall back to the current Git worktree root. A configured subproject keeps its own snapshot. If the working directory has neither root, the hook skips the snapshot and notice.

On session start, a notice names the existing manual handoff, Git snapshot, or both, and suggests asking to resume. It checks only file existence. The automatic file holds Git evidence; the manual handoff supplies the goal and next steps. No task resumes until you request it.

The commands require `sh`, Git, `date`, `dirname`, `mkdir`, `mktemp`, `mv`, `rm`, and `cat` on PATH. Claude Code and Qoder use explicit Bash. Windows requires Git for Windows with its POSIX utilities available on PATH; native Windows vendor launches have not been verified. Commands execute inline and do not use helper scripts or cache paths.

Start Claude Code at the project root so it loads the emitted `.claude/settings.json`. [Claude reads shared project settings from its starting directory](https://code.claude.com/docs/en/settings#where-claude-code-keeps-the-local-file-in-a-git-repository).

The vendor's hook enablement and trust settings still apply. Review changed Codex definitions with `/hooks` and approve Gemini project hooks when prompted. Managed-only policies can exclude these hooks. Claude and Codex can cut off session-end work at their deadlines. Gemini `PreCompress` is asynchronous and `SessionEnd` is best effort, so neither event promises a completed update before compression or exit.

### Factory notices

Factory snapshots produce no stdout. To display its startup notice in the transcript, add this user-owned settings spec and sync. [Factory documents `showHookOutput`](https://docs.factory.com/droid-cli/settings) as the setting that displays hook stdout and stderr:

```yaml
# .agnostic-ai/settings/handoff-notices.yaml
name: handoff-notices
targets: [factory]
x-factory:
  showHookOutput: true
```

The built-in does not enable this preference. With neither handoff file present, the Factory startup hook prints nothing. With a file present, the single notice also enters session context. Handoff file contents are never injected. Factory hooks use project configuration; `sync --global` has no Factory hook surface.

### Global hooks and local files

Claude Code, Codex, Gemini CLI, and Qoder CLI can receive these hooks from the global home config through `agnostic-ai sync --global`. They still resolve the current project's root at runtime.

Project `agnostic-ai sync` adds the `.agnostic-ai/local/` ignore rule. A global-only hook or skill installation does not change a checkout's ignore files. Users running only global hooks or skills must ignore `.agnostic-ai/local/` in that checkout themselves.

## Resume in another tool

Open the other tool in the same checkout and say "continue where Codex left off" or "resume the handoff". The skill reads the handoff only after a request. It reports the goal and next steps, compares the branch and HEAD with git, and asks before continuing on a mismatch. This check also catches a handoff copied into another worktree.

If a newer `.agnostic-ai/local/HANDOFF.auto.md` exists, the skill uses that snapshot as the latest git state and keeps the written handoff's task details.

## Scope and overrides

Handoffs stay on the same machine. Project sync ignores `.agnostic-ai/local/`, so another checkout or teammate does not receive the file through a push. A globally installed skill still writes into the current project's local folder.

A project skill named `handoff` overrides the built-in. Pack skills also override it, and a personal local skill can extend the winning skill. `list` shows the winning layer. A custom project `handoff` next to a global built-in produces the usual shared-name warning. The same built-in enabled at both scopes shares one source and produces no warning.

Native imports keep bundled specs out of project source, so opting out and later binary updates still work. To customize a built-in, create a project spec with the same name. For a clash with a global built-in, rename that custom spec or disable the built-in in the global home config.

## Trace the built-in

```bash
agnostic-ai list --json
agnostic-ai explain builtin:handoff
agnostic-ai explain builtin:handoff --global --json
agnostic-ai why .claude/skills/handoff/SKILL.md --json
agnostic-ai status --json
agnostic-ai explain --inputs
```

Text reports show `builtin:handoff (agnostic-ai v0.80.0)` for that release, or `(agnostic-ai dev)` for a source build. JSON reports use an empty `path`, `layer: "builtin"`, and a `builtin` object with `name` and `version`. `explain --inputs` includes `builtin:handoff@<content-hash>` so an input digest changes when the bundled skill changes.

Built-in files are cached by content hash and are read-only. `migrate` skips them; lint and the language server omit findings on them. To customize one, write your own spec with the same name.

After an upgrade changes the built-in text, `sync --check` reports drift until you sync. Editors using an older release schema flag `builtins:` until the release with that schema ships. Set `requires` to at least 0.80.0 when enabling it.

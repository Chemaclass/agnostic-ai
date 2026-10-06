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

Each write replaces the previous handoff in that checkout. It excludes secrets and tool-owned memory stores. Git records the code's history. `sync --watch` ignores both `HANDOFF.md` and `HANDOFF.auto.md` in this local folder.

The skill uses the nearest ancestor with `agnostic-ai.yaml` or the legacy `agnostic.config.yaml` as the project root. Separately configured subprojects keep separate handoffs, including requests from a nested directory. A globally installed skill falls back to the Git worktree root when no project config exists.

## Resume in another tool

Open the other tool in the same checkout and say "continue where Codex left off" or "resume the handoff". The skill reads the handoff only after a request. It reports the goal and next steps, compares the branch and HEAD with git, and asks before continuing on a mismatch. This check also catches a handoff copied into another worktree.

If a newer `.agnostic-ai/local/HANDOFF.auto.md` exists, the skill uses that snapshot as the latest git state and keeps the written handoff's task details.

## Scope and overrides

Handoffs stay on the same machine. `.agnostic-ai/local/` is gitignored, so another checkout or teammate does not receive the file through a push. A globally installed skill still writes into the current project's local folder.

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

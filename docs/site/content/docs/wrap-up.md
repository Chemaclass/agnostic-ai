+++
title = "Session wrap-ups"
description = "End a session with a short summary of what changed, what is still open, and a note to share."
weight = 75

[extra]
group = "Workflows"
+++

# Session wrap-ups

The built-in `wrap-up` skill ends a session with a short summary for people. To pass a task to the next agent, use a [handoff](@/docs/handoff.md); for a parallel task, a [side conversation](@/docs/side-conversation.md).

## Enable the skill

Add this to `agnostic-ai.yaml`, then run `agnostic-ai sync`:

```yaml
requires: ">=0.94.0"
builtins: [wrap-up]
```

- Off unless you list it. For every project, add the same lines to `~/.agnostic-ai/agnostic-ai.yaml` and run `agnostic-ai sync --scope user`.
- Most tools get a skill; tools with `emit-skills-as-commands` also get a command. Aider gets inline text when `outputs.aider.rules-file` is set. Jules has no skill support.

## Ask for a wrap-up

Say "wrap up" or "write a wrap-up". The reply has three parts, in this order:

````markdown
## What changed
- Added a retry to the payment webhook (commits a1b2c3d, e4f5a6b).

## Still open
- The webhook tests are not run on Windows yet; no runner is set up.

## Note to share
```
The payment webhook now retries twice before it fails.
Windows tests are still missing.
```
````

An empty part says "nothing". The reply fits on one screen.

## What it reads and what it never does

- It reads the session and local Git: the current branch, `git status`, and `git log`.
- It makes no network call. It names a PR only when the session already showed one.
- The note has no secrets, no absolute home paths, and no text copied from files the session only read.
- It does not post, commit, push, or write a file unless you ask.
- It never runs by itself, at the start or at the end of a session.

## Wrap-up or handoff

| Skill | For | Result | When |
|---|---|---|---|
| `handoff` | The next agent | A private note per branch | Mid-task, to continue elsewhere |
| `wrap-up` | People | A reply in the conversation | When the work ends |

## See where it comes from

```bash
agnostic-ai list
agnostic-ai explain builtin:wrap-up
agnostic-ai why .claude/skills/wrap-up/SKILL.md
```

To replace the built-in, add a project skill named `wrap-up`; it wins. A user-level copy can still win on some tools ([shared names](@/docs/global-configuration-reference.md#global-shared-names)).

+++
title = "Side conversations"
description = "Start a second task in its own conversation, in a new terminal tab, linked to the session it came from."
weight = 77

[extra]
group = "Workflows"
+++

# Side conversations

Use the built-in `side-conversation` skill to follow a side idea in its own conversation without derailing the current task. The agent writes a short brief and starts a new conversation in a new terminal tab, in the same folder. You talk to the new conversation; the current one goes on.

## Enable the skill

Add this to `agnostic-ai.yaml`, then run `agnostic-ai sync`:

```yaml
requires: ">=0.94.0"
builtins: [side-conversation]
```

It is off unless you list it. For every project, add the same lines to `~/.agnostic-ai/agnostic-ai.yaml` and run `agnostic-ai sync --global`.

The skill runs [`agnostic-ai conversations start`](@/docs/cli-reference/conversations.md#conversations-start), so `agnostic-ai` must be on `PATH`. It works from Claude Code and Codex sessions on macOS; the command page lists [where it runs](@/docs/cli-reference/conversations.md#where-it-runs).

## Start one

Say "spin this off into its own conversation", or agree when the agent offers. The agent writes a brief in the handoff format, cut short:

```markdown
Write the release notes for the parser change.

## Context
Branch main. The change is in internal/parser/.

## Report back
The draft, and any change you were unsure about.
```

The first line is the goal and becomes the new conversation's title. The brief stays under 40 lines and holds no secrets. To start it in another tool, name it: "spin this off in Codex".

The agent says in one line what started, then goes on with its own task. It does not wait for the new conversation or read it.

The new conversation shows under this one in the [Conversations](@/docs/cli-reference/serve.md) page and the [companion](@/docs/cli-reference/companion.md), while both sit in the same group. The [link line](@/docs/cli-reference/conversations.md#the-link-line) lists when it is a normal row instead.

## What it never does

- It never opens a tab unless you ask or agree.
- It is not a subagent: nothing comes back to the session that started it. When the session needs the answer, the agent delegates instead.
- When the command refuses, the agent shows why and does not work around it.

## Side conversation, handoff or wrap-up

| Skill | For | This session | Result |
|---|---|---|---|
| `side-conversation` | A second task, started now | Goes on | A new conversation in a new tab |
| [`handoff`](@/docs/handoff.md) | The next agent on the same task | Ends or pauses | A private note per branch |
| [`wrap-up`](@/docs/wrap-up.md) | People, when the work ends | Ends | A reply in the conversation |

## See where it comes from

```bash
agnostic-ai list
agnostic-ai explain builtin:side-conversation
agnostic-ai why .claude/skills/side-conversation/SKILL.md
```

To replace the built-in, create your own project skill named `side-conversation`; it wins over the built-in.

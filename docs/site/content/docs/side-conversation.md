+++
title = "Side conversations"
description = "Start a second task in its own conversation, in a new terminal tab, linked to the session it came from."
weight = 77

[extra]
group = "Workflows"
+++

# Side conversations

The built-in `side-conversation` skill follows a side idea in its own conversation without derailing the current task. The agent writes a brief and opens a new conversation in a new terminal tab, same folder. The current one goes on.

## Enable the skill

Add this to `agnostic-ai.yaml`, then run `agnostic-ai sync`:

```yaml
requires: ">=0.94.0"
builtins: [side-conversation]
```

- Off unless you list it. For every project, add the same lines to `~/.agnostic-ai/agnostic-ai.yaml` and run `agnostic-ai sync --scope user`.
- It runs [`agnostic-ai conversations start`](@/docs/cli-reference/conversations.md#conversations-start), so `agnostic-ai` must be on `PATH`.
- It works from Claude Code, Codex and Cursor terminal agent (`cursor-agent`) sessions on macOS; see [where it runs](@/docs/cli-reference/conversations.md#where-it-runs).

## Start one

Say "spin this off into its own conversation", or agree when the agent offers. The agent writes a brief in the handoff format, cut short:

```markdown
Write the release notes for the parser change.

## Context
Branch main. The change is in internal/parser/.

## Report back
The draft, and any change you were unsure about.
```

- The first line is the goal and the new conversation's title. The brief stays under 40 lines and holds no secrets.
- To use another tool, name it: "spin this off in Codex".
- The agent says in one line what started, then continues. It does not wait for or read the new conversation.
- The new conversation shows under this one in the [Conversations](@/docs/cli-reference/serve.md) page and the [companion](@/docs/cli-reference/companion.md) while both sit in the same group. The [link line](@/docs/cli-reference/conversations.md#the-link-line) says when it is a normal row instead.

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

To replace the built-in, add a project skill named `side-conversation`; it wins. A user-level copy can still win on some tools ([shared names](@/docs/global-configuration-reference.md#global-shared-names)).

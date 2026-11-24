+++
title = "Conversations helpers API"
description = "The helpers field of the conversations list: subagents, agent team teammates and codex exec runs, how each is reached, and opening one."
weight = 47

[extra]
group = "Reference"
+++

# Conversations helpers API

## `helpers` {#helpers}

In the [conversations response](@/docs/cli-reference/conversations-api.md#get-api-views-conversations), `helpers` lists active helpers started by the conversation. These include native Claude Code and Codex subagents, Claude Code agent team teammates, and `codex exec` runs started from local Claude Code. Each helper is listed under its conversation, without a row of its own. Its `state` is always `working`. The field is omitted if there are no working helpers or their states cannot be read.

Each helper has `{tool, id, title, kind?, reply?, state, updatedAt, parentId?, reach?, parentName?}`. The tool supplies `kind`, such as `Explore`. If the tool gives no `title`, or the title may hold a credential, `title` is an empty string.

For Claude Code subagents, `reply` is the latest message in one line, read from the last 256 KB of the transcript. A later message addressed to the helper, such as one from a teammate, leaves the reply in place. The field is omitted if the helper has not replied, the reply may hold a credential, or the last reply is outside those 256 KB.

- `parentId` is the `id` of the helper that started this one. The list stays flat; a client builds the tree. The parent may be missing from the list, such as one that finished.
- `reach` says how to reach the helper's own view: `pane` (a terminal pane of its own), `rows` (the agent rows below its conversation's prompt), `nested` (inside another helper; `parentName` names it), or `codex-threads` (Codex's `/subagents` command). Pane ids and terminals stay out of the reply.
- A Claude Code subagent comes from the session's `subagents` folder and the meta file Claude Code writes beside each one. It is listed while the live process's task events say it works. A session no process runs lists none. It has no route of its own: open its conversation, with its `id` as `helper`.
- A Claude Code teammate is listed only while `~/.claude/teams/<team>/config.json` lists it, and the file names this conversation as its lead. A teammate in the lead's process works from a new message until its turn ends, as its own transcript says. A teammate in a tmux pane works while its team marks it active; its `id` is its member id, such as `reviewer@session-0d2fda98`.
- A Codex subagent is a child of a Codex thread. One that a subagent started is listed under the thread at the top, with `parentId` naming its parent. It is listed while a process holds its thread and its rollout says a turn runs. Open it with `POST /api/conversations/open` and its own `tool` and `id`. One in a terminal switches to the process that runs it, never a second `codex resume`.
- A `codex exec` run is linked to its Claude Code session only from the running `codex` process: the process holds the run's writer lock, and its environment holds `CLAUDE_CODE_SESSION_ID` and `CLAUDE_CODE_CHILD_SESSION=1`, which Claude Code's Bash tool sets. The server reads the environment the kernel keeps for the process (`kern.procargs2` on macOS, `/proc/<pid>/environ` on Linux), keeps only those two variables, and links nothing when either is missing or set twice. Every scan reads the link again; nothing is saved, so a run that ended is listed nowhere. Titles, folders and times are never used to guess. When process environments cannot be read, the Codex tool's `note` says so.
- `POST /api/conversations/open` opens a helper only when a conversation in the server's scope lists it.
- `workingSubagents` stays as it was and does not count `codex exec` runs.

## Open a helper {#open-a-helper}

Send `helper` with [`POST /api/conversations/open`](@/docs/cli-reference/conversations-api.md#post-api-conversations-open).

- `helper` is the `id` of a Claude Code subagent or teammate the conversation lists in `helpers`. A teammate with a tmux pane, or a subagent inside one, gets that pane: tmux selects it and its terminal tab comes to the front. Otherwise its conversation opens as without it, and `message` adds how to reach it: the down arrow and Enter below the prompt, the helper it runs inside, or, for a pane in a tmux session no terminal shows, `command` holds `tmux -L <socket> attach`. Nothing is typed into the terminal. The reply's `helper` is that helper. A helper the conversation no longer lists has finished: its conversation still opens, and `message` says so. A companion sends an open without `helper` again once to a server that refuses the field. A Codex subagent opens with its own `tool` and `id` instead; when its terminal came to the front, `message` says to type `/subagents`.
- `reached` is set when the open named a helper, or opened a Codex subagent: `subagent` when its own pane came to the front, `parent` when the conversation it runs in did and `message` says how to reach it. A reply without it, from an older server, means `parent`.

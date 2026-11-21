+++
title = "Conversations commands"
description = "Start a new conversation in a new terminal tab, linked to the session it starts from."
weight = 45

[extra]
group = "Reference"
+++

# Conversations commands

For actions on a listed conversation, see the [Conversations API](@/docs/cli-reference/conversations-api.md) and learn how to [stop a forgotten shell](@/docs/cli-reference/companion-list.md#stop-a-forgotten-shell).

## conversations start

Start a second conversation from a running session, in a new terminal tab in the same folder. The brief you pass becomes its first prompt. The session you run it from goes on with its own task. The [`side-conversation` skill](@/docs/side-conversation.md) tells the agent when to run it.

The [Conversations API](@/docs/cli-reference/conversations-api.md) names the session it came from as the new row's `parent`.

```bash
agnostic-ai conversations start <<'BRIEF'
Write the release notes for the parser change.

## Context
Branch main. The change is in internal/parser/.

## Report back
The draft, and any change you were unsure about.
BRIEF
```

Run from Claude Code in a Ghostty tab, it prints one line:

```text
Started Claude Code in a new Ghostty tab in /Users/me/shop, linked to claude:2f0c1e9a-5b7d-4c3e-9a1f-6d8e2b4c7a90.
```

| Flag | Description |
|------|-------------|
| `--tool <name>` | Start the new conversation in `claude`, `codex` or `cursor` (Cursor's terminal agent, `cursor-agent`). Without it, the new conversation uses the tool the command runs in. |
| `--brief-file <path>` | Read the brief from a file instead of standard input. |

The brief must not be empty and must fit in 4096 bytes. Start it with the goal as one plain sentence: it becomes the new conversation's title.

### Where it runs

It works from Claude Code, Codex, and Cursor's terminal agent (`cursor-agent`). A Cursor editor chat is refused.

{% <details summary="Which sessions can start one"> %}

| Your session runs in | The new conversation opens in |
|---|---|
| Claude Code | Works. The command finds the session from its parent processes. |
| Codex | Works when Codex sets `CODEX_THREAD_ID` for commands. Without it, it works only when the Codex process that runs the command holds one thread. |
| Cursor's terminal agent (`cursor-agent`) | Works. `cursor-agent` sets `CURSOR_CONVERSATION_ID` for the commands it runs. Without it, it works only when the `cursor-agent` process that runs the command has one chat open. |
| A Cursor editor chat | Refused: an editor chat does not run in a terminal tab. Run the command from `cursor-agent`, Claude Code or Codex. |

{% </details> %}

| Terminal | New conversation |
|---|---|
| Ghostty | A new tab in the front window |
| iTerm | A new tab in the front window |
| WezTerm | A new tab in the window the command ran in |
| kitty, with remote control on | A new tab in the active window |
| Terminal, macOS 26 or later | A new tab |
| Terminal, older macOS | A new window |
| Warp, an editor's terminal, or another app | A new Terminal window |
| Linux with `x-terminal-emulator` (Debian, Ubuntu) | A new window of that terminal |
| Other Linux, or Windows | Refused. The message gives the line to run in a terminal yourself. |

The first time, macOS may ask whether the terminal that runs the command may control Terminal, Ghostty or iTerm.

#### kitty remote control

kitty takes commands from other programs only with remote control on. Add these two lines to `kitty.conf`, then restart kitty:

```
allow_remote_control socket-only
listen_on unix:/tmp/kitty-{kitty_pid}
```

Then any program you run can control kitty through that socket, not only agnostic-ai. Without them, a kitty session's app comes forward and the message gives these lines. agnostic-ai never edits `kitty.conf`.

### When it refuses

<details>
<summary>All the cases where it refuses</summary>

It exits with code 1 and starts nothing when:

- It cannot tell which session it runs in: no Claude Code, Codex or `cursor-agent` session is among its parent processes, the nearest tool among them cannot be resolved, or the Codex or `cursor-agent` process holds several threads or chats and does not say which.
- It runs inside the Cursor editor with no session it can name in between, such as from a Cursor editor chat: an editor chat does not run in a terminal tab.
- The tool is not installed: its command is not on `PATH`.
- The brief is empty, longer than 4096 bytes, holds a null byte or text that is not UTF-8, or standard input is a terminal and no `--brief-file` was given.
- The folder or the path of `agnostic-ai` holds a control character (C0, DEL or C1) or text that is not UTF-8, which a terminal would act on as it is typed.
- No tab can be opened. The message then gives the line that starts it by hand.

</details>

### Limits

{% <details summary="Claude Code process detection"> %}

- A Claude Code process is recognised by its session file, or by the name `claude`. An install that runs as `node` and has no session file is not recognised. Started inside a Codex session, the command then names the Codex thread as the parent, not that Claude Code session.

{% </details> %}

### The link line

The new conversation's first prompt starts with one line that names the session it came from, then a blank line, then your brief:

```text
Continues claude:2f0c1e9a-5b7d-4c3e-9a1f-6d8e2b4c7a90

Write the release notes for the parser change.
```

<details>
<summary>How the link line is read and shown</summary>

- The line is `Continues <tool>:<id>`, where `<tool>` is `claude`, `codex` or `cursor`, and `<id>` has that tool's id form. It must be the whole first line of what was typed, with one space after `Continues` and nothing after the id. Text a tool adds around the prompt, such as a reminder block before it, does not count. Anything else is plain text: spaces before the line, two spaces, a tab, or words after the id.
- The Conversations list reads it as the [`parent`](@/docs/cli-reference/conversations-api.md#get-api-views-conversations) of the row, and cuts it from the title, the opening and the latest prompt.
- The Conversations page and the companion draw the new conversation under its parent while both sit in the same group, such as Open now, or Needs you in the companion. Otherwise it is a normal row in its own group: the parent sits in another group, is on a week not loaded yet, is filtered out by the search, tool or place, or is hidden or deleted. A conversation started from a child sits under the same parent, one level deep.
- You can type the line yourself at the start of a new conversation. It only decides where the row is shown.
- The tool's own history still shows the line.
- Hiding, archiving, renaming or deleting the parent never changes the new conversation.

</details>

### What runs where

<details>
<summary>How the brief reaches the new tab</summary>

The brief never becomes part of a shell or AppleScript string. The command saves it to a one-time file in agnostic-ai's own settings folder (`conversation-briefs/`, readable by you only, under a random name). The new tab runs a fixed line, `cd '<folder>' && '<agnostic-ai>' conversations run <id>`. That reads the brief, finds the tool, removes the file, and starts the tool with the brief as its one argument. Running the line a second time starts nothing. When the tool is missing from that tab's `PATH`, the brief is kept, so the line works once the tool is there. A brief no tab ran is removed after a day.

</details>

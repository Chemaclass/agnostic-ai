+++
title = "Cloud handoffs"
description = "Send the current task to a Claude Code or Codex cloud session that goes on after you close your laptop."
weight = 79

[extra]
group = "Workflows"
+++

# Cloud handoffs

The built-in `cloud-handoff` skill sends the task you are working on to a cloud session that goes on without this computer. The cloud session starts from a fresh clone, so the agent first pushes the work and writes down the goal. To pass a task to another tool on this machine, use a [handoff](@/docs/handoff.md); for a parallel task here, a [side conversation](@/docs/side-conversation.md).

## Enable the skill

Add this to `agnostic-ai.yaml`, then run `agnostic-ai sync`:

```yaml
requires: ">=0.96.0"
builtins: [cloud-handoff]
```

- Off unless you list it, because it pushes and starts paid work in the cloud. `init` does not turn it on. For every project, add the same lines to `~/.agnostic-ai/agnostic-ai.yaml` and run `agnostic-ai sync --global`.
- Most tools get a skill; tools with `emit-skills-as-commands` also get a command. Aider gets inline text when `outputs.aider.rules-file` is set, so its sessions carry the skill's words every time. Jules has no skill support.

## Say the phrase

Say "send this to a remote agent", "run it in the cloud", or "I'm closing my laptop". The agent then:

1. Checks that the folder is a Git checkout with an `origin` remote and that the tool's command is installed.
2. Commits the task's files and pushes them to a branch.
3. Writes the cloud prompt: the goal on its first line, then what is done, the next step, the branch, and how to verify. It asks the cloud session to aim any pull request at that branch, not the default branch.
4. Starts the cloud session and replies with its link, such as `Sent to the cloud: https://claude.ai/code/session_...`.

It never runs on its own. Without the phrase, the session stays local.

## What gets pushed

- Only the files of this task. The agent shows `git status --short` first.
- Never a `.env` file, a key, or anything else that looks like a secret.
- Never the default branch. On the default branch or a detached HEAD, it creates a new branch and names it.
- No force-push. A failing commit hook or push stops the handoff with its message.

The cloud session starts from the pushed branch. A Claude Code session puts its own work on a new `claude/...` branch, so your branch stays yours; a Codex task returns a diff.

## What each tool needs

| Tool | Command it runs | You need |
|---|---|---|
| Claude Code | `claude --cloud "<prompt>"`, inside `script` | A claude.ai sign-in (`claude auth login`), and `script`, which macOS and Linux have |
| Codex | `codex cloud exec --env "<id>" --branch "<branch>" -` | A ChatGPT sign-in (`codex login`) and a cloud environment id |
| Other tools | none | The agent says so and offers a local [handoff](@/docs/handoff.md) note |

- `claude --cloud` only starts from a terminal. The agent runs it inside `script`, reads the link, then stops the local process; the cloud session keeps running. On Windows, where `script` is missing, the agent gives you the command to run yourself.
- Codex needs an environment id; the agent asks for it once per session and does not store it. Create or find one at [chatgpt.com/codex/settings/environments](https://chatgpt.com/codex/settings/environments), or browse them with `codex cloud` in a terminal. The `codex cloud` command is marked experimental.

## Where the session shows up

- The agent's last reply starts with the session link, so the local chat shows where the work went.
- The session shows as a Cloud row in the [companion](@/docs/cli-reference/companion.md#claude-code-cloud-sessions) within about a minute, and in the global web interface when it runs with `agnostic-ai serve --global --cloud-sessions`, after a reload. It opens in the browser for Claude Code and in the Codex app for Codex. A project web interface lists no cloud sessions.
- Bring the work back with `claude --teleport <session-id>`, or with `codex cloud diff <url>` and then `codex cloud apply <url>`.

## Limits

- Claude Code cloud sessions are listed on macOS and Linux, where the companion reads the claude.ai sign-in from the macOS keychain or from Claude Code's `.credentials.json` (not yet tried with a real Linux sign-in). On Windows the handoff still works (you run the Claude Code command yourself), and no Claude Code row shows, without a message.
- Claude Code names the session itself. The prompt's first line can shape that title, but does not set it.
- A Codex task runs on the repository its environment names, not on this checkout's remote. Pick an environment for this repository.
- The prompt holds no chat history: only what the agent writes in it.
- Both cloud commands come from their vendors and can change with any release.

## See where it comes from

```bash
agnostic-ai list
agnostic-ai explain builtin:cloud-handoff
agnostic-ai why .claude/skills/cloud-handoff/SKILL.md
```

To replace the built-in, add a project skill named `cloud-handoff`; it wins. A global copy can still win on some tools ([shared names](@/docs/global-configuration-reference.md#global-shared-names)).

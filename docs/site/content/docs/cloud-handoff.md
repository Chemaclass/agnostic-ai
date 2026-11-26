+++
title = "Cloud handoffs"
description = "Send the current task to a Claude Code, Codex or GitHub Copilot cloud session that goes on after you close your laptop."
weight = 79

[extra]
group = "Workflows"
+++

# Cloud handoffs

Use the built-in `cloud-handoff` skill to continue a task in the cloud while your computer is off. The agent pushes the work and records the goal before it launches the session. Use a [handoff](@/docs/handoff.md) to pass a task to another tool on this machine. Start a [side conversation](@/docs/side-conversation.md) to run another task in parallel here.

## Enable the skill

Add this to `agnostic-ai.yaml`, then run `agnostic-ai sync`:

```yaml
requires: ">=0.96.0"
builtins: [cloud-handoff]
```

- Off unless you list it, because it pushes and starts paid work in the cloud. `init` does not turn it on. For every project, add the same lines to `~/.agnostic-ai/agnostic-ai.yaml` and run `agnostic-ai sync --scope user`.
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

The cloud session starts from the pushed branch. A Claude Code session puts its own work on a new `claude/...` branch. A Codex task returns a diff. A GitHub Copilot task opens a pull request against the pushed branch.

For Claude Code, the agent starts the launch in a temporary clone of the pushed branch, with one commit, one branch and no tags. It checks that the clone is clean and matches the commit it pushed. This keeps other branches, older local commits and uncommitted files out of the upload when Claude Code uses a [local repository bundle](https://code.claude.com/docs/en/claude-code-on-the-web#send-local-repositories-without-github). If the clone or either check fails, the handoff stops.

## What each tool needs

| Tool | Command it runs | You need |
|---|---|---|
| Claude Code | `claude --cloud "<prompt>"`, inside `script` | A claude.ai sign-in (`claude auth login`), and `script`, which macOS and Linux have |
| Codex | `codex cloud exec --env "<id>" --branch "<branch>" -` | A ChatGPT sign-in (`codex login`) and a cloud environment id |
| GitHub Copilot | `gh agent-task create -F - --base "<branch>"` | `gh` 2.80 or newer, signed in (`gh auth login`), and Copilot coding agent enabled for the repository |
| Other tools | none | The agent says so and offers a local [handoff](@/docs/handoff.md) note |

- `claude --cloud` only starts from a terminal. The agent runs it inside `script`, reads the link and stops the local process. The cloud session keeps running. On Windows, where `script` is missing, the agent gives you the command to run from the verified temporary clone.
- Codex needs an environment id. The agent first reads `git config --get agnostic-ai.codex-cloud-env`. If that is empty, it asks you for the id and stops. After a launch with an id you gave, it offers to save it with `git config --local agnostic-ai.codex-cloud-env <id>`, and saves it only if you agree. The id is not a secret; it stays in this checkout's local Git config, out of commits. If Codex reports the saved id as not found, the agent says it is stale and asks for a new one; clear it with `git config --local --unset agnostic-ai.codex-cloud-env`. Create or find an id at [chatgpt.com/codex/settings/environments](https://chatgpt.com/codex/settings/environments), or browse them with `codex cloud` in a terminal. The `codex cloud` command is marked experimental.
- The agent uses the GitHub Copilot row when it runs in Copilot, such as VS Code or Copilot CLI, or when you name Copilot. `gh` sends the task to the repository it picks for this checkout; with more than one remote, the agent checks `gh repo set-default --view` names the repository `origin` points to.

## Where the session shows up

- The agent's last reply starts with the session link, so the local chat shows where the work went.
- The session shows as a Cloud row in the [companion](@/docs/cli-reference/companion.md#claude-code-cloud-sessions) within about a minute, and in the user web interface when it runs with `agnostic-ai serve --scope user --cloud-sessions`, after a reload. It opens in the browser for Claude Code and in the Codex app for Codex. A project web interface lists no cloud sessions.
- For Claude Code, **Copy resume command** in the companion or web Conversations gives you `claude --teleport <session-id>` to run in a terminal. For Codex, use `codex cloud diff <url>` and then `codex cloud apply <url>`.
- A GitHub Copilot task is not listed in the companion. It shows as a pull request against the pushed branch, and in `gh agent-task list`; follow it with `gh agent-task view`.

## Limits

- Claude Code cloud sessions are listed on macOS, Linux and Windows. The companion reads the claude.ai sign-in from the macOS keychain, or from Claude Code's `.credentials.json` (not yet tried with a real Linux or Windows sign-in).
- Claude Code names the session itself. The prompt's first line can shape that title, but does not set it.
- A Codex task runs on the repository its environment names, not on this checkout's remote. Pick an environment for this repository.
- The prompt holds no chat history: only what the agent writes in it.
- `gh agent-task` is marked preview. What it prints after a launch was not checked, so the agent shows its output and takes the task or pull request link from it when there is one; otherwise use `gh agent-task list`.
- The cloud commands come from their vendors and can change with any release.

## See where it comes from

```bash
agnostic-ai list
agnostic-ai explain builtin:cloud-handoff
agnostic-ai why .claude/skills/cloud-handoff/SKILL.md
```

To replace the built-in, add a project skill named `cloud-handoff`; it wins. A user-level copy can still win on some tools ([shared names](@/docs/global-configuration-reference.md#global-shared-names)).

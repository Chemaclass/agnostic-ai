+++
title = "Use RTK and Caveman with agnostic-ai"
description = "Set up RTK for shorter command output or Caveman for shorter answers, and compare their costs."
weight = 78

[extra]
nav_title = "RTK and Caveman"
group = "Workflows"
+++

# Use RTK and Caveman with agnostic-ai

[RTK](https://github.com/rtk-ai/rtk) shortens terminal command output before the agent reads it. [Caveman](https://github.com/JuliusBrussee/caveman/tree/main/skills/caveman) makes the agent answer in fewer words. agnostic-ai sets up either. In our small test, combining them saved no extra cost ([results](#test-results)).

## Turn them on

Add the names to `builtins` in `agnostic-ai.yaml`, next to any you already use:

```yaml
version: 1
targets: [claude]
builtins: [rtk]
```

Both are off by default; list both with `builtins: [rtk, caveman]`. Then run:

```sh
agnostic-ai sync
```

| Built-in | Adds | Notes |
|---|---|---|
| `rtk` | A hook in `.claude/settings.json` (a command Claude Code runs before each shell command) | Claude Code only; Bash tool on macOS or Linux, not PowerShell |
| `caveman` | `SKILL.md` in `.claude/skills/caveman/` and in every other tool that supports skills | Type `/caveman` to start. License and notices: [upstream](https://github.com/JuliusBrussee/caveman) |

- agnostic-ai installs neither program. Install RTK from its [guide](https://github.com/rtk-ai/rtk#installation); without it, commands run as normal.
- Preview an RTK rewrite without running it: `agnostic-ai doctor rtk --command 'git status'`.

## What happens on a command

```text
Claude asks to run: git status
RTK changes it to:  rtk git status
Claude checks its permission rules on the new command
RTK runs it and returns shorter output
```

Claude checks the changed command, so an "ask first" rule for `git status` needs a twin for `rtk git status` ([tested cases](https://github.com/Chemaclass/agnostic-ai/blob/main/docs/internal/experiments/trio-live-hooks.md)).

## If you already use RTK or Caveman

- Keep one copy. If RTK is already on for all projects, two hooks can run on one command; leave the built-in off or remove your own copy first.
- A project hook identical to the RTK one is taken over by agnostic-ai, and removing `rtk` later deletes it. Save a copy first to keep it.

## Turn them off

- Say `stop caveman` to get normal answers again in this session.
- Remove the name from `builtins` and run `agnostic-ai sync`. Use `builtins: []` to turn off all built-ins.
- `disabled: true` has no effect on the RTK hook.

A copy you installed yourself, outside agnostic-ai, needs its own removal.

## Share with a team

The [example packs](https://github.com/Chemaclass/agnostic-ai/tree/main/docs/examples/rtk-and-caveman) offer RTK and Caveman as [packs](@/docs/packs.md) instead of built-ins. Use one or the other for each tool.

<details>
<summary>Distinguish the Caveman program and read the test results</summary>

<a id="caveman-runtime-is-a-separate-choice"></a>

## Caveman's output shrinking is separate

Caveman can also run as a program that shortens command output; the built-in does not turn that on. In our test, RTK cut one failing test log from 23,865 to 240 bytes and Caveman cut nothing more. See the [details](https://github.com/Chemaclass/agnostic-ai/blob/main/docs/internal/rtk-caveman-runtime.md) before using both programs.

## Test results

We ran 24 short debugging sessions in Claude Code 2.1.295 with RTK 0.51.0 and `claude-haiku-5-5`:

| Turned on | Cost on larger tasks | Speed |
| --- | --- | --- |
| RTK | Lowest | Slower in 4 of 6 runs |
| Caveman | No saving | Slower in all 6 |
| Both | More than RTK alone | Slower in all 6 |

All 24 answers were correct. On a short command RTK does not shorten, RTK did not help.

These are price estimates from a small test, not your bill; measure a full task in your project. See the [full results](https://github.com/Chemaclass/agnostic-ai/blob/main/docs/internal/experiments/trio-evaluation-results.md) and the [investigation](https://github.com/Chemaclass/agnostic-ai/issues/1957).

</details>

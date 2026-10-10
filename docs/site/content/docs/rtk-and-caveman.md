+++
title = "Use RTK and Caveman with agnostic-ai"
description = "Turn on RTK for shorter command output and Caveman for shorter answers, and what each one costs."
weight = 71

[extra]
group = "Workflows"
+++

# Use RTK and Caveman with agnostic-ai

[RTK](https://github.com/rtk-ai/rtk) shortens the output of common terminal commands before the coding agent reads it. [Caveman](https://github.com/JuliusBrussee/caveman/tree/main/skills/caveman) is a skill that asks the agent to answer in fewer words. agnostic-ai can set up either one for you.

Start with RTK if you want lower cost. In our tests, adding Caveman on top of RTK cost more, not less. Pick Caveman only if you like its style.

## Turn them on

Add the names to `builtins` in `agnostic-ai.yaml`, next to any you already use:

```yaml
version: 1
targets: [claude]
builtins: [rtk]
```

Use `builtins: [rtk, caveman]` for both. Both are off by default. Then run:

```sh
agnostic-ai sync
```

- **RTK** adds a hook to `.claude/settings.json`. A hook is a command Claude Code runs before each shell command. It works only in Claude Code, and only for the Bash tool on macOS or Linux, not PowerShell.
- **Caveman** adds a skill to `.claude/skills/caveman/` and to every other tool that supports skills. Type `/caveman` to start it.

Install RTK yourself with its [install guide](https://github.com/rtk-ai/rtk#installation). If RTK is missing, commands run as normal. agnostic-ai does not install either tool. `doctor rtk` runs RTK to preview a rewrite without executing the supplied command.

## What happens on a command

```text
Claude asks to run: git status
RTK changes it to:  rtk git status
Claude checks its permission rules on the new command
RTK runs it and returns shorter output
```

Claude checks permissions against the changed command. If you have an "ask first" rule for `git status`, add one for `rtk git status` too. See the [tested permission cases](https://github.com/Chemaclass/agnostic-ai/blob/main/docs/internal/experiments/trio-live-hooks.md).

## If you already use RTK or Caveman

Keep only one copy. If RTK is already on for all your projects, two hooks can run for the same command. Either leave the built-in off or remove your own copy first.

If your project already has a hook exactly like the RTK one, agnostic-ai takes it over. Removing `rtk` later deletes it. Save a copy first if you want to keep it.

## Turn them off

- Say `stop caveman` to get normal answers again in this session.
- Remove the name from `builtins` and run `agnostic-ai sync`. Use `builtins: []` to turn off all built-ins.
- Do not use `disabled: true` for the RTK hook. It has no effect on this hook.

A copy you installed yourself, outside agnostic-ai, needs its own removal.

## Share with a team

The [example packs](https://github.com/Chemaclass/agnostic-ai/tree/main/docs/examples/rtk-and-caveman) offer RTK and Caveman as separate [packs](../packs/) instead of built-ins. Use a pack or a built-in for each tool, not both.

<a id="caveman-runtime-is-a-separate-choice"></a>

## Caveman's output shrinking is separate

Caveman can also run as a program that shortens command output. The `caveman` built-in does not turn that on. In our test, RTK cut one failing test log from 23,865 bytes to 240 bytes, and Caveman cut nothing more. See the [details](https://github.com/Chemaclass/agnostic-ai/blob/main/docs/internal/rtk-caveman-runtime.md) before using both programs together.

## Test results

We ran 24 short debugging sessions in Claude Code 2.1.295 with RTK 0.51.0 and `claude-haiku-5-5`:

| Turned on | Cost on larger tasks | Speed |
| --- | --- | --- |
| RTK | Lowest | Slower in 4 of 6 runs |
| Caveman | No saving | Slower in all 6 |
| Both | More than RTK alone | Slower in all 6 |

All 24 answers were correct. On a short command RTK does not shorten, RTK did not help.

These are price estimates from a small test, not your bill. Measure a full task in your own project before you decide. See the [full results](https://github.com/Chemaclass/agnostic-ai/blob/main/docs/internal/experiments/trio-evaluation-results.md) and the [investigation](https://github.com/Chemaclass/agnostic-ai/issues/1957).

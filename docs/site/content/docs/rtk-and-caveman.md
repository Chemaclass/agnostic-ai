+++
title = "Use RTK and Caveman with agnostic-ai"
description = "Optional setup for shared instructions, smaller command output and concise responses, with measured results."
weight = 71

[extra]
group = "Workflows"
+++

# Use RTK and Caveman with agnostic-ai

agnostic-ai keeps your project instructions and hooks (commands that run when the coding tool reaches a named event) in one place. [RTK](https://github.com/rtk-ai/rtk) reduces supported terminal output. [Caveman's response skill](https://github.com/JuliusBrussee/caveman/tree/main/skills/caveman) asks the coding agent to write concise answers. Each tool keeps its own job.

Start with RTK alone when reducing the cost of noisy commands is your goal. Add Caveman when you want its response style. The tests below found no extra cost or speed benefit from adding Caveman to RTK.

**Experimental setup.** The earlier manual recipe was tested on macOS with agnostic-ai 0.81.0 and RTK 0.51.0. The built-in skill retains Caveman revision [`2e08b917`](https://github.com/JuliusBrussee/caveman/tree/2e08b9177c07bb7249a8a2d1a6758e5db281d002/skills/caveman). Local checks cover enabling each YAML option, generated files, missing RTK, and removal. Live Claude Code 2.1.295 tests cover running hooks and applying approval rules without an interactive approval dialog. Interactive dialogs and other coding tools have not been tested.

## Enable either feature

Add the names you want to the `builtins` list in `agnostic-ai.yaml`. Keep any built-ins you already use:

```yaml
version: 1
targets: [claude]
builtins: [rtk]
```

This example enables RTK alone. Add `caveman` when you want its response style, or use `builtins: [rtk, caveman]` to enable both. Keep other names already in your list. Both are off by default, including in projects created by `init`. A personal `agnostic-ai.local.yaml` can replace the shared list for one machine. These names require a build that includes this feature; agnostic-ai 0.81.0 does not support them.

Install RTK separately using its [installation guide](https://github.com/rtk-ai/rtk#installation). The shell running the hook must be able to find `rtk` on its `PATH`. If it cannot, the hook leaves the command unchanged for Claude to handle. Sync does not run or install either third-party tool.

The `rtk` built-in adds RTK's Claude hook for Bash. The hook needs a POSIX-compatible shell, such as `sh` on macOS or Linux. It does not handle Claude's PowerShell tool. See Claude's [hook shell requirements](https://code.claude.com/docs/en/hooks#command-hook-fields). Other targets receive no RTK hook. The `caveman` built-in adds a copy of the default response skill from the recorded Caveman revision to coding tools that support skills. It needs no Caveman executable. Running Caveman as a program to shrink command output, or using its `ultracave` and `megacave` modes, requires a separate choice.

If RTK or Caveman is already enabled for all your projects through a hook or plugin, choose which installation you will keep before adding a project copy. Two RTK hooks can run for the same command even when their settings use different command strings. Keep the existing installation or move its setup to the project. If an existing project hook exactly matches the generated RTK hook, sync starts managing it. Removing `rtk` then removes that hook. Leave the built-in off to keep the existing setup, or save a copy of the hook before letting sync manage it.

## Sync and inspect

```sh
agnostic-ai sync
agnostic-ai sync --check
```

Review the generated RTK entry in `.claude/settings.json` and the generated Caveman skill in `.claude/skills/caveman/`. Change the `builtins` list to enable or remove either feature. A project spec with the same kind and name takes precedence over the built-in spec.

For a supported shell command, the intended flow is:

```text
Claude requests git status
  → RTK's hook returns rtk git status
  → Claude applies its execution and approval rules
  → RTK runs the command and returns filtered output
```

Invoke `/caveman` in Claude Code before the task to request the response style. Installing the skill makes it available; automatic activation is not assumed. Caveman changes prose; code, commands, paths, and errors still need to remain exact.

RTK replies tell Claude which command to run instead. Do not add `decision: stdout` to this hook: that agnostic-ai option expects a different JSON format. Allow only the commands you intend to run; enabling RTK does not require permission to run every RTK command.

Claude evaluates permissions against the rewritten command. In the live test, an ask rule for `git status` plus an allow rule for `rtk git status` let the rewritten command run without asking. Apply ask rules to the RTK form of the command too. See the [tested approval cases](https://github.com/Chemaclass/agnostic-ai/blob/main/docs/internal/experiments/trio-live-hooks.md) before changing your permissions.

## Turn it off or remove it

Say `stop caveman` to return to normal prose in the current session.

To remove either feature, remove its name from `builtins` and run `agnostic-ai sync`. Use `builtins: []` to turn off every built-in for that config. Check the resulting diff. A hook or plugin installed separately for all projects needs its own removal step. Do not use `disabled: true` to turn off this Claude hook; that field does not disable it on this target.

## Share the setup

The [project example](https://github.com/Chemaclass/agnostic-ai/tree/main/docs/examples/rtk-and-caveman) provides separate RTK and Caveman [spec packs](../packs/). Add either component independently. Its instructions cover keeping an existing installation, transferring one selected hook, and restoring that handler after pack removal.

A local check script tests generated files, removing each feature separately, missing tools, and failed writes. The packs include source files from the recorded revisions and their licenses. Pack removal is followed by sync so generated entries are removed too.

The packs are an alternative to the built-ins. Choose either a pack or a built-in for each tool so the same hook or skill is managed in one place.

## Shrinking output with Caveman is a separate choice

Leave Caveman's command-output compression off when starting. In a Linux test, RTK reduced an ordinary failing transcript from 23,865 bytes to 240 bytes; Caveman made no further reduction. A constructed repetitive failure tail did shrink further, from 4,248 bytes to 2,215 bytes. These measurements use generated test output. They do not measure the cost of a coding session.

Running both programs to shrink output requires two local stores for retrieving the removed text. Caveman retrieves the RTK summary; RTK separately retrieves the original command output. Both stores must remain available. The tested command-running interface kept exit status 7 and returned the RTK summary unchanged when Caveman's store was unavailable. See the [runtime results and reproduction steps](https://github.com/Chemaclass/agnostic-ai/blob/main/docs/internal/rtk-caveman-runtime.md) before enabling this separate runtime feature.

Measure a complete task with and without the enabled components. Include added instructions, repeat attempts, reads of saved output, and answer correctness when measuring model usage. Smaller command output is useful evidence, but it is not a measured reduction in the cost of completing the task. See Caveman's [measurement notes](https://github.com/JuliusBrussee/caveman/blob/main/docs/HONEST-NUMBERS.md).

## What complete sessions measured

In 24 synthetic diagnostic sessions with Claude Code 2.1.295, RTK 0.51.0 and `claude-haiku-5-5`, RTK alone had the lowest observed CLI price estimate for two larger failing test transcripts. Adding Caveman cost more than RTK alone in every comparison of the same task and repetition. Each task had two repetitions. These estimates are not account charges or predictions for your project.

| Enabled feature | Two larger tasks | Short, unsupported command |
| --- | --- | --- |
| RTK | Lower estimate than concise instructions in both repetitions of each task | Mixed cost direction; command and output unchanged |
| Caveman | No cost benefit established | Higher estimate in both repetitions |
| Both | Lower than concise instructions, higher than RTK alone | Higher estimate in both repetitions |

A separate model reviewer accepted all 24 answers without seeing which setup produced them or what they cost. The original automated check accepted 23 because it rejected one correct statement naming the failed command. The report retains that disagreement and its cost; a later fix only changes future checks. Claude also shortened four larger outputs before the model read them, so that behavior is part of the comparison.

Use RTK for supported noisy commands after checking that the required details survive. Keep it off for this short diagnostic task when lower cost is the goal. For the lowest observed cost on these larger tasks, use `builtins: [rtk]`. Choose Caveman when you want its response style. Invoking its skill adds instructions and a tool call. This study found no extra cost benefit from adding it to RTK. Both features remain off by default.

RTK finished slower in four of six comparisons and faster in two. Caveman and both together finished slower in all six comparisons. This study does not establish a fastest setup or an overall best setup for other projects.

Read the [complete results, task-by-task comparisons and recorded evidence](https://github.com/Chemaclass/agnostic-ai/blob/main/docs/internal/experiments/trio-evaluation-results.md) before applying these observations to another workload. The [collaboration investigation](https://github.com/Chemaclass/agnostic-ai/issues/1957) also records setup, removal, approval and command-output checks.

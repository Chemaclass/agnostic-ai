+++
title = "Use RTK and Caveman with agnostic-ai"
description = "Opt-in setup for shared instructions, smaller command output and concise responses, with measured tradeoffs."
weight = 71

[extra]
group = "Workflows"
+++

# Use RTK and Caveman with agnostic-ai

agnostic-ai keeps your project instructions and hooks in one place. [RTK](https://github.com/rtk-ai/rtk) reduces supported terminal output. [Caveman's response skill](https://github.com/JuliusBrussee/caveman/tree/main/skills/caveman) asks the coding agent to write concise answers. Each tool keeps its own job.

**Experimental setup.** The earlier manual recipe was tested on macOS with agnostic-ai 0.81.0 and RTK 0.51.0. The built-in skill retains Caveman revision [`2e08b917`](https://github.com/JuliusBrussee/caveman/tree/2e08b9177c07bb7249a8a2d1a6758e5db281d002/skills/caveman). Local checks cover the YAML opt-ins, generated files, missing RTK, and removal. Live Claude Code 2.1.295 tests cover native hook execution and approval rules in noninteractive manual mode. Interactive approval dialogs and other hosts have not been tested.

## Enable either feature

Add the names you want to the `builtins` list in `agnostic-ai.yaml`. Keep any built-ins you already use:

```yaml
version: 1
targets: [claude]
builtins: [rtk, caveman]
```

Add `rtk`, `caveman`, or both to your existing `builtins` list. Both are off by default, including in projects created by `init`. A personal `agnostic-ai.local.yaml` can replace the shared list for one machine. These names require a build that includes this feature; agnostic-ai 0.81.0 does not support them.

Install RTK separately using its [installation guide](https://github.com/rtk-ai/rtk#installation). RTK must be on the hook process's `PATH`. If it is absent, the generated hook returns no replacement and leaves command handling to Claude. Sync does not run or install either third-party tool.

The `rtk` built-in adds RTK's native Claude command hook for Bash and requires a POSIX shell. It does not handle Claude's PowerShell tool. See Claude's [hook shell requirements](https://code.claude.com/docs/en/hooks#command-hook-fields). Other targets receive no RTK hook. The `caveman` built-in adds the pinned default response skill to hosts that support skills. It needs no Caveman executable. Runtime compression and the `ultracave` and `megacave` companion modes are separate choices.

If RTK or Caveman is already active through a global hook or plugin, choose which installation owns it before adding a project copy. Two RTK registrations can run for the same command even when their command strings differ. Keep the existing installation or explicitly move ownership to the project sources. If an existing project handler exactly matches the generated RTK handler, sync adopts it. Removing `rtk` then removes that handler. Leave the built-in off to keep the existing owner, or save the handler before transferring ownership.

## Sync and inspect

```sh
agnostic-ai sync
agnostic-ai sync --check
```

Review the generated RTK entry in `.claude/settings.json` and the generated Caveman skill in `.claude/skills/caveman/`. Change the `builtins` list to enable or remove either feature. A project spec with the same kind and name overrides the bundled spec.

For a supported shell command, the intended flow is:

```text
Claude requests git status
  → RTK's native hook returns rtk git status
  → Claude applies its execution and approval rules
  → RTK runs the command and returns filtered output
```

Invoke `/caveman` in Claude Code before the task to request the response style. Installing the skill makes it available; automatic activation is not assumed. Caveman changes prose; code, commands, paths, and errors still need to remain exact.

RTK replies contain a replacement tool input. Do not add `decision: stdout` to this hook: that agnostic-ai option expects a different JSON format. Keep approval rules specific to the commands you intend to allow; a blanket RTK allowlist is not needed to configure the integration.

Claude evaluates permissions against the rewritten command. In the live test, an ask rule for `git status` plus an allow rule for `rtk git status` let the rewritten command run without asking. Keep ask rules aligned with the rewritten form too. See the [tested approval cases](https://github.com/Chemaclass/agnostic-ai/blob/main/docs/internal/experiments/trio-live-hooks.md) before changing your permissions.

## Turn it off or remove it

Say `stop caveman` to return to normal prose in the current session.

To remove either feature, remove its name from `builtins` and run `agnostic-ai sync`. Use `builtins: []` to turn off every built-in for that config. Check the resulting diff. A separately installed global hook or plugin remains owned by that installation and needs its own removal step. Do not use `disabled: true` to turn off this Claude hook; that field does not disable it on this target.

## Share the setup

The [project example](https://github.com/Chemaclass/agnostic-ai/tree/main/docs/examples/rtk-and-caveman) provides separate RTK and Caveman [spec packs](../packs/). Add either component independently. Its instructions cover keeping an existing installation, transferring one selected hook, and restoring that handler after pack removal.

A local verifier checks generated files, independent removal, missing tools, and failed writes. The packs include pinned source and license files. Pack removal is followed by sync so generated entries are removed too.

The packs are an alternative to the built-ins. Choose one source for each component so the same hook or skill has one owner.

## Caveman runtime is a separate choice

Keep Caveman runtime compression off for the initial setup. In a Linux test, RTK reduced an ordinary failing transcript from 23,865 bytes to 240 bytes; Caveman made no further reduction. A constructed repetitive failure tail did shrink further, from 4,248 bytes to 2,215 bytes. These are synthetic output measurements, not provider savings.

Combining the runtimes adds two recovery stores. Caveman retrieves the RTK summary; RTK separately retrieves the original command output. Both stores must remain available. The tested command wrapper preserved exit status 7 and passed the RTK summary through when Caveman's store was unavailable. See the [runtime results and reproduction steps](https://github.com/Chemaclass/agnostic-ai/blob/main/docs/internal/rtk-caveman-runtime.md) before enabling this separate runtime feature.

Measure a complete task with and without the enabled components. Count prompt overhead, retries, recovery reads, and task correctness alongside provider usage. Smaller command output is useful evidence, but it is not a measured reduction in the cost of completing the task. See Caveman's [measurement notes](https://github.com/JuliusBrussee/caveman/blob/main/docs/HONEST-NUMBERS.md).

## What complete sessions measured

In 24 synthetic diagnostic sessions with Claude Code 2.1.295, RTK 0.51.0 and `claude-haiku-5-5`, RTK alone had the lowest observed CLI price estimate for two larger failing test transcripts. Adding Caveman cost more than RTK alone in every paired task. Each task had two repetitions. These are CLI list-price estimates, not billed savings or predictions for your project.

| Enabled feature | Two larger tasks | Short, unsupported command |
| --- | --- | --- |
| RTK | Lower estimate than concise instructions in both repetitions of each task | Mixed cost direction; command and output unchanged |
| Caveman | No cost benefit established | Higher estimate in both repetitions |
| Both | Lower than concise instructions, higher than RTK alone | Higher estimate in both repetitions |

An independent blind model review accepted all 24 answers. The original automated check accepted 23 because it rejected one correct statement naming the failed command. The report retains that disagreement and its cost; a later fix only changes future checks. Claude also truncated four unfiltered larger outputs, so the comparison includes its native output handling.

Use RTK for supported noisy commands after checking that the required details survive. Keep it off for this short diagnostic task when lower cost is the goal. Choose Caveman when you want its response style. Its explicit skill activation adds instructions and a tool turn, and this study found no extra cost benefit from adding it to RTK. Both features remain off by default.

Read the [complete results, paired values and raw evidence](https://github.com/Chemaclass/agnostic-ai/blob/main/docs/internal/experiments/trio-evaluation-results.md) before applying these observations to another workload. The [collaboration investigation](https://github.com/Chemaclass/agnostic-ai/issues/1957) also records lifecycle, approval and runtime checks.

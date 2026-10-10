+++
title = "Use RTK and Caveman with agnostic-ai"
description = "An experimental project recipe for shared setup, smaller command output, and concise responses."
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

Use `[rtk]` or `[caveman]` to enable one. Both are off by default, including in projects created by `init`. A personal `agnostic-ai.local.yaml` can replace the shared list for one machine. These names require a build that includes this feature; agnostic-ai 0.81.0 does not support them.

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

## What the local experiments show

The project example passed 56 checks across seven fixtures. It covers independent pack selection, repeated sync, missing RTK, ownership transfer, removal, and failed writes. Unrelated handwritten hooks and settings survived. The skill, license, and notice files were preserved. These checks used a synthetic Claude-shaped request, rather than a launched coding session.

Standalone Caveman CLI 2.1.0 with runtime `bin-v2.1.0` compressed a repetitive 37,874-byte test transcript to 271 bytes and recovered the exact original. A failing command retained exit code 7. Missing-engine and short-input cases returned the original bytes.

In a separate synthetic test log, RTK reduced 21,220 bytes to 239 bytes and kept the failure and path. Passing that summary to Caveman gave no additional reduction. This supports testing each component separately before adding another compression step. No provider usage or billed cost was measured.

Track the remaining work in the [project collaboration investigation](https://github.com/Chemaclass/agnostic-ai/issues/1957): runtime composition and whole-task measurements. The live-host report records tested versions, duplicate registrations, removal, and approval differences.

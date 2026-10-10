+++
title = "Use RTK and Caveman with agnostic-ai"
description = "An experimental project recipe for shared setup, smaller command output, and concise responses."
weight = 71

[extra]
group = "Workflows"
+++

# Use RTK and Caveman with agnostic-ai

agnostic-ai keeps your project instructions and hooks in one place. [RTK](https://github.com/rtk-ai/rtk) reduces supported terminal output. [Caveman's response skill](https://github.com/JuliusBrussee/caveman/tree/main/skills/caveman) asks the coding agent to write concise answers. Each tool keeps its own job.

**Experimental project recipe.** Tested on macOS with agnostic-ai 0.81.0, RTK 0.51.0, and the Caveman skill revision below. Local checks cover generated files, direct RTK hook replies, and removal. Live Claude execution and approvals are still being investigated. Linux has not been tested.

## Add the project sources

Keep `claude` among the targets in your existing `agnostic-ai.yaml`:

```yaml
version: 1
targets: [claude]
```

Add `.agnostic-ai/hooks/rtk-shell-output.yaml`:

```yaml
name: rtk-shell-output
description: Use RTK for supported shell commands.
target: claude
event: PreToolUse
matcher: Bash
command: 'command -v rtk >/dev/null 2>&1 || exit 0; rtk hook claude'
```

RTK must be available on the host hook's PATH. The hook passes the request to RTK's own Claude processor. When RTK is absent, it returns no replacement and leaves the command to Claude's normal handling. The hook does not install RTK or run the requested command itself.

Install RTK separately using its [installation guide](https://github.com/rtk-ai/rtk#installation). Review the pinned Caveman source before fetching it:

```sh
(
set -eu
caveman_revision=2e08b9177c07bb7249a8a2d1a6758e5db281d002
caveman_source="https://raw.githubusercontent.com/JuliusBrussee/caveman/$caveman_revision"
mkdir -p .agnostic-ai/skills/caveman
curl -fL "$caveman_source/skills/caveman/SKILL.md" -o .agnostic-ai/skills/caveman/SKILL.md
curl -fL "$caveman_source/skills/caveman/README.md" -o .agnostic-ai/skills/caveman/README.md
for caveman_notice in LICENSE LICENSE-MIT NOTICE; do
  curl -fL "$caveman_source/$caveman_notice" -o ".agnostic-ai/skills/caveman/$caveman_notice"
done
)
```

Keep the revision with the sources you commit. This recipe uses the default `/caveman` response skill. Its `ultracave` and `megacave` companion modes are not included. The response skill needs no Caveman CLI.

Your project sources now look like this:

```text
.agnostic-ai/
├── hooks/
│   └── rtk-shell-output.yaml
└── skills/
    └── caveman/
        └── SKILL.md
```

If RTK or Caveman is already active through a global hook or plugin, choose which installation owns it before adding a project copy. Two RTK registrations can run for the same command even when their command strings differ. Keep the existing installation or explicitly move ownership to the project sources.

## Sync and inspect

```sh
agnostic-ai sync
agnostic-ai sync --check
```

Review the generated RTK entry in `.claude/settings.json` and the generated Caveman skill in `.claude/skills/caveman/`. Edit the source files when changing the setup.

For a supported shell command, the intended flow is:

```text
Claude requests git status
  → RTK's native hook returns rtk git status
  → Claude applies its execution and approval rules
  → RTK runs the command and returns filtered output
```

Invoke `/caveman` in Claude Code before the task to request the response style. Installing the skill makes it available; automatic activation is not assumed. Caveman changes prose; code, commands, paths, and errors still need to remain exact.

RTK replies contain a replacement tool input. Do not add `decision: stdout` to this hook: that agnostic-ai option expects a different JSON format. Keep approval rules specific to the commands you intend to allow; a blanket RTK allowlist is not needed to configure the integration.

## Turn it off or remove it

Say `stop caveman` to return to normal prose in the current session.

To remove the project integration, delete the RTK hook source and the Caveman skill source, then run `agnostic-ai sync`. Check the resulting diff. A separately installed global hook or plugin remains owned by that installation and needs its own removal step. Do not use `disabled: true` to turn off this Claude hook; that field does not disable it on this target.

## Share the setup

The [project example](https://github.com/Chemaclass/agnostic-ai/tree/main/docs/examples/rtk-and-caveman) provides separate RTK and Caveman [spec packs](../packs/). Add either component independently. Its instructions cover keeping an existing installation, transferring one selected hook, and restoring that handler after pack removal.

A local verifier checks generated files, independent removal, missing tools, and failed writes. The packs include pinned source and license files. Pack removal is followed by sync so generated entries are removed too.

No new `integrations` key is required. A convenience setting is an open design question only if hooks, skills, and packs leave a concrete setup problem unsolved.

## Caveman runtime is a separate choice

Keep Caveman runtime compression off for the initial setup. In a Linux test, RTK reduced an ordinary failing transcript from 23,865 bytes to 240 bytes; Caveman made no further reduction. A constructed repetitive failure tail did shrink further, from 4,248 bytes to 2,215 bytes. These are synthetic output measurements, not provider savings.

Combining the runtimes adds two recovery stores. Caveman retrieves the RTK summary; RTK separately retrieves the original command output. Both stores must remain available. The tested command wrapper preserved exit status 7 and passed the RTK summary through when Caveman's store was unavailable. See the [runtime results and reproduction steps](https://github.com/Chemaclass/agnostic-ai/blob/main/docs/internal/rtk-caveman-runtime.md) before enabling this separate runtime feature.

Measure a complete task with and without the enabled components. Count prompt overhead, retries, recovery reads, and task correctness alongside provider usage. Smaller command output is useful evidence, but it is not a measured reduction in the cost of completing the task. See Caveman's [measurement notes](https://github.com/JuliusBrussee/caveman/blob/main/docs/HONEST-NUMBERS.md).

## What the local experiments show

The project example passed 56 checks across seven fixtures. It covers independent pack selection, repeated sync, missing RTK, ownership transfer, removal, and failed writes. Unrelated handwritten hooks and settings survived. The skill, license, and notice files were preserved. These checks used a synthetic Claude-shaped request, rather than a launched coding session.

Standalone Caveman CLI 2.1.0 with runtime `bin-v2.1.0` compressed a repetitive 37,874-byte test transcript to 271 bytes and recovered the exact original. A failing command retained exit code 7. Missing-engine and short-input cases returned the original bytes.

In a separate synthetic test log, RTK reduced 21,220 bytes to 239 bytes and kept the failure and path. Passing that summary to Caveman gave no additional reduction. This supports testing each component separately before adding another compression step. No provider usage or billed cost was measured.

Track the remaining work in the [project collaboration investigation](https://github.com/Chemaclass/agnostic-ai/issues/1957): existing installation ownership, live-host approvals, runtime composition, and whole-task measurements.

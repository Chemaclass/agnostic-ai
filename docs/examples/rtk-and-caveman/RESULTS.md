# Recorded configuration and hook checks

Captured 2026-10-10 using `agnostic-ai version 0.81.0`, `rtk 0.51.0`. The final verifier passed 56 checks. 4 sync checks exited 0. The verifier ran seven separate Claude-only test projects.

## Captured sync output

RTK alone:

```text
  + .claude/rules/fixture.md  .agnostic-ai/AGNOSTIC_AI.md  CLAUDE.md
  ~ .claude/settings.json
✓ synced 1 target · 3 created · 1 updated · 26ms
✓ claude now reads, from .agnostic-ai/:
    instructions     CLAUDE.md
    1 rule           .claude/rules/         fixture
    1 hook           .claude/settings.json  rtk-shell-output
  edit .agnostic-ai/ and run agnostic-ai sync to change what every tool reads
```

Then add the Caveman skill:

```text
  + skill caveman → 6 files in 1 target
  + .claude/skills/caveman/SKILL.md  .claude/skills/caveman/LICENSE  .claude/skills/caveman/LICENSE-MIT  (+3 more)
✓ synced 1 target · 6 created · 19ms
```

Repeat sync:

```text
✓ 1 target up to date · 10ms
```

<a id="captured-native-settings"></a>

## Captured Claude settings

```json
{
  "env": {
    "EXAMPLE_SENTINEL": "preserve",
    "AGNOSTIC_AI_TARGET": "claude"
  },
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Read",
        "hooks": [
          {
            "type": "command",
            "command": "printf sentinel"
          }
        ]
      },
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "command -v rtk >/dev/null 2>&1 || exit 0; rtk hook claude"
          }
        ]
      }
    ]
  }
}
```

<a id="captured-hook-protocol"></a>

## Captured hook request and reply

Claude-shaped request sent to the exact generated command:

```json
{
  "hook_event_name": "PreToolUse",
  "tool_name": "Bash",
  "permission_mode": "default",
  "tool_input": {
    "command": "git status",
    "description": "preserve",
    "timeout": 1000
  }
}
```

Reply from installed RTK:

```json
{
  "hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "permissionDecisionReason": "RTK auto-rewrite",
    "updatedInput": {
      "command": "rtk git status",
      "description": "preserve",
      "timeout": 1000
    }
  }
}
```

Already-prefixed `rtk git status` and unsupported `printf unsupported` both returned exit 0 and empty stdout. The missing-executable check returned exit 0 and empty stdout, and did not create the marker file its test command would create.

<a id="lifecycle-results"></a>

## Add, remove, and restore results

| Check | Result |
|---|---|
| Install RTK without Caveman, then add Caveman | Passed |
| Remove either pack while the other stays available | Passed |
| Repeat sync and sync with an empty PATH retain output bytes | Passed |
| Unrelated handwritten hook and environment survive all syncs | Passed |
| Skill instructions from the fixed revision, license texts, notices, and copied README survive sync | Passed |
| Keep the existing RTK-managed hook by omitting its pack | Passed |
| Move one selected RTK hook command to agnostic-ai, remove the pack, and restore the command | Passed |
| Refuse duplicate restoration or an unmatched transfer without edits | Passed |
| Restore a hook when settings have no hooks object | Passed |
| Keep, move, and restore an existing Caveman-managed skill directory | Passed |
| Source configuration and rule retain their original bytes | Passed |

Repeat sync changed only `.agnostic-ai/.command-lock` and `.agnostic-ai/.sync-state`. The evidence records their before/after hashes separately. Generated files and source hashes matched.

The first skill-restoration test project omitted the parent directory that sync pruned after removal. The corrected test project and documented restore command recreate `.claude/skills` before restoring the directory.

These checks cover local configuration and Claude's JSON requests and replies. They do not show that a running Claude session used a rewritten command or Caveman skill, that global installations changed, that a separate Caveman program compressed requests, or that model-provider token use or costs fell. The test restores selected local settings; it does not test an external recovery system.

Run the [verifier](README.md#reproduce-the-checks) to capture the complete command output and check results for your machine.

## Failed-write and no-execution checks

The final run included a controlled file-size limit in a child process. The old helper truncated the Claude settings file when its write failed. The fixed helper left all original bytes and permissions intact, removed its temporary file, and kept the selected-hook backup private. Symlinked settings files and directories were refused without changing their targets.

The missing-RTK test command uses shell builtins and redirection, so it can execute even with an empty PATH. Running the test command directly first created its marker; passing it to the hook then returned no reply and left the marker absent. These checks supplement the captured add, remove, and restore output above.

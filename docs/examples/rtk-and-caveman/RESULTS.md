# Recorded configuration and protocol checks

Captured 2026-10-10 using `agnostic-ai version 0.81.0`, `rtk 0.51.0`. All 48 assertions passed. 4 sync checks exited 0. The verifier ran four isolated Claude-only project fixtures.

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

## Captured native settings

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

## Captured hook protocol

Host-shaped request sent to the exact emitted command:

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

Native reply from installed RTK:

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

Already-prefixed `rtk git status` and unsupported `printf unsupported` both returned exit 0 and empty stdout. The missing-binary probe returned exit 0 and empty stdout, and did not create its payload marker.

## Lifecycle results

| Check | Result |
|---|---|
| Install RTK without Caveman, then add Caveman | Passed |
| Remove either pack while the other stays available | Passed |
| Repeat sync and sync with an empty PATH retain output bytes | Passed |
| Unrelated handwritten hook and environment survive all syncs | Passed |
| Pinned skill body, license texts, notices, and bundled README survive emission | Passed |
| Keep existing upstream RTK ownership by omitting its pack | Passed |
| Transfer one selected RTK handler, remove pack, and restore handler | Passed |
| Refuse duplicate restoration or an unmatched transfer without edits | Passed |
| Restore a hook when settings have no hooks object | Passed |
| Keep, transfer, and restore an existing upstream skill directory | Passed |
| Source configuration and rule retain their original bytes | Passed |

Repeat sync changed only `.agnostic-ai/.command-lock` and `.agnostic-ai/.sync-state`. The evidence records their before/after hashes separately. Native output and source hashes matched.

The first skill-restoration fixture omitted the parent directory that sync pruned after removal. The corrected fixture and documented restore command recreate `.claude/skills` before restoring the directory.

These are local configuration and native JSON protocol checks. They do not establish that a running Claude session applied a replacement or activated Caveman, that global ownership changed, that a Caveman runtime compressed requests, or that provider tokens or costs fell. The fixture restores local selected configuration records; it does not validate an external recovery system.

Run the [verifier](README.md#reproduce-the-checks) to capture the complete command and assertion transcript for your machine.

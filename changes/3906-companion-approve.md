### General

- An `x-<target>` block on a hook spec now sets `timeout` for that target, in seconds like the top-level `timeout`, as it already set `event`, `matcher`, `command` and `args`. A spec that already had such a block gets that timeout on its next sync (#3906).

### By tool

#### Claude Code

- With `approvals.companion: true` in the home config, the companion shows a Claude Code request left to you in full, with **Approve** and **Deny**. A click answers that one call; the terminal prompt stays open and still works. The approvals hook waits up to 300 seconds for the click, only while the companion is open, and only for commands and file edits it can show in full. The Claude Code hook entry's timeout is now 310 seconds. See [Answer from the companion](https://agnostic-ai.org/docs/approvals/#answer-from-the-companion) (#3906).

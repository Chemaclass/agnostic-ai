### General

- An `x-<target>` block on a hook spec now also sets `statusMessage` for that target. A spec that already had one gets it on its next sync (#3906).

### By tool

#### Codex

- With `approvals.companion: true`, the companion shows a Codex request left to you, with **Approve**, **Deny** and **Answer in terminal**. The hook waits at most 60 seconds and Codex shows "Waiting for your answer in the agnostic-ai companion, then asks here" meanwhile ([docs](https://agnostic-ai.org/docs/approvals/#answer-from-the-companion), #3906).
- The Codex approvals hook entry now has a 70 second timeout and a status line. After `sync --scope user`, Codex stops at startup with **Hooks need review** once for every approvals user; trust the hook again to keep hook approvals (#3906).

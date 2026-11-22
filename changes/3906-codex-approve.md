### General

- An `x-<target>` block on a hook spec now also sets `statusMessage` for that target. A spec that already had one gets it on its next sync (#3906).

### By tool

#### Codex

- With `approvals.companion: true`, the companion shows a Codex request left to you, with **Approve**, **Deny** and **Answer in terminal**. The hook waits at most 60 seconds and Codex shows "Waiting for your answer in the agnostic-ai companion, then asks here" meanwhile. The approvals hook entry changes, so Codex asks once to review hooks ([docs](https://agnostic-ai.org/docs/approvals/#answer-from-the-companion), #3906).

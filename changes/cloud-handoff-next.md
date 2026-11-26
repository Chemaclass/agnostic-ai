### By tool

#### Codex

- `cloud-handoff` reads the Codex environment id from `git config agnostic-ai.codex-cloud-env`. After a launch with an id you gave, it offers to save the id there.

#### Copilot

- `cloud-handoff` can send the task to the Copilot coding agent with `gh agent-task create` (preview). The work comes back as a pull request against the pushed branch.

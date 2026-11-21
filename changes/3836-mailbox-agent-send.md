### General

- Agents can use the session mailbox themselves: `agnostic-ai mailbox send`, `done`, `list`, `show`, `claim`, `release`, `accept` and `decline` run from inside a Claude Code or Codex session and act as that session. The `mailbox` built-in now also adds a `mailbox` skill to both tools, so your next `sync` writes it. Agent messages wait for your approval in the companion, and the thread shows which messages an agent sent, the model it says it runs and who accepted a handoff (#3836).

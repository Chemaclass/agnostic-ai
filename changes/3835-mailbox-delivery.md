### General

- The `mailbox` built-in delivers waiting mailbox messages to Claude Code and Codex with the next prompt, and when a session starts or resumes. Codex runs the two new hooks only after you trust them in `/hooks`, and asks again when they change (#3835).
- Messages an agent sends wait in the companion as **Held for you** until you approve or reject them. **Queue for delivery** hands a staged message back to the hook. Update the companion and the CLI together: an older one cannot read a thread the hook changed (#3835).

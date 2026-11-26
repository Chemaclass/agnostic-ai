### By tool

#### Codex

- The protected paths hook finds the project root from the folder sync writes it to, not from a fixed two levels up. A copy elsewhere uses the nearest `agnostic-ai.yaml`, then the Git worktree root, and blocks the edit when it finds neither. In a project below its Git root, the hook command now adds the project's path, so the script is found and the guard runs.

#### Gemini CLI

- The protected paths hook finds the project root from the folder sync writes it to, not from a fixed two levels up. A copy elsewhere uses the nearest `agnostic-ai.yaml`, then the Git worktree root, and blocks the edit when it finds neither.

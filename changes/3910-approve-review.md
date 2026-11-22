### General

- Only the companion that started its server can answer or read a held request: the server needs a secret the companion passes to it at launch, so another account on the machine can no longer approve one (#3910).
- A held request shows the folder it runs in, and a command shows whether it runs in the background and its time limit. A request without a folder, or with an option the row cannot show, stays in the terminal (#3910).
- A held Codex request no row can show goes to the Codex prompt 5 seconds after the hook holds it, even while the companion's list reads change nothing (#3910).
- Opening a teammate's tmux pane matches only a Claude Code process, and opens the lead when more than one process matches (#3910).

### By tool

#### Claude Code

- After you answer a prompt in the terminal and Claude Code asks again, the companion never offers the earlier request. When two held requests could be the one that waits, the row shows no buttons (#3910).

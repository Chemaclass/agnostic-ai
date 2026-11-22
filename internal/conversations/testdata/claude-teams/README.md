# Claude Code agent team fixtures

Captured from Claude Code 2.1.330 on macOS. Prompts, folders, and the lead's verify key are removed.

- `in-process-*`, `nested-subagent.meta.json`: a lead with a teammate in its own process, and a subagent that teammate started.
- `tmux-*`: a lead inside tmux with `teammateMode: "tmux"`; the teammate runs in a split pane of the lead's tmux session.
- `swarm-*`: the same lead outside tmux; Claude Code starts its own tmux server, `claude-swarm-<lead pid>`, with no client attached. `swarm-list-panes.txt` is `tmux list-panes -a -F '#{pid}\t#{pane_tty}\t#{pane_id}\t#{session_id}\t#{window_id}'` on that server.
- `*-ps.txt`: the teammate process's command line.
- `tmux-teammate-pid.txt` and `tmux-list-panes.txt`: `ps -o pid,ppid,tty,args` of the teammate, and `tmux list-panes -a -F '#{pid}\t#{pane_tty}\t#{pane_id}\t#{session_id}\t#{window_id}'`, where `#{pid}` is the server's pid. The teammate's parent, 56159, is the tmux server: it runs as the pane's own process.

A pane teammate writes its own session transcript and no `sessions/<pid>.json`, so it does not show as a live row of its own.

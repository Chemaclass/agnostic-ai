### General

- `doctor` warns, without failing, when Git ignores generated files and nothing regenerates them after a pull or branch switch, and names the cloud agents (Codex Cloud, Jules, Copilot cloud agent, Claude Code on the web) whose root instruction file is ignored.
- `init` closing notes, with ignored outputs, name `install-hook --post-checkout` and say that cloud agents read only committed files.
- `lint` and `memory lint` warn (LINT067) when the `memory` built-in is on and `.agnostic-ai/memory/MEMORY.md` is missing, so the `CLAUDE.md` import loads nothing. `memory index` creates the missing project index, and `init --memory` starts an empty one.

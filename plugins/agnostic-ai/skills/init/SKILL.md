---
name: init
description: Set up agnostic-ai in a project by scaffolding the spec directories, picking target AI CLIs, and producing the first generated config. Use when a project has no .agnostic-ai/ directory yet.
---

# init

Turns a project with no shared AI config into one where a single set of specs drives every tool.

## Steps

1. Confirm the CLI is present: `agnostic-ai --version`. If missing, run the `install` skill first.
2. Check whether the project already has AI config worth keeping: `CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, `.cursor/rules/`, `.github/copilot-instructions.md`. If any exist, stop and use the `import` skill instead; it captures them as specs rather than overwriting them.
3. Scaffold:

   ```bash
   agnostic-ai init --all      # every target, no prompt
   agnostic-ai init --demo     # plus one example spec per kind
   ```

   `init` alone opens a target picker when stdin is a TTY. In a non-interactive session, pass `--all` or pipe a list: `echo "claude,codex" | agnostic-ai init`.
4. Ask which AI tools the team actually uses, then narrow `targets:` in `agnostic-ai.yaml` to those. Fewer targets means fewer generated files to review.
5. Write the project's real conventions as rules under `.agnostic-ai/rules/`, one file per rule. Keep each rule to a single concern with a `description` that says when it applies.
6. Run `agnostic-ai sync`, then show the user what appeared with `git status --short`.

## Notes

- `init` writes `gitignore.enabled: true` by default: generated files stay out of git and each contributor runs `sync`. Pass `--gitignore=false` to commit them instead, which suits a team where not everyone has the CLI.
- One spec per file. `agnostic-ai new rule <name>` (also `agent`, `skill`, `hook`, `mcp`) scaffolds the right frontmatter for the kind.
- Never hand-edit a generated file such as `CLAUDE.md` or `.cursor/rules/*.mdc`. The next `sync` overwrites it. Edit the spec under `.agnostic-ai/` instead.

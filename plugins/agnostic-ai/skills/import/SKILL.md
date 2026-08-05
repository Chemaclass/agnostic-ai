---
name: import
description: Adopt a project's existing AI config (CLAUDE.md, AGENTS.md, .cursor/rules, and friends) into agnostic-ai specs, then fan it out to every other tool. Use when the project already has config for one AI CLI and should feed the rest.
---

# import

Captures what one tool already knows and makes it drive all of them, without retyping the conventions.

## Steps

1. Confirm the CLI is present: `agnostic-ai --version`. If missing, run the `install` skill first.
2. Inventory what exists. Check for `CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, `CONVENTIONS.md`, `.cursor/rules/`, `.cline/rules/`, `.devin/rules/`, `.continue/rules/`, `.github/copilot-instructions.md`, and any `.claude/{agents,skills,settings.json}`.
3. Scaffold the config if absent: `agnostic-ai init`.
4. Import each source you found, one command per tool:

   ```bash
   agnostic-ai import claude     # CLAUDE.md + .claude/{agents,skills,settings.json}
   agnostic-ai import codex      # AGENTS.md, root and nested
   agnostic-ai import cursor     # .cursor/rules/*.mdc
   agnostic-ai import cline      # .cline/rules/, .cline/agents/
   agnostic-ai import windsurf   # .devin/rules/
   agnostic-ai import continue   # .continue/rules/
   ```

   `import` writes spec files only. It never edits `targets:` or the rest of the config.
5. Commit the import before syncing, then sync as its own commit:

   ```bash
   git add .agnostic-ai/ agnostic-ai.yaml
   git commit -m "chore(agnostic-ai): import existing config"
   agnostic-ai sync
   git add -A
   git commit -m "chore(agnostic-ai): regenerate per-target configs"
   ```

   Splitting them keeps the review honest: the first commit carries the real content, the second is mechanical (headers, key order) and can be skimmed.
6. Read the imported specs with the user. An imported rule often mixes several concerns because it came from one long file; split it into one file per rule so scoping and per-target emission work properly.

## Notes

- Importing from several tools: run each `import` and its commit before the final `sync`.
- Round-trip is byte-stable: `import` strips the generated markers it wrote, so a later `sync` produces the same bytes.
- What `import` does not capture: helper scripts referenced from `settings.json`, `statusline.sh`, and ad-hoc config files. Keep those in git next to `.agnostic-ai/`. Files bundled inside a skill directory do round-trip.

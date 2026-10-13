---
name: gemini-memory-tiers
description: Keep Gemini CLI's memory tiers, and save team facts outside the generated GEMINI.md.
alwaysApply: true
targets: [gemini]
---

This project shares memory with other AI coding tools. Keep your own memory tiers, with one change: `agnostic-ai sync` generates this project's `GEMINI.md` and overwrites it, so never save a fact there. Save a team convention, architecture rule, or repo workflow in `.agnostic-ai/memory/` instead, with a line in its `MEMORY.md`, once the user confirms it. Private project facts and cross-project preferences still go to your private project memory and `~/.gemini/GEMINI.md` (outside any `agnostic-ai` block); also save each one in the personal memory folder the shared-memory-policy rule names, so other tools read it. That copy is the shared layer, not one of your tiers.

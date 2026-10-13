---
name: gemini-memory-tiers
description: Where Gemini CLI's memory tiers map in a project that shares memory.
alwaysApply: true
targets: [gemini]
---

This project shares memory with other AI coding tools, so your memory tiers map to its files. Project instructions tier (team conventions, architecture rules, repo workflows): propose the fact and, once the user confirms, save it in `.agnostic-ai/memory/` with a line in its `MEMORY.md`; never edit `GEMINI.md`, which `agnostic-ai sync` generates and overwrites. Private project memory tier (your machine, local setup, not committed): the personal memory folder the shared-memory-policy rule names. Global personal memory tier (preferences across projects): `~/.gemini/GEMINI.md`, outside any `agnostic-ai` block. One fact, one file, as your routing rules say.

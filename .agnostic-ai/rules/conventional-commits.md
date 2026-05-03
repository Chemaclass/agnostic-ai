---
name: conventional-commits
description: Always use Conventional Commits format for git commits.
globs: "**/*"
alwaysApply: true
---

Use Conventional Commits for every commit:

- `feat:` new feature (new adapter, new spec kind, new flag)
- `fix:` bug fix
- `docs:` documentation only
- `refactor:` code change without feature or fix
- `test:` tests only
- `chore:` build, deps, CI

Subject line under 72 chars. Body explains why, not what. Never mention AI assistance in commit messages.

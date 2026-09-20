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
- `ref:` code change without feature or fix. Not `refactor:`: this repo is
  split three to two the other way in its own history, and the gh-issue
  skill has always said `ref:`, so the shorter one wins and the two now agree
- `test:` tests only
- `chore:` build, deps, CI

Subject line under 72 chars. Body explains why, not what. Never mention AI assistance in commit messages.

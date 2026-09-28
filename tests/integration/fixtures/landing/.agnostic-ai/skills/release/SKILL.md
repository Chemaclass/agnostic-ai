---
name: release
description: Cut a release and verify the published package.
---

1. Run `./check.sh` from this skill folder. Stop if it fails.
2. Bump the version with `pnpm version <patch|minor|major>`.
3. Push the tag and wait for the release workflow.
4. Install the new version in a scratch folder and run `--version`.

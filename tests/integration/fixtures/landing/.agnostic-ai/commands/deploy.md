---
name: deploy
description: Deploy the current branch to an environment.
argument-hint: <env>
---

Deploy the current branch to {{env}}.

1. Run `pnpm test` and stop on any failure.
2. Build with `pnpm build`.
3. Run `fly deploy --config fly.{{env}}.toml`.
4. Reply with the release URL.

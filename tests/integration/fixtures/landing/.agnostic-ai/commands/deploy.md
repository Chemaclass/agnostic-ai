---
name: deploy
description: Deploy to an environment.
argument-hint: <env>
---

Run `pnpm test`, then `fly deploy --config fly.{{env}}.toml`.

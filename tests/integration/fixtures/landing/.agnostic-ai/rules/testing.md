---
name: testing
description: How tests are written in this repo.
globs: ["**/*.test.ts"]
---

- One behavior per test, named after that behavior.
- Use the factories in `test/factories/`, never raw fixtures.
- No network: mock HTTP with `msw`.

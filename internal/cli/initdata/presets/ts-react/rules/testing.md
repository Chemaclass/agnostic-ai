---
name: testing
description: Test conventions for TypeScript + React.
globs: "**/*.{test,spec}.{ts,tsx}"
alwaysApply: true
---

- Vitest or Jest, one runner per repo. Do not mix.
- Component tests use `@testing-library/react`. Query by accessible role, not by `data-testid`, unless the role is genuinely missing.
- Avoid snapshot tests for components that change often; assert on the parts you care about.
- Mock at the network boundary (MSW), not at the module boundary. Module mocks rot.
- One `describe` per file maximum; nested `describe` is a smell.

---
name: typescript
description: TypeScript style with strict typing.
globs: "**/*.{ts,tsx}"
alwaysApply: true
---

- `strict: true` in `tsconfig.json`. No implicit `any`, no implicit `this`.
- Prefer `unknown` over `any` for boundary values; narrow before use.
- Use `type` for unions and aliases, `interface` for object shapes meant to be extended.
- Avoid `enum`; use union of string literals (`type Status = "open" | "closed"`).
- Imports use the project's path aliases when configured; do not write deep relative paths.
- Public functions carry explicit return types so refactors do not silently widen the surface.

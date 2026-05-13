---
name: react
description: React component conventions.
globs: "**/*.{tsx,jsx}"
alwaysApply: true
---

- Function components only. No class components in new code.
- One component per file. File name matches the component (`Button.tsx` exports `Button`).
- Hooks at the top of the body. No conditional hook calls. No hooks inside loops.
- Props typed as a named `Props` type, not inline. Optional props get explicit defaults.
- Inline event handlers OK for simple cases; extract when they grow past three lines.
- Server-bound data lives in a query/state hook (TanStack Query, SWR, Redux); components are presentational.

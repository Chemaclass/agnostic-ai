---
name: learnings
description: Preserve Unicode escapes when editing Go source.
globs: "**/*.go"
alwaysApply: false
---

- Some edit tools turn `\uXXXX` in Go source into the literal character. Write `\U0000XXXX` instead.

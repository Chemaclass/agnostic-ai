---
name: typing
description: Type-checking discipline (mypy / pyright).
globs: "**/*.py"
alwaysApply: true
---

- Run `mypy --strict` (or `pyright` in strict mode) in CI. New code must pass.
- Public functions and class attributes carry annotations. Private helpers may infer.
- Use `typing.Final` for module-level constants meant not to be reassigned.
- Avoid `Any` at module boundaries. Prefer `object`, `TypedDict`, or a `Protocol` for structural typing.
- `Optional[X]` is `X | None`. Pick one form per project; do not mix.
- For runtime-only validation, pair static types with `pydantic` or `attrs` rather than handwritten guards.

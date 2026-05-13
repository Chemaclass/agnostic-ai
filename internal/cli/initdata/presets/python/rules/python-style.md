---
name: python-style
description: Pythonic style and formatting.
globs: "**/*.py"
alwaysApply: true
---

- PEP 8 with `ruff` (or `black` + `flake8`). Format on save.
- Type hints on every public function. `from __future__ import annotations` in libraries that target older runtimes.
- Prefer dataclasses (`@dataclass(slots=True)` when stability matters) over plain classes for value types.
- F-strings over `%` and `.format()`. No bare `except:`; catch specific exceptions.
- Pathlib over `os.path` for new code. `pathlib.Path` is the obvious type for filesystem paths.
- Imports sorted by `ruff`/`isort`: standard library, third-party, local.

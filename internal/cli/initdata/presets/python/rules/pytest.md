---
name: pytest
description: pytest conventions.
globs: "**/test_*.py"
alwaysApply: true
---

- File names start with `test_`. Test functions start with `test_`.
- Use fixtures (`@pytest.fixture`) over `setUp`/`tearDown`. Scope fixtures as narrowly as the test allows.
- Parametrize with `@pytest.mark.parametrize` instead of looping inside a test.
- Assert with plain `assert`, never `unittest.TestCase.assertEqual`. pytest's introspection beats both.
- Use `tmp_path` (built-in fixture) for filesystem tests. Never write to the project root.
- Skip with reason: `pytest.skip("reason")`. Mark expected failures with `@pytest.mark.xfail(reason=...)`.

# CI

Use `sync --check` (or `doctor`) to fail a build when emitted files drift
from source specs. Pair with a release-time check to keep agents and
generated configs in lockstep.

## sync --check

Minimal step inside any workflow:

```yaml
- run: agnostic-ai sync --check
```

Exit code is non-zero if any target file is out of date. The command
does not write to disk; it only compares planned output against the
files on disk. Combine with `doctor` for stricter pre-merge gates.

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

## GitHub Action

The official action wraps the binary download, version pinning, and
caching so a workflow needs three lines:

```yaml
- uses: chemaclass/agnostic-ai-action@v1
  with:
    version: latest        # or a pinned vX.Y.Z
    command: check         # check | sync | doctor
```

Defaults to `agnostic-ai sync --check` when `command` is omitted. The
binary is cached across runs keyed on `version`, so warm runs finish in
under five seconds on `ubuntu-latest`. On drift, the action posts a
one-line job summary listing the files that are out of date.

Pin to a major (`@v1`) for compatible upgrades, a minor (`@v1.0`) for
patch-only, or a full tag (`@v1.0.0`) for byte-stable runs.

Source and release notes live at
[chemaclass/agnostic-ai-action](https://github.com/chemaclass/agnostic-ai-action).

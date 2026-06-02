# CI

Use `sync --check` (or `doctor`) to fail a build when emitted files drift from source specs.

## sync --check

Minimal step inside any workflow:

```yaml
- run: agnostic-ai sync --check
```

Exit code is non-zero if any target file is out of date. The command never writes to disk; it compares planned output against files on disk. Combine with `doctor` for stricter pre-merge gates.

## GitHub Action

The official action handles binary download, version pinning, and caching:

```yaml
- uses: chemaclass/agnostic-ai-action@v1
  with:
    version: latest        # or a pinned vX.Y.Z
    command: check         # check | sync | doctor
```

- Defaults to `agnostic-ai sync --check` when `command` is omitted.
- Binary is cached across runs keyed on `version`. Warm runs finish under five seconds on `ubuntu-latest`.
- On drift, the action posts a one-line job summary listing the out-of-date files.

Pin to a major (`@v1`) for compatible upgrades, a minor (`@v1.0`) for patch-only, or a full tag (`@v1.0.0`) for byte-stable runs.

Source and release notes: [chemaclass/agnostic-ai-action](https://github.com/chemaclass/agnostic-ai-action).

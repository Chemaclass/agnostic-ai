# Git hooks

Catch spec drift at commit time, before it ever reaches CI. Each
recipe below runs `agnostic-ai sync --check` whenever a spec or
`agnostic.config.yaml` is staged, and blocks the commit if any
generated target file is out of date.

The same `sync --check` powers the [CI gate](ci.md). Running it
locally on the relevant changes shortens the feedback loop from
"push, wait, fail" to "commit, fix, commit".

## Why a pre-commit hook

- Drift surfaces at the moment you create it, not 30 seconds into
  the next CI run.
- Every contributor, every machine, same gate. No "works on my
  branch" surprises.
- The hook is opt-in per checkout, so a fresh clone still works
  without any setup.

## pre-commit (Python)

[pre-commit](https://pre-commit.com) is the most common option in
polyglot repos. Add `.pre-commit-config.yaml`:

```yaml
repos:
  - repo: local
    hooks:
      - id: agnostic-ai-check
        name: agnostic-ai sync --check
        entry: agnostic-ai sync --check
        language: system
        pass_filenames: false
        files: '^(\.agnostic-ai/|agnostic\.config\.yaml$)'
```

Install once per checkout:

```bash
pre-commit install
```

`files:` scopes the hook to spec changes, so commits that touch
unrelated paths skip the check entirely. `pass_filenames: false`
runs the binary on the whole project (the same way CI does) instead
of passing each staged path.

## lefthook

[lefthook](https://lefthook.dev) is a single Go binary, no runtime
dependency. This repo dogfoods it; see [`lefthook.yml`](../../lefthook.yml).

Add to `lefthook.yml`:

```yaml
pre-commit:
  commands:
    agnostic-ai-check:
      glob: "{.agnostic-ai/**,agnostic.config.yaml}"
      run: agnostic-ai sync --check
```

Install once per checkout:

```bash
lefthook install
```

`glob:` keeps the hook silent unless a spec or the config is staged.

## husky + lint-staged

In Node projects, [husky](https://typicode.github.io/husky) plus
[lint-staged](https://github.com/lint-staged/lint-staged) is the
standard pairing.

`package.json`:

```json
{
  "scripts": {
    "prepare": "husky"
  },
  "lint-staged": {
    "{.agnostic-ai/**,agnostic.config.yaml}": "agnostic-ai sync --check --"
  }
}
```

`.husky/pre-commit`:

```sh
npx lint-staged
```

Install once per checkout:

```bash
npm install
```

The trailing `--` swallows the staged file paths that lint-staged
appends; `sync --check` reads the project root, not individual
files.

## Tips

- The hook needs `agnostic-ai` on `PATH`. Document the install in
  `CONTRIBUTING.md` so new contributors don't see a confusing
  `command not found` on their first commit.
- To recover from drift, run `agnostic-ai sync` and stage the
  regenerated outputs alongside the spec change.
- Set `gitignore.enabled: true` in `agnostic.config.yaml` if you
  want generated outputs out of git entirely; the hook still
  catches drift because `sync --check` ignores `gitignore`.
- Skip a hook for a single commit with `git commit --no-verify`.
  Save it for true emergencies; the gate exists for a reason.

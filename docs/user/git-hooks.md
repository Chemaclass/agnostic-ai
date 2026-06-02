# Git hooks

Catch spec drift at commit time, before CI. Each recipe runs `agnostic-ai sync --check` whenever a spec or `agnostic-ai.yaml` is staged, blocking the commit if any generated file is out of date.

The same `sync --check` powers the [CI gate](ci.md). Running it locally shortens the feedback loop from "push, wait, fail" to "commit, fix, commit".

## Why a pre-commit hook

- Drift surfaces when you create it, not 30 seconds into the next CI run.
- Same gate on every contributor and machine.
- Opt-in per checkout, so a fresh clone still works without setup.

## pre-commit (Python)

[pre-commit](https://pre-commit.com) is the common choice in polyglot repos. Add `.pre-commit-config.yaml`:

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

`files:` scopes the hook to spec changes; unrelated commits skip the check. `pass_filenames: false` runs the binary on the whole project (like CI) instead of passing each staged path.

## lefthook

[lefthook](https://lefthook.dev) is a single Go binary, no runtime dependency. This repo dogfoods it; see [`lefthook.yml`](../../lefthook.yml).

Add to `lefthook.yml`:

```yaml
pre-commit:
  commands:
    agnostic-ai-check:
      glob: "{.agnostic-ai/**,agnostic-ai.yaml}"
      run: agnostic-ai sync --check
```

Install once per checkout:

```bash
lefthook install
```

`glob:` keeps the hook silent unless a spec or the config is staged.

## husky + lint-staged

In Node projects, [husky](https://typicode.github.io/husky) plus [lint-staged](https://github.com/lint-staged/lint-staged) is the standard pairing.

`package.json`:

```json
{
  "scripts": {
    "prepare": "husky"
  },
  "lint-staged": {
    "{.agnostic-ai/**,agnostic-ai.yaml}": "agnostic-ai sync --check --"
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

The trailing `--` swallows the staged paths lint-staged appends; `sync --check` reads the project root, not individual files.

## Tips

- The hook needs `agnostic-ai` on `PATH`. Document the install in `CONTRIBUTING.md` so new contributors avoid `command not found` on their first commit.
- To recover from drift, run `agnostic-ai sync` and stage the regenerated outputs alongside the spec change.
- Set `gitignore.enabled: true` in `agnostic-ai.yaml` to keep generated outputs out of git. The hook still catches drift because `sync --check` ignores `gitignore`.
- Skip a hook for one commit with `git commit --no-verify`. Save it for emergencies.

# Unreleased changelog entries

Each pull request with a user-visible change adds one file here, `changes/<slug>.md`, instead of editing [CHANGELOG.md](../CHANGELOG.md). Two pull requests then never edit the same lines.

Name the file after the branch or the issue, such as `3757-merge-churn.md`. Write the headings it belongs under as they read in the changelog, then its bullets:

```markdown
### By tool

#### Codex

- `sync` writes the Codex hook timeout in seconds (#3757).
```

Use `### General`, `### By tool` with a `#### <Tool>` heading, or `### Site`. One file may hold several headings. Follow the [changelog-curator](../.agnostic-ai/agents/changelog-curator.md) entry rules.

A release runs `make changelog`. It appends each entry under its heading in `## [Unreleased]` and deletes the files. CI runs `scripts/changelog-assemble.sh --check` to reject a file it could not place.

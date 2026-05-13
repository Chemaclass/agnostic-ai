# Authoring a spec pack

A pack is a Git repo (or a local directory) shaped like an agnostic
project's source layout. Users install it with `agnostic-ai packs add`
and the contents merge into their layered spec load.

## Layout

```
<repo-root>/
├── agents/
├── skills/
├── rules/
├── hooks/
└── mcps/
```

Empty directories may be omitted. No `agnostic-ai.yaml` is needed
in a pack: the loader uses the default per-kind subdirectory names.

## Spec content

Specs use the same frontmatter and Markdown body documented in the
[user spec format](../user/spec-format.md). Pack authors should:

- Name every entry. The `name:` frontmatter field is the merge key
  consumers will use to override pack content.
- Keep `description:` short and action-oriented. Adapters surface the
  description directly in `AGENTS.md`-style merged documents.
- Avoid project-specific paths in `globs:`. Prefer language- or
  framework-level globs that travel.

## Versioning

Tag releases with semver (`v1.2.0`). Users pin against a tag when
they install:

```bash
agnostic-ai packs add github.com/your-org/your-pack@v1.2.0
```

Breaking changes (renamed entries, removed kinds, schema bumps) call
for a major bump. Adding entries is non-breaking; renames are
breaking because they invalidate downstream overrides.

## Naming

A pack's installed directory defaults to the last path segment of its
source URL (`github.com/foo/go-rules` → `go-rules`). Pick a name that
will not collide with another pack a user might also install. Users
can rename at install time with `--name`, but a stable default
matters for sharing snippets in docs and READMEs.

## Distribution

Any Git host works. The CLI invokes the system `git` binary for
clones, so the same auth setup that works for `git clone` works for
`agnostic-ai packs add`.

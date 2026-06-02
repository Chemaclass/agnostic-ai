# Authoring a spec pack

A pack is a Git repo (or local directory) shaped like an agnostic project's source layout. Users install via `agnostic-ai packs add`; contents merge into their layered spec load.

## Layout

```
<repo-root>/
├── agents/
├── skills/
├── rules/
├── hooks/
└── mcps/
```

Omit empty dirs. No `agnostic-ai.yaml` needed; the loader uses default subdir names.

## Spec content

Same frontmatter + Markdown body as [user spec format](../user/spec-format.md). Authors should:

- Set `name:` (merge key for downstream overrides).
- Write a short, action-oriented `description:` (adapters surface it in merged docs).
- Avoid project-specific `globs:`. Prefer language/framework-level patterns.

## Versioning

Semver tags. Users pin against a tag:

```bash
agnostic-ai packs add github.com/your-org/your-pack@v1.2.0
```

Renames are breaking (they invalidate downstream overrides). Adding entries is non-breaking.

## Naming

Default installed dir is the last path segment (`github.com/foo/go-rules` → `go-rules`). Pick a non-colliding name. Users can rename with `--name`.

## Distribution

Any Git host. The CLI invokes system `git` for clones; same auth as `git clone`.

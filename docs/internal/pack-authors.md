# Authoring a spec pack

[Contributor docs](README.md)

A pack is a Git repository or local directory with the same source layout as an agnostic-ai project. Users install it with `agnostic-ai packs add`; its specs become shared defaults for their project.

## Layout

```
<repo-root>/
├── agents/
├── skills/
├── rules/
├── hooks/
└── mcps/
```

Leave out empty directories. No `agnostic-ai.yaml` is needed; the loader uses the default directory names.

## Spec content

Use the same YAML header fields (frontmatter) and Markdown body as [user spec format](../site/content/docs/spec-format/_index.md). Authors should:

- Set `name:` (name that projects use to override a spec).
- Write a short, action-oriented `description:` (adapters show it in combined documents).
- Avoid project-specific `globs:`. Prefer language/framework-level patterns.

## Versioning

Use Semantic Versioning tags, such as `v1.2.0`. Users can select an exact version:

```bash
agnostic-ai packs add github.com/your-org/your-pack@v1.2.0
```

Renames are breaking (project overrides using the old names stop working). Adding entries is non-breaking.

## Naming

Default installed dir is the last path segment (`github.com/foo/go-rules` → `go-rules`). Choose a name that does not conflict with another installed pack. Users can rename with `--name`.

## Distribution

Use any Git host. The CLI runs the system `git` command to clone packs and uses the same authentication as `git clone`.

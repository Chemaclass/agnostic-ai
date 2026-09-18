+++
title = "Spec packs"
description = "Install, update, and layer reusable bundles of agnostic-ai specs."
weight = 70

[extra]
group = "Workflows"
+++

# Spec packs


A pack is a versioned directory of agnostic specs (agents, skills, rules, hooks, MCPs), published as a Git repo or shared on disk. Use one to reuse conventions across projects instead of copying spec files between them.

## Install

```bash
agnostic-ai packs add github.com/chemaclass/go-rules@v1.2.0
agnostic-ai packs add ./shared/security-rules
```

The pack is fetched into `.agnostic-ai/packs/<name>/` and pinned in `agnostic.packs.lock`. `agnostic-ai sync` loads pack specs in a layer below the project layer, so a project-local spec with the same name wins.

## List, update, remove

```bash
agnostic-ai packs list
agnostic-ai packs update                # all packs
agnostic-ai packs update go-rules       # one pack
agnostic-ai packs remove go-rules
```

`update` re-fetches each pack at the ref in the lockfile and refreshes the recorded commit sha.

## Sources

| Source           | Example                              |
|------------------|--------------------------------------|
| Git URL          | `github.com/foo/bar`, `gitlab.com/x` |
| Git URL with ref | `github.com/foo/bar@v1.2.0`          |
| Local directory  | `./local/pack`, `file:///abs/path`   |

Git URLs are cloned with `--depth 1`. The `.git` directory is stripped after the sha is captured.

## Pack layout

A pack mirrors the standard agnostic source layout at its root:

```
<pack>/
├── agents/
├── skills/
├── rules/
├── hooks/
└── mcps/
```

Empty directories may be omitted. Frontmatter rules mirror the [spec format](@/docs/spec-format.md).

## Lockfile

`agnostic.packs.lock` is a YAML file at the project root:

```yaml
version: 1
packs:
  - name: go-rules
    source: github.com/chemaclass/go-rules
    ref: v1.2.0
    sha: 9f8b3c1e0a5d4e8b2c7d6f3a1b0c4d5e6f7a8b9c
```

Commit it so peers and CI install the same revisions. The file is sorted by name, and removing the last pack deletes it.

## Layer precedence

Packs load before the project and personal project layers:

```
packs  →  project  →  project-user
```

Higher layers override by `(kind, name)`. A project rule named `conventional-commits` masks the pack rule with the same name, which is how you adapt a pack convention to one project.

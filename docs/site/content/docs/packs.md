+++
title = "Spec packs"
description = "Install, update, and layer reusable bundles of agnostic-ai specs."
weight = 70

[extra]
group = "Workflows"
+++

# Spec packs

A pack is a versioned directory of specs (agents, skills, rules, hooks, MCPs), published as a Git repo or shared on disk. Use packs to reuse conventions across projects instead of copying spec files.

## Install

```bash
agnostic-ai packs add github.com/obra/superpowers@v6.4.2
agnostic-ai packs add ./shared/security-rules
```

`packs add` fetches the pack into `.agnostic-ai/packs/<name>/` and pins it in `agnostic.packs.lock`. `sync` loads pack specs in a layer below the project layer, so a project spec with the same name wins.

## Packs you can install

Checked on 2026-10-03 with `packs add` and `sync`. Each repo keeps its skills in a root `skills/` directory.

- `github.com/anthropics/skills`: Anthropic's example skills. Installs as `skills`.
- `github.com/obra/superpowers@v6.4.2`: a development workflow skill set. Installs as `superpowers`.
- `github.com/vercel-labs/agent-skills`: Vercel's web and deployment skills. Installs as `agent-skills`.

## List, update, remove

```bash
agnostic-ai packs list
agnostic-ai packs update                # all packs
agnostic-ai packs update superpowers     # one pack
agnostic-ai packs remove superpowers
```

`update` re-fetches each pack at the ref in the lockfile and refreshes the recorded commit sha.

## Sources

| Source           | Example                              |
|------------------|--------------------------------------|
| Git URL          | `github.com/foo/bar`, `gitlab.com/x` |
| Git URL with ref | `github.com/foo/bar@v1.2.0`          |
| Local directory  | `./local/pack`, `file:///abs/path`   |

Git URLs are cloned with `--depth 1`. The `.git` directory is removed after the sha is recorded.

## Pack layout

A pack has the standard source layout at its root:

```
<pack>/
├── agents/
├── skills/
├── rules/
├── hooks/
└── mcps/
```

Empty directories may be omitted. Frontmatter follows the [spec format](@/docs/spec-format/_index.md).

## Lockfile

`agnostic.packs.lock` is a YAML file at the project root:

```yaml
version: 1
packs:
  - name: superpowers
    source: github.com/obra/superpowers
    ref: v6.4.2
    sha: 8ca22dba9a94f28898bbce59f2537ff4d87c747d
```

Commit it so teammates and CI install the same revisions. The file is sorted by name. Removing the last pack deletes it.

## Layer precedence

Layers load in this order:

```
packs  →  project  →  project-user
```

`project-user` is your ignored [local layer](@/docs/local-overrides.md). A higher layer overrides a lower one by `(kind, name)`. To adapt a pack convention to one project, add a project spec with the same name. A project rule named `conventional-commits` replaces the pack rule with that name.

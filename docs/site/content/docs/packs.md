+++
title = "Spec packs"
description = "Install, update, and share collections of agnostic-ai specs you can reuse."
weight = 70

[extra]
group = "Workflows"
+++

# Spec packs

A pack is a directory of specs (agents, skills, rules, hooks, MCPs), shared as a Git repo or a local folder. Use a pack to reuse conventions across projects instead of copying spec files.

## Install

```bash
agnostic-ai packs add github.com/obra/superpowers@v6.4.2
agnostic-ai packs add ./shared/security-rules
```

`packs add` copies the pack into `.agnostic-ai/packs/<name>/` and records its version in `agnostic.packs.lock`. `sync` then includes the pack specs. If your project has a spec with the same name, yours wins.

## Packs you can install

Checked on 2026-10-03. Each repo keeps its skills in a root `skills/` directory.

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

`update` fetches each pack again at the version in the lockfile.

## Sources

| Source           | Example                              |
|------------------|--------------------------------------|
| Git URL          | `github.com/foo/bar`, `gitlab.com/x` |
| Git URL with ref | `github.com/foo/bar@v1.2.0`          |
| Local directory  | `./local/pack`, `file:///abs/path`   |

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

Leave out any directory you do not need. Frontmatter follows the [spec format](@/docs/spec-format/_index.md).

## Lockfile

`agnostic.packs.lock` sits at the project root:

```yaml
version: 1
packs:
  - name: superpowers
    source: github.com/obra/superpowers
    ref: v6.4.2
    sha: 8ca22dba9a94f28898bbce59f2537ff4d87c747d
```

Commit it so teammates and CI install the same versions.

## Which spec wins

When two specs share a kind and name, the later one in this list wins:

1. Built-in specs (when enabled with `builtins`)
2. Pack specs
3. Your project specs
4. Your personal setup, which changes the winning spec field by field instead of replacing it (see [local overrides](@/docs/local-overrides.md))

To adapt a pack convention to one project, add a project spec with the same name. A project rule named `conventional-commits` replaces the pack rule with that name.

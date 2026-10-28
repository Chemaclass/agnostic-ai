+++
title = "Spec packs"
description = "Install, update, and share collections of agnostic-ai specs you can reuse."
weight = 70

[extra]
group = "Workflows"
+++

# Spec packs

A pack is a set of source specs (agents, skills, rules, hooks, and MCP server connections) shared as a Git repository or local folder. Use one to reuse conventions across projects instead of copying spec files.

## Install

Review the pack's instructions and scripts before using them.

```bash
agnostic-ai packs add github.com/obra/superpowers@v6.4.2
agnostic-ai packs add ./shared/security-rules
```

`packs add` copies the pack into `.agnostic-ai/packs/<name>/` and records its version in `agnostic.packs.lock`. Then run `agnostic-ai sync` to generate its files for your tools. A project spec with the same name wins; see [which spec wins](#which-spec-wins).

<details>
<summary>Find example packs</summary>

## Packs you can install

Checked on 2026-10-03. Each repo keeps its skills in a root `skills/` directory.

- `github.com/anthropics/skills`: Anthropic's example skills. Installs as `skills`.
- `github.com/obra/superpowers@v6.4.2`: a development workflow skill set. Installs as `superpowers`.
- `github.com/vercel-labs/agent-skills`: Vercel's web and deployment skills. Installs as `agent-skills`.

</details>

## List, update, remove

```bash
agnostic-ai packs list
agnostic-ai packs update                # all packs, at the recorded commits
agnostic-ai packs update superpowers     # one pack
agnostic-ai packs update --latest        # move to the newest commit of each ref
agnostic-ai packs remove superpowers
```

`update` installs each Git pack at the commit `sha` in the lockfile, so a moved tag or a force-pushed branch does not change what you get. It fails and leaves the lockfile unchanged when the remote no longer has that commit. `update --latest` fetches the newest commit of each pack's `ref`, prints the old and new commit, and records the new one. A local folder pack is copied again as it is now.

The lockfile comes from the repository, so `update` checks every entry before it changes any file. It stops on a pack name that starts with a dot or is not a single folder name, and on a Git source other than `https://`, `ssh://`, `git@host:path`, or a host path such as `github.com/foo/bar`. It skips a pack from a local folder outside the project and prints the `packs add` command that refreshes it; `packs update <name>` stops with that command instead. `packs add` refuses the same names and Git sources.

<details>
<summary>Share a pack, choose its source, and inspect its lockfile</summary>

## Sources

| Source           | Example                              |
|------------------|--------------------------------------|
| Git URL          | `github.com/foo/bar`, `gitlab.com/x` |
| Git URL with a tag or branch | `github.com/foo/bar@v1.2.0`          |
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

Leave out any directory you do not need. Spec front matter follows the [spec format](@/docs/spec-format/_index.md).

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

Commit it so teammates and CI install the same commits.

</details>

## Which spec wins

When two specs share a kind and name, the later one in this list wins:

1. Built-in specs (when enabled with `builtins`)
2. Pack specs
3. Your project specs
4. Your personal setup, which changes the winning spec field by field instead of replacing it (see [local overrides](@/docs/local-overrides.md))

To adapt a pack convention to one project, add a project spec with the same name. A project rule named `conventional-commits` replaces the pack rule with that name.

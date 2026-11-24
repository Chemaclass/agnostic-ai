+++
title = "Scopes"
description = "Where specs live: project for the team, local for you in one project, user for you in every project."
weight = 43

[extra]
group = "Workflows"
+++

# Scopes

Specs can live at the project, local, or user scope. Use `--scope` to select the scope a command reads or writes.

| Scope | Who gets it | Lives in | In Git |
|---|---|---|---|
| `project` | The team, in this project | `.agnostic-ai/` and `agnostic-ai.yaml` | Committed |
| `local` | You, in this project | `.agnostic-ai/local/` and `agnostic-ai.local.yaml` | Ignored |
| `user` | You, in every project | `~/.agnostic-ai/`, or `$AGNOSTIC_AI_HOME` | Your own repository, or none |

`project` is the default. Create a spec at each scope with [`new`](@/docs/cli-reference/start.md#new):

```bash
agnostic-ai new rule team-style                    # project
agnostic-ai new rule scratch-notes --scope local   # local
agnostic-ai new rule my-style --scope user         # user
```

## Which spec wins

In a project, `agnostic-ai sync` reads built-ins, then [packs](@/docs/packs.md), then project specs, then local ones. A later layer wins over an earlier one with the same kind and name. A local spec does not replace the shared one: it [changes only the fields it writes](@/docs/local-overrides.md#override-fields).

`agnostic-ai sync --scope user` is a separate run. It reads built-ins, then `~/.agnostic-ai/`, then its own `local/` folder, and writes each tool's user files. It loads no project specs and no packs.

Project and user specs never merge. When a project and your user setup both have a skill or agent of one name, each tool decides which it loads, and a project sync warns about the clash. See [shared names](@/docs/global-configuration-reference.md#global-shared-names).

`agnostic-ai list` prints the layer each spec came from: `builtin`, `pack:<name>`, `project` or `local`. `agnostic-ai list --scope user` prints `builtin`, `user` or `user-local`.

## What each command takes

| Commands | Scopes |
|---|---|
| `new`, `list` | `project`, `local`, `user` |
| `sync`, `validate`, `lint`, `import`, `migrate`, `explain`, `install-hook`, `view` (and `ui`), `serve`, `mcp tools` | `project`, `user` |
| `why`, `status`, `render`, `compare`, `graph`, `revert`, `cleanup` | `project` only. Asked for `--scope user`, each stops and names the user-level command that does the job. |

Local specs are part of a project, so the commands in the second row read them with `--scope project`.

`init` and `doctor` work on a project and take no `--scope`. To start a user setup, create `~/.agnostic-ai/` yourself, run `new ... --scope user`, or use **Open user settings** in the web interface.

## Go further

- [Local overrides](@/docs/local-overrides.md): change shared specs for yourself.
- [User configuration](@/docs/global-configuration.md): set up `~/.agnostic-ai/` and sync it.
- [Folder instructions](@/docs/scoped-context.md): a rule for one directory. That is the rule's `scope` field, set with `new rule --dir`.

## Older names

Releases up to v0.96.0 called the user scope "global" and took `--global` in place of `--scope user`. `--global` still works and prints a deprecation warning. Config keys, API routes and the `state/global.json` file keep that word.

+++
title = "Local overrides"
description = "Keep personal specs and instructions on top of the shared ones, out of Git, per project or per machine."
weight = 75

[extra]
group = "Workflows"
+++

# Local overrides

A local layer holds your personal setup. It overrides shared specs, adds new ones, and extends the shared instructions. It stays out of Git, so the committed `.agnostic-ai/` remains the source of truth for the team.

There are two local layers:

| Layer | Root | Loaded by | Inspect with |
|---|---|---|---|
| `project-user` | `<project>/.agnostic-ai.local/` | `agnostic-ai sync` in that project | `agnostic-ai list` |
| `global-local` | `~/.agnostic-ai/local/` (or `$AGNOSTIC_AI_HOME/local/`) | `agnostic-ai sync --global` | `agnostic-ai list --global` |

A layer loads only when its directory exists. The two never mix: project sync ignores `~/.agnostic-ai/`, and global sync ignores every project.

## Layout

The project layer mirrors `.agnostic-ai/` with fixed directory names:

```text
my-project/
├── .agnostic-ai/                  # committed
│   ├── AGNOSTIC_AI.md
│   ├── rules/testing.md
│   └── skills/review/SKILL.md
└── .agnostic-ai.local/            # ignored
    ├── AGNOSTIC_AI.md             # extends the shared instructions
    ├── rules/scratch-notes.md     # new name: appends
    └── skills/review/SKILL.md     # same name: replaces the shared skill
```

It reads every spec kind: `agents/`, `skills/`, `rules/`, `hooks/`, `mcps/`, `commands/`, `settings/`, `reviews/`, `environments/`, and `ignore/`. Custom `sources` paths in `agnostic-ai.yaml` apply to `.agnostic-ai/` only.

The global layer reads `AGNOSTIC_AI.md`, `agents/`, `skills/`, `rules/`, and `hooks/`. See [global configuration](@/docs/configuration.md#global-configuration).

## Override a spec

A local spec with the same kind and name as a shared one replaces the whole entry: frontmatter, body, and skill assets. Fields are not merged today, so copy the fields you want to keep into the local file.

```text
.agnostic-ai/skills/review/SKILL.md         # shared
.agnostic-ai.local/skills/review/SKILL.md   # wins on this machine
```

A local spec with a new name appends to the shared set. Every target receives it like any other spec.

## Extend the instructions

`.agnostic-ai.local/AGNOSTIC_AI.md` extends `.agnostic-ai/AGNOSTIC_AI.md`. Sync appends its text to every entry-point file it writes (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, and the rest), after the shared body and any inlined rules:

```md
<!-- agnostic-ai:local:start -->

Answer in Spanish. Run `make test` before every commit.

<!-- agnostic-ai:local:end -->
```

- The shared `.agnostic-ai/AGNOSTIC_AI.md` does not change.
- `::target` fences and `@path` imports work as in the [shared file](@/docs/configuration.md#per-target-paragraphs).
- `agnostic-ai import` drops the marked block, so local instructions never land in `.agnostic-ai/`. Import still captures local specs that sync emitted, such as a local rule inlined into `AGENTS.md`, so review the diff after an import.
- Generated entry points are ignored by default. If your project commits them, the local text reaches the commit too.

In the global layer, `local/AGNOSTIC_AI.md` comes last in the managed instructions block, after the shared instructions and rules.

## Stay out of Git

`init` and `sync` keep `/.agnostic-ai.local/` in the managed `.gitignore` block, next to `/agnostic-ai.local.yaml`. Nothing to add by hand. See [gitignore](@/docs/configuration.md#gitignore).

For the global layer, add this line to `~/.agnostic-ai/.gitignore` when that home is a Git repository:

```gitignore
/local/
```

Per-machine config tweaks belong in `agnostic-ai.local.yaml`, which deep-merges over `agnostic-ai.yaml`. See [configuration](@/docs/configuration.md#local-overrides).

## See the effective layer

`agnostic-ai list` prints each loaded spec as `kind`, `name`, and the layer that supplied it:

```console
$ agnostic-ai list
skill   review           project-user
rule    testing          project
rule    scratch-notes    project-user
```

`agnostic-ai list --global` does the same for the `global` and `global-local` layers.

## Watch

`sync --watch` watches `.agnostic-ai.local/`. An edit to a local spec or to `.agnostic-ai.local/AGNOSTIC_AI.md` triggers a full re-sync. See [`sync --watch`](@/docs/cli-reference.md#sync).

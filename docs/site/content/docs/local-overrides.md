+++
title = "Local overrides"
description = "Keep personal specs and instructions on top of the shared ones, out of Git, per project or per machine."
weight = 75

[extra]
group = "Workflows"
+++

# Local overrides

A local layer holds your personal setup. It edits shared specs field by field, adds new ones, and extends the shared instructions. It stays out of Git, so the committed `.agnostic-ai/` remains the source of truth for the team.

There are two local layers:

| Layer | Root | Loaded by | Inspect with |
|---|---|---|---|
| `project-user` | `<project>/.agnostic-ai/local/` | `agnostic-ai sync` in that project | `agnostic-ai list` |
| `global-local` | `~/.agnostic-ai/local/` (or `$AGNOSTIC_AI_HOME/local/`) | `agnostic-ai sync --global` | `agnostic-ai list --global` |

A layer loads only when its directory exists. The two never mix: project sync ignores `~/.agnostic-ai/`, and global sync ignores every project.

## Why a local layer

The team agrees on `.agnostic-ai/`. You still work your own way. Without a local layer, the only options are to edit the shared specs, which leaks into the next commit, or to edit generated files, which the next `sync` overwrites.

A local layer keeps them apart. Git holds what the team agreed on. Your machine holds the difference.

Typical reasons:

- **Model and effort.** Your plan includes Opus, the team default is Sonnet. Change one field, keep the rest of the skill.
- **Personal steps.** Add "run my local benchmark" to the end of the shared review skill, without forking it.
- **Experiments.** Try a new agent or rule for a week. Promote it to `.agnostic-ai/` when it earns its place, or delete it.
- **Machine specifics.** Point an MCP server at your local port, or add an env var your setup needs.
- **Stricter or looser tools.** Drop a tool from an agent, or add one, on your machine only.

## Layout

Both layers have the same shape: a `local/` folder inside the source directory, mirroring it.

```text
my-project/.agnostic-ai/
├── AGNOSTIC_AI.md                 # shared, committed
├── rules/testing.md
├── skills/review/SKILL.md
└── local/                         # yours: ignored, still synced
    ├── AGNOSTIC_AI.md             # extends the shared instructions
    ├── rules/scratch-notes.md     # new name: appends
    └── skills/review/SKILL.md     # same name: edits the shared skill
```

It reads every spec kind: `agents/`, `skills/`, `rules/`, `hooks/`, `mcps/`, `commands/`, `settings/`, `reviews/`, `environments/`, and `ignore/`. The folder stays at `.agnostic-ai/local/` even when custom `sources` paths in `agnostic-ai.yaml` move the shared specs.

The global layer reads `AGNOSTIC_AI.md`, `agents/`, `skills/`, `rules/`, and `hooks/`. See [global configuration](@/docs/configuration.md#global-configuration).

## Add a spec

A local spec with a new name appends to the shared set. Every target receives it like any other spec.

## Override fields

A local spec with the same kind and name as a shared one merges into it. Write only what changes.

Shared, committed:

```md
---
name: custom
description: Draft the release notes.
model:
  claude: sonnet
  codex: gpt-5.3-codex
---
Read the merged PRs since the last tag.
```

Local, ignored:

```md
---
name: custom
model:
  claude: opus
---
```

What every target receives:

```md
---
name: custom
description: Draft the release notes.
model:
  claude: opus
  codex: gpt-5.3-codex
---
Read the merged PRs since the last tag.
```

The frontmatter merges key by key, the same way `agnostic-ai.local.yaml` merges over `agnostic-ai.yaml`:

| Local value | Result |
|---|---|
| a map | merges into the shared map, key by key, at any depth |
| a scalar or a list | replaces the shared value |
| `null` | removes the key |
| missing | keeps the shared value |

So `tools: [Read]` replaces the whole shared list, and `tools: null` drops it. Inside an `x-<target>` map, `null` keeps its usual meaning: `x-codex.model: null` drops `model` for Codex only. `x-claude:` merges with the shared `x-claude:` like any other map.

Where the local file sits decides its scope. `local/rules/style.md` applies to the whole project even when the shared `rules/backend/style.md` applies to `backend/` only.

The same rules apply to YAML specs: hooks, MCP servers, settings, and environments. A local `mcps/docs.yaml` can change `env.PORT` and keep the shared command and args.

## Turn a shared spec off

Exclude the targets you do not want. Nothing else to copy:

```md
---
name: review
targets-exclude: [claude, codex, cursor]
---
```

## Stay current after a pull

A local file holds only your changes. When the team edits the shared spec, the next sync picks up their change and applies yours on top. No diff to merge by hand, unless you replaced the whole body.

## Extend the body

The body follows three rules:

- **Empty local body.** The shared body stays.
- **A `::parent` line.** The shared body goes there. Put your text before it, after it, or around it.
- **Any other body.** It replaces the shared one.

Shared:

```md
---
name: custom
---
foo for
```

Local:

```md
---
name: custom
---
::parent
bar baz
```

Result:

```md
foo for
bar baz
```

`::parent` must start at column 0 and stand alone on its line, like the `::target` and `::end` fences. Keep it outside a `::target` fence: fences do not nest, so the shared body's own fences would end yours. A fence the shared body leaves open is closed after it, so your lines still reach every target. A `::parent` line in a local spec with a new name has nothing to extend, so sync drops it.

## Skill assets

A local skill folder with only a `SKILL.md` keeps the shared folder's scripts and templates. Once the local folder holds any other file, its own files ship instead of the shared ones.

## What does not merge

Only the two local layers merge. A project spec over a [pack](@/docs/packs.md) spec still replaces it whole, because a project adapting a pack means to own that spec.

Two files with the same name inside one layer are still an authoring mistake: one wins, and `agnostic-ai lint` reports the other (LINT003).

## Extend the instructions

`.agnostic-ai/local/AGNOSTIC_AI.md` extends `.agnostic-ai/AGNOSTIC_AI.md`. Sync appends its text to every entry-point file it writes (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, and the rest), after the shared body and any inlined rules:

```md
<!-- agnostic-ai:local:start -->

Answer in Spanish. Run `make test` before every commit.

<!-- agnostic-ai:local:end -->
```

- The shared `.agnostic-ai/AGNOSTIC_AI.md` does not change.
- A legacy [`outputs.<target>.rules-file`](@/docs/configuration.md#entry-point-files) that names the entry point gets the block too, after the rule bodies.
- `::target` fences and `@path` imports work as in the [shared file](@/docs/configuration.md#per-target-paragraphs).
- `agnostic-ai import` drops the marked block, so local instructions never land in `.agnostic-ai/`. Import still captures local specs that sync emitted, such as a local rule inlined into `AGENTS.md`, so review the diff after an import.
- Generated entry points are ignored by default. If your project commits them, the local text reaches the commit too.

In the global layer, `local/AGNOSTIC_AI.md` comes last in the managed instructions block, after the shared instructions and rules.

## Stay out of Git

`init` and `sync` keep `/.agnostic-ai/local/` in the managed `.gitignore` block, next to `/agnostic-ai.local.yaml`. Nothing to add by hand. See [gitignore](@/docs/configuration.md#gitignore).

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

`sync --watch` watches `.agnostic-ai/local/`. An edit to a local spec or to `.agnostic-ai/local/AGNOSTIC_AI.md` triggers a full re-sync. See [`sync --watch`](@/docs/cli-reference.md#sync).

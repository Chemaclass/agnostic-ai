+++
title = "Local overrides"
description = "Keep personal specs and instructions on top of the shared ones, out of Git, per project or per machine."
weight = 75

[extra]
group = "Workflows"
+++

# Local overrides

A `local/` folder holds your personal setup. It changes shared specs field by field, adds new specs, and extends the shared instructions. It stays out of Git, so the committed `.agnostic-ai/` stays the team's setup.

There are two `local/` folders:

| Name | Folder | Loaded by | Inspect with |
|---|---|---|---|
| `project-user` | `<project>/.agnostic-ai/local/` | `agnostic-ai sync` in that project | `agnostic-ai list` |
| `global-local` | `~/.agnostic-ai/local/` (or `$AGNOSTIC_AI_HOME/local/`) | `agnostic-ai sync --global` | `agnostic-ai list --global` |

A folder loads only when it exists. Project sync ignores `~/.agnostic-ai/`, and global sync ignores every project. Your personal setup wins over the shared one.

## Why use it

The team agrees on `.agnostic-ai/`, but you work your own way. If you edit the shared specs, the change lands in your next commit. If you edit generated files, the next `sync` overwrites them. A `local/` folder keeps your changes on your machine.

- **Model and effort.** Your plan includes Opus; the team default is Sonnet. Change one field, keep the rest of the skill.
- **Personal steps.** Add "run my local benchmark" to the shared review skill without forking it.
- **Experiments.** Try a new agent or rule for a week. Move it to `.agnostic-ai/` if it works, or delete it.
- **Machine specifics.** Point an MCP server at your local port, or add an env var.
- **Tools.** Drop or add a tool for an agent on your machine only.

## Layout

`local/` mirrors the source directory:

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

The project folder reads every spec kind: `agents/`, `skills/`, `rules/`, `hooks/`, `mcps/`, `commands/`, `settings/`, `reviews/`, `environments/`, and `ignore/`. It stays at `.agnostic-ai/local/` even when custom `sources` paths in `agnostic-ai.yaml` move the shared specs.

The global folder reads `AGNOSTIC_AI.md`, `agents/`, `skills/`, `rules/`, and `hooks/`. See [global configuration](@/docs/configuration.md#global-configuration).

## Add a spec

A local spec with a new name joins the shared set. Every target gets it like any other spec.

## Override fields

A local spec with the same kind and name as a shared one merges into it. Write only what changes. When the team edits the shared spec, the next sync keeps your changes on top.

Shared, committed:

```md
---
name: custom
description: Draft the release notes.
model:
  claude: sonnet
  codex: gpt-5.6-terra
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
  codex: gpt-5.6-terra
---
Read the merged PRs since the last tag.
```

Frontmatter merges key by key, the same way `agnostic-ai.local.yaml` merges over `agnostic-ai.yaml`:

| Local value | Result |
|---|---|
| a map | merges into the shared map, key by key, at any depth |
| a scalar or a list | replaces the shared value |
| `null` | removes the key |
| missing | keeps the shared value |

So `tools: [Read]` replaces the whole shared list, and `tools: null` drops it. Inside an `x-<target>` map, `null` removes one key: `x-codex.model: null` drops `model` for Codex only.

The local file's location sets its scope. `local/rules/style.md` applies to the whole project even when the shared `rules/backend/style.md` applies to `backend/` only.

YAML specs (hooks, MCP servers, settings, environments) follow the same rules. A local `mcps/docs.yaml` can change `env.PORT` and keep the shared command and args.

## Turn a shared spec off

Exclude the targets you do not want. There is nothing to copy:

```md
---
name: review
targets-exclude: [claude, codex, cursor]
---
```

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

`::parent` must start at column 0 and stand alone on its line. Keep it outside a `::target` fence. In a spec with a new name, `::parent` has nothing to extend, so sync drops it.

## Skill assets

A local skill folder with only a `SKILL.md` keeps the shared folder's scripts and templates. If the local folder holds any other file, its own files replace the shared ones.

## What does not merge

Only your personal setup merges. A project spec over a [pack](@/docs/packs.md) spec replaces it whole, because a project that adapts a pack means to own that spec.

Two files with the same name in one folder are a mistake. One wins, and `agnostic-ai lint` reports the other (LINT003).

## Extend the instructions

`.agnostic-ai/local/AGNOSTIC_AI.md` extends `.agnostic-ai/AGNOSTIC_AI.md`. Sync appends its text to every entry-point file it writes (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, and the rest), after the shared body and any inlined rules:

```md
<!-- agnostic-ai:local:start -->

Answer in Spanish. Run `make test` before every commit.

<!-- agnostic-ai:local:end -->
```

- A legacy [`outputs.<target>.rules-file`](@/docs/configuration.md#entry-point-files) that names the entry point gets the block too, after the rule bodies.
- `::target` fences and `@path` imports work as in the [shared file](@/docs/configuration.md#per-target-paragraphs).
- Generated entry points are ignored by default. If your project commits them, your local text lands in the commit too.

In the global folder, `local/AGNOSTIC_AI.md` comes last in the managed instructions block, after the shared instructions and rules.

## Import

`agnostic-ai import` never writes your local specs into `.agnostic-ai/`. Sync writes local specs into the native files that import reads, such as a local rule inlined into `AGENTS.md`. Import skips every name your local folder declares. A shared spec that your local folder extends keeps its own content. The run lists what it skipped.

Hooks are matched by event, matcher, and handler. A local handler is removed from the imported hook, and a script that only local hooks run stays out of `.agnostic-ai/scripts/`.

Settings, reviews, and environments share one native file per target. If your local folder holds one of these kinds, import leaves every shared spec of that kind unchanged and says so.

## Stay out of Git

`init` and `sync` add `/.agnostic-ai/local/` to the managed `.gitignore` block, next to `/agnostic-ai.local.yaml`. Add nothing by hand. See [gitignore](@/docs/configuration.md#gitignore).

If `~/.agnostic-ai/` is a Git repository, add this line to its `.gitignore`:

```gitignore
/local/
```

Put per-machine config tweaks in `agnostic-ai.local.yaml`, which merges over `agnostic-ai.yaml`. See [configuration](@/docs/configuration.md#local-overrides).

## See what wins

`agnostic-ai list` prints each loaded spec as `kind`, `name`, and the setup that supplied it:

```console
$ agnostic-ai list
skill   review           project-user
rule    testing          project
rule    scratch-notes    project-user
```

`agnostic-ai list --global` does the same for the `global` and `global-local` setups.

## Watch

`sync --watch` re-syncs when you edit a local spec or `.agnostic-ai/local/AGNOSTIC_AI.md`. See [`sync --watch`](@/docs/cli-reference/sync.md#sync).

+++
title = "Local overrides"
description = "Keep personal specs and instructions on top of the shared ones, out of Git, per project or per machine."
weight = 75

[extra]
group = "Workflows"
+++

# Local overrides

Keep personal instructions in `.agnostic-ai/local/` without changing the team's source files. For example, create `.agnostic-ai/local/AGNOSTIC_AI.md`:

```markdown
Answer in Spanish.
```

Then run `agnostic-ai sync`. The text extends the shared project instructions. Your local folder stays out of Git. If the project commits generated instruction files, your personal text appears there too; review before committing.

For personal configuration values, use `agnostic-ai.local.yaml`. To alter a shared skill or rule, add a local spec with the same kind and name and write only the fields you want to change.

## Why use it

Personal preferences and experiments stay on your machine. The team keeps one shared setup.

## Add a spec

A new local spec name adds a spec for your machine. Run `agnostic-ai sync`, then `agnostic-ai list` to check which source won.

<details>
<summary>Choose project-local or machine-wide personal sources</summary>

There are two `local/` folders:

| Name | Folder | Loaded by | Inspect with |
|---|---|---|---|
| `project-user` | `<project>/.agnostic-ai/local/` | `agnostic-ai sync` in that project | `agnostic-ai list` |
| `global-local` | `~/.agnostic-ai/local/` (or `$AGNOSTIC_AI_HOME/local/`) | `agnostic-ai sync --global` | `agnostic-ai list --global` |

A folder loads only when it exists. Project sync ignores `~/.agnostic-ai/`, and global sync ignores every project. Your personal setup wins over the shared one.

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

For the global folder’s supported kinds and layout, see [global configuration](@/docs/global-configuration.md#source-and-targets).

</details>

<details>
<summary>Change shared fields, extend bodies, and keep supporting files</summary>

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

After sync, Claude uses `opus`; the description, body, and Codex model stay as shared. A personal change replaces only the fields it names.

Frontmatter (the YAML fields between `---` lines) merges key by key, the same way `agnostic-ai.local.yaml` merges over `agnostic-ai.yaml`:

| Local value | Result |
|---|---|
| a group of keys | merges into the shared group, key by key, at any depth |
| a single value or a list | replaces the shared value |
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
Review the changed code.
```

Local:

```md
---
name: custom
---
::parent
Then run my local benchmark.
```

The result is the shared review instruction followed by your benchmark step.

`::parent` must start at column 0 and stand alone on its line. Keep it outside a `::target` fence. In a spec with a new name, `::parent` has nothing to extend, so sync drops it.

## Skill assets

A local skill folder with only a `SKILL.md` keeps the shared folder's supporting files, such as scripts and templates. If the local folder holds any other file, its own supporting files replace the shared supporting files.

## What does not merge

Only your personal setup merges. A project spec over a [pack](@/docs/packs.md) spec replaces it whole, because a project that adapts a pack means to own that spec.

Two files with the same name in one folder are a mistake. One wins, and `agnostic-ai lint` reports the other (LINT003).

## Extend the instructions

`.agnostic-ai/local/AGNOSTIC_AI.md` extends `.agnostic-ai/AGNOSTIC_AI.md`. Sync appends its text to every root instruction file it writes (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, and the rest), after the shared body and any inlined rules:

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

</details>

## Stay out of Git

`init` and `sync` add `/.agnostic-ai/local/` to the managed `.gitignore` block, next to `/agnostic-ai.local.yaml`. Add nothing by hand. See [gitignore](@/docs/configuration.md#gitignore).

If `~/.agnostic-ai/` is a Git repository, add this line to its `.gitignore`:

```gitignore
/local/
```

Put per-machine config tweaks in `agnostic-ai.local.yaml`, which merges over `agnostic-ai.yaml`. See [configuration](@/docs/configuration.md#local-overrides).

## See what wins

`agnostic-ai list` prints each spec and the source that won:

```console
$ agnostic-ai list
skill   review           project-user
rule    testing          project
rule    scratch-notes    project-user
```

`agnostic-ai list --global` does the same for the `global` and `global-local` setups.

## Watch

`sync --watch` re-syncs when you edit a local spec or `.agnostic-ai/local/AGNOSTIC_AI.md`. See [`sync --watch`](@/docs/cli-reference/sync.md#sync).

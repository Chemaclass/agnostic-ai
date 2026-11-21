+++
title = "Local overrides"
description = "Keep personal specs and instructions on top of the shared ones, out of Git, per project or per machine."
weight = 45

[extra]
group = "Workflows"
+++

# Local overrides

Put personal specs and instructions in `.agnostic-ai/local/`. They extend or override the team's setup, and Git ignores the folder. Example `.agnostic-ai/local/AGNOSTIC_AI.md`:

```markdown
Answer in Spanish.
```

Run `agnostic-ai sync`. The text extends the shared instructions.

Generated instruction files are ignored by default. If your project commits them, your personal text lands in them too, so review before committing.

For personal configuration values, use `agnostic-ai.local.yaml` (see [configuration](@/docs/configuration.md#local-overrides)). In `agnostic-ai ui`, **Local** on **Settings** saves built-ins and settings fields as local overrides; see [saving for yourself](@/docs/cli-reference/serve.md#edit-scope).

## Add a spec

```bash
agnostic-ai new rule scratch-notes --scope local
```

- A new name adds a spec on your machine.
- The same kind and name as a shared spec [changes only the fields you write](#override-fields). `new` then starts the file with the name only.

`new --scope local` and `doctor` warn when Git does not ignore `.agnostic-ai/local/` or `agnostic-ai.local.yaml`.

Run `agnostic-ai sync`, then `agnostic-ai list` to see which source won.

<details>
<summary>Choose project-local or machine-wide personal sources</summary>

| Name | Folder | Loaded by | Inspect with |
|---|---|---|---|
| `project-user` | `<project>/.agnostic-ai/local/` | `agnostic-ai sync` in that project | `agnostic-ai list` |
| `global-local` | `~/.agnostic-ai/local/` (or `$AGNOSTIC_AI_HOME/local/`) | `agnostic-ai sync --scope user` | `agnostic-ai list --scope user` |

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

A local spec with the same kind and name as a shared one merges into it. Write only what changes; when the team edits the shared spec, the next sync keeps your changes on top.

Shared, committed:

```md
---
name: custom
description: Draft the release notes.
model:
  claude: sonnet
  codex: terra
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

After sync, Claude uses `opus`; the description, body, and Codex model stay as shared.

Frontmatter (the YAML fields between `---` lines) merges key by key, the same way `agnostic-ai.local.yaml` merges over `agnostic-ai.yaml`:

| Local value | Result |
|---|---|
| a group of keys | merges into the shared group, key by key, at any depth |
| a single value or a list | replaces the shared value |
| `null` | removes the key |
| missing | keeps the shared value |

For example, `tools: [Read]` replaces the shared list, and `tools: null` removes it.

For settings, a local per-tool `model` or `effort` map treats a shared single value as its `default`. Shared `model: sonnet` with local `model: {codex: sol}` gives Codex `sol` and every other tool `sonnet`. A local `default` overrides that shared value.

Inside an `x-<target>` map, `null` removes one key: `x-codex.model: null` drops `model` for Codex only.

The local file's location sets its scope. `local/rules/style.md` applies to the whole project even when the shared `rules/backend/style.md` applies to `backend/` only.

YAML specs (hooks, MCP servers, settings, environments) follow the same rules: a local `mcps/docs.yaml` can change `env.PORT` and keep the shared command and args.

## Turn a shared spec off

List the targets you do not want in `targets-exclude`:

```md
---
name: review
targets-exclude: [claude, codex, cursor]
---
```

## Extend the body

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

Only personal overrides merge field by field. A project spec replaces a [pack](@/docs/packs.md) spec with the same kind and name in full.

Two files with the same name in one folder are a mistake: one wins, and `agnostic-ai lint` reports the other (LINT003).

## Extend the instructions

`.agnostic-ai/local/AGNOSTIC_AI.md` extends `.agnostic-ai/AGNOSTIC_AI.md`. Sync appends its text to every root instruction file it writes (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, and the rest), after the shared body and any inlined rules:

```md
<!-- agnostic-ai:local:start -->

Answer in Spanish. Run `make test` before every commit.

<!-- agnostic-ai:local:end -->
```

- A legacy [`outputs.<target>.rules-file`](@/docs/configuration-reference.md#entry-point-files) that names the entry point gets the block too, after the rule bodies.
- `::target` fences and `@path` imports work as in the [shared file](@/docs/configuration-reference.md#per-target-paragraphs).

In the global folder, `local/AGNOSTIC_AI.md` comes last in the managed instructions block, after the shared instructions and rules.

## Import

`agnostic-ai import` never writes your local specs into `.agnostic-ai/`, even though sync puts them in the native files import reads (such as a local rule inlined into `AGENTS.md`). Import skips every name your local folder declares and lists what it skipped. A shared spec your local folder extends keeps its own content.

Hooks are matched by event, matcher, and handler. A local handler is removed from the imported hook, and a script that only local hooks run stays out of `.agnostic-ai/scripts/`.

Settings, reviews, and environments share one native file per target. If your local folder holds one of these kinds, import leaves every shared spec of that kind unchanged and says so.

</details>

## Stay out of Git

`init` and `sync` add `/.agnostic-ai/local/` and `/agnostic-ai.local.yaml` to the managed `.gitignore` block; add nothing by hand ([gitignore](@/docs/sync-settings.md#gitignore)).

If `~/.agnostic-ai/` is a Git repository, add this line to its `.gitignore`:

```gitignore
/local/
```

## See what wins

`agnostic-ai list` prints each spec and the source that won:

```console
$ agnostic-ai list
skill   review           project-user
rule    testing          project
rule    scratch-notes    project-user
```

`agnostic-ai list --scope user` does the same for the `global` and `global-local` setups.

## Watch

`sync --watch` re-syncs when you edit a local spec or `local/AGNOSTIC_AI.md` ([`sync --watch`](@/docs/cli-reference/sync.md#sync)).

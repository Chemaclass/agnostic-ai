+++
title = "Aider"
description = "Share project rules with Aider through CONVENTIONS.md, then check the file it reads."
weight = 60

[extra]
group = "Reference"
target_id = "aider"

[extra.moved]
set-up-this-tool = "@/docs/targets/aider.md#first-rule"
+++

# Aider (`aider`)

## Set up this tool {#set-up-this-tool .legacy-anchor}

[Aider](https://aider.chat/docs/config/aider_conf.html) reads rules from `CONVENTIONS.md`. agnostic-ai writes that file, an optional `.aider.conf.yml` that loads it, and `.aiderignore`.

## First rule

1. Follow the [official setup guide](https://aider.chat/docs/install.html).
2. Select `aider` in `agnostic-ai.yaml`.
3. Follow [First rule](@/docs/getting-started.md#first-rule). The commit-prefix rule becomes a section in `CONVENTIONS.md`.

Aider does not load that file on its own. Load it with `aider --read CONVENTIONS.md`, or turn on the [project config](#output).

Already have Aider files? Read [migration](@/docs/migration.md) and the [`import aider` command](@/docs/cli-reference/import.md#import) before syncing.

## Output

| Source | Default output |
|---|---|
| Shared instructions and rules | `CONVENTIONS.md` |
| Project config | `.aider.conf.yml`, only when `conf-file` is set |
| Ignore entries | `.aiderignore`, only when the project has ignore entries |

- **Rules**: `CONVENTIONS.md` has the instructions that point to source specs, then a marked `## Rules` block with the project-wide rule bodies. `import aider` strips that block.
- **Auto-load**: set `outputs.aider.conf-file: .aider.conf.yml`. Sync adds a `read:` entry to Aider's [project config](https://aider.chat/docs/config/aider_conf.html), so you skip `--read`.
- **Ignore**: entries go to `.aiderignore`, Aider's default path. It needs no setup.

{% <details summary="Config file and ignore details"> %}
- `model` and `weak-model` go into `.aider.conf.yml` when set.
- Your other keys stay. The `read:` list has no duplicates.
- Each sync rewrites the file with sorted keys and drops its comments.
- If you point `outputs.aider.ignore-file` elsewhere, Aider reads it only when the config's `aiderignore:` key names that path ([Aider config docs](https://aider.chat/docs/config/aider_conf.html)).
- When aider leaves `targets` or `conf-file` is unset, sync removes only `model`, `weak-model`, and the `read:` entry it added. It keeps your key order and remaining comments, and removes the header.
- When `rules-file` changes, sync removes the `read:` entry it added for the old path.
{% </details> %}

## Verify

1. Run `agnostic-ai sync --check`.
2. Look for the commit-prefix rule in `CONVENTIONS.md`.
3. Run `aider --read CONVENTIONS.md` and ask for a commit subject for a small fix. It should use `fix:`.

## Config keys

| Key | Default | Notes |
|---|---|---|
| `outputs.aider.conf-file` | empty, opt-in | |
| `outputs.aider.model` | | |
| `outputs.aider.weak-model` | | |
| `outputs.aider.rules-file` | unset | writes all rules into one legacy file and skips instructions that point to source specs |
| `outputs.aider.ignore-file` | `.aiderignore` | |

With [`builtins: [handoff]`](@/docs/handoff.md), a configured `rules-file` includes the handoff instructions.

## Protected paths

Not enforced. This target takes no settings specs. Sync reports a `protected` block as unsupported. See [protected-path alternatives](@/docs/spec-format/settings.md#protected-paths).

Next, [enable automatic rule loading](#output) so you do not need `--read` on each launch.

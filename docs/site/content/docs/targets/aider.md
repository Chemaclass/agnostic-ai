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

Follow the [official setup guide](https://aider.chat/docs/install.html), select `aider` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule). The shared commit-prefix rule becomes a section in `CONVENTIONS.md`. Aider does not load this file automatically unless you enable the [project config](#output).

Already have Aider files? Review [migration](@/docs/migration.md) and the [`import aider` command](@/docs/cli-reference/import.md#import) before syncing.

## Output

| Source | Default output |
|---|---|
| Shared instructions and rules | `CONVENTIONS.md` |
| Project config | `.aider.conf.yml`, only when `conf-file` is set |
| Ignore entries | `.aiderignore`, only when the project has ignore entries |

- **Rules**: `CONVENTIONS.md` holds the instructions that point to source specs plus a marked `## Rules` block with the project-wide rule bodies. `import aider` strips that block. Load it with `aider --read CONVENTIONS.md`.
- **Auto-load**: set `outputs.aider.conf-file: .aider.conf.yml` to add a `read:` entry to Aider's [project config](https://aider.chat/docs/config/aider_conf.html), so you do not need `--read` on each launch. `model` and `weak-model` go into the same file when set. Your other keys stay, and the `read:` list has no duplicates. Each sync rewrites the file with sorted keys and drops its comments.
- **Ignore**: ignore entries go to `.aiderignore` in the project root. That is Aider's default path ([Aider config docs](https://aider.chat/docs/config/aider_conf.html)), so it needs no setup. If you point `outputs.aider.ignore-file` elsewhere, Aider reads it only when the config file's `aiderignore:` key names that path.

{% <details summary="What sync removes from .aider.conf.yml"> %}
When aider leaves `targets` or `conf-file` is unset, sync removes only `model`, `weak-model`, and the `read:` entry it added. It preserves your key order and remaining comments, and removes the header. When `rules-file` changes, sync removes the `read:` entry it added for the old path.
{% </details> %}

## Verify

Run `agnostic-ai sync --check` and inspect `CONVENTIONS.md` for the commit-prefix rule. Start Aider with `aider --read CONVENTIONS.md`. Ask it to write a commit subject for a small fix; it should use `fix:`.

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

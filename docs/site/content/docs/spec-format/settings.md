+++
title = "Settings"
description = "settings/: one permission policy, default model, and effort for every tool."
weight = 70

[extra]
group = "Reference"

[extra.moved]
reviews = "@/docs/spec-format/reviews.md"
environments = "@/docs/spec-format/environments.md"
ignore = "@/docs/spec-format/ignore.md"
overwrite-behaviour = "@/docs/spec-format/ignore.md"
+++

# Settings

`settings/` sets what agents may run without asking, what they must never run, and which model and effort they start with. Claude Code keeps this in `.claude/settings.json`, Factory in command lists, Codex in `config.toml`. A settings spec states it once.

# Settings

`settings/` sets what agents may run without asking, what they must never run, and which model and effort they start with. Claude Code keeps this in `.claude/settings.json`, Factory in command lists, Codex in `config.toml`. A settings spec states it once.

## Write one {#write-one}

1. Run `agnostic-ai new settings project-defaults`. It creates an empty template.
2. Edit `.agnostic-ai/settings/project-defaults.yaml`:

```yaml
permissions:
  allow:
    - shell(go test:*)
model:
  claude: sonnet
  codex: sol
```

3. Run `agnostic-ai explain settings/project-defaults.yaml`, then sync.

Claude Code gets the permission in `.claude/settings.json`. Codex gets the model, and shell rules only with `outputs.codex.exec-policies-from-permissions: true` (see the [translation limits](@/docs/targets/codex-reference.md#translate-bash-permissions)). Other tools are in [support by target](@/docs/spec-format/settings-reference.md#support-by-target).

`explain` and `compare` show the written policy, not what the tool enforces. Confirm approval behavior in your coding tool.

## Fields {#fields}

| Field | Default | Description |
|-------|---------|-------------|
| `permissions.allow` | empty | Rules approved without prompting. |
| `permissions.deny` | empty | Rules always blocked. |
| `permissions.ask` | empty | Rules that prompt before running. |
| `permissions.default-mode` | unset | Claude Code's starting mode for `sync --scope user`: `default`, `manual`, `acceptEdits`, `plan`, `auto`, `dontAsk`, or `bypassPermissions`. Other tools raise a sync note. |
| `model` | empty | A string, a map per target with an optional `default` (like [agent `model`](@/docs/spec-format/agents.md#per-target-model-and-effort)), or a [tier](@/docs/spec-format/agents.md#model-tiers) name. A target with no entry and no `default` gets no model. |
| `effort` | empty | A single value, or a map per target with an optional `default`. An agent's own `effort` is separate. See [effort by target](@/docs/spec-format/settings-reference.md#effort-by-target). |
| `session-retention-days` | unset | Personal age limit for sessions, an integer of 1 or more. Only `sync --scope user` writes it. See [session retention](@/docs/spec-format/settings-reference.md#session-retention). |
| `voice` | unset | Personal dictation: `enabled`, `mode` (`hold` or `tap`), `language`. Only `sync --scope user` writes it. See [voice dictation](@/docs/spec-format/settings-reference.md#voice-dictation). |
| `protected` | unset | Files agents must not edit without asking. See [protected paths](#protected-paths). |

All fields are optional. Tool-specific keys go under `x-<target>` ([native keys](@/docs/spec-format/settings-reference.md#target-specific-keys)).

## Permission rules {#permission-rules}

A rule uses the [capability](@/docs/spec-format/agents.md#capabilities) names an agent's `can:` takes. Claude Code keeps the scope:

| Rule | Claude Code |
|------|-------------|
| `read(src/**)` | `Read(src/**)` |
| `edit(src/**)` | `Edit(src/**)` |
| `shell(git status)` | `Bash(git status)` |
| `mcp:github/get_issue` | `mcp__github__get_issue` |

- `write` takes no path. Write `edit(<path>)` to cover a file; it covers writes too.
- `web` has no scoped form. Use the tool's own permission fields, or the `WebFetch(domain:example.com)` alias.
- Put a `shell` wildcard at the end of an `allow` or `deny` rule, as in `shell(git push:*)`.
- A bare capability under `allow` or `ask` covers the whole tool.
- `lint` and `sync` stop on a rule they cannot read.

<details>
<summary>Details: aliases, warnings, wildcards, migration</summary>

- Each rule becomes the tool's own permission name. `delete` raises a sync note where a tool has no delete permission.
- A Claude Code rule still works as an alias, such as `WebFetch(domain:go.dev)`, and a list can mix both. A Claude Code rule is a bare tool name (the whole tool) or `Scope(argument)`. `Scope()` with an empty argument is dropped, not read as the bare tool, which would widen it.
- Claude Code checks file writes against `Edit` rules only.
- `validate`, `lint` (LINT036), and `sync` stop on a rule they cannot read, including one in a pack and an unquoted rule that YAML reads as a mapping, since a tool would drop it.
- `agnostic-ai migrate --only capabilities` rewrites each Claude Code rule that has a capability of its own, and an adjacent `WebFetch, WebSearch` pair as `web`. The rest stay as aliases, and sync writes the same files.
- `explain settings/<name>.yaml` lists the rules each tool gets and any extra access. `on-unsupported: error` fails when a rule widens, such as a Codex exact shell allow that becomes a command prefix.
- Older releases matched lowercase bare capabilities to no tool, so an old spec now grants more: `lint` warns (LINT038, which `lint --strict` fails on) and `sync` prints a note once per run, naming the affected permissions. `on-unsupported: silent` or `sync --quiet` hides the note. Scoped rules, `deny` rules, tool-specific overrides that replace the rule, and user lists (written only for Claude Code) get no warning.
- `shell(git * main)` also approves options placed at the `*`, and Claude Code matches a mid-command `*` in a `deny` rule literally, so it blocks nothing. `lint` reports both as LINT009.

</details>

## Protected paths {#protected-paths}

A `protected` block says "do not edit these files without asking" once, for CI workflows, lock files, generated code, or migrations. Sync uses each tool's strongest guard.

```yaml
protected:
  paths:
    - .github/**
    - composer.lock
  decision: ask     # ask (default) or deny
  reason: CI and the lock file change only on purpose.
```

| Field | Default | Description |
|-------|---------|-------------|
| `paths` (required) | | Globs from the project root. `composer.lock` protects only the root file; `**/composer.lock` every copy. |
| `decision` | `ask` | `ask` makes the agent ask first. `deny` blocks the edit. |
| `reason` | empty | Shown to the agent when it is blocked. |

| Target | Protection | How |
|---|---|---|
| Claude Code | enforced (permission) | `Edit(/<path>)` rules in `permissions.ask` or `permissions.deny` ([details](@/docs/targets/claude.md#protected-paths)) |
| Codex | enforced (hook) | a generated `PreToolUse` hook in `.codex/hooks/` that blocks a matching `apply_patch` ([details](@/docs/targets/codex.md#protected-paths)) |
| Cursor | enforced (permission), CLI only, `deny` only | `Write(<path>)` rules in `permissions.deny` of `.cursor/cli.json`; `ask` is not enforced ([details](@/docs/targets/cursor.md#protected-paths)) |
| Gemini CLI | enforced (hook) | a generated `BeforeTool` hook in `.gemini/hooks/` that blocks a matching `write_file` or `replace` ([details](@/docs/targets/gemini.md#protected-paths)) |
| Every other target | not enforced | a sync note. State the paths in a rule |

Protection covers the agent's edit tools; a shell command can still change the file.

<details>
<summary>Details: path syntax, case, lint codes</summary>

- `*` and `?` match inside one path segment, `**` across segments. A path covers every file under it (`vendor` protects `vendor/a.go`); a trailing `/` means the directory's contents.
- Character classes, braces, negation, and paths outside the project are rejected, because tools would read them differently.
- Each settings spec holds one block, so use one file per decision.
- The Codex and Gemini CLI hooks and `lint` ignore case, so `.GITHUB/ci.yml` counts as protected. Claude Code's docs do not say whether `Edit` rules ignore case.
- `lint` warns (LINT022) when a protected path covers a file sync writes, since sync rewrites it, and reports an invalid block as LINT023.
- `sync --scope user` does not write protected paths.

</details>

## Merging {#merging}

Several settings files merge. Permission lists are joined in source order with duplicates removed. The last `model` and `effort` set wins, per target, so `model: {codex: gpt-6-luna}` in a later file changes only Codex.

Removing a rule from a spec removes it from Claude Code's `settings.json` on the next sync. Rules you wrote there by hand stay ([Claude settings](@/docs/targets/claude-reference.md#claude-settings)).

<details>
<summary>Details: compare and user-level sync</summary>

- `agnostic-ai compare claude codex` reports each portable permission rule, including partial mappings, native overrides, output prerequisites, and that `permissions.default-mode` is written only by `sync --scope user`. It does not compare other settings fields.
- `sync --scope user` also reads settings specs from your home folder, for `model`, `effort`, tool-specific keys, `permissions.default-mode`, and, for Claude Code only, the `permissions` lists. See [default model and effort](@/docs/global-configuration-reference.md#global-default-model-and-effort).

</details>

## Edit scalar settings in the browser {#edit-scalar-settings-in-the-browser}

Run `agnostic-ai ui` in a project, or `agnostic-ai ui --scope user` for user settings. In the Settings view, save or clear a `model` or `effort` value for every tool, or for one tool under **What each tool gets**. A browser save does not sync: run `agnostic-ai sync` (or `sync --scope user`) afterward.

<details>
<summary>Details: which file it writes, what it cannot edit</summary>

- A single-tool value writes `model.<tool>` or `effort.<tool>`.
- Only the user-level view edits `session-retention-days`.
- The first save creates `settings/defaults.yaml` when no settings spec exists; with several, choose the file.
- `x-*` keys, nested values, and a `model` that names a model tier need the source file.
- A personal value for one tool writes only that tool's entry; the shared value still reaches the other tools (see [local overrides](@/docs/local-overrides.md)).

</details>

## More

<span id="session-retention" hidden></span>
<span id="voice-dictation" hidden></span>
<span id="effort-by-target" hidden></span>
<span id="support-by-target" hidden></span>
<span id="target-specific-keys" hidden></span>

- [Session retention](@/docs/spec-format/settings-reference.md#session-retention), [voice dictation](@/docs/spec-format/settings-reference.md#voice-dictation), [effort by target](@/docs/spec-format/settings-reference.md#effort-by-target), [support by target](@/docs/spec-format/settings-reference.md#support-by-target) and [native keys](@/docs/spec-format/settings-reference.md#target-specific-keys) are in the [settings reference](@/docs/spec-format/settings-reference.md).

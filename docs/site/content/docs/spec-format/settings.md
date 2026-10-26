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

## Write one

`agnostic-ai new settings project-defaults` creates a template with no permissions or model. Settings specs are plain YAML. Save this as `.agnostic-ai/settings/project-defaults.yaml`:

```yaml
permissions:
  allow:
    - shell(go test:*)
model:
  claude: sonnet
  codex: sol
```

Claude Code writes the permission into `.claude/settings.json`. Codex writes the model, but writes shell rules only with `outputs.codex.exec-policies-from-permissions: true`; review the [translation limits](@/docs/targets/codex-reference.md#translate-bash-permissions) first. Other tools are listed [below](#support-by-target).

Run `agnostic-ai explain settings/project-defaults.yaml`, then sync. `explain` and `compare` show the written policy, not what the tool enforces, so confirm approval behavior in your coding tool.

## Fields

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `permissions.allow` | no | empty | Rules approved without prompting. |
| `permissions.deny` | no | empty | Rules always blocked. |
| `permissions.ask` | no | empty | Rules that prompt before running. |
| `permissions.default-mode` | no | unset | Claude Code's starting mode for `sync --global`: `default`, `manual`, `acceptEdits`, `plan`, `auto`, `dontAsk`, or `bypassPermissions`. Other tools raise a sync note. |
| `model` | no | empty | Default model: a string, a map per target with an optional `default` (like [agent `model`](@/docs/spec-format/agents.md#per-target-model-and-effort)), or a [tier](@/docs/spec-format/agents.md#model-tiers) name. A target with no entry and no `default` gets no model. |
| `effort` | no | empty | Default reasoning effort: a single value, or a map per target with an optional `default`. An agent's own `effort` is separate. See [effort by target](#effort-by-target). |
| `session-retention-days` | no | unset | Personal session age limit, an integer of 1 or more. Only `sync --global` writes it. See [session retention](#session-retention). |
| `protected` | no | unset | Files agents must not edit without asking. See [protected paths](#protected-paths). |

## Session retention

Keep this personal setting under `~/.agnostic-ai/settings/`, outside team project specs:

```yaml
# ~/.agnostic-ai/settings/retention.yaml
session-retention-days: 3650
```

`sync --global` writes the tool's own age limit; unset leaves it alone. Values that are not an integer of 1 or more fail validation (LINT045). Project sync only prints a note.

| Tool | User settings file | Native key |
|------|--------------------|------------|
| Claude Code | `~/.claude/settings.json` | `cleanupPeriodDays: 3650` |
| CodeBuddy Code | `~/.codebuddy/settings.json` | `cleanupPeriodDays: 3650` |
| Gemini CLI | `~/.gemini/settings.json` | `general.sessionRetention.maxAge: "3650d"` |
| Qoder CLI | `~/.qoder/settings.json` | `general.sessionRetention.maxAge: "3650d"` |

- An `x-claude.cleanupPeriodDays` or `x-codebuddy.cleanupPeriodDays` value wins for its target.
- An existing invalid Claude value, including `cleanupPeriodDays: 0`, can pause cleanup. Sync leaves it unchanged, even with `--backup`, and asks you to fix it by hand or set `x-claude.cleanupPeriodDays`.
- If Gemini or Qoder has `general.sessionRetention.enabled: false`, sync leaves its age limit alone and prints a note. It never sets the cleanup switch or `maxCount`, and Gemini's existing `maxCount` can still remove younger sessions.
- Other targets print a note. Kilo's deletion switch and Factory's organization setting are outside this field.
- Lowering the limit warns that older sessions will be deleted, including when removing the field restores a shorter native limit. Pass `--backup` to replace a native value set outside agnostic-ai.
- Removing the field restores the original value's bytes, even one that already matched, and removes an empty file that sync created. A value changed by hand stays yours. This restore data needs ownership state version 7; older releases refuse that state rather than discard it.
- `explain --global settings/retention.yaml` lists the mapped keys.

agnostic-ai changes settings only; it never reads, copies, or archives transcripts. Claude Code also applies this cutoff to some other session data and orphaned worktrees, and Desktop and Cowork transcripts have a separate setting. See [Claude cleanup](https://code.claude.com/docs/en/claude-directory), [Gemini configuration](https://geminicli.com/docs/reference/configuration/), and [Qoder settings](https://docs.qoder.com/cli/settings-reference).

## Protected paths

A `protected` block states "do not edit these files without asking" once, for CI workflows, lock files, generated code, or migrations. Sync writes the strongest guard each tool has.

```yaml
protected:
  paths:
    - .github/**
    - composer.lock
  decision: ask     # ask (default) or deny
  reason: CI and the lock file change only on purpose.
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `paths` | yes | | Globs from the project root, so `composer.lock` protects only the root file and `**/composer.lock` every copy. `*` and `?` match inside one path segment, `**` across segments. A path covers every file under it (`vendor` protects `vendor/a.go`); a trailing `/` means the directory's contents. |
| `decision` | no | `ask` | `ask` makes the agent ask first. `deny` blocks the edit. |
| `reason` | no | empty | Shown to the agent when it is blocked from an edit. |

- Character classes, braces, negation, and paths outside the project are rejected, because tools would read them differently.
- Each settings spec holds one block, so use one file per decision.
- The Codex and Gemini CLI hooks and `lint` ignore case, so `.GITHUB/ci.yml` counts as protected. Claude Code's docs do not say whether `Edit` rules ignore case.

| Target | Protection | How |
|---|---|---|
| Claude Code | enforced (permission) | `Edit(/<path>)` rules in `permissions.ask` or `permissions.deny` ([details](@/docs/targets/claude.md#protected-paths)) |
| Codex | enforced (hook) | a generated `PreToolUse` hook in `.codex/hooks/` that blocks a matching `apply_patch` ([details](@/docs/targets/codex.md#protected-paths)) |
| Cursor | enforced (permission), CLI only, `deny` only | `Write(<path>)` rules in `permissions.deny` of `.cursor/cli.json`; `ask` is not enforced ([details](@/docs/targets/cursor.md#protected-paths)) |
| Gemini CLI | enforced (hook) | a generated `BeforeTool` hook in `.gemini/hooks/` that blocks a matching `write_file` or `replace` ([details](@/docs/targets/gemini.md#protected-paths)) |
| Every other target | not enforced | a sync note. State the paths in a rule |

Protection covers the agent's edit tools; a shell command can still change the file. `lint` warns (LINT022) when a protected path covers a file sync writes, since sync rewrites it, and reports an invalid block as LINT023. `sync --global` does not write protected paths.

## Permission rules

A rule uses the [capability](@/docs/spec-format/agents.md#capabilities) names an agent's `can:` takes. Unlike an agent's tool list, a permission rule keeps its scope on Claude Code: `read(src/**)` becomes `Read(src/**)`, `edit(src/**)` becomes `Edit(src/**)`, `shell(git status)` becomes `Bash(git status)`, and `mcp:github/get_issue` becomes `mcp__github__get_issue`.

- Each rule becomes the tool's own permission name. `delete` raises a sync note where a tool has no delete permission.
- A Claude Code rule still works as an alias, such as `WebFetch(domain:go.dev)`, and a list can mix both. A Claude Code rule is a bare tool name (the whole tool) or `Scope(argument)`. `Scope()` with an empty argument is dropped, not read as the bare tool, which would widen it.
- `write` takes no path. Claude Code checks file writes against `Edit` rules only, so write `edit(<path>)` to cover a file; it also covers writes.
- `web` has no scoped form. To limit web access, use the tool's own permission fields, or the `WebFetch(domain:example.com)` alias.
- `validate`, `lint` (LINT036), and `sync` stop on a rule they cannot read, including one in a pack and an unquoted rule that YAML reads as a mapping, since a tool would drop it.
- `agnostic-ai migrate --only capabilities` rewrites each Claude Code rule that has a capability of its own, and an adjacent `WebFetch, WebSearch` pair as `web`. The rest stay as aliases, and sync writes the same files.
- `explain settings/<name>.yaml` lists the rules each tool gets and any extra access. `on-unsupported: error` fails when a rule widens, such as a Codex exact shell allow that becomes a command prefix.

A bare capability under `allow` or `ask` covers the whole tool. Older releases matched these lowercase rules to no tool, so an old spec now grants more: `lint` warns (LINT038, which `lint --strict` fails on) and `sync` prints a note once per run, naming the affected permissions. `on-unsupported: silent` or `sync --quiet` hides the note. Scoped rules, `deny` rules, tool-specific overrides that replace the rule, and global lists (written only for Claude Code) get no warning.

Keep a `shell` wildcard at the end of an `allow` or `deny` rule. `shell(git * main)` also approves options placed at the `*`, and Claude Code matches a mid-command `*` in a `deny` rule literally, so it blocks nothing. `lint` reports both as LINT009.

## Merging

Several files merge. Permission lists are joined in source order with duplicates removed. The last `model` and `effort` set wins, per target, so `model: {codex: gpt-6-luna}` in a later file changes only Codex.

Removing a rule from a spec removes it from Claude Code's `settings.json` on the next sync. Rules you wrote there by hand stay ([Claude settings](@/docs/targets/claude-reference.md#claude-settings)).

`agnostic-ai compare claude codex` reports each portable permission rule, including partial mappings, native overrides, output prerequisites, and that `permissions.default-mode` is written only by `sync --global`. It does not compare other settings fields.

`sync --global` also reads settings specs from your home folder, for `model`, `effort`, tool-specific keys, `permissions.default-mode`, and, for Claude Code only, the `permissions` lists. See [default model and effort](@/docs/global-configuration.md#global-default-model-and-effort).

## Effort by target

`effort` reaches four tools. A value the tool does not accept, or `effort` on any other tool, raises a sync note instead. `x-<target>` wins, so `x-claude.effortLevel` overrides the portable value. `import` fills `effort` from these keys when the tool accepts the value and no other settings spec sets it; otherwise the value goes under `x-<target>`.

| Target | Native key | Accepted values |
|---|---|---|
| Claude Code | `effortLevel` in `.claude/settings.json` | `low`, `medium`, `high`, `xhigh` |
| Copilot | `effortLevel` in `.github/copilot/settings.json` | `low`, `medium`, `high`, `xhigh` |
| Codex | `model_reasoning_effort` in `.codex/config.toml` | any string; `outputs.codex.config.model-reasoning-effort` and the captured overlay win |
| Factory | `reasoningEffort` in `.factory/settings.json` | `none`, `dynamic`, `off`, `minimal`, `low`, `medium`, `high`, `xhigh`, `max`; each model accepts a subset |

## Support by target

| Target | `permissions` | `model` |
|---|---|---|
| Claude Code, Qoder, Kilo Code, OpenCode | yes | yes |
| Factory | `Bash` rules only; `ask` goes to `commandDenylist` (which prompts), `deny` to `commandBlocklist` | yes |
| Codex | `Bash` rules via `outputs.codex.exec-policies-from-permissions` ([translation](@/docs/targets/codex-reference.md#translate-bash-permissions)); otherwise a note | yes |
| Windsurf | yes | no |
| Augment | `allow` and `deny` only, for whole `read`, `edit`, and `write` tools | no |
| Cursor | `allow` and `deny` in `.cursor/cli.json`, for the Cursor CLI ([details](@/docs/targets/cursor.md#permissions)) | no |
| Copilot, Junie, Gemini | no; Copilot and Junie report the whole policy in a note | yes |

Every other target takes neither. A field a tool cannot hold raises a sync note, and the other tools still get their files. A rule the vendor cannot scope, such as a path rule on Factory's shell lists, raises a note instead of widening.

A shared Claude model name raises the same note as an [agent `model`](@/docs/spec-format/agents.md#per-target-model-and-effort) on Codex, Gemini, OpenCode, Kilo Code, and Factory; Codex skips it when `outputs.codex.config.model` or the [captured overlay](@/docs/targets/codex-reference.md#codex-config) sets the model. Review an imported `model` before you enable more tools, because model names differ between vendors.

## Target-specific keys

Tool-specific keys go under `x-<target>` and merge into that tool's settings file (`x-factory.sandbox` reaches `.factory/settings.json`). When sync also writes the same key:

- Lists are joined, with the entries sync writes first.
- Objects merge key by key, at every level.
- A single value such as `model` is replaced.
- A list and a string cannot merge. The `x-<target>` value wins, with a sync note.
- Maps of whole records (`x-qoder.mcpServers`, `x-augment.mcpServers`) merge by name. A server both sides name comes whole from the `x-<target>` block.
- Blocks with a fixed order (`x-qoder.hooks`, `x-augment.hooks`) keep their order, with your own events added at the end.

Four keys take one type each: `x-augment.toolPermissions` a list, and `x-windsurf.permissions`, `x-kilo.permission`, and `x-opencode.permission` an object. Any other type is skipped with a sync note naming the key, and the rules from your `permissions` are still written.

Exceptions:

- Codex: `.codex/config.toml` comes from the captured overlay plus `outputs.codex.config`, so an `x-codex` block raises a sync note that names both.
- Ellipsis reads no portable field, only `x-ellipsis`: its reviewer, pull request filters, posting rules, and budget go to `.ellipsis/code-review.yaml`, and the environment, its repository, and native options to `.ellipsis/environments/<name>.yaml`. A later spec replaces each top-level block it sets ([details](@/docs/targets/ellipsis-reference.md#how-specs-map-to-the-file)).
- OpenCode V2 MCP imports use `x-opencode.mcp` to keep native connection details, disabled states, OAuth settings, and separate timeouts. Keep it in a settings spec, not in MCP specs. See [OpenCode](@/docs/targets/opencode.md).

## Edit scalar settings in the browser

Run `agnostic-ai serve --open` in a project, or `agnostic-ai serve --global --open` for user settings. The Settings view saves, updates, and clears a single `model` or `effort` value; only the global view edits `session-retention-days`. The first save creates `settings/defaults.yaml` when no settings spec exists; with several, choose the file. Maps and `x-*` keys need the source file. A browser save does not sync: run `agnostic-ai sync` or `agnostic-ai sync --global` afterward.

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

`settings/` sets what agents may run without asking, what they must never run, and which model and effort they start with. Every tool spells this differently: Claude Code `permissions` in `.claude/settings.json`, Factory command lists, a Codex `config.toml`. A settings spec states the policy once.

- **One policy.** Allow `go test`, deny `rm`, and ask before pushing, in every tool that supports permissions.
- **Safe translation.** A rule a tool cannot express raises a coverage note instead of being widened.
- **Protected files.** List the files an agent must not edit without asking, and each tool gets its own guard for them.
- **One default model.** Set it per tool with a fallback, since model names differ between vendors.
- **Tool-only keys too.** An `x-<target>` block merges into that tool's own settings file.

## Write one

Pure YAML, one file per settings group, such as `settings/permissions.yaml`.

```yaml
permissions:
  allow:
    - Bash(go test:*)
  deny:
    - Bash(rm:*)
  ask:
    - Bash(git push:*)
model: claude-opus-4-8
effort:
  claude: xhigh
  default: high
```

## Fields

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `permissions.allow` | no | empty | Rules approved without prompting. |
| `permissions.deny` | no | empty | Rules always blocked. |
| `permissions.ask` | no | empty | Rules that prompt before running. |
| `permissions.default-mode` | no | unset | Claude Code starting mode for `sync --global`: `default`, `manual`, `acceptEdits`, `plan`, `auto`, `dontAsk`, or `bypassPermissions`. Other targets raise a coverage note. |
| `model` | no | empty | Default model: a string, or a map per target with an optional `default`, like [agent `model`](@/docs/spec-format/agents.md#per-target-model-and-effort), or a [tier](@/docs/spec-format/agents.md#model-tiers) name. A target with no entry and no `default` gets no model. |
| `effort` | no | empty | Default reasoning effort: a scalar, or a map per target with an optional `default`, like [agent `effort`](@/docs/spec-format/agents.md#per-target-model-and-effort). Separate from an agent's own `effort`. |
| `protected` | no | unset | Files agents must not edit without asking: `paths`, `decision` (`ask` or `deny`), and `reason`. See [protected paths](#protected-paths). |

## Protected paths

"Do not edit these files without asking" is a common project rule: CI workflows, lock files, generated code, migrations. A `protected` block states it once, and sync writes each target's strongest native form of it.

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
| `paths` | yes | | Globs relative to the project root. `*` and `?` match inside one path segment, `**` matches across segments, and a path also covers every file under it, so `vendor` protects `vendor/a.go`. A trailing `/` means the directory's contents. |
| `decision` | no | `ask` | `ask` makes the agent ask first. `deny` blocks the edit. |
| `reason` | no | empty | Shown to the agent when an edit is blocked. |

A path is anchored at the project root: `composer.lock` protects only the root file, and `**/composer.lock` protects every copy. Character classes, braces, negation, and paths outside the project are rejected, because the targets would read them differently. Each settings spec holds one block, so use one file per decision. The Codex and Gemini CLI hooks and `lint` ignore case, so `.GITHUB/ci.yml` counts as protected on a case-insensitive file system. Claude Code's permission docs do not say whether `Edit` rules ignore case.

| Target | Protection | How |
|---|---|---|
| Claude Code | enforced (permission) | `Edit(/<path>)` rules in `permissions.ask` or `permissions.deny` ([details](@/docs/targets/claude.md#protected-paths)) |
| Codex | enforced (hook) | a generated `PreToolUse` hook in `.codex/hooks/` that blocks a matching `apply_patch` ([details](@/docs/targets/codex.md#protected-paths)) |
| Cursor | enforced (permission), CLI only, `deny` only | `Write(<path>)` rules in `permissions.deny` of `.cursor/cli.json`; `ask` stays advisory ([details](@/docs/targets/cursor.md#protected-paths)) |
| Gemini CLI | enforced (hook) | a generated `BeforeTool` hook in `.gemini/hooks/` that blocks a matching `write_file` or `replace` ([details](@/docs/targets/gemini.md#protected-paths)) |
| Every other target | advisory | a coverage note on sync; state the paths in a rule |

Protection covers the agent's edit tools. A shell command or script that writes the file directly can still change it. `agnostic-ai lint` warns (LINT022) when a protected path covers a file sync writes, since sync regenerates that file from its source spec, and reports an invalid block as LINT023. `sync --global` does not write protected paths.

## Permission rules

A rule is a bare tool name (whole tool) or `Scope(argument)`. An MCP tool is `mcp__<server>__<tool>`. `Scope()` with an empty argument is dropped, not read as the bare tool, which would widen it.

Keep a `Bash` wildcard at the end of an `allow` or `deny` rule. `Bash(git * main)` also approves options inserted at the `*`, and Claude Code matches a mid-command `*` in a `deny` rule literally, so it blocks nothing. `agnostic-ai lint` reports both as LINT009.

## Merging

Multiple files merge: permission lists concatenate, de-duplicated in source order, and the last non-empty `model` and `effort` win. Each target resolves its own map entry first, so `model: {codex: gpt-6-luna}` in a later file changes only Codex.

Removing a rule from a spec removes it from Claude Code's `settings.json` on the next sync; rules you wrote there by hand stay ([Claude settings](@/docs/targets/claude.md#claude-settings)).

`sync --global` also reads settings specs from the home, for `model`, `effort`, target-specific keys, and `permissions.default-mode`; see [default model and effort](@/docs/configuration.md#global-default-model-and-effort).

## Effort by target

`effort` reaches four targets, each under its own key. A value the target does not accept is not written and raises a coverage note; the other targets still emit. `x-<target>` wins, so `x-claude.effortLevel` overrides the portable value. Every other target reports a coverage note.

`import` fills `effort` from these keys when the target accepts the value and no other settings spec sets it; otherwise the value lands under `x-<target>`.

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
| Factory | `Bash` rules only | yes |
| Codex | `Bash` rules via `outputs.codex.exec-policies-from-permissions` | yes |
| Windsurf | yes | no |
| Augment | `allow` and `deny` only | no |
| Copilot, Junie, Gemini | no | yes |

Every other target takes neither. A field a target cannot represent produces a coverage note while the others still emit. Copilot and Junie report the whole policy. Codex does too unless `outputs.codex.exec-policies-from-permissions: true` turns simple Bash rules into exec policies; the rest raise a note ([Bash permission translation](@/docs/targets/codex.md#translate-bash-permissions)). Rules translate only as far as the vendor allows: Augment gates `read`, `edit`, and `write` as whole tools, and Factory's command lists take shell patterns, so a path-scoped rule raises a note instead of widening. Factory's `commandDenylist` prompts, so portable `ask` goes there and `deny` goes to `commandBlocklist`. Review an imported `model` before enabling more targets, since identifiers differ between vendors. A shared Claude model name raises the same coverage note as an [agent `model`](@/docs/spec-format/agents.md#per-target-model-and-effort) on Codex, Gemini, OpenCode, Kilo Code, and Factory. Codex skips it when `outputs.codex.config.model` or the [captured overlay](@/docs/targets/codex.md#codex-config) sets the model.

## Target-specific keys

Target-specific keys go under `x-<target>` and merge into that target's settings file (`x-factory.sandbox` reaches `.factory/settings.json`). Codex is the exception: its `.codex/config.toml` comes from the captured overlay plus `outputs.codex.config`, so an `x-codex` block raises a coverage note naming both routes.

On a key this tool also writes, lists union (translated entries first), objects merge recursively, and a scalar such as `model` is replaced. A list against a string cannot merge: the `x-<target>` value wins with a coverage note. Maps of whole records (`x-qoder.mcpServers`, `x-augment.mcpServers`) merge by name, and a server both sides name comes from the `x-<target>` block entire (#974). Fixed-order blocks (`x-qoder.hooks`, `x-augment.hooks`) keep their order, with your own events appended (#976).

Four keys take one shape each: `x-augment.toolPermissions` a list; `x-windsurf.permissions`, `x-kilo.permission`, `x-opencode.permission` an object. Another shape is skipped, the translated rules ship, and a coverage note names the key (#976).

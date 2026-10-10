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

`settings/` sets what agents may run without asking, what they must never run, and which model and effort they start with. Every tool spells this differently: Claude Code uses `permissions` in `.claude/settings.json`, Factory uses command lists, Codex uses `config.toml`. A settings spec states the policy once.

- **One policy.** Allow `go test`, deny `rm`, and ask before pushing, in every tool that supports permissions.
- **No silent widening.** A rule a tool cannot express raises a coverage note.
- **Protected files.** List the files an agent must not edit without asking. Each tool gets its own guard.
- **One default model.** Set it per tool with a fallback, since model names differ between vendors.
- **Tool-only keys.** An `x-<target>` block merges into that tool's own settings file.

## Write one

Settings specs are plain YAML, one file per group of settings, such as `settings/permissions.yaml`.

```yaml
permissions:
  allow:
    - read(src/**)
    - shell(go test:*)
  deny:
    - shell(rm:*)
    - edit(.env)
  ask:
    - shell(git push:*)
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
| `permissions.default-mode` | no | unset | Claude Code's starting mode for `sync --global`: `default`, `manual`, `acceptEdits`, `plan`, `auto`, `dontAsk`, or `bypassPermissions`. Other tools raise a coverage note. |
| `model` | no | empty | Default model: a string, a map per target with an optional `default` (like [agent `model`](@/docs/spec-format/agents.md#per-target-model-and-effort)), or a [tier](@/docs/spec-format/agents.md#model-tiers) name. A target with no entry and no `default` gets no model. |
| `effort` | no | empty | Default reasoning effort: a single value, or a map per target with an optional `default` (like [agent `effort`](@/docs/spec-format/agents.md#per-target-model-and-effort)). An agent's own `effort` is separate. |
| `protected` | no | unset | Files agents must not edit without asking: `paths`, `decision` (`ask` or `deny`), and `reason`. See [protected paths](#protected-paths). |

## Protected paths

"Do not edit these files without asking" is a common rule for CI workflows, lock files, generated code, and migrations. A `protected` block states it once. Sync writes the strongest guard each tool has.

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
| `paths` | yes | | Globs relative to the project root. `*` and `?` match inside one path segment. `**` matches across segments. A path also covers every file under it, so `vendor` protects `vendor/a.go`. A trailing `/` means the directory's contents. |
| `decision` | no | `ask` | `ask` makes the agent ask first. `deny` blocks the edit. |
| `reason` | no | empty | Shown to the agent when it is blocked from an edit. |

A path starts at the project root. `composer.lock` protects only the root file. `**/composer.lock` protects every copy.

- Character classes, braces, negation, and paths outside the project are rejected, because tools would read them differently.
- Each settings spec holds one block, so use one file per decision.
- The Codex and Gemini CLI hooks and `lint` ignore case, so `.GITHUB/ci.yml` counts as protected. Claude Code's permission docs do not say whether `Edit` rules ignore case.

| Target | Protection | How |
|---|---|---|
| Claude Code | enforced (permission) | `Edit(/<path>)` rules in `permissions.ask` or `permissions.deny` ([details](@/docs/targets/claude.md#protected-paths)) |
| Codex | enforced (hook) | a generated `PreToolUse` hook in `.codex/hooks/` that blocks a matching `apply_patch` ([details](@/docs/targets/codex.md#protected-paths)) |
| Cursor | enforced (permission), CLI only, `deny` only | `Write(<path>)` rules in `permissions.deny` of `.cursor/cli.json`; `ask` is not enforced ([details](@/docs/targets/cursor.md#protected-paths)) |
| Gemini CLI | enforced (hook) | a generated `BeforeTool` hook in `.gemini/hooks/` that blocks a matching `write_file` or `replace` ([details](@/docs/targets/gemini.md#protected-paths)) |
| Every other target | not enforced | a coverage note on sync. State the paths in a rule |

Protection covers the agent's edit tools. A shell command that writes the file can still change it.

- `agnostic-ai lint` warns (LINT022) when a protected path covers a file sync writes, because sync rewrites that file from its spec.
- `lint` reports an invalid block as LINT023.
- `sync --global` does not write protected paths.

## Permission rules

A rule is a [capability](@/docs/spec-format/agents.md#capabilities), the same names an agent's `can:` takes. In a permission list, `read` and `edit` also take a path pattern.

| Rule | Covers | Claude Code rule |
|------|--------|------------------|
| `read`, `read(<path>)` | Reading files, or the files that match | `Read`, `Read(<path>)` |
| `edit`, `edit(<path>)` | Changing files, or the files that match | `Edit`, `Edit(<path>)` |
| `write` | The write tool | `Write` |
| `delete` | Deleting files, where the target has a delete tool | No native tool |
| `shell`, `shell(<pattern>)` | Every command, or the commands that match | `Bash`, `Bash(<pattern>)` |
| `web` | Fetching pages and searching the web | `WebFetch` and `WebSearch` |
| `mcp:<server>`, `mcp:<server>/<tool>` | One MCP server, or one of its tools | `mcp__<server>`, `mcp__<server>__<tool>` |

- Each rule becomes the tool's own permission name. `delete` raises a coverage note where a tool has no delete permission.
- A Claude Code rule still works as an alias, such as `WebFetch(domain:go.dev)`. A list can mix both.
- `write` takes no path. Claude Code checks file writes against `Edit` rules only and never consults a `Write(<path>)` rule, so write `edit(<path>)` to cover a file.
- `validate`, `lint` (LINT036), and `sync` stop on a rule they cannot read, including one in a pack and an unquoted rule that YAML reads as a mapping, since a tool would drop it.
- `agnostic-ai migrate --only capabilities` rewrites each Claude Code rule that has a capability of its own. An adjacent `WebFetch, WebSearch` pair becomes `web`. The rest stay as aliases, and sync writes the same files.

A bare capability under `allow` or `ask` covers the whole tool. Before permission capabilities existed, these lowercase rules matched no tool and did nothing. `lint` warns (LINT038) and `sync` prints the same note once per run, naming the permissions on enabled tools that take the rule. Scoped rules, `deny` rules, and tool-specific overrides that replace the rule get no warning.

Use `shell(git status)`, `read(src/**)`, `edit(src/**)`, or `mcp:github/get_issue` to limit the rule. `edit(<path>)` also covers writes and edits. `web` has no scoped form. To limit web access, use the tool's own permission fields, or the `WebFetch(domain:example.com)` alias to allow fetches from one domain.

Set `on-unsupported: silent` or pass `sync --quiet` to hide this sync note. `lint` still reports LINT038, and `lint --strict` fails on it. Global permission lists have no native allow or ask rule and raise no LINT038.

A Claude Code rule is a bare tool name (the whole tool) or `Scope(argument)`. An MCP tool is `mcp__<server>__<tool>`. `Scope()` with an empty argument is dropped, not read as the bare tool, which would widen it.

Keep a `shell` wildcard at the end of an `allow` or `deny` rule.

- `shell(git * main)` also approves options placed at the `*`.
- Claude Code matches a mid-command `*` in a `deny` rule literally, so it blocks nothing.

`agnostic-ai lint` reports both as LINT009.

`explain settings/<name>.yaml` lists the rules each configured tool gets and any extra access they grant. `on-unsupported: error` fails when a rule widens, including a Codex exact shell allow that becomes a command prefix.

## Merging

Several files merge. Permission lists are joined in source order with duplicates removed. The last `model` and `effort` that is set wins. Each target resolves its own map entry first, so `model: {codex: gpt-6-luna}` in a later file changes only Codex.

Removing a rule from a spec removes it from Claude Code's `settings.json` on the next sync. Rules you wrote there by hand stay ([Claude settings](@/docs/targets/claude.md#claude-settings)).

`sync --global` also reads settings specs from your home folder, for `model`, `effort`, tool-specific keys, and `permissions.default-mode`. See [default model and effort](@/docs/configuration.md#global-default-model-and-effort).

## Effort by target

`effort` reaches four tools, each under its own key. A value the tool does not accept is not written and raises a coverage note. The other tools still get their files. `x-<target>` wins, so `x-claude.effortLevel` overrides the portable value. Every other tool reports a coverage note.

`import` fills `effort` from these keys when the tool accepts the value and no other settings spec sets it. Otherwise the value goes under `x-<target>`.

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
| Cursor | `allow` and `deny` in `.cursor/cli.json`, for the Cursor CLI ([details](@/docs/targets/cursor.md#permissions)) | no |
| Copilot, Junie, Gemini | no | yes |

Every other target takes neither. A field a tool cannot hold raises a coverage note, and the other tools still get their files.

- Copilot and Junie report the whole policy in a note.
- Codex does too, unless `outputs.codex.exec-policies-from-permissions: true` turns simple Bash rules into exec policies. The rest raise a note ([Bash permission translation](@/docs/targets/codex.md#translate-bash-permissions)).
- Rules carry over only as far as the vendor allows. Augment gates `read`, `edit`, and `write` as whole tools. Factory's command lists take shell patterns. A path-scoped rule raises a note instead of widening.
- Factory's `commandDenylist` prompts, so `ask` goes there and `deny` goes to `commandBlocklist`.
- Review an imported `model` before you enable more tools, because model names differ between vendors.
- A shared Claude model name raises the same coverage note as an [agent `model`](@/docs/spec-format/agents.md#per-target-model-and-effort) on Codex, Gemini, OpenCode, Kilo Code, and Factory. Codex skips it when `outputs.codex.config.model` or the [captured overlay](@/docs/targets/codex.md#codex-config) sets the model.

## Target-specific keys

Tool-specific keys go under `x-<target>` and merge into that tool's settings file (`x-factory.sandbox` reaches `.factory/settings.json`). Codex is the exception. Its `.codex/config.toml` comes from the captured overlay plus `outputs.codex.config`, so an `x-codex` block raises a coverage note that names both.

When agnostic-ai also writes the same key:

- Lists are joined, with the entries sync writes first.
- Objects merge key by key, at every level.
- A single value such as `model` is replaced.
- A list and a string cannot merge. The `x-<target>` value wins, with a coverage note.
- Maps of whole records (`x-qoder.mcpServers`, `x-augment.mcpServers`) merge by name. A server both sides name comes whole from the `x-<target>` block (#974).
- Blocks with a fixed order (`x-qoder.hooks`, `x-augment.hooks`) keep their order, with your own events added at the end (#976).

Four keys take one type each: `x-augment.toolPermissions` a list, and `x-windsurf.permissions`, `x-kilo.permission`, and `x-opencode.permission` an object. Any other type is skipped, the rules from your `permissions` are still written, and a coverage note names the key (#976).

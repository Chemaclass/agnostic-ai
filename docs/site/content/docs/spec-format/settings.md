+++
title = "Settings, reviews, environments, and ignore"
description = "Settings, review, environment, and ignore specs."
weight = 50

[extra]
group = "Reference"
+++

# Settings, reviews, environments, and ignore

## Settings

Pure YAML, one file per settings group. Agent permissions and the default model live here instead of in each tool's settings file.

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

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `permissions.allow` | no | empty | Rules approved without prompting. |
| `permissions.deny` | no | empty | Rules always blocked. |
| `permissions.ask` | no | empty | Rules that prompt before running. |
| `permissions.default-mode` | no | unset | Claude Code starting mode for `sync --global`: `default`, `manual`, `acceptEdits`, `plan`, `auto`, `dontAsk`, or `bypassPermissions`. Other targets raise a coverage note. |
| `model` | no | empty | Default model: a string, or a map per target with an optional `default`, like [agent `model`](@/docs/spec-format/agent-specs.md#per-target-model-and-effort). A target with no entry and no `default` gets no model. |
| `effort` | no | empty | Default reasoning effort: a scalar, or a map per target with an optional `default`, like [agent `effort`](@/docs/spec-format/agent-specs.md#per-target-model-and-effort). Separate from an agent's own `effort`. |

A rule is a bare tool name (whole tool) or `Scope(argument)`. An MCP tool is `mcp__<server>__<tool>`. `Scope()` with an empty argument is dropped, not read as the bare tool, which would widen it.

Keep a `Bash` wildcard at the end of an `allow` or `deny` rule. `Bash(git * main)` also approves options inserted at the `*`, and Claude Code matches a mid-command `*` in a `deny` rule literally, so it blocks nothing. `agnostic-ai lint` reports both as LINT009.

Multiple files merge: permission lists concatenate, de-duplicated in source order, and the last non-empty `model` and `effort` win. Each target resolves its own map entry first, so `model: {codex: gpt-6-luna}` in a later file changes only Codex.

`sync --global` also reads settings specs from the home, for `model`, `effort`, target-specific keys, and `permissions.default-mode`; see [default model and effort](@/docs/configuration.md#global-default-model-and-effort).

`effort` reaches four targets, each under its own key. A value the target does not accept is not written and raises a coverage note; the other targets still emit. `x-<target>` wins, so `x-claude.effortLevel` overrides the portable value. Every other target reports a coverage note.

`import` fills `effort` from these keys when the target accepts the value and no other settings spec sets it; otherwise the value lands under `x-<target>`.

| Target | Native key | Accepted values |
|---|---|---|
| Claude Code | `effortLevel` in `.claude/settings.json` | `low`, `medium`, `high`, `xhigh` |
| Copilot | `effortLevel` in `.github/copilot/settings.json` | `low`, `medium`, `high`, `xhigh` |
| Codex | `model_reasoning_effort` in `.codex/config.toml` | any string; `outputs.codex.config.model-reasoning-effort` and the captured overlay win |
| Factory | `reasoningEffort` in `.factory/settings.json` | `none`, `dynamic`, `off`, `minimal`, `low`, `medium`, `high`, `xhigh`, `max`; each model accepts a subset |

| Target | `permissions` | `model` |
|---|---|---|
| Claude Code, Qoder, Kilo Code, OpenCode | yes | yes |
| Factory | `Bash` rules only | yes |
| Windsurf | yes | no |
| Augment | `allow` and `deny` only | no |
| Codex, Copilot, Junie, Gemini | no | yes |

Every other target takes neither. A field a target cannot represent produces a coverage note while the others still emit. Copilot, Junie, and Codex report the whole policy (Codex's note points at `outputs.codex.exec-policies`). Rules translate only as far as the vendor allows: Augment gates `read`, `edit`, and `write` as whole tools, and Factory's command lists take shell patterns, so a path-scoped rule raises a note instead of widening. Factory's `commandDenylist` prompts, so portable `ask` goes there and `deny` goes to `commandBlocklist`. Review an imported `model` before enabling more targets, since identifiers differ between vendors.

Target-specific keys go under `x-<target>` and merge into that target's settings file (`x-factory.sandbox` reaches `.factory/settings.json`). Codex is the exception: its `.codex/config.toml` comes from the captured overlay plus `outputs.codex.config`, so an `x-codex` block raises a coverage note naming both routes.

On a key this tool also writes, lists union (translated entries first), objects merge recursively, and a scalar such as `model` is replaced. A list against a string cannot merge: the `x-<target>` value wins with a coverage note. Maps of whole records (`x-qoder.mcpServers`, `x-augment.mcpServers`) merge by name, and a server both sides name comes from the `x-<target>` block entire (#974). Fixed-order blocks (`x-qoder.hooks`, `x-augment.hooks`) keep their order, with your own events appended (#976).

Four keys take one shape each: `x-augment.toolPermissions` a list; `x-windsurf.permissions`, `x-kilo.permission`, `x-opencode.permission` an object. Another shape is skipped, the translated rules ship, and a coverage note names the key (#976).

## Reviews

Markdown with optional YAML frontmatter, one file per group of code-review-bot guidance.

```markdown
---
scope: backend
---

Flag any handler that talks to the database directly instead of going through a repository.
```

Reviews honor `scope` and the source layout like rules do. Specs with the same scope concatenate into one review file, written as a plain body without frontmatter. [Cursor](@/docs/targets/cursor.md) (Bugbot), [Codex](@/docs/targets/codex.md) (code review), and [Goose](@/docs/targets/goose.md) support reviews; other targets report them as unsupported. For Codex the text lands in a `## Code Review Rules` section of the root or scoped `AGENTS.md`, which every `AGENTS.md` reader loads; a `targets:` filter that omits `codex` keeps a spec out of it.

A line holding only `@path` includes that file, read from the project root, so a folder's `README.md` can serve as its review without a symlink or a copy:

```markdown
---
scope: apps/engine/src/integrations/create-candidate
target: cursor
---

@apps/engine/src/integrations/create-candidate/README.md
```

Sync writes the file's text where the line stood, so a change to the README shows up in `sync --check`. A missing file, an absolute path, or a path that leaves the project fails the load ([AAI-001](@/docs/errors.md#aai-001-spec-parse-failed)). A line inside a fenced code block stays as written, and an included file is not searched for further includes. Only reviews take `@path` lines; on `AGNOSTIC_AI.md` the [`resolve-imports`](@/docs/configuration.md#syncresolve-imports) setting governs them.

## Environments

Pure YAML, one file per environment group. It describes how a coding agent boots the dev environment: install dependencies, start services, forward ports, open terminals.

```yaml
install: go mod download
terminals:
  - name: dev
    command: go run ./cmd/agnostic-ai
```

`setup` holds the commands a tool runs in a new worktree (one command or a list); `setup-windows` replaces it on Windows.

`dev-commands` lists dev servers a tool can start and preview. Each entry needs a unique `name` and a `command` (a string or a list of words); `cwd` (relative to the project root), `port`, `auto-port`, `env`, and `url` are optional. A string command with shell syntax (pipe, several lines, `VAR=value`, a builtin like `cd`) runs through `sh -c`, which on Windows needs a POSIX shell such as Git Bash on the `PATH`. A list runs with no shell. `env` values are written as text.

```yaml
name: dev
setup: bash scripts/setup-worktree.bash
dev-commands:
  - name: Docs
    command: [pnpm, dev:mintlify]
    cwd: apps/docs
    port: 3000
    auto-port: true
```

Specs merge by top-level key, and the last value wins.

- [Cursor](@/docs/targets/cursor.md): `setup` and `setup-windows` go to `.cursor/worktrees.json`. The rest goes to `environment.json`, except the routing fields (`name`, `scope`, `target(s)`, `target(s)-exclude`, `description`) and `dev-commands`, which get a no-effect note.
- [Claude Code](@/docs/targets/claude.md): `dev-commands` goes to `.claude/launch.json`; every other field gets a no-effect note. Run worktree setup from a `WorktreeCreate` or `SessionStart` [hook](@/docs/spec-format/hooks.md#hooks) instead.
- [OpenHands](@/docs/targets/openhands.md) and [Amp](@/docs/targets/amp.md): `install` becomes a setup script. Amp also turns `terminals` into services. Both note `setup` and `dev-commands` as having no effect.
- Other targets report the spec as unsupported.

`lint` reports a dev command with no `name` or `command`, a repeated name, an unknown key, or a wrong-typed value (LINT016).

## Ignore

Markdown with optional YAML frontmatter, one file per group. The body holds gitignore-syntax patterns for what an agent must not read or index.

````markdown
Secrets and build artifacts the agent should never read.

```gitignore
*.env
secrets/
dist/
```
````

With fenced code blocks, only the lines inside them are patterns and the surrounding text is prose, which formatters like Prettier can rewrite safely. A body without a fence is read whole. Prettier strips trailing spaces inside a block: write a name ending in a space as `name[ ]`.

Specs concatenate into each target's native ignore file under a `#` provenance header, with a blank line between them. Order and whitespace are kept; CRLF becomes LF. Override the path with `outputs.<target>.ignore-file`. Targets without an ignore file report the spec as unsupported.

### Overwrite behaviour

Sync replaces an ignore file with no agnostic-ai provenance header only when every existing exclusion survives, unchanged and in order. Extra patterns are allowed. Missing or reordered patterns, added negations (`!pattern`), and changed whitespace fail with `AAI-103` and leave the file untouched. The check is conservative, so an equivalent rewrite can still fail.

Run `agnostic-ai import <target>` to copy the patterns into `ignore/<target>.md` as a fenced block, with comments, order, and whitespace intact and a leading UTF-8 byte-order mark dropped. The spec sets `target: <target>`; remove the line to share the patterns with every ignore-capable target. Review the combined order if other specs add negations or reorder patterns.

Comment and blank lines exclude nothing. `#` starts a comment only at the start of a line. `outputs.<target>.provenance-header: false` removes the marker and disables this check. Dry-run skips the check; `sync --check` still reports unsafe overwrites.


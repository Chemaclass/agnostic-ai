+++
title = "Claude Code"
description = "Share project rules and skills with Claude Code, then check the native files it loads."
weight = 10

[extra]
group = "Reference"
target_id = "claude"

[extra.moved]
config-keys = "@/docs/targets/claude-reference.md"
agent-memory = "@/docs/targets/claude-reference.md"
claude-settings = "@/docs/targets/claude-reference.md"
import = "@/docs/targets/claude-reference.md"
set-up-this-tool = "@/docs/targets/claude.md#first-rule"
+++

# Claude Code (`claude`)

## Set up this tool {#set-up-this-tool .legacy-anchor}

## Config keys {#config-keys .legacy-anchor}

## Agent memory {#agent-memory .legacy-anchor}

## Claude settings {#claude-settings .legacy-anchor}

## Import {#import .legacy-anchor}


Write a project rule once under `.agnostic-ai/`. Sync gives Claude Code its native `.claude/rules/` file and gives other selected tools their own format. Skills, agents, hooks, MCP servers, and settings also have native outputs.

## First rule

Install the client using the [official setup guide](https://code.claude.com/docs/en/quickstart), then open your project in it.

Select `claude` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule). The shared commit-prefix rule becomes `.claude/rules/conventional-commits.md`. If you also select Codex, the same source becomes a section in `AGENTS.md`.

Write shared project instructions in `.agnostic-ai/AGNOSTIC_AI.md`. Sync writes Claude's `CLAUDE.md` from that source. Edit the source file for later changes.

Already have Claude files? Check [import coverage](@/docs/targets/claude-reference.md#import) before syncing. Import reads native rules, agents, skill folders, commands, hooks, permissions, MCP servers, and a settings overlay. It reports files it cannot import.

## Output

| Source | Default output |
|---|---|
| Shared instructions | `CLAUDE.md` |
| Rules | `.claude/rules/<name>.md` |
| Skills and bundled files | `.claude/skills/<name>/SKILL.md` |
| Agents | `.claude/agents/<name>.md` |
| Commands | `.claude/commands/<name>.md` |
| Hooks and settings | `.claude/settings.json`, when configured |
| MCP servers | `.mcp.json`, when configured |
| Local preview commands | `.claude/launch.json`, when configured |

An empty kind creates no corresponding files. For output overrides and field mappings, use the [native reference](@/docs/targets/claude-reference.md).

## Loading and limits

- Claude Code [discovers `.claude/rules/` recursively at launch](https://code.claude.com/docs/en/memory). Sync translates file patterns into native `paths` conditions. Start a new session after changing the setup, and check the rule is not excluded by native settings.
- When another selected tool writes identical shared instructions to `AGENTS.md`, `CLAUDE.md` can import it with `@AGENTS.md`. Starting in a subfolder can require import approval; non-interactive `claude -p` skips that outside import. See [instruction loading](@/docs/targets/claude-reference.md#rules-and-instruction-loading).
- Claude Code lists every agent description at startup and warns when the total passes 15,000 tokens, but still loads every agent. `lint` estimates that total; see [context budgets](@/docs/cli-reference/lint-codes.md#context-budgets).
- Explore and Plan start without your `CLAUDE.md` files. A rule they must follow, such as "ignore the vendor/ directory", has to be restated in the prompt Claude gives them when it delegates. A `CLAUDE.md` in a subdirectory still loads when they read files there.
- An [agent-only MCP server](@/docs/spec-format/mcps.md#agent-only) is written into the agent file, not `.mcp.json`. Claude Code loads it only after you trust the project folder in an interactive session. A parent folder's trust does not count, and neither does the trust a `claude -p` or SDK session gets. Until then the agent runs without the server ([Claude Code docs](https://code.claude.com/docs/en/sub-agents#inline-server-trust)).
- A read-only agent blocks Claude's edit tools but still allows Bash. Hooks and permissions also differ across tools; check the native mapping before treating shared text as an enforced restriction.

With `builtins: [memory]`, shared memory adds to [Claude Code's own memory](@/docs/memory.md#claude-code-s-own-memory). For home instructions and user settings, see [global configuration](@/docs/global-configuration.md).

## Protected paths

A settings spec's `protected` block becomes `Edit(/<path>)` rules in `.claude/settings.json`, under `ask` or `deny`. The leading `/` anchors them at the project root. Removing a path removes its generated rule on the next sync.

A deny also covers file commands Claude recognizes in Bash, such as `sed` and redirections, but cannot block an arbitrary script that writes files. An ask guards edit tools only. See [protected paths](@/docs/spec-format/settings.md#protected-paths).

## Verify

Run `agnostic-ai sync --check` and inspect `.claude/rules/conventional-commits.md`. Start `claude` from the project root. Ask for a commit subject for a small fix; it should use `fix:`.

To check what Claude Code loaded from `.claude/agents/`:

- `claude plugin validate .claude/agents` lists agent files whose frontmatter does not parse. It needs Claude Code 2.1.233 or later, checks only the directory you name, and does not flag a file that parses but has no `name`.
- `claude --debug` writes a log to `~/.claude/debug/<session-id>.txt`. Search it for `does not recognize or apply` to find misspelled agent fields. It needs Claude Code 2.1.296 or later.

For a file-specific rule, run `agnostic-ai explain --file src/main.go --target claude` to inspect the source and planned output. Confirm native loading with both a matching and an unrelated file in a new session.

## Run Explore on a cheaper model

A project agent named `Explore` replaces Claude Code's built-in Explore and keeps its own `model`. Write one that only Claude Code gets:

```markdown
---
name: Explore
target: claude
description: Finds files and code for broad lookups. Use proactively before reading many files.
model: haiku
can: [read, Grep, Glob]
---

Search only. Return file paths with one line each.
```

Save it as `.agnostic-ai/agents/explore.md`. `target: claude` keeps the agent away from Junie, which rejects a name that starts with a capital letter. The `description` is required: Claude Code skips an agent file that has none.

Next, [add a reusable skill](@/docs/spec-format/skills.md) or compare the same setup in [Codex](@/docs/targets/codex.md).

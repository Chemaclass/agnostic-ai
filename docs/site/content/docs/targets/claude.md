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


Write a rule once under `.agnostic-ai/`. Sync writes `.claude/rules/` for Claude Code and each other selected tool's own format. Skills, agents, hooks, MCP servers, and settings get native files too. Using this provider through Composer? See the [Composer setup guide](@/docs/composer.md).

## First rule

1. Follow the [official setup guide](https://code.claude.com/docs/en/quickstart).
2. Select `claude` in `agnostic-ai.yaml`.
3. Follow [First rule](@/docs/getting-started.md#first-rule). The commit-prefix rule becomes `.claude/rules/conventional-commits.md`. With Codex also selected, it becomes a section in `AGENTS.md` too.

Write shared instructions in `.agnostic-ai/AGNOSTIC_AI.md`; sync writes `CLAUDE.md` from it. Already have Claude files? Read [import coverage](@/docs/targets/claude-reference.md#import) first. Import lists what it cannot read.

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

An empty kind writes no files. Overrides and field mappings are in the [native reference](@/docs/targets/claude-reference.md).

## Verify

1. Run `agnostic-ai sync --check`.
2. Open `.claude/rules/conventional-commits.md`.
3. Start `claude` in the project root and ask for a commit subject for a small fix. It should use `fix:`.

For a file-specific rule, run `agnostic-ai explain --file src/main.go --target claude`. Then confirm in a new session with a matching file and an unrelated one.

{% <details summary="Check which agents loaded"> %}
- `claude plugin validate .claude/agents` lists agent files whose frontmatter does not parse.
- It needs Claude Code 2.1.233 or later and checks only the directory you name.
- It does not flag a file that parses but has no `name`.
- `claude --debug` logs to `~/.claude/debug/<session-id>.txt`. Search it for `does not recognize or apply` to find misspelled agent fields.
- That log needs Claude Code 2.1.296 or later.
{% </details> %}

## Loading and limits

- Claude Code [finds `.claude/rules/` recursively at launch](https://code.claude.com/docs/en/memory). Start a new session after a change, and check native settings do not exclude the rule.
- Sync turns file patterns into native `paths` conditions.
- When another tool writes the same shared text to `AGENTS.md`, `CLAUDE.md` imports it with `@AGENTS.md`. See [instruction loading](@/docs/targets/claude-reference.md#rules-and-instruction-loading).
- A read-only agent blocks edit tools but still allows Bash. Hooks and permissions differ across tools, so check the native mapping before relying on a restriction.
- Claude Code warns when agent descriptions pass 15,000 tokens, but loads every agent. `lint` estimates the total: [context budgets](@/docs/cli-reference/lint-codes.md#context-budgets).

{% <details summary="More limits"> %}
- **Subfolder start:** the `@AGENTS.md` import needs approval in an interactive session. `claude -p` skips it.
- **Explore and Plan:** they start without your `CLAUDE.md` files. Restate a rule they must follow, such as "ignore vendor/", in the delegation prompt. A `CLAUDE.md` in a subdirectory still loads when they read files there.
- **Agent-only MCP server:** the [server](@/docs/spec-format/mcps.md#agent-only) goes into the agent file, not `.mcp.json`. Claude Code loads it only after you trust the project folder in an interactive session.
- A parent folder's trust does not count. Neither does trust from `claude -p` or an SDK session. Until then the agent runs without the server ([docs](https://code.claude.com/docs/en/sub-agents#inline-server-trust)).
- **Commands run on their own:** Claude can run a `.claude/commands/` command without you typing it. Add `disable-model-invocation: true` to the command spec's frontmatter to stop that ([docs](https://code.claude.com/docs/en/skills)).
- **Memory:** with `builtins: [memory]`, shared memory adds to [Claude Code's own memory](@/docs/memory.md#claude-code-s-own-memory).
- **Home files:** see [user configuration](@/docs/global-configuration.md).
- **Voice:** a home `voice` setting writes `voice.enabled` and `voice.mode` to `~/.claude/settings.json`. `voice.language` is not written, because Claude takes its dictation language from its response language. See [voice dictation](@/docs/spec-format/settings-reference.md#voice-dictation).
{% </details> %}

## Protected paths

A settings spec's `protected` block becomes `Edit(/<path>)` rules in `.claude/settings.json`, under `ask` or `deny`. The `/` anchors them at the project root. Removing a path removes its rule on the next sync.

- A deny also covers file commands Claude recognizes in Bash, such as `sed` and redirections. It cannot block an arbitrary script that writes files.
- An ask guards edit tools only.

See [protected paths](@/docs/spec-format/settings.md#protected-paths).

## Run Explore on a cheaper model

A project agent named `Explore` replaces the built-in one and keeps its own `model`. Save this as `.agnostic-ai/agents/explore.md`:

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

- `target: claude` keeps the agent away from Junie, which rejects names starting with a capital letter.
- `description` is required: Claude Code skips an agent file without one.

Next, [add a reusable skill](@/docs/spec-format/skills.md) or compare [Codex](@/docs/targets/codex.md).

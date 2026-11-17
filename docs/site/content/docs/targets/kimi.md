+++
title = "Kimi Code CLI"
description = "Sync and import Kimi Code instructions, agents, skills, MCP connections, and selected user configuration."
weight = 280

[extra]
group = "Reference"
target_id = "kimi"

[extra.moved]
set-up-this-tool = "@/docs/targets/kimi.md#first-rule"
native-paths = "@/docs/targets/kimi.md#output"
output-overrides = "@/docs/targets/kimi-reference.md"
agents-and-skills = "@/docs/targets/kimi-reference.md"
mcp-connections = "@/docs/targets/kimi-reference.md"
selected-profile = "@/docs/targets/kimi-reference.md"
import-and-ownership = "@/docs/targets/kimi-reference.md"
sources = "@/docs/targets/kimi-reference.md"
+++

# Kimi Code CLI (`kimi`)

## Set up this tool {#set-up-this-tool .legacy-anchor}

## Native paths {#native-paths .legacy-anchor}

## Output overrides {#output-overrides .legacy-anchor}

## Agents and skills {#agents-and-skills .legacy-anchor}

## MCP connections {#mcp-connections .legacy-anchor}

## Selected settings and hooks {#selected-profile .legacy-anchor}

## Import and ownership {#import-and-ownership .legacy-anchor}

## Sources {#sources .legacy-anchor}

Sync generates native files for [Kimi Code CLI 2.5.1](https://github.com/MoonshotAI/kimi-code/releases/tag/%40moonshot-ai/kimi-code%402.5.1), the maintained `@moonshot-ai/kimi-code` package. Select `kimi` explicitly: it is not a default target, because its native agent tool lists cannot express scoped shell restrictions such as `Bash(git diff *)`.

## First rule

Select `kimi` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule). The shared commit-prefix rule becomes `.kimi-code/AGENTS.md`.

{% <details summary="Quick start with a rule and an agent"> %}

```sh
mkdir -p .agnostic-ai/rules .agnostic-ai/agents
cat > agnostic-ai.yaml <<'YAML'
version: 1
targets: [kimi]
YAML
cat > .agnostic-ai/rules/changes.md <<'MD'
---
name: changes
---
Keep changes small and explain failures.
MD
cat > .agnostic-ai/agents/reviewer.md <<'MD'
---
name: reviewer
description: Review a patch
can: [read]
---
Read the patch and report defects.
MD
agnostic-ai sync --dry-run
agnostic-ai sync
agnostic-ai sync --check
```

{% </details> %}

Already have native files? Run `agnostic-ai import kimi` before syncing; see [import and ownership](@/docs/targets/kimi-reference.md#import-and-ownership).

## Output

| Source | Project output | User output with `sync --global --only kimi` |
|---|---|---|
| Rules | `.kimi-code/AGENTS.md`, scoped `<scope>/.kimi-code/AGENTS.md` | `~/.kimi-code/AGENTS.md` |
| Agents | `.kimi-code/agents/<name>.md` | `~/.kimi-code/agents/<name>.md` |
| Skills and resources | `.kimi-code/skills/<name>/SKILL.md` | `~/.kimi-code/skills/<name>/SKILL.md` |
| MCP servers | `.kimi-code/mcp.json` | `~/.kimi-code/mcp.json` |
| Hooks | Explicit profile export only | `~/.kimi-code/config.toml`, `[[hooks]]` |
| Settings | Explicit profile export only | Owned scalar keys in `~/.kimi-code/config.toml` |

`KIMI_CODE_HOME` relocates all user paths together. Sync never changes the older `~/.kimi` data directory or runs Kimi's migration. Field mappings, [output overrides](@/docs/targets/kimi-reference.md#output-overrides), and the profile export are in the [Kimi native reference](@/docs/targets/kimi-reference.md).

## Verify

Run `agnostic-ai sync --check` and inspect `.kimi-code/AGENTS.md` for the commit-prefix rule. When a configured peer already writes the plain `AGENTS.md`, Kimi shares that file instead. Start Kimi from the project root and ask for a commit subject for a small fix; it should use `fix:`.

## Loading and limits

- Run Kimi from your synced Git project root. Its own workspace trust choice controls project MCP loading.
- Kimi loads instructions from the Git root through the working directory. Kimi-specific and ordinary `AGENTS.md` instructions are additive.
- Agents and skills are discovered from the nearest Git root, including linked Git worktrees.
- Kimi's branded MCP file is relative to the session working directory. When launching from a subdirectory, explicitly choose a repository-root MCP mirror (`root-mcp-file`) if required.

{% <details summary="More limits"> %}

- The uppercase plain filename wins over `agents.md` in one directory. Kimi-only projects use the branded path. When a configured peer already emits the plain `AGENTS.md`, Kimi shares that file and its scoped instruction files so common text is delivered once. Target-specific rules must be compatible with every reader of a shared file. See [shared entry points](@/docs/configuration.md#entry-point-files) for instruction content and path overrides.
- Kimi merges the user MCP file, repository-root `.mcp.json`, then working-directory `.kimi-code/mcp.json`; later same-name entries replace earlier entries. The adapter writes only its branded project file by default.
- Overrides change generated paths. Kimi must be configured to discover a relocated file. Skills remain at project root; nested skill scopes are refused.
- Agent `model`, `effort`, and `maxTurns` have no effect in this Kimi release and raise coverage notes. Portable `readonly` has no direct mapping.
- Scoped tool rules such as `shell(git diff *)` cannot be represented by Kimi's allowlist and stop sync.
- Standalone command specs are unsupported; skills run by hand as `/skill:<name>`.

{% </details> %}

## Protected paths

Not enforced. Sync reports portable protections as unsupported. Native permission rules in an explicit profile constrain individual Kimi tools and do not provide a general filesystem boundary for Bash or MCP tools.

Next, see the [Kimi native reference](@/docs/targets/kimi-reference.md) for agents, skills, MCP, hooks, and settings.

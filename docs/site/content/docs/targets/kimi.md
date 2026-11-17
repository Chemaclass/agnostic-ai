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

Sync generates native files for [Kimi Code CLI 2.5.1](https://github.com/MoonshotAI/kimi-code/releases/tag/%40moonshot-ai/kimi-code%402.5.1), the maintained `@moonshot-ai/kimi-code` package.

Select `kimi` explicitly. It is not a default target, because its native agent tool lists cannot express scoped shell restrictions such as `Bash(git diff *)`.

## First rule

Select `kimi` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule). The commit-prefix rule becomes `.kimi-code/AGENTS.md`.

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

Already have native files? Run `agnostic-ai import kimi` first. See [import and ownership](@/docs/targets/kimi-reference.md#import-and-ownership).

## Output

| Source | Project output | User output (`sync --global --only kimi`) |
|---|---|---|
| Rules | `.kimi-code/AGENTS.md`, scoped `<scope>/.kimi-code/AGENTS.md` | `~/.kimi-code/AGENTS.md` |
| Agents | `.kimi-code/agents/<name>.md` | `~/.kimi-code/agents/<name>.md` |
| Skills and resources | `.kimi-code/skills/<name>/SKILL.md` | `~/.kimi-code/skills/<name>/SKILL.md` |
| MCP servers | `.kimi-code/mcp.json` | `~/.kimi-code/mcp.json` |
| Hooks | Explicit profile export only | `~/.kimi-code/config.toml`, `[[hooks]]` |
| Settings | Explicit profile export only | Owned scalar keys in `~/.kimi-code/config.toml` |

`KIMI_CODE_HOME` moves all user paths together. Sync never touches the older `~/.kimi` data directory or runs Kimi's migration. Mappings, [output overrides](@/docs/targets/kimi-reference.md#output-overrides), and the profile export: [native reference](@/docs/targets/kimi-reference.md).

## Verify

1. Run `agnostic-ai sync --check`.
2. Check `.kimi-code/AGENTS.md` for the commit-prefix rule. If a configured peer already writes plain `AGENTS.md`, Kimi shares that file.
3. Start Kimi from the project root and ask for a commit subject for a small fix. It should use `fix:`.

## Loading and limits

- Run Kimi from your synced Git project root. Kimi's workspace trust choice controls project MCP loading.
- Instructions load from the Git root down to the working directory. Kimi-specific and plain `AGENTS.md` instructions add together.
- Agents and skills come from the nearest Git root, including linked worktrees.
- Kimi's MCP file is relative to the session working directory. Launching from a subdirectory? Set `root-mcp-file` for a repository-root mirror.

{% <details summary="More limits: shared files, MCP merge, agent fields, commands"> %}
- The uppercase `AGENTS.md` wins over `agents.md` in one directory.
- Kimi-only projects use the branded path.
- When a peer already writes plain `AGENTS.md`, Kimi shares that file and its scoped files. Rules must suit every reader. See [shared entry points](@/docs/configuration.md#entry-point-files).
- Kimi merges the user MCP file, repository-root `.mcp.json`, then working-directory `.kimi-code/mcp.json`. Later same-name entries win. Sync writes only the branded project file by default.
- Overrides change generated paths. Configure Kimi to find a relocated file. Skills stay at project root, and nested skill scopes are refused.
- Agent `model`, `effort`, and `maxTurns` have no effect in this release and raise coverage notes. Portable `readonly` has no mapping.
- Scoped tool rules such as `shell(git diff *)` stop sync.
- Command specs are unsupported. Run skills by hand as `/skill:<name>`.
{% </details> %}

## Protected paths

Not enforced. Sync reports portable protections as unsupported. Native permission rules in an explicit profile constrain individual Kimi tools and do not provide a general filesystem boundary for Bash or MCP tools.

Next, see the [Kimi native reference](@/docs/targets/kimi-reference.md) for agents, skills, MCP, hooks, and settings.

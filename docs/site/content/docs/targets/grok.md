+++
title = "Grok Build"
description = "Set up Grok Build with readable native rules, agents, skills, commands, hooks, and MCP servers."
weight = 265

[extra]
group = "Reference"
target_id = "grok"

[extra.moved]
native-mappings = "@/docs/targets/grok-reference.md"
import = "@/docs/targets/grok-reference.md"
global-configuration = "@/docs/targets/grok-reference.md"
+++

# Grok Build (`grok`)

## Native mappings {#native-mappings .legacy-anchor}

## Import {#import .legacy-anchor}

## Global configuration {#global-configuration .legacy-anchor}

Select Grok explicitly and keep its project instructions visible to Git. Grok skips ignored rule files, so it is excluded from the default targets.

## First rule

Install and open the CLI using the [official setup guide](https://docs.x.ai/build/overview). Grant project trust through Grok; sync does not grant trust or run Grok.

Create `agnostic-ai.yaml`:

```yaml
version: 1
targets: [grok]
gitignore:
  enabled: true
  commit: [grok:instructions]
```

`agnostic-ai init` writes the `commit` line when it selects Grok with the managed `.gitignore` on. Without it, `lint` reports [LINT053](@/docs/cli-reference/lint-codes.md) and sync stops for every tool.

Create `.agnostic-ai/rules/style.md`:

```markdown
---
name: style
---

Use Conventional Commits for commit subjects.
```

Run `agnostic-ai sync`, then `agnostic-ai sync --check`. Already have native files? Run [`agnostic-ai import grok`](@/docs/targets/grok-reference.md#import) first.

## Output

| Spec | Native path |
|---|---|
| Rule | `.grok/rules/<name>.md` |
| Directory rule | `<scope>/.grok/rules/<name>.md` |
| Agent | `.grok/agents/<name>.md` |
| Skill and resources | `.grok/skills/<name>/SKILL.md` and sibling files |
| Command | `.grok/commands/<name>.md` |
| Command hook | `.grok/hooks/<name>.json` |
| Shared hook scripts | `.grok/hook-scripts/` |
| MCP and permission lists | `.grok/config.toml` |

Scoped skills, agents, and commands use the corresponding directory below their source scope. Output overrides use `outputs.grok.dir`, `rules-dir`, `agents-dir`, `skills-dir`, `commands-dir`, `hooks-dir`, `mcp-file`, and `conf-file`. `conf-file` separates permission settings from the MCP destination. A custom destination must still be a location Grok reads. Rule visibility checks include custom paths. Field mappings, import, and global configuration are in the [Grok native reference](@/docs/targets/grok-reference.md).

## Verify

Read `.grok/rules/style.md` and run `git check-ignore --no-index .grok/rules/style.md`: it should report no matching ignored path. Then open a Grok session and check that it follows the rule.

{% <details summary="If the rule file is ignored"> %}

A root or global ignore pattern can still hide the file. Sync rejects that case before emitting Grok output; remove the matching pattern yourself. Explicitly disabling agnostic-ai's ignore management is also supported when your own ignore policy keeps the rules visible. The check uses Grok's root `.gitignore` and global ignore file, not nested ignore files or `.git/info/exclude`. It conservatively requires parent directories to be re-included before a file below an ignored directory.

{% </details> %}

## Loading and limits

- Rules apply when Grok discovers their directory while walking from its working directory to the repository root. File glob conditions have no native rule equivalent; use a whole directory scope.
- Project hooks run only after Grok's hook trust approval. Hook errors fail open; native explicit denies block according to the event.
- Portable `on`/`match` hooks are not translated in this adapter. Use Grok-native events and its documented reply protocol.
- Empty tool restrictions on agents are rejected, because native empty tools inherit all tools.

{% <details summary="More limits"> %}

- On Windows, shared script paths that need shell-specific quoting, such as paths with spaces, require an explicit native wrapper command. Sync refuses an automatic rewrite it cannot preserve. Existing native invocation commands keep their quoting.
- Project model, effort, permission default mode, and session retention are unsupported. Ignore, review, and environment specs have no native output.
- Global commands and permission lists are not emitted, and global effort is unsupported.
- Grok also reads compatibility files from other enabled tools, so those can contribute additional instructions, hooks, or servers.

{% </details> %}

{% <details summary="Native evidence and runtime limits"> %}

Paths and fields were checked against the [official project rules](https://docs.x.ai/build/features/project-rules), [MCP](https://docs.x.ai/build/features/mcp-servers), [hooks](https://docs.x.ai/build/features/hooks), [subagents](https://docs.x.ai/build/features/subagents), [skills](https://docs.x.ai/build/features/skills-plugins-marketplaces), and [settings](https://docs.x.ai/build/settings) documentation, plus public source snapshot [2bdd1d6a](https://github.com/xai-org/grok-build/tree/2bdd1d6a6369de0e8c68132ea4539e9abd9e14a8). The source snapshot is an independent format reference, not proof of exact binary source parity. Validation uses native-format fixtures and isolated filesystem tests; it does not install, authenticate, approve trust, or run a model session. Grok also reads compatibility files from other enabled tools, so those can contribute additional instructions, hooks, or servers.

{% </details> %}

## Protected paths

Not enforced. Use an instruction rule or explicit permission rules. See [protected-path alternatives](@/docs/spec-format/settings.md#protected-paths).

Next, see the [Grok native reference](@/docs/targets/grok-reference.md) for agents, skills, MCP, hooks, and settings.

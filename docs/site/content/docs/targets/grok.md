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

## User configuration {#global-configuration .legacy-anchor}

Write Grok Build rules, agents, skills, commands, hooks, and MCP servers from your specs. Grok skips Git-ignored rule files, so `grok` is not a default target: select it and keep its instructions visible to Git.

## First rule

1. Install the CLI with the [official setup guide](https://docs.x.ai/build/overview) and grant project trust in Grok. Sync does not do either.
2. Create `agnostic-ai.yaml`:

   ```yaml
   version: 1
   targets: [grok]
   gitignore:
     enabled: true
     commit: [grok:instructions]
   ```

   `agnostic-ai init` writes the `commit` line for you. Without it, `lint` reports [LINT053](@/docs/cli-reference/lint-codes.md) and sync stops for every tool.
3. Create `.agnostic-ai/rules/style.md`:

   ```markdown
   ---
   name: style
   ---

   Use Conventional Commits for commit subjects.
   ```

4. Run `agnostic-ai sync`, then `agnostic-ai sync --check`.

Already have native files? Run [`agnostic-ai import grok`](@/docs/targets/grok-reference.md#import) first.

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

Scoped skills, agents, and commands go in the same directory under their source scope. Field mappings, import, and user configuration: [Grok native reference](@/docs/targets/grok-reference.md).

{% <details summary="Output overrides"> %}
- Keys under `outputs.grok`: `dir`, `rules-dir`, `agents-dir`, `skills-dir`, `commands-dir`, `hooks-dir`, `mcp-file`, `conf-file`.
- `conf-file` moves permission settings away from the MCP destination.
- A custom destination must still be a place Grok reads.
- Rule visibility checks cover custom paths.
{% </details> %}

## Verify

1. Run `git check-ignore --no-index .grok/rules/style.md`. It should report no ignored path.
2. Open a Grok session and check that it follows the rule.

{% <details summary="If the rule file is ignored"> %}
- A root or global ignore pattern can still hide the file. Sync rejects that case before it writes Grok output. Remove the pattern yourself.
- You can also turn off agnostic-ai's ignore management when your own policy keeps the rules visible.
- The check reads Grok's root `.gitignore` and global ignore file. It skips nested ignore files and `.git/info/exclude`.
- A file under an ignored directory counts as hidden unless every parent directory is re-included.
{% </details> %}

## Loading and limits

- Grok loads a rule when it finds the rule's directory on the way from its working directory to the repository root.
- A file glob has no native rule form. Scope a rule to a whole directory.
- Project hooks run only after Grok's hook trust approval.

{% <details summary="More limits"> %}
- Grok also reads compatibility files from other enabled tools. They can add instructions, hooks, or servers.
- Ignore, review, and environment specs have no native output.
- Hooks, agent tools, Windows script paths, and unsupported settings: see the [reference](@/docs/targets/grok-reference.md#hooks).
{% </details> %}

{% <details summary="Native evidence and runtime limits"> %}
- Checked against the official docs for [project rules](https://docs.x.ai/build/features/project-rules), [MCP](https://docs.x.ai/build/features/mcp-servers), [hooks](https://docs.x.ai/build/features/hooks), [subagents](https://docs.x.ai/build/features/subagents), [skills](https://docs.x.ai/build/features/skills-plugins-marketplaces), and [settings](https://docs.x.ai/build/settings).
- Also checked against the public source snapshot [2bdd1d6a](https://github.com/xai-org/grok-build/tree/2bdd1d6a6369de0e8c68132ea4539e9abd9e14a8). It is a format reference, not proof of exact binary parity.
- Tests use native-format fixtures and isolated filesystems. They do not install Grok, sign in, approve trust, or run a model.
{% </details> %}

## Protected paths

Not enforced. Use an instruction rule or explicit permission rules. See [protected-path alternatives](@/docs/spec-format/settings.md#protected-paths).

Next, see the [Grok native reference](@/docs/targets/grok-reference.md) for agents, skills, MCP, hooks, and settings.

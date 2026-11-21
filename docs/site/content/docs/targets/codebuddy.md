+++
title = "CodeBuddy Code"
description = "Generate native Tencent CodeBuddy Code configuration."
weight = 127

[extra]
group = "Reference"
target_id = "codebuddy"

[extra.moved]
native-files = "@/docs/targets/codebuddy.md#output"
import-an-existing-setup = "@/docs/targets/codebuddy-reference.md#import-an-existing-setup"
enable-project-mcp-output = "@/docs/targets/codebuddy-reference.md#enable-project-mcp-output"
fields-and-limits = "@/docs/targets/codebuddy-reference.md#fields-and-limits"
config-keys = "@/docs/targets/codebuddy-reference.md#config-keys"
+++

# CodeBuddy Code (`codebuddy`)

## Fields and limits {#fields-and-limits .legacy-anchor}

Use `codebuddy` for Tencent's CodeBuddy Code CLI. It follows the [official CLI documentation](https://www.codebuddy.ai/docs/cli/installation) and npm package `@tencent-ai/codebuddy-code` 2.179.3.

It is not in the default targets. Select it with `targets: [codebuddy]`, because native agent names reject digits and scoped or empty agent tool lists cannot be represented.

## First rule

```bash
mkdir -p .agnostic-ai/rules
cat > agnostic-ai.yaml <<'YAML'
version: 1
targets: [codebuddy]
YAML
cat > .agnostic-ai/rules/commits.md <<'RULE'
---
name: commits
---
Use Conventional Commits.
RULE
agnostic-ai sync
agnostic-ai sync --check
```

`CODEBUDDY.md` now holds the rule. Edit the source rule and sync again. Starting from existing files? See [import an existing setup](@/docs/targets/codebuddy-reference.md#import-an-existing-setup).

## Output

| Source | Project output | User output (`sync --scope user --only codebuddy`) |
|---|---|---|
| Instructions and always-on rules | `CODEBUDDY.md` (or imported `.codebuddy/CODEBUDDY.md`) | `~/.codebuddy/CODEBUDDY.md` |
| Directory-scoped rules | `<scope>/CODEBUDDY.md` | Not supported |
| Conditional rules | `.codebuddy/rules/<name>.md` | Not supported |
| Agents | `.codebuddy/agents/<name>.md` | `~/.codebuddy/agents/<name>.md` |
| Skills and bundled files | `.codebuddy/skills/<name>/SKILL.md` | `~/.codebuddy/skills/<name>/SKILL.md` |
| Commands | `.codebuddy/commands/<name>.md` | Project-only in agnostic-ai |
| Settings and hooks | `.codebuddy/settings.json` | `~/.codebuddy/settings.json` |
| MCP servers | Root `.mcp.json`, [opt-in](@/docs/targets/codebuddy-reference.md#enable-project-mcp-output) | `~/.codebuddy/.mcp.json` |

Output keys: [config keys](@/docs/targets/codebuddy-reference.md#config-keys).

## Verify

1. Run `agnostic-ai sync --check`.
2. Open `CODEBUDDY.md` and find the rule text.
3. Start CodeBuddy from the project root and ask for a commit subject for a small fix. It should follow the rule.

## Loading and limits

- Agent names use lowercase letters and hyphens.
- An empty `tools` or `can` list fails sync. Set `x-codebuddy.tools: []` for an agent with no tools.
- Sync never approves anything. CodeBuddy asks before it first connects to a project MCP server.
- No output for ignore files, review configuration, environments, plugins, credentials, sessions, or automatic memories.

{% <details summary="More limits: instruction loading, scoped patterns, hook trust"> %}
- CodeBuddy loads ancestor instructions and finds descendant `CODEBUDDY.md` files when it works there.
- Its rules directory loads from the current working directory only.
- Conditional rules need `alwaysApply: false` and `paths`. Paths alone do not make a rule conditional.
- Use portable `scope` for a whole directory. A scoped rule with narrower file filters is reported as unsupported.
- Sync rejects scoped shell and file tool patterns. Keep those policies in permission rules.
- Frontmatter hooks need the user's `allowUntrustedFrontmatterHooks` choice.
- Agent fields, tool mappings, skill and command fields, MCP permission rules, settings keys, and hook rules: [fields and limits](@/docs/targets/codebuddy-reference.md#fields-and-limits).
{% </details> %}

## Protected paths

Not enforced. CodeBuddy's `protected` paths have no registered enforcement mapping in this adapter. Use native permission rules for the operations you want to deny.

## Turn on project MCP output

Root MCP output is off by default. Set the file to write:

```yaml
outputs:
  codebuddy:
    root-mcp-file: .mcp.json
```

Sync writes an explicit `type` for each server and never approves project servers. Field and user-scope details: [project MCP output](@/docs/targets/codebuddy-reference.md#enable-project-mcp-output).

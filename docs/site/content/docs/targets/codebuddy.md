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

Use `codebuddy` for Tencent's CodeBuddy Code CLI. This adapter follows the [official CLI documentation](https://www.codebuddy.ai/docs/cli/installation) and npm package `@tencent-ai/codebuddy-code` 2.179.3.

Select it with `targets: [codebuddy]`. It is excluded from the defaults because native agent names reject digits, and scoped or portable empty agent tool lists cannot be represented.

## First rule

Create one rule, generate its native files, and check them:

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

Open `CODEBUDDY.md` and find the rule text. CodeBuddy loads that file as project instructions. Keep editing the source rule, then sync again. To start from existing files, see [import an existing setup](@/docs/targets/codebuddy-reference.md#import-an-existing-setup).

## Output

| Source | Project output | User output with `sync --global --only codebuddy` |
|---|---|---|
| Instructions and always-on rules | `CODEBUDDY.md` (or imported `.codebuddy/CODEBUDDY.md`) | `~/.codebuddy/CODEBUDDY.md` |
| Directory-scoped rules | `<scope>/CODEBUDDY.md` | Not supported |
| Conditional rules | `.codebuddy/rules/<name>.md` | Not supported |
| Agents | `.codebuddy/agents/<name>.md` | `~/.codebuddy/agents/<name>.md` |
| Skills and bundled files | `.codebuddy/skills/<name>/SKILL.md` | `~/.codebuddy/skills/<name>/SKILL.md` |
| Commands | `.codebuddy/commands/<name>.md` | Project-only in agnostic-ai |
| Settings and hooks | `.codebuddy/settings.json` | `~/.codebuddy/settings.json` |
| MCP servers | Root `.mcp.json`, opt-in | `~/.codebuddy/.mcp.json` |

Output keys are in the [native reference](@/docs/targets/codebuddy-reference.md#config-keys).

## Verify

Run `agnostic-ai sync --check`, then open `CODEBUDDY.md` and find the rule text. Start CodeBuddy from the project root and ask for a commit subject for a small fix; it should follow the rule.

## Loading and limits

- CodeBuddy loads ancestor instructions and discovers descendant `CODEBUDDY.md` files when it works there. Its modular rules directory is loaded from the current working directory only. Conditional rules carry `alwaysApply: false` and `paths`; paths alone do not make a rule conditional. Use portable `scope` for a whole directory. A scoped rule with narrower file filters is reported as unsupported.
- Agent names use lowercase letters and hyphens. A portable empty `tools` or `can` list fails sync; set `x-codebuddy.tools: []` for an agent without tools. Sync rejects scoped shell and file tool patterns, so keep those policies in permission rules.
- Sync never makes trust or approval decisions. CodeBuddy asks the user before the first connection to a project MCP server, and frontmatter hooks need the user's `allowUntrustedFrontmatterHooks` choice.
- Ignore files, review configuration, environments, plugins, credentials, sessions, and automatic memories have no output in this adapter.

{% <details summary="More limits"> %}
Agent fields, tool mappings, skill and command fields, MCP permission rules, settings keys, and hook rules are in [fields and limits](@/docs/targets/codebuddy-reference.md#fields-and-limits).
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

Sync then writes an explicit `type` for each server and never approves project servers. See [project MCP output](@/docs/targets/codebuddy-reference.md#enable-project-mcp-output) for field and user-scope details.

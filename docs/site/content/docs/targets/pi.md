+++
title = "Pi"
description = "Native Pi instructions, skills, prompt commands, MCP servers, and model settings."
weight = 34

[extra]
group = "Reference"
target_id = "pi"

[extra.moved]
native-mappings = "@/docs/targets/pi-reference.md"
global-configuration = "@/docs/targets/pi-reference.md"
config-keys = "@/docs/targets/pi-reference.md"
limits-and-trust = "@/docs/targets/pi.md#loading-and-limits"
import = "@/docs/targets/pi-reference.md"
+++

# Pi (`pi`)

## Limits and trust {#limits-and-trust .legacy-anchor}

## Native mappings {#native-mappings .legacy-anchor}

## User configuration {#global-configuration .legacy-anchor}

## Config keys {#config-keys .legacy-anchor}

## Import {#import .legacy-anchor}

agnostic-ai writes Pi's native project files and imports existing ones. It follows [Pi v1.4.0](https://github.com/earendil-works/pi/releases/tag/v1.4.0) (`@earendil-works/pi-coding-agent`).

## First rule

1. Add `pi` to `targets` in `agnostic-ai.yaml`.
2. Save `.agnostic-ai/rules/commit-prefix.md`:

```markdown
---
name: commit-prefix
---

Start commit subjects with fix:, feat:, docs:, or chore:.
```

3. Run `agnostic-ai sync -t pi`, then `agnostic-ai sync --check -t pi`.
4. Find the rule in `AGENTS.md`. Start Pi from the project root and ask for a commit subject.

Have an existing setup? [Import it](@/docs/targets/pi-reference.md#import) before syncing.

## Output

| Source | Default output |
|---|---|
| Root instructions and rules | `AGENTS.md` |
| Directory-scoped rules | `<scope>/AGENTS.md` |
| Skills and bundled files | `.pi/skills/<name>/SKILL.md` |
| Commands (as prompt templates) | `.pi/prompts/<name>.md` |
| MCP servers | `.pi/mcp.json` |
| Model and thinking settings | `.pi/settings.json` |

Overrides, field mappings, user-level sync, and import: [native reference](@/docs/targets/pi-reference.md).

## Loading and limits

- Project settings, MCP, skills, and prompt templates need Pi's project trust approval. Instruction files do not.
- Pi has no agents, hooks, or per-tool permission rules, so those specs have no output.
- MCP supports stdio and HTTP, not SSE.

{% <details summary="More limits"> %}
- Trust stays Pi's decision. Sync writes no trust approvals and no `defaultProjectTrust`.
- Agent, hook, ignore, review, and environment specs have no native output. Portable permission settings are unsupported.
- Skill `allowed-tools` is metadata, not an enforced permission limit.
- Invalid user-level MCP specifications stop sync before native files change.
- The default prompt directory loads direct Markdown children. Nested command names are unsupported.
- Commands sync only at project scope. User-level command sync is unsupported.
{% </details> %}

{% <details summary="Recipe: add a disabled MCP server"> %}
Save `.agnostic-ai/mcps/docs-search.yaml`:

```yaml
name: docs-search
type: http
url: https://mcp.example.com/mcp
disabled: true
```

After sync, `.pi/mcp.json` has the server with `enabled: false`. Replace the example URL with your endpoint before enabling it.
{% </details> %}

## Protected paths

Not enforced. A settings spec with a `protected` block produces a coverage note. See [protected-path alternatives](@/docs/spec-format/settings.md#protected-paths).

Next, [add a skill](@/docs/spec-format/skills.md) or see [user configuration](@/docs/targets/pi-reference.md#global-configuration).

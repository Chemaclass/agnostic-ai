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

## Global configuration {#global-configuration .legacy-anchor}

## Config keys {#config-keys .legacy-anchor}

## Import {#import .legacy-anchor}

agnostic-ai writes Pi's native project files and imports existing ones. This target follows [Pi v1.4.0](https://github.com/earendil-works/pi/releases/tag/v1.4.0), published as `@earendil-works/pi-coding-agent`.

## First rule

Select the target in `agnostic-ai.yaml`:

```yaml
targets: [pi]
```

Save `.agnostic-ai/rules/commit-prefix.md`:

```markdown
---
name: commit-prefix
---

Start commit subjects with fix:, feat:, docs:, or chore:.
```

Run:

```bash
agnostic-ai sync -t pi
agnostic-ai sync --check -t pi
```

Inspect `AGENTS.md` for the rule, then start Pi from the project root and ask for a commit subject to check that it loaded. For an existing setup, [import it](@/docs/targets/pi-reference.md#import) before syncing.

## Output

| Source | Default output |
|---|---|
| Root instructions and rules | `AGENTS.md` |
| Directory-scoped rules | `<scope>/AGENTS.md` |
| Skills and bundled files | `.pi/skills/<name>/SKILL.md` |
| Commands (as prompt templates) | `.pi/prompts/<name>.md` |
| MCP servers | `.pi/mcp.json` |
| Model and thinking settings | `.pi/settings.json` |

For output overrides, field mappings, global sync, and import coverage, see the [native reference](@/docs/targets/pi-reference.md).

## Loading and limits

- Project settings, MCP, skills, and prompt templates need Pi's project trust approval. Instruction files load without that approval. Trust remains Pi's decision; sync does not write trust approvals or `defaultProjectTrust`.
- Pi has no native agent definitions, lifecycle hooks, or per-tool permission rules. Agent, hook, ignore, review, and environment specs have no native output; portable permission settings are unsupported. Skill `allowed-tools` is metadata, not an enforced permission limit.
- MCP supports stdio and HTTP; SSE is unsupported. Invalid global MCP specifications stop sync before native files change.
- The default prompt directory loads direct Markdown children. Nested command names are unsupported.
- Commands sync only at project scope. Global command sync is outside this target's support.

## Add a disabled MCP server

Save `.agnostic-ai/mcps/docs-search.yaml` to define an HTTP server without enabling it:

```yaml
name: docs-search
type: http
url: https://mcp.example.com/mcp
disabled: true
```

After sync, `.pi/mcp.json` contains the server with `enabled: false`. Replace the example URL with your server's endpoint before enabling it.

## Protected paths

Not enforced. A settings spec with a `protected` block produces a coverage note. See [protected-path alternatives](@/docs/spec-format/settings.md#protected-paths).

Next, [add a skill](@/docs/spec-format/skills.md) or see [global configuration](@/docs/targets/pi-reference.md#global-configuration).

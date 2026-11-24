+++
title = "Spec format"
description = "What each folder under .agnostic-ai/ holds, and the rules every spec shares."
weight = 110
sort_by = "weight"
template = "docs/hub.html"
page_template = "docs/page.html"
aliases = ["/docs/spec-format/skills-rules-commands/"]

[extra]
group = "Reference"
hub = true
nav_first = "Reference"

[extra.moved]
mcp-servers = "@/docs/spec-format/mcps.md"
+++

# Spec format

A spec is a file under `.agnostic-ai/`. `agnostic-ai sync` writes it for each enabled tool in that tool's own path and format, such as `.claude/agents/`, `.cursor/rules/`, `.codex/config.toml`, or `.gemini/settings.json`.

Choose the folder for your spec. Write when it applies, what to do, and what to return. Check it with `agnostic-ai lint` and `agnostic-ai sync --dry-run`, then sync and run `agnostic-ai sync --check`.

## Folders

| Folder | Holds | Use it when |
|--------|-------|-------------|
| [`agents/`](@/docs/spec-format/agents.md) | Subagents with their own prompt, tools, and model | a separate agent should handle one part of a task |
| [`skills/`](@/docs/spec-format/skills.md) | Procedures loaded on demand, with bundled files | a workflow is long, occasional, or needs scripts |
| [`rules/`](@/docs/spec-format/rules.md) | Standing conventions, for the project, a directory, or a file pattern | the agent must follow it every time |
| [`commands/`](@/docs/spec-format/commands.md) | Slash commands | a person starts the workflow by name |
| [`hooks/`](@/docs/spec-format/hooks.md) | Commands the tool runs at events such as session start | something must happen every time, not when the model remembers |
| [`mcps/`](@/docs/spec-format/mcps.md) | MCP server connections | the agent needs an outside tool or data source |
| [`settings/`](@/docs/spec-format/settings.md) | Permissions, default model, and effort | you need shared permissions or model defaults |
| [`reviews/`](@/docs/spec-format/reviews.md) | Guidance for code-review bots | a review bot should check project rules |
| [`environments/`](@/docs/spec-format/environments.md) | Worktree setup, dev servers, and cleanup | agents work in fresh worktrees |
| [`ignore/`](@/docs/spec-format/ignore.md) | Context and indexing exclusions for supported tools | irrelevant files crowd search or context |
| [`overlays/`](@/docs/spec-format/overlays.md) | Native settings and helper files that `import` captured | a tool setting has no portable spec |

Beside them, `AGNOSTIC_AI.md` holds the text every [entry-point file](@/docs/configuration-reference.md#entry-point-files) carries, and `local/` holds [personal specs](@/docs/local-overrides.md) that stay out of Git.

- Paths are relative to `.agnostic-ai/`, so `rules/*.md` means `.agnostic-ai/rules/*.md`. [`sources`](@/docs/configuration-reference.md#sources) overrides the directories.
- Subfolders load too: every `.md` under `agents/`, `rules/`, `commands/`, `reviews/`, and `ignore/`, and every `.yaml` under `hooks/`, `mcps/`, `settings/`, and `environments/`. Under `skills/`, Markdown beside a `SKILL.md` is bundled reference material, not another skill.
- `agnostic-ai new <kind> <name>` creates a starter spec for [each supported kind](@/docs/cli-reference/start.md#new). [Getting started](@/docs/getting-started.md) walks through a first rule.
- The [tool support table](@/docs/targets/_index.md#capability-matrix) shows which tools get each kind; each tool's page shows how it writes it.

## Rules every spec shares

[Shared spec features](@/docs/spec-format/shared-features.md) covers what applies to all kinds:

- [per-directory scope](@/docs/spec-format/shared-features.md#nested-layout-per-directory-scope) for specs in subfolders;
- [target fields](@/docs/spec-format/shared-features.md#target-scoping) (`target`, `targets`, and their exclusions);
- [frontmatter rules](@/docs/spec-format/shared-features.md#frontmatter-rules);
- [per-tool body fences](@/docs/spec-format/shared-features.md#per-target-body-fences) (`::target`);
- [path variables](@/docs/spec-format/shared-features.md#path-variables-name) such as `{{$SKILLS_DIR}}` and [agent and skill references](@/docs/spec-format/shared-features.md#agent-and-skill-references);
- [`x-<target>` blocks](@/docs/spec-format/shared-features.md#target-specific-extensions-x-target-namespace) for fields only one tool reads;
- [terms](@/docs/spec-format/shared-features.md#terms) used in these pages.

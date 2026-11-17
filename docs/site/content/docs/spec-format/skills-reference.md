+++
title = "Skill reference"
description = "How each tool handles a skill's disable-model-invocation field."
weight = 21

[extra]
group = "Reference"
+++

# Skill reference

Per-tool details for the fields on the [skills page](@/docs/spec-format/skills.md).

## `disable-model-invocation` support by target {#disable-model-invocation-support-by-target}

Without the field, every tool lets the model use the skill on its own. Each target page gives the native details and links the vendor docs.

| Target | Behavior |
|--------|----------|
| [Claude Code](@/docs/targets/claude.md), [Cursor](@/docs/targets/cursor.md), [Zed](@/docs/targets/zed.md), [Qwen Code](@/docs/targets/qwen.md) | Written to `SKILL.md` |
| [Copilot](@/docs/targets/copilot.md) | Written to `SKILL.md`, with `argument-hint` |
| [OpenCode](@/docs/targets/opencode.md) | Written to `SKILL.md`. V2 honors it; V1 does not |
| [Codex](@/docs/targets/codex.md) | `allow_implicit_invocation: false` in `agents/openai.yaml` |
| [Windsurf](@/docs/targets/windsurf.md) | `triggers: [user]` in `SKILL.md` |
| [Crush](@/docs/targets/crush.md) | Written to `SKILL.md` with `user-invocable: true` |
| [Factory](@/docs/targets/factory.md) | Dropped with a note. Set `x-factory.disable-model-invocation` |
| Cline, Amp, Warp, Antigravity, Continue, Gemini CLI, Goose, OpenHands | Dropped with a note: no documented field keeps a skill user-only |

<details>
<summary>Shared skills folders and Factory</summary>

- Copilot, Zed, Windsurf, Crush, and Qwen Code write the field only while no other target writes the same skills folder. Every writer of a shared `SKILL.md` must agree on its bytes.
- Otherwise the field is dropped with a note. Point the tool's `skills-dir` at a folder of its own to keep it.
- Factory skills go in the shared `.agents/skills/` folder, where the key would reach tools that ignore it and let their models use a manual-only skill.
- So while another target writes that folder, `x-factory.disable-model-invocation` also needs `outputs.factory.skills-dir: .factory/skills`. Otherwise sync stops with `AAI-102`.

</details>

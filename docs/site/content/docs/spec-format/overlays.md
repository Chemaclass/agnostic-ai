+++
title = "Overlays"
description = "overlays/: tool-specific settings and helper files that import captured, so sync keeps writing them."
weight = 110

[extra]
group = "Reference"
+++

# Overlays

`overlays/` keeps the parts of a tool's configuration that no shared spec covers yet, such as a Claude Code `statusLine`, Codex `[tui]` settings, or a status-line script. `agnostic-ai import` saves them here, and `sync` writes them back into the tool's files. Sync also restores a deleted `.claude/` or `.codex/` directory with those keys.

To change an imported setting, edit its overlay below and run `agnostic-ai sync --dry-run` to review the result. Then sync and check the tool's generated file. An overlay reaches one tool only; move a setting into a portable spec when it should reach other tools and that spec kind supports it.

## Files

| Overlay | Written into | Holds |
|---------|--------------|-------|
| `claude.settings.json` | `.claude/settings.json` | Every key import did not move into a spec, such as `statusLine` |
| `claude.settings.hook-events.json` | `.claude/settings.json` | The keys that sat next to `hooks`, and the order of its events, so the `hooks` block keeps both |
| `codex.config.toml` | `.codex/config.toml` | The keys import did not move into a spec, such as `sandbox`, `[history]`, and `[tui]`. Keys Codex ignores in a project config stay here but are not written ([list](@/docs/targets/codex-reference.md#keys-codex-ignores-in-a-project-config)) |
| `codex.exec-policies.yaml` | `.codex/rules/default.rules` | Every `prefix_rule(...)` from the imported file |
| `claude/<file>`, `codex/<file>` | `.claude/<file>`, `.codex/<file>` | Helper files: Claude `CLAUDE.md`, `README.md`, and `statusline.sh`; Codex `README.md`. File modes are kept, so a script stays executable |

`sync --watch` re-runs when an overlay changes. The target pages say which source wins: [Claude settings](@/docs/targets/claude-reference.md#claude-settings), [Codex config](@/docs/targets/codex-reference.md#codex-config), and [Codex exec policies](@/docs/targets/codex-reference.md#codex-exec-policies).

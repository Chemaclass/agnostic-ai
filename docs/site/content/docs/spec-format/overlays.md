+++
title = "Overlays"
description = "overlays/: native settings and helper files that import captured, so sync keeps writing them."
weight = 110

[extra]
group = "Reference"
+++

# Overlays

`overlays/` keeps the parts of a tool's configuration that no portable spec holds yet, such as a Claude Code `statusLine`, Codex `[profiles.*]`, or a status-line script. `agnostic-ai import` writes them here, and `sync` writes them back into the tool's files. Nothing is lost when you adopt agnostic-ai, and sync rebuilds a deleted `.claude/` or `.codex/` directory with those keys.

- **Safe adoption.** Import moves what it can into specs and keeps the rest verbatim.
- **Still editable.** An overlay is tracked source; edit it and sync, like any spec.
- **Known precedence.** On Claude Code, specs and `outputs.claude.settings` win over the overlay. On Codex, the overlay wins over `outputs.codex.config`.

An overlay reaches one tool only. When a portable spec kind covers a setting, move it into that spec so every tool gets it.

## Files

| Overlay | Written into | Holds |
|---------|--------------|-------|
| `claude.settings.json` | `.claude/settings.json` | Every key import did not move into a spec, such as `statusLine` |
| `claude.settings.hook-events.json` | `.claude/settings.json` | The hook event order import found, so the `hooks` block keeps it |
| `codex.config.toml` | `.codex/config.toml` | The keys import did not move into a spec, such as `sandbox`, `[history]`, and `[profiles.*]` |
| `codex.exec-policies.yaml` | `.codex/rules/default.rules` | Every `prefix_rule(...)` from the imported file |
| `claude/<file>`, `codex/<file>` | `.claude/<file>`, `.codex/<file>` | Helper files: Claude `CLAUDE.md`, `README.md`, and `statusline.sh`; Codex `README.md`. File modes are kept, so a script stays executable |

`sync --watch` re-runs when an overlay changes. The target pages give each layer's precedence: [Claude settings](@/docs/targets/claude.md#claude-settings), [Codex config](@/docs/targets/codex.md#codex-config), and [Codex exec policies](@/docs/targets/codex.md#codex-exec-policies).

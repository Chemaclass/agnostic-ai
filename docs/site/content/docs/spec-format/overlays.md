+++
title = "Overlays"
description = "overlays/: imported tool-specific settings and helper files that sync restores."
weight = 110

[extra]
group = "Reference"
+++

# Overlays

`overlays/` keeps the parts of a Claude Code or Codex configuration that no shared spec covers yet, such as a Claude Code `statusLine`, Codex `[tui]` settings, or a status-line script. `agnostic-ai import` saves them here, and `sync` writes them back into the tool's files, restoring a deleted `.claude/` or `.codex/` directory with those keys.

Import leaves out of `claude.settings.json` each `env` value, helper command such as `apiKeyHelper`, and `statusLine` command that holds a credential, and any literal value of a header variable such as `OTEL_EXPORTER_OTLP_HEADERS`, and prints a warning naming the key. The [setting value rule](@/docs/spec-format/mcp-import.md#setting-values) decides which `env` values hold a credential. The next sync then removes it from `.claude/settings.json`. To keep it, set it under `x-claude` in a settings spec in [`local/`](@/docs/local-overrides.md), which Git ignores.

To change an imported setting, edit its overlay, check `agnostic-ai sync --dry-run`, then sync. An overlay reaches one tool only; move a setting into a portable spec when other tools should get it.

## Files

| Overlay | Written into | Holds |
|---------|--------------|-------|
| `claude.settings.json` | `.claude/settings.json` | Every key import did not move into a spec, such as `statusLine` |
| `claude.settings.hook-events.json` | `.claude/settings.json` | The keys that sat next to `hooks`, and the order of its events, so the `hooks` block keeps both |
| `codex.config.toml` | `.codex/config.toml` | The keys import did not move into a spec, such as `sandbox`, `[history]`, and `[tui]`. Keys Codex ignores in a project config stay here but are not written ([list](@/docs/targets/codex-reference.md#keys-codex-ignores-in-a-project-config)) |
| `codex.exec-policies.yaml` | `.codex/rules/default.rules` | Every `prefix_rule(...)` from the imported file |
| `codex.exec-policies-header.txt` | `.codex/rules/default.rules` | The comment block above the first rule |
| `claude/<file>`, `codex/<file>` | `.claude/<file>`, `.codex/<file>` | Helper files: Claude `CLAUDE.md`, `README.md`, and `statusline.sh`; Codex `README.md`. File modes are kept, so a script stays executable |

Sync reads only these files. Any other file here, such as `gemini.settings.json` or a misspelled `claude.setings.json`, gets a sync note and a `lint` warning (LINT051). Dot files, dot folders, and README files at the top are skipped. For Gemini CLI and the other tools that merge `x-<target>` from a [settings spec](@/docs/spec-format/settings.md), put the keys there; Cursor's settings spec drops that block.

`sync --watch` re-runs when an overlay changes. The target pages say which source wins: [Claude settings](@/docs/targets/claude-reference.md#claude-settings), [Codex config](@/docs/targets/codex-reference.md#codex-config), and [Codex exec policies](@/docs/targets/codex-reference.md#codex-exec-policies).

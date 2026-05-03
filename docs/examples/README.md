# Examples

Drop-in templates. Copy any of these into your project's source dirs and
run `agnostic-ai sync`.

| File | Drop into | What it shows |
|------|-----------|---------------|
| [agnostic.config.yaml](agnostic.config.yaml) | project root | Full config schema with defaults shown as comments. |
| [agents/code-reviewer.md](agents/code-reviewer.md) | `agents/` | Minimal agent with `tools` and `model`. |
| [skills/yaml-validator.md](skills/yaml-validator.md) | `skills/` | Flat skill (`name`, `description`, body). |
| [rules/conventional-commits.md](rules/conventional-commits.md) | `rules/` | Always-applied rule. |
| [hooks/format-on-save.yaml](hooks/format-on-save.yaml) | `hooks/` | `PostToolUse` hook (Claude Code only). |

## Quick demo

```bash
agnostic-ai init
cp -R docs/examples/agents/*  agents/
cp -R docs/examples/rules/*   rules/
cp -R docs/examples/skills/*  skills/
cp -R docs/examples/hooks/*   hooks/
agnostic-ai sync --dry-run
```

See [user docs](../user/README.md) for the full spec format and per-target
behavior.

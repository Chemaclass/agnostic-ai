# Examples

[All docs](../README.md) · [Spec format](../user/spec-format.md)

## Try the starter specs

Run in an empty scratch directory:

```bash
echo "claude,cursor" | agnostic-ai init --demo
agnostic-ai sync --dry-run
```

The demo seeds examples for agents, skills, rules, hooks, and MCP servers. Some targets do not support every kind; preview output includes warnings for those omissions. Edit or delete the samples before using them in a real project.

For stack-specific starters, use `init --preset go`, `init --preset ts-react`, or `init --preset python`. The bundled sources live in [internal/cli/initdata](../../internal/cli/initdata/).

## Configure only what you need

Start with the config created by `init`. [agnostic-ai.yaml](agnostic-ai.yaml) is a small, commented starter for Claude Code and Cursor. The [configuration reference](../user/configuration.md#full-schema) contains the expanded field listing.

See [Configuration](../user/configuration.md) for defaults and [Targets](../user/targets.md) for supported fields per tool.

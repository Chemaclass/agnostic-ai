# Examples

[All docs](../README.md) · [Spec format](https://agnostic-ai.org/docs/spec-format/)

## Try the starter specs

Run in an empty scratch directory:

```bash
echo "claude,cursor" | agnostic-ai init --demo
agnostic-ai sync --dry-run
```

The demo creates examples for agents, skills, rules, hooks, and MCP servers. Some targets do not support every kind; preview output includes warnings for those omissions. Edit or delete the samples before using them in a real project.

For language-specific starters, use `init --preset go`, `init --preset ts-react`, or `init --preset python`. The bundled sources live in [internal/cli/initdata](../../internal/cli/initdata/).

## Try directory-specific instructions

The [directory-specific instructions walkthrough](https://agnostic-ai.org/docs/scoped-context/#start-with-one-directory) creates one payments rule for Claude, Codex, Gemini, and Cursor, with commands to inspect each tool's output path. Run it in a scratch project or use its rule in your existing project.

## Configure only what you need

Start with the config created by `init`. [agnostic-ai.yaml](agnostic-ai.yaml) is a small, commented starter for Claude Code and Cursor. The [configuration reference](https://agnostic-ai.org/docs/configuration/) describes every field.

See [Configuration](https://agnostic-ai.org/docs/configuration/) for defaults and [Targets](https://agnostic-ai.org/docs/targets/) for supported fields per tool.

## Add RTK or Caveman independently

[Project packs for RTK and Caveman](rtk-and-caveman/README.md) supplies two local packs (folders of shared specs) for Claude Code on macOS or Linux. It explains how to replace an existing installation, remove either pack, and repeat the configuration checks. The RTK hook and default Caveman response skill remain separate choices.

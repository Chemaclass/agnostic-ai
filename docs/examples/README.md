# Examples

[All docs](../README.md) · [Spec format](https://agnostic-ai.org/docs/spec-format/)

## Try the starter specs

Run in an empty scratch directory:

```bash
echo "claude,cursor" | agnostic-ai init --demo
agnostic-ai sync --dry-run
```

The demo creates examples for agents, skills, rules, hooks, and MCP servers. Some coding tools cannot use every spec kind; the preview warns about anything they will skip. Edit or delete the samples before using them in a real project.

For language-specific starters, use `init --preset go`, `init --preset ts-react`, or `init --preset python`. The bundled sources live in [internal/cli/initdata](../../internal/cli/initdata/).

## Try directory-specific instructions

The [directory-specific instructions walkthrough](https://agnostic-ai.org/docs/scoped-context/#start-with-one-directory) creates a payments rule for Claude, Codex, Gemini, and Cursor and shows how to inspect each tool's output. Run it in a scratch project or add the rule to an existing project.

## Configure only what you need

Start with the config created by `init`. [agnostic-ai.yaml](agnostic-ai.yaml) is a small, commented starter for Claude Code and Cursor. The [configuration reference](https://agnostic-ai.org/docs/configuration/) describes every field.

Check [Targets](https://agnostic-ai.org/docs/targets/) for the fields your tool can use. After editing, run `agnostic-ai sync`, inspect the generated files, and finish with `agnostic-ai sync --check`.

## Add RTK or Caveman independently

[Project packs for RTK and Caveman](rtk-and-caveman/README.md) provides two independent packs (folders of shared specs) for Claude Code on macOS or Linux: an RTK hook and the default Caveman response skill. The guide covers replacing an existing installation, removing either pack, and repeating the configuration checks.

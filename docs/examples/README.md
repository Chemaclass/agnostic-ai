# Examples

[All docs](../README.md) · [Spec format](https://agnostic-ai.org/docs/spec-format/)

## Try the starter specs

Run in an empty scratch directory:

```bash
echo "claude,cursor" | agnostic-ai init --demo
agnostic-ai sync --dry-run
```

The demo creates sample agents, skills, rules, hooks, and MCP servers. The preview warns when a tool skips an unsupported spec kind. Edit or delete the samples before using them in a real project.

Use `init --preset go`, `init --preset ts-react`, or `init --preset python` for a language-specific starter. The [bundled sources](../../internal/cli/initdata/) show what each starter contains.

## Try directory-specific instructions

The [directory-specific instructions walkthrough](https://agnostic-ai.org/docs/scoped-context/#start-with-one-directory) creates a payments rule for Claude, Codex, Gemini, and Cursor and shows how to inspect each tool's output. Run it in a scratch project or add the rule to an existing project.

## Configure only what you need

Start with the config created by `init`. [agnostic-ai.yaml](agnostic-ai.yaml) is a small, commented starter for Claude Code and Cursor. The [configuration reference](https://agnostic-ai.org/docs/configuration/) describes every field.

Check [Targets](https://agnostic-ai.org/docs/targets/) for the fields your tool can use. After editing, run `agnostic-ai sync`, inspect the generated files, and finish with `agnostic-ai sync --check`.

## Add RTK or Caveman independently

Add an RTK hook or the Caveman response skill to Claude Code on macOS or Linux. The guide covers existing installations, removal, and configuration checks. [Project packs for RTK and Caveman](rtk-and-caveman/README.md)

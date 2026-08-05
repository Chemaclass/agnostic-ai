# agnostic-ai

One spec, every AI CLI. Write your agents, skills, rules, hooks, and MCP servers once, then emit them to Claude Code, Codex, Gemini, Cursor, Copilot, and 20 more in each tool's native format.

```bash
npx agnostic-ai init --demo   # scaffold specs, one example per kind
npx agnostic-ai sync          # emit native config for every target
```

Or install it globally:

```bash
npm install -g agnostic-ai
```

This package is a thin wrapper: it downloads the prebuilt Go binary for your platform from [GitHub Releases](https://github.com/Chemaclass/agnostic-ai/releases) and runs it. Supported platforms are macOS, Linux, and Windows on x64 and arm64.

The download normally happens on install. Under npm's install-script gating (`--ignore-scripts`, or npm 11's default prompt), it happens on first run instead. Pin a different binary version with `AGNOSTIC_AI_VERSION=v0.45.0`.

Full docs, targets, and configuration: [github.com/Chemaclass/agnostic-ai](https://github.com/Chemaclass/agnostic-ai).

MIT licensed.

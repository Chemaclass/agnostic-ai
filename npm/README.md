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

npm is one route of several. The same binary installs through Homebrew on macOS and Linux:

```bash
brew install --cask Chemaclass/tap/agnostic-ai
```

Or through the install script, which needs no package manager:

```bash
curl -fsSL https://raw.githubusercontent.com/Chemaclass/agnostic-ai/main/scripts/install.sh | bash
```

Windows, Go, and manual download are covered in [all install options](https://agnostic-ai.org/docs/installation/).

This package is a thin wrapper: it downloads the prebuilt Go binary for your platform from [GitHub Releases](https://github.com/Chemaclass/agnostic-ai/releases) and runs it. Supported platforms are macOS, Linux, and Windows on x64 and arm64.

The download normally happens on install. Under npm's install-script gating (`--ignore-scripts`, or npm 11's default prompt), it happens on first run instead. Set `AGNOSTIC_AI_VERSION` to any [release tag](https://github.com/Chemaclass/agnostic-ai/releases) to pin a different binary.

Full docs, targets, and configuration: [agnostic-ai.org](https://agnostic-ai.org).

MIT licensed.

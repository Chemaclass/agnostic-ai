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

This package is a thin wrapper around the prebuilt Go binary. The binary ships inside a platform package (`@chemaclass/agnostic-ai-darwin-arm64` and five siblings), declared as optional dependencies. npm reads each one's `os` and `cpu` and installs only the one that matches your machine. Supported platforms are macOS, Linux, and Windows on x64 and arm64.

Nothing is downloaded and no install script runs, so the package works under `--ignore-scripts`, behind a proxy, and from an offline npm mirror. To pin a version, pin the package: `npm install -g agnostic-ai@<version>`.

Set `AGNOSTIC_AI_BINARY` to an absolute path to run a binary this package does not ship, such as one you built yourself.

Full docs, targets, and configuration: [agnostic-ai.org](https://agnostic-ai.org).

MIT licensed.

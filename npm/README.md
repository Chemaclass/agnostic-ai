# agnostic-ai

Keep one set of instructions and settings for your coding tools. Write source files (specs) once, then run sync to create the files Claude Code, Codex, Gemini, Cursor, Copilot, and other supported tools read.

```bash
npx agnostic-ai init --from all # import existing tool files; select the tools you use
npx agnostic-ai sync --plan     # review the planned files
npx agnostic-ai sync            # write each selected tool's configuration
npx agnostic-ai sync --check    # confirm the files match their sources
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

This package is a launcher for the ready-to-run command. The binary ships inside a platform package (`@agnostic-ai/darwin-arm64` and five siblings), declared as optional dependencies. npm reads each one's `os` and `cpu` and installs only the one that matches your machine. Supported platforms are macOS, Linux, and Windows on x64 and arm64.

npm obtains the wrapper and matching platform package from its registry or cache. No install script makes a separate binary download, so the package works under `--ignore-scripts` and through npm's proxy settings. Offline installs need both packages in the cache or mirror. To pin a version, pin the package: `npm install -g agnostic-ai@<version>`.

Set `AGNOSTIC_AI_BINARY` to an absolute path to run a binary this package does not ship, such as one you built yourself.

Full docs, targets, and configuration: [agnostic-ai.org](https://agnostic-ai.org).

MIT licensed.

# Security Policy

## Supported versions

The latest minor release only.

## Reporting a vulnerability

Open a [private advisory](https://github.com/Chemaclass/agnostic-ai/security/advisories/new). Do not open a public issue.

A fix or mitigation ships within 90 days of a confirmed report. Public disclosure follows that window.

## Verify a download

Every release carries checksums, a signed build provenance attestation and an SBOM per archive, npm provenance, and a signed tag. [Verify a release](https://agnostic-ai.org/docs/verify-a-release/) shows each check.

## What agnostic-ai does when it runs

agnostic-ai reads specs from the project and writes configuration files. It sends no telemetry.

**Network.** Only two commands connect to the network:

- `upgrade` downloads a release from GitHub and checks it against `checksums.txt` before it replaces the binary.
- `packs add` and `packs update` clone the pack's Git repository.

`sync`, `check`, `doctor`, `import`, `lint`, and every other command work offline.

**Programs it runs.**

- `git`, to read the index, history, and hook settings.
- `agnostic-ai verify` runs the `verify.command` from your config, and each tool's `--version`.
- A target in `targets:` that is not built in runs `agnostic-ai-adapter-<name>` from your `PATH`. Check that a target name is spelled right.
- `install-hook` writes Git hooks that run `agnostic-ai sync`.

**Files it writes.** Project outputs go under the project root. `sync --global` writes user configuration under your home directory, such as `~/.claude/settings.json`.

## What to review

agnostic-ai never runs a hook. The AI tools it configures do. A hook spec, an MCP server spec, or a settings spec becomes a command or a permission those tools act on. A pack does the same when you install it.

Review changes under `.agnostic-ai/`, and the packs you install, like code. Run `agnostic-ai sync --check` in CI so every generated change shows up in a pull request's diff before anyone's tools load it.

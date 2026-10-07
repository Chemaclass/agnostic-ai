# Changelog

Each release lists general changes first, then changes by tool, then site work. Versioning: [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Entry style, section order, and what belongs here instead of the issue or the docs: `.agnostic-ai/agents/changelog-curator.md`.

## [Unreleased]

## v0.81.0 - 2026-10-07

### General

- **Breaking:** `lint` and `sync` stop on a plain MCP `env` or `headers` value not marked `!literal`; run `agnostic-ai migrate --only secrets` (#1794).
- New `memory` built-in: one memory every tool loads at session start and saves to, for the team and for you (#1844, #1845, #1850, #1851, #1852).
- `memory.personal: repo` shares one personal memory across every worktree, also when `~/.agnostic-ai` is a link (#1859, #1869, #1877).
- New `memory lint`, `index`, `list`, and `path` commands check, rebuild, list, and locate memory; `doctor` flags broken links and secrets (#1847, #1877).
- Sync keeps hooks you wrote by hand when it adds its own, on Claude Code, Codex, Cursor, Gemini CLI, Qoder, and Factory (#1858).

### By tool

#### Claude Code

- **Breaking:** sync stops on an agent name that starts with `-` or contains `:`, which Claude Code would skip; rename the agent (#1870).
- With `memory`, Claude Code's own memory saves into the shared personal memory, so other tools see it (#1846).
- `lint` counts the `AGENTS.md` that `CLAUDE.md` imports in Claude Code's word total, which it used to leave out.

#### Cursor

- `.cursor/cli.json` always has both `allow` and `deny`; the Cursor CLI refused a file with only one, so its rules never loaded.
- In repo mode, the Cursor CLI can save personal memory: sync allows writes to its folder in `.cursor/cli.json`.
- Turning off the last Cursor hook removes `.cursor/hooks.json` instead of leaving `{"version": 1}` behind.

#### Copilot

- A hook with a Windows command now runs on Windows: sync writes separate `bash` and `powershell` commands (#1856).

#### Qoder

- In repo mode, Qoder saves personal memory without asking for approval each time (#1872).

#### Kilo Code

- In repo mode, Kilo finds personal memory through the rule; sync no longer lists a file Kilo ignores (#1871).

#### Augment

- `hook run` now shows that a `SessionStart` hook's output reaches the model (#1853).

#### OpenHands

- `hook run` no longer claims a `SessionStart` hook's output reaches the model; OpenHands only logs it (#1853).

### Site

- Docs rewritten in plain words and trimmed, with a new shared memory guide.

Releases before v0.81.0 are in [docs/CHANGELOG-archive.md](docs/CHANGELOG-archive.md).

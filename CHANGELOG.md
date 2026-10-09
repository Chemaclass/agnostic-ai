# Changelog

Each release lists general changes first, then changes by tool, then site work. Versioning: [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Entry style, section order, and what belongs here instead of the issue or the docs: `.agnostic-ai/agents/changelog-curator.md`.

## [Unreleased]

### General

- Builds use Go 1.27.2, which fixes an HTTP/2 crash in the standard library (GO-2026-6617).
- `compare` runs in under a second on large projects, down from 47 s at 500 specs (#1891).
- `lint` runs in a quarter second at 500 specs, down from 4 s; `doctor` drops from 11 s to 5 s (#1893).
- `graph` runs in about 2 s at 500 specs, down from 40 s, and `graph --target` only renders that target (#1892).
- `sync` asks git once, not once per 500 outputs, which files it tracks but ignores: at 500 specs `sync` takes half the time (#1897).
- `sync`, `sync --check`, and `status` read each skill folder once per run, not once per tool: `status` is about 15% faster at 500 specs (#1896).
- `explain` on a spec runs in about 0.3 s at 500 specs, down from 2 s (#1899).
- `doctor` loads the project once for its checks, drift, and migration plans, and scans for unmanaged config while other checks run: it drops from 11 s to about 3 s at 500 specs (#1894).
- Release binaries no longer embed the build machine's source paths (#1902).
- A `sync` with nothing to change is about 30% faster at 500 specs: it reads each output once and skips repeated folder, state, and skill-folder reads (#1895).
- `doctor` lists the installed AI CLIs in name order, so two runs print the same output (#1903).
- `memory lint`, `lint`, and `doctor` warn (LINT039) when the two memory indexes pass 6,000 bytes. With Codex, Copilot, Cursor, Gemini CLI, Qoder, or Factory enabled, the finding names the facts their session-start hook never loads; otherwise it names the tools that load the whole indexes every session (#1930).
- A value you only reordered in a merged JSON file now counts as your edit, so it stays when the spec that wrote it is removed. OpenCode applies the last matching rule, so order changes what it allows (#1885).
- Turning on `memory.personal: repo` no longer changes the committed `.gitignore` and `.worktreeinclude`, so teammates and CI stay in sync. With the `memory` built-in on, the block now always lists `.codex/config.toml`, `.cursor/cli.json`, `.cursor/.agnostic-ai-permissions.json`, and `.devin/config.json` for their tools; to commit one of them, add it to `gitignore.allow` (#1936).
- The `shared-memory-policy` rule describes `memory.personal: repo` in one shorter sentence, so every session loads 155 fewer bytes (#1931).
- Re-import keeps unedited specs, including linked sources and older overlays, and refuses edits that lose text for other tools (#1938, #1941).

### By tool

#### Claude Code

- `failClosed: true` on a hook now blocks when the hook fails or times out, in project and global sync and in `hook run` (#1916).
- `import claude` keeps each hook command's own timeout, shell, and other settings, so the next sync no longer adds a duplicate group (#1920).
- `sync --global` edits each tool's global hooks file in place, so removing a hook gives back the file exactly as you wrote it (#1919).
- `doctor` names a Claude hook spec an older import merged, which runs each hook twice, and says to delete it, run `sync`, then `import claude` (#1922).
- `import claude` keeps `onFailure` on an MCP-tool or prompt hook, and any value other than `block`, as `x-claude.onFailure`, and sync writes it back, so the next sync no longer adds a second group that ignores it. Specs an older import gave `failClosed: true` on these handlers keep one group without a re-import (#1922).

#### Codex

- **Breaking:** Codex now applies the sandbox you set in `outputs.codex.config.sandbox`, and an invalid value such as `workspace` stops sync; use `read-only`, `workspace-write`, or `danger-full-access` (#1880).

#### Kilo Code

- The note for a command spec named `goal` now says Kilo offers it as `/goal:command` and warns, instead of rejecting it (#1906).

#### OpenCode

- In repo mode, OpenCode saves personal memory without asking each time: sync allows its folder under `permission.external_directory` (#1881).
- In repo mode, moving `AGNOSTIC_AI_HOME` no longer leaves the old personal memory index in `instructions` (#1886).

#### Windsurf

- In repo mode, Devin CLI saves personal memory without asking: sync allows writes to its folder in `.devin/config.json` (#1882).

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

## v0.80.0 - 2026-10-06

### General

- Opt-in `handoff` built-ins carry a task between tools, save approved rules, and add Git snapshots with resume notices (#1829, #1830, #1831).
- Built-in text can change with a release and show as `sync --check` drift. Run `sync` after upgrading (#1829).
- With no changed files, `sync` and `sync --check` run 2 to 5x faster; `sync --check --against` is about 5x faster at 20k+ files (#1819, #1821).
- `@path` lines inside a code fence stay as written, and a one-line code span no longer hides the lines after it (#1823, #1828).

### By tool

#### Kiro

- Commands sync to and import from `.kiro/prompts/` for CLI V3 slash commands, with native argument templates intact (#1822).

### Site

- The Amp page drops the services `agent` key, which Amp no longer documents (#1835, #1839).

Releases before v0.80.0 are in [docs/CHANGELOG-archive.md](docs/CHANGELOG-archive.md).

# Changelog

Changes by release, following [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### General

- Set one personal session age limit for Claude Code, Gemini CLI, and Qoder CLI with `session-retention-days` and `sync --global` (#2127).
- Keep private handoffs and Git snapshots across worktrees; tools can write them, and `handoff list` shows task state and local merge suggestions (#2117).
- Share personal memory across worktrees; mode changes copy facts into an empty store, and new projects can enable it with `init --memory` (#2104, #2109).
- Sync keeps the facts Claude Code saved before it moved its memory; for a folder `lint` flags, run `agnostic-ai memory import` (#2100).
- Flag hidden characters in memory files with `memory lint` (LINT044), and leave them out of the memory the session-start hook loads (#2105).

### By tool

#### Augment

- Auggie CLI on Unix loads shared memory through an executable session-start script; the IDE keeps the memory rule (#2106).

#### Cline

- Add a project memory TaskStart hook for VS Code, native Windows scripts, and explicit Unix VS Code diagnostics (#2106).
- Write global hook event scripts for the SDK and CLI under `~/.cline/hooks/`; the VS Code extension keeps its separate Documents folder (#2126).

#### Claude Code

- Claude Code loads personal memory natively; subagents read the same store, with an import fallback when native loading is off (#2116).

#### Crush

- Preserve MCP channel opt-in and automatic direct and group replies through import and sync (#2086).

#### Antigravity

- Sync warns when included project and checkout personal memory indexes push the built-in rule over 24,000 bytes (#2157).

- Load project and checkout personal memory indexes through an always-on native include rule; repo personal memory keeps its lookup fallback (#2108).
- Sync writes `SessionStart` hooks, and `hook run` accepts `auto_approve` replies with native permission limits (#2124, #2125).
- `hook run` excludes policy-dependent `allow` replies from Antigravity expectation checks (#2125).

#### Qoder

- Import project MCP servers from `.qoder/settings.json`, including JSONC, so fresh checkouts can recreate them (#2087).

#### Trae

- Load the shared memory index when a project session starts and project hooks are enabled (#2106).

#### Copilot

- Send the shared memory index under the reply key VS Code agent sessions read, as well as the one Copilot CLI reads (#2101).

#### Kiro

- Kiro loads project memory and checkout personal memory through a steering include, so you no longer add the include by hand (#2108).

#### Gemini CLI

- With the `memory` built-in, Gemini CLI saves team facts to `.agnostic-ai/memory/` instead of the generated `GEMINI.md`, which sync overwrites (#2103).

#### Windsurf / Devin

- Load shared memory through a Devin CLI session-start hook with an explicit POSIX launcher and timeout; retain the rule fallback (#2106).
- Explain Devin's discovery of shared agent profiles without assuming a matching Devin copy or promising that target scoping prevents discovery (#2084).

### Site

- The shared memory rule now says a tool's own memory can stay on, and the docs list each tool's own memory and default (#2102).
- Clarify setup, daily use, tool support and contributor guides; include every user guide in full plain-text docs for coding agents (#2083).

## v0.83.0 - 2026-10-11

### General

- Rank startup context and compare hooks, MCP servers and permissions across tools (#2017, #1985, #1986, #1987).
- Run hooks with the project binary, use locked installs, and explain failed checks and timeouts (#2012, #2046, #2018).
- Scaffold all ten spec kinds, plan imported skill changes, and check project targets and text skill links (#1990, #2013, #2011, #2060).
- Hide credentials in previews and permissions; verify build downloads and npm releases, and update unsafe dependencies (#2021, #2044, #2022, #2016).
- Keep sync JSON valid, skip unchanged skill links, and reduce staged-check time and typo-check memory (#2079, #2080, #2020, #2055).

### By tool

#### Claude Code

- Handoff hooks use the active checkout; refresh setup in linked worktrees and safely remove clean ones (#2068, #2014, #2054).
- Preview RTK command rewrites and declared approval changes with `doctor rtk --command` (#2015).
- Explain instructions for a file with `explain --file <path> --target claude`, including scoped rules and session limits (#1988).
- Explain when a home skill takes precedence over a project skill with the same name, even when sync warnings are silenced (#2069).

#### OpenCode

- Import V2 MCP servers, disabled states and separate timeouts without losing servers added by hand (#2063).
- Run opt-in V2 tool hooks on macOS and Linux; Windows requires V1; set `x-opencode.plugin-api: v2` per hook (#2062).
- Preserve manual-only skills and native invocation metadata through sync and import; V2 enforces the policy (#2064).

#### Copilot

- Sync and import user hooks across projects; keep handwritten handlers without duplicate execution (#2065).

#### Codex

- Explain instructions for a file with `explain --file <path> --target codex`, including planned outputs and launch directory limits (#1989).

#### Amp

- Explain when a home skill takes precedence over a project skill with the same name, even when sync warnings are silenced (#2069).

#### VS Code

- Editors report source and load errors, clear fixed diagnostics, explain failed checks, and keep checks current (#1981, #1982, #1983, #1984).

#### Cline

- Keep tool names in hook overrides as literal data, including quotes and newlines, so they cannot add shell commands (#2019).

### Site

- Check binaries, generated output and skill links before starting a cached environment with the project readiness checklist (#2052).

## v0.82.0 - 2026-10-10

### General

- Interactive commands offer upgrades with Yes as the default, release notes, project migrations, and sync checks (#1952).
- Enable RTK and Caveman independently with `builtins`; both stay off by default and no programs are installed (#1964).
- Sync preserves JSON, hooks and ignore files; import avoids text loss; memory notes shrink and warnings name tools (#1889, #1923, #1943, #1934, #1935, #1937).
- Commands run faster at 500 specs, `graph --target` shows only that tool, and `doctor` lists tools in name order (#1905, #1913, #1914, #1953, #1956).
- Release builds avoid an HTTP/2 crash and omit the build machine's source paths (#1912, #1926).

### By tool

#### Claude Code

- Block supported hook failures with `failClosed: true`; sync names stop, completion, and background hooks where it has no effect (#1918, #1975).
- `import claude` preserves each hook's settings and `onFailure` values, so sync no longer adds duplicate hook groups (#1921, #1929).
- `doctor` names hook specs that cause duplicate runs; delete the named spec, then run `sync` and `import claude` (#1924, #1925).

#### Codex

- **Breaking:** Codex applies and validates `outputs.codex.config.sandbox`; set `read-only`, `workspace-write`, or `danger-full-access` (#1884).
- Sync shows how to give Codex access to repo memory when you keep a hand-written config (#1949).
- `import codex` keeps manual skill sources intact when their generated files are imported again (#1948).

#### Kiro

- `sync --global` writes CLI V3 hooks to `~/.kiro/hooks/`; `import --global kiro` adopts hook files a spec can preserve (#1976).
- Portable `on: before-tool` hooks select built-in tool IDs or one MCP server; other portable events remain unmapped (#1980).
- Test CLI V3 prompt and Stop exits, category tags, wildcards, and supported MCP selectors with `hook run` (#1974).

#### OpenCode

- OpenCode saves personal memory without asking each time with `memory.personal: repo` (#1884).
- Moving `AGNOSTIC_AI_HOME` removes the old personal memory path from OpenCode's instructions (#1888).

#### Trae

- Empty agent tool lists disable all tools, including after import and sync (#1954).
- `import trae` reads shared `.agents/skills/` folders and assets; `.trae/skills/` takes priority for matching names (#1955).

#### Windsurf / Devin CLI

- Let Devin CLI save personal memory without asking each time with `memory.personal: repo` (#1884).

#### Kilo Code

- A command named `goal` is accepted with a warning that Kilo offers it as `/goal:command` (#1917).

#### Warp

- `sync --global` writes MCP servers to `~/.warp/.mcp.json`; `import --global warp` reads them back (#1978).

#### Antigravity

- `sync --global` writes MCP servers to `~/.gemini/config/mcp_config.json`; `import --global antigravity` reads them back (#1977).

### Site

- RTK and Caveman guides separate cost, speed and style; setup, approvals and measured results retain failed checks (#1963, #1959, #1960, #1961).
- Memory and project setup guides use plain words and explain when tools load shared memory (#1933, #1951).
- Kiro docs name current tool categories and mark `keyboardShortcut` as CLI 2.x only (#1979).

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

+++
title = "Inspect and preview"
description = "See where a spec goes, compare tools, preview a spec, and trace a generated file."
weight = 40
aliases = ["/docs/trace/", "/docs/why/", "/docs/graph/"]

[extra]
nav_title = "Inspect and preview"
group = "Reference"
+++

# Inspect and preview

Start with the item you know: `explain <spec>` finds a spec's output, `why <file>` finds a generated file's source, and `render <spec>` previews file contents. Use `compare` before changing tools and `graph` for an overview.

## explain

List the output files and sections a spec contributes to, using the `<!-- source: ... -->` markers in merged documents. With `--file`, list instructions that apply to a source file.

Pass an [error code](@/docs/errors.md) such as `AAI-003` or a [lint code](@/docs/cli-reference/check.md#lint) such as `LINT011` to see its cause and fix. Lint codes also show severity and any related `lint` config key (`severity` and `config` in JSON).

```bash
agnostic-ai explain rules/conventional-commits.md --json
agnostic-ai explain builtin:handoff --json
```

| Flag | Description |
|------|-------------|
| `--json` | Stable JSON for editor extensions and scripts. |
| `--layers` | Show which project source layers replace or extend a spec. See [below](#debug-a-replaced-spec). |
| `--global` | Explain a spec in `$AGNOSTIC_AI_HOME` (default `~/.agnostic-ai`) or its `local/` layer. A relative path resolves against that root. |
| `--file <path>` | Explain a source file. See [below](#explain-a-source-file). |
| `--context` | Rank startup context. See [below](#rank-context-contributions). |
| `--inputs` | List generator inputs. See [below](#list-generator-inputs). |

Contributions are grouped by configured target, each tagged `(full file)` or `(section "<name>")`, plus a "would emit if enabled" list for other tools. A tool outside `targets` that cannot render your setup is left out; a failure from a tool in `targets` stops the command and names it once. With `--global`, an agent or skill is a whole file, a rule a section of the instructions file, a hook its event in a shared hooks file or its standalone hook files, an MCP server its key in the user MCP file, and a settings spec each key it sets, tagged `(key "<key>")`.

Directory-based global hooks (Cline, Grok, and Kiro) also list copied command scripts, using the same paths and target root settings as `sync --global`.

A spec that sets `model` or `effort` also lists its [tier](@/docs/spec-format/agents.md#model-tiers) and the model and effort it resolves to per target; `tool default` means the target writes no model. A target with no key for it, such as Codex skills, drops it and `sync` prints a coverage note.

For an agent's `can` or `tools`, and settings `permissions`, it lists each target's native names, unsupported rules included. A widening line names extra access a target grants (for Codex, from the merged permissions and any explicit command policy source); `explain` shows it even with `on-unsupported: error`, where `sync` fails. Tool-specific overrides show their values and the field that wins.

<details>
<summary>Read explain JSON</summary>

```json
{"version": "1", "command": "explain", "spec": {"kind": "rule", "name": "...", "path": "..."},
 "contributions": [{"target": "...", "path": "...", "section": "...", "mode": "full|section|key"}],
 "would_emit_if_enabled": [],
 "model_tier": "strong", "models": [{"target": "...", "model": "...", "effort": "..."}]}
```

`model_tier` and `models` are omitted when the spec sets neither `model` nor `effort`. The `capabilities` array, omitted for specs without capability fields, holds `target`, `field`, `capability`, `native`, and `supported`; `widening` lists extra access when present, and `override` names a native field or policy source that replaces the portable rule.

</details>

### Claude skill overrides

For a skill sync writes for Claude Code, `explain` lists each Claude settings file whose `skillOverrides` names the skill, highest precedence first. It reads the same files and applies the same rules as [`doctor`](@/docs/cli-reference/check.md#claude-skill-overrides), including how rejected values, duplicate keys, and files that are not strict JSON show. A line that does not take effect is marked, so the first unmarked line is the value Claude Code applies:

```text
claude overrides:
  .claude/settings.local.json: off (Claude Code refuses to run it; change it in /skills)
  ~/.claude/settings.json: name-only (not applied: a higher file sets it)
```

The block is omitted when no file names the skill and every file reads. `--json` adds the same entries under `overrides`, with `rejected`, `conflict`, `uncertain`, or `problem` as in doctor:

```json
"overrides": [{"target": "claude", "path": ".claude/settings.local.json", "scope": "local", "value": "off", "effective": true}]
```

### Debug a replaced spec

When editing a pack rule has no effect, inspect its source layers:

```bash
agnostic-ai explain rules/conventional-commits.md --layers
```

```text
rule conventional-commits
  .agnostic-ai/packs/team/rules/conventional-commits.md [pack:team] base (replaced)
  rules/conventional-commits.md [project] replace (contributor)
  .agnostic-ai/local/rules/conventional-commits.md [project-user] extend (contributor)
effective source: .agnostic-ai/local/rules/conventional-commits.md
```

Edit either contributor to change the effective spec. Passing the replaced pack path gives the same report. Sources follow sync's order: enabled built-ins, installed packs, project specs, then project-local overrides. `base` starts a spec, `replace` discards the earlier chain, and `extend` merges with it. `contributor` means the source survives resolution; it does not attribute fields or body text, which an extension can replace.

With `--json`, `spec` is the effective source and `sources` lists each candidate's `source`, `operation`, and final `role`. Built-ins show a symbolic `builtin` name and version, not a cache path. The view reads installed sources offline, supports project specs only, and cannot combine with the other explain views or `--target`.

### Explain a source file

List the instructions planned for one project file, with their source, output path, file or directory condition, and reason. Claude Code, Codex, and Cursor are supported.

```bash
agnostic-ai explain --file services/payments/handler.go --target cursor
```

| Flag | Description |
|------|-------------|
| `--file <path>` | Project file to inspect. It does not have to exist. Cannot combine with a spec or code argument. |
| `--target <name>` | Required with `--file`. A configured target: `cursor`, `claude`, or `codex`. |

The report describes planned sync output. It cannot confirm what the model loaded. It does not see user instructions, the session launch directory, import approval, or session settings that turn off project instructions.

<details>
<summary>Read instruction statuses, native discovery rules, and JSON</summary>

| Status | Meaning |
|--------|---------|
| `always` | No file condition: Cursor's `alwaysApply: true` or root `AGENTS.md`; Claude Code's project `CLAUDE.md` or rules without `paths`; Codex's root native instruction file. |
| `match` | A file pattern or a nested instruction directory covers the file. |
| `no-match` | A selector misses the file, or a planned instruction filename takes priority over this output. |
| `model-selected` | `alwaysApply: false` with a description and no globs. Cursor's agent decides. |
| `manual` | `alwaysApply: false` with neither. Loads only when `@`-mentioned. |
| `unknown` | The command cannot establish discovery or evaluate the selector. The reason names the missing information. |
| `excluded` | Target selection (`target`, `targets`, `target-exclude`) leaves the target out. |
| `not-emitted` | The rule selects the target but sync writes no instruction for it. |

| Tool | Discovery the report models |
|------|-----------------------------|
| Cursor | A root `AGENTS.md` written for another tool reaches Cursor too. Braces, character classes, negation, or unreadable frontmatter report `unknown`. |
| Codex | Root `AGENTS.md` and `AGENTS.override.md` have no file condition; in one directory the override wins. Nested instructions report `unknown` and say whether the file is under their directory, since [Codex discovers](https://learn.chatgpt.com/docs/agent-configuration/agents-md) from the root to the session launch directory. Custom filenames report `unknown` because they need a fallback filename or session setting. |
| Claude Code | Reads generated `paths`, including tool-specific overrides, and Markdown rules under `.claude/rules/`. A nested `CLAUDE.md` covers files beneath it; elsewhere it depends on the session start directory, so `unknown`. Imports and outputs outside native discovery that need session approval or settings, and patterns it cannot evaluate (braces, character classes), report `unknown` with the reason. See [Claude Code's instruction discovery](https://code.claude.com/docs/en/memory). |

```json
{"version": "1", "command": "explain", "file": "...", "target": "cursor", "note": "...",
 "instructions": [{"status": "match", "source": "...", "output": "...", "selector": "...", "reason": "..."}]}
```

</details>

### Rank context contributions

`--context --target <name>` ranks the sources that add the most text at session start, counting words like `lint` does, with bytes. It needs a configured target and takes no spec argument.

```bash
agnostic-ai explain --context --target claude --json
agnostic-ai explain --context --target cursor --file src/main.go
```

Counts are estimates from planned output and spec text, not model tokens; runtime context, external imports, user-owned files, and host truncation are not counted. Inlined imports keep their source paths; unresolved ones are not read.

The report ends with opportunities: paragraphs of at least eight words written more than once in the specs that load at startup for that target, byte for byte after trimming. Each lists its copies, source files, size per copy, and the minimum words and bytes one session start would drop. A spec that two generated files both carry counts once, because the fix is the tool setting that reads both files, not the spec. Similar wording is not reported, because only you can tell if two instructions mean the same. Review the copies and edit the canonical specs, then run the command again to see the new counts.

<details>
<summary>Read context accounting groups and JSON</summary>

Entry-point layers, marked rule and review sections, inlined imports, always-on rules, each skill and agent description, and generated framing count separately, so the startup total matches lint. Sources sharing a document count once. A root `AGENTS.md` another tool writes counts for Cursor, and for Copilot when VS Code reads it.

Skill and agent bodies and conditional rule bodies appear under `on-demand`, outside the startup total, with their own `on_demand` sum. With `--file` for Cursor or Claude Code, matching scoped rule bodies appear under `file-scope` with their own total; unmatched or uncertain ones stay on demand. Entries sort by group, then words and bytes (descending), then source path and category. Text and JSON use the same numbers.

```json
{"version": "1", "command": "explain", "target": "claude", "note": "...",
 "startup": {"words": 80, "bytes": 500}, "file_scope": {"words": 0, "bytes": 0},
 "on_demand": {"words": 300, "bytes": 2000},
 "contributions": [{"source": ".agnostic-ai/skills/review/SKILL.md",
 "category": "skill discovery", "load": "startup", "words": 40, "bytes": 220}],
 "opportunities": [{"kind": "duplicate-passage", "target": "claude", "load": "startup",
 "passage": "...", "copies": 2, "size": {"words": 14, "bytes": 74},
 "reduction": {"words": 14, "bytes": 74},
 "sources": [{"source": ".agnostic-ai/AGNOSTIC_AI.md", "category": "entry-point layer"}],
 "suggestion": "..."}]}
```

`file` is present when requested. `load` is `startup`, `file-scope`, or `on-demand`. `on_demand` and `opportunities` were added in version `1` without changing other fields; `opportunities` is an empty array when there are none, largest `reduction` first. `size` is one copy; `reduction` is the other copies.

</details>

### List generator inputs

```bash
agnostic-ai explain --inputs
```

List the files and directories that can affect generated output, one per line relative to the repository root (`--json` for an array). Takes no spec argument or `--file`. Used by [git hooks](@/docs/git-hooks.md#check-staged-files). It lists the config files, `.agnostic-ai/**`, source directories outside `.agnostic-ai/`, files reviews pull in with `@path` (and the entry point's, with `sync.resolve-imports: inline`), `agnostic.packs.lock`, `.gitignore`, and `builtin:<name>@<content-hash>` for each enabled built-in.

## compare

Compare how two built-in targets handle the project's agent and skill fields, rule activation, hooks, MCP connections, and portable permissions, before you switch or add a tool. The [compare targets](@/docs/compare.md) page shows sample paths for any pair.

```bash
agnostic-ai compare claude cursor
```

| Flag | Description |
|------|-------------|
| `--json` | Stable JSON for scripts. |

Each field gets one result per target, with the output paths or the reason, and a `next:` step when known, such as a required `outputs.zed.tasks-file`. `(differs)` marks a field whose results differ. A spec whose target filter skips both tools is left out, except settings. A built-in spec shows as `builtin:<name>`, and its next step is the `builtins` setting or a project spec with the same name.

| Result | Meaning |
|---|---|
| `preserved` | Written under the same key with the same values. This describes the file, not that the tools behave the same, a hook ran, or an MCP server connects. |
| `translated` | Written under another key or file, with rewritten values, or only in part. |
| `unsupported` | The target has no place for the field or the kind. A field the tool ignores keeps that reason even when its native key is written. |
| `excluded` | The spec never reaches the target: a target filter, an opt-in output, or a scope the target cannot express. |
| `unknown` | The written files do not show either way. |

The command fails on unknown targets, the same target twice, invalid specs or config, and external adapters.

<details>
<summary>Read compared fields, permission mappings, and JSON</summary>

It covers agent and skill fields; rule `scope`, `paths`, `globs`, and `alwaysApply`; hook `on`, `match`, `event`, `matcher`, `command`, `args`, `timeout`, and `failClosed`, using current event mappings and written handlers; MCP transport (`type`), `command`, `args`, `url`, and each `env` and `headers` entry; and portable settings permissions one source rule at a time (allow, ask, deny, default-mode). Other settings, kinds, and hook fields are out of scope. Skill fields such as `argument-hint`, `effort`, and `disable-model-invocation` are judged from the files each adapter writes, including Codex's separate policy files.

MCP results never show command, argument, URL, environment, or header values, defaults, or reference text. A server whose URL or argument reference the tool cannot represent may be omitted whole; no explicit transport gives `unknown`.

Permission rows use the source index, such as `permissions.deny[0]`, name the rule, and show native override values and partial mappings. A source or native rule that holds a credential shows as its tool name around `<redacted>`, the marker `sync --global` prints, such as `shell(<redacted>)` or `prefix_rule(<redacted>)`; in an `x-<target>` override, only the command pattern or string that holds it is replaced. Recognized credentials in complete native permission records, such as Augment webhook policies, are hidden in text and JSON summaries but compared as written; filename patterns and ordinary permission actions stay visible. Codex shell permissions need `outputs.codex.exec-policies-from-permissions: true`. Default-mode applies only to global sync, so project comparison reports it unsupported. A difference in effective decision or partial support counts, even when both results are translated.

```json
{"version": "1", "command": "compare", "targets": ["claude", "cursor"], "coverage": "...", "caveat": "...",
 "specs": [{"kind": "agent", "name": "...", "path": "...", "fields": [{"field": "tools", "differs": true,
   "results": [{"target": "cursor", "status": "unsupported", "reason": "...", "next": "..."}]}]}],
 "fields": 6, "differences": 3}
```

A built-in spec has an empty `path` and a `"builtin": {"name", "version"}` object instead.

</details>

## render

Print what each target would write for one spec.

```bash
agnostic-ai render rules/no-console-log.md --target claude,codex
```

| Flag | Description |
|------|-------------|
| `-t, --target <list>` | Targets to render, repeated or comma-separated. Default: all in `agnostic-ai.yaml`. |

Each file prints as `# target: <name>: <output path>` and its body. A target that writes nothing for that kind prints a note. Known credential values are hidden; see [credential previews](@/docs/cli-reference/sync.md#credential-previews).

## graph

Show which targets and files each spec feeds (spec → target → file). When a spec seems missing from a tool, run `agnostic-ai graph --spec <name>`: a `-` means that tool gets no file, so check the tool's supported kinds and the spec's target filters. Rows sort by spec name, then target. Without `targets`, graph covers every tool and leaves out any that cannot render your setup.

```text
$ agnostic-ai graph
spec           | claude cursor codex
code-reviewer  | agent  agent  agent
no-console-log | rule   rule   rule
```

`mermaid` starts with `graph LR`, `dot` pipes to Graphviz (`agnostic-ai graph --format dot | dot -Tsvg > graph.svg`), and `json` gives one record per file:

```json
{"spec": "no-console-log", "kind": "rule", "target": "claude", "path": ".claude/rules/no-console-log.md"}
```

| Flag | Description |
|------|-------------|
| `--format` | `text` (default, aligned table), `mermaid`, `dot`, `json`. |
| `--json` | Same as `--format json`. Fails with another `--format`. |
| `--target` | Restrict to one configured target. Any other name fails. |
| `--spec` | Restrict to one spec name. |
| `--kind` | Restrict to one kind: agent, skill, rule, hook, mcp, command, settings, review, environment, ignore. Any other kind fails. |

## why

Find the source to edit for a generated file. Use a project-relative path, even for a file not synced yet; symlinks resolve to the real path. In VS Code, `agnostic-ai: Open canonical source` does the same for the current file.

```sh
$ agnostic-ai why .cursor/rules/no-console-log.mdc
.cursor/rules/no-console-log.mdc
  adapter: cursor
  output keys: (adapter defaults)
  last sync: 2026-05-15T10:23:00Z
  sources:
    [rule] no-console-log (.agnostic-ai/rules/no-console-log.md): full
```

Edit the path under `sources`, then run `agnostic-ai sync`.

`why` can recognize files from tools outside your `targets` list. If one of those tools cannot generate files with your configuration, it skips that tool. A failure from a tool in `targets` stops the command and names that tool once.

| Field | Meaning |
|-------|---------|
| `adapter` | Tool that wrote the file. For a shared file such as `AGENTS.md`, the first tool that reads it. For a path several tools share (`.agents/skills/`), a tool listed in `targets`. If only a tool you did not list writes the path, it reads `adapter: amp (not configured)`. |
| `output keys` | Every `outputs.<target>.*` key whose value appears in the path, or `(adapter defaults)`. |
| `last sync` | UTC time from `.agnostic-ai/.sync-state`, or `unknown` when that file is missing. |
| `configured` | JSON only. `true` when the tool is listed in `targets`. |
| `sources` | Every spec that feeds the file: `full` when the spec owns the file, `section` when it is one of several merged into a shared file. Root instruction files list `.agnostic-ai/AGNOSTIC_AI.md` first, then `.agnostic-ai/local/AGNOSTIC_AI.md` when present, then each [inlined rule](@/docs/target-behavior.md#inlined-rules) as a `section`. |

Without `.agnostic-ai/.sync-state`, run `sync` first. A spec path points you to `explain`, and `AGNOSTIC_AI.md` names the entry-point files it feeds; a path sync does not write reports "not synced or not tracked".

| Flag | Description |
|------|-------------|
| `--format` | `text` (default) or `json`. |
| `--json` | Same as `--format json`. Fails with another `--format`. |

## view

Open the bundled web interface in your browser; `ui` is an alias. From a project folder or any folder inside it, `view` reads that project. Outside a project, it reads the user configuration in `$AGNOSTIC_AI_HOME` (default `~/.agnostic-ai`). Keep the terminal running; Ctrl-C stops it. See the [browser walkthrough](@/docs/inspect-configuration.md).

```bash
agnostic-ai ui
agnostic-ai view --json
agnostic-ai view --global
```

| Flag | Description |
|------|-------------|
| `--json` | Print the overview as JSON (schema `overview/v1`) without starting a server or opening a browser. |
| `--dir` | Read this project directory instead of discovering one. |
| `--addr` | Explicit loopback IP and port. Default `127.0.0.1:4310`. When set, `view` stays on this port and fails if another configuration serves it. |
| `--global` | Read the user configuration instead of a project. Cannot combine with `--dir`. |

A second run for the same project and scope opens the running server and exits. When the default port serves another project or scope, an `--api-only` server, or another program, `view` takes the next free loopback port up to `127.0.0.1:4319` and prints it; later runs find that server. An invalid project configuration reports an error.

The overview lists context, authority, tools, lifecycle, and settings, with findings for the selected targets; the JSON overview also compares generated files. Specs scoped only to other targets are left out of lists and counts. Search within a section to find a source file or value, or press **Search** (`Cmd+K` or `Ctrl+K`) to search every section, findings included. Opening a result puts a link to that entry in the address bar; it survives a reload. The browser has light and dark themes and works on narrow screens. Other browser features are under [serve](#serve).

## serve

Serve the bundled web interface and a local JSON API. It works offline without a source checkout or frontend build, and reloads configuration on every request.

```bash
agnostic-ai serve --global
agnostic-ai serve --api-only
```

| Flag | Description |
|------|-------------|
| `--addr` | Explicit loopback IP and port. Default `127.0.0.1:4310`. |
| `--allow-origin` | Additional exact browser origin, such as `http://127.0.0.1:5173` for Vite. The server accepts loopback hosts and same-origin requests otherwise. |
| `--dir` | Project directory to read. Default: the current directory. |
| `--ui` | Serve a built client directory, such as `ui/dist`, instead of the bundled one. |
| `--api-only` | Serve only the JSON API. Cannot combine with `--ui` or `--open`. |
| `--open` | Open the served URL in a browser. |
| `--global` | Serve the user configuration. Its overview shows `lint --global` findings and drift against what `sync --global` would write; doctor checks are skipped. |
| `--no-installed` | Skip comparing generated files with disk. |
| `--cloud-sessions` | Also list Claude Code cloud sessions, the ones at claude.ai/code, and Codex cloud threads. Needs `--global`. Off by default: without it nothing reads your sign-in or leaves this machine. See [cloud sessions](#claude-code-cloud-sessions). |

In the browser:

- **Docs** beside each section heading opens the matching spec or reference page in a new tab, including empty sections.
- **Authority** lists permission files and rules. **Add rules** or **Edit rules** opens an editor for Allow, Ask first, or Always blocked; preview the source-file change before saving. Protected paths are editable there too, and the default permission mode in the global view. See [permission editing](#permission-editing).
- **Tools** lists the MCP server specs.
  - In `view --json`, an MCP item whose spec sets `disabled` carries it in `fields`, and a local override's value wins. A turned-off server is left out of the verdict and rail counts and shown as off.
  - Under **Found in tools**, it also lists servers a configured tool keeps in its own MCP file that no spec of the same name covers, such as one you signed in to from inside Claude Code.
  - Each native row names the server, the tool or tools that hold it, the file (`~/.claude.json`, `.mcp.json`), and the transport. It also shows the `agnostic-ai import <tool>` command that turns the server into a spec (`import --global <tool>` for a user file).
  - A project page reads each tool's project and user files, plus Claude Code's servers for this project under `projects` in `~/.claude.json`. `--global` reads user files only.
  - A native row has no command when no import reads its file, such as Claude Code's per-project servers or an `outputs` MCP file path the tool's import does not follow.
  - Native rows hide the server's own command, arguments, URL, environment, and header values. A file the page cannot read adds no row.
  - The JSON overview carries these rows as `native` on the Tools section.
- **View and edit**, shown once beside the filter on Authority and Settings, switches between Project (Global in the global view) and Local values. **Save new servers for** above the native server list chooses Project or Local for **Add to your setup**.
- **Add to your setup** on a native-only server previews one spec file, its contents, the variables you must set, and the tools it will reach after sync.
- **Apply** saves the spec using the [MCP import rules](@/docs/spec-format/mcp-import.md), then asks you to run `agnostic-ai sync` (or `sync --global`).
- Project and Global saves use the project's configured MCP source folder (`.agnostic-ai/mcps/` by default), or `mcps/` in the global home. Local saves use `.agnostic-ai/local/mcps/`, or `local/mcps/` in the home.
- An existing spec name or a credential import cannot rewrite stops the save.
- Tool files stay as they are, including the original server. Each tool signs in on its own. Snapshots have no **Add to your setup** action.
- **Add hook**, at the end of the Lifecycle toolbar, opens a form: name, stage, tool match (for Before a tool runs and After a tool runs only), command, timeout in seconds, description, and **Save for** (Project or Global, or Local). **Preview** shows the one hook file it writes, in the [portable form](@/docs/spec-format/hook-reference.md#portable-events) with only the fields you filled. **Apply** saves it and asks you to run `agnostic-ai sync` (or `sync --global`).
  - Project and Global saves use the project's configured hook source folder (`.agnostic-ai/hooks/` by default), or `hooks/` in the global home. Local saves use `.agnostic-ai/local/hooks/`, or `local/hooks/` in the home, with the same Git-ignore check as other local saves.
  - A name that is taken, in any layer and in any letter case, stops the save, and so does a hook that `validate` would report, such as a tool match a configured tool lacks.
  - The form cannot edit or delete hooks, or write a tool's own event name. Snapshots and the mock have no **Add hook** action.
- **Copy to global** on a project rule, agent, skill, hook, or MCP spec (**Copy to project** in a global view started inside a project) shows the destination file and whether a spec is already there. **Copy** keeps the original; **Move** also deletes it. A skill goes with its whole folder. When a spec is already there, both stay off until you tick **Replace it**, which replaces the whole spec. After saving, the page reloads and names the sync commands to run, such as `agnostic-ai sync` and `agnostic-ai sync --global` after a move.
  - Built-ins, pack specs, and local specs have no such action. Commands, review instructions, and settings stay too, since the global setup does not read them as specs. A rule with a scope, paths, or a target condition cannot go to the global setup, where rules apply to every file.
  - When the global setup does not exist yet, create it with **Open global settings** first.
- **Conversations** (`/`) is the page the interface opens on, except a static snapshot, which has no server to read chats and opens on **Context**. It lists recent Claude Code, Codex and Cursor chats from each tool's own store: open ones first under **Open now**, with **Working** or **Waiting for you** when Claude Code reports it, then by day, newest last message first. It loads the past week, and the week before each time you scroll to the end, or with **Load the week before**. The tool filter beside the search shows one tool's chats; search covers every conversation, loaded or not. Each row's **Open in** action reopens the chat where it lives: its Terminal tab, a new Terminal window, the editor panel (after opening its folder there), the Cursor chat, or the desktop app. The first time, macOS asks to let agnostic-ai control Terminal. Codex and Cursor chats need the `sqlite3` command. See [`/api/views/conversations`](#api).
- The rail on wide screens labels the page **Project** or **Global** and shows its path under the name; on phones the top bar shows the name and **Menu** shows the path.
- The foot of the rail (in **Menu** on phones) names the agnostic-ai version that serves the page. When the config sets [`requires`](@/docs/configuration.md#requires), the line under it names that range, in the warning color when the running release is outside it, and the rail then shows **Update needed** beside the version.
- **Overview JSON** at the foot of the rail (in **Menu** on phones) opens the overview document behind the page, with its endpoint, schema version, revision, and the command that prints it (`agnostic-ai view --json`). `Esc` closes it.
- **agnostic-ai on GitHub** at the foot of the rail (in **Menu** on phones) opens the agnostic-ai repository in a new tab.
- When the project's `origin` remote points at GitHub, a GitHub button at the foot of the rail (in **Menu** on phones) opens that repository in a new tab. It reads SSH and HTTPS remotes and drops any user name or token in the remote URL. Projects with no remote or another host, and the user configuration, show no button.
- On a project page, **Open global settings** at the foot of the rail (in **Menu** on phones) opens the user configuration in a new tab. It reuses a `serve --global` already running on `127.0.0.1:4310` to `4319` that started in this project, or starts one on the next free port.
- When the user configuration folder does not exist or holds no agnostic-ai files (`agnostic-ai.yaml`, `AGNOSTIC_AI.md`, a spec folder, `local/` or `state/`), the button shows a short panel instead. **Create global setup** writes an empty `AGNOSTIC_AI.md` and `agnostic-ai.yaml`, makes sure `.gitignore` lists `/local/` (an existing one is kept), runs `git init` when the box is checked, git is installed and the folder is not already in a repository, and then opens the global page. It never touches a folder that already holds a setup.
- A global page that started in a project (from that button, or `ui --global` or `serve --global` run in a project folder) shows **Open project** with the project's name to go back.
- A server the button starts stops with the server that started it. The way back from that global page returns to that server, also when it runs on another `--addr`.
- Each browser tab edits only the configuration it serves.
- **Settings > Editor** picks which editor Open links use: Cursor, VS Code, VS Code Insiders, VSCodium, Zed, or a custom link template with `{path}` and `{line}`. The choice is saved in the browser, not in config files. Allow the browser to launch the editor when prompted; if nothing opens, change the editor or copy the path. A finding on a file opens at its line when known. **Copy path** sits beside each link, because the browser cannot tell whether the editor opened.
- All page links stay visible on narrow screens.
- A page that cannot load shows **Could not load the overview**, or **Could not load the snapshot** for a snapshot, with **Retry**.
- **Context**, **Tools**, and **Settings** use tabs for their source groups. Tools separates MCP servers, servers found in tools, and environments. Settings separates built-ins, coding tools, preferences, config files, and the editor. Config files lists the source files, then shows their combined effective values once.
- Tabs support arrow, Home, and End keys. On phones, Context and Tools tabs scroll horizontally; Settings keeps its tabs visible: five on a running server, four in a snapshot or the mock, which have no Editor tab.
- The selected tab survives refresh and browser history. Switching tabs keeps unsaved input and previews.
- Search covers all source tabs and opens a matching group. Linked results open their owning tab.
- Authority keeps permissions together, and Lifecycle keeps its session timeline.
- With `--global`, the `AGNOSTIC_AI.md` row in **Context** has **Copy refine prompt**. The prompt asks a coding agent to review the file against current guidance for instruction files, propose a full rewrite with a change table, recommend which rules belong in an existing or new skill or agent, and leave the file unchanged until you approve.
- **Context size** opens first in **Context** for a project served by a running server. Static snapshots, user configuration, and projects with no coding tool open the first tab with content instead.

Choose a coding tool to see its [context report](#rank-context-contributions): startup, file-scoped, and on-demand sizes in words and bytes. **Check against a file** updates the file scope as you type. The page compares startup words with [`lint.instructions-words`](@/docs/configuration.md#lint).

**How this is counted** explains when text loads and links to the tool's model and effort in Settings. Sources are ranked by words, with each source's share of its group, followed by duplicate startup passages. The tool and file stay in the page address.

- **What can I do?** on a source says what it is, why it loads when it does, and what to change. **Copy prompt** copies a prompt that asks a coding agent to propose the smallest safe cut and its tradeoffs before editing. The page lists **Read at every start**, **Read for the named file** and **Read on demand** tables, with a words-at-startup bar and **Change budget**, which opens the budget docs. **Show content** on a row shows the file's frontmatter as chips and its text.
- A source from a built-in names the built-in and links to **Settings**, Built-ins.
- Each duplicate has edit links to its sources and **Copy prompt**.
- **Read again** reads your setup files again and shows the change against the previous counts under **Before**. The tool and file stay in the page address.

- Each finding has **Copy prompt**. **Details** holds **Open** in your editor, **Copy path** and **Recheck**, then the suggested fix, explanation, and full prompt.
- After you fix the source, **Recheck** runs the checks again with the same refresh and keeps your page, search, and unsaved input. It says whether that finding still appears and marks new findings from the same check on the same file. A failed refresh or a lint run that stopped early shows as unknown, never as fixed.
- A setup check finding (from `doctor`) that disappears is shown as not confirmed, because the setup check skips files it cannot read. Snapshots cannot recheck.
- **Setup health** in the rail shows findings by severity, such as **1 error, 3 warnings**, or **No findings**. On phones, the top bar shows a chip such as **1E 3W**. Either opens **Setup health** (`?view=health`).

The health page starts with what needs fixing and what needs review. Findings appear under **Fix first** and **Worth a look**, then by section and item, with the same rows and actions as elsewhere. Click an item or section name to open it. With two or more findings, **Copy all fixes** copies one prompt, errors first.

Select findings or suggestions with their checkboxes. A bar at the foot of the page shows the count, **Copy prompt**, and **Clear**. The copied prompt lists errors, warnings, then suggestions. **Recheck**, beside **Checked just now**, reruns every section's checks and reports findings that disappeared, appeared, or remain. Doctor and incomplete lint results use the limits described above.

**You could add** lists unused features your enabled tools support: a context kind with no project spec, a hook stage with no matching hook (built-in hooks count), or the disabled `handoff` built-in. Each suggestion shows its tool count and **Copy prompt**. A row of bars below the opening sentence shows supported features and which are in use, such as **7 of 9 features your tools support are in use**. When all are in use, no suggestions appear. Suggestions never count as findings, and findings do not lower feature usage. User configuration shows no suggestions.

- In a project, the counts and **Setup health** include what `lint --global` reports for the global setup (`~/.agnostic-ai`), which reaches every project. Each such finding is labelled **global**, sits under Settings as **Global setup**, and its fix prompt names the file in the global setup. Doctor checks read project files, so the global setup has none. Without a global setup there are none. The JSON overview marks them `"layer": "global"`, with `path` relative to the global source.
- With warnings only, a line under the page title counts them and **Review** opens the folded list at the end of the page. Errors stay visible above the tabs with their findings.
- Tool lists use logos, with the full name on hover. Source files appear in compact lists with paths and edit links.
- **Lifecycle** lists stages with hooks. **Show all stages** adds the rest. Events specific to one tool, such as `SubagentStart`, have their own heading and a description of what their hooks do.

Each stage starts closed, with its timing and hook count. Hooks with `disabled: true` are marked **Turned off** and counted separately. Open a stage to see its tools and hooks. **Expand all** and **Collapse all** control the visible stages. Hooks show their purpose and source; built-ins link to **Change in Settings**.

**Command** shows the command, hook name, and key fields (`match` or `matcher`, `type`, `timeout`, and `disabled`). Built-in launchers show the `agnostic-ai` call, with **Full command** for the script. **Copy command** is hidden when the command appears to contain a credential.

On a running server, **Full file** shows every source field, unless the file appears to contain a credential. A sensitive key field shows `<redacted>`. Built-in hooks have no source file and show only key fields.

- Search shows and opens the stages with matching hooks; clearing it closes them. A linked hook or finding opens its stage and receives focus. Open states are not saved.
- Native settings are grouped by tool, with the `x-<tool>` prefix shown once.
- **Settings** shows configured fields first and the rest under **Unconfigured fields**, including a default model or reasoning effort.
- Global scalar fields offer **Apply** and **Clear**. Session retention and voice dictation are editable only with `--global`. Voice has an on/off switch, a hold or tap menu, and a spoken language field.
- Default model and reasoning effort have **What each tool gets**: the value sync writes for each configured tool, and the file or `agnostic-ai.yaml` key it comes from (Claude Code `outputs.claude.settings.model`, Codex `outputs.codex.config`).
  - It also says why each other value loses: a later spec, a local override or clear, a spec scoped to other tools, or a per-tool map with no entry.
  - It names the model tier and alias, says when a tool has no place for the value, and says when sync writes none, without guessing the tool's own default.
  - It reports whether installed files match, not what a running tool uses.
  - It points out an `x-<tool>.model` native key. Other `x-<tool>` keys and imported native settings files are not covered.
  - The JSON overview carries it as `resolved` on those fields.
- Under **What each tool gets**, each tool has its own edit control. It saves `model.<tool>` or `effort.<tool>` in the chosen file, which wins over the value for every tool.
  - The origin line says where the value comes from, such as `local · codex` or `project · all tools` (`global · all tools` in the global view), and names the `agnostic-ai.yaml` key that replaces it when one does.
  - **Use the value for every tool** removes that tool's entry.
  - Effort has a control only for tools that take an effort setting. A spec whose `model` names a model tier stays read-only here, and so does one tool's effort when the tier gives the effort.
- In a project, each built-in switch previews the config edit, generated files, and findings for every target.
- **Apply** and **Cancel** stay at the top. Tools, files, and review notes start collapsed. Notes group by tool in a bounded list, with **Copy notes** for the full messages. Pending source changes remain visible.
- **Apply** writes the change. **Cancel** discards the preview, so a reload does not bring it back.
- **Undo last change** restores the files and permissions from before the last apply. Undo survives a browser reload, expires when the server stops, and is replaced by the next apply.
- Turning on `memory` in a Git checkout also adds a `.git/info/exclude` line so Git ignores Claude Code's private `settings.local.json`.
- In a project, a default model or reasoning effort uses **Preview**, **Preview removal**, and **Preview local reset**. The preview names the settings file, the targets, and every source and generated file that changes.
- **Apply** writes those files and checks the generated ones. That does not mean a running tool has reloaded them. Built-ins and settings share one **Undo last change**, which names the change.
- **Coding tools** in **Settings** lists tools with a switch, a **Docs** link, and a **found in project** badge when a disabled tool has native files. Snapshots and the mock show the list without switches.

A switch saves the complete `targets` list to `agnostic-ai.yaml` in Project or `agnostic-ai.local.yaml` in Local. The local list replaces the project list, as built-ins do. With `--global`, it saves `agnostic-ai.yaml` or `local/agnostic-ai.yaml` in the home folder and lists only tools global sync supports.

When no file lists `targets`, the first project change writes the default list with your change. Switching the tool back on removes that list only in the same `serve` session, with no intervening file edit. Otherwise, the file keeps the full list.

Turning a tool off asks for confirmation and explains the next sync. In a project, sync removes files it wrote for that tool, backs up hand edits as `.bak`, and keeps unrelated keys in shared settings files. With `--global`, files stay until you remove them. The last supported tool cannot be turned off; external adapter names do not count.

After saving, run the displayed `agnostic-ai sync` or `sync --global` command. The page does not sync tool selection. A running `sync --watch` applies it unless started with `--target`, `--only`, or `--except`. A stale switch stops with **Reload**.

- Global built-ins and settings edits save source files without syncing. Run `agnostic-ai sync` (or `sync --global`) to apply them.
- In the global view, if a boolean or built-in changed since the page loaded, the save stops and offers **Reload**.

Apply needs no earlier sync: the preview lists the generated files, sync state, and lock `.gitignore` entries it creates. It includes pending source changes when files are missing or match sync's recorded checksums, and undo can restore them. These need the CLI: hand edits, files without ownership records, external adapters, paths outside the project, linked skill folders, and memory changes that need fact migration or external storage. A changed input or output blocks apply or undo until a fresh preview. A failed write restores the files the operation owns; a recovery error names any it kept and points to `agnostic-ai doctor`.

Edits keep unrelated text and private file permissions. A validation failure restores the edit, unless another process changed the file; that is reported as a conflict.

### Save project or local changes {#edit-scope}

**View and edit**, beside the filter at the top of **Settings** and **Authority**, selects the values shown and where every editor on the page saves. It reads **Project | Local** in a project and **Global | Local** in the global view, matching the chip beside the project name. The browser remembers your choice. **Project**, the default, shows the project's built-in states and settings, and writes files Git tracks. **Local** shows your overrides, with values inherited from the project labeled, and writes git-ignored [local overrides](@/docs/local-overrides.md): built-ins go to `agnostic-ai.local.yaml`, and a settings field to `.agnostic-ai/local/settings/<name>.yaml`, which overlays the project spec of that name. In the global view **Global** writes `~/.agnostic-ai/`, and **Local** writes `local/agnostic-ai.yaml` and `local/settings/<name>.yaml` there. Outputs a local built-in adds go to `.git/info/exclude`, not the project's `.gitignore` block. The config snapshot and **What each tool gets** still describe the combined configuration used for sync.

A value from a local file shows a **local** badge in Local, and an item from either level's `local/` folder shows the layer `local`. Local values win for you; rows where they differ show both values, such as **Project: Off · Local: On**. Built-in counts follow the selected view.

A project control blocked by a local override shows its project value, stays disabled, and points to **View local settings**. In Local, **Reset to the project value**, or **Preview local reset** for a project model or effort, removes your override and the local file once it is empty.

Your first local built-in change copies the project list into `agnostic-ai.local.yaml`, since that list replaces the project one whole. **Reset built-ins to the project list** drops it. A local list set through a YAML merge (`<<`) is refused; edit it by hand.

A local save is refused when its file resolves, through links, outside the local folder, into a project config or source folder, or to a file Git tracks.

Before a local save writes its file, the server checks that Git ignores it. When no rule does yet, as in a project that never ran `sync`, it adds the local folder (or `agnostic-ai.local.yaml`) to `.git/info/exclude`, which linked worktrees share. Your other lines in that file stay. If Git still would not ignore the file, the save is refused; add the path to `.gitignore` and save again. A new `.git/info/exclude` rule remains if it was added before a refusal or stale conflict, to protect local files. Outside a Git repository there is nothing to ignore, so the save writes the file directly.

In a linked worktree, a built-in only you turn on needs `agnostic-ai sync` in the CLI. The first local change keeps the `memory.personal` mode inherited from the main checkout, and a reset leaves `memory.personal: checkout` behind rather than switch to the main checkout's mode.

### Edit permission rules {#permission-editing}

In **Authority**, select Project or Local, then **Add rules** or **Edit rules** on Allow, Ask first, or Always blocked. Enter one [permission rule](@/docs/spec-format/settings.md#permission-rules) per line, such as `shell(git status)` or `read(src/**)`. Blank lines are ignored.

Each saved rule also has **Edit**, which opens the editor on that line, and **Remove**, which goes straight to the preview without that rule.

**Preview** shows the file, scope, and exact diff, with added and removed lines marked. **Apply** saves only that source file. Run the displayed `agnostic-ai sync` or `agnostic-ai sync --global` command to apply it.

The support table names which tools accept permissions.

**Default permission mode** is editable in the global view only, because only `sync --global` writes Claude Code's `defaultMode`. There it offers the mode names Claude Code accepts, and **Save** writes `permissions.default-mode` to the same settings file as the rule lists. A project shows it read-only with **Open global settings**.

**Protected paths** opens an editor with one path per line, a decision (`ask` or `deny`), and a reason. **Preview** shows the diff and **Apply** writes the whole [protected block](@/docs/spec-format/settings.md#protected-paths) to that file, replacing the block already there. **Remove protected paths** previews removing it. A local block starts from the project block merged under it, as sync reads them, and cannot lower a project `deny` to `ask`. A local file that sets `protected: null` turns the project paths off for you; Local says so and **Reset to project paths** removes the key, so the next sync protects them again. The global view keeps it read-only, because `sync --global` does not write protected paths.

A local edit starts with the project rules and replaces only the selected list for that spec; other settings keep inheriting. A changed local list offers **Compare project and local**. **Reset to the project rules** previews removing the override. An empty list is an explicit override with no rules; resetting removes it and inherits Project again. Project editing stays blocked while a local list replaces it.

If a local spec clears the whole group with `permissions: null`, individual lists stay read-only in both views. Project still shows its baseline; Local shows the clear and names the file to edit. Remove that whole-group clear in the local file before editing one list.

When several settings files are editable, choose one explicitly; lists from other specs still contribute their rules. With no settings spec, the editor creates `settings/permissions.yaml` under the configured source root, or `local/settings/permissions.yaml` for Local. Pack-only sources remain read-only.

Invalid rules, unsafe file paths, and credentials are refused. A changed file stops saving and offers **Reload**, which keeps your draft for a fresh preview. Permission saves have no browser undo.

### API

Edit and impact requests take JSON. Every edit and preview names its `audience`: `shared` or `personal`. Except for `/api/edit/mcp`, `/api/edit/hook`, `/api/edit/permissions`, and `/api/edit/protected`, the older `scope` field (`team` or `local`) is accepted until the next minor release; a request that sends both with different meanings returns 400. A change plan carries both `audience` and `scope` until then. A boolean edit must include its `enabled` or `value` field, even when false. Invalid values and YAML that cannot be spliced safely are refused without changing the file. Previews hide recognized credentials using the [credential preview rules](@/docs/cli-reference/sync.md#credential-previews), and run no MCP servers, hooks, or external adapters.

| Endpoint | Body or query |
|----------|---------------|
| `GET /api/health` | None. Returns `ok`, absolute `root`, `global`, `ui` (whether the web interface is served), and `version` (as `agnostic-ai --version` prints it), without loading specs. A global server started in a project adds that project's absolute `projectRoot`. When the config sets [`requires`](@/docs/configuration.md#requires), it adds `requires` as written and `requiresMet`: `true` or `false`, or `null` for a build that is not a release. It reads them on each request. |
| `POST /api/switch` | `{}` with `Content-Type: application/json`. Finds or starts the server for the other configuration: the user configuration from a project, or `projectRoot` from a global server. Returns `url`, `root`, `global`, and `started`. Answers 404 on a global server with no `projectRoot`, 409 with `{error, missing, root}` and no server when the user configuration folder has no setup, and 503 with the command to run when no server could start. |
| `POST /api/global/create` | `{"git": true}` or `{"git": false}`, project servers only. Writes the starter files into the user configuration folder fixed at startup, and replies `{root, created, git}`. Answers 409 when a setup already exists. Any other field is a 400. |
| `GET /api/views/overview` | None. `global` is true for user configuration. `repository` is `{url, name}` when the project's `origin` remote is on GitHub: `url` is the repository's `https://github.com/<owner>/<repo>` page and `name` is `<owner>/<repo>`. Absent otherwise and for user configuration. `health.gaps` lists what the configured tools support and the project does not use yet, each `{id, section, label, supported, missing, fix}`: `id` is `kind:<kind>` (rule, agent, skill, command, review or ignore with no spec of the project's own), `stage:<stage>` (a hook stage with no hook) or `builtin:handoff` (off); `supported` lists the configured targets that would use it, and `fix` is a prompt a coding agent can run. Built-in specs fill no gap. Empty for user configuration. `health.improvements` is `{applied, total}`: `total` counts every kind, stage, and built-in the configured targets support, and each one not applied is a gap, so `total - applied` equals the number of gaps. Absent for user configuration. |
| `GET /api/views/context` | `?target=<tool>`, a configured target; default the first one. `&file=<path>` adds scoped rules for that file, as `--file` does; `fileTargets` lists the targets that accept it. Returns `targets`, `target`, absolute `root`, `startupWordBudget` (the `lint.instructions-words` value the page compares startup words with), and `report`, the `explain --context --json` output. With `--global`, `supported` is false and `reason` says why. |
| `GET /api/views/source` | `?path=<absolute path>`, the `openPath` of a row the Context or Lifecycle page lists: an instructions file, rule, agent, skill, command, review instructions, ignore file or hook of the served configuration, project or global, including `local/`. Returns `path`, `kind`, `frontmatter` (a list of `{key, value}` in file order), `body`, `bytes` (the file size), `truncated`, true when the body was cut at 256 KB, and `withheld`, true when the text is hidden: it may hold a credential (any credential-named key, or any of the words password, passphrase, secret, credential, API key, private key, access key, secret key, or client secret, even in guidance that only names them), or the file is too large to check whole (over 320 KB), and then `body` and `frontmatter` are empty. A leading frontmatter block is read whole, up to 64 KB more, so its values can be checked; one that does not close within that is withheld. A hook returns every field of its YAML file in `frontmatter` and an empty `body`, withheld by the same rule; a hook file that does not parse returns no fields. Any other path, or a file whose symlinks lead outside the source folders the configuration loads, returns 404. |
| `GET /api/views/conversations` | `?editor=<id>`, the editor chosen in Settings, so editor links use its scheme, `&before=<time>` for a later page, `&q=<words>` to search, and `&tool=claude`, `codex` or `cursor` to list one tool. A search keeps the conversations of every age whose title, folder, branch, latest prompt, tool or app, as its row shows them, holds each word, ignoring case; a withheld field does not match. Its pages hold only matches, and the page adds `matches`, the count across all pages. A `q` over 200 characters or another `tool` is a 400. Lists Claude Code, Codex and Cursor conversations from each tool's own store, newest first by the last message from the person or the agent, one week per page: the first page holds every open conversation and the past week, and a page asked for with `before` holds the week before that time, reaching past weeks with no conversation. The page returns `from` and `to`, and `next`, the `before` value of the following page, absent when no older conversation remains; a `before` that is not an RFC 3339 time is a 400. A project server lists the conversations started in the project or one of its Git worktrees; a global server lists them all. Runs a program started (Claude Code SDK sessions, `codex exec`) are left out. Returns `tools` (`{tool, label, found, count, shown, note?}`: `count` is every conversation the tool holds, `shown` how many this page holds, and `note` says why a store was not read) and `conversations`, each `{tool, app, id, title, folder?, branch?, startedAt?, updatedAt, opening?, prompt?, withheld?, live?, activity?, open}`. `app` is `cli`, `ide`, `desktop` or `background`; `folder` is where the conversation started, where it resumes. `live` is true while it is open: a running Claude Code process holds it, a Codex process has its thread loaded, or a Cursor window shows it selected. `activity` is `working`, `waiting` or `idle` for a live Claude Code session. `opening` holds up to three lines of the first prompt and `prompt` the latest one; any of `title`, `opening` and `prompt` that may hold a credential is left out and named in `withheld`, by the rule `/api/views/source` uses. `open` is `{kind, label, command?, link?}`: `focus` switches to the Terminal tab that runs it, `terminal` runs `command` in a new Terminal window, and `link` opens the editor extension, Cursor chat or desktop app it came from; for an editor extension the server first opens the conversation's folder in that editor, so the link reaches the window that holds it. Codex and Cursor stores are read with the `sqlite3` command. Each row has a `state`, the first of these that holds: `blocked` (open, and its tool says it waits for you), `working` (open, and its tool says the agent is busy), `completed` (you marked it completed and nothing was written since), `unread` (written after you last saw it), `active` (open, or written in the last 24 hours) and `dormant` (closed and quiet for longer). Only Claude Code reports waiting and busy. `seenAt` and `completedAt` are your marks, when set, and `needsYou` counts the blocked and unread conversations of every age and tool the server lists, whatever the page or the search. The marks live in `conversation-marks.json` in the `agnostic-ai` folder of your user configuration folder, with tool names, ids and times only. The first read starts the file, and what was written before it counts as seen. A read deletes a completion that a later message overtook, and that conversation is then unread. `marksNote` says why the file could not be read or written; rows then carry no marks. On a server started with `--cloud-sessions`, Claude Code cloud sessions are listed too, with `app` `cloud`; `cloudNote` says why they are missing when they could not be read. Codex and Cursor chats are read again only when their files change, for at most a minute, so one of them can show as open for up to a minute after its app quit. The reply has an `ETag` that changes when a chat or a mark changed, when a new minute began (a process that ended or a chat that went dormant shows no other way), or when the query differs. Sent back as `If-None-Match`, an unchanged list answers 304 with no body and reads no chat. |
| `POST /api/conversations/open` | `{tool, id, editor?}` with `Content-Type: application/json`, from the server's own origin. Reopens a conversation the list holds along its `open` route and returns `{opened, message, command?}`. `opened` is false when it could not: a live session in another app's terminal, a system that cannot open Terminal, or macOS refusing Automation access; `command` then resumes it by hand. The route comes from the server's own scan, never from the request; a conversation outside the list returns 404, and any other field is a 400. Opening Terminal asks once for macOS Automation access. A conversation that opened is marked seen, and a completion on it is taken back. |
| `POST /api/conversations/mark` | `{tool, id, as}` with `Content-Type: application/json`, from the server's own origin. `as` is `seen` (read up to now), `completed` (done until something new is written; this also marks it seen), `open` (takes a completion back) or `hidden` (left out of every list, search and count until something new is written; the tool's own store keeps it, and this also marks it seen). Returns 204. A conversation outside the list returns 404, and another `as` or any other field is a 400. |
| `GET /api/views/authority` | `?agentId=<id>&target=<tool>` for per-agent decisions. |
| `POST /api/views/authority/impact` | A permission proposal to preview. |
| `GET /api/views/authority/schema` | None. |
| `POST /api/edit/builtin` | `{audience, name, enabled, expected?}`, or `{audience: "personal", reset: true, expected?}` to drop the personal list. `expected` is the shown boolean, or the shown list for reset; a changed value returns 409 without saving the requested change. Built-ins apply to every tool, so a `target` other than `all` returns 400. |
| `POST /api/edit/targets` | `{audience, name, enabled, expected?}`, or `{audience: "personal", reset: true, expected?}` to drop the personal list. `name` is a built-in coding tool; with `--global`, one `sync --global` can write. `expected` is the shown boolean, or the shown list of tools that are on for reset; a changed value returns 409 without saving the requested change. Turning off the last tool returns 400. Returns `{ok, file}`. |
| `POST /api/edit/value` | `{audience, path, key, value, expected?}`, where `value` is a boolean, or `null` with `audience: personal` to reset. `expected` is the shown boolean; a changed value returns 409 without saving the requested change. The key already names the tool (`x-claude.verbose`), so a `target` other than `all` returns 400. |
| `POST /api/edit/mcp` | `{action, audience, name, target, path, scope, expected?}`. `name`, `target`, `path`, and `scope` identify one native row from the overview; here `scope` is its source scope (`project` or `user`). `action: "preview"` returns `file`, `spec`, `variables`, `targets`, `syncCommand`, and `expected`. `action: "apply"` requires that `expected`, writes only the spec, and returns those fields with `ok: true`. A changed preview or existing spec stops apply; a credential import cannot rewrite is refused with its import message. Personal saves use the same Git-ignore check as other edits. |
| `POST /api/edit/hook` | `{action, audience, name, on, match?, command, timeout?, description?, expected?}`. `name` uses lowercase letters, digits, and hyphens; `on` is a [portable event](@/docs/spec-format/hook-reference.md#portable-events); `match` (`shell`, `edit`, `read`, or `web`) goes only with `before-tool` and `after-tool`; `timeout` is a whole number of seconds above zero. `action: "preview"` returns `file`, `spec`, `syncCommand`, and `expected`, and answers 422 with `problems` when `validate` would report the hook. `action: "apply"` requires that `expected`, writes only the new hook file, and returns those fields with `ok: true`. An existing hook name returns 409; so does a changed preview. Local saves (`audience: personal`) use the Git-ignore check. |
| `POST /api/edit/spec/move` | `{action, kind, name, from, to, mode, overwrite?, expected?}`. Copies or moves one shared spec between the project and the global setup. `kind` is `rule`, `agent`, `skill`, `hook`, or `mcp`, since the global setup reads no other kind; `from` is the level this server reads and `to` is the other; `mode` is `copy` or `move`. A skill folder moves whole. `action: "preview"` returns `source`, `destination`, `exists`, `files`, `syncCommands`, and `expected`. `action: "apply"` requires that `expected`, which holds for either `mode` and `overwrite`, writes the destination, removes the source for `move`, and returns those fields with `ok: true`, plus `warning` when a leftover folder needs deleting by hand. An existing destination returns 409 unless `overwrite` is `true`, which replaces it whole; so does a spec of that name at another path, a changed preview, a destination that is the source itself (one setup's folder links to the other's), or a skill the destination would read differently (under another skill's folder, or over a folder of other skills). A built-in, a pack spec, a local spec, a `SKILL.md` directly in the skills folder, or a destination folder that links outside its setup returns 422, as does a rule sent to the global setup with a scope, paths, or target condition, including one under an `x-<target>` key for a target the global setup selects. A project server whose global setup does not exist yet returns 409 with `missing: true` and `root`; a global server that knows no project returns 409. |
| `POST /api/edit/permissions` | `{action, audience, path?, key, rules, before?, expected?}`. `key` is `permissions.allow`, `permissions.ask`, or `permissions.deny`; `rules` is a string list, or `null` to remove that list. `before` is the selected audience's own list as shown, or `null` when absent; a mismatch returns 409. Preview returns `schemaVersion: "permission-edit/v1"`, `file`, `key`, `audience`, `before`, `rules`, `diff`, `expected`, and `syncCommand`. Apply requires the preview's `expected`, rechecks its inputs, saves only the source file, and returns the preview fields with `ok: true`. Personal saves use the Git-ignore check. |
| `POST /api/edit/protected` | `{action, audience, path?, protected, before?, expected?}`. `protected` is `{paths, decision, reason}`, where `decision` is `ask` or `deny`, or `null` to remove the block. It writes the block to the settings spec the permission lists use, replacing any block there. A local save writes only the keys that differ from the project block, so a later project change to the others still applies, and returns 400 when it would lower a project `deny` to `ask`. `before` is the selected audience's own block as shown, or `null`; a mismatch returns 409. Preview returns `schemaVersion: "protected-edit/v1"`, `file`, `audience`, `before`, `protected`, `diff`, `expected`, and `syncCommand`, plus `cleared: true` when the file sets `protected: null`, which `protected: null` in the request removes; apply works as for `/api/edit/permissions`. The global server returns 400, because `sync --global` does not write protected paths. |
| `POST /api/edit/field` | `{audience, path?, key, target?, value, expected?}` for `model`, `effort`, and global `permissions.default-mode`, `session-retention-days`, `voice.enabled`, `voice.mode`, or `voice.language`. `target` is `all` (the default) or a configured tool; for one tool the edit writes `model.<tool>` or `effort.<tool>`, turning `model: x` into `{default: x, <tool>: value}`, and an effort level is checked against that tool only. A tool with no effort setting returns 400. When a model tier gives the spec its effort, one tool's effort returns 422, because a spec value turns the tier's effort off for every tool. Session retention takes only `all`. With `all`, a field that is already a per-tool map gets its `default` entry. `null` clears the value at that address, or resets your override with `audience: personal`; a map left with only `default` goes back to a single value, except in a personal file over a shared per-tool map, where a single value would replace the whole shared map, so it stays `{default: x}`. `expected` is the value the page showed at that address in the file being written, or `null` if that file did not set it. If the file now holds another value, or a personal file now sets that address for a shared save, the answer is 409 and nothing is written. A file that already holds the new value counts as saved. Voice values are a boolean for `voice.enabled`, `hold` or `tap` for `voice.mode`, and a non-empty string for `voice.language`; only the global view and target `all` are accepted. `null` clears one member, removing an empty `voice` map. |
| `GET /api/changes/builtin` | None. Returns the session's change state. |
| `POST /api/changes/builtin/preview` | `{audience, name, enabled}` for any shipped built-in, `{audience: "personal", reset: true}`, or `{audience, setting: {path?, key, target?, value, expected?}}` for `model` or `effort`, with the fields `/api/edit/field` takes. A setting plan carries `key`, `target` for one tool, and a `title` such as `effort set to high` or `model for codex set to sol`; undo state names it in `undoTitle`. |
| `POST /api/changes/builtin/apply`, `.../undo`, `.../cancel` | `{id}` from the change state. Cancel discards that preview without writing files and keeps the last undo; it answers 409 when that preview was already replaced or closed. |

Omit `path` only when at most one editable settings spec exists; with none, a field save creates `settings/defaults.yaml`, or a permission save creates `settings/permissions.yaml`, for the selected audience. The response names the edited file.

### Claude Code cloud sessions

`agnostic-ai serve --global --cloud-sessions` adds the Claude Code sessions that run at claude.ai/code to the conversations list. They count among Claude Code's conversations, with `app` `cloud`, and take the same states and marks as the others. A project server lists none: a cloud session names no folder.

- **What it reads.** It asks `GET https://api.anthropic.com/v1/code/sessions?limit=100`, which answers at most 100 sessions; it does not page past them yet. It uses their id, title, status and times. It never reads a session's messages.
- **The sign-in.** It reads the claude.ai sign-in Claude Code keeps in the macOS keychain (the `Claude Code-credentials` entry) for each call, and sends it only to `api.anthropic.com`. It never keeps, prints or logs it, and never uses the refresh token. Run `claude` to sign in or renew it. Windows and Linux are not supported yet.
- **How often.** At most once a minute, with a 5-second timeout and no retry. A cloud change shows in the list at most a minute late (65 seconds in the companion, which asks every 5 seconds).
- **When it fails.** Local conversations still list. A refusal, no answer in time, or an answer this version cannot read passes by itself: the last good list stands in for up to 10 minutes and `cloudNote` stays empty. After that, or at once for a missing or expired sign-in, no cloud row shows and `cloudNote` says why. A system with no supported sign-in store lists none and says nothing.
- **Opening one** opens the session's own page, `https://claude.ai/code/session_<id>`, in the browser. It is the only kind of web page the server opens.
- **Status.** `working` shows as working, only while the session's worker runs, and `blocked` as waiting for you; other statuses are not live. No process on this machine can be checked for a cloud conversation, so either cloud's status is believed for a day after the conversation's last write.

The same flag adds the threads that run in the Codex cloud. They count among Codex's conversations, with `app` `cloud`.

- **What it reads.** It asks `GET https://codex-cloud-backend.chatgpt.com/v1/threads?limit=100`, the list the Codex app reads, which answers at most 100 threads; it does not page past them yet. It uses their id, name, status and times, and never reads a thread's messages. The endpoint is undocumented and can change with any Codex release.
- **The sign-in.** It reads the ChatGPT login the Codex command keeps in `auth.json` (in `$CODEX_HOME`, default `~/.codex`) for each call, and sends it only to that host. It never keeps, prints or logs it, and never reads the refresh token or an API key. Without that file nothing is asked and nothing is said: the person does not use Codex. A login with an API key has no cloud threads, and `cloudNote` says so.
- **How often, and failures:** as for Claude Code above. `cloudNote` holds one sentence for each cloud that could not be read.
- **Opening one** opens `codex://threads/<id>?hostId=durable`, which the Codex app handles.
- **Status.** An `active` thread shows as working, or as waiting for you when it waits on an approval or on your input; other statuses are not live.

The endpoint is undocumented and can change with any Claude Code release.

## companion

Show a small circle that floats over every app and counts the Claude Code, Codex and Cursor conversations that wait for you. Click it, or press its shortcut, for a searchable list that opens each conversation where it lives and marks it as read or done.

```bash
agnostic-ai companion install
agnostic-ai companion
```

The companion is a separate program that is built on your machine, so `companion install` comes first, and again after each upgrade of agnostic-ai. It builds the companion from the same commit as the `agnostic-ai` you run and prints each step as it goes. It needs:

| System | Needs |
|--------|-------|
| macOS | Go and the Xcode Command Line Tools. |
| Windows | Go. Compiles; not yet run on a Windows machine. |
| Linux | Go, a C compiler, and the GTK 4, WebKitGTK 6.0 and X11 development packages. Not yet built or run on a Linux machine. |

When something is missing, the command names it and how to get it, and builds nothing. A build that fails leaves the installed companion as it was.

| Flag | Description |
|------|-------------|
| `install --source` | Build from this folder, the `companion` folder of an agnostic-ai checkout, instead of fetching the source. |

`agnostic-ai companion` starts the companion and returns. Running it again brings a hidden companion back and opens its list. It says when the companion was built for another release.

The companion reads no chat itself. It starts its own `agnostic-ai serve --global --api-only --cloud-sessions` on a free loopback port and asks it every 5 seconds whether the list changed, with the list's `ETag`; the server answers without reading anything while no chat changed. **Hide Agnostic AI companion**, in the list's ⋯ menu, takes the circle off the screen and stops that server: nothing is read or asked until the shortcut brings the companion back.

## mcp tools

Connect to each configured MCP server, list its tools, and cache the result so `view` and `serve` show them without connecting. It is the only command that connects to MCP servers.

```bash
agnostic-ai mcp tools
agnostic-ai mcp tools --global
```

| Flag | Description |
|------|-------------|
| `--dir` | Project directory to read. Default: the current directory. |
| `--global` | Read the user configuration in `$AGNOSTIC_AI_HOME` (default `~/.agnostic-ai`). |
| `--json` | Print the cached result as JSON. |
| `--timeout` | Maximum probe time per server, including its header helper. Default `20s`. Process cleanup is bounded separately. |

Stdio servers start with their `command`, `args`, and `env`, with relative or omitted working directories resolved against the configuration root; HTTP servers use their headers and optional `headersHelper`. `${NAME}` resolves from the environment and `${workspaceFolder}` is the root; an unset reference is reported without connecting. SSE, WebSocket, and disabled servers are not probed. Up to four servers run at once; cancellation stops owned processes. Errors and tool descriptions are redacted before display or caching.

The cache lives in the user cache directory, shared across directory aliases. Changed specs mark a listing stale; rerun `mcp tools` to show it in the overview.

+++
title = "Kiro"
description = "How agnostic-ai emits Kiro configuration: native paths, capability limits, and output options."
weight = 160

[extra]
group = "Reference"
target_id = "kiro"
+++

# Kiro (`kiro`)

## Output

```
AGENTS.md                          # canonical entry-point pointer body (written by sync, shared path)
.kiro/steering/<name>.md           # one per rule (inclusion: always, or fileMatch + fileMatchPattern from globs)
.kiro/skills/<name>/SKILL.md       # one per skill, Kiro's native skill-folder surface (+ bundled scripts/, references/, assets/)
.kiro/agents/<name>.md             # one per agent, Kiro's native agent-profile surface
.kiro/hooks/<name>.json            # one per hook, Kiro's native hook surface
.kiro/settings/mcp.json            # when MCP entries exist
.kiroignore                        # when ignore entries exist
```

AWS Kiro loads [steering files](https://kiro.dev/docs/steering/) whose YAML frontmatter must be the first content in the file. The adapter maps rules onto Kiro's inclusion modes: unscoped rules are `always`, globbed rules become `fileMatch` with `fileMatchPattern`. Kiro also reads the root `AGENTS.md` (always included), which carries the shared pointer body.

Skills are [native](https://kiro.dev/docs/skills/) too, not a steering-file convention: one folder per skill at `.kiro/skills/<name>/SKILL.md` ("Workspace skills (`.kiro/skills/`)"). This is the tree Kiro's own skill picker globs (`skill://.kiro/skills/*/SKILL.md`) and the standard Agent Skills layout (`name` + `description` frontmatter), which this adapter shares byte-for-byte with the `.agents/skills/` render ten other targets already produce. Bundled sibling assets (`scripts/`, `references/`, `assets/`) copy alongside SKILL.md.

A prior version of this adapter flattened skills into `.kiro/steering/skill-<name>.md` with `inclusion: auto`, which dropped bundled assets entirely and never reached Kiro's skill picker (#642). `sync` sweeps a stale file of that shape left behind for a current skill name, the same convention agents already use below.

Kiro's steering docs separately confirm `auto` and `manual` inclusion modes exist; this adapter has no rule shape that needs either.

Agents are [native custom agents](https://kiro.dev/docs/custom-agents/), not a steering-file convention: one YAML-frontmatter Markdown file per agent at `.kiro/agents/<name>.md`, the tree Kiro's agent picker reads. `description` (falls back to the agent name) and `model` pass through. The canonical spec name remains the safe filename, while `x-kiro.name` sets Kiro's optional display name and survives import.

Kiro's [documented agent schema](https://kiro.dev/docs/custom-agents/configuration-reference/) also carries `tools`, `mcpServers`, `permissions`, `hooks`, `keyboardShortcut`, and `welcomeMessage`; only `tools` has an agnostic-ai spec equivalent.

That page documents Kiro's own `tools` vocabulary (category tags `read`/`write`/`shell`/`web`/`subagent`/`knowledge`/`todo_list`, `@server_name`, `@server_name/tool_name`, `@mcp`, `@builtin`, `*`). [`/docs/tools/`](https://kiro.dev/docs/tools/), updated 2026-08-21 versus configuration-reference's 2026-08-04, tables the same field as `read`/`write`/`shell`/`web`/`subagent`/`spec`/`context` instead, folding `knowledge` into a new `context` bundle (`disclose_context`/`introspect`/`knowledge`) and dropping `todo_list`. The two pages disagree and neither says which one the shipping product follows, so this section keeps citing configuration-reference rather than guessing.

It makes no functional difference here, since both pages agree on the four categories a spec's generic `tools` list actually translates onto:

| Spec `tools` values | Kiro category |
| --- | --- |
| `Read`, `Grep`, `Glob` | `read` |
| `Write`, `Edit` | `write` |
| `Bash` | `shell` |
| `WebFetch`, `WebSearch` | `web` |

(deduplicated when a spec lists more than one value that maps to the same category.)

Kiro's [built-in-tools catalog](https://kiro.dev/docs/tools/) documents each category as a bundle rather than a single tool, so this widens access beyond a single Claude-style name on its own: `write` also covers `delete_file`, so declaring only `Edit` grants delete too, and `web` covers both fetch and search, so either `WebFetch` or `WebSearch` alone grants both.

A tools value outside agnostic-ai's Read/Write/Edit/Bash/Grep/Glob/WebFetch/WebSearch set has no confirmed Kiro equivalent; it is dropped rather than written unconfirmed and surfaces a coverage note, while any name in the same list that does translate still emits (the same unconfirmed-vocabulary failure class Kilo Code and Augment still hit, since their own tool vocabularies remain undocumented).

Set `x-kiro.tools` to bypass the translation table with Kiro's own vocabulary directly, or `x-kiro.mcpServers`, `x-kiro.permissions`, `x-kiro.hooks`, `x-kiro.keyboardShortcut`, or `x-kiro.welcomeMessage` for fields with no agnostic-ai equivalent. Arbitrary `x-kiro` keys always pass through, and `x-kiro.tools` always wins outright over the translated form.

A prior version of this adapter flattened agents into `.kiro/steering/agent-<name>.md` with `inclusion: manual`, which never reached the agent picker; `sync` sweeps a stale file of that shape left over from an older sync.

Hooks are [native](https://kiro.dev/docs/hooks/) too: one JSON file per hook spec, `{"version": "v1", "hooks": [{name, trigger, matcher, action, timeout, enabled, description}]}`. `event` becomes `trigger`, passed through verbatim. `command` (string or list) becomes `action: {"type": "command", "command": ...}`, one entry per command sharing the file when the spec lists several, `name` suffixed `-2`, `-3`, ... to stay unique.

`disabled: true` writes `"enabled": false`; the enabled default needs no key. The spec's generic `description` field (free-form documentation) now reaches the file too; the vendor lists the matching `hooks[].description` as "Documentation only".

Every entry marshals from a map rather than a fixed set of fields, so arbitrary `x-kiro` keys pass through as well: `confirm` ("Ask for confirmation before a Stop command hook runs", taking `question`, `options` (`id`/`label`/`run` each), and an optional `confirmCommand`) has no agnostic-ai spec equivalent and is only reachable this way. `x-kiro.action` accepts `{"type": "agent", "prompt": ...}` or `{"type": "command", "command": ...}`, written in spec YAML as `x-kiro.action: {type: agent, prompt: ...}` or `{type: command, command: ...}`. A valid explicit action needs no generic `command` and replaces the whole fallback command list with one native action. Invalid actions fail sync instead of running a fallback.

Explicit `timeout: 0` disables the command timeout; omission retains Kiro's 60-second default. Before #642, `description` and `confirm` were unreachable at any layer, including x-kiro, because the prior fixed-struct shape had no route for a key it did not declare.

Unlike Claude Code, Codex, Gemini, and Cursor, stashed hook scripts under `.agnostic-ai/scripts/` do not materialize into `.kiro/hooks/`: that directory is where Kiro looks for hook definitions, not scripts.

MCP servers write to `.kiro/settings/mcp.json` under the standard `mcpServers` map, the workspace tier on [Kiro's own configuration page](https://kiro.dev/docs/mcp/configuration/) ("Workspace Level: `.kiro/settings/mcp.json`"). A local server carries `command` plus optional `args` and `env`; a remote server carries `url` plus optional `headers` and `env`.

Both tables also document `disabled` ("Whether the server is disabled (default: false)"), which passes through under that literal name, unlike Claude Code and Cursor which have no file-based equivalent. They also document `autoApprove` ("Tool names to auto-approve without prompting", `"*"` for all) and `disabledTools` ("Tool names to omit when calling the Agent").

A remote server adds an `oauth` object, `{clientId, clientSecret, redirectUri, clientMetadataUrl, oauthScopes}`, and the top-level `oauthScopes` fallback. `oauth.oauthScopes` wins when both are set; an explicitly empty `oauthScopes: []` emits as written, since the vendor makes that the documented remedy for scope errors. All four were unreachable before #634, top-level or namespaced.

Kiro's `oauth` is not Claude Code's, so each target maps only the sub-keys its own vendor documents. See [`disabled` support by target](@/docs/spec-format.md#disabled-support-by-target).

Ignore specs emit as `.kiroignore` in the project root, plain gitignore syntax: "To exclude files in a specific project, create a `.kiroignore` file in your project root (or any subdirectory) and add patterns for files you want to exclude" and "`.kiroignore` uses standard gitignore syntax" ([kiro.dev/docs/kiroignore](https://kiro.dev/docs/kiroignore/), target-audit 2026-09-11, #728).

Two vendor caveats gate how far the file reaches, and neither is something `sync` can set: the IDE reads ignore filenames from its own Agent Ignore Files setting, so `.kiroignore` has to be in the `kiroAgent.agentIgnoreFiles` array before the IDE honors it (the vendor suggests `[".gitignore", ".kiroignore"]`). CLI V3 applies it to content- and filename-search results only rather than across every agent tool.

Multiple specs concatenate. Override via `outputs.kiro.ignore-file`.

## Config keys

| Key | Default |
| --- | --- |
| `outputs.kiro.rules-dir` | `.kiro/steering` |
| `outputs.kiro.agents-dir` | `.kiro/agents` |
| `outputs.kiro.skills-dir` | `.kiro/skills` |
| `outputs.kiro.hooks-dir` | `.kiro/hooks` |
| `outputs.kiro.mcp-file` | `.kiro/settings/mcp.json` |
| `outputs.kiro.ignore-file` | `.kiroignore` |

## Import

`agnostic-ai import kiro` reverses the Kiro layout. Native agent and skill trees import first. Then the flat `.kiro/steering/` files (frontmatter-first `inclusion:` block, filename prefix picks the kind) fill in rules plus anything a pre-native sync still left flattened there:

| Source | Becomes |
|--------|---------|
| `.kiro/agents/<name>.md` (native agent profile) | `<agents>/<name>.md` |
| `.kiro/skills/<name>/SKILL.md` (native skill folder) | `<skills>/<name>/SKILL.md`, bundled sibling assets included |
| `.kiro/steering/<name>.md` (`inclusion: always`) | `<rules>/<name>.md` (unscoped rule) |
| `.kiro/steering/<name>.md` (`inclusion: fileMatch` + `fileMatchPattern`) | `<rules>/<name>.md` with `globs: <fileMatchPattern>` |
| `.kiro/steering/agent-<name>.md` (legacy, pre-native sync) | `<agents>/<name>.md`, body only |
| `.kiro/steering/skill-<name>.md` (legacy, pre-native sync) | `<skills>/<name>/SKILL.md`, body only |
| `.kiro/hooks/<id>.json` (`hooks[]`) | one hook spec per group of entries that differ only in `action.command` |
| `.kiro/settings/mcp.json` (`mcpServers.<name>`) | `<mcps>/<name>.yaml` |
| `AGENTS.md` | `.agnostic-ai/AGNOSTIC_AI.md` |

The native rows run after the legacy steering sweep, so a name present under both wins on the native copy.

Hooks read back the whole vendor field table, not just the keys `sync` writes. `trigger` becomes `event` verbatim, `matcher`, `description` and `timeout` map straight across (including an explicit `timeout: 0`), and `enabled: false` becomes `disabled: true`. An `action.type: "agent"` action lands under `x-kiro.action` rather than the portable prompt handler, which [spec-format.md](@/docs/spec-format.md#hooks) scopes to Claude Code, Cursor and Copilot. `confirm` and every other key the vendor has not closed off land under `x-kiro` too, so a future field is kept rather than dropped.

Four shapes a hand-authored file can hold that `sync` never writes are handled explicitly. One file carrying several `trigger` values splits into one spec per trigger, since a spec holds exactly one `event`. Entries that differ only in `action.command` recombine into one spec with a `command:` list, dropping the `-2`/`-3` suffixes emit added, so a spec carrying three commands does not return as three specs. A name the vendor writes for humans ("Lint on save" is its own example) slugs into the spec filename and stays intact on `name:`. An action the vendor marks conditional but leaves unset (`type: "command"` with no `command`, `type: "agent"` with no `prompt`) is skipped with a warning instead of becoming a spec that fails the next sync.

Some data is lossy on round-trip. Kiro's emit cannot carry it, so the reconstructed spec drops it without changing Kiro's output:

- A rule's source-layout scope collapses into an equivalent `globs:`.
- A legacy flattened steering agent or skill keeps only its body. That flattened form never carried a description, model, or bundled sibling assets in the first place.
- A hook's explicit `enabled: true` leaves no key. It is the vendor default, and emit writes nothing for it.
- Two hook files declaring the same `name` keep both hooks, but the second takes a deterministic generated name: a spec name is unique across the bundle, and emit derives the hook filename from it.
- `{{filePath}}` inside a command survives as literal text. The vendor calls it "new in 3.0 and only available in the new format", so it means nothing on any other target.

## Verify

1. Install Kiro from [kiro.dev](https://kiro.dev).
2. Check the tree: `ls AGENTS.md .kiro/steering/ .kiro/skills/ .kiro/agents/ .kiro/hooks/ .kiro/settings/mcp.json .kiroignore`, `head -2 .kiro/steering/*.md .kiro/skills/*/SKILL.md .kiro/agents/*.md` (frontmatter first, no leading blank lines), `python -m json.tool .kiro/settings/mcp.json > /dev/null`, and `python -m json.tool .kiro/hooks/*.json > /dev/null` for each hook file.
3. Open the project; the steering panel lists every rule file with its inclusion mode and no parse warnings, the skill picker lists every `.kiro/skills/<name>/` folder, and the agent picker lists every `.kiro/agents/<name>.md` profile.
4. Trigger a hook's `trigger` event (e.g. save a file for `PostFileSave`); the configured command runs with no schema warning.

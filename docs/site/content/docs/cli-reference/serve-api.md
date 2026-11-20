+++
title = "Local API"
description = "Routes of the JSON API that serve runs on your machine, for programs that read or edit configuration."
weight = 43

[extra]
group = "Reference"

[extra.moved]
get-api-views-conversations = "@/docs/cli-reference/conversations-api.md"
post-api-conversations-open = "@/docs/cli-reference/conversations-api.md"
post-api-conversations-send = "@/docs/cli-reference/conversations-api.md"
post-api-conversations-mark = "@/docs/cli-reference/conversations-api.md"
post-api-conversations-rename = "@/docs/cli-reference/conversations-api.md"
post-api-conversations-delete = "@/docs/cli-reference/conversations-api.md"
+++

# Local API

`agnostic-ai serve` runs a JSON API on `127.0.0.1:4310` (change it with `--addr`). The web interface uses it, and scripts can too. The routes that list, open, send to, mark, rename and delete conversations are on [Conversations API](@/docs/cli-reference/conversations-api.md).

The calls most people use:

- `GET /api/health` checks that the server runs and which folder it reads.
- `GET /api/views/overview` returns what the setup holds and what is missing.
- An edit in two steps: send the request with `"action": "preview"`, read the plan, then send it again with `"action": "apply"` and the `expected` value the preview returned. Nothing is written until apply.

```bash
curl -s http://127.0.0.1:4310/api/health
```

The routes below are the full reference. Edit and impact requests take JSON.

- Every edit and preview names its `audience`: `shared` or `personal`.
- The older `scope` field (`team` or `local`) is accepted until the next minor release, except on `/api/edit/mcp`, `/api/edit/hook`, `/api/edit/permissions`, and `/api/edit/protected`. A request that sends both with different meanings returns 400.
- A change plan carries both `audience` and `scope` until then.
- A boolean edit must include its `enabled` or `value` field, even when false.
- Invalid values, and YAML that cannot be spliced safely, are refused without changing the file.
- Previews hide recognized credentials using the [credential preview rules](@/docs/cli-reference/sync.md#credential-previews). They run no MCP servers, hooks, or external adapters.

## `GET /api/health`

Takes no input.

- Returns `ok`, absolute `root`, `global`, `ui` (whether the web interface is served), and `version` (as `agnostic-ai --version` prints it). It does not load specs.
- A global server started in a project adds that project's absolute `projectRoot`.
- When the config sets [`requires`](@/docs/configuration.md#requires), it adds `requires` as written and `requiresMet`: `true` or `false`, or `null` for a build that is not a release. It reads both on each request.
- While a conversation list is being read, it adds `scanningSince`: when that read began, in RFC 3339. The companion replaces a server whose read has run for over two minutes.

## `POST /api/switch`

Body: `{}` with `Content-Type: application/json`.

- Finds or starts the server for the other configuration: the user configuration from a project, or `projectRoot` from a global server.
- Returns `url`, `root`, `global`, and `started`.
- Answers 404 on a global server with no `projectRoot`.
- Answers 409 with `{error, missing, root}` and no server when the user configuration folder has no setup.
- Answers 503 with the command to run when no server could start.

## `POST /api/global/create`

Body: `{"git": true}` or `{"git": false}`. Project servers only.

- Writes the starter files into the user configuration folder fixed at startup.
- Replies `{root, created, git}`.
- Answers 409 when a setup already exists.
- Any other field is a 400.

## `GET /api/views/overview`

Takes no input.

- `global` is true for user configuration.
- `repository` is `{url, name}` when the project's `origin` remote is on GitHub. `url` is the repository's `https://github.com/<owner>/<repo>` page and `name` is `<owner>/<repo>`. Absent otherwise, and for user configuration.
- `health.gaps` lists what the configured tools support and the project does not use yet. Each is `{id, section, label, supported, missing, fix}`. Built-in specs fill no gap. Empty for user configuration.
  - `id` is `kind:<kind>` (rule, agent, skill, command, review or ignore with no spec of the project's own), `stage:<stage>` (a hook stage with no hook) or `builtin:handoff` (off).
  - `supported` lists the configured targets that would use it.
  - `fix` is a prompt a coding agent can run.
- `health.improvements` is `{applied, total}`. `total` counts every kind, stage, and built-in the configured targets support. Each one not applied is a gap, so `total - applied` equals the number of gaps. Absent for user configuration.

## `GET /api/views/context`

Query: `?target=<tool>`, a configured target. The default is the first one.

- `&file=<path>` adds scoped rules for that file, as `--file` does. `fileTargets` lists the targets that accept it.
- Returns `targets`, `target`, absolute `root`, `startupWordBudget` (the `lint.instructions-words` value the page compares startup words with), and `report`, the `explain --context --json` output.
- With `--global`, `supported` is false and `reason` says why.

## `GET /api/views/source`

Query: `?path=<absolute path>`, the `openPath` of a row the Context or Lifecycle page lists. That row is an instructions file, rule, agent, skill, command, review instructions, ignore file or hook of the served configuration, project or global, including `local/`.

Returns:

- `path` and `kind`.
- `frontmatter`: a list of `{key, value}` in file order.
- `body`.
- `bytes`: the file size.
- `truncated`: true when the body was cut at 256 KB.
- `withheld`: true when the text is hidden. Then `body` and `frontmatter` are empty. The text is hidden in two cases:
  - It may hold a credential: any credential-named key, or any of the words password, passphrase, secret, credential, API key, private key, access key, secret key, or client secret, even in guidance that only names them.
  - The file is too large to check whole (over 320 KB).

Rules:

- A leading frontmatter block is read whole, up to 64 KB more, so its values can be checked. One that does not close within that is withheld.
- A hook returns every field of its YAML file in `frontmatter` and an empty `body`, withheld by the same rule. A hook file that does not parse returns no fields.
- Any other path returns 404. So does a file whose symlinks lead outside the source folders the configuration loads.

## `GET /api/views/authority`

Query: `?agentId=<id>&target=<tool>` for per-agent decisions.

## `POST /api/views/authority/impact`

Body: a permission proposal to preview.

## `GET /api/views/authority/schema`

Takes no input.

## `POST /api/edit/builtin`

Body: `{audience, name, enabled, expected?}`, or `{audience: "personal", reset: true, expected?}` to drop the personal list.

- `expected` is the shown boolean, or the shown list for reset. A changed value returns 409 without saving the requested change.
- Built-ins apply to every tool, so a `target` other than `all` returns 400.

## `POST /api/edit/targets`

Body: `{audience, name, enabled, expected?}`, or `{audience: "personal", reset: true, expected?}` to drop the personal list.

- `name` is a built-in coding tool. With `--global`, it is one `sync --global` can write.
- `expected` is the shown boolean, or the shown list of tools that are on for reset. A changed value returns 409 without saving the requested change.
- Turning off the last tool returns 400.
- Returns `{ok, file}`.

## `POST /api/edit/value`

Body: `{audience, path, key, value, expected?}`. `value` is a boolean, or `null` with `audience: personal` to reset.

- `expected` is the shown boolean. A changed value returns 409 without saving the requested change.
- The key already names the tool (`x-claude.verbose`), so a `target` other than `all` returns 400.

## `POST /api/edit/mcp`

Body: `{action, audience, name, target, path, scope, expected?}`.

- `name`, `target`, `path`, and `scope` identify one native row from the overview. Here `scope` is its source scope (`project` or `user`).
- `action: "preview"` returns `file`, `spec`, `variables`, `targets`, `syncCommand`, and `expected`.
- `action: "apply"` requires that `expected`, writes only the spec, and returns those fields with `ok: true`.
- A changed preview or existing spec stops apply.
- A credential that import cannot rewrite is refused with its import message.
- Personal saves use the same Git-ignore check as other edits.

## `POST /api/edit/hook`

Body: `{action, audience, name, on, match?, command, timeout?, description?, expected?}`.

- `name` uses lowercase letters, digits, and hyphens.
- `on` is a [portable event](@/docs/spec-format/hook-reference.md#portable-events).
- `match` (`shell`, `edit`, `read`, or `web`) goes only with `before-tool` and `after-tool`.
- `timeout` is a whole number of seconds above zero.
- A `command` or `description` that holds a recognized literal credential, such as a token, a `Bearer` value, or a password, returns 400, and so does a `name` that is a token. The text inside `$(...)` and backticks is checked too. Read the secret at run time instead: `$GITHUB_TOKEN`, `${GITHUB_TOKEN}`, `$(gh auth token)`, or a backtick command. A quoted assignment such as `AUTH="Bearer $TOKEN"` is still refused.
- `action: "preview"` returns `file`, `spec`, `syncCommand`, and `expected`. It answers 422 with `problems` when `validate` would report the hook.
- `action: "apply"` requires that `expected`, writes only the new hook file, and returns those fields with `ok: true`.
- An existing hook name returns 409. So does a changed preview.
- Local saves (`audience: personal`) use the Git-ignore check.

## `POST /api/edit/spec/move`

Body: `{action, kind, name, from, to, mode, overwrite?, expected?}`. Copies or moves one shared spec between the project and the global setup.

- `kind` is `rule`, `agent`, `skill`, `hook`, or `mcp`, since the global setup reads no other kind.
- `from` is the level this server reads and `to` is the other.
- `mode` is `copy` or `move`.
- A skill folder moves whole.
- `action: "preview"` returns `source`, `destination`, `exists`, `files`, `syncCommands`, and `expected`.
- `action: "apply"` requires that `expected`, which holds for either `mode` and `overwrite`. It writes the destination, removes the source for `move`, and returns those fields with `ok: true`, plus `warning` when a leftover folder needs deleting by hand.

These return 409:

- An existing destination, unless `overwrite` is `true`, which replaces it whole.
- A spec of that name at another path.
- A changed preview.
- A destination that is the source itself (one setup's folder links to the other's).
- A skill the destination would read differently (under another skill's folder, or over a folder of other skills).
- A project server whose global setup does not exist yet. The reply carries `missing: true` and `root`.
- A global server that knows no project.

These return 422:

- A built-in, a pack spec, a local spec, a `SKILL.md` directly in the skills folder, or a destination folder that links outside its setup.
- A rule sent to the global setup with a scope, paths, or target condition, including one under an `x-<target>` key for a target the global setup selects.

## `POST /api/edit/permissions`

Body: `{action, audience, path?, key, rules, before?, expected?}`.

- `key` is `permissions.allow`, `permissions.ask`, or `permissions.deny`.
- `rules` is a string list, or `null` to remove that list.
- `before` is the selected audience's own list as shown, or `null` when absent. A mismatch returns 409.
- Preview returns `schemaVersion: "permission-edit/v1"`, `file`, `key`, `audience`, `before`, `rules`, `diff`, `expected`, and `syncCommand`.
- Apply requires the preview's `expected`, rechecks its inputs, saves only the source file, and returns the preview fields with `ok: true`.
- Personal saves use the Git-ignore check.

## `POST /api/edit/protected`

Body: `{action, audience, path?, protected, before?, expected?}`.

- `protected` is `{paths, decision, reason}`, where `decision` is `ask` or `deny`, or `null` to remove the block.
- It writes the block to the settings spec the permission lists use, replacing any block there.
- A local save writes only the keys that differ from the project block, so a later project change to the others still applies. It returns 400 when it would lower a project `deny` to `ask`.
- `before` is the selected audience's own block as shown, or `null`. A mismatch returns 409.
- Preview returns `schemaVersion: "protected-edit/v1"`, `file`, `audience`, `before`, `protected`, `diff`, `expected`, and `syncCommand`, plus `cleared: true` when the file sets `protected: null`, which `protected: null` in the request removes.
- Apply works as for `/api/edit/permissions`.
- The global server returns 400, because `sync --global` does not write protected paths.

## `POST /api/edit/field`

Body: `{audience, path?, key, target?, value, expected?}` for `model`, `effort`, and global `permissions.default-mode`, `session-retention-days`, `voice.enabled`, `voice.mode`, or `voice.language`.

- `target` is `all` (the default) or a configured tool.
- For one tool, the edit writes `model.<tool>` or `effort.<tool>`, turning `model: x` into `{default: x, <tool>: value}`. An effort level is checked against that tool only.
- A tool with no effort setting returns 400.
- When a model tier gives the spec its effort, one tool's effort returns 422, because a spec value turns the tier's effort off for every tool.
- Session retention takes only `all`.
- With `all`, a field that is already a per-tool map gets its `default` entry.
- `null` clears the value at that address, or resets your override with `audience: personal`.
- A map left with only `default` goes back to a single value. The exception is a personal file over a shared per-tool map. There a single value would replace the whole shared map, so it stays `{default: x}`.
- `expected` is the value the page showed at that address in the file being written, or `null` if that file did not set it.
- If the file now holds another value, or a personal file now sets that address for a shared save, the answer is 409 and nothing is written.
- A file that already holds the new value counts as saved.
- Voice values are a boolean for `voice.enabled`, `hold` or `tap` for `voice.mode`, and a non-empty string for `voice.language`. Only the global view and target `all` are accepted.
- For voice, `null` clears one member, removing an empty `voice` map.

## `GET /api/changes/builtin`

Takes no input. Returns the session's change state.

## `POST /api/changes/builtin/preview`

Body, one of:

- `{audience, name, enabled}` for any shipped built-in.
- `{audience: "personal", reset: true}`.
- `{audience, setting: {path?, key, target?, value, expected?}}` for `model` or `effort`, with the fields `/api/edit/field` takes.

A setting plan carries `key`, `target` for one tool, and a `title` such as `effort set to high` or `model for codex set to sol`. Undo state names it in `undoTitle`.

## `POST /api/changes/builtin/apply`, `.../undo`, `.../cancel`

Body: `{id}` from the change state.

- Cancel discards that preview without writing files and keeps the last undo.
- Cancel answers 409 when that preview was already replaced or closed.

## `GET /api/views/approvals`

Takes no input. Returns the [approvals](@/docs/cli-reference/companion.md#approvals) mode and what the decision log holds.

- `mode` is the loosest mode the home config sets: `ask`, `review` or `yolo`. `AGNOSTIC_AI_APPROVALS` in the server's environment makes it `ask`.
- `tools` is what `agnostic-ai approvals status --json` prints in its `tools`, for the folder the server runs in.
- `sessions` holds each session in `$AGNOSTIC_AI_HOME/local/approvals/decisions.jsonl` and its older file, latest decision first: `{tool, session, approved, asked, decisions}`. `decisions` are its latest 20 log lines, newest first.
- `problem` says why the settings could not be read; `tools` is then empty.

```json
{"mode": "yolo", "tools": [{"tool": "claude", "mode": "yolo", "effective": "ask", "from": "approvals.mode"}],
 "sessions": [{"tool": "claude", "session": "4f3c…", "approved": 3, "asked": 1, "decisions": [{"time": "2026-11-20T10:00:00Z", "tool": "claude", "session": "4f3c…", "cwd": "/Users/you/Code/shop", "call": "Bash", "detail": "git push", "mode": "yolo", "decision": "asked", "rule": "publish"}]}]}
```

## `POST /api/approvals/ask`

Body: `{"mode": "ask"}` with `Content-Type: application/json`. It is the only write approvals have here: no request loosens them.

- Writes `approvals.mode: ask` to `$AGNOSTIC_AI_HOME/local/agnostic-ai.yaml`, as `agnostic-ai approvals ask` does. That block replaces the shared one, so no tool or project keeps `review` or `yolo`. Its `always-ask` patterns stay.
- It works from the next request, with no sync and no restart.
- Replies `{"mode": "ask", "path": "<the file it wrote>"}`.
- Any other mode, any other field, or a body that is not JSON is a 400. A request from another origin is a 403.
- A file it cannot edit safely, such as one with two YAML documents or an anchor on `approvals`, is a 409 that says to set it by hand.

## When `path` is omitted

Omit `path` only when at most one editable settings spec exists. With none, a field save creates `settings/defaults.yaml`, or a permission save creates `settings/permissions.yaml`, for the selected audience. The response names the edited file.

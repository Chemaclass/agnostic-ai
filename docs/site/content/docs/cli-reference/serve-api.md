+++
title = "Local API"
description = "Routes of the JSON API that serve runs on your machine, for programs that read or edit configuration."
weight = 43

[extra]
group = "Reference"
+++

# Local API

Edit and impact requests take JSON.

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

## `GET /api/views/conversations`

Query:

- `?editor=<id>`: the editor chosen in Settings, so editor links use its scheme.
- `&before=<time>`: the next page, which holds older conversations.
- `&q=<words>`: a search.
- `&tool=claude`, `codex` or `cursor`: list one tool.

A search keeps the conversations of every age where each word appears in the title, folder, branch, latest prompt, tool or app, as the row shows them. Case is ignored.

- A withheld field does not match.
- Its pages hold only matches, and the page adds `matches`, the count across all pages.
- A `q` over 200 characters, or another `tool`, is a 400.

The route lists Claude Code, Codex and Cursor conversations from each tool's own store. The newest come first, by the last message from the person or the agent.

- Each page holds one week. The first page holds every open conversation and the past week.
- A page asked for with `before` holds the week before that time, and reaches past weeks with no conversation.
- The page returns `from` and `to`, and `next`, the `before` value of the following page, absent when no older conversation remains.
- A `before` that is not an RFC 3339 time is a 400.
- A project server lists the conversations started in the project or one of its Git worktrees. A global server lists them all.
- Runs a program started (Claude Code SDK sessions, `codex exec`) are left out.

Returns `tools` and `conversations`:

- `tools` is `{tool, label, found, count, shown, note?}`. `count` is every conversation the tool holds, `shown` is how many this page holds, and `note` says why a store was not read.
- Each of `conversations` is `{tool, app, id, title, folder?, branch?, startedAt?, updatedAt, opening?, prompt?, withheld?, live?, activity?, open}`.

Conversation fields:

- `app` is `cli`, `ide`, `desktop`, `background` or `cloud`.
- `folder` is where the conversation started and where it resumes.
- `live` is true while it is open: a running Claude Code process holds it, a Codex process has its thread loaded, a Cursor window shows it selected, or a cloud provider reports an active session.
- `activity` is `working`, `waiting` or `idle` when available. Claude Code reports local session status. Local Codex activity is inferred from its rollout. Cloud sessions use provider status.
- `opening` holds up to three lines of the first prompt, and `prompt` holds the latest one.
- Any of `title`, `opening` and `prompt` that may hold a credential is left out and named in `withheld`, by the rule `/api/views/source` uses.
- `title` is the name you chose with `POST /api/conversations/rename`, when you chose one. Otherwise it is the tool's own title, or the first line of the first prompt.
- `open` is `{kind, label, command?, link?}`:
  - `focus` switches to the Terminal or Ghostty tab that runs it. `label` then names the app, such as `Switch to Ghostty`. For a Codex thread that Codex's background service holds, the server looks for the Codex terminal that shows it.
  - `terminal` runs `command` in a new tab of the app the session last ran in, Terminal or Ghostty. It never types into the old tab: a shell waiting in `read` looks the same as one at its prompt. With no tab remembered, it opens a new Terminal window. `label` then names the app, such as `Open in Ghostty`.
  - `link` opens the editor extension, Cursor chat or desktop app it came from, or a cloud session in its browser or app. For an editor extension, the server first opens the conversation's folder in that editor, so the link reaches the window that holds it.

Codex and Cursor stores are read with the `sqlite3` command.

Each row has a `state`, the first of these that holds:

1. `blocked`: open, and its tool says it waits for you.
2. `working`: open, and its tool says the agent is busy.
3. `completed`: you marked it completed. It holds until something is written or its tool says the agent is working.
4. `unread`: written after you last saw it.
5. `active`: open, or written in the last 24 hours.
6. `dormant`: closed and quiet for longer.

The `activity` of a loaded local Codex thread is `working` during an unfinished turn. It is inferred to be `waiting` after five minutes without a rollout write, which makes the state `blocked` until you see it. A long-running command can also meet that threshold.

Marks:

- `seenAt` and `completedAt` are your marks, when set.
- `needsYou` counts the blocked and unread conversations of every age and tool the server lists, whatever the page or the search.
- The marks live in `conversation-marks.json` in the `agnostic-ai` folder of your user configuration folder. The file holds tool names, ids, times and the names you chose, never conversation text.
- The first read starts the file, and what was written before it counts as seen.
- An older `serve` that still runs after an upgrade drops the chosen names when it rewrites the file. Restart it, and the companion, after you upgrade.
- A read clears a completion or a hiding once a later message overtook it or the conversation started working. The entry keeps its seen time and chosen name. After a new message the conversation is unread. After work that wrote nothing it is active again when the work ends.
- `marksNote` says why the file could not be read or written. Rows then carry no marks.

Cloud sessions and refresh:

- On a server started with `--cloud-sessions`, Claude Code cloud sessions and Codex cloud threads are listed too, with `app` `cloud`. `cloudNote` says why they are missing when they could not be read.
- Codex and Cursor chats are read again only when their files change, for at most a minute. One of them can show as open for up to a minute after its app quit.
- The reply has an `ETag`. It changes when a chat or a mark changed, when a new minute began (a process that ended or a chat that went dormant shows no other way), or when the query differs.
- Sent back as `If-None-Match`, an unchanged list answers 304 with no body and reads no chat.

## `POST /api/conversations/open`

Body: `{tool, id, editor?}` with `Content-Type: application/json`, from the server's own origin.

- Reopens a conversation the list holds along its `open` route and returns `{opened, message, command?}`.
- For a `focus` route, `opened` is true only when the session's own tab came to the front. A running session is never started a second time.
- `opened` is false when the app came forward but its tab could not be chosen (Warp, or an editor's terminal), when Codex does not say which of its sessions shows the thread, when no terminal shows a running session, when the system cannot open a terminal, or when macOS refuses Automation access. `message` says where the session runs. `command`, when set, resumes it by hand; a Codex thread still open in the background gets none.
- Not supported yet: iTerm2, tmux, WezTerm, kitty and Warp. For their sessions the app comes forward, or `message` names the terminal device.
- The route comes from the server's own scan, never from the request.
- A conversation outside the list returns 404.
- Any other field is a 400.
- macOS asks once for Automation access to each app the server controls: Terminal and Ghostty. The prompt names the companion app, or the terminal that runs `agnostic-ai serve`.
- The tab each running session uses is kept in `conversation-tabs.json`, beside the marks file: the app and the terminal device. It holds no conversation text.
- A conversation that opened is marked seen, and a completion on it is taken back.

## `POST /api/conversations/mark`

Body: `{tool, id, as}` with `Content-Type: application/json`, from the server's own origin.

`as` is one of:

- `seen`: read up to now.
- `completed`: done until something new is written or it starts working. This also marks it seen.
- `open`: takes a completion back.
- `hidden`: left out of every list, search and count until something new is written or it starts working. The tool's own store keeps it. This also marks it seen.

Replies:

- Returns 204.
- A conversation outside the list returns 404.
- Another `as`, or any other field, is a 400.

## `POST /api/conversations/rename`

Body: `{tool, id, title}` with `Content-Type: application/json`, from the server's own origin.

- Saves `title` as the conversation's name in the marks file. Every server that shares the file shows it and finds it in a search.
- The name stays after new messages. It leaves the seen, completed and hidden marks as they were.
- Spaces and line breaks are joined into one line.
- On macOS, a running session's Terminal or Ghostty tab takes the name as its title. The tab is named after the 204. A tab that cannot be named keeps its title. The name stays on the tab after the session ends, and names the whole tab when it holds split panes.

Replies:

- Returns 204.
- A conversation outside the list returns 404.
- An empty name, a name longer than 120 characters, a name that may hold a credential, or any other field is a 400.

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

## When `path` is omitted

Omit `path` only when at most one editable settings spec exists. With none, a field save creates `settings/defaults.yaml`, or a permission save creates `settings/permissions.yaml`, for the selected audience. The response names the edited file.

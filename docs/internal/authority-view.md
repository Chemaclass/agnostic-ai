# Authority view

[Contributor docs](README.md) · [Architecture](architecture.md)

The Authority API resolves an agent or session's permissions for one coding tool, records the source rules, and previews changes. `serve` exposes it at `/api/views/authority`. The [browser client](../../ui/README.md) displays the overview's Authority section.

Keep [`internal/authority/types.go`](../../internal/authority/types.go), [`internal/overview/types.go`](../../internal/overview/types.go), and their TypeScript mirror, [`ui/src/contract.ts`](../../ui/src/contract.ts), in sync. Authority types are canonical. Changing a field's meaning requires a schema version bump; adding an optional field keeps `authority/v1`.

## Four states, kept apart

Each operation reports four separate states:

| State | Field | Comes from | Values |
|---|---|---|---|
| declared | `declared` | the specs, by neutral precedence | `allowed`, `ask`, `denied`, `unavailable`, `default` |
| effective | `effect` | the selected tool's adapter | the same, plus `unresolved` |
| installed | `installed` | comparing what sync would write with disk | `in-sync`, `stale`, `not-generated`, `not-checked`, `not-applicable` |
| runtime | `runtime` | nothing: a running tool is never observed | `not-observed` |

`resolution` explains how the declared rule becomes an effective decision:

| Value | Meaning |
|---|---|
| `translated` | The adapter writes the winning rule, and this repository documents how the tool matches it |
| `translated-unverified` | The adapter writes the rule, but matching is undocumented; the effect stays `unresolved` |
| `hook` | A generated hook enforces the rule |
| `not-expressed` | The adapter writes no rule |
| `agent-scope` | The agent spec decides, on a tool that reads agent tool lists |
| `no-rule` | No rule applies |
| `disabled` | The provider is switched off, and the tool honors that setting |

When the generated file is stale, missing, or unchecked, `hypothetical` is true. Use conditional wording: "Would be permitted once synced".

## Precedence

1. Agent narrowing comes first. A tool absent from the agent's `can`, `tools`, or `mcpServers`, or blocked by `readonly`, is `unavailable` whatever the project says. The grant names the excluding list.
2. Deny wins over ask, then allow, across settings files and both server-wide and tool-level MCP rules. Specificity does not change this order.
3. Protected paths contribute `ask` or `deny` grants on edit and write.
4. No grant means `default`: the tool's own default decides, and the view does not restate it.

## Evidence threshold

An effective decision requires a declared rule, an adapter translation, and documented matching behavior under `docs/site/content/docs/targets/`. Translation uses the same calls as `explain`: `adapters.TranslatePermissionCapabilityIn` and `TranslateAgentCapabilityIn`.

Whole-tool rules and exact MCP tool names qualify on every tool that writes them. Prefix, glob, and domain rules qualify only when the target page documents matching. Translations that widen access remain unresolved. Installed status requires a disk comparison; runtime behavior is not observed.

## Resources

| Kind | Basis | What establishes it |
|---|---|---|
| `file-pattern` | rule-pattern | a read, edit, or protected path pattern; `matches` resolves it against the project tree, bounded |
| `command-pattern` | rule-pattern | a shell rule, normalised to a command prefix |
| `web` | rule-pattern | fetch and search, or one allowed domain |
| `declared-root` | declared-root | an MCP `roots` entry with a `file://` URI; the [MCP specification](https://modelcontextprotocol.io/specification/2026-07-28/client/roots) defines roots as informational filesystem guidance, not access control |
| `undeclared-scope` | undeclared | an MCP server with no file root: "Scope not declared, credential reach not inspected" |

Other root URI schemes create no resource and raise `root-not-filesystem`. Named cloud resources are outside this model; they would need metadata that explicitly grants no access.

The API does not query MCP tool lists. It creates operations for tools named by a rule or the subject's `can`, plus "any other tool" with the server-wide effect and `tool-list-unknown`.

## Alternate routes

`alternateRoutes` lists other ways to reach the same system, such as a provider's shell command. An empty list does not rule out other routes.

`routeCoverage.checked` records decisions for the whole shell, scoped shell prefixes, and local files. `routeCoverage.notModeled` lists excluded routes: delegation, other MCP servers, web access, and runtime behavior absent from configuration.

## Secrets

Secrets are credentials referenced as `${NAME}`. The API reports four rights:

- `discover`: the name appears in configuration.
- `readValue`: unknown, with a reason; shell or file rules may provide another route.
- `use`: allowed when the subject can use the provider.
- `manage`: unknown; the provider handles rotation.

Display fields redact credential-named values, recognized tokens, and resolved secrets. Matching uses the private original rules; public identifiers stay distinct after redaction.

## Snapshot and revision

`snapshot.revision` hashes every config file and loaded spec in a fixed order. Authority and overview use the same function, so identical sources produce the same revision. `snapshot.evidence` records declared rules as loaded and installed files as checked or unchecked. Runtime is not observed.

## Proposal and impact

`POST /api/views/authority/impact` accepts a `Proposal`: subject, harness, optional revision, and `add` or `remove` changes to `allow`, `ask`, or `deny`. It changes a copy of the specs and rebuilds every subject on that harness without writing files.

`ImpactResult` returns `status: "proposed"`, changed operations, direction, affected subjects, and exact before/after file content. Direction ranks allowed above ask above denied, treating unavailable as denied. A `default` or `unresolved` value on either side makes direction `indeterminate`, with a reason. A revision mismatch returns a problem without recomputing.

Permission proposals are preview-only (`supportedActions` lists apply as unsupported). Edit the spec and run `sync` to apply one.

## Overview

`GET /api/views/overview` returns five configuration sections with items, counts, and tool support from adapter capability declarations:

| Section | Question | Items |
|---|---|---|
| Context | What is the agent told? | Instructions (`AGNOSTIC_AI.md`), rules, agents, skills; commands, review instructions, and ignore files only when present |
| Authority | What may the agent do? | Settings specs with permission rules or protected paths |
| Tools | What can the agent use? | MCP servers, each with `tools` from the cache `mcp tools` fills: `listed`, `stale` when the spec changed after the listing, `failed` with the reason, or `not-listed`; environments only when present |
| Lifecycle | What runs around the agent, and when? | Hooks, each with `group` set to its stage; `stages` lists every portable event in session order, with the configured tools that run it and their event names, even when no hook runs there, then one stage per native event a hook names |
| Settings | How are your tools set up? | Every shipped built-in with `enabled`, the config files read, and every other settings spec |

Settings and config files expose `values` by dotted key, typed `list`, `bool`, or `text`. Values under credential-named keys, recognized tokens, and 32-character key runs display as `<redacted>`. `${VAR}` references remain visible.

Lint and doctor findings attach to each matching file or containing skill folder. Unmatched findings attach to their named section, defaulting to Settings. The API stores findings on items and sections, without a separate list.

The same comparison as `sync --check` sets each target's `sync` state: `in-sync`, `drifted` with differing files, or `not-checked`.

## Endpoints

| Method | Path | Returns |
|---|---|---|
| GET | `/api/views/authority?agentId=<id>&target=<tool>` | `View` |
| POST | `/api/views/authority/impact` | `ImpactResult` for the posted `Proposal` |
| GET | `/api/views/authority/schema` | the schema version and endpoint list |
| GET | `/api/views/overview` | `overview.View` |
| GET | `/api/health` | `{ok, root, global, ui, projectRoot?}`, without loading specs; `projectRoot` only on a global server started in a project |
| POST | `/api/switch` | finds or starts the server for the other configuration and returns `{url, root, global, started}`; 409 `{error, missing, root}`, with no server started, when the user configuration folder has no setup |
| POST | `/api/global/create` | project servers only: `{git}` writes the starter files into the user configuration folder fixed at startup and replies `{root, created, git}`; 409 when a setup exists |
| POST | `/api/edit/builtin` | `{audience, name, enabled}` switches a built-in in the team config (`audience: shared`) or the personal one (`audience: personal`); `{audience: "personal", reset: true}` drops the personal list. Replies `{ok, file}`, or 422 with `problems` when the edit adds a validation problem and was undone |
| POST | `/api/edit/value` | `{audience, path, key, value}` sets a boolean at a dotted key in the shared settings spec or its personal overlay; `value: null` with `audience: personal` removes the personal value. Same replies |
| POST | `/api/edit/field` | `{audience, path?, key, target?, value, expected?}` sets `model`, `effort`, or global `session-retention-days`. `target` is `all` or one configured tool, which writes `model.<tool>` or `effort.<tool>`. Same replies, and 409 when `expected` no longer matches the value at that address |
| GET | `/api/changes/builtin` | the current built-in or setting preview and undo state for this server session |
| POST | `/api/changes/builtin/preview` | a project change plan for `{audience, name, enabled}`, `{audience: "personal", reset: true}`, or `{audience, setting: {path?, key, target?, value, expected?}}` for `model` or `effort`; the plan names its `audience` (and `scope`), and a setting plan its `key`, `target`, and `title` |
| POST | `/api/changes/builtin/apply` | writes the reviewed config and generated files for `{id}` |
| POST | `/api/changes/builtin/undo` | restores the files and permissions from before the last successful apply for `{id}` |
| POST | `/api/changes/builtin/cancel` | discards the preview `{id}` without writing files and keeps the last undo; 409 when that preview was already replaced or closed |

`/api/edit/builtin`, `/api/edit/value`, and `/api/edit/field` save source files without syncing. Only `/api/edit/field` accepts a `target` other than `all`; the other two return 400. Boolean inputs must be JSON booleans. Edits preserve unrelated text and file modes, validate before and after writing, and check for concurrent changes before replacement or rollback. Failed restorations are reported.

Shared edits write committed files. Personal edits write Git-ignored overlays and reject paths outside the local folder, inside team sources, or already tracked by Git. An empty local built-in list disables shared built-ins.

The change apply and undo endpoints also write generated files. See [prerequisites, conflicts, and recovery](../site/content/docs/cli-reference/inspect.md#serve).

`serve` binds to an explicit loopback IP and reloads configuration per request. Every route checks Host and Origin. Other browser origins require an exact `--allow-origin`; CLI requests without Origin are allowed. Request bodies have size limits and are read before taking the adapter lock. Header, read, write, and idle deadlines limit stalled clients.

`/api/switch` is the only route that starts a server. Its destination is fixed at startup: the user configuration for a project server, or the starting project for a global server. Requests carry no path, port, or argument. `/api/global/create` runs `git init` once in its fixed folder when asked.

Switching first looks on `127.0.0.1:4310` to `4319` for a server whose health names the destination configuration. A global server must also report the project it would find from the caller's project folder, or no project when none would be found. If none matches, the switch starts the same binary as `serve --addr <port> --global` or `serve --addr <port> --dir <project>`, with `--switch-back <this server's address>` and no `--allow-origin`. It stops the child when the parent stops, or on Linux when the parent dies. If two project pages compete for a port, the loser tries the next.

A child validates `--switch-back` as a loopback IP and port and tries it before the port range. This lets it return to a parent on any `--addr`. The address comes from the parent's listener, never from a request.

Server reuse trusts a local process that reports the expected root, as `ui` does. Another user on a shared machine could occupy the port first. Host/Origin checks prevent that process's page from calling this user's servers; requiring `Content-Type: application/json` also blocks cross-site forms.

Each server edits one configuration. The global server has a separate origin, preventing a project tab from editing `~/.agnostic-ai`. Requests cannot choose a configuration path.

Installed comparisons use builtin adapters only. The web server never executes an external adapter or probes an MCP server. The `mcp tools` command fills the listing cache separately. `--global` reads the user-level configuration from any directory; its overview uses global lint and output comparisons, without project doctor checks.

## Uncertainty codes

| Code | Raised when |
|---|---|
| `runtime-not-observed` | always |
| `target-not-expressed` | a configured tool cannot express a declared rule |
| `target-matching-unverified` | a tool writes the rule but its matching is not documented |
| `provider-scope-undeclared` | an MCP server has no file root |
| `credential-reach-not-inspected` | any MCP provider |
| `root-not-filesystem` | a root's URI is not a file URI |
| `tool-list-unknown` | tools come from rules, not from the server |
| `pattern-containment-approximate` | a scoped file rule exists |
| `alternate-route-not-modeled` | always, since unmodeled routes exist |
| `secret-read-unknown` | any secret reference |
| `installed-not-checked` | the caller skipped the disk comparison |
| `domain-defaulted` | an unknown provider fell to AI & Agent Operations |

## Illustrative data

[`internal/authority/testdata/demo-project`](../../internal/authority/testdata/demo-project) contains three example agents, permission lists, protected paths, MCP servers, and secret references. It supplies the client fixture `ui/src/fixtures/overview.mock.json`.

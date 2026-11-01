# Authority view

[Contributor docs](README.md) · [Architecture](architecture.md)

The Authority view resolves an agent or session's permissions for one coding tool. The server resolves rules, records their sources, and analyzes proposed changes. `serve` hosts the versioned JSON document at `/api/views/authority`. The [browser client](../../ui/README.md) displays the configuration overview, including its Authority section.

The contract lives in three places that change together: [`internal/authority/types.go`](../../internal/authority/types.go) (canonical), [`internal/overview/types.go`](../../internal/overview/types.go) (the overview), and [`ui/src/contract.ts`](../../ui/src/contract.ts) (the TypeScript mirror). A field changes meaning only with a schema version bump; adding an optional field keeps `authority/v1`.

## Four states, kept apart

Every operation carries four values. Do not combine them in one badge.

| State | Field | Comes from | Values |
|---|---|---|---|
| declared | `declared` | the specs, by neutral precedence | `allowed`, `ask`, `denied`, `unavailable`, `default` |
| effective | `effect` | the selected tool's adapter | the same, plus `unresolved` |
| installed | `installed` | comparing what sync would write with disk | `in-sync`, `stale`, `not-generated`, `not-checked`, `not-applicable` |
| runtime | `runtime` | nothing: a running tool is never observed | `not-observed` |

`resolution` says how the step from declared to effective went: `translated` (the adapter writes the winning rule and the tool's matching for that rule form is documented in this repository), `translated-unverified` (the rule is written but its matching is not documented, so the effect stays `unresolved`), `hook` (a generated hook enforces it), `not-expressed` (the adapter writes nothing for it), `agent-scope` (the subject's own spec decides, on a tool that reads agent tool lists), `no-rule`, or `disabled`.

`hypothetical` is true while the generated file is stale, missing, or unchecked. A client then uses conditional wording: "Would be permitted once synced".

## Precedence

1. Agent narrowing comes first. A tool absent from the agent's `can`, `tools`, or `mcpServers`, or blocked by `readonly`, is `unavailable` whatever the project says. The grant names the excluding list.
2. Then deny over ask over allow, across settings files and across server-wide and tool-level MCP rules alike. Specificity never overrides.
3. Protected paths contribute `ask` or `deny` grants on edit and write.
4. No grant means `default`: the tool's own default decides, and the view does not restate it.

## Evidence threshold

A permitted, approval-required, or blocked badge is effective only when a loaded spec declares the rule, the selected adapter translates it, and the tool's matching behavior for that rule form is documented under `docs/site/content/docs/targets/`. Translation uses the same calls as `explain`: `adapters.TranslatePermissionCapabilityIn` and `TranslateAgentCapabilityIn`. Whole-tool rules and exact MCP tool names qualify on every tool that writes them. Prefix, glob, and domain forms qualify only where the target page documents how they match. A translation reported as widening stays unresolved. Installed status requires a disk comparison. Runtime behavior is never claimed.

## Resources

| Kind | Basis | What establishes it |
|---|---|---|
| `file-pattern` | rule-pattern | a read, edit, or protected path pattern; `matches` resolves it against the project tree, bounded |
| `command-pattern` | rule-pattern | a shell rule, normalised to a command prefix |
| `web` | rule-pattern | fetch and search, or one allowed domain |
| `declared-root` | declared-root | an MCP `roots` entry with a `file://` URI; the [MCP specification](https://modelcontextprotocol.io/specification/2026-07-28/client/roots) defines roots as informational filesystem guidance, not access control |
| `undeclared-scope` | undeclared | an MCP server with no file root: "Scope not declared, credential reach not inspected" |

A root with another URI scheme creates no resource and raises the `root-not-filesystem` uncertainty. Named cloud resources need their own explicitly non-enforcing metadata, which is a separate proposal.

Tool lists are not queried. An MCP tool appears as an operation when a rule or the subject's `can` names it, plus one "any other tool" operation at the server-wide effect, with the `tool-list-unknown` uncertainty.

## Alternate routes

`alternateRoutes` lists ways to reach the same system outside this operation's rules, such as shell access to a provider's command-line client. An empty list does not prove that no other route exists. `routeCoverage.checked` lists each evaluated route and the subject's decision: shell as a whole, each scoped shell prefix, and local files. `routeCoverage.notModeled` lists what the view does not evaluate: delegation, other MCP servers, web access, and runtime behavior absent from configuration.

## Secrets

A secret is a credential the configuration references by name through `${NAME}`. Four rights are reported separately: `discover` (the name is in the configuration), `readValue` (reported `unknown` with the reason, since a shell or file rule may reach it another way), `use` (allowed when the provider is available to the subject), and `manage` (unknown; rotation happens in the provider). Display fields redact credential-named values, recognized tokens, and resolved secret values. Matching retains private original rules; public identifiers remain distinct after redaction.

## Snapshot and revision

`snapshot.revision` is a short hash over every input used in resolution: the config files and every loaded spec, in a fixed order. The overview builds its snapshot through the same function, so both views report one revision for the same sources. `snapshot.evidence` states coverage per class: declared is always loaded, installed is checked or not, runtime is not observed.

## Proposal and impact

`POST /api/views/authority/impact` takes a `Proposal` (subject, harness, optional revision, and `add` or `remove` changes on the `allow`, `ask`, or `deny` lists), applies it to a copy of the specs, rebuilds every subject on the harness, and returns an `ImpactResult` with `status: "proposed"`, each operation whose declared or effective effect changes, the direction, affected subjects, and the exact before and after of the edited file. Nothing is written. Direction is ranked only between known decisions (allowed over ask over denied, with unavailable counted as denied); any side that is `default` or `unresolved` is `indeterminate` with a reason. A revision mismatch is reported as a problem, not recomputed.

Permission proposals are preview-only (`supportedActions` lists apply as unsupported). Edit the spec and run `sync` to apply one.

## Overview

`GET /api/views/overview` returns five harness sections with their configured items, counts, and per-tool support from the adapters' capability declarations:

| Section | Question | Items |
|---|---|---|
| Context | What is the agent told? | Instructions (`AGNOSTIC_AI.md`), rules, agents, skills; commands, review instructions, and ignore files only when present |
| Authority | What may the agent do? | Settings specs with permission rules or protected paths |
| Tools | What can the agent use? | MCP servers, each with `tools` from the cache `mcp tools` fills: `listed`, `stale` when the spec changed after the listing, `failed` with the reason, or `not-listed`; environments only when present |
| Lifecycle | What runs around the agent, and when? | Hooks, each with `group` set to its stage; `stages` lists every portable event in session order, with the configured tools that run it and their event names, even when no hook runs there, then one stage per native event a hook names |
| Settings | How is the harness configured? | Every shipped built-in with `enabled`, the config files read, and every other settings spec |

Settings specs and config files carry `values`: one entry per dotted key, typed `list`, `bool`, or `text`, so a client can render chips, a switch, or text. A value whose key names a credential, or that looks like a known token format or a 32-character key run, shows as `<redacted>`; a `${VAR}` reference shows as written.

Read-only check results appear on affected items or sections, never in a separate list. A lint or doctor finding appears on every item whose file it names, or whose skill folder contains that file. If it names no configured item, it appears on its stated section, or Settings when no section is stated. The capture-and-compare path used by `sync --check` sets each target's `sync` state: `in-sync`, `drifted` with the files that differ, or `not-checked`.

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

`/api/edit/builtin`, `/api/edit/value`, and `/api/edit/field` write source files without syncing. Only `/api/edit/field` takes a `target` other than `all`; the other two return 400 for one. They require explicit JSON booleans, preserve unrelated text and file modes, and validate before and after the edit. Snapshots detect concurrent changes before replacement or rollback; failed restorations are reported. A shared edit writes the committed file; a personal edit writes the Git-ignored overlay, and is refused when its file resolves outside the local folder, into a team source, or to a tracked file. An empty local built-in list disables shared built-ins.

The change apply and undo endpoints also write generated files. See [prerequisites, conflicts, and recovery](../site/content/docs/cli-reference/inspect.md#serve).

`serve` binds to an explicit loopback IP and reloads the configuration on every request. One shared Host/Origin guard protects every route. Other browser origins require an exact `--allow-origin`; no-Origin CLI requests remain allowed, while cross-site browser requests are refused. Bodies are bounded and read before the adapter serialization lock. Header, read, write and idle deadlines limit stalled clients.

`/api/switch` is the only route that starts a server. `/api/global/create` runs `git init` once, in the folder fixed at startup, when asked. The switch target is fixed when the server starts: the user configuration for a project server, or the project a global server started in. The request carries no path, port, or argument. It reuses a server whose health names that configuration on `127.0.0.1:4310` to `4319` (for the user configuration, one whose `projectRoot` is this project), or runs the same binary as `serve --addr <port> --global` or `serve --addr <port> --dir <project>`, plus `--switch-back <this server's address>`, with no `--allow-origin`, and stops it when the parent stops, or on Linux when the parent dies. A started global server counts only when its health names the project it would find from this project's folder, or no project when it would find none, so when two project pages start one on the same port at once, the one that loses the port tries the next. A started server checks that `--switch-back` is a loopback IP and port, and tries it before the range, so the way back finds a parent on any `--addr`; the address comes from the parent's own listener, never from a request. Reuse trusts any local process that answers health with that root, as `ui` reuse does: on a shared machine another user could bind the port first, but its page cannot call this user's servers, because the guard refuses its origin. It sits behind the same Host/Origin guard and also requires `Content-Type: application/json`, so a cross-site form cannot reach it. A project tab still cannot edit `~/.agnostic-ai`: the global server is another origin, and the guard refuses the project page's requests to it. Edits keep one configuration per server; a per-request configuration field would let a project tab change every project's user configuration.

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

[`internal/authority/testdata/demo-project`](../../internal/authority/testdata/demo-project) is an illustrative project with three agents, allow, ask, and deny lists, protected paths, MCP servers, and secret references. Every name in it is an example. The client's fixture `ui/src/fixtures/overview.mock.json` is generated from it.

# Authority view

[Contributor docs](README.md) · [Architecture](architecture.md)

The Authority view answers one question for one subject on one coding tool: which exact action on which exact target is permitted, with what conditions, and why. It is one versioned JSON document that the `view authority` command prints and `serve` hosts, and that the client under [`ui/`](../../ui/README.md) renders. The server owns normalization, precedence, provenance, and impact analysis; the client owns rendering, selection, and motion.

The contract lives in three places that change together: [`internal/authority/types.go`](../../internal/authority/types.go) (canonical), [`internal/overview/types.go`](../../internal/overview/types.go) (the overview), and [`ui/src/contract.ts`](../../ui/src/contract.ts) (the TypeScript mirror). A field changes meaning only with a schema version bump; adding an optional field keeps `authority/v1`.

## Four states, kept apart

Every operation carries four values that are never merged into one badge.

| State | Field | Comes from | Values |
|---|---|---|---|
| declared | `declared` | the specs, by neutral precedence | `allowed`, `ask`, `denied`, `unavailable`, `default` |
| effective | `effect` | the selected tool's adapter | the same, plus `unresolved` |
| installed | `installed` | comparing what sync would write with disk | `in-sync`, `stale`, `not-generated`, `not-checked`, `not-applicable` |
| runtime | `runtime` | nothing: a running tool is never observed | `not-observed` |

`resolution` says how the step from declared to effective went: `translated` (the adapter writes the winning rule and the tool's matching for that rule form is documented in this repository), `translated-unverified` (the rule is written but its matching is not documented, so the effect stays `unresolved`), `hook` (a generated hook enforces it), `not-expressed` (the adapter writes nothing for it), `agent-scope` (the subject's own spec decides, on a tool that reads agent tool lists), `no-rule`, or `disabled`.

`hypothetical` is true while the generated file is stale, missing, or unchecked. A client then words the effect conditionally: "Would be permitted once synced".

## Precedence

1. Agent narrowing comes first. A tool absent from the agent's `can`, `tools`, or `mcpServers`, or blocked by `readonly`, is `unavailable` whatever the project says. The grant names the excluding list.
2. Then deny over ask over allow, across settings files and across server-wide and tool-level MCP rules alike. Specificity never overrides.
3. Protected paths contribute `ask` or `deny` grants on edit and write.
4. No grant means `default`: the tool's own default decides, and the view does not restate it.

## Evidence threshold

A permitted, approval-required, or blocked badge is shown as effective only when a loaded spec declares the rule and the selected tool's adapter translates it, through the same calls `explain` uses (`adapters.TranslatePermissionCapabilityIn` and `TranslateAgentCapabilityIn`), and the tool's matching for that rule form is documented under `docs/site/content/docs/targets/`. Whole-tool rules and exact MCP tool names qualify on every tool that writes them; prefix, glob, and domain forms qualify only where the target page documents how they match. Any translation the adapter reports as widening stays unresolved. Installed requires a disk comparison. Runtime is never claimed.

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

`alternateRoutes` lists routes that could reach the same system and that the operation does not govern, such as shell access to a provider's command-line client. It is never a claim of absence: `routeCoverage.checked` lists every route the view evaluated with the subject's decision on it (shell as a whole, each scoped shell prefix, and local files), and `routeCoverage.notModeled` names what the view does not evaluate (delegation to other agents, other MCP servers, web access, anything a running tool does that configuration does not describe).

## Secrets

A secret is a credential the configuration references by name through `${NAME}`. Four rights are reported separately: `discover` (the name is in the configuration), `readValue` (reported `unknown` with the reason, since a shell or file rule may reach it another way), `use` (allowed when the provider is available to the subject), and `manage` (unknown; rotation happens in the provider). No value appears in any payload, log, or diff.

## Snapshot and revision

`snapshot.revision` is a short hash over every input used in resolution: the config files and every loaded spec, in a fixed order. The overview builds its snapshot through the same function, so both views report one revision for the same sources. `snapshot.evidence` states coverage per class: declared is always loaded, installed is checked or not, runtime is not observed.

## Proposal and impact

`POST /api/views/authority/impact` takes a `Proposal` (subject, harness, optional revision, and `add` or `remove` changes on the `allow`, `ask`, or `deny` lists), applies it to a copy of the specs, rebuilds every subject on the harness, and returns an `ImpactResult` with `status: "proposed"`, each operation whose declared or effective effect changes, the direction, affected subjects, and the exact before and after of the edited file. Nothing is written. Direction is ranked only between known decisions (allowed over ask over denied, with unavailable counted as denied); any side that is `default` or `unresolved` is `indeterminate` with a reason. A revision mismatch is reported as a problem, not recomputed.

Applying is listed under `supportedActions` as unsupported. Editing the spec and running `sync` is the only way to apply; an API apply needs revision checks, approval, and rollback first.

## Overview

`GET /api/views/overview` returns five harness sections with their configured items, counts, and per-tool support from the adapters' capability declarations:

| Section | Question | Items |
|---|---|---|
| Context | What is the agent told? | Instructions (`AGNOSTIC_AI.md`), rules, agents, skills; commands, review instructions, and ignore files only when present |
| Authority | What may the agent do? | Settings specs with permission rules or protected paths; the deep view resolves them |
| Tools | What can the agent use? | MCP servers; environments only when present |
| Lifecycle | What runs around the agent, and when? | Hooks, each with `group` set to its lifecycle point: the portable `on` event in session order, then native events |
| Settings | How is the harness configured? | Enabled built-ins, the config files read, and every other settings spec |

Read-only check results are shown on what they concern, never as a separate list. A lint or doctor finding sits on every item whose file it names, or whose skill folder holds that file; one that names no configured item sits on the section it states, else on Settings. Drift, from the capture-and-compare path `sync --check` uses, sets each target's `sync` state: `in-sync`, `drifted` with the files that differ, or `not-checked`.

## Endpoints

| Method | Path | Returns |
|---|---|---|
| GET | `/api/views/authority?agentId=<id>&target=<tool>` | `View` |
| POST | `/api/views/authority/impact` | `ImpactResult` for the posted `Proposal` |
| GET | `/api/views/authority/schema` | the schema version and endpoint list |
| GET | `/api/views/overview` | `overview.View` |
| GET | `/api/health` | `{ok, revision}` |

`serve` reloads the project on every request, serializes API requests because rendering adapters swaps package-level warning state, allows cross-origin requests from `localhost` and `127.0.0.1` for a dev server, and serves the built client from `--ui` with a single-page fallback. `--global` reads the user-level configuration from any directory. Its overview runs `lint --global` and compares the files `sync --global` would write with disk, as `sync --global --check` does; the doctor checks read a project and are skipped, and the evidence note says so.

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

[`internal/authority/testdata/demo-project`](../../internal/authority/testdata/demo-project) is an illustrative project with three agents, allow, ask, and deny lists, protected paths, MCP servers, and secret references. Every name in it is an example. The client's fixtures under `ui/src/fixtures/` are generated from it.

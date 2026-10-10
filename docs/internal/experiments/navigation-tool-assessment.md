# CodeGraph, Ponytail and Headroom: what helps this repo?

Keep CodeGraph as a small navigation pilot. Skip Ponytail and Headroom for now. The trial produced correct source answers, but it did not prove that any tool saves time, money or model input. Related to #2024.

| Tool | Decision | Why and next step |
|---|---|---|
| CodeGraph | Pilot | It helps find source relationships, but answers still need source checks. Test a supported contributor setup and measure a clear benefit before adopting it. |
| Ponytail | Skip | Its small-change and reuse guidance overlaps our rules. Revisit only if normal review misses known defects that its adapted prompt finds. |
| Headroom | Skip | Adding its server does not compress other tools' replies. Revisit when a real noisy-output case needs compression before the model reads it, with the original text still recoverable. |

## What the answers established

[compareTargets](https://github.com/Chemaclass/agnostic-ai/blob/571f2fabed7efba5f8655a49637f275cd5655247/internal/cli/compare.go#L169) sends permission fields to [classifyPermissions](https://github.com/Chemaclass/agnostic-ai/blob/571f2fabed7efba5f8655a49637f275cd5655247/internal/cli/compare_permissions.go#L94). It captures generated settings, translates a rule for its target, then captures settings without that rule. [captureEmit](https://github.com/Chemaclass/agnostic-ai/blob/571f2fabed7efba5f8655a49637f275cd5655247/internal/cli/render.go#L218) keeps these writes in memory. A matching rule in the full output can still count when removing one duplicate makes no visible difference.

Codex needs outputs.codex.exec-policies-from-permissions: true to translate portable shell rules. [Explicit native policies](https://github.com/Chemaclass/agnostic-ai/blob/571f2fabed7efba5f8655a49637f275cd5655247/internal/adapters/codex/permission_policies.go#L123), including an empty list, take precedence. OpenCode applies [native overrides last](https://github.com/Chemaclass/agnostic-ai/blob/571f2fabed7efba5f8655a49637f275cd5655247/internal/adapters/opencode/permission.go#L102), per tool.

All four answers traced these paths correctly. They distinguished generated output from what the target enforces when it runs and identified unsupported project default-mode and partial Codex command-prefix translation. Both graph answers checked missing or ambiguous relationships against source. The [comparison tests](https://github.com/Chemaclass/agnostic-ai/blob/571f2fabed7efba5f8655a49637f275cd5655247/internal/cli/compare_permissions_test.go) cover opt-in, empty policies, duplicates, partial lists and overrides.

## History and next step

The [earlier assessment](https://github.com/Chemaclass/agnostic-ai/blob/06cbea998412a40de57447ead04dfc25d16e1ab8/docs/internal/experiments/navigation-tool-assessment.md) records the initial trial in full. That revision did not support permission comparison; the model was unrecorded and keeping CodeGraph unavailable during ordinary search relied on instructions. The second trial addresses those gaps.

Independent reviews checked the source answers, usage, tool restrictions and source copies, and confirmed that all 59 saved evidence files matched their recorded checksums. Local transcripts, prompts, flags, profiles and freshness results remain outside public commits because they include temporary host paths.

Any supported contributor setup or product integration needs a separate issue covering activation, cleanup, file ownership and licensing. A performance claim needs a runner committed before measurement, following the [existing method](trio-evaluation.md). This assessment changes documentation only.

<details>
<summary>Trial record and measurements</summary>

## Tools checked

CodeGraph [1.6.2](https://github.com/colbymchenry/codegraph/releases/tag/v1.6.2) was checked at source commit 6560052a6f856855d3f71eee838fd66ccfa4285d. The macOS arm64 archive matched GitHub's SHA-256: d74d1bfb4060db63ec3c2b72c4e17f76c31978af3bad6ace4370d1501ac0662e. The executed CLI reported 1.6.2; bundled Node reported v24.16.0. It is [MIT licensed](https://github.com/colbymchenry/codegraph/blob/6560052a6f856855d3f71eee838fd66ccfa4285d/LICENSE). Its [language list](https://github.com/colbymchenry/codegraph/blob/6560052a6f856855d3f71eee838fd66ccfa4285d/site/src/content/docs/reference/languages.md) includes Go, TypeScript and Kotlin, but not Markdown or YAML. Its installer writes native tool files, so we used temporary configuration instead.

Ponytail was inspected at [9b58c1f](https://github.com/DietrichGebert/ponytail/tree/9b58c1ffb790c075ca32e70a89cf1d80588f4abf), version 5.1.0, MIT. Its [prompt](https://github.com/DietrichGebert/ponytail/blob/9b58c1ffb790c075ca32e70a89cf1d80588f4abf/skills/ponytail/SKILL.md) overlaps our [Go rules](../../../.agnostic-ai/rules/go-style.md) and [review requirements](../../../.agnostic-ai/skills/pr-sweep/SKILL.md), and prescribes closing notes and comments that conflict with our conventions. No hooks were installed and no extra review benefit is claimed.

Headroom was inspected at [976aa714](https://github.com/headroomlabs-ai/headroom/tree/976aa714ef3e19174dc0cb586cee7b20d064e4ef), version 0.40.0, Beta, [Apache-2.0](https://github.com/headroomlabs-ai/headroom/blob/976aa714ef3e19174dc0cb586cee7b20d064e4ef/LICENSE). Its [package](https://github.com/headroomlabs-ai/headroom/blob/976aa714ef3e19174dc0cb586cee7b20d064e4ef/pyproject.toml) needs Python 3.10 or newer and a Rust build. Its [server](https://github.com/headroomlabs-ai/headroom/blob/976aa714ef3e19174dc0cb586cee7b20d064e4ef/headroom/ccr/mcp_server.py) offers explicit compression and original-text retrieval with a one-hour session lifetime. Compressing output after the model has read it cannot recover that input cost. No runtime or retrieval test was needed for the skip decision.

## Navigation trial

On 2026-10-10, two fresh ordinary-search sessions and two fresh CodeGraph sessions answered the same question on public revision [571f2fab](https://github.com/Chemaclass/agnostic-ai/tree/571f2fabed7efba5f8655a49637f275cd5655247). Their order was ordinary, graph, graph, ordinary. All four answers were checked against source and existing tests.

Three temporary source copies matched all 2,551 tracked files at that revision, allowing only the declared Windows line endings in gradlew.bat. Their source archive SHA-256 was a3538ba49c9c8ee3aadbbf40f9fb45cfe5fc3e419d7fe5ae3cb31711d7d56970. Generated graph configuration and indexes were separate additions.

The host was macOS 27.0 arm64 with Codex CLI 0.162.1. Every session explicitly selected gpt-6.1-sol with medium reasoning. The provider's internal model build is unknown. RTK and Caveman activation was neither assumed nor changed. No personal configuration, account setup or credentials were inspected or changed.

Each session used --no-daemon, --ignore-user-config, --ignore-rules, --ephemeral, --json and project_doc_max_bytes=0. Both workflows had the same source-reading commands. Only graph sessions registered CodeGraph through MCP, the host's tool connection. The exact shared prompt was:

> Use only the current public repository fixture. Establish a source-verified answer to how compare turns a portable settings permission entry into target output evidence at this pinned revision, including the Codex output opt-in and a target override. Trace the actual functions and report file:line evidence, limitations and completeness. Verify claimed graph relationships against source. Do not modify source, install anything, access credentials or other projects, or run tests/builds. Use at most 12 tool calls and finish within 140 seconds. Return a concise answer with source citations and any unresolved uncertainty.

Each session had a hard five-minute limit and an output-size limit, within a 35-minute investigation ceiling. All four finished within 77 seconds and twelve calls. The CLI could not cap or report billed currency.

## Keeping ordinary search separate

A macOS execution profile covered the CLI and its child processes. Ordinary-search sessions could run only approved Codex binaries and source-reading programs. CodeGraph, Node, npm, npx, Python, Ruby and a new executable were rejected. Existing graph files and indexes could not be read through either temporary-path alias. Source reads succeeded; source writes failed. Direct commands, child-shell tests and an actual host diagnostic confirmed these restrictions. Network access stayed available in both workflows.

The CLI's own sandbox was disabled inside this profile because combining the two sandboxes failed. Graph sessions additionally allowed the pinned CodeGraph runtime and writes to its index. The later freshness-test index was added to the second ordinary session's denied paths before it ran. The model, question and public source stayed fixed. These checks cover this trial's tool separation, not general sandbox security.

Graph configuration set DO_NOT_TRACK=1, CODEGRAPH_NO_UPDATE_CHECK=1, CODEGRAPH_NO_DAEMON=1 and CODEGRAPH_NO_WATCH=1, with --no-watch. The exclusions were tests/integration/fixtures/, internal/adapters/**/testdata/, docs/site/ and editors/**/test-fixtures/. The release's [telemetry controls](https://github.com/colbymchenry/codegraph/blob/6560052a6f856855d3f71eee838fd66ccfa4285d/TELEMETRY.md) were checked; network traffic was not audited.

## Recorded results and limits

| Session | CLI seconds | Commands (nonzero results) | Graph calls | Input / cached input / output tokens |
|---|---:|---:|---:|---:|
| Ordinary 1 | 76.121 | 10 (1) | 0 | 564,011 / 487,424 / 3,070 |
| Graph 1 | 56.860 | 7 (2) | 2 | 491,917 / 407,424 / 2,336 |
| Graph 2 | 72.318 | 10 (2) | 2 | 685,860 / 592,896 / 3,078 |
| Ordinary 2 | 63.138 | 9 (2) | 0 | 456,052 / 381,312 / 2,596 |

Counts come from completed CLI events. Cached input is part of input. A command can read several files; nonzero results include missing paths, absent Git metadata, blocked commands and failed searches. Successful CLI time totalled 268.437 seconds. The shared window including coordination and independent source checks was 392.711 seconds; checking time was not allocated per workflow. Provider retries, billed currency and separate startup-context tokens remain unknown. No Headroom retrieval ran.

The original initial-index record was overwritten. Its initial time is unknown; an already-initialized repeat took 0.170 seconds and is not a fresh measurement. The separate freshness-test index took 3.627 seconds and indexed 1,513 files. Download and release-check time were not timed.

Four earlier setup sessions failed because an approved tool-host binary was missing from the execution profile. Their separate totals were 76.917 seconds, 264,023 input tokens and 1,957 output tokens. Missing daemon controls also caused two startup failures from a locked index. The experiment-owned daemon was stopped and explicit server environment settings fixed the cause. A later host boundary check took 17.078 seconds and used 58,655 input tokens (45,568 cached) and 704 output tokens. Its recorded commands confirmed allowed source reads and denied graph-file reads. These setup and checking costs are separate from the four successful sessions.

Fresh server connections found an exact Go declaration, found its new name after an edit, then stopped returning it after deletion. Checks excluded the query heading and unrelated fuzzy matches. Query times were 0.501, 0.549 and 0.500 seconds. This tests startup/query freshness for one Go edit/delete sequence, not watchers, branch switches or every language.


</details>

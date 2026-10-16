# Observable audit metrics

Record one run in `audit-metrics.json` and include `scripts/audit-metrics.sh report <run-dir>` in its report. The recorder needs Bash and jq, runs offline, and does not change routing. The coordinator is the only writer. Keep each run directory separate; an existing artifact is never replaced by `init`.

Initialize before fetching. Choose a stable run ID, matching the run manifest when one exists. Supply observed provenance as JSON: audited commit, worktree state, requested targets, audit window, built-in baseline, source lock revision or hash, execution policy, and scenario (`clean`, `vendor-change`, or `builtin-only`). Unknown values stay null; a scenario is a description of the inputs, not an outcome.

```sh
run=local/target-audit/2026-10-16-run
scripts/audit-metrics.sh init "$run" 2026-10-16-example '{"audited_commit":"<commit>","worktree_state":"clean","targets":["codex"],"audit_window":"<start>..<end>","builtin_baseline":"<commit>","source_lock":"<hash>","execution_policy":"<approved policy reference>","scenario":"vendor-change"}'
```

`init` and `finish` use the current Unix time, or accept an explicit final argument in Unix seconds for an observed timestamp. JSON records accept `-` to read stdin. Do not reconstruct times or usage from prose, token estimates, or missing logs.

## Intervals and effort

Record a stage when it starts, then replace the same ID with its observed end and status. Use distinct IDs for parallel batches. A `running` stage has a null end. A `completed` or `interrupted` stage has an observed end. If the interruption time is unknown, keep the stage running and record that limit.

```sh
scripts/audit-metrics.sh record "$run" stage '{"id":"review-codex","phase":"audit","name":"review","start":1792141200,"end":null,"status":"running"}'
scripts/audit-metrics.sh record "$run" stage '{"id":"review-codex","phase":"audit","name":"review","start":1792141200,"end":1792141230,"status":"completed"}'
```

Capture fetch, recovery, triage, review, confirmation, and synthesis intervals as they happen. `jev-triage.sh` also records its own interval in `triage-metrics.json`. Include it as a stage in the coordinator record when comparing all stages. Do not add it twice under different IDs.

Finish the audit after synthesis and its report, before any later authorized implementation or merge work. Record those later stages with `phase: "delivery"`. Audit stages must fit within the observed run bounds; `finish` rejects an end before a recorded audit stage start or end. An unclosed stage keeps its unknown end. Delivery intervals are recorded separately without audit bounds. The report shows each interval and computes audit elapsed time from the run start to its explicit finish. It also shows the union of closed audit intervals and their overlap, never the sum of parallel durations as wall time. Unclosed intervals remain unknown and are excluded from the union.

Auditor usage and effort are optional observed records, keyed by auditor ID. Record an ID for each auditor that ran, even when every value is unknown. A missing field becomes null. Zero means a measured zero. Any missing contributor makes the aggregate unknown; the artifact retains each known value.

```sh
scripts/audit-metrics.sh record "$run" usage '{"id":"codex-reviewer","model":"<model>","input_tokens":null,"output_tokens":null,"effort_seconds":null}'
```

## Routing and findings

Record one review decision per requested target after applying the approved coverage rules. Decisions are `deep`, `omitted`, or `sampled`. Each needs a reason; omitted and sampled decisions also need `approval`, pointing to the approved policy or explicit decision. `builtin_override: true` marks a built-in change that kept deep review despite a possible fast path. Such a target cannot be omitted or sampled.

```sh
scripts/audit-metrics.sh record "$run" review '{"target":"codex","decision":"deep","reason":"changed built-in hook","builtin_override":true}'
scripts/audit-metrics.sh record "$run" review '{"target":"cursor","decision":"omitted","reason":"eligible unchanged coverage","approval":"<approved policy reference>"}'
scripts/audit-metrics.sh record "$run" review '{"target":"kiro","decision":"sampled","reason":"random eligible target","approval":"<approved policy reference>"}'
```

The counters describe recorded routing, not causal time savings. An all-built-in-deep run can have triage leads and token cost while omitting no reviews. A sample is a review that happened and does not count as omitted.

Record confirmed, rejected, and unresolved candidates with an evidence path. Link a known lead by its file and one-based row number. Preserve that file; do not rerun triage in the completed run directory. A missing lead is null, including findings discovered independently of triage. A lead association does not prove the finding would have been missed without it.

```sh
scripts/audit-metrics.sh record "$run" finding '{"id":"codex-hooks","outcome":"confirmed","lead":{"file":"triage.tsv","line":2},"evidence":"codex-hooks.md"}'
scripts/audit-metrics.sh record "$run" finding '{"id":"cursor-candidate","outcome":"rejected","evidence":"cursor-candidate.md"}'
scripts/audit-metrics.sh record "$run" limit 'One vendor page remained unreadable after recovery'
scripts/audit-metrics.sh finish "$run"
scripts/audit-metrics.sh report "$run"
```

## Report and comparison limits

Both JSON artifacts carry `schema_version: 1` and run identity. `triage-metrics.json` inherits identity from `audit-metrics.json` when initialized first; otherwise its run ID is the directory name. It records mode, request counts, returned token usage, and fallback or cap limits. `planned` counts generated requests; `capped` counts additional requests omitted by the cap. `unanswered` includes generated requests that failed or were never sent after a fatal response. Token totals are null if any generated request lacks usage; known subtotals remain available. Failed attempts and retries can incur usage the API did not return, so these counts are not a billing total. No-key runs generated no requests and have zero request usage. A missing artifact is reported as unavailable.

Add the recorder summary to the audit report, link both artifacts, and name missing stages, routing rows, auditor usage, source evidence, or other research limits. The report accepts a triage sidecar only when its schema and run ID match and its summary fields are valid. Missing, malformed, or mismatched sidecars are unavailable and appear as research limits. Record the no-change early return too. The helper does not prove coverage completeness or authorize a fast path. If metrics recording fails, continue the audit and state that metrics are unavailable.

Compare representative runs with their provenance, source changes, built-in invalidations, coverage policy, samples, and confirmed outcomes visible. Keep audit duration separate from later fixes and merges. Differences in elapsed time are observations, not proof that triage caused a speedup. Replay remains optional for measurement work; isolated-excerpt recall is not full-run recall. Threshold or question changes still follow `docs/internal/jev-triage.md`.

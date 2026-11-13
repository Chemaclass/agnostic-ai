# Prepare an audit run

Create a dedicated audit worktree first using the isolation entry command in `scripts/target-audit/manifest.md`. Keep evidence in the persistent directory it records. Generate native instructions before starting the manifest. Record whether the audit uses the current committed tree or an explicitly selected revision; never silently omit requested working changes.

Run `scripts/target-facts.sh --list` inside that worktree. Record the audited commit, worktree state, and open PRs before delegating.

Take the newest date from the latest completed local report, legacy `extra.audit_marker`, audit issue creation, or commit that records an audit's source corrections in `.agnostic-ai/skills/target-audit/references/sources.md`. A commit that only trims or restructures that file, such as #1109's, is not an audit and does not move the window. Widen it slightly for changelog overlap and record the chosen window. Scratch reproductions and incomplete runs are not completed reports.

Select the built-in baseline using `.agnostic-ai/skills/target-audit/references/builtins.md`, then start a unique audit run using `scripts/target-audit/manifest.md`. Freeze the requested target-kind-URL set before fetching and use that run path below wherever `<date>-run` appears.

Reuse the `scripts/audit-manifest` binary built for isolation, as `scripts/target-audit/manifest.md` describes. Give its absolute path to auditors and reuse it for coordinator commands. Ingest each completed structured target result instead of transcribing its individual source outcomes; keep manual recording for fast paths and older checkpoints.

Initialize the run metrics before fetching, using `.agnostic-ai/skills/target-audit/references/metrics.md`. Record observed stages, routing decisions, usage, and outcomes there, including a no-change early return.

Export `TARGET_FACTS_REVIEW_HELPER="$audit_tools/audit-manifest"` immediately after building the current helper, as the manifest setup shows. Without a usable helper, routing stays conservative.

Fetch once and capture the fetched evidence using `scripts/target-audit/manifest.md`. The fetch saves fresh bodies, comparison hashes and statuses against `scripts/target-audit/sources.lock`. Read its summary and failed rows before delegating.

After capture, run `<helper> comment-reviews <run>`. Review each pending baseline/path pair once using its saved before/after source. Check changed prose for native behavior claims against this run's vendor evidence. Save a conclusion artifact and record `<helper> review-comment <run> <id> <artifact> <reason>`. Carry confirmed findings and unresolved claims into synthesis. These are prose checks, not permission to infer runtime changes or Clean. Missing or stale shared reviews block fast paths and finalization, including otherwise unchanged runs. Fetch-only watcher runs do not need a manifest.

For an explicit research continuation, independently start, initialize metrics, fetch and capture a new run, then run `resume <new-run> <prior-run-directory>` as the manifest reference defines. Reuse only its validated completed targets; assign the pending targets below. Read `reused` in the new manifest and preserve the findings and coverage in their copied artifacts. Remove those targets from deep batches and sweeps before dispatch or recording. Reused research is completed coverage, not an automatic Clean verdict. A same-day report alone does not authorize evidence reuse.

Start saves `builtins.txt`. Run `scripts/target-facts.sh --changed local/target-audit/<date>-run/docfetch.tsv 5 --builtins-since <rev>` with the same baseline, a single revision or the per-target list. Keep its `builtin-deep:` set for the fast-path checks below. Save the separate review-reason TSV and pass each worker its target's rows as described in `.agnostic-ai/skills/target-audit/references/review-reasons.md`. When it prints no numbered batch, preserve reused coverage and record each pending target as a justified fast path, with its assignment and page dispositions. Skip new research, triage, and auditors. Reused findings, signals, or uncertain publication continue through synthesis, relevant issue dedupe, and delivery under the requested mode. Only a pure no-change run can skip issue indexing and signals before its report and finalization.

Fetch the `target-audit` issue index into the run directory with three scoped calls rather than paginating the whole collection: `--state open`, `--state all --search 'created:>=<window start>'`, and `--state closed --search 'closed:>=<window start>'`. Before treating a candidate as new, run one title-scoped search for it (`--state all --search '<target> in:title <keyword>'`). Fetch bodies and comments only for relevant matches. Refresh matching issue state before filing.

Read `scripts/target-audit/signals.tsv` for the signal history, and the published capability entries under `docs/site/content/updates/` for the articles behind it. Preserve every stable signal ID. Give each worker only relevant entries and their source paths. A later briefing omitting a signal does not resolve it.

Read the cross-target kind notes in `docs/site/content/docs/target-behavior.md` once and pass relevant claims to each worker. The fact script prints only the lines that name a target, so a shared paragraph that lists no targets is missing from it; those can stay stale after a target page is fixed.

Dedupe rules:

- Already open: record `still-open #N`, do not file again.
- Already closed: inspect the resolution and current code. Report a regression only when a resolved gap has returned; closure alone does not prove a fix shipped.
- Prior reports and source notes are leads, never substitutes for fresh vendor evidence.

## Worker handoff

Use returned agent IDs for messages. Give each read-only worker only its next scheduled target and:

- Its target, audited commit, window, repository and run paths.
- Its fetched rows, deltas, triage leads, and review-reason rows.
- Shared issue-index path and relevant signal IDs and published source paths.
- `builtins.txt`, baseline, affected repository paths, and `builtin-deep:` membership.
- Read-only ownership and a validation budget.

Do not paste the skill, full issue history, or unrelated vendor material into prompts. Auditors follow `.agnostic-ai/agents/target-auditor.md`: one checkpointed report per target, and a reply of at most 20 lines. Read findings from the reports, never from the reply, which can be truncated.

## Relaunch a stopped worker

When an auditor stops before its assignment is finished (usage limit, crash, lost session), first confirm it has ended; an auditor that lost contact may still be writing. Check that target with `checkpoints` from `scripts/target-audit/manifest.md`. Ingest a valid `done` report before further dispatch. For `pending` or `stale`, release the assignment with `--stopped` and dispatch it again. The manifest records a new attempt and the observed interruption; retain any missing times as unknown. Record the matching metrics wave separately.

For an explicit resume from a stopped run, first record its `done` targets in that prior run the same way, then start the fresh run and resume as above.

## Refill review slots

After triage, flatten the remaining numbered routing lines into pending deep targets and initialize the manifest schedule using the commands in `scripts/target-audit/manifest.md`. Dispatch one target per idle auditor. Use the coordinator's available worker capacity, not a fixed five-way fan-out. Keep eligible sweeps outside the queue and record them normally.

Read each completed target's checkpoint and structured result immediately. Ingest its coverage, begin independent confirmation, and dispatch the next target while other auditors continue. A stopped worker needs explicit `release --stopped` before its unfinished target can be retried. Every attempt keeps its own timing and owner; finished evidence remains immutable.

# Audit coverage manifest

`scripts/docfetch.sh --update <run>/docfetch.tsv` requires a completed version 1 `audit.json` in that run directory. The checker verifies recorded coverage and saved bytes. It cannot verify whether an auditor understood a page.

This development helper needs Go. JSON comparison hashes also need `jq` when the fetched bytes were normalized with it. Fetch-only vendor-watch runs do not use the helper or need a manifest.

## Start and fetch

Start before fetching. Initialize the run metrics before the fetch using the [metrics reference](../../.agnostic-ai/skills/target-audit/references/metrics.md). Use the completed built-in baseline selected by the [built-in audit reference](../../.agnostic-ai/skills/target-audit/references/builtins.md): one revision, or the per-target `<target>=<rev>,...` list from `scripts/target-facts.sh --builtins-baseline local/target-audit/*.md`. `start` resolves each revision to its full commit and rejects an unknown or repeated target. Omit `--builtins-since` when no usable baseline exists; fast paths then remain unavailable. A target the list omits has no baseline and cannot take a fast path.

```bash
run=$(go run ./scripts/audit-manifest start --builtins-since <commit> claude zed)
provenance=$(jq '{audited_commit, targets, builtin_baseline: .builtins_since, source_lock: .lock_baseline_sha256}' "$run/audit.json")
scripts/audit-metrics.sh init "$run" "$(jq -r .run_id "$run/audit.json")" "$provenance"
scripts/docfetch.sh --out "$run" claude zed
go run ./scripts/audit-manifest capture "$run"
```

Omit the target arguments from both commands to audit every registered target. `start` prints a unique run directory under ignored `local/target-audit/`. `--out <directory>` selects another fresh directory, either outside the checkout or ignored by Git. An existing directory is rejected. A new invocation always starts a new run and fetches every requested source fresh.

The manifest fixes the requested targets and target-kind-URL source set before any fetch returns. It records all registered source owners, the source-list digest, audited commit and repository file digest (including working changes and built-in assets), the resolved built-in baseline, saved built-in inventory and exact source-lock baseline. `requested-targets.txt` also lets `target-facts --changed` preserve requested targets when a fetch row is missing.

`capture` requires exactly one fetched row for every expected source and rejects unknown rows. It saves the fetch-row digest, each source's normalized `comparison_sha256`, the exact saved evidence path and SHA-256, and related response and release-note artifacts. A failed HTTP fetch with no body path gets a saved failure-row artifact. JSON key sorting can give two byte-different files the same comparison hash; their evidence digests still differ. Auditors read the captured evidence paths and preserve those files.

## Resume completed research

Explicit resume selects one prior run directory. Start a new run with the independently requested target list and baseline, fetch every requested source fresh, and capture it before selecting reuse:

```bash
run=$(go run ./scripts/audit-manifest start --builtins-since <commit> claude zed)
provenance=$(jq '{audited_commit, targets, builtin_baseline: .builtins_since, source_lock: .lock_baseline_sha256}' "$run/audit.json")
scripts/audit-metrics.sh init "$run" "$(jq -r .run_id "$run/audit.json")" "$provenance"
scripts/docfetch.sh --out "$run" claude zed
go run ./scripts/audit-manifest capture "$run"
go run ./scripts/audit-manifest resume "$run" <prior-run-directory>
```

Resume runs once before assignments or page dispositions are added. It validates both runs in the same checkout, including the repository commit, staged index, working and untracked files, embedded ignored built-ins, policy/extraction version, source inventory, exact lock baseline and built-in baseline. Any unknown context change rejects reuse for the whole run. It does not adopt a prior run's requested targets or baseline. Old manifests without the research version cannot resume.

A target is reused only when its assignment and every requested page are complete, eligible, and backed by valid artifacts. Fresh saved evidence, response and release-note artifact bytes must match the prior run exactly. Matching normalized comparison hashes alone is insufficient. A changed successful source leaves its target pending for fresh research; corrupt or missing saved artifacts reject resume. Pending assignments, research limits and failed fetches remain pending. Resume prints the number of reused and pending targets. Assign and review the pending targets normally; missing coverage still rejects finalization.

Copied research artifacts live under `resumed/<prior-run-id>/` in the new run. Each reused target records the prior run ID and the digest of a saved prior manifest. The current run retains its fresh fetch paths and hashes. Finalization validates the copies, so deleting the prior directory later does not invalidate completed copies. Copies without an atomically saved manifest entry are unfinished work and authorize no reuse. A retry after fixing a copy failure validates all inputs again.

This is local research continuation for one checkout. Resume does not execute issue creation, comments, fixes, or any other external action. Reconcile interrupted publication using the [issue filing reference](../../.agnostic-ai/skills/target-audit/references/issues-and-fixes.md).

## Record coverage

One coordinator writes progress. Assign every requested target exactly once, including targets taking a fast path. Owners can be an auditor name, a batch name, or the coordinator.

```bash
go run ./scripts/audit-manifest assign "$run" claude batch-1
go run ./scripts/audit-manifest complete "$run" claude reviewed claude.md 'Checked emitted paths and built-ins'
go run ./scripts/audit-manifest record "$run" claude docs <source-url> reviewed claude.md 'Verified current page against repository claims'
```

Assignment completion and page dispositions are separate. Complete one assignment per target and record one disposition per expected target-kind-URL. Each command takes `reviewed`, `fast-path`, or `research-limit`, followed by an artifact path relative to the run directory and a nonempty reason. Finish each artifact before recording it; later edits invalidate its digest. A target artifact can explain several pages. Unknown targets, unknown sources, repeated assignments and repeated completions are rejected.

A `fast-path` needs a usable built-in baseline, a target outside the computed `builtin-deep` set, an artifact recording the proof, and its justification. Preserve the audit skill's other fast-path conditions. A failed fetch requires `research-limit`; it never advances its hash. A fetch recovered by the fetcher's ladder is captured with its final successful evidence and can be reviewed normally. Manual recovery after capture can support findings, but advancing that source requires a fresh run and fetch. A successful page that cannot be reviewed also takes `research-limit` with the reason and attempted routes in its artifact.

## Validate and finalize

```bash
go run ./scripts/audit-manifest validate "$run"
scripts/docfetch.sh --update "$run/docfetch.tsv"
```

Finalization rechecks assignments, coverage, versions, source inventory, built-in classification, repository context, exact evidence and artifact bytes, fetched rows, and the lock baseline. Structural gaps reject the whole update and leave the lock unchanged. Fully accounted research limits retain their old hashes while eligible reviewed or justified fast-path pages advance. Repeating `finalize` on a finalized manifest revalidates its inputs, evidence and exact committed lock hash, then succeeds without rewriting the lock. Other mutations are rejected.

Source identity is target, kind and URL. The lock still has one row per URL. Every registered owner of a shared URL must be present in this run and eligible before that URL advances. An owner outside the requested scope, a failed page, or a research limit retains the prior URL row for all owners. Eligible owners must agree on the comparison hash; disagreement rejects finalization. The first registry owner supplies the one lock row. Finalization preserves sorting and prunes URLs absent from the current registry. Snapshot storage and age-based pruning retain their existing behavior.

Keep tracked source corrections, signal-history edits and implementation changes until after finalization. If the audited repository, source list or lock changes first, start a fresh run and fetch again. Reports and evidence stay in the ignored run directory. A source correction that must precede completion needs a fresh run against that correction.

Manifest and lock writes use temporary files and atomic renames. Concurrent state writes or lock finalizations fail closed. A killed writer can leave `<run>/.audit-writing` or `<lock>.audit-finalizing`; the latter records its PID and run. Confirm that writer has exited before removing that exact marker, then use a new fresh-fetch run and explicit resume, or retry finalization when reconciling its saved receipt. Half-written temporary files never become valid progress.

Finalization saves `finalization.json` with the validated manifest digest and intended exact lock hash before committing the lock. If saving the final manifest marker fails afterwards, the error says `lock committed; manifest finalization marker failed` and includes that hash. Preserve the run and error, fix the filesystem problem, and rerun `finalize`. It verifies the receipt, unchanged research inputs and exact current lock bytes before saving the missing marker. A different current lock rejects reconciliation. If the lock still equals the original baseline, validation runs before any new commit. This receipt reconciles only this local lock write.

## Manual compatibility path

`scripts/docfetch.sh --update-unvalidated <docfetch.tsv> [target...]` explicitly keeps the old manual update behavior, including failed-fetch preservation, URL pruning and snapshots. It prints that coverage was not checked. It can advance successfully fetched pages without review and has no shared-owner protection. Use it only when deliberately making an unvalidated manual lock update. Fetch-only watcher workflows never update the source lock.

## Watcher comparisons

Fetch-only runs retain raw `.body` responses and the comparison text. `deltas.tsv` records versioned source classifications, and each `.delta` records the comparison version `source-aware-v1`. The watcher suppresses an actionable alert only when valid old and current snapshots differ solely in Junie's top-level Marketplace `downloads` fields, or the watcher count in both repository metadata blocks of the pinned Grok GitHub tree layout. Other fields remain part of the comparison. Missing snapshots, malformed data, and unknown layouts keep the existing changed or unknown report.

GitHub release indexes and changelogs whose level-two Markdown headings contain only an ISO date can be labeled `historical-additions:v1` when every old entry is unchanged and each added date precedes the oldest existing entry. Newer additions and edits to any existing entry have separate labels. Unknown dates, removed entries, changed surrounding text, or Markdown layouts with nested headings or fenced sections keep the existing report. Historical additions remain unreviewed and actionable. This classification neither writes the source lock nor changes a manifest disposition.

GitHub release indexes now start with `# docfetch-release-index-v2` and retain note bodies as well as tags and publication dates, so edits to old release notes change the hash. Existing version-one lock rows deliberately report changed on the first fetch. Review that delta before advancing the lock through the normal validated audit. There is no automatic migration or baseline promotion. Without a valid API array or `jq`, the comparison retains the entire response rather than omitting possibly changed notes.

Download the workflow's `vendor-watch-pages` artifact to inspect each reported `.delta`, its sibling `.body`, and its normalized `.txt` comparison. The issue report names raw delta paths, explains suppressed counters when other changes are reported, and marks historical additions as unreviewed. A counter-only run creates no actionable issue; its classification and raw evidence remain in the artifact. URL/hash deduplication and failed-fetch reporting are unchanged.

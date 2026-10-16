# Audit coverage manifest

`scripts/docfetch.sh --update <run>/docfetch.tsv` requires a completed version 1 `audit.json` in that run directory. The checker verifies recorded coverage and saved bytes. It cannot verify whether an auditor understood a page.

This development helper needs Go. JSON comparison hashes also need `jq` when the fetched bytes were normalized with it. Fetch-only vendor-watch runs do not use the helper or need a manifest.

## Start and fetch

Start before fetching. Use the completed built-in baseline selected by the [built-in audit reference](../../.agnostic-ai/skills/target-audit/references/builtins.md). Omit `--builtins-since` when no usable baseline exists; fast paths then remain unavailable.

```bash
run=$(go run ./scripts/audit-manifest start --builtins-since <commit> claude zed)
scripts/docfetch.sh --out "$run" claude zed
go run ./scripts/audit-manifest capture "$run"
```

Omit the target arguments from both commands to audit every registered target. `start` prints a unique run directory under ignored `local/target-audit/`. `--out <directory>` selects another fresh directory, either outside the checkout or ignored by Git. An existing directory is rejected. A new invocation always starts a new run and fetches every requested source fresh.

The manifest fixes the requested targets and target-kind-URL source set before any fetch returns. It records all registered source owners, the source-list digest, audited commit and repository file digest (including working changes and built-in assets), the resolved built-in baseline, saved built-in inventory and exact source-lock baseline. `requested-targets.txt` also lets `target-facts --changed` preserve requested targets when a fetch row is missing.

`capture` requires exactly one fetched row for every expected source and rejects unknown rows. It saves the fetch-row digest, each source's normalized `comparison_sha256`, the exact saved evidence path and SHA-256, and related response and release-note artifacts. A failed HTTP fetch with no body path gets a saved failure-row artifact. JSON key sorting can give two byte-different files the same comparison hash; their evidence digests still differ. Auditors read the captured evidence paths and preserve those files.

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

Finalization rechecks assignments, coverage, versions, source inventory, built-in classification, repository context, exact evidence and artifact bytes, fetched rows, and the lock baseline. Structural gaps reject the whole update and leave the lock unchanged. Fully accounted research limits retain their old hashes while eligible reviewed or justified fast-path pages advance. Finalized manifests cannot run again.

Source identity is target, kind and URL. The lock still has one row per URL. Every registered owner of a shared URL must be present in this run and eligible before that URL advances. An owner outside the requested scope, a failed page, or a research limit retains the prior URL row for all owners. Eligible owners must agree on the comparison hash; disagreement rejects finalization. The first registry owner supplies the one lock row. Finalization preserves sorting and prunes URLs absent from the current registry. Snapshot storage and age-based pruning retain their existing behavior.

Keep tracked source corrections, signal-history edits and implementation changes until after finalization. If the audited repository, source list or lock changes first, start a fresh run and fetch again. Reports and evidence stay in the ignored run directory. A source correction that must precede completion needs a fresh run against that correction.

Manifest and lock writes use temporary files and atomic renames. Concurrent state writes or lock finalizations fail closed. A killed writer can leave `<run>/.audit-writing` or `<lock>.audit-finalizing`; the latter records its PID and run. Confirm that writer has exited before removing that exact marker, then start a fresh run. Half-written temporary files never become valid progress. There is no resume command.

The lock commits before the final manifest marker is saved. If saving that marker fails, the error says `lock committed; manifest finalization marker failed` and includes the committed lock's SHA-256. Compare the current lock's exact file hash with that checksum. A match confirms the lock update succeeded. Preserve the run, evidence, error and checksum in the report, fix the filesystem problem, then start a fresh run for further work. If the checksum differs, reconcile the later lock change first. Do not retry the old run or use an unvalidated update as a repair.

## Manual compatibility path

`scripts/docfetch.sh --update-unvalidated <docfetch.tsv> [target...]` explicitly keeps the old manual update behavior, including failed-fetch preservation, URL pruning and snapshots. It prints that coverage was not checked. It can advance successfully fetched pages without review and has no shared-owner protection. Use it only when deliberately making an unvalidated manual lock update. Fetch-only watcher workflows never update the source lock.

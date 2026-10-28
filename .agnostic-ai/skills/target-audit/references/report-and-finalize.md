# Report and finalize

Write `local/target-audit/<YYYY-MM-DD>.md`. Preserve an existing same-day report or deliberately update it as a continuation. Include audited commit, window, target coverage, counts, source dates, issue/PR links, and research limits. Add a `Built-ins` line naming each target whose built-in behavior this run checked, but only when `scripts/target-facts.sh --changed <run>/docfetch.tsv --builtins-since HEAD` prints no `builtin-deep:` line, so the files audited match the recorded commit. The next run takes its baseline only from a report with that line.

Include the metrics summary and artifact links per `.agnostic-ai/skills/target-audit/references/metrics.md`. End the audit interval before later implementation or merge work; state unavailable measurements as unknown.

The report is the decision view. For each finding write the heading, kind, severity, one paragraph of impact and smallest fix, the issue or PR link, and a relative link to its run-directory file, for example `local/target-audit/<date>-run/claude-mcp.md`. Vendor quotes, repository lines, and reproduction commands live once, in that file. Do not paste a run-directory file into the report.

Use these sections, even when empty:

1. Critical vendor changes
2. Cross-target opportunities
3. Breaking: we write where the tool no longer reads
4. Missing: native surface we skip today
5. Degraded: works, but a better surface exists
6. Cosmetic: docs only
7. Needs a human: unconfirmed, with the question that would settle it
8. Clean
9. Source fixes needed

Select critical changes using the capability reference's priority rules. Each synthesized signal retains its stable ID, independently verified semantics, representation result, disposition, confidence, and next action. Attach challenger metadata to challenged items.

List moved or broken source URLs with verified replacements. Apply corrections to the canonical `.agnostic-ai/skills/target-audit/references/sources.md` after finalizing coverage, even in report-only mode. Edit source specs, never generated native copies. A correction made before finalization requires a fresh manifest and fetch because the source inventory changed.

After the report and complete coverage are recorded, run `scripts/docfetch.sh --update local/target-audit/<date>-run/docfetch.tsv`. It validates the manifest before changing the lock. Missing coverage rejects the update; accounted research limits retain their old hashes while reviewed pages can advance. Shared URLs require eligible coverage for every registered owner, as the manifest reference defines. Keep tracked corrections, signal edits, and replay-case edits until after finalization. Text snapshots remain stored by comparison hash and old snapshots are pruned after 30 days. The same step removes `pages/` from every other run that nothing cites and that had no write for 30 days, and prints each removal. A run counts as cited when its directory name appears in `scripts/target-audit/*.tsv` or in a Markdown file of another run or report under `local/target-audit/`. A run with no report and no writes for 30 days loses its pages at the next finalization. Reports, `rows/`, and `docfetch.tsv` stay. `--update-unvalidated` is an explicit manual compatibility path and does not establish validated audit completion. With `--no-file-issues`, leave validated audit-maintenance edits in the working tree. Otherwise deliver all changed audit-owned files through the maintenance bucket in `.agnostic-ai/skills/target-audit/references/issues-and-fixes.md`, even when the audit is clean or every finding was already tracked. A changed lock must be committed; an unchanged lock does not excuse other maintenance edits.

## Publish findings

Unless `--no-file-issues` is set, read `.agnostic-ai/skills/target-audit/references/issues-and-fixes.md` and file confirmed findings. The gitignored report and run directory must not be their only detailed record. After finalization, add or update one row per signal in `scripts/target-audit/signals.tsv`. A new row takes `vendor-date` from the vendor entry's own date and `first-source` from the kind of row whose delta showed it (`docs`, `changelog`, `schema`, or `code`). `cut-release` sets `shipped-date` through `scripts/signals-shipped.sh`; a row a release missed gets it here from `scripts/signals-shipped.sh --all`. Leave a value empty rather than guess it. The gaps between the three dates show which sources earn their fetch; the report states them for signals shipped this run. Spec candidates may receive design issues but never enter fix buckets without a separate implementation decision.

With `--fix`, finish synthesis and settle all buckets before launching isolated `adapter-fixer` agents. Use the reference's scope, checklist, and validation rules. Never widen an active fixer's assignment; handle later findings separately.

Finish with the top findings, recommended next actions, report link, and issue/PR links. State validation limits accurately. Stop at reviewable PRs; merging belongs to the human.


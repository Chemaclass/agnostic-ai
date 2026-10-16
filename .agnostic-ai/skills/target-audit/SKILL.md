---
name: target-audit
description: Audit every agnostic-ai target against current vendor docs, report evidence-backed drift, and report project capability changes. Use for the recurring adapter truth check or when a tool ships a new config feature.
argument-hint: "[target...] [--no-file-issues] [--fix] [--compare-models <model>]"
disable-model-invocation: false
---

# target-audit

Check whether tools still read what agnostic-ai emits and which new project capabilities need a product decision. Complete the requested scope using current vendor evidence.

## Arguments and boundaries

- No arguments: audit every registered target and file an issue per confirmed finding.
- Target names: audit only those targets.
- `--no-file-issues`: report only. No GitHub writes.
- `--fix`: file issues and open fix PRs. Never merge or enable auto-merge.
- `--compare-models <model>`: bounded challenge of breaking findings, spec candidates, and conflicting evidence. Continue if unavailable.

The report and issues are the only published output. Never create or edit site articles or publication PRs. Never change legacy articles; they are the dedupe history. Generic schema changes require a separate implementation decision.

All paths below are relative to the repository root, including when this skill is emitted into a nested directory.

The coordinator owns shared inputs, routing, manifest coverage, synthesis, publication, finalization, and metrics. Auditors own per-target investigation, comparisons, reproductions, and evidence reports. Read `.agnostic-ai/skills/target-audit/references/evidence-and-output.md` before delegating or researching inline; it defines the shared evidence and output contract.

## 1. Prepare shared inputs once

Run `scripts/target-facts.sh --list`. Record the audited commit, worktree state, and open PRs before delegating.

Select the built-in baseline using `.agnostic-ai/skills/target-audit/references/builtins.md`, then start a unique audit run using `scripts/target-audit/manifest.md`. Freeze the requested target-kind-URL set before fetching and use that run path below wherever `<date>-run` appears.

Initialize the run metrics before fetching, using `.agnostic-ai/skills/target-audit/references/metrics.md`. Record observed stages, routing decisions, usage, and outcomes there, including a no-change early return.

Fetch once and capture the fetched evidence using `scripts/target-audit/manifest.md`. The fetch saves fresh bodies, comparison hashes and statuses against `scripts/target-audit/sources.lock`. Read its summary and failed rows before delegating. Fetch-only watcher runs do not need a manifest.

For an explicit research continuation, independently start, initialize metrics, fetch and capture a new run, then run `resume <new-run> <prior-run-directory>` as the manifest reference defines. Reuse only its validated completed targets; assign the pending targets below. Read `reused` in the new manifest and preserve the findings and coverage in their copied artifacts. Remove those targets from deep batches and sweeps before dispatch or recording. Reused research is completed coverage, not an automatic Clean verdict. A same-day report alone does not authorize evidence reuse.

Start saves `builtins.txt`. Run `scripts/target-facts.sh --changed local/target-audit/<date>-run/docfetch.tsv 5 --builtins-since <rev>`. Keep its `builtin-deep:` set for the fast-path checks below. Save the separate review-reason TSV and pass each batch its rows as described in `.agnostic-ai/skills/target-audit/references/review-reasons.md`. When it prints no numbered batch, preserve reused coverage and write each pending target as a fast-path Clean row, record only the pending assignments and page dispositions, and finalize the manifest. Skip the issue index, signals, auditors and filing.

Fetch the `target-audit` issue index into the run directory with three scoped calls rather than paginating the whole collection: `--state open`, `--state all --search 'created:>=<window start>'`, and `--state closed --search 'closed:>=<window start>'`. Before treating a candidate as new, run one title-scoped search for it (`--state all --search '<target> in:title <keyword>'`). Fetch bodies and comments only for relevant matches. Refresh matching issue state before filing.

Read `scripts/target-audit/signals.tsv` for the signal history, and the published capability entries under `docs/site/content/updates/` for the articles behind it. Preserve every stable signal ID. Give each batch only relevant entries and their source paths. A later briefing omitting a signal does not resolve it.

Read the cross-target kind notes in `docs/site/content/docs/target-behavior.md` once and pass relevant claims to each batch. The fact script prints only the lines that name a target, so a shared paragraph that lists no targets is missing from it; those can stay stale after a target page is fixed.

Take the newest date from the latest completed local report, legacy `extra.audit_marker`, audit issue creation, or commit that records an audit's source corrections in `.agnostic-ai/skills/target-audit/references/sources.md`. A commit that only trims or restructures that file, such as #1109's, is not an audit and does not move the window. Widen it slightly for changelog overlap and record the chosen window. Scratch reproductions and incomplete runs are not completed reports.

Dedupe rules:

- Already open: record `still-open #N`, do not file again.
- Already closed: inspect the resolution and current code. Report a regression only when a resolved gap has returned; closure alone does not prove a fix shipped.
- Prior reports and source notes are leads, never substitutes for fresh vendor evidence.

## 2. Audit in bounded batches

In the `--changed` output from step 1, numbered lines are deep batches, at most five and sized to available worker slots. The `sweep` line lists targets eligible for unchanged-vendor coverage under the built-in baseline check; record them as fast path without an agent. For fewer than six deep targets, audit them inline using `.agnostic-ai/agents/target-auditor.md`. Queue batches when needed; every requested target must be assigned exactly once, including assignments copied by explicit resume.

The coordinator preserves copied assignments and records each pending assignment and each source's separate reviewed, justified fast-path or research-limit disposition using `scripts/target-audit/manifest.md`. Finish evidence artifacts before recording their hashes. A research limit accounts for coverage but keeps the previous source hash.

Run `scripts/jev-triage.sh local/target-audit/<date>-run` before dispatch. It always exits 0. Its `triage.tsv` columns are target, url, delta, claim, verdict, probability, p_contradicts, via. With `TYPESAFE_API_KEY`, Jev checks every target claim against each changed page and scores project-config additions (`noul`). Without a key or after API failure, lexical `paired` leads match paths, keys, or terms; unjudged page parts also get trim-1 top-five changed-line matches against the target's other claims. Trim needs Go; stderr records skipped reasons, method, and input tokens. Leads are questions, never findings. Empty leads never clear a page.

Prioritize targets with Jev leads or `noul >= 0.5`. Pass only each batch's leads. Jev keeps `contradicts` or `p_contradicts >= 0.4`. `triage-pages.tsv` statuses control eligibility:

- `clear`: all questions answered, no retained lead, `noul < 0.1`, and no emitted path moved.
- `lead`: retained lead.
- `judged`: no lead, but a `mentions:` page or `noul >= 0.1`.
- `unjudged`: capped or failed request.
- `lexical`: Jev did not run.

Deep review is the default. A Jev fast path requires every `new`/`changed` row to be `clear` and the target absent from `builtin-deep:`. First read its changed deltas against the built-ins reference's hook and skill checklist (event, launcher, output, project/global paths, root discovery), which Jev never sees. Any hit keeps deep review. Only then remove it from its batch and mark `fast path by Jev`. Any other status or missing row, including no delta or failed fetch, requires deep review. Without Jev, every changed row is read.

Randomly select one Jev-fast-path target for deep review and record whether it was clean. A finding disables that fast path until a replay case passes. After filing each confirmed finding or signal, add its exact vendor quote to the matching `triage-cases.tsv` or `surface-cases.tsv`. Recall checks are `scripts/jev-triage.sh --replay scripts/target-audit/triage-cases.tsv` (Jev, lexical, trim-1) and `--replay-surface scripts/target-audit/surface-cases.tsv` (`noul`). Setup and cutoff measurements: `docs/internal/jev-triage.md`.

Use `target-auditor` agents named Frodo, Sam, Gandalf, Aragorn, Legolas in batch order. Use returned agent IDs for all messages. Start each agent with minimal context when supported. Pass:

- target list, audited commit, date window, repository path;
- the run directory and the target's own `docfetch.tsv` rows;
- the target's `triage.tsv` leads, when the file exists;
- shared issue-index path and relevant published signals;
- shared `builtins.txt` path, built-in baseline, affected paths and `builtin-deep:` membership;
- read-only scope and validation budget.

Do not paste this skill, the full issue history, or unrelated source sections into each prompt. Auditors already have their role instructions. No broad tests during research. Build one current binary for compatible read-only reproductions when needed.

Auditors follow `.agnostic-ai/agents/target-auditor.md`, including per-target source extraction, delta reading, recovery, comparisons, and reproduction rules. Keep pages and reproductions in the run directory. Require evidence artifacts and one coverage row per target under the shared contract before recording completed coverage.

## 3. Synthesize and verify

Accept findings only under the shared evidence contract. Drop unsupported claims. Reopen every breaking finding's source and repository line yourself. Recheck negative claims through an independent fetch route before accepting absence or removal. A 200 response may be an app shell, soft 404, or unrelated redirect.

For permission boundaries, overlapping order or precedence, path discovery, and import fidelity, apply `.agnostic-ai/skills/target-audit/references/behavior-evidence.md` before accepting or filing a claim. Verify its recorded confirmation outcome and complete the independent check, even without `--compare-models`. Keep unresolved claims in `Needs a human` with the missing evidence; never send them to a fixer. Source-only URL corrections and capability opportunities keep their existing evidence paths.

Keep built-in drift (a target the built-in emits to stopped behaving as it needs) apart from a coverage opportunity (a target it does not list gained compatible support). Drift is a finding; an opportunity goes under Cross-target opportunities. Apply the fix rule in the built-ins reference: hook capability alone never enables a target.

Collapse one vendor change affecting several targets into one finding, with independent evidence for each target. Sort `breaking`, `missing-feature`, `degraded`, `cosmetic`. Unconfirmed findings belong only in the report, with the question that would settle them.

Before merging capability signals, read `.agnostic-ai/skills/target-audit/references/capability-intelligence.md`. Apply its representation test and per-target verification before choosing `adapter-gap`, `spec-candidate`, `target-extension`, or `watch`. Try existing kinds and `x-<target>` passthroughs before proposing a schema.

When requested, run that reference's bounded challenger pass after synthesis. Record requested and actual model identities (or unavailable), evidence added, and unresolved disputes. Agreement is not vendor evidence.

## 4. Write the report

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

After the report and complete coverage are recorded, run `scripts/docfetch.sh --update local/target-audit/<date>-run/docfetch.tsv`. It validates the manifest before changing the lock. Missing coverage rejects the update; accounted research limits retain their old hashes while reviewed pages can advance. Shared URLs require eligible coverage for every registered owner, as the manifest reference defines. Keep tracked corrections and signal edits until after finalization. Text snapshots remain stored by comparison hash and old snapshots are pruned after 30 days. `--update-unvalidated` is an explicit manual compatibility path and does not establish validated audit completion. With `--no-file-issues`, leave the lock change in the working tree beside the source corrections. In every other mode, commit it even when the audit is clean or every finding was already tracked, unless `git diff --quiet scripts/target-audit/sources.lock` shows it did not move: in the cosmetic/source docs PR when one exists, otherwise in a `chore(target-audit)` PR of its own.

## 5. File and fix

Unless `--no-file-issues` is set, read `.agnostic-ai/skills/target-audit/references/issues-and-fixes.md` and file confirmed findings. The gitignored report and run directory must not be their only detailed record. After filing, add or update one row per signal in `scripts/target-audit/signals.tsv`. A new row takes `vendor-date` from the vendor entry's own date and `first-source` from the kind of row whose delta showed it (`docs`, `changelog`, `schema`, or `code`). `cut-release` sets `shipped-date` through `scripts/signals-shipped.sh`; a row a release missed gets it here from `scripts/signals-shipped.sh --all`. Leave a value empty rather than guess it. The gaps between the three dates show which sources earn their fetch; the report states them for signals shipped this run. Spec candidates may receive design issues but never enter fix buckets without a separate implementation decision.

With `--fix`, finish synthesis and settle all buckets before launching isolated `adapter-fixer` agents. Use the reference's scope, checklist, and validation rules. Never widen an active fixer's assignment; handle later findings separately.

Finish with the top findings, recommended next actions, report link, and issue/PR links. State validation limits accurately. Stop at reviewable PRs; merging belongs to the human.

## Scheduling and invariants

The `vendor-watch` workflow (`.github/workflows/vendor-watch.yml`) runs `scripts/docfetch.sh` daily with no AI and posts moved pages to one open issue labeled `vendor-watch`. `scripts/vendor-watch.sh` keys each page by URL and hash, so a page is reported once per text change. A changed page whose URL already served that exact text on an earlier run stays quiet: a stale CDN copy or an alternating render is not news. A page that returns to earlier text within the 30-day snapshot window is therefore reported only by the next full audit. Start a run from that issue: audit the targets it lists, and close it once the lock moves.

The `tool-load` workflow (`.github/workflows/tool-load.yml`) runs `scripts/tool-load.sh` weekly: it syncs a probe project for Codex, Gemini CLI, and OpenCode and asks each tool, at its latest release, what it loaded. A failed check opens one issue labeled `tool-load`. Treat it as a breaking lead for that target: reproduce with `scripts/tool-load.sh <tool>` and find the vendor change before fixing. `tests/integration/import_fidelity_test.go` is its offline counterpart for import: real repositories' native config, imported and synced, against a list of known gaps in `tests/fidelity/<name>/expected.txt`.

Daily runs are cheap: a day when nothing moved ends after step 1. A scheduler wraps this skill; unattended runs should use `--fix` or default issue filing so results survive outside ignored local files.

Every registered target needs a `## <target>` section with `docs:` and `watch:` in the source registry. `tests/integration/target_audit_sources_test.go` enforces this. Add missing vendor sources, never weaken the test.

`scripts/docfetch.sh --urls` must resolve at least one docs URL and one changelog URL for every registered target. `scripts/docfetch_test.sh` enforces this, so a source line written outside the documented URL grammar fails the shell suite.

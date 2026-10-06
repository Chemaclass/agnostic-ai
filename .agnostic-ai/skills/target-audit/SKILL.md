---
name: target-audit
description: Audit every agnostic-ai target against current vendor docs, report evidence-backed drift, and surface project capability changes. Use for the recurring adapter truth check or when a tool ships a new surface.
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

The report and issues are the publication boundary. Never create or edit site articles or publication PRs. Legacy articles are immutable dedupe history. Generic schema changes require a separate implementation decision.

All paths below are relative to the repository root, including when this skill is emitted into a nested directory.

## 1. Prepare shared inputs once

Run `scripts/target-facts.sh --list`. Record the audited commit, worktree state, and open PRs before delegating.

Run `scripts/docfetch.sh` once. It fetches every source URL fresh, runs the recovery ladder on pages that do not serve usable text, saves the bodies under `local/target-audit/<date>-run/pages/`, and writes `docfetch.tsv` with one row per URL carrying its mode, content hash, and status against `scripts/target-audit/sources.lock`. Read its summary line and the `failed` rows before delegating.

Then run `scripts/target-facts.sh --changed local/target-audit/<date>-run/docfetch.tsv 5 --builtins-since <rev>`, where `<rev>` is the audited commit of the last completed audit (see the window below). A target whose shipped built-ins changed since then lands in a deep batch even when no vendor page moved, so a vendor-only fast path never covers a changed built-in. When it prints no numbered batch, nothing moved, nothing failed, and no built-in changed: write the report with every target as a fast-path Clean row and stop. Skip the issue index, signals, auditors, and filing. The lock does not move on such a day, so there is nothing to commit.

Fetch the `target-audit` issue index into the run directory with three scoped calls rather than paginating the whole collection: `--state open`, `--state all --search 'created:>=<window start>'`, and `--state closed --search 'closed:>=<window start>'`. Before treating a candidate as new, run one title-scoped search for it (`--state all --search '<target> in:title <keyword>'`). Fetch bodies and comments only for relevant matches. Refresh matching issue state before filing.

Read `scripts/target-audit/signals.tsv` for the signal history, and the published capability entries under `docs/site/content/updates/` for the articles behind it. Preserve every stable signal ID. Give each batch only relevant entries and their source paths. A later briefing omitting a signal does not resolve it.

Shipped built-ins are an audit input of their own. Read `.agnostic-ai/skills/target-audit/references/builtins.md` once: it names the specs, docs, and behavioral tests that are the repository evidence, and the native behavior each built-in depends on. Each target's fact sheet lists the built-ins it emits.

Read the cross-target kind notes in `docs/site/content/docs/target-behavior.md` once and pass relevant claims to each batch. The fact script prints only the lines that name a target, so a shared paragraph that lists no targets is missing from it; those can stay stale after a target page is fixed.

Take the newest date from the latest completed local report, legacy `extra.audit_marker`, audit issue creation, or commit that records an audit's source corrections in `.agnostic-ai/skills/target-audit/references/sources.md`. A commit that only trims or restructures that file, such as #1109's, is not an audit and does not move the window. Widen it slightly for changelog overlap and record the chosen window. Scratch reproductions and incomplete runs are not completed reports.

Dedupe rules:

- Already open: record `still-open #N`, do not file again.
- Already closed: inspect the resolution and current code. Report a regression only when a resolved gap has returned; closure alone does not prove a fix shipped.
- Prior reports and source notes are leads, never substitutes for fresh vendor evidence.

## 2. Audit in bounded batches

In the `--changed` output from step 1, numbered lines are deep batches of targets whose pages or changelog moved, at most five and sized to available worker slots. The `sweep` line lists targets whose rows are all unchanged; record them as fast path without an agent, since nothing they serve moved since an auditor last read it. For fewer than six deep targets, audit them inline using `.agnostic-ai/agents/target-auditor.md`. Queue batches when needed; every requested target must be assigned exactly once.

Then run `scripts/jev-triage.sh local/target-audit/<date>-run`. It always exits 0 and writes `triage.tsv`, likeliest drift first: target, url, delta, claim, verdict, probability, p_contradicts, via. With `TYPESAFE_API_KEY`, Jev judges every claim of each changed page's target (`contradicts`, `supports`) and rates each changed page (`noul`: it changes a project-scoped config surface, the lead for a capability signal no claim covers). Without a key, or once the API fails, rows are lexical leads (verdict `paired`, via `lexical`): claims that share a path, key, or term with the moved words. Its stderr line says which ran and how many input tokens it spent. A Jev row is kept only when its verdict is `contradicts` or its `p_contradicts` reaches 0.4, the cut that separated confirmed drift from everything else on the replay cases, so an empty file after a Jev run means no lead, not a clean page. Move targets with a Jev row, or a `noul` of 0.5 or more, into the earliest batches, and pass each batch its targets' rows as leads. A lead is a question, never a fact. `triage-pages.tsv` gives each changed page a status: `clear` (Jev answered every question for it, kept no row, its surface score stayed under 0.1, and no path we write moved on it), `lead`, `judged` (no lead, but a `mentions:` page or a surface score of 0.1 or more), `unjudged` (a request was capped or failed), or `lexical` (Jev did not run). A target whose every `new` or `changed` row is `clear` there, and whose built-ins did not change, is a Jev fast path: take it out of its batch and give it a Clean row marked `fast path by Jev`. Any other status, and any row missing from the file (no delta, a failed fetch), keeps the deep read, so without Jev every changed row is read as before. When a run has a Jev fast path, audit one of those targets anyway, chosen at random, and record in the report whether it was clean. A finding there turns the fast path off until a replay case for it passes. `scripts/jev-triage.sh --replay scripts/target-audit/triage-cases.tsv` measures Jev's recall and the lexical pairing's on past confirmed drift, and `--replay-surface scripts/target-audit/surface-cases.tsv` measures the `noul`. After filing, add a row to the matching cases file for each confirmed finding or signal, with a short exact vendor quote. Setup, cost, and the measurements behind these cuts: `docs/internal/jev-triage.md`.

Use `target-auditor` agents named Frodo, Sam, Gandalf, Aragorn, Legolas in batch order. Use returned agent IDs for all messages. Prefer minimal-context spawns when supported. Pass:

- target list, audited commit, date window, repository path;
- the run directory and the target's own `docfetch.tsv` rows;
- the target's `triage.tsv` leads, when the file exists;
- shared issue-index path and relevant published signals;
- whether the target's built-ins changed since the last audit, and the path to the built-ins reference;
- read-only scope and validation budget.

Do not paste this skill, the full issue history, or unrelated source sections into each prompt. Auditors already have their role instructions. No broad tests during research. Build one current binary for compatible read-only reproductions when needed.

Read only each assigned target's source section. `scripts/target-facts.sh --sources <target>...` extracts those sections without loading the other targets. Run `scripts/target-facts.sh <target>...` once per batch for our claims; read exact implementation lines when checking a candidate.

Every source URL was fetched fresh this run, so the lock decides what a model must read, never whether the evidence is current. Auditors read `deltas.tsv` first: each `new` or `changed` row carries a label (`mentions:<paths>`, `prose`, `chrome-only`, `whitespace-only`, `no-snapshot`) and, when a snapshot exists, a word `.delta` beside its page. Labels rank the reading order and never clear a row. Then each `schema` row's delta, which lists added and removed key paths only, and each `code` row's delta, a vendor source file that moves before the docs, then the changelog delta, then each changed page's delta and saved text, retaining each page's URL, fetch date, and exact relevant excerpt. Read the full relevant section when an excerpt leaves scope or precedence unclear. Never treat a prior run's saved page, report, or lock row as current evidence. A target may reach a Clean row on the fast path only when every one of its rows is `unchanged`, or `clear` in `triage-pages.tsv` by the Jev rule in step 2; mark that row as fast path so a human can see it. Recover every `failed`, `app-shell`, `soft-404`, and `redirected` row by hand through `.agnostic-ai/skills/target-audit/references/fetch-playbook.md`, and report what stays unreadable as a research limit.

Keep raw pages and reproductions in the local run directory when useful. Return concise evidence blocks, capability signals separately, and one coverage row per target. Report inaccessible sources as research limits, never as clean targets.

## 3. Synthesize and verify

Every drift finding requires:

- vendor URL and a short exact quote proving the behavior;
- contradictory repository `file:line`;
- concrete user loss and the smallest fix.

Drop unsupported claims. Reopen every breaking finding's source and repository line yourself. Recheck negative claims through an independent fetch route before accepting absence or removal. A 200 response may be an app shell, soft 404, or unrelated redirect.

Keep built-in drift (a target the built-in emits to stopped behaving as it needs) apart from a coverage opportunity (a target it does not list gained compatible support). Drift is a finding; an opportunity goes under Cross-target opportunities. Apply the fix rule in the built-ins reference: hook capability alone never enables a target.

Collapse one vendor change affecting several targets into one finding, with independent evidence for each target. Sort `breaking`, `missing-feature`, `degraded`, `cosmetic`. Unconfirmed findings belong only in the report, with the question that would settle them.

Before merging capability signals, read `.agnostic-ai/skills/target-audit/references/capability-intelligence.md`. Apply its representation test and per-target verification before choosing `adapter-gap`, `spec-candidate`, `target-extension`, or `watch`. Try existing kinds and `x-<target>` passthroughs before proposing a schema.

When requested, run that reference's bounded challenger pass after synthesis. Record requested and actual model identities (or unavailable), evidence added, and unresolved disputes. Agreement is not vendor evidence.

## 4. Write the report

Write `local/target-audit/<YYYY-MM-DD>.md`. Preserve an existing same-day report or deliberately update it as a continuation. Include audited commit, window, target coverage, counts, source dates, issue/PR links, and research limits.

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

List moved or broken source URLs with verified replacements. Apply corrections to the canonical `.agnostic-ai/skills/target-audit/references/sources.md` during this run, even in report-only mode. Edit source specs, never generated native copies.

After the report is written, run `scripts/docfetch.sh --update local/target-audit/<date>-run/docfetch.tsv <targets that produced a coverage row>`. The lock moves only after a run finishes, so a crash never marks a page as seen that no auditor read. Text snapshots under `local/target-audit/snapshots/` are stored by hash on every fetch, so the next run's deltas diff against exactly the text this lock names; `--update` also drops lock rows for URLs no source lists and snapshots the lock stopped naming 30 days ago. With `--no-file-issues`, leave the lock change in the working tree beside the source corrections. In every other mode, commit it even when the audit is clean or every finding was already tracked, unless `git diff --quiet scripts/target-audit/sources.lock` shows it did not move: in the cosmetic/source docs PR when one exists, otherwise in a `chore(target-audit)` PR of its own, so an audit never leaves `main` dirty and the vendor watcher compares against what was read.

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

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

Fetch the `target-audit` issue index into the run directory with three scoped calls rather than paginating the whole collection: `--state open`, `--state all --search 'created:>=<window start>'`, and `--state closed --search 'closed:>=<window start>'`. Before treating a candidate as new, run one title-scoped search for it (`--state all --search '<target> in:title <keyword>'`). Fetch bodies and comments only for relevant matches. Refresh matching issue state before filing.

Read `scripts/target-audit/signals.tsv` for the signal history, and the published capability entries under `docs/site/content/updates/` for the articles behind it. Preserve every stable signal ID. Give each batch only relevant entries and their source paths. A later briefing omitting a signal does not resolve it.

Read the cross-target kind notes in `docs/site/content/docs/target-behavior.md` once and pass relevant claims to each batch. The fact script prints only the lines that name a target, so a shared paragraph that lists no targets is missing from it; those can stay stale after a target page is fixed.

Take the newest date from the latest completed local report, legacy `extra.audit_marker`, audit issue creation, or commit that records an audit's source corrections in `.agnostic-ai/skills/target-audit/references/sources.md`. A commit that only trims or restructures that file, such as #1109's, is not an audit and does not move the window. Widen it slightly for changelog overlap and record the chosen window. Scratch reproductions and incomplete runs are not completed reports.

Dedupe rules:

- Already open: record `still-open #N`, do not file again.
- Already closed: inspect the resolution and current code. Report a regression only when a resolved gap has returned; closure alone does not prove a fix shipped.
- Prior reports and source notes are leads, never substitutes for fresh vendor evidence.

## 2. Audit in bounded batches

Run `scripts/target-facts.sh --changed local/target-audit/<date>-run/docfetch.tsv`. Numbered lines are deep batches of targets whose pages or changelog moved, at most five and sized to available worker slots. The `sweep` line lists targets whose rows are all unchanged; record them as fast path without an agent, since nothing they serve moved since an auditor last read it. For fewer than six deep targets, audit them inline using `.agnostic-ai/agents/target-auditor.md`. Queue batches when needed; every requested target must be assigned exactly once.

Use `target-auditor` agents named Frodo, Sam, Gandalf, Aragorn, Legolas in batch order. Use returned agent IDs for all messages. Prefer minimal-context spawns when supported. Pass:

- target list, audited commit, date window, repository path;
- the run directory and the target's own `docfetch.tsv` rows;
- shared issue-index path and relevant published signals;
- read-only scope and validation budget.

Do not paste this skill, the full issue history, or unrelated source sections into each prompt. Auditors already have their role instructions. No broad tests during research. Build one current binary for compatible read-only reproductions when needed.

Read only each assigned target's source section. `scripts/target-facts.sh --sources <target>...` extracts those sections without loading the other targets. Run `scripts/target-facts.sh <target>...` once per batch for our claims; read exact implementation lines when checking a candidate.

Every source URL was fetched fresh this run, so the lock decides what a model must read, never whether the evidence is current. Auditors read the saved changelog first, then the saved pages with status `new` or `changed`, retaining each page's URL, fetch date, and exact relevant excerpt. Read the full relevant section when an excerpt leaves scope or precedence unclear. Never treat a prior run's saved page, report, or lock row as current evidence. A target may reach a Clean row on the fast path only when every one of its rows is `unchanged`; mark that row as fast path so a human can see it. Recover every `failed`, `app-shell`, `soft-404`, and `redirected` row by hand through `.agnostic-ai/skills/target-audit/references/fetch-playbook.md`, and report what stays unreadable as a research limit.

Keep raw pages and reproductions in the local run directory when useful. Return concise evidence blocks, capability signals separately, and one coverage row per target. Report inaccessible sources as research limits, never as clean targets.

## 3. Synthesize and verify

Every drift finding requires:

- vendor URL and a short exact quote proving the behavior;
- contradictory repository `file:line`;
- concrete user loss and the smallest fix.

Drop unsupported claims. Reopen every breaking finding's source and repository line yourself. Recheck negative claims through an independent fetch route before accepting absence or removal. A 200 response may be an app shell, soft 404, or unrelated redirect.

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

After the report is written, run `scripts/docfetch.sh --update local/target-audit/<date>-run/docfetch.tsv <targets that produced a coverage row>`. The lock moves only after a run finishes, so a crash never marks a page as seen that no auditor read. Leave the lock change in the working tree beside the source corrections, in report-only mode too.

## 5. File and fix

Unless `--no-file-issues` is set, read `.agnostic-ai/skills/target-audit/references/issues-and-fixes.md` and file confirmed findings. The gitignored report and run directory must not be their only detailed record. After filing, add or update one row per signal in `scripts/target-audit/signals.tsv`. Spec candidates may receive design issues but never enter fix buckets without a separate implementation decision.

With `--fix`, finish synthesis and settle all buckets before launching isolated `adapter-fixer` agents. Use the reference's scope, checklist, and validation rules. Never widen an active fixer's assignment; handle later findings separately.

Finish with the top findings, recommended next actions, report link, and issue/PR links. State validation limits accurately. Stop at reviewable PRs; merging belongs to the human.

## Scheduling and invariants

The `vendor-watch` workflow (`.github/workflows/vendor-watch.yml`) runs `scripts/docfetch.sh` daily with no AI and posts moved pages to one open issue labeled `vendor-watch`. `scripts/vendor-watch.sh` keys each page by URL and hash, so a page is reported once per text change. Start a run from that issue: audit the targets it lists, and close it once the lock moves.

Weekly runs are sufficient for the full registry. A scheduler wraps this skill; unattended runs should use `--fix` or default issue filing so results survive outside ignored local files.

Every registered target needs a `## <target>` section with `docs:` and `watch:` in the source registry. `tests/integration/target_audit_sources_test.go` enforces this. Add missing vendor sources, never weaken the test.

`scripts/docfetch.sh --urls` must resolve at least one docs URL and one changelog URL for every registered target. `scripts/docfetch_test.sh` enforces this, so a source line written outside the documented URL grammar fails the shell suite.

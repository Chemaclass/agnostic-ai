---
name: target-audit
description: Audit every agnostic-ai target against current vendor docs, report evidence-backed drift, and report project capability changes. Use for the recurring adapter truth check or when a tool ships a new config feature.
argument-hint: "[target...] [--no-file-issues] [--fix] [--compare-models <model>]"
disable-model-invocation: false
---

# target-audit

Check whether tools still read what agnostic-ai emits and identify released project capabilities that need a product decision. Complete the requested scope with fresh vendor evidence.

## Scope and boundaries

- No arguments: all registered targets, one issue per confirmed finding.
- Target names: only those targets.
- `--no-file-issues`: report only, no GitHub writes. Leave validated lock and source corrections in the working tree.
- `--fix`: file issues and open fix PRs. Never merge or enable auto-merge.
- `--compare-models <model>`: bounded challenge of breaking findings, spec candidates, and conflicting evidence. Continue if unavailable.

Publish reports and issues, plus implementation PRs under `--fix`. Never create or edit site articles or publication PRs; legacy articles are immutable dedupe history. Generic schema changes need a separate implementation decision.

All paths are relative to the repository root, including from nested generated skills. Edit canonical sources, never generated native copies.

## 1. Prepare and route

Read `.agnostic-ai/skills/target-audit/references/preparation.md` for the coordinator's setup. It owns baseline selection, fresh fetch, explicit resume, issue dedupe, signals, and review reasons. Use `scripts/target-audit/manifest.md` for command syntax and `.agnostic-ai/skills/target-audit/references/metrics.md` to record observed intervals and routing. Initialize metrics before fetching.

Freeze inputs before fetching. Keep repository inputs, source inventory, and lock unchanged until finalization. Parallel implementation belongs in separate worktrees. Reused research needs explicit manifest validation, not matching dates or hashes alone.

Run `scripts/target-facts.sh --changed <run>/docfetch.tsv 5 --builtins-since <rev>` and save review reasons. Preserve reused assignments and remove them from pending batches and sweeps. Every requested target needs exactly one assignment and every requested source needs its own disposition.

If no pending target needs deep review, record justified fast paths and skip triage and auditors. Reused findings, signals, or uncertain publication still need synthesis, relevant issue history, and delivery. Only a run with none of those can skip issue indexing and signals, write its no-change report, and finalize. Reuse never implies Clean.

Otherwise read `.agnostic-ai/skills/target-audit/references/triage.md` before dispatch. Use offline triage when all pending deep targets require built-in review; Jev cannot exempt those targets. Deep review remains the default.

## 2. Investigate bounded batches

Read `.agnostic-ai/skills/target-audit/references/evidence-and-output.md` before research or delegation. The coordinator owns shared inputs, routing, coverage, synthesis, publication, finalization, and metrics. Auditors own per-target investigation and evidence artifacts.

Numbered routing lines are deep batches, at most five. Queue batches to fit available worker slots. Record the `sweep` targets as eligible unchanged-vendor fast paths without an agent. For fewer than six deep targets, investigate inline using `.agnostic-ai/agents/target-auditor.md`.

Use `target-auditor` agents named Frodo, Sam, Gandalf, Aragorn, Legolas in batch order, with minimal context. Use returned IDs for messages. Give each batch only:

- Its targets, audited commit, window, repository and run paths.
- Its fetched rows, deltas, triage leads, and review-reason rows.
- Shared issue-index path and relevant signal IDs and published source paths.
- `builtins.txt`, baseline, affected repository paths, and `builtin-deep:` membership.
- Read-only ownership and a validation budget.

Do not paste the skill, full issue history, or unrelated vendor material into prompts. Auditors follow their role instructions. Build one current binary for compatible isolated reproductions; no broad tests during research.

Finish artifacts before recording hashes. Complete assignments and record page dispositions separately through the manifest. A research limit accounts for coverage but retains the prior source hash. Pending built-in checks block Clean even when vendor text is unchanged.

## 3. Synthesize and confirm

Accept findings only under the evidence contract. Reopen each breaking claim's vendor source and repository line yourself. Independently fetch negative claims before accepting absence or removal; HTTP 200 can still be an app shell, soft 404, or unrelated redirect.

For permission, precedence, discovery, or import claims, read `.agnostic-ai/skills/target-audit/references/behavior-evidence.md`. Require its bounded reproduction, confirmation outcome, and independent check even without `--compare-models`. Keep unresolved claims in Needs a human and never assign them to a fixer.

Distinguish drift in an existing built-in target from an opportunity to enable another target. Apply `.agnostic-ai/skills/target-audit/references/builtins.md`; hook capability alone never enables a target.

Collapse a shared vendor change into one finding with independent evidence for each affected target. Sort breaking, missing-feature, degraded, cosmetic. Drop disproved claims and retain unresolved questions with their missing evidence.

For capability signals or a requested challenger pass, read `.agnostic-ai/skills/target-audit/references/capability-intelligence.md`. Test existing kinds and native passthroughs before proposing a schema. Preserve stable signal IDs. Model agreement is not vendor evidence.

## 4. Report, finalize, and deliver

Read `.agnostic-ai/skills/target-audit/references/report-and-finalize.md` for report sections, metrics, built-in baseline proof, lock finalization, source corrections, and signal publication. Finish the audit interval after synthesis and the report, before implementation. Unknown measurements stay unknown.

Validate complete coverage before updating the lock. Keep source corrections and signal edits until after finalization. Unless `--no-file-issues` is set, commit a changed lock even when the audit is clean or every finding is already tracked.

Before GitHub writes or `--fix` work, read `.agnostic-ai/skills/target-audit/references/issues-and-fixes.md`. Refresh dedupe matches, file confirmed findings with durable evidence, settle all buckets, then launch isolated `adapter-fixer` agents. Never widen an active bucket or implement a spec candidate without a separate decision. Stop at reviewable PRs.

Finish with top findings, recommended next actions, report and issue/PR links, and specific validation limits.

For vendor-watch, tool-load, or source-registry maintenance, read `.agnostic-ai/skills/target-audit/references/scheduled-runs.md`.

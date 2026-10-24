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
- `--no-file-issues`: no GitHub writes or fixers, including when combined with `--fix`. Leave validated audit-maintenance edits in the working tree.
- `--fix`: file issues and open fix PRs. Never merge or enable auto-merge.
- `--compare-models <model>`: bounded challenge of breaking findings, spec candidates, and conflicting evidence. Continue if unavailable.

Default delivery includes confirmed issues and a PR for changed audit-maintenance files. Implementation PRs require `--fix`. Never create or edit site articles or their publication PRs; legacy articles are immutable dedupe history. Generic schema changes need a separate implementation decision.

All paths are relative to the repository root, including from nested generated skills. Edit canonical sources, never generated native copies.

## 1. Prepare and route

Follow `.agnostic-ai/skills/target-audit/references/preparation.md` for the window, baseline, manifest, metrics, fresh fetch, explicit resume, issue dedupe, and routing. Command syntax belongs to `scripts/target-audit/manifest.md`.

Freeze repository inputs, source inventory, and lock until finalization. Preserve validated reused assignments; remove them from pending work. Every requested target needs exactly one assignment and every requested source its own disposition. Reuse never implies Clean.

When no pending target needs deep review, record justified fast paths and skip triage and auditors. Reused findings, signals, or uncertain publication still require synthesis and delivery. Only a pure no-change run can skip issue indexing and signals before reporting and finalizing.

Otherwise follow `.agnostic-ai/skills/target-audit/references/triage.md` before dispatch. Deep review remains the default. All-built-in deep work uses offline triage; Jev cannot exempt it.

## 2. Investigate bounded batches

Read `.agnostic-ai/skills/target-audit/references/evidence-and-output.md` before research or delegation. The coordinator owns shared inputs, routing, coverage, synthesis, publication, finalization, and metrics. Auditors own per-target investigation and evidence artifacts.

Numbered routing lines are deep batches, at most five. Queue `target-auditor` agents named Frodo, Sam, Gandalf, Aragorn, Legolas in batch order to fit worker capacity. Use minimal context and the batch handoff in the preparation reference. Record eligible unchanged `sweep` targets without an agent.

For fewer than six deep targets, or when auditor delegation is unavailable, investigate inline using `.agnostic-ai/agents/target-auditor.md`. Coordinator-authored behavior claims still need a separate checker under the behavior contract; without one, keep them unconfirmed. Reuse one current binary for isolated reproductions; no broad tests during research.

Finish artifacts before recording hashes. Complete assignments and record page dispositions separately through the manifest. A research limit accounts for coverage but retains the prior source hash. Pending built-in checks block Clean even when vendor text is unchanged.

## 3. Synthesize and confirm

Accept findings only under the evidence contract. Reopen each breaking claim's vendor source and repository line yourself. Independently fetch negative claims before accepting absence or removal; HTTP 200 can still be an app shell, soft 404, or unrelated redirect.

For permission, precedence, discovery, or import claims, read `.agnostic-ai/skills/target-audit/references/behavior-evidence.md`. Require its bounded reproduction, confirmation outcome, and independent check even without `--compare-models`. Keep unresolved claims in Needs a human and never assign them to a fixer.

Distinguish drift in an existing built-in target from an opportunity to enable another target. Apply `.agnostic-ai/skills/target-audit/references/builtins.md`; hook capability alone never enables a target.

Collapse a shared vendor change into one finding with independent evidence for each affected target. Sort breaking, missing-feature, degraded, cosmetic. Drop disproved claims and retain unresolved questions with their missing evidence.

For capability signals or a requested challenger pass, read `.agnostic-ai/skills/target-audit/references/capability-intelligence.md`. Test existing kinds and native passthroughs before proposing a schema. Preserve stable signal IDs. Model agreement is not vendor evidence.

## 4. Report, finalize, and deliver

Follow `.agnostic-ai/skills/target-audit/references/report-and-finalize.md` for the report, metrics, built-in baseline proof, validated lock update, and maintenance delivery. Finish the audit interval after synthesis and the report, before implementation. Unknown measurements stay unknown.

Finalize complete coverage before editing source corrections, signal history, or replay cases. Keep recorded evidence artifacts immutable; save later publication state separately.

Before GitHub writes or `--fix` work, read `.agnostic-ai/skills/target-audit/references/issues-and-fixes.md`. Refresh dedupe matches, file confirmed findings with durable evidence, settle all buckets, then launch isolated `adapter-fixer` agents only under `--fix`. Never widen an active bucket or implement a spec candidate without a separate decision. Stop at reviewable PRs.

Finish with top findings, recommended next actions, report and issue/PR links, and specific validation limits.

For vendor-watch, tool-load, or source-registry maintenance, read `.agnostic-ai/skills/target-audit/references/scheduled-runs.md`.

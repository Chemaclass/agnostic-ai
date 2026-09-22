# Capability intelligence

Use this procedure for capability signals, optional model comparison, and
evidence that can inform a later release briefing. Confirmed drift keeps the
evidence and issue flow defined in
`.agnostic-ai/skills/target-audit/SKILL.md`.

## Capability signal contract

A capability signal is a documented, released, project-scoped vendor
addition that materially changes what a team can configure or automate. It
may describe a concept outside agnostic-ai's current spec kinds. It is not a
claim that the adapter is wrong.

Use this schema:

```markdown
### <stable semantic name>
- signal-id: cap-<stable-lowercase-slug>
- targets: <independently verified targets only>
- evidence:
  - <target>: <vendor URL> : "<quoted sentence>"
- observed: <release date or date the source was checked>
- availability: stable | preview | experimental, plus any access gate
- project-scope: <native project path or project behavior per target>
- user-value: <specific team outcome>
- ours: <repository paths inspected and what they model today>
- representation-test: <command or minimal reproduction and observed result>
- semantics: <verified common behavior and per-target differences>
- disposition: adapter-gap | spec-candidate | target-extension | watch
- confidence: confirmed | conflicting
- recommended-action: <one decision or next check>
```

Assign `signal-id` from the capability's user-facing semantic concept, not
the vendor's feature name or current disposition. Once published, never
rename or reuse it. This keeps repeated audits tied to the same decision
history. Record every synthesized signal as one row in
`scripts/target-audit/signals.tsv`; that file and the published articles are
the dedupe history a later run reads instead of the whole issue collection.

Exclude user-tier-only behavior, waitlists, unreleased beta features,
marketing claims without usable documentation, cosmetic changes, and
product rankings. A signal needs a vendor URL and exact quote, but it does
not need a contradictory repository line. Model output and cross-vendor
analogy are not evidence.

## Representation test

Before recommending any disposition:

1. Inspect the relevant spec structs and kinds, adapter capability list,
   renderer or import path, config outputs, and `x-<target>` passthroughs.
2. If an existing representation could cover the capability, create the
   smallest temporary project that exercises it. Run sync and inspect the
   emitted native file. Import it too when the capability has an import side.
3. Record the exact reproduction, emitted result, and any loss of semantics.
   If execution is not meaningful, name the repository paths checked and
   explain why.
4. Do not infer portability. For every target in a comparison, open that
   target's own vendor source and verify its scope, lifecycle, values,
   defaults, and native shape.

An existing generic representation that preserves the documented semantics
is stronger evidence than a new schema proposal. An `x-<target>` escape
hatch proves an immediate target-specific route, not a portable abstraction.

## Dispositions

- `adapter-gap`: the current generic model represents the semantics, but an
  adapter does not emit or import the target's native surface. This can also
  be a normal confirmed drift finding when it meets that stricter contract.
- `spec-candidate`: at least two targets independently document a shared
  project-scoped user intent, the current generic model cannot preserve it,
  and the representation test rules out existing escape hatches as a shared
  solution. Record schema differences and the design decision needed. Never
  treat this as implementation approval.
- `target-extension`: the capability is useful and confirmed, but its
  semantics are target-specific or an `x-<target>` passthrough is the honest
  representation.
- `watch`: the capability is released and meaningful, but evidence conflicts,
  only one target establishes a possible shared concept, or a safe product
  decision needs more information. State the question that changes its
  disposition.

When one signal also proves drift, keep both records linked by `signal-id`.
Do not weaken the drift finding's vendor quote, repository contradiction, or
user-impact threshold.

## Critical-change selection

The report is a decision view, not a vendor changelog dump. Put an item in
`Critical vendor changes` when it has at least one of these consequences,
in this order:

1. Current emitted or imported configuration no longer works.
2. Permissions, code execution, data access, or another safety boundary
   changed for a surface agnostic-ai emits.
3. Independently verified targets now share an unmodeled project intent that
   could remove a material workflow gap.
4. A documented native surface removes a workaround or lossy representation
   users need today.

For each item, state the affected targets, user impact, meaningful semantic
differences, disposition, confidence, and next decision. Keep lower-value
signals in `Cross-target opportunities` or the detailed update sections.
Never rank products or equate release-note prominence with user impact.

## Bounded challenger pass

`--compare-models <model>` is a challenge pass, not a second audit. Give the
challenger only:

- breaking findings,
- synthesized `spec-candidate` signals,
- claims whose sources or target semantics conflict.

Ask it to disprove each claim, inspect the cited vendor source and repository
location, repeat the representation test where relevant, and report omitted
evidence. Do not ask it to scan clean targets or discover unrelated changes.

Use the requested model only when it is distinct from the primary research
model and available in the runtime. Record:

```markdown
- primary-actual: <runtime identity | not exposed>
- challenger-requested: <model>
- challenger-actual: <runtime identity | not exposed | unavailable>
- challenger-result: agrees | adds-evidence | disputes | unavailable
- challenger-notes: <new evidence, exact conflict, or availability reason>
```

Use the actual runtime model identity when exposed. Never guess it from a
display name. If the requested model is unavailable, is the same as the
primary, or cannot be selected, record `unavailable` with the specific reason
and finish the primary audit.

Agreement does not raise evidence quality. New facts count only when they
meet the normal source rules. A dispute with new evidence must be resolved by
the orchestrator. If it cannot be resolved, move the claim to `Needs a human`
or leave the signal at `watch` with the settling question.

## Release briefing inputs

The target audit produces evidence, not a publication. Its local report and
filed issues are inputs to the next release. The audit must not create or edit
an article under `docs/site/content/updates/`, change site templates, or open a
publication PR.

For each item that could matter to release readers, keep these facts together:

- stable signal ID, affected targets, disposition, and confidence;
- direct vendor URL and short exact quote;
- observed or released date and availability state;
- independently verified per-target semantics;
- current agnostic-ai support state and repository evidence;
- representation-test command and result;
- linked drift or design issue when one exists;
- specific user consequence and next decision.

The `cut-release` skill reopens the primary sources and verifies that the news
is still current before selecting it. An audit result is a research lead, not
proof that an upstream change belongs in a public briefing. Release selection
favors breaking behavior, changed defaults, removals, and deprecations, then
safety boundaries and substantial project-scoped additions.

Keep shipped support and upstream news distinct. An adapter fix can appear in
the exact release changelog section. A vendor capability the project has not
shipped can appear only in the upstream section with an explicit support
status. Model output and challenger agreement never count as vendor evidence.

Legacy audit articles remain immutable. Never change their canonical path,
`.html` compatibility alias, `rss_guid`, `audit_marker`, report digest, counts,
or capability markers. Their published signal IDs remain part of the dedupe
history even though future audits do not publish articles.

`spec-candidate` signals can receive a separate design issue. The issue must
carry the vendor evidence, representation reproduction, independently verified
per-target semantics, and explicit decision question. It must not enter
`--fix` until a later implementation decision confirms the generic schema and
scope.

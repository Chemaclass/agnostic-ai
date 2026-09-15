# Capability intelligence

Use this procedure for capability signals, optional model comparison, and
the rolling GitHub radar. Confirmed drift keeps the evidence and issue flow
defined in `.agnostic-ai/skills/target-audit/SKILL.md`.

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
rename or reuse it. This keeps retries and later disposition changes tied to
the same decision history.

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
   emitted native file. Import it too when the capability has an import
   side.
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
signals in `Cross-target opportunities` or the radar. Never rank products or
equate release-note prominence with user impact.

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

## Rolling Target capability radar

The GitHub issue titled `Target capability radar`, identified by the stable
`<!-- target-capability-radar:v1 -->` marker, preserves product decisions
across full, scoped, repeated, and failed runs. Search open and closed issues
before creating one. Prefer the open marked issue. If only a closed one
exists, read its closure reason and comments, carry its evidence and decisions
forward, and do not create or reopen anything until the recorded lifecycle
decision supports it. Put an ambiguous lifecycle under `Needs a human`. Use
the `target-audit` label. Never write it when `--no-file-issues` is set.

The issue is the public developer digest described in
`docs/user/target-radar.md`. The body has one managed envelope and one
stable envelope per signal:

```markdown
<!-- target-capability-radar:v1 -->
<!-- target-capability-radar:start -->
# Target capability radar

This issue tracks consequential upstream changes, current agnostic-ai
support, and proposed improvements. Subscribe to receive each audit digest.

Radar observations are not shipped support. Use
[Targets](https://github.com/Chemaclass/agnostic-ai/blob/main/docs/user/targets.md)
for current support and
[CHANGELOG](https://github.com/Chemaclass/agnostic-ai/blob/main/CHANGELOG.md)
for released agnostic-ai changes.

<!-- target-capability:cap-example:start -->
## Example capability

**What changed:** <documented upstream behavior and affected targets>

**Why it matters:** <specific workflow, safety, or compatibility impact>

**Current agnostic-ai position:** <disposition, shipped support, workaround,
or missing representation>

**Next action:** <one adapter fix, design decision, target extension, or
research check>

<details>
<summary>Target comparison and evidence</summary>

- Targets: <independently verified targets>
- Observed: <release date or source-check date>
- Availability: <stable, preview, or experimental, plus access gate>
- Vendor evidence: <direct vendor URL and exact quote for each target>
- Repository evidence: <full GitHub source permalink pinned to a commit SHA>
- Representation test: <reproduction and observed result>
- Semantics: <verified common behavior and target differences>

</details>

<details>
<summary>Research review</summary>

- Confidence: <confirmed or conflicting>
- Research disagreement: <source or model conflict, or None>
- Decision history: <preserved decision, linked design issue, or open question>

</details>
<!-- target-capability:cap-example:end -->
<!-- target-capability-radar:end -->
```

Keep the four developer questions outside `<details>`. They are the digest,
not metadata. Use at most two balanced disclosure blocks per signal when the
supporting material needs them. Never hide the impact, current position, or
next action inside a disclosure block.

Issue evidence must work for a reader outside the checkout. Keep each vendor
link direct. Convert every repository `file:line` citation into a full
`https://github.com/Chemaclass/agnostic-ai/blob/<commit-sha>/<path>#L<line>`
permalink pinned to the audited commit. Do not publish a `main` branch link
as evidence because its lines move.

Keep prose outside the radar markers byte-for-byte. For a scoped audit, only
add or update signals supported by the audited targets. Preserve every other
signal, prior decision, and target's coverage inside the managed envelope.
Absence from a bounded changelog window is not evidence that a capability
disappeared. Remove or resolve an entry only from explicit current vendor
evidence, and record the reason in its decision history.

Before each mutation, fetch the latest issue body again. Replace individual
signal envelopes by `signal-id`; append new envelopes before the radar end
marker. If markers are missing, duplicated, or malformed, do not rewrite the
body. Keep the local report and put the issue under `Needs a human` in the
run summary.

Merge research into the latest envelope instead of regenerating it from the
current run alone. Never replace recorded decision text, a linked design
issue, or evidence for an out-of-scope target with an inferred outcome. Add
new evidence and disposition history alongside them. Before creating a
design issue, check the radar entry and the normal issue dedupe set for an
existing issue that asks the same decision.

After the body update, add one dated summary comment containing:

```markdown
<!-- target-capability-audit:<YYYY-MM-DD>:<sorted-target-scope>:<report-digest> -->
## Target audit, <YYYY-MM-DD>

**What changed:** <critical changes and changed signal ids, or No consequential changes>

**Why it matters:** <developer impact, or No current user impact>

**Current agnostic-ai position:** <support, workaround, and disposition summary>

**Next action:** <recommended fix, design decision, or next audit>

<details>
<summary>Target comparison and evidence</summary>

<independently verified target differences, direct vendor sources, exact
quotes, full commit-pinned GitHub source permalinks, and representation tests>

</details>

<details>
<summary>Research disagreements and audit coverage</summary>

- Audited: <targets and changelog window>
- Challenger: <requested model, actual model, result, and disputes, or Not requested>
- Preserved: <out-of-scope or unchanged coverage summary>
- Local working report: `local/target-audit/<YYYY-MM-DD>.md`

</details>
```

The body and dated comment must stand alone for a GitHub reader. Publish the
evidence and explanation that support every conclusion. The gitignored local
report is a working artifact and must never carry the only detailed account.

Normalize `<sorted-target-scope>` as sorted lowercase target names joined by
commas, or `all`. Set `<report-digest>` to the first 12 lowercase hex
characters of the completed local report's SHA-256 digest. Before posting,
search all issue comments for that exact marker. If it exists, update nothing
and report the run as already persisted. This makes an exact retry safe after
an ambiguous API response without hiding a distinct same-day run. If body
update or comment creation fails, do not retry blindly. Re-fetch once, check
both the signal envelopes and comment marker, then either complete the
missing step or report the persistence failure. The local report remains the
full record.

`spec-candidate` signals may also receive a separate design issue. Link it
from the radar entry. The issue must carry the vendor evidence,
representation reproduction, independently verified per-target semantics,
and explicit decision question. It must not enter `--fix` until a later
implementation decision confirms the generic schema and scope.

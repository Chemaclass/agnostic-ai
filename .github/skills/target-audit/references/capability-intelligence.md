# Capability intelligence

Use this procedure for capability signals, optional model comparison, and
the public GitHub Pages updates. Confirmed drift keeps the evidence and issue
flow defined in `.agnostic-ai/skills/target-audit/SKILL.md`.

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
history.

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

## GitHub Pages target updates

The public developer digest lives under `docs/site/updates/`, not in a
rolling GitHub issue. `docs/site/updates/index.html` is the archive,
`docs/site/updates/feed.xml` is the RSS feed, and every audit gets a dated
static article. Merging the documentation PR publishes it through the
existing Pages workflow.

Use the current article and `docs/site/updates/styles.css` as the structural
and visual template. Keep the article useful without JavaScript. The first
viewport and opening paragraphs are editorial: they explain the few changes
developers should understand now. Full evidence, target coverage, and
research limits follow lower in the article or inside at most two disclosure
blocks per major signal.

### Durable paths and markers

For a full audit, prefer `docs/site/updates/<YYYY-MM-DD>.html`. For a scoped
audit, use
`docs/site/updates/<YYYY-MM-DD>-<sorted-target-scope>.html`. Normalize scope
as sorted lowercase target names joined by commas, or `all` for a full audit.

Put this exact marker immediately inside the article body:

```html
<!-- target-capability-audit:<YYYY-MM-DD>:<sorted-target-scope>:<report-digest> -->
```

Set `<report-digest>` to the first 12 lowercase hex characters of the
completed local report's SHA-256 digest. Mark each capability section with
its stable signal ID so future audits can recover decision history:

```html
<!-- target-capability:cap-example:start -->
<section>
  ...
</section>
<!-- target-capability:cap-example:end -->
```

Never rename a published signal ID. A later article can update its evidence,
disposition, or action while the earlier edition stays immutable.

`docs/site/updates/index.html` and `docs/site/updates/feed.xml` contain
managed envelopes:

```html
<!-- target-updates:index:start -->
... newest archive entry first ...
<!-- target-updates:index:end -->
```

```xml
<!-- target-updates:feed:start -->
... newest RSS item first ...
<!-- target-updates:feed:end -->
```

If either envelope is missing, duplicated, or malformed, stop publication,
keep the local report, and put the problem under `Needs a human`. Never
regenerate the surrounding page or feed from the current run alone.

### Article contract

The title should describe the week's developer consequence, not the audit
process. The article must answer these questions in visible prose:

- **What changed?** The documented upstream behavior and affected targets.
- **Why does it matter?** The workflow, safety, or compatibility impact.
- **Where is agnostic-ai now?** Shipped support, target extension,
  workaround, adapter gap, or design candidate.
- **What happens next?** The linked implementation issue, design decision,
  target extension, research check, or next audit.

The article must also include:

- the audit date, target count, finding count, and clean count;
- direct vendor sources and exact short quotes for each conclusion;
- repository evidence as full GitHub permalinks pinned to the audited commit;
- independently verified differences for cross-target capability claims;
- the challenger request, actual model, result, and disputes, or `Not
  requested`;
- clean targets and specific research limits;
- a clear statement that observations are not shipped support.

The gitignored local report cannot carry the only detailed explanation.
Article evidence must stand alone for a reader outside the checkout. Keep the
release changelog separate: it records what agnostic-ai shipped, while the
updates archive records what changed upstream and what the project may do.

### Idempotency and preservation

Before writing, search all published articles for the exact audit marker. If
it exists, make no publication change and report the run as already
persisted. This makes an exact retry safe without hiding a distinct same-day
run.

If the preferred article path exists with a different digest, preserve it and
append the new digest to the filename. Never overwrite a published edition.
Before changing the archive or feed, read their latest bytes again and
replace only the content inside the managed envelope. Preserve navigation,
introductory copy, prior editions, and every entry outside the current audit.

The sitemap needs no weekly edit. `scripts/build-site-sitemap.sh` discovers
dated article files and assigns each page its own commit date during the Pages
build.

### Publication PR

File confirmed drift and design issues first so the article can link to the
work. Then:

1. Create the dated article and prepend its archive and RSS entries.
2. Validate HTML, XML, links, markers, responsive rendering, and the Pages
   assembly path.
3. Stage only the update article, archive, and feed, plus a source-index fix
   when the audit confirmed one.
4. Commit with `docs: publish target update for <YYYY-MM-DD>`.
5. Push a dedicated branch and open a PR titled
   `docs: publish target update for <YYYY-MM-DD>`. Never merge it.

Use an isolated worktree from fresh `main` when the current checkout contains
unrelated changes or belongs to another task. A scoped run includes its scope
in the branch, commit, and PR title. Do not open a second PR when the exact
marker already exists in an open publication PR.

If article, archive, feed, push, or PR creation fails, keep the local report,
disclose the failed persistence step, and continue the ordinary confirmed
drift issue flow where possible. Do not fall back to a digest issue or issue
comment.

`spec-candidate` signals may also receive a separate design issue. Link it
from the article. The issue must carry the vendor evidence, representation
reproduction, independently verified per-target semantics, and explicit
decision question. It must not enter `--fix` until a later implementation
decision confirms the generic schema and scope.

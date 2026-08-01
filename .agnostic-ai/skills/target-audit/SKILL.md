---
name: target-audit
description: Audit every agnostic-ai target against its vendor's current docs and report evidence-backed drift. Use for the recurring "are our adapters still true?" check, or when one tool ships a new surface.
argument-hint: "[target...] [--file-issues]"
disable-model-invocation: false
---

# target-audit

Answers one question: **do these tools still read what agnostic-ai
emits?**

Adapters encode a snapshot of every vendor's config format. Those vendors
ship weekly. Drift is silent. A moved skills directory keeps syncing
cleanly, keeps `sync --check` green, and stops reaching the tool. This
skill finds that drift on a schedule instead of via a user bug report.

## Arguments

- no args: audit every registered target
- `claude zed kilo`: audit only those
- `--file-issues`: also open one GitHub issue per confirmed finding.
  Without it, the run stops at the report.

## Phase 1: Scope

```bash
scripts/target-facts.sh --list                                  # registered targets
gh issue list --label target-audit --state all --limit 100 --json number,title,state,createdAt
```

**Dedupe set.** A finding already filed and open is not a new finding.
Note it as `still-open #N` and move on. A finding already filed and
**closed** that reappears is a regression. Say so loudly: it means a fix
was reverted or the vendor moved back.

**Since when.** Auditors need a date to bound their changelog reading.
Take the newest of: the most recent report under `local/target-audit/`,
the newest `target-audit` issue's `createdAt`, or the last commit touching
`references/sources.md`. All three under-report, because a clean run
leaves no trace anywhere, so the window is always a little wide. That is
the safe direction. Re-reading a changelog entry costs nothing, missing
one costs a release.

## Phase 2: Fan out

Never hardcode the target list. It grows. Derive the batches:

```bash
scripts/target-facts.sh --batches 5
```

Spawn one `target-auditor` per printed batch, **all in a single message**
so they run in parallel. Name them after Lord of the Rings characters in
batch order: Frodo, Sam, Gandalf, Aragorn, Legolas.

Registry order is roughly chronological, so batch 1 holds the fast-moving
vendors (claude, codex, gemini, cursor, copilot) and the last batch holds
the newest, thinnest-documented entrants. Both ends need the most
attention, for opposite reasons.

Give each agent its target list, the dedupe set from Phase 1, and the
"since when" date. The agent already knows its method. Do not restate it.

When the run is scoped to fewer than six targets, skip the fan-out and
audit them inline. Spawning costs more than it saves at that size.

## Phase 3: Merge

Findings arrive per agent. Merge them yourself:

1. Drop anything missing a vendor URL **with** a quoted sentence, or
   missing the `file:line` it contradicts. Unevidenced findings are the
   main failure mode of this skill. They cost more to disprove than to
   never file.
2. Spot-check every `breaking` finding yourself: open the cited URL and
   the cited line before it goes in the report. Agents do misread docs.
3. Collapse duplicates. One vendor change often hits several targets. A
   shared `.agents/skills/` move touches codex, amp, zed, crush, and
   openhands at once. File that once, list every affected target.
4. Sort: `breaking`, then `missing-feature`, then `degraded`, then
   `cosmetic`.

## Phase 4: Report

Write `local/target-audit/<YYYY-MM-DD>.md`. `local/` is gitignored, so
`mkdir -p` it first.

```markdown
# Target audit, <date>

Audited <N> targets against vendor docs. <X> findings, <Y> clean.
Window: changes since <since-date>.

## Breaking: we write where the tool no longer reads
## Missing: native surface we skip today
## Degraded: works, but a better surface exists
## Cosmetic: docs only
## Needs a human: unconfirmed, with the question that would settle it
## Clean
## Source fixes needed
```

`Source fixes needed` lists every URL in `references/sources.md` that
moved or 404'd, with its replacement. Apply those edits to `sources.md` in
the same run. That file's accuracy is what keeps the next audit cheap. It
is the only file this skill edits without being asked.

Then print the top findings to the user with a recommended next action
each. Keep it short. The report file holds the detail.

## Phase 5: File issues (only with `--file-issues`)

Per confirmed finding, most severe first:

```bash
gh issue create \
  --title "<target>: <one-line claim>" \
  --label target-audit --label enhancement \
  --assignee Chemaclass \
  --body "<body>"
```

Use `--label bug` instead of `enhancement` for `breaking` findings. Create
the `target-audit` label once if missing:
`gh label create target-audit --description "Drift found by the target-audit skill" --color 5319e7`.

The body carries the evidence verbatim (vendor URL, quoted sentence, our
`file:line`, user impact, proposed fix) plus a checklist of what a fix
must touch:

- [ ] `internal/adapters/<target>/`: emission and `caps.Supports`
- [ ] the `import` side, if the moved path is one we read back
- [ ] adapter package doc comment
- [ ] `docs/user/targets.md`: capability matrix row and per-target section
- [ ] `references/sources.md`, if a URL moved
- [ ] tests: `capability_parity_test.go`, `kitsink_golden_test.go`, and
      the target's round-trip test under `tests/integration/`
- [ ] `agnostic-ai sync`, then commit the regenerated per-target files

Never open an issue for an `unconfirmed` finding. Those go in the report
under `Needs a human` with the question that would settle it.

## What this skill does not do

It does not change adapters. A confirmed finding becomes an issue, then
the normal `gh-issue` flow implements it with tests. Keeping audit and fix
separate is deliberate: an audit that also edits code cannot be run
unattended.

## Invariants worth knowing

`references/sources.md` must carry one `## <target>` section, with a
`docs:` and a `watch:` line, for every registered target.
`tests/integration/target_audit_sources_test.go` fails the build
otherwise, so a new adapter cannot merge un-audited. If that test fails,
the fix is to add the vendor's docs, never to relax the test.

## Scheduling

The skill is the unit of work. Scheduling wraps it.

- Ad hoc: `/target-audit`
- Recurring in-session: `/loop 7d /target-audit`
- Unattended: a cloud routine (`/schedule`) or a GitHub Actions cron
  running `claude -p "/target-audit --file-issues"`. Unattended runs
  should always pass `--file-issues`. A report nobody reads is not an
  audit.

Weekly is the right cadence. Batch 1 drifts within a release cycle. The
long tail rarely moves in under a month.

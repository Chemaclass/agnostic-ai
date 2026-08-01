---
name: target-audit
description: Audit every agnostic-ai target against its vendor's current docs and report evidence-backed drift. Use for the recurring "are our adapters still true?" check, or when one tool ships a new surface.
argument-hint: "[target...] [--file-issues]"
disable-model-invocation: false
---

# target-audit

Answers one question: **is what agnostic-ai emits still what these tools
actually read?**

Adapters encode a snapshot of 25 vendors' config formats. Those vendors
ship weekly. Drift is silent — a moved skills directory keeps syncing
cleanly and stops reaching the tool. This skill finds that drift on a
schedule instead of via a user bug report.

## Arguments

- no args -> audit all 25 registered targets
- `claude zed kilo` -> audit only those
- `--file-issues` -> also open one GitHub issue per confirmed finding
  (without it, the run stops at the report)

## Phase 1: Scope

```bash
scripts/target-facts.sh --list          # registered targets
git log -1 --format=%cd -- .agnostic-ai/skills/target-audit/references/sources.md
gh issue list --label target-audit --state all --limit 100 --json number,title,state
```

The last audit's issues are the dedupe set. A finding already filed and
open is not a new finding; note it as `still-open #N` and move on. A
finding already filed and **closed** that reappears is a regression —
say so loudly, it means a fix was reverted or the vendor moved back.

## Phase 2: Fan out

Spawn one `target-auditor` per batch, **all in a single message** so they
run in parallel. Name them after Lord of the Rings characters.

| Agent   | Targets                                  | Why grouped                    |
|---------|------------------------------------------|--------------------------------|
| Frodo   | claude, codex, cursor, copilot, gemini    | highest churn, ship weekly      |
| Sam     | amp, zed, opencode, crush, warp           | AGENTS.md / `.agents/` cluster  |
| Gandalf | cline, windsurf, continue, trae, junie    | IDE-plugin cluster              |
| Aragorn | kiro, antigravity, jules, goose, augment  | newer entrants, thin docs       |
| Legolas | qoder, openhands, factory, kilo, aider    | newest + long-tail             |

Give each agent: its target list, the dedupe set from Phase 1, and the
date of the previous audit (use `sources.md`'s last commit date; that is
what "since last time" means). The agent already knows its method — do
not restate it in the prompt.

When the run is scoped to fewer than six targets, skip the fan-out and
audit them inline. Spawning costs more than it saves at that size.

## Phase 3: Merge

Findings arrive per agent. Merge them yourself:

1. Drop anything missing a vendor URL **with** a quoted sentence, or
   missing the `file:line` it contradicts. Unevidenced findings are the
   main failure mode of this skill — they cost more to disprove than to
   never file.
2. Spot-check every `breaking` finding personally: open the cited URL and
   the cited line before it goes in the report. Agents do misread docs.
3. Collapse duplicates. One vendor change often hits several targets
   (a shared `.agents/skills/` move touches codex, amp, zed, crush,
   openhands). File that once, list every affected target.
4. Sort: `breaking` > `missing-feature` > `degraded` > `cosmetic`.

## Phase 4: Report

Write `local/target-audit/<YYYY-MM-DD>.md` (`local/` is gitignored):

```markdown
# Target audit — <date>

Audited <N> targets against vendor docs. <X> findings, <Y> clean.

## Breaking — we write where the tool no longer reads
## Missing — native surface we skip today
## Degraded — works, but a better surface exists
## Cosmetic — docs only
## Clean
## Source fixes needed
```

`Source fixes needed` lists every URL in `references/sources.md` that
moved or 404'd, with its replacement. Apply those edits to `sources.md`
in the same run — that file's accuracy is what keeps the next audit
cheap. It is the only file this skill edits without being asked.

Then print the top findings to the user with a recommended next action
each. Keep it short; the report file holds the detail.

## Phase 5: File issues (only with `--file-issues`)

Per confirmed finding, most severe first:

```bash
gh issue create \
  --title "<target>: <one-line claim>" \
  --label target-audit --label enhancement \
  --assignee Chemaclass \
  --body "<body>"
```

Use `--label bug` instead of `enhancement` for `breaking` findings.
Create the `target-audit` label once if missing:
`gh label create target-audit --description "Drift found by the target-audit skill" --color 5319e7`.

Issue body carries the evidence verbatim — vendor URL, quoted sentence,
our `file:line`, user impact, proposed fix — plus a checklist of what a
fix must touch:

- [ ] `internal/adapters/<target>/` — emission + `caps.Supports`
- [ ] adapter package doc comment
- [ ] `docs/user/targets.md` — capability matrix row + per-target section
- [ ] `.agnostic-ai/skills/target-audit/references/sources.md` if a URL moved
- [ ] tests: `capability_parity_test.go`, `kitsink_golden_test.go`
- [ ] `agnostic-ai sync` + commit the regenerated per-target files

Never open an issue for an `unconfirmed` finding. Put those in the report
under a `Needs a human` heading with the question that would settle it.

## What this skill does not do

It does not change adapters. A confirmed finding becomes an issue, then
the normal `gh-issue` flow implements it with tests. Keeping audit and
fix separate is deliberate: an audit that also edits code cannot be run
unattended.

## Scheduling

The skill is the unit of work; scheduling wraps it.

- Ad hoc: `/target-audit`
- Recurring in-session: `/loop 7d /target-audit`
- Unattended: a cloud routine (`/schedule`) or a GitHub Actions cron
  running `claude -p "/target-audit --file-issues"`. Unattended runs
  should always pass `--file-issues` — a report nobody reads is not an
  audit.

Weekly is the right cadence. The fast-moving five (Frodo's batch) drift
within a release cycle; the long tail rarely moves in under a month.

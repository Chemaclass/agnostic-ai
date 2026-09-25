# Issues and fix PRs

Read this when filing audit findings or running `--fix`. All paths are repository-root relative.

## File actionable evidence

Refresh matching issue state and relevant comments before filing. Open findings are `still-open #N`; resolved gaps that return are regressions. Never file an unconfirmed finding.

Create the `target-audit` label only if absent:

```bash
gh label create target-audit --description "Drift found by the target-audit skill" --color 5319e7
```

File most severe first, assigned to Chemaclass, labeled `target-audit` and `bug` for breaking findings or `enhancement` otherwise. Use a structured body argument or an exact Markdown file with `--body-file`.

Each issue must explain what changed, why it matters, current support, and the smallest next action. Include the vendor URL and exact quote, repository evidence linked to the audited commit, user impact, and reproduction. Keep evidence detailed enough to act without the ignored report.

For multi-target findings, include one checkbox per target with its native path and implementation location. Identify the cheapest independent fix and why. A partial PR must not close the whole issue.

Include applicable checklist items, marking exclusions with a reason:

- Adapter emission, `caps.Supports`, and package doc comment.
- Import side when the affected surface is read back.
- `docs/site/data/capabilities.toml`, the target page, and cross-target notes in `docs/site/content/docs/target-behavior.md`.
- Configuration/spec docs and schema when their public contract changes; README for visible behavior.
- `CHANGELOG.md` under Unreleased.
- Source-registry URL corrections, plus `scripts/target-audit/sources.lock` and `signals.tsv`.
- Target tests, `capability_parity_test.go`, `kitsink_golden_test.go`, and `tests/integration/fixtures/golden/<target>/`. Check whether a round-trip test exists; golden trees often exist without one.
- `make playground-build` if capabilities change. The playground derives capabilities from the compiled registry; never add a second list.
- Build the current binary, sync generated outputs, review tracked changes, and verify `sync --check`. Keep ignored generated files ignored.

Design issues need the capability reference's evidence, representation test, independently verified semantics, and explicit decision question. Filing is not approval to implement a generic schema.

## Settle fix buckets

| Bucket | PR scope |
|---|---|
| Breaking | One PR per target and breaking finding |
| Missing-feature and degraded | One additive batch |
| Cosmetic and audit-source corrections | One docs PR, including `scripts/target-audit/sources.lock` and `signals.tsv` |
| No findings | One `chore(target-audit)` PR with the lock update alone |

Settle every bucket before spawning. Give each `adapter-fixer` the issue links and verbatim finding blocks, including vendor quote, repository line, and reproduction. If newer research changes the issue's conclusion, include the URL, extraction method, and command that establishes it. Do not ask fixers to trust a summary or infer one vendor's schema from another.

Every fixer must use a separate worktree branched from fresh `origin/main`. Use native worktree isolation when available; otherwise create the worktree before spawning and pass its absolute path as the exclusive edit location. Never let two fixers sync in one checkout. Respect the runtime's worker capacity and queue settled buckets when necessary.

Tell workers their ownership, that peers are working concurrently, and not to revert others' edits. A peer or orchestrator must not widen an active bucket. Later findings get another isolated task or are handled by the orchestrator.

## Validate and hand off

Acceptance is the smallest user-visible scenario that closes the issue. State it and non-goals before implementation. Preserve adapter independence and existing user files.

Follow `.agnostic-ai/agents/adapter-fixer.md`. Coordinate expensive checks across workers: targeted reproductions during diagnosis, then one `make preflight` per completed code change set and one WASM build where capabilities changed. Fix failures before pushing. Rebuild after rebasing over adapter changes; a stale binary gives false sync drift.

Fixers may reject a finding. Recheck their evidence; if wrong, explain what was missed on the issue and label it `invalid`. Do not reopen a disproved claim or force the proposed implementation.

Review completed diffs against the evidence. Open PRs with `Closes #N` only for fully resolved issues. Never merge or enable auto-merge.

When checking CI, use `gh pr checks N` and verify the current head SHA has actual checks. Missing checks or pending conclusions are not success. Read failing logs before retrying; check GitHub platform status when checks are absent or stale.

If an authorized rebase is needed, preserve both valid sides of shared docs and recompute counts from source. A clean merge can still leave duplicate table rows or contradictory prose; inspect both. Do not run merge-specific work merely because this audit opened several PRs.

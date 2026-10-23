# Jev triage for target audits

[Contributor docs](README.md) · [Decision 005](decisions.md#005-jev-triage-for-target-audits-optional-and-measured) · [Target audit skill](../../.agnostic-ai/skills/target-audit/SKILL.md)

`scripts/jev-triage.sh` is an optional first pass that decides which changed pages `/target-audit` should review. It asks TypeSafe's Jev, a model that returns structured answers instead of prose, which changed vendor pages probably break a claim agnostic-ai makes, and which pages are safe to skip. The audit works the same without it.

Use `scripts/jev-triage.sh --lexical <run> <pending-deep-target>...` when every pending deep target is in `builtin-deep:`. Those targets need full review regardless of Jev scores. Other runs use the API mode when available. Limit either mode to pending deep targets so resumed and unchanged coverage does not consume triage work. This changes triage cost, not the required evidence or review coverage.

## What it does

The script reads a `scripts/docfetch.sh` run directory. For each changed page with a `.delta`, it sends one request to Jev with the page's changed text and every claim `scripts/target-facts.sh` prints for that target. Each claim is a Choice question (a selection from named answers) (`supports`, `contradicts`, `says_nothing`). One Noul question (a numeric score) per page asks whether the change documents a configuration feature for a project, which is the only check that can catch a new tool configuration feature that none of our current claims covers.

It writes two TSV files into the run directory:

- `triage.tsv`: one row per possible problem, most likely difference from vendor documentation first. A Jev row is kept only when its verdict is `contradicts` or its `p_contradicts` reaches 0.4, or, for the question about configuration features, when its Noul reaches 0.5.
- `triage-pages.tsv`: one status per changed page. `clear` means Jev answered every question for the page, kept no row, its score for configuration features stayed under 0.1, and no path we write moved on it (`mentions:` label). The others are `lead`, `judged` (no lead, but a `mentions:` page or a score for configuration features of 0.1 or more), `unjudged` (a request was capped or failed), and `lexical` (Jev did not run).

The skill reviews targets with a possible problem from Jev first and passes the rows to reviewers as questions. A target can take a "fast path by Jev" only when every changed row is `clear`, it is absent from `builtin-deep:`, and a manual delta check finds no built-in hook or skill behavior changes. Each run still reviews one randomly chosen target from that group. If that review finds a problem, skipping reviews is disabled until a saved test case for the problem passes.

## Per-run metrics

When jq is available, the script also writes versioned `triage-metrics.json` with run identity, its stage interval, mode, answered and capped requests, returned token usage, and research limits. Missing usage is null, distinct from measured zero; a known subtotal survives partial responses. The stderr format and TSV columns stay the same, with `unknown input tokens` when an answered request omits usage. Metrics failures preserve the offline fallback.

The audit coordinator records the rest of the run with `scripts/audit-metrics.sh`, including overlapping stages, approved omitted reviews, samples, built-in overrides, and findings linked to known leads. Follow the [metrics reference](../../.agnostic-ai/skills/target-audit/references/metrics.md) for commands and report fields. Audit elapsed time ends before later fixes and merges. Counts of omitted reviews are routing observations, not time saved. Compare clean, vendor-change, and built-in-only runs with their provenance and research limits; replay on isolated excerpts does not measure full-run recall or prove a speedup.

## Backward compatibility

Without `TYPESAFE_API_KEY`, with `--lexical`, or when the API fails, the script still exits 0. `triage.tsv` then holds possible problems found by matching words (claims that share a path, key, or term with the changed text), and every page is `lexical` or `unjudged`, so an auditor reads every changed row as before. Where Jev does not run, or leaves a page part unjudged, trim-1 adds a second set of possible problems (see below). A page with no saved earlier copy has no change to compare, never appears in `triage-pages.tsv`, and always keeps the full review.

A 401, 403, no response, or exhausted retries stop further requests. Any other 4xx loses only that request's page. `JEV_MAX_REQUESTS` (default 40) caps a run; pages past the cap are `unjudged`.

## trim-1 leads without a key

[trim-1](https://github.com/caveman-ai/trim-1) is a small offline model (pure Go, 1.2 MB, deterministic) that scores how much each line of a text matters to a question. When Jev does not run (no key, `--lexical`, or no `curl` or `jq`), or leaves a page part unjudged (a failed or capped request), the script runs it on those parts through `scripts/trim-leads`, a separate Go module, so the release binary never includes it.

For each changed page part and each claim of its target, the helper scores the part's changed lines and the target's other claims, with the claim as the question. When a changed line ranks in the top `JEV_TRIM_TOP` (default 5), the claim becomes a row with verdict `paired`, via `trim`, and trim-1's score for that line as its probability. The target's other claims are what a changed line has to beat: most `.delta` parts are one to five lines, so ranking those lines alone would keep every claim. For a target with few claims, the cut shrinks to the top fifth of its claims, rounded up, so unrelated text cannot outrank them all: Jules has 9 claims, so its cut is 2.

`triage.tsv` lists lexical rows first, then trim-1 rows by score. A page and claim pair that matched words already keeps its lexical row, and a claim Jev answered gets no trim-1 row. Page statuses do not change.

Setup: Go 1.23 or newer. The first run downloads trim-1 at the version `scripts/trim-leads/go.mod` pins, checked against `go.sum`; later runs reuse Go's caches. To skip the build on each run, build once with `go build -C scripts/trim-leads -o /some/dir/trim-leads .` and set `JEV_TRIM_HELPER` to that file. If Go is missing or the helper fails, the run keeps the lexical rows and prints one `trim-1 skipped (reason)` line.

## Setup on a laptop

Store the key in the macOS Keychain, typing it at the prompt so it never reaches shell history:

```sh
security add-generic-password -a "$USER" -s typesafe-api-key -w
```

Load it in every zsh shell from `~/.zshenv`:

```sh
export TYPESAFE_API_KEY="$(security find-generic-password -a "$USER" -s typesafe-api-key -w 2>/dev/null)"
```

Start Claude Code or Codex from a new terminal so the session inherits it. The script's standard-error message says which method ran: `N of M Jev requests answered, T input tokens`, `lexical and trim-1 leads only (reason)`, or `lexical leads only (reason)`. Remove the line to go back to word-matching mode.

## Cost

TypeSafe charged $0.042 per million input tokens with free output (dashboard footnote, 2026-09-28). One changed page is one request of about 11,500 input tokens, about $0.0005.

| Day | Jev requests | Cost |
|---|---|---|
| No pages changed | 0; the audit stops after the fetch | $0 |
| 3 to 30 pages changed | 3 to 30 | $0.0015 to $0.015 |
| Past the cap | 40 | about $0.02 |

The full audit of 2026-09-28 (25 targets, 19 changed pages) made 13 requests with 136,688 input tokens. The TypeSafe dashboard moved from 848 to 861 requests, by 156,131 tokens including free output, and by $0.0057. Tuning the script the same day, mostly replay runs, cost 848 requests, 7.3 million tokens, and $0.27; do not repeat that in daily use.

## Measure before changing it

Two sets with expected answers live beside the saved source versions:

- `scripts/target-audit/triage-cases.tsv`: a claim, a short exact vendor quote, and the expected verdict, from closed target-audit issues.
- `scripts/target-audit/surface-cases.tsv`: a vendor quote and whether it documents a project configuration feature, from records of newly supported features, issues, and saved pages.

Run both with a key before changing a default, a threshold, or the question wording, and at least twice, because Jev's answers vary slightly between runs:

```sh
scripts/jev-triage.sh --replay scripts/target-audit/triage-cases.tsv
scripts/jev-triage.sh --replay-surface scripts/target-audit/surface-cases.tsv
```

Each run costs about $0.01 to $0.015. Without a key, or with `--lexical`, the claim replay costs nothing and prints only the recall of word matching, trim-1, and either of them. After an audit files a finding or records a signal, add its case to the matching file.

## What the measurements showed

All numbers are from 2026-09-28.

**Ask every claim, not a word-matching shortlist.** The first version sent Jev only the claims that shared a term with the change. That filter dropped 5 of the 11 confirmed differences before Jev saw them, and the replay skipped the filter, so it could not show the loss.

**Put all of a target's claims in one request.** Each labeled excerpt was hidden in about 3,400 characters of text resembling changed content built from the target's own saved vendor pages, with the target's claims beside it:

| Claims per request | Differences caught by the selected answer | Lowest `p_contradicts` on a difference | Highest on anything else | Input tokens per claim |
|---|---|---|---|---|
| 1 | 11/11, 11/11 | not measured | not measured | about 1,465 |
| 6 | 10/11, 9/11 | not measured | not measured | about 440 |
| 12 | 10/11, 9/11, 10/11 | 0.34 | 0.22 | about 340 |
| 40 (default) | 11/11 in 5 runs | 0.63 | 0.16 | about 250 |

**Request a review at `p_contradicts` 0.4.** At 40 claims per request, confirmed differences scored 0.63 or more and everything else 0.16 or less. A 0.8 threshold would have missed 2 or 3 of the 11 differences. Unrelated claims crossed 0.3 in about 2% of answers and 0.5 in under 1%.

**Keep the question wording.** Putting the claim instructions in the model's state reduced tokens by 16%, but a confirmed difference scored 0.39 while an unrelated claim scored 0.62.

**The question about configuration features is weaker, so skipping a review requires a strict threshold.** Over 16 cases about configuration features:

| Wording | Features caught at 0.5 | False alarms at 0.5 | Lowest score on a feature | Highest score without a feature |
|---|---|---|---|---|
| "says it was added" | 6/10 | 0/6 | 0.13 | 0.22 |
| "documents a surface" (current) | 7 or 8/10 | 0/6 | 0.16 | 0.43 |
| plus frontmatter examples | 8/10 | 0 or 1/6 | 0.22 | 0.52 |

Every real configuration feature scored 0.16 or more across three runs, and 12 of 18 answers without a configuration feature scored 0.04 or less, so a page is `clear` only under 0.1.

**Word matching is the fallback, not a substitute.** Matching path segments and plurals improved word matching from 6 to 8 of 11 differences. On the 33-case set it pairs 12 of 18, where Jev flags all 18.

**Without a key, trim-1 finds drift that word matching misses.** On the 70 claim cases of 2026-10-14, with each excerpt ranked for its case claim against the target's claims:

| Method, no key | Differences found (54) | Rows on unrelated text (9) | Rows on the 2026-10-14 audit (48 page parts) |
|---|---|---|---|
| Word matching | 31 | 1 | 154 |
| trim-1, top 5 (default) | 39 | 2 | 48 more |
| Either, top 5 | 44 | 2 | 202 |
| Either, top 10 | 45 | 2 | 240 |
| Random ranking, top 5 | about 8 | about 1.5 | |

The random row is what the cut would keep if trim-1 ranked lines at random. Top 10 found one more difference for 86 trim-1 rows instead of 48, so the default is 5. Before the cut for small targets, top 10 added 164 rows and flagged every Jules claim for a pricing sentence. A no-key run on that audit took about 3 seconds.

**Real audits agreed with the manual reviews.** On the 2026-09-28 full audit, Jev's leads pointed at both confirmed findings (#1376 at `p_contradicts` 0.52 to 0.68, #1377 through a score for configuration features of 0.87), cleared kiro and junie, and the random spot check of junie came back Clean. On a Copilot page that the manual review found Clean, the word-matching pass had produced 3 stray leads and Jev none.

## Limits

- 35 claim cases and 18 cases about configuration features are small sets, and each excerpt is asked alone, not inside a real page change.
- The trim-1 replay ranks each excerpt alone. In a run, the other changed lines of the same part compete too.
- The threshold for configuration features of 0.1 keeps some harmless pages `judged`; on 2026-09-28 it cost one extra auditor read (Cursor's subagents page).
- Pages without a saved earlier copy need a full review. Claude, Codex, and Warp had such pages on 2026-09-28.
- Jev judges our claims and one question about configuration features; it does not write findings, verify repository lines, or file issues. The auditors still do that.

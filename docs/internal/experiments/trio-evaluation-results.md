<a id="whole-session-rtk-and-caveman-results"></a>

# RTK and Caveman results for complete sessions

RTK alone had the lowest observed cost estimate reported by the CLI using published prices for both larger diagnostic tasks in both repetitions. Adding Caveman cost more than RTK alone in every paired task. The short, unsupported command found no consistent reduction with RTK, and the skill configurations cost more. Keep both features opt-in; choose Caveman for its response style rather than a cost claim from this study.

Collected on 2026-10-10 for [#1961](https://github.com/Chemaclass/agnostic-ai/issues/1961), following the unchanged [plan written before collection](trio-evaluation-analysis-plan.md). The [method](trio-evaluation.md) and [full evidence](trio-evaluation-data/) accompany these results. There are 24 planned and attempted cases, not a selected subset of successful sessions.

<a id="scope-and-provenance"></a>

## Tasks, versions and source records

Each live Claude session summarized one fixed, harmless failing transcript. These are synthetic diagnostic tasks, not a measurement of code repair or a real project test suite. Payment has 700 passes and one failure, Unicode has 300 passes and two failures, and short has zero passes and one failure. Each command exits 7. Each answer must name every failing test, its path, and its expected and actual values on the same line. The visible summaries contain enough information to answer. The study rules reject calls to retrieve saved output.

The design is four configurations, three tasks and two repetitions, shuffled with seed 1957. Configurations are concise instructions, RTK, explicitly invoked Caveman, and both. Each starts with a fresh local Claude profile and separate case directory. All have Bash, Skill, the same three skills supplied by Claude and the same three built-in plugins; Caveman is the only extra skill in its 12 enabled cases. No personal plugin or MCP server was loaded. HOME stays unchanged. On macOS RTK still reads its existing configuration; history and recovery stores are redirected. Whether the provider has already cached shared input is uncontrolled.

The collection uses clean main `9932169b2d0836bea33144c0d9413052c488bddf`, Claude Code 2.1.295, RTK 0.51.0, `claude-haiku-5-5`, and pinned Caveman revision `2e08b9177c07bb7249a8a2d1a6758e5db281d002`, on macOS arm64. The agnostic-ai binary prints version 0.81.0 but is an unreleased main build, not the published release. The published 0.81.0 release lacks these builtins. Exact binary, runner, settings, fixture and source hashes are in the [manifest](trio-evaluation-data/final/manifest.json) and [export provenance](trio-evaluation-data/final/export-provenance.json). Its Go build information records the exact revision and `vcs.modified=false`.

The measured runner SHA256 is `828de40f154b2eb49d610125269fc206cfd30d41fdc1bd588684206a797b1d0a`. The SHA256 of the plan written before collection is `b5f2f29c9d7808d3f21771e7800216e580f20e0a63dc6877bde8ddbe58dce93b`. The later #1971 check fix is for future runs only. This collection, its original automated rejection and excluded comparison remain unchanged.

<a id="all-attempts-correctness-and-accounting"></a>

## All attempts, answer checks and usage

All 24 sessions completed with valid setup, execution and complete measurement. A separate Sol 6 reviewer accepted all 24 final answers, without seeing the configuration, costs, case labels or automated verdicts. Its verdicts were fixed before comparative cost analysis. This was model review, not human correction of answers. The original automated answer check accepts 23 cases; its disagreement is described below. There are 18 planned comparisons; 17 pass both the original and independent answer checks.

Every command executed once with exit 7. Each of the eight larger cases with RTK calls its hook once and runs the RTK command once. Each of the four short cases with RTK calls its hook once. The hook succeeds without returning a replacement, so Claude runs the original command. The other 12 cases invoke neither. All eight short cases preserve the original 204-byte output after Claude's exit-prefix and trailing-newline convention. The 12 skill cases explicitly load the exact generated pinned body before Bash. No visible retry, extra task call, recovery read, permission denial or human answer correction occurs.

Claude shortens the four payment outputs from commands run without RTK: the result text including its exit prefix changes from 21,230 to 10,040 bytes and contains a truncation marker. All decisive failure facts remain visible. Unrewritten Unicode and short output remains intact. Thus the comparison includes Claude's output shortening; it is not a comparison with every original byte delivered to the model.

Sixty unique assistant message IDs reconcile input and cache counters with the final result and per-model usage. Streamed assistant output counters are placeholders; final result and modelUsage agree on the actual output. Thinking is included within output and is not added again. See Claude's [usage accounting rules](https://code.claude.com/docs/en/agent-sdk/cost-tracking).

All required token, cost and elapsed values are known, with zero unknown attempts in each category. The final-only estimate is $0.02404614. All session limits are $0.12, and the complete cost record with the maximum reserved for each next session stayed within the selected $2 allowance. The outer allowance is recorded in the [collection invocation](trio-evaluation-data/final/collection-invocation.json), not its manifest. Prices are cost estimates reported by the CLI using published prices, not account invoices; connection-level retries and actual billed spend cannot be observed.

| Configuration | Planned / attempted | Independent / original answer-check passes | Input | Cache creation | Cache read | Output (thinking within it) | Estimate, USD | Session seconds |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| concise | 6 / 6 | 6 / 6 | 24 | 24705 | 32800 | 1738 (812) | 0.00614040 | 32.513 |
| rtk | 6 / 6 | 6 / 6 | 24 | 8528 | 32812 | 1817 (848) | 0.00294462 | 34.236 |
| caveman | 6 / 6 | 6 / 5 | 36 | 36672 | 65110 | 2312 (1034) | 0.00914510 | 38.388 |
| both | 6 / 6 | 6 / 6 | 36 | 20354 | 65062 | 2182 (961) | 0.00581602 | 38.804 |

These are six-attempt totals per configuration for the equal-frequency mix of three synthetic tasks. The Caveman total includes the rejected attempt. Smaller terminal output did not reduce complete-session output tokens here: RTK output rose from 1,738 to 1,817. Its main recorded reduction is cache creation input, from 24,705 to 8,528. The other token categories stay separate.

The skill configurations add 68 prompt bytes, a 4,557-byte generated body and an actual Skill turn per session. Their higher cache-read totals include that activation. These are observed complete-session differences, not a fixed universal overhead rate. Sync elapsed time, generated bytes and prompt bytes are separate setup measurements in the raw rows. Shared local caches and provider conditions prevent a controlled cold-start comparison.

<a id="every-paired-difference"></a>

## Every task-by-task difference

The cache columns count input tokens saved for reuse and input tokens read from that saved copy. Output includes thinking tokens. A qualified comparison passes both answer checks.

Each value is configuration minus concise, paired by task and repetition. Negative means less of that recorded measure. Repetition numbers are the original zero-based case labels. Raw numeric differences remain visible for the rejected comparison; they are not successful-task savings. The [case CSV](trio-evaluation-data/final/case-metrics.csv), [paired CSV](trio-evaluation-data/final/paired-metrics.csv) and [analysis JSON](trio-evaluation-data/final/paired-analysis.json) retain the underlying values.

| Task | Repeat | Configuration | Qualified | Input | Cache creation | Cache read | Output | Thinking within output | Estimate, USD | Seconds |
| --- | ---: | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| payment | 0 | rtk | yes | +0 | -4181 | +2 | +115 | +82 | -0.00077868 | +0.818 |
| payment | 0 | caveman | yes | +2 | +2016 | +5384 | +204 | +123 | +0.00055924 | +1.092 |
| payment | 0 | both | yes | +2 | -2252 | +5374 | +97 | +52 | -0.00034796 | +0.785 |
| payment | 1 | rtk | yes | +0 | -4181 | +2 | +44 | +9 | -0.00081418 | +0.316 |
| payment | 1 | caveman | no | +2 | +2015 | +5384 | +72 | -18 | +0.00049304 | +0.921 |
| payment | 1 | both | yes | +2 | -2189 | +5374 | +103 | +30 | -0.00033236 | +1.032 |
| unicode | 0 | rtk | yes | +0 | -3915 | +2 | -6 | +17 | -0.00078598 | +0.793 |
| unicode | 0 | caveman | yes | +2 | +2004 | +5392 | +63 | +16 | +0.00048642 | +1.234 |
| unicode | 0 | both | yes | +2 | -1927 | +5381 | +86 | +50 | -0.00028839 | +1.102 |
| unicode | 1 | rtk | yes | +0 | -3890 | +2 | -87 | -85 | -0.00082148 | -0.119 |
| unicode | 1 | caveman | yes | +2 | +1982 | +5392 | +26 | -9 | +0.00046352 | +0.759 |
| unicode | 1 | both | yes | +2 | -1933 | +5381 | -32 | -68 | -0.00034859 | +0.997 |
| short | 0 | rtk | yes | +0 | -5 | +2 | -7 | -5 | -0.00000448 | -0.096 |
| short | 0 | caveman | yes | +2 | +1975 | +5379 | +89 | +42 | +0.00049349 | +0.808 |
| short | 0 | both | yes | +2 | +1975 | +5376 | +75 | +25 | +0.00048646 | +0.880 |
| short | 1 | rtk | yes | +0 | -5 | +2 | +20 | +18 | +0.00000902 | +0.011 |
| short | 1 | caveman | yes | +2 | +1975 | +5379 | +120 | +68 | +0.00050899 | +1.061 |
| short | 1 | both | yes | +2 | +1975 | +5376 | +115 | +60 | +0.00050646 | +1.495 |

For payment, RTK is lower by $0.00077868 and $0.00081418; both is lower by $0.00034796 and $0.00033236. For Unicode, RTK is lower by $0.00078598 and $0.00082148; both is lower by $0.00028839 and $0.00034859. All eight comparisons qualify. The larger-task group chosen before collection totals are $0.00524974 for concise, $0.00204942 for RTK and $0.00393244 for both: 60.96% and 25.09% lower against concise, calculated from group sums. Both still costs $0.00043072 to $0.00049759 more than RTK alone in each of all six direct pairs. Percentages from individual tools are never added.

Short RTK differences split directions: -$0.00000448 and +$0.00000902. Its total is $0.00089520 against concise $0.00089066. Since the command is unchanged, these tiny session differences do not establish a filtering benefit. Caveman and both cost more in both short repetitions, with totals $0.00189314 and $0.00188358. Prefer the concise setup for this short diagnostic task when cost is the objective.

Caveman-only Unicode and short comparisons qualify and cost more in both repetitions. Its second payment comparison fails the original automated gate, so no two-qualified-repetition claim is made for that task. Its known costs remain in every applicable total.

RTK is slower in four elapsed comparisons and faster in two, with differences from -0.119 to +0.818 seconds. Caveman and both are slower in all six observed pairs, by 0.759 to 1.234 and 0.785 to 1.495 seconds. Two repetitions cannot establish a stable speed effect, statistical significance, equal answer quality across other workloads or expected savings on other projects.

## Recorded check disagreement

`payment-caveman-1`, blind label B004, reports the exact counts and linked failure and says `` `cargo test --test payment` failed (exit code 7). `` The original automated answer check looks only for the literal phrase `command failed`. It records `correct: false`, with that missing statement as its sole complaint. The independent reviewer accepts the content. A separate raw-stream audit confirms that the named command is the exact executed command, resolving the blind review's limit on that extra string.

Keep both verdicts, the $0.00181431 estimate, 6.387320834 seconds and the comparison excluded from successful-task claims. This is a correct answer rejected by the wording check, not evidence of lost diagnostic facts. [#1971](https://github.com/Chemaclass/agnostic-ai/issues/1971) adds a regression and exact named-command support for future runs. No answer, raw event, prompt, case, recorded flag or final comparison was replaced or rescored. No further provider request was made to fix this disagreement.

## Pilots and earlier work

The original four-case trial run has a $0.00317468 estimate and four accepted answers. Its first export altered public accounting labels and pinned source text, so it failed publication review. #1970 repaired export while preserving the private original and failed copy. The [original-pilot archive](trio-evaluation-data/original-pilot/raw.tar.gz) is the subsequently verified export of those same four calls, not a new run.

After that repair landed, a fresh four-case trial run on `9932169b` passed all six conditions written before collection. Its separate estimate is $0.00277400; a written [GO decision](trio-evaluation-data/fresh-pilot/decision.md) preceded the final collection. Both trial-run datasets and their estimates remain excluded from the final 24-case calculations.

The retained historical cost record lists 16 legacy debug attempts, including eight failed setup checks because RTK was not applied, at $0.01703452; eight isolated debug attempts at $0.00794282 lack the final host/source controls. Those attempts remain excluded, not substituted for final cases. Preparation and repeated exports added no observed provider calls. The [historical snapshot](trio-evaluation-data/historical-ledger.json) predates the fresh pilot and final collection. Its counts do not include those later phases. Missing unretained attempts and costs stay unknown, so no complete investigation cost is claimed. The separate live-hook study has eight calls at $0.00246653 and is not pooled into this study.

## Publication and reproduction

Three archives contain the exact reviewed copies with private details removed: 580 final files, 110 original-pilot files and 110 fresh-pilot files. [Archive hashes](trio-evaluation-data/artifacts.json) identify the containers; each export manifest separately records private-original and hashes of the copies with private details removed and byte counts. No source file is silently replaced by its hash of its edited copy. All 4,820 numeric and boolean values, usage objects, fixtures, public pinned assets and 12 loaded skill bodies survive the final export unchanged. Paired identifiers retain their relationships. The published evidence excludes authentication, home-directory, profile and saved-output files. The privacy review checks those selected files for the recorded private names, paths, identifiers and credentials; it is not a claim to detect every private fact in arbitrary prose.

Verify each archive and recompute all pairs without a provider request:

```sh
python3 docs/internal/experiments/trio-evaluation-verify.py \
  docs/internal/experiments/trio-evaluation-data
python3 docs/internal/experiments/trio-evaluation-analysis.py \
  docs/internal/experiments/trio-evaluation-data/final \
  docs/internal/experiments/trio-evaluation-data/final/blind-verdicts.json \
  docs/internal/experiments/trio-evaluation-data/final/blind-map.json \
  /tmp/trio-recomputed-pairs.json
```

The output path must be new. The archive contains sanitized JSONL streams, prompts, answers, hook frames, generated settings and pinned skill assets. To inspect them, extract a selected `raw.tar.gz` into a new temporary directory. The exact [collection runner bytes](trio-evaluation-data/final/collection-runner.txt) and its recorded commit remain available; current main includes the later check fix. A new measured collection requires its own fresh pilot, directory and recorded method revision.

The stored [answer review](trio-evaluation-data/final/answer-review.md), [raw accounting review](trio-evaluation-data/final/accounting-review.md), [export review](trio-evaluation-data/final/export-review.md) and [independent arithmetic review](trio-evaluation-data/final/analysis-review.md) explain their separate checks. They record the original data review; this report and the public guide receive a separate final review before merge.

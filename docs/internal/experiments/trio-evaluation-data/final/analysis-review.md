# Final #1961 analysis review

No arithmetic or denominator defect found in the supplied paired analysis. The original automated failure is retained correctly. The result supports a narrow preference for RTK alone on the two larger diagnostic fixtures, not a claim that combining the tools is cheapest or that RTK saves on unsupported short commands.

Reviewed the final collection at commit `9932169b2d0836bea33144c0d9413052c488bddf`, the paired-analysis JSON, predeclared analysis plan, original results, independent blind verdicts and mapping, and the answer-review report. No measurements were edited. No provider calls, builds, gates, commits, or repository edits were performed.

## Independent arithmetic and denominator checks

Recomputed every candidate-minus-concise difference directly from original rows using Decimal values: all 126 differences match the published JSON. Recomputed all 28 arm metric totals; each matches its original six attempts. All four arms have six planned and six attempted cases. There are exactly 24 unique cases, 18 unique planned comparison slots, and 17 qualified pairs. No row is absent, duplicated, or silently replaced.

The review map is one-to-one and covers all 24 cases. All 24 blinded content verdicts are pass. All 24 sessions completed, all setup/comparison flags pass, and all recorded accounting fields used in the paired analysis are known. Each row records one fixture execution, zero visible tool retries, and zero recovery calls. These are checks against the recorded results; they are not a new independent audit of the private raw provider transport.

| Arm | Attempts | Independent content passes | Original qualified cases | All-attempt CLI list-price estimate, USD |
| --- | --- | --- | --- | --- |
| Concise | 6 | 6 | 6 | 0.00614040 |
| RTK | 6 | 6 | 6 | 0.00294462 |
| Caveman | 6 | 6 | 5 | 0.00914510 |
| Both | 6 | 6 | 6 | 0.00581602 |

The final collection total is USD 0.02404614 in CLI list-price estimates. It is not an invoice or a whole-investigation total. The Caveman total includes the ineligible payment attempt's USD 0.00181431 and 6.387320834 seconds. Unknown accounting counts are zero for this collection; the report should still retain the original completeness flags rather than infer completeness from a successful answer.

## Oracle disagreement must remain visible

`payment-caveman-1`, blind label B004, says `` `cargo test --test payment` failed (exit code 7). `` and reports the correct counts and linked error details. Its only oracle complaint is the absence of the literal phrase `command failed`. The independent content review passes it. The named command matches the original row's command, resolving the blind review's explicit limit about that extra string.

The paired JSON correctly keeps the original `correct: false` result ineligible. Thus Caveman has five qualified comparisons, not six, and only three of its four larger-output comparisons qualify. A future #1971 checker improvement cannot be retroactively substituted into this dataset. Describe this as an automated wording-check mismatch, not as a loss of diagnostic facts caused by Caveman. Keep the row, cost, answer, and both verdicts visible; do not remove it from all-attempt totals or promote the pair to qualified.

## Recommendation boundaries fixed by the plan

RTK and both each cost less than concise in both payment repetitions and both unicode repetitions. Every one of those comparisons qualifies. The larger-output group totals are concise USD 0.00524974, RTK USD 0.00204942, and both USD 0.00393244. Reductions calculated from those sums are 60.96% and 25.09%, respectively. Those percentages describe four observed synthetic diagnostic comparisons at the recorded versions and model, not expected production savings.

Both costs more than RTK directly in all six task/repetition pairs, by USD 0.00043072 to USD 0.00049759 per attempt. This supports RTK alone as the simpler cost choice for the tested larger diagnostic tasks. Do not add single-tool percentages or imply an additional combined benefit. The combined arm's large-task reduction against concise does not mean that adding Caveman improved RTK.

Short RTK cost differences are -USD 0.00000448 and +USD 0.00000902. The direction is mixed; the predeclared rule does not permit a positive savings recommendation for this task. The short command was unsupported and unchanged, so these tiny session differences cannot be attributed to RTK output compression. Its summed estimate is USD 0.00089520 versus concise USD 0.00089066, which also establishes no useful observed reduction.

For short, Caveman and both each cost more than concise in both repetitions. The corresponding two-case totals are USD 0.00189314 and USD 0.00188358. Keep the integrations off for this small diagnostic workload when reduced cost is the objective. This does not preclude choosing the response style for an unmeasured preference.

Caveman-only unicode and short comparisons all qualify and cost more in both repetitions. Its payment pair 1 is ineligible, so do not make a two-qualified-repetition quality claim for payment. The whole-arm USD 0.00914510 remains a valid all-attempt expenditure total, including the oracle mismatch. It is not a six-success efficiency result.

The all-three-task RTK total is 52.05% below concise, while both is 5.28% below concise. These are descriptive totals for the predeclared equal-frequency task mix. They cannot override the mixed short result or justify an all-task recommendation. No significance, population quality equivalence, monthly savings, or stable performance claim follows from two repetitions.

## Tokens, time, and overhead

The token categories remain separate. RTK's six-attempt cache-creation total falls from 24,705 to 8,528, while cache-read input rises from 32,800 to 32,812 and output rises from 1,738 to 1,817. Consequently “RTK reduced output tokens” is false for this collection's whole-session total, even though it reduces the larger terminal transcripts. Attribute the recorded accounting components precisely.

Caveman and both add an explicit skill call and loaded instructions. Their uncached input totals are 36 versus baseline 24, cache-read totals 65,110 and 65,062 versus 32,800, and output totals 2,312 and 2,182 versus 1,738. These are observed whole-session totals that include activation, not an isolated estimate of the skill's intrinsic overhead. Output contains thinking, which must not be added a second time. Generated bytes and setup timing are separate telemetry.

RTK is slower in four of six elapsed-time pairs and faster in two, with deltas ranging from -0.119 to +0.818 seconds. Caveman and both are slower in all six observed pairs, by about 0.759 to 1.234 seconds and 0.785 to 1.495 seconds. Report observed seconds and tradeoffs without a stable speed claim. Provider conditions and caches remain uncontrolled; two repetitions do not characterize latency tails.

The accepted protocol excludes recovery and extra calls. Zero recovery reads and retries do not prove that real tasks require neither. This evaluation covers answers recoverable from the visible summary, not code repair or the cost of subsequent human corrections. Setup sync time cannot be amortized into provider savings or compared as controlled cold-start performance.

The final public report and guide were not yet supplied for this review. Their wording should preserve these denominators, the original mismatch, the separate token categories, and the workload-specific recommendation boundaries.

Publication note: local source paths in this review are role labels. Scratch audit commands describe the original review environment. Use the verification and arithmetic commands in the main results report for the published artifacts.

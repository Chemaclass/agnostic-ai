# Predeclared analysis for #1961

Prepared before inspecting any final provider results. Method reviewed at commit `594958fd2402779fc5c228616166f49af996da6a`; final collection must use the merged harness on current main and record its actual commit and binary hashes. Sources: [issue #1961](https://github.com/Chemaclass/agnostic-ai/issues/1961), `docs/internal/experiments/trio-evaluation.md`, and `trio-evaluation.py` at that revision. This plan does not change the runner or authorize more requests.

## Question and fixed scope

For three fixed diagnostic tasks, does enabling RTK, the explicitly invoked Caveman response skill, or both reduce the observed whole-session CLI list-price estimate while preserving the required answer and execution behavior compared with concise instructions?

The final design is 24 planned cases: payment, unicode, and short; four arms; two repetitions; seed 1957. Pair by `(task, repetition)`, regardless of execution order. Each nonbaseline arm therefore has six planned comparisons with concise. Keep payment and unicode as the predeclared larger-output group (four pairs per arm), and short as the no-op group (two pairs per arm). Do not redefine groups after seeing outcomes.

This is a diagnostic summary task, not test execution performance, code repair, general coding quality, or production cost. The fake command must fail with exit 7; that expected failure is successful task evidence when the assistant reports it correctly. A failed evaluation row means the task, protocol, setup, or session did not meet its checks.

## Four-case live pilot: go or no-go

Use a separate new directory with `--tasks short --repetitions 1 --live`, the final selected binaries and pinned skill. Preserve its manifest, four expected case IDs, raw events, answers, known costs, and a written decision. Pilot rows never enter the final 24-case calculations.

Go requires all of the following, not merely four answers:

1. Four attempts complete, all runner `correct` checks pass, and an independent reader confirms the counts, linked identifier/path/expected/actual facts, explicit failure statement, and absence of contradictory claims.
2. All four host inventories pass and share the same common host profile. Bash and Skill and the three native skills are present throughout; only the generated Caveman skill differs as planned. No personal plugins, unexpected skills, or MCP servers appear.
3. Every case runs exactly one unchanged `fixture-report short`, returns the original full short output with exit 7, and has matching hook/tool evidence. RTK arms each invoke the emitted processor once, receive a successful empty replacement reply, and invoke no RTK host wrapper. Disabled arms invoke neither RTK processor nor host wrapper.
4. Caveman arms explicitly call Skill(caveman), verify the generated project's pinned body and source, then call Bash once. Other arms call Bash once. No denial, retry, extra call, or recovery call occurs.
5. All input, cache creation, cache read, output, cost, and elapsed fields needed for publication are present and reconciled with unique assistant messages and final modelUsage/result accounting. Missing thinking detail is allowed if identified as unavailable, since it is already within output. No secret or unreviewed private identity enters the export.
6. Per-session limits and the separate pilot budget were honored; the final run still has its approved budget. The pilot is a host and instrumentation check, not evidence that RTK successfully rewrites the two larger tasks. That remains a final-run acceptance check.

Any failed condition is no-go. Keep every attempted row and known expense. Classify the cause before another request: authentication/host inventory, instrumentation, missing accounting, or genuine task/protocol failure. Do not relax the oracle or tune prompts to erase a bad answer. A harness repair requires a recorded method revision and another complete four-case pilot in a new directory; retain the earlier attempt. A genuine pilot failure requires an explicit decision about a revised experiment, rather than silent retries until four pass. Pilot cost or token direction alone is never a go criterion.

## Evidence and independent correctness review

Before examining arm costs, give the independent reviewer each final answer, fixture truth, and a case label, withholding arm/cost where practical. Record per-answer verdict and concrete missing or incorrect facts. Preserve the original verdict if corrected later, with the reason. No model-answer edits or human fixes can turn the recorded attempt into a successful answer.

A publication-eligible successful row requires runner `correct`, independently accepted content, verified setup and protocol, and complete session evidence. Measurement completeness is a separate condition. Thus a correct answer with unknown usage remains correct but cannot support a complete cost comparison. If the reviewer and oracle disagree, resolve the discrepancy against the fixed task requirements before making a recommendation; record the discrepancy instead of silently dropping the row.

Keep all planned case IDs in the analysis, including not-started cases after a budget or host stop. Distinguish planned, attempted, session-completed, protocol-valid, independently correct, and measurement-complete counts. Record setup failures before a provider call separately. Failed and incomplete attempted rows remain in every applicable count, known cost subtotal, observed latency listing, and paired table. A stopped dataset is an incomplete experiment, not a smaller replacement design.

## Measurements and arithmetic

Publish raw per-case values first: uncached input tokens, cache creation input tokens, cache read input tokens, output tokens, optional thinking tokens within output, CLI list-price estimate in USD, elapsed seconds, tool sequence/count, executions, retries, attempted recovery reads, quality verdict, protocol/setup verdict, and measurement completeness. Include reason codes for failures and unknown values. Do not count thinking twice or convert generated bytes into provider tokens.

For each task and repetition and each of rtk/caveman/both, compute `delta = arm - concise` separately for every metric that is known in both rows. Negative cost/latency/token deltas mean less of that metric. Give all six pair slots even if a pair is invalid or missing; label its quality and completeness. Raw numeric deltas for failed attempts may be shown, but never called savings for successful work. The correctness-qualified subset is secondary and must expose its denominator alongside the full outcomes.

For cost, optional relative reduction is `100 * (concise - arm) / concise`, only with a known positive baseline and complete quality-qualified pair. Zero denominators produce no percentage. For any group whose planned rows all qualify, compute aggregate reduction from the two group sums, not the mean of pair percentages. Keep absolute USD beside percentages. Do not add RTK and Caveman percentages: the both arm is measured directly against its paired concise baseline. Comparing both with each single arm is secondary and paired on the same task/repetition.

For each arm and each fixed task, show both repetitions and their deltas. Summaries may show median and observed range, but cannot replace the two observations. Also show totals across all six attempts per arm and the predeclared large/no-op groups where complete. An overall total is only the equal-frequency mix of these three synthetic tasks, not an estimate of a user's workload.

All-attempt totals include known costs of failures, denials, and interrupted runs. If any cost is unknown, report a known subtotal and unknown-case count; the whole total is unknown. The same rule applies independently to each missing token category and elapsed measurement. Do not impute zero or a budget cap as an actual charge. Report accepted tasks out of planned and attempted tasks next to expenditure. Do not calculate a lower cost per successful task by discarding failures or invent the expense of a retry that did not happen.

Reconcile accumulated final usage against unique message IDs and modelUsage before publication. Any unexplained mismatch makes that measurement unresolved; retain both raw counters and state which comparisons cannot be supported. Label cost throughout as a CLI list-price estimate, not billed spend. Provider-side reasoning beyond exposed counters remains unknown.

Elapsed time covers the host session and all task requests, including skill activation and failure paths. Include observed failed/timeout duration; it is not successful-task latency. Report paired seconds without a speedup claim from two samples. Preserve shuffled order because time, provider caches, and service conditions can vary despite fresh local profiles.

## Predeclared recommendation rules

Correctness takes priority over cost. A configuration that fails either repetition of a task cannot be recommended as preserving quality for that task from this study, even if its failed answer is cheaper. If concise fails, that pair cannot establish a successful-task cost improvement over concise; keep both outcomes visible. An incomplete or invalid comparison yields no positive savings recommendation.

A candidate can be described as using a lower observed list-price estimate for a particular task only if both repetitions of it and concise meet all quality, protocol, and measurement criteria and its estimate is lower in both paired observations. Report both absolute differences, token components, and latency tradeoffs. If signs differ, call the result mixed. If cost is equal, call it no observed cost reduction. No arbitrary minimum percentage or significance threshold is introduced after results exist.

A larger-output recommendation requires that condition separately for payment and unicode. An all-three-task recommendation also requires it for short. Otherwise limit the recommendation to the qualifying named tasks, or keep the integration off for this tested workload when it adds cost, fails required facts, or has incomplete evidence. More compressed terminal output or a shorter final response alone cannot justify enabling an arm. Prefer the simpler configuration when this small experiment establishes no useful difference; do not claim an unmeasured universal break-even point.

For short, explicitly report processor-only overhead and skill activation/catalog overhead alongside the preserved output. It is valid and informative for RTK to leave the command unchanged and save no tokens, or for either integration to increase total usage. Do not infer a universal overhead amount from two samples. Do not call a combined result synergistic merely because it is smaller: describe only its measured paired differences.

Two repetitions per task cannot establish statistical significance, stable tail latency, population quality equivalence, or general expected savings. Use no p-values, confidence intervals, or extrapolated monthly savings. The report's strongest conclusion is a workload- and version-specific observation with all six pair outcomes visible.

## Boundaries and publication

The accepted protocol requires one task command and excludes recovery and extra calls. Preapproval does not make other calls impossible. Retain attempted recovery/retries as protocol failures with their known usage, but zero accepted recovery calls cannot show that real tasks never need recovery. No cost of human corrections or code repair is measured; record any actual correction intervention separately without pretending it is part of the provider totals. Exact recovery and missing stores belong to the separate runtime experiment.

Sync elapsed time, generated bytes, prompt bytes, skill bytes, YAML, hashes, and source versions are setup/provenance telemetry. They are separate from live token/cost/latency outcomes. Native cache warmth is uncontrolled, so do not rank arms by sync timing or amortize setup cost into provider savings. Skill loading and extra activation requests that actually occur remain within whole-session usage.

Publish the predeclared plan, actual collection commit and hashes, fixed settings/model/task fixtures, randomized order, separate pilot disposition, all 24 planned case slots, independent answer review, raw paired metrics, and a short scoped recommendation. Retain exploratory and failed pilot datasets separately with known phase costs; do not claim an investigation-wide total without a complete ledger. Review the allowlisted export for privacy and preserve private original hashes. If a method change becomes necessary after final data collection starts, date and explain it, retain the interrupted dataset, and label the later collection as a distinct version rather than replacing unfavorable rows.

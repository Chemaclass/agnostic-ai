# Final 24-case instrumentation and accounting review

No verified instrumentation or accounting defect found in `private-final-source`. All 24 planned sessions completed with valid execution protocol and complete usage/cost accounting. The original runner retains 23 passing task verdicts and one false negative; the parent reports fixed blind verdicts of 24 passes. The original dataset and qualification flags were not changed.

## Provenance and independent checks

The manifest baseline and build revision both equal `9932169b2d0836bea33144c0d9413052c488bddf`. Independently hashed the selected agnostic-ai, RTK, and Claude executables against their manifest hashes. Read-only Go binary information verifies the exact agnostic-ai revision and `vcs.modified=false`. Agnostic-ai binary SHA-256 is `81da49052803957ada1f8972e56c10f7c7e938d5a1db2201f7a6bfe29d09764b`. The measured runner matches manifest SHA-256 `828de40f154b2eb49d610125269fc206cfd30d41fdc1bd588684206a797b1d0a`.

Ran the provided accounting audit after reviewing its source. It reports zero findings, 24 planned/attempted/completed sessions, no missing cases, 24 complete accounting records, and no unknown token, cost, or elapsed-time values. Independently reconstructed raw host profiles, compared actual fixture bytes and prompts with the committed runner templates, checked real observed fixture arguments, verified all selected Caveman source files against the bundled revision and digests, and checked actual loaded skill bodies. These independent checks do not rely on the helper's zero finding count.

| Requirement | Actual evidence |
| --- | --- |
| Balanced schedule and retained attempts | Three tasks, four arms, two repetitions. Each task has eight cases and each arm six. The shuffled order exactly matches the recorded seed and plan. There are exactly 24 private case directories with streams and 24 result rows. No failed or partial attempt was discarded. |
| Fixture and prompt consistency | All 24 transcripts equal the committed fixture generator byte for byte, with matching row/manifest hashes. All 24 CLI prompt strings equal the predeclared templates, including only the intended Caveman activation prefix. Observed fixture argv matches the prescribed program and arguments. |
| Common host setup | All 24 raw system init inventories equal their recorded inventories. Reconstructed canonical profiles match in every case. Actual inventory has Bash and Skill, three identical builtin plugin identity triples, the same three native skills, and no MCP servers. Caveman is the only extra skill in its 12 enabled cases. |
| Actual generated Caveman activation | Twelve actual successful Skill calls select caveman. Each synthetic load names the exact generated case skill directory and contains the exact generated body. The generated body after its header and all associated assets match the selected pinned source. All six selected source files match bundled upstream revision `2e08b9177c07bb7249a8a2d1a6758e5db281d002` and its digests. Off arms have no generated Caveman skill. |
| Native RTK behavior | Eight large-task cases have one actual native processor call and one command wrapper call. Four short-task cases have one processor call, empty successful reply, and no wrapper call. The twelve off-arm cases have no RTK calls. Processor/command argv logs exactly match their intended roles. Large-task native pre-tool replies rewrite to the prescribed `rtk` command; post-failure events record that same rewritten command. Short cases retain the original command and empty reply. |
| Execution and failure evidence | Every fixture executes exactly once with its expected arguments. Every actual Bash tool result reports exit 7. All required test identifiers, error paths, and expected/actual values remain visible. Each post-failure hook error equals the raw model-visible Bash failure text. All eight short cases retain their exact original output after the documented CLI exit-prefix/trailing-newline convention. |
| No extra work | Tool sequences are Bash alone or successful Skill then Bash. No extra task command, visible tool retry, recovery read, permission denial, API-error status, safety stop, or subagent call occurs. |
| Complete accounting | All 24 result usage objects and modelUsage buckets match their result rows. Sixty unique assistant message IDs reconcile uncached input, cache creation, and cache reads with final totals. Repeated streaming content blocks are deduplicated. All final output counters reconcile with modelUsage; all 24 assistant-output sums are placeholders rather than final output totals. Thinking is within output and agrees across final accounting views. |

## Whole-session totals and budget

All-attempt totals, with separate cache categories:

| Measure | Total |
| --- | ---: |
| Uncached input tokens | 120 |
| Cache creation input tokens | 90,259 |
| Cache read input tokens | 195,784 |
| Output tokens (thinking included) | 8,049 |
| CLI list-price estimate, USD | 0.02404614 |
| Summed session elapsed seconds | 143.94099379099998 |

Every modelUsage record labels cost basis as list. Model bucket costs reconcile with each final result and retained row. The terminal summary matches the sum of all 24 retained attempts and reports 24 completed of 24 planned. These totals describe complete diagnostic sessions, including setup-visible skill/context and tool turns, not only each final request. Elapsed time includes each CLI session startup and task calls; the sum is not a parallel collection wall-clock measurement.

All actual provider argv set the per-session cap to 0.12 USD. Each session's recorded estimate stays below it. The sequential cumulative ledger plus the next 0.12 reservation stays below the predeclared/default 2.0 USD total allowance throughout. No budget, unknown-cost, or host-inventory stop occurred. The outer runner's selected total-budget option is not stored in the manifest; its default/predeclared allowance and the entire resulting ledger were checked, rather than treating that missing option as a recorded measurement.

The CLI list-price estimate is not actual billed spend. The streams do not expose provider transport retries. Final output is corroborated by the CLI result and modelUsage views, but cannot be reconstructed from assistant output placeholders. These limits are retained in the numeric summary.

## Original verdict discrepancy

The retained `payment-caveman-1` row is `correct=false`, with only `explicit command failed statement` missing and no contradiction. Independently checked the raw final result, not only the row answer: it says `` `cargo test --test payment` failed (exit code 7). `` The actual raw Bash request is exactly `cargo test --test payment`, and the requested fixture executed once. Its reported counts and linked failure details remain intact. The original oracle requires the literal phrase `command failed`, which rejects that explicit statement naming the command.

The parent reports fixed blind verdicts of 24 passes. This audit did not read the blind-review answer packets or independently rerun those verdicts. It independently verifies the specific false-negative reason against the private raw result and exact executed command. Keep `correct=false`, the runner's 23/24 count, and its 17/18 qualified pairs. Report the separate independent blind-review count without retroactively changing the predeclared oracle or rerunning a case.

## Native Bash truncation observation

The four unrewritten payment cases contain Claude's native `... [11230 characters truncated] ...` marker. Their original result text including the exit prefix is 21,230 bytes; actual tool-result text is 10,040 bytes. All decisive failure details survive. This truncation is already in the native Bash result and matching post-failure hook error, not introduced by RTK or the exporter. The four unrewritten Unicode cases and all eight short cases preserve the original output text. Eight rewritten large cases intentionally show RTK summaries.

Therefore an exact-output claim applies to the eight short cases, and to the independently checked unrewritten Unicode cases. It does not apply to every large unfiltered transcript. This observation changes no verdict or measurement and is recorded separately from instrumentation defects.

Safe aggregate counts, totals, independent checks, and observations are saved in `private-final-source-accounting.json`. No private identity names, credentials, session IDs, blind-review materials, or comparative ranking is included.

Publication note: local source paths in this review are role labels. Scratch audit commands describe the original review environment. Use the verification and arithmetic commands in the main results report for the published artifacts.

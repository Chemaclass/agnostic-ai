# RTK and Caveman runtime composition, issue #1960

This local experiment tested RTK 0.51.0 with Caveman CLI 2.1.0 and its signed `bin-v2.1.0` engine on macOS arm64. The [runner](../../scripts/rtk-caveman-runtime.py) generates fixed test output, runs each command once, and writes [machine-readable results](rtk-caveman-runtime-results.json). It uses temporary home, history, and recovery stores, passes no provider credentials to either tool, and never starts a proxy or coding agent.

## Recommendation

Keep RTK as the command-output filter and keep Caveman command shrinking off in the initial project recipe. For the ordinary failing test fixture, RTK reduced 23,865 original bytes to a 240-byte summary, and Caveman left that summary unchanged. A constructed case with four repeated 1,000-character success lines in RTK's failure tail gave Caveman more to remove, reducing 4,248 RTK-summary bytes to 2,215 bytes. The one-command `tools shrink -- rtk test ...` path emitted 2,367 bytes after adding its recovery status line. That proves a nested transformation can run, not that a second stage helps typical coding tasks. It adds a second recovery store and another failure path. The task-quality and provider-cost effect is unmeasured.

If a user deliberately combines the tools for a supported command, use RTK's command wrapper with its SQLite recall store enabled. Preserve the RTK recall hint through any Caveman transform and keep Caveman's recovery store available. Do not use `rtk pipe` as a substitute for this recovery path: its filtered text alone cannot restore elided command output. The tested `caveman tools compress` entry point exits 1 with no compact output when its recovery-store parent is unavailable, so a caller must retain the RTK summary on error. The command-running `caveman tools shrink -- ...` path kept the RTK summary and exit status in the same store-failure case. These are distinct interfaces and need separate handling.

The runner also sent a 21,219-byte Rust test transcript to `rtk pipe --filter cargo-test`. It returned 239 bytes with the failure path, no recall hint, and exit 0 because it filtered stdin rather than running the failing test command. A shell pipeline must preserve the original command's exit status separately. This input-filter path is useful for a known disposable transcript; it does not provide the two-store recovery contract above.

## Data flow and exact recovery

| Case | Model-facing steps | Source each recovery returned | Failure and exit |
| --- | --- | --- | --- |
| Ordinary failure | Original 23,865 bytes → RTK 240 bytes → Caveman 240 bytes | `rtk recall --full` returned the exact 23,865-byte original; Caveman made no handle | RTK exit 7; error and path visible |
| Repeated tail | Original 27,975 bytes → RTK 4,248 bytes → Caveman 2,215 bytes | `caveman tools retrieve` returned the exact 4,248-byte RTK summary; `rtk recall --full` separately returned the exact 27,975-byte original | RTK exit 7; error, path, and RTK recall hint visible after both steps |

Each command wrote a counter once. Retrievals, including attempts against the wrong stores, left the counter at one. The Caveman handle identified only the bytes Caveman received. It did not recreate pre-RTK output. The runner records SHA-256 digests for originals and transformed outputs so a repeat can compare bytes without storing a large fixture in Git.

## Failure and no-op cases

- A wrong Caveman store could not retrieve the RTK summary. A wrong RTK store could not retrieve the original. Neither command was run again.
- A missing Caveman engine passed the RTK summary through unchanged. A six-byte input also passed through unchanged.
- With an unavailable Caveman recovery-store parent, `tools compress` exited 1, wrote no compact output, and named the store error. `tools shrink -- rtk test ...` still ran the command once, retained exit 7 and the decisive error, and passed the RTK summary through.
- With an isolated RTK recall store disabled, RTK still ran the failing command once and returned a filtered response without a recall hint. That mode cannot support a claim that all elided bytes remain recoverable.
- A missing RTK executable under `tools shrink -- ...` failed without running the target command. The error was not treated as a successful run.
- A nested `tools shrink -- rtk test ...` wrapper ran the harmless failing command once, returned exit 7, and kept `tests/payment_test.go:42` plus the expected/actual values visible.
- The same wrapper on the repeated-tail case reduced the RTK summary, preserved both recovery hints, returned exit 7, and retrieved the exact RTK summary without rerunning the command. Its appended status line explains the difference between its 2,367 output bytes and the direct compressor's 2,215 bytes.

The local runner checked immediate retrieval only. [RTK 0.51.0 documents](https://github.com/rtk-ai/rtk/blob/v0.51.0/docs/guide/getting-started/configuration.md) default SQLite recall limits of 10 MiB per entry, 200 entries, and 30 days. Store loss, eviction, or a different store makes a prior hint unusable. [Caveman's recovery documentation](https://github.com/JuliusBrussee/caveman/blob/main/docs/technical/context-recovery.md) likewise says a handle needs the same local store and is not an archive.

## Reproduce

Use reviewed local installations and pass absolute paths. The Caveman CLI argument names its installed `dist/index.js`, and the Node argument names a real executable rather than a version-manager shim. The command uses a new temporary directory for each run; it does not modify global agent or provider configuration.

```sh
python3 scripts/rtk-caveman-runtime.py \
  --rtk /absolute/path/to/rtk \
  --node /absolute/path/to/node \
  --caveman-cli /absolute/path/to/@caveman-ai/cli/dist/index.js \
  --caveman-engine /absolute/path/to/caveman-engine \
  --output docs/internal/rtk-caveman-runtime-results.json
```

The result applies to these synthetic terminal fixtures and local command interfaces. It does not establish proxy routing, real coding-agent behavior, provider input reduction, task quality, or billed savings. The project setup remains independent: agnostic-ai emits configuration, RTK owns command filtering and its recall store, and Caveman owns its runtime transform and recovery store.

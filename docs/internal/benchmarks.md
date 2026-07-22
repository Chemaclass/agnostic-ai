# Benchmarks

A permanent benchmark suite covers the `sync` hot paths. It exists so any
hot-path perf change lands against a measured baseline, per
[`bench-before-perf-refactor`](../../.agnostic-ai/rules/bench-before-perf-refactor.md).
The rule blocks refactoring a hot path without a bench, so this suite is a
prerequisite for the perf work (capture fast-path, parallel emission,
render memoization).

The benchmarks live in `internal/cli/bench_test.go`, colocated so they can
call the unexported sync internals directly. Fixtures use `testing` only,
no external deps.

## Run

```bash
make bench
```

That expands to:

```bash
go test -run '^$' -bench . -benchmem ./...
```

Target one area or size while iterating:

```bash
go test -run '^$' -bench BenchmarkSyncEmit -benchmem ./internal/cli/
go test -run '^$' -bench 'Fingerprint' -benchmem -benchtime=100x ./internal/cli/
```

Benchmarks are not a CI gate. They report numbers for local comparison,
not pass or fail.

## What it covers

| Benchmark | Path |
| --- | --- |
| `BenchmarkSyncEmit` | Emission loop: resolve every target and render the bundle for it in capture mode (no disk). The CPU core of `sync`. |
| `BenchmarkSyncFull` | Full end-to-end `sync`: emit to disk with change detection, distribute entry points, reconcile the ledger, write state. Timed as a steady-state re-sync. |
| `BenchmarkSyncCheck` | The `--check` capture-compare pass against an in-sync tree. |
| `BenchmarkEntryPointRender` | Entry-point body render plus byte-dedupe across targets. |
| `BenchmarkFolderFingerprint` | Shared-skills folder fingerprint (`folderFingerprint`). |

## Read the output

```
BenchmarkSyncEmit/specs=100-14   26   45589017 ns/op   80354676 B/op   317246 allocs/op
```

- `ns/op`: nanoseconds per call. Divide by 1000 for the `μs/call` the rule
  asks for.
- `B/op`, `allocs/op`: bytes and allocations per call, from
  `b.ReportAllocs()`.
- `specs=N`: the spec-count parameter. Each fixture holds N rules, N
  agents, and N skills plus a fixed handful of hooks, MCPs, and commands.
  The suite sweeps 10, 100, and 500. The end-to-end `BenchmarkSyncFull`
  stops at 100 to bound disk churn.

Every benchmark builds its fixture, then calls `b.ResetTimer()` so setup
stays out of the measured region. Fixture content is fixed per index, so
numbers are comparable across runs on the same machine.

## Three subjects

`BenchmarkFolderFingerprint` follows the "three subjects" shape from the
rule so a future proposal has a place to slot in:

- `status-quo`: `folderFingerprint`, the production impl. It streams each
  entry into the digest.
- `baseline`: `naiveFolderFingerprint`, the direct uncached version that
  concatenates every entry into one buffer before hashing. It anchors the
  lower bound.

When you propose a hot-path change, add a third `proposed` sub-benchmark
next to these two, run all three side by side, and compute the crossover
before touching the production code. Keep the bench after the proposal
closes: future runtime or compiler changes can re-measure it cheaply.

# Benchmarks

[Contributor docs](README.md)

Measure sync performance before and after changing a hot path. The permanent benchmark suite provides repeatable baselines for emission, comparison, and rendering.

Most benchmarks live in `internal/cli/bench_test.go`, colocated so they can
call the unexported sync internals directly. `BenchmarkCompareToDisk` lives
in `internal/adapters/internal/emit` beside the compare helper it measures.
Fixtures use `testing` only, no external deps.

## Run

```bash
make bench
```

That expands to:

```bash
go test -timeout 30m -run '^$' -bench . -benchmem ./...
```

At 100 and 500 specs, `BenchmarkCompare`, `BenchmarkGraph`, and
`BenchmarkLint` run once per invocation, and one run can differ from the
next by up to 3x. To compare a change, run each side with `-count=6` and
read the two outputs with `benchstat`.

Target one area or size while iterating:

```bash
go test -run '^$' -bench BenchmarkSyncEmit -benchmem ./internal/cli/
go test -run '^$' -bench 'Fingerprint' -benchmem -benchtime=100x ./internal/cli/
```

Benchmarks are not a CI gate. They report numbers for local comparison,
not pass or fail.

To time the built binary end to end on the same fixture (500 specs by
default), including process startup and git calls the benchmarks skip:

```bash
make bench-commands
scripts/bench-commands.sh --specs 100 --runs 5
```

It prints `<ms>\t<exit>\t<command>` per command, fastest of the runs.

## What it covers

| Benchmark | Path |
| --- | --- |
| `BenchmarkSyncEmit` | Emission loop: resolve every target and render the bundle for it in capture mode (no disk). The CPU core of `sync`. |
| `BenchmarkSyncFull` | Full end-to-end `sync`: emit to disk with change detection, distribute entry points, reconcile the ledger, write state. Timed as a steady-state re-sync. |
| `BenchmarkSyncCheck` | The `--check` capture-compare pass against an in-sync tree. |
| `BenchmarkEntryPointRender` | Entry-point body render plus byte-dedupe across targets. |
| `BenchmarkFolderFingerprint` | Shared-skills folder fingerprint (`folderFingerprint`). |
| `BenchmarkCompare` | `compare claude codex` against a synced tree: every agent, skill, and rule emitted alone to both targets. |
| `BenchmarkGraph` | `graph` edges against a synced tree: every spec emitted alone to every target. |
| `BenchmarkExplain` | `explain` for one rule against a synced tree: every target rendered with and without it. |
| `BenchmarkLint` | The full project `lint` report, the spec-health pass `doctor` also runs. |
| `BenchmarkCompareToDisk` | Capture-compare drift verdict: status-quo full read vs the `CompareToDisk` size-precheck fast path, swept by scenario and file size. |

## Read the output

```
BenchmarkSyncEmit/specs=100-14   26   45589017 ns/op   80354676 B/op   317246 allocs/op
```

- `ns/op`: nanoseconds per call. Divide by 1000 for `μs/call`.
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

`BenchmarkFolderFingerprint` follows the "three subjects" shape so a
future proposal has a place to slot in:

- `status-quo`: `folderFingerprint`, the production impl. It streams each
  entry into the digest.
- `baseline`: `naiveFolderFingerprint`, the direct uncached version that
  concatenates every entry into one buffer before hashing. It anchors the
  lower bound.

When you propose a hot-path change, add a third `proposed` sub-benchmark
next to these two, run all three side by side, and compute the crossover
before touching the production code. Keep the bench after the proposal
closes: future runtime or compiler changes can re-measure it cheaply.

## Profiling a slow sync

The benchmark suite measures the hot paths in-repo. To diagnose a slow
`sync` in the field (a large monorepo, a specific adapter), `sync` has two
opt-in hooks that need no benchmark harness.

`--profile <file>` (or `AGNOSTIC_AI_PROFILE=<file>`) writes a
`runtime/pprof` CPU profile of the whole run. It uses the stdlib profiler
only and is off by default. Read it with the standard tool:

```bash
agnostic-ai sync --profile cpu.prof
go tool pprof -top cpu.prof
go tool pprof -http=:0 cpu.prof   # flame graph in the browser
```

`sync --verbose` appends per-target wall time to each target line, so a
slow run attributes to a specific adapter without a profile:

```
→ claude: 12 created, 3 updated, 0 unchanged in 42ms
→ codex: 40 created, 0 updated, 2 unchanged in 210ms
✓ synced 2 targets · 52 created · 3 updated · 260ms
```

Each per-target time is measured around that target's emit and reported
independently, so under `--jobs > 1` the times overlap. Read them as
per-adapter cost, not a serial breakdown of the run total.

The flag lives on the root command (`internal/cli/root.go`): profiling
starts in the persistent pre-run hook and stops in the persistent post-run
hook, so the profile covers a completed run. The per-target stopwatch wraps
the emit call in `emitTargetsConcurrent` (`internal/cli/sync_run.go`).

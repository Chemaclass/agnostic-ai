# Benchmarks

[Contributor docs](README.md)

Measure sync performance before and after changing code that runs frequently. The permanent benchmark suite provides repeatable before-and-after measurements for writing, comparing, and rendering output.

Most benchmarks live in `internal/cli/bench_test.go`, in the same package so they can
call the unexported sync internals directly. `BenchmarkCompareToDisk` lives
in `internal/adapters/internal/emit` beside the compare helper it measures.
Fixtures use `testing` only, no external dependencies.

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

Benchmarks do not decide whether continuous integration (CI) passes. They report numbers for local comparison,
not pass or fail.

To time the built binary end to end on the same test project (500 specs by
default), including process startup and git calls the benchmarks skip:

```bash
make bench-commands
scripts/bench-commands.sh --specs 100 --runs 5
```

It prints `<ms>\t<exit>\t<command>` per command, fastest of the runs.
The final three rows measure `status`, `doctor`, and `sync --dry-run`
without `.sync-state`, as in a fresh checkout. A tracked hand-written
Claude launch configuration checks whether history identifies generated files without changing
which files count as generated.

## What it covers

| Benchmark | Path |
| --- | --- |
| `BenchmarkSyncEmit` | Select every target and render its specs without writing files. Measures the main computation in `sync`. |
| `BenchmarkSyncFull` | Run `sync` completely: write changed files, distribute entry points, update the generated-file record, and save state. Measures a repeat sync. |
| `BenchmarkSyncCheck` | Run `--check` by rendering and comparing output with files that already match. |
| `BenchmarkEntryPointRender` | Render entry-point content and remove identical copies across targets. |
| `BenchmarkFolderFingerprint` | Calculate the content hash for a shared-skills folder (`folderFingerprint`). |
| `BenchmarkCompare` | `compare claude codex` against a synced tree: every agent, skill, and rule emitted alone to both targets. |
| `BenchmarkGraph` | `graph` edges against a synced tree: every spec emitted alone to every target. |
| `BenchmarkExplain` | `explain` for one rule against a synced tree: every target rendered with and without it. |
| `BenchmarkLint` | The full project `lint` report, the source-spec checks `doctor` also runs. |
| `BenchmarkCompareToDisk` | Compare rendered output with files: the current full read versus checking size first with `CompareToDisk`, across scenarios and file sizes. |

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
  stops at 100 to limit disk writes.

Every benchmark builds its fixture, then calls `b.ResetTimer()` so setup
is excluded from the timing. Fixture content is fixed per index, so
numbers are comparable across runs on the same machine.

## Three subjects

`BenchmarkFolderFingerprint` includes two comparison versions. Add a third when proposing a change:

- `status-quo`: `folderFingerprint`, the current implementation. It feeds each entry into the hash calculation.
- `baseline`: `naiveFolderFingerprint`, the direct uncached version that
  concatenates every entry into one buffer before hashing. It sets the lower bound for comparison.

When you propose a change to frequently run code, add a third `proposed` sub-benchmark
next to these two, run all three side by side, and find when the proposed version becomes faster
before touching the production code. Keep the benchmark after the proposal
closes: future runtime or compiler changes can re-measure it cheaply.

## Profiling a slow sync

The benchmark suite measures frequently run code in this repository. To diagnose a slow `sync` in another project, such as a large repository or a particular adapter, use these two optional tools without setting up benchmarks.

`--profile <file>` (or `AGNOSTIC_AI_PROFILE=<file>`) writes a
`runtime/pprof` CPU profile of the whole run. It uses the Go standard-library profiler
only and is off by default. Read it with the standard tool:

```bash
agnostic-ai sync --profile cpu.prof
go tool pprof -top cpu.prof
go tool pprof -http=:0 cpu.prof   # flame graph in the browser
```

`sync --verbose` appends elapsed time for each target to each target line, so you can identify a slow adapter without a profile:

```
→ claude: 12 created, 3 updated, 0 unchanged in 42ms
→ codex: 40 created, 0 updated, 2 unchanged in 210ms
✓ synced 2 targets · 52 created · 3 updated · 260ms
```

Each per-target time is measured around that target's emit and reported
independently, so under `--jobs > 1` the times overlap. Read them as
time for each adapter, rather than parts that add up to the total run time.

The flag lives on the root command (`internal/cli/root.go`): profiling
starts in the persistent pre-run hook and stops in the persistent post-run
hook, so the profile covers a completed run. The per-target stopwatch wraps
the emit call in `emitTargetsConcurrent` (`internal/cli/sync_run.go`).

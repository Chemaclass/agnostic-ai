# Benchmarks

[Contributor docs](README.md)

Use the benchmark suite to measure writing, comparing, and rendering output before and after changing frequently run code.

Most benchmarks live in `internal/cli/bench_test.go`, in the same package so they can
call the unexported sync internals directly. `BenchmarkCompareToDisk` lives
in `internal/adapters/internal/emit` beside the compare helper it measures.
Test projects (fixtures) use Go's `testing` package only, with no external dependencies.

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

Benchmarks report measurements for local comparison; they do not decide whether continuous integration (CI) passes.

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

## Record and compare built binaries

The existing fastest-run table remains the default. Recording needs Python 3.9 or newer (standard library only), Git, Go, and two built binaries. `specs=500` means 500 rules, 500 agents, and 500 skills, plus five each of hooks, MCP servers, and commands. `--specs 100` uses 100 of each of those first three kinds. Fixture construction still uses `TestWriteBenchFixture`; recording reuses the command list and synced/no-ledger setup from the table. Recording adds `--json` to doctor so its expected diagnostic failure can be checked; the legacy table keeps human doctor output.

Build the baseline from a known fetched main revision before measuring a candidate. Use the same compiler and build flags. Recordings check binary hashes and read Go's embedded compiler version, commit, and dirty flag. An embedded commit must match the supplied full revision. If build metadata lacks a commit, the revision is recorded as operator-supplied; provide `--baseline-dirty clean` or `--candidate-dirty dirty` when known, otherwise it stays `unknown`. A dirty binary records its commit plus its dirty state, not the full producing patch; retain that patch separately.

For example, from the candidate checkout, using new unused paths:

```sh
git fetch origin main
baseline_revision=$(git rev-parse origin/main)
candidate_revision=$(git rev-parse HEAD)
git worktree add --detach /tmp/agnostic-bench-main "$baseline_revision"
(cd /tmp/agnostic-bench-main && go build -trimpath -o /tmp/agnostic-bench-baseline ./cmd/agnostic-ai)
go build -trimpath -o /tmp/agnostic-bench-candidate ./cmd/agnostic-ai
scripts/bench-commands.sh --record /tmp/agnostic-command-recording \
  --baseline /tmp/agnostic-bench-baseline --baseline-revision "$baseline_revision" \
  --candidate /tmp/agnostic-bench-candidate --candidate-revision "$candidate_revision" \
  --machine local-desktop-1 --specs 500 --runs 6
scripts/bench-commands.sh --compare \
  /tmp/agnostic-command-recording/baseline.json \
  /tmp/agnostic-command-recording/candidate.json
```

Supply a stable machine label without private information. The recorder saves only selected OS, architecture, compiler, and supplied machine metadata, not the host environment or credentials. Timed commands and fixture sync use isolated HOME, XDG, Codex, Claude, and agnostic-ai homes, ignore global/system Git configuration, and disable update checks. Before isolation, the recorder resolves the original GOCACHE, GOMODCACHE, and GOPATH. Only the fixture-generator subprocess receives those build paths; measured commands use isolated tool homes, including an existing private CODEX_HOME. Cache paths are not saved in recordings. No coding tool or model is started.

One invocation records both binaries. For each command it runs baseline then candidate once as a warm-up, then alternates baseline and candidate for at least six measured pairs. Setup and restoration stay outside the timer. Both sides use the same active path and restored snapshot; restoration copies changed files only. Each command starts from the synced snapshot or the no-ledger snapshot with its tracked hand-written launch configuration. The retained fixture directory and content hashes identify the input. Each scenario is loaded once into an immutable snapshot of paths, types, permissions, times, file bytes, and link targets. Before each call, restoration scans the active tree, checks every regular file against the cached bytes even when its metadata matches, restores directory permissions, and removes extra paths. Missing or changed files are written and read back to verify them. Successful restoration establishes the snapshot identity after checking all active entries, without a second full tree scan. Git state is restored along with the other files. Hashes cover directory permissions as well as file paths, types, permissions, and content. Both binary hashes are checked again before either recording can be marked complete. Method `alternating-restored-tree-v3` identifies this protocol; older recordings cannot be compared with it. One progress line per command is written to stderr outside timing. Binary timings include process startup and command execution; they exclude fixture construction and restoration. The warm-up timings remain in JSON but do not enter the summary.

`baseline.json` and `candidate.json` retain every measured duration in milliseconds, command arguments, expected and actual exit codes, warm-ups, binary identity, fixture hashes, and environment metadata. Recorded `doctor --json` must exit 1 with a version 1 report, no reported errors or lint errors, and exactly the five untrusted fixture Codex hooks. The synced scenario must report no drift. The no-ledger scenario must report exactly one leftover, `.vscode/settings.json` under target `unledgered`, with zero bytes. Each doctor sample retains its scenario, checked hook count, and expected write records. Configuration/read failures, unknown hook trust, missing JSON, and unexpected hook findings abort. The other commands must exit 0. Unexpected exits, setup errors, and timeouts abort recording. Incomplete JSON records remain for diagnosis and cannot be compared. A failure is never inferred to be expected from its observed exit.

Comparison requires matching schema, measurement method, paired-session identity, environment (including compiler), fixture, command list, warm-up policy, and sample count. It rejects missing or malformed data and unexpected exits. Keep the pair from one recording session together; separate sessions do not prove alternation. A comparison returns success when its inputs are valid, regardless of timing differences.

Each row shows sample count, median, minimum and maximum (the spread), candidate minus baseline in milliseconds, and that difference as a percentage of the baseline median. Positive differences mean more elapsed time. The raw samples remain in the JSON files. A range or slow sample is evidence to investigate, not a confirmed regression. Rerun anomalous observations as another complete paired session and retain both pairs with their exact commands, revisions, build flags, and machine label. These local measurements add no CI latency threshold or performance claim.

The normal shell tests run small synthetic recording controls, not the 500-spec benchmark:

```sh
python3 scripts/bench-recordings_test.py
make test-shell
```

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
  The suite tests sizes of 10, 100, and 500. The end-to-end `BenchmarkSyncFull`
  stops at 100 to limit disk writes.

Every benchmark builds its fixture, then calls `b.ResetTimer()` so setup
is excluded from the timing. Fixture content is fixed per index, so
numbers are comparable across runs on the same machine.

## Three subjects

`BenchmarkFolderFingerprint` includes two comparison versions. Add a third when proposing a change:

- `status-quo`: `folderFingerprint`, the current implementation. It feeds each entry into the hash calculation.
- `baseline`: `naiveFolderFingerprint`, the direct uncached version that
  concatenates every entry into one buffer before hashing. It sets the lower bound for comparison.

Before changing frequently run production code, add a `proposed` sub-benchmark beside these two. Run all three to find when the proposed version becomes faster. Keep the benchmark after the proposal closes so later runtime or compiler changes can be measured against it.

## Profiling a slow sync

Use the benchmark suite to measure frequently run code in this repository. To diagnose a slow `sync` in another project, such as a large repository or a particular adapter, use these two optional tools without setting up benchmarks.

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

Each target's time measures its emit call. With `--jobs > 1`, these times overlap and do not add up to the total run time.

The flag lives on the root command (`internal/cli/root.go`): profiling
starts in the persistent pre-run hook and stops in the persistent post-run
hook, so the profile covers a completed run. The per-target stopwatch wraps
the emit call in `emitTargetsConcurrent` (`internal/cli/sync_run.go`).

## Staged checks in a large repository

`BenchmarkSyncCheckAgainstIndex` measures the production `sync --check --against index` command in process, including the Git commands it starts and work in temporary checkouts. Setup is excluded. Each fixture has exactly 23,000 tracked baseline files, three targets (Claude Code, Codex and Cursor), scoped rules, shared skills and one asset per skill. The 100- and 500-spec cases count rules plus skills, split equally, rather than N copies of each spec kind. Source files span roughly 1,100 package directories so previous-state directory discovery has representative work.

Fixture setup sets repository-local `core.autocrlf=false` and `core.symlinks=true` on every operating system. This lets Git export the shared-skill links as links, including on Windows. Before timing, setup records the Git setting and checks one linked skill's staged mode and target, then uses `checkout-index` outside the fixture to verify Git restores the link and its body/assets. If sync cannot create symlinks, its real-copy fallback remains valid and all controls still run. No global Git setting changes.

Merge the benchmark setup before changing the implementation. Measure the merged current-main revision first, record its commit and Go version, and use the same machine and fixture method for the later change. Keep raw samples and rerun anomalous results. No latency threshold decides whether a run passes.

```sh
go test ./internal/cli -run '^TestStagedCheckBenchmarkControls$' -count=1
go test ./internal/cli -run '^$' -bench '^BenchmarkSyncCheckAgainstIndex$' -benchtime=1x -count=6 -benchmem -timeout 30m
```

Six controls run at both sizes:

| Case | Staged change | Expected check |
| --- | --- | --- |
| clean | None | Pass |
| yaml-comment | Comment added to configuration | Pass, rendered content unchanged |
| semantic-config | Claude rule output directory changed without staging new output | Fail |
| spec-change | One rule changed; correct output exists only in the working tree | Fail |
| deleted-output | One generated rule removed from the index but left on disk | Fail |
| deleted-spec | One source rule removed while its generated output remains tracked | Fail, history-based leftover detection retained |

The small regular test runs these controls without building the large fixture, verifies the expected result, and checks unchanged index and working-tree status. Every expected failure must report output that differs from the specs, including each large-fixture benchmark iteration. Git or filesystem errors fail the benchmark rather than becoming timing samples. Fixture setup verifies that the generated rule used by the deleted-output control is tracked. No agent tools, servers or package installation run.

For built-binary measurements or CPU profiles, export one fixture into a new empty absolute directory. The export test is skipped unless explicitly requested:

```sh
fixture=$(mktemp -d)
AGNOSTIC_AI_STAGED_BENCH_FIXTURE="$fixture" \
AGNOSTIC_AI_STAGED_BENCH_SPECS=100 \
AGNOSTIC_AI_STAGED_BENCH_CASE=yaml-comment \
go test ./internal/cli -run '^TestWriteStagedCheckBenchFixture$' -count=1 -v
```

The exported fixture is retained. Choose `100` or `500` and any case from the table. Its Git history and staging are part of the scenario. The fixture config has no release requirement so comparison binaries use the same inputs. Use isolated temporary HOME, CODEX_HOME and AGNOSTIC_AI_HOME directories for each built-binary comparison to exclude user configuration. Disable update checks with `AGNOSTIC_AI_NO_UPDATE_CHECK=1`. Record binary hashes, producing commits, build flags, compiler versions, exact commands, exit status and stdout/stderr. Alternate the baseline and candidate on this same retained fixture with one warm-up and six measured calls per binary. Time only the calls, not fixture construction.

Profile a separate call on that fixture with the matching built binary:

```sh
(cd "$fixture" && /absolute/path/agnostic-ai sync --check --against index --profile /absolute/path/staged-comment.prof)
go tool pprof -top /absolute/path/agnostic-ai /absolute/path/staged-comment.prof
```

The profile command returns nonzero for expected failure cases. Keep its actual exit status and use profiles outside the fixture; profiling overhead does not belong in latency samples. Compare the unchanged and comment controls before attributing cost to historical rendering. If CPU samples leave most elapsed time unexplained, capture bounded Git subprocess and filesystem stage timings before choosing an optimization. The safe boundaries are `enterAgainstTree`, `sameInputs`, `renderRef`, `checkoutPaths`, recollection and cleanup in `internal/cli/sync_against.go`. Full local and operating-system checks remain required for a later implementation touching these paths.

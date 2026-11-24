# Benchmarks

[Contributor docs](README.md)

Measure writing, comparing, and rendering output before and after changing frequently run code.

Most benchmarks are in `internal/cli/bench_test.go`, where they can call unexported sync functions. Find `BenchmarkImportWalks` in `internal/cli/import_bench_test.go` and `BenchmarkCompareToDisk` in `internal/adapters/internal/emit`. Test projects use only Go's `testing` package.

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

Benchmarks support local comparisons. CI uses separate checks.

To time the built binary end to end on the same test project (500 specs by
default), including process startup and git calls the benchmarks skip:

```bash
make bench-commands
scripts/bench-commands.sh --specs 100 --runs 5
```

It prints `<ms>\t<exit>\t<command>` per command, fastest of the runs.
`import all` runs last among the synced rows because it writes specs; with
`--runs` above 1, the later runs re-import a tree that already matches. The
script uses a separate copy of the project for the rows without `.sync-state`.
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

Supply a stable machine label without private information. The recorder saves only selected OS, architecture, compiler, and supplied machine metadata, not the host environment or credentials. Timed commands and fixture sync use isolated HOME, USERPROFILE, APPDATA, LOCALAPPDATA, XDG, Codex, Claude, and agnostic-ai homes, ignore global/system Git configuration, and disable update checks. Before isolation, the recorder resolves the original GOCACHE, GOMODCACHE, and GOPATH. Only the fixture-generator subprocess receives those build paths; measured commands use isolated tool homes, including an existing private CODEX_HOME. Cache paths are not saved in recordings. No coding tool or model is started.

One invocation records both binaries. For each command it runs baseline then candidate as a warm-up, then alternates them for at least six measured pairs. Timing includes process startup and command execution. Fixture construction, restoration, and the per-command stderr progress line stay outside the timer. Warm-up timings remain in JSON but do not enter the summary.

Both binaries use the same active path. Before each call, the recorder restores either the synced snapshot or the no-ledger snapshot with its tracked hand-written launch configuration. Each scenario is loaded once into an immutable snapshot of paths, types, permissions, times, file bytes, and link targets. Restoration checks every regular file against cached bytes even when metadata matches, restores directory permissions and Git state, removes extra paths, and copies only missing or changed files. It reads written files back to verify them. Checking all active entries establishes the snapshot identity without a second full scan.

The retained fixture directory and hashes identify the input. Hashes cover directory permissions and file paths, types, permissions, and content. Both binary hashes are checked again before recording completes. Method `alternating-restored-tree-v3` identifies this protocol; older recordings cannot be compared with it.

`baseline.json` and `candidate.json` retain every measured duration in milliseconds, command arguments, expected and actual exit codes, warm-ups, binary identity, fixture hashes, and environment metadata.

Recorded `doctor --json` must exit 1 with a version 1 report, no reported errors or lint errors, and exactly five untrusted fixture Codex hooks. The synced scenario must report no drift. The no-ledger scenario must report exactly one zero-byte leftover, `.vscode/settings.json` under target `unledgered`. Each doctor sample retains its scenario, checked hook count, and expected write records. Configuration/read failures, unknown hook trust, missing JSON, and unexpected hook findings abort recording.

Every other command must exit 0. Unexpected exits, setup errors, rejected doctor reports, and timeouts abort recording. Errors include the last lines of command output, except on timeouts. Only the baseline binary syncs the fixture, so candidate output must match it byte for byte. A failed candidate `sync --check` is named in the error with its final output lines. Incomplete JSON records remain for diagnosis and cannot be compared. Expected failures are defined in advance, never inferred from the observed exit.

Comparison requires matching schema, measurement method, paired-session identity, environment (including compiler), fixture, command list, warm-up policy, and sample count. It rejects missing or malformed data and unexpected exits. Keep the pair from one recording session together; separate sessions do not prove alternation. A comparison returns success when its inputs are valid, regardless of timing differences.

Pairs recorded with this method before the `import all` and `explain --context` rows were added still compare with each other.

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
| `BenchmarkSyncOutsideRepository` | `BenchmarkSyncFull` at 10 specs, serial, in a folder outside any Git repository. `status-quo` starts Git for every sync status question; `proposed` skips the questions that must fail. |
| `BenchmarkSyncInsideRepository` | The same sync after `git init` in that folder. Shows that a repository pays nothing extra for the skip. |
| `BenchmarkLayoutAtFresh` | Read the repository layout around a folder (`internal/gitquery`) in a work tree, a bare repository and a folder outside any repository. See [Git outside a repository](#git-outside-a-repository). |
| `BenchmarkSyncCheck` | Run `--check` by rendering and comparing output with files that already match. |
| `BenchmarkLoadProject` | Load the project once: parse the config, resolve the spec layers, and load every spec. One `sync` invocation loads the project once. |
| `BenchmarkLoad` | Load one project config with `config.Load` (`internal/config`), for a base file alone and with `agnostic-ai.local.yaml`: read, shape checks, decode, and every validation. |
| `BenchmarkLoadDecodeOnly` | The lower bound for `BenchmarkLoad`: read the base file and decode it, with no checks. |
| `BenchmarkWatchResync` | One `sync --watch` event for a rule saved without changes, so the pass writes nothing: the current path, which loads the project to pick targets, again in the pass, and parses the config to refresh the watch list, versus one load handed to the pass, and the pass alone over the same targets. |
| `BenchmarkEntryPointRender` | Render entry-point content and remove identical copies across targets. |
| `BenchmarkFolderFingerprint` | Calculate the content hash for a shared-skills folder (`folderFingerprint`). |
| `BenchmarkCompare` | `compare claude codex` against a synced tree: every agent, skill, and rule emitted alone to both targets. |
| `BenchmarkGraph` | `graph` edges against a synced tree: every spec emitted alone to every target. |
| `BenchmarkExplain` | `explain` for one rule against a synced tree: every target rendered with and without it. |
| `BenchmarkExplainContext` | `explain --context` for Claude and Codex, without and with `--file`, against a synced tree: each rule rendered alone to find its output. Baselines are one render of the whole bundle and one render of one rule; `µs/rule` divides the command's time by the rule count. |
| `BenchmarkExplainFile` | `explain --file` for Claude and Codex against a synced tree, with the same baselines as `BenchmarkExplainContext`. |
| `BenchmarkImportWalks` | Import in a project of 1,000 and 8,000 directories: the OpenCode skill import and `import` over codex, cursor, warp, opencode, and factory in one run, against one bare directory walk. `walks/op` counts the whole-project walks one run made. |
| `BenchmarkLint` | The full project `lint` report, the source-spec checks `doctor` also runs. |
| `BenchmarkValidate` | Every issue `validate` reports (`validate`), and the cost of rendering every target, which its case-only collision check adds where letter case does not count (`render`). |
| `BenchmarkCompareToDisk` | Compare rendered output with files: the current full read versus checking size first with `CompareToDisk`, across scenarios and file sizes. |
| `BenchmarkAgentPermission` | Build one Kilo agent permission map: compiling each wildcard pattern on every comparison versus the per-pattern cache, cold and warm, and a matcher with no regular expression, across narrow and wide tool lists with and without project rules. |
| `BenchmarkFrontmatter` | Render one frontmatter block: encoding each value alone versus encoding the whole map once with `FrontmatterStyled`, and plain `yaml.Marshal`, for agent, skill, and styled metadata. |
| `BenchmarkRewriteHookRoot` | Rewrite one hook command below a Git root: finding the root before reading the command versus finding it only when needed with `RewriteHookRoot`, and a root found once, for commands with no reference, a native variable, and a Git root. |
| `BenchmarkNoteDroppedSkillFields` | Check 100 skills for dropped fields with `NoteDroppedSkillFields`, which renders and parses each skill, versus the render alone. |

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

## Git outside a repository

`BenchmarkLayoutAtFresh` in `internal/gitquery/gitquery_bench_test.go` compares three ways to read the repository layout:

- `status-quo`: the earlier read, kept in the test file. It asks for the top level first, so a bare repository or a folder outside any repository fails and starts a second Git process.
- `proposed`: `LayoutAtFresh`, which asks for the top level last. A bare repository still prints the other answers before Git fails, so one process answers every case.
- `baseline`: one `rev-parse` whose output is thrown away, the cost of a single Git process.

Each process costs about 8 to 10 ms on an Apple M4 Pro with Git 2.54.0, so `outside` and `bare` drop by about half and `worktree` stays the same. `BenchmarkSyncOutsideRepository` and `BenchmarkSyncInsideRepository` each run two subjects. `status-quo` turns off the skip in `runGitWithin` (`internal/cli/sync_report.go`), so the sync status helpers start Git as before; `proposed` keeps it on. Neither subject brings back the second `rev-parse` of the earlier layout read; `BenchmarkLayoutAtFresh` measures that. Outside a repository `proposed` gains less than the layout read because the rest of sync dominates; inside a repository the two subjects match.

```bash
go test -run '^$' -bench 'BenchmarkLayoutAtFresh' -benchmem -count=6 ./internal/gitquery
go test -run '^$' -bench 'BenchmarkSync(Outside|Inside)Repository' -benchmem -count=6 ./internal/cli
```

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

`sync -vv` shows elapsed time for every target, so you can identify a slow adapter without a profile. This runs sync again:

```
✓ synced 2 targets · 52 created · 3 updated · 260ms
Target  Created  Updated  Unchanged  Time
claude       12        3          0  42ms
codex        40        0          2  210ms
```

Each target's time measures its emit call. With `--jobs > 1`, these times overlap and do not add up to the total run time.

The flag lives on the root command (`internal/cli/root.go`): profiling
starts in the persistent pre-run hook and stops in the persistent post-run
hook, so the profile covers a completed run. The per-target stopwatch wraps
the emit call in `emitTargetsConcurrent` (`internal/cli/sync_emit.go`).

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

## Which process holds a thread

`BenchmarkHolders` in `internal/conversations/holders_bench_test.go` measures how a scan asks which Codex thread locks and `cursor-agent` chat stores a running process holds. It needs `lsof`, writes a folder of old locks plus 20 recent ones, and holds one old lock open itself:

- `cutoff`: the locks written in the last week, named to `lsof`. It misses the old held lock, so an old open thread showed as closed.
- `per-file`: every lock named to `lsof`, 128 at a time.
- `under-folder`: one `lsof +D` run over the folder, then a check that `lsof` could reach each lock: one `lstat` per lock and one open per folder. This is what the scan uses since [#3527](https://github.com/Chemaclass/agnostic-ai/issues/3527).

Measured 2026-11-17 on an Apple silicon Mac, `-benchtime 5x`, in milliseconds per scan:

| Old locks | cutoff | per-file | under-folder |
|---:|---:|---:|---:|
| 100 | 119 | 133 | 114 |
| 1,000 | 115 | 1,074 | 123 |
| 5,000 | 115 | 5,352 | 172 |

Each `lsof` run costs about 120 ms whatever it is asked, because it reads every process. Naming files only adds runs. One folder read stays near the cost of the old cutoff and still finds a held lock of any age. The reach check alone costs about 6 ms for 5,020 locks.

## Reading and pruning the marks

`BenchmarkDecodeMarks` in `internal/conversations/prune_bench_test.go` reads a marks file of 2,000 entries, as every scan does:

- `status-quo`: a plain decode that drops the fields it does not know.
- `proposed`: one strict decode that fails on an unknown field, and only then a second pass that keeps them. This is what the scan uses since [#3540](https://github.com/Chemaclass/agnostic-ai/issues/3540).
- `baseline`: a check that the bytes are JSON, with no decode.

`BenchmarkPruneMarks` is the pass over the same 2,000 marks once a store was read in full, when nothing changes.

Measured 2026-11-17 on an Apple M4 Pro, Go 1.27.4, `-count 6 -cpu 4`, on a busy machine, in milliseconds per read:

| status-quo | proposed | baseline | prune pass |
|---:|---:|---:|---:|
| 1.23 | 1.56 | 0.24 | 0.23 |

The prune pass reads the marks twice: once to find the stores whose missing marks are due a save, so they are saved in one write, and once to age them. The strict pass adds about 0.3 ms a scan at 2,000 marks, against about 120 ms for one `lsof` run in the same scan. A first try that decoded every entry twice took 3.5 ms.

## Listing Claude Code transcripts

`BenchmarkListTranscripts` in the same file lists 5,000 transcripts in 100 project folders, each session with its own folder beside it, as every Claude Code scan does:

- `status-quo`: `filepath.Glob(projects/*/*.jsonl)`, which skips a folder it cannot open without saying so.
- `shipped`: `listTree`, which also says whether every folder was read, so a scan can prove it listed the whole store. It compares a name with a plain suffix or equality check and joins a path only for a match.
- `baseline`: a bare `os.ReadDir` walk with a suffix check and no error checks.

Measured the same day, `-count 6 -cpu 4`, in milliseconds per scan:

| status-quo | shipped | baseline |
|---:|---:|---:|
| 11.5 | 7.7 | 7.4 |

A first `listTree` that ran `filepath.Match` on every name and joined every path took 11.6 ms. `cursor-agent` chat stores use the same walk.

## Stamping the stores for a search

`BenchmarkSearchFirstPage_CursorAgentChats` in `internal/conversations/cursor_agent_bench_test.go` times one first-page search request over 2,000 `cursor-agent` chats, on a scanner that already read them once:

- `status_quo`: `Stamp` for the list tag, then `Scan`, which globs and stats every store file again for its search key.
- `proposed`: `TakeStamp` for the list tag, then `ScanStamped`, which keys the search on that stamp. This is what the server uses since [#3623](https://github.com/Chemaclass/agnostic-ai/issues/3623).
- `direct`: `Scan` alone with no tag, the floor of any stamp sharing.

Measured 2026-11-19 on an Apple M4 Pro, `-benchtime 20x -count 6`, on a busy machine, in milliseconds per request:

| status_quo | proposed | direct |
|---:|---:|---:|
| 380 | 311 | 303 |

The search still stamps the store files once after its read, to catch a write made during it. A new scanner per request reads every store cold, about 14 seconds here, which `BenchmarkScan_CursorAgentChats/uncached` already times.

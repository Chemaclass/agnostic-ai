package cli

// Permanent benchmark suite for the sync hot paths and the commands
// that emit per spec. See docs/internal/benchmarks.md for how to run and
// read these, and for the "three subjects" shape a hot-path perf change
// measures against before it lands.
//
// Covered paths:
//   - BenchmarkSyncEmit:          the emission loop over N specs x every
//     target (capture mode, no disk), the core of `sync`.
//   - BenchmarkSyncFull:          the full end-to-end `sync` including
//     disk compare, ledger, and state write (steady-state re-sync).
//   - BenchmarkSyncOutsideRepository: BenchmarkSyncFull at specs=10,
//     serial, in a folder outside any Git repository, status-quo
//     (Git started for every question) and proposed (skipped).
//   - BenchmarkSyncInsideRepository:  the same after git init.
//   - BenchmarkSyncCheck:         the `--check` capture-compare pass.
//   - BenchmarkLoadProject:       one config and spec load, which a
//     `sync` invocation pays once.
//   - BenchmarkEntryPointRender:  the entry-point body render plus
//     byte-dedupe across targets.
//   - BenchmarkFolderFingerprint: the shared-skills folder fingerprint,
//     shaped as "three subjects" (status-quo + naive baseline) so a
//     future proposal slots a third subject in.
//   - BenchmarkCompare:           `compare claude codex`, per-spec emits.
//   - BenchmarkGraph:             `graph` edges, per-spec emits.
//   - BenchmarkExplain:           `explain` for one rule, two emits per target.
//   - BenchmarkExplainContext:    `explain --context`, one emit per rule.
//   - BenchmarkExplainFile:       `explain --file`, one emit per rule.
//   - BenchmarkLint:              the full project `lint` report.
//   - BenchmarkWatchResync:       one `sync --watch` event, three subjects.
//
// Fixtures are deterministic (content fixed per index) so numbers stay
// comparable across runs. Every benchmark calls b.ReportAllocs() and
// resets the timer after fixture setup.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/gitquery"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// benchSpecCounts parameterizes the spec-scaling benchmarks. Each count
// is the number of rules, agents, and skills the fixture holds (plus a
// small fixed set of hooks, MCPs, and commands).
var benchSpecCounts = []int{10, 100, 500}

// benchFingerprintFileCounts parameterizes the fingerprint benchmark by
// the number of files in one skill folder.
var benchFingerprintFileCounts = []int{8, 64, 512}

// benchFixedSpecs is the fixed count of hooks, MCPs, and commands every
// fixture carries regardless of the spec-count parameter. They exercise
// the pure-YAML and command loaders without dominating setup time.
const benchFixedSpecs = 5

// Sinks defeat dead-code elimination of benchmarked calls whose results
// are otherwise unused. The int/string sinks avoid the interface boxing
// an `any` sink would add to the per-op allocation count.
var (
	benchIntSink    int
	benchStringSink string
)

// BenchmarkSyncEmit measures the emission loop: resolve every target and
// render the whole bundle for it, in capture mode so no bytes touch disk.
// This is the CPU core of `sync` (sync_emit.go emitTarget loop) and the
// subject a future parallel-emission change would optimize.
func BenchmarkSyncEmit(b *testing.B) {
	for _, n := range benchSpecCounts {
		b.Run(fmt.Sprintf("specs=%d", n), func(b *testing.B) {
			root := benchProject(b, n)
			cfg, bundle := benchLoad(b, root)
			resetBenchBuffers()
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				out, err := captureRenders(cfg, newTargetRenders(cfg, bundle), cfg.Targets)
				if err != nil {
					b.Fatal(err)
				}
				benchIntSink = len(out)
			}
		})
	}
}

// BenchmarkSyncFull measures a full end-to-end `sync` re-run: load, emit
// every target to disk with change detection, distribute entry points,
// reconcile the ledger, and write state. The fixture is primed with one
// sync before timing so the measured iterations are the steady-state
// re-sync (create-then-skip), the common dev and CI path.
//
// Each spec count runs twice, serial (--jobs 1) and parallel (--jobs 0,
// one worker per CPU), so the two subjects sit side by side and the
// parallel-emission crossover (the spec count where fan-out beats
// serial) is read straight off the numbers.
func BenchmarkSyncFull(b *testing.B) {
	jobsCases := []struct {
		name string
		jobs int
	}{
		{"serial", 1},   // status-quo: today's sequential emission
		{"parallel", 0}, // proposed: bounded fan-out, one worker per CPU
	}
	for _, n := range []int{10, 100} {
		for _, jc := range jobsCases {
			b.Run(fmt.Sprintf("specs=%d/%s", n, jc.name), func(b *testing.B) {
				root := benchProject(b, n)
				benchQuiet(b)
				benchChdir(b, root)
				// Prime once so timed iterations measure the re-sync path.
				if err := runSyncOnce(".", nil, false, false, "off", jc.jobs); err != nil {
					b.Fatal(err)
				}
				resetBenchBuffers()
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					if err := runSyncOnce(".", nil, false, false, "off", jc.jobs); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

// BenchmarkSyncOutsideRepository is BenchmarkSyncFull at specs=10, serial,
// in a folder outside any Git repository, where every Git question fails.
// status-quo turns off the skip in runGitWithin, so the sync helpers start
// Git as they did before; proposed keeps it on.
func BenchmarkSyncOutsideRepository(b *testing.B) {
	benchSyncGitSubjects(b, false)
}

// BenchmarkSyncInsideRepository is BenchmarkSyncOutsideRepository after
// git init in the project folder: a repository must pay nothing extra.
func BenchmarkSyncInsideRepository(b *testing.B) {
	benchSyncGitSubjects(b, true)
}

func benchSyncGitSubjects(b *testing.B, initRepository bool) {
	b.Helper()
	for _, subject := range []struct {
		name string
		skip bool
	}{
		{"status-quo", false},
		{"proposed", true},
	} {
		b.Run(subject.name, func(b *testing.B) {
			prev := skipGitOutsideRepository
			skipGitOutsideRepository = subject.skip
			b.Cleanup(func() { skipGitOutsideRepository = prev })
			benchSyncGitRepository(b, initRepository)
		})
	}
}

func benchSyncGitRepository(b *testing.B, initRepository bool) {
	b.Helper()
	root := benchProject(b, 10)
	b.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(root))
	if initRepository {
		if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
			b.Fatalf("git init: %v: %s", err, out)
		}
	}
	benchQuiet(b)
	benchChdir(b, root)
	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		b.Fatal(err)
	}
	resetBenchBuffers()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSyncCheck measures the `--check` capture-compare pass: render
// every target in capture mode and diff each would-be file against disk.
// The fixture is synced first so the compare hits the in-sync path (the
// green-CI case) rather than reporting everything missing.
func BenchmarkSyncCheck(b *testing.B) {
	for _, n := range benchSpecCounts {
		b.Run(fmt.Sprintf("specs=%d", n), func(b *testing.B) {
			root := benchProject(b, n)
			benchQuiet(b)
			benchChdir(b, root)
			// Prime disk via the check path's own writers so the timed
			// compare finds every file in sync.
			reports, err := collectDrift(nil)
			if err != nil {
				b.Fatal(err)
			}
			if _, err := fixDrift(reports, false); err != nil {
				b.Fatal(err)
			}
			resetBenchBuffers()
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				out, err := collectDrift(nil)
				if err != nil {
					b.Fatal(err)
				}
				benchIntSink = len(out)
			}
		})
	}
}

// BenchmarkLoadProject measures one project load: parse the config,
// resolve the spec layers, and load every spec. Each extra load in a
// `sync` invocation costs this much.
func BenchmarkLoadProject(b *testing.B) {
	for _, n := range benchSpecCounts {
		b.Run(fmt.Sprintf("specs=%d", n), func(b *testing.B) {
			root := benchProject(b, n)
			benchQuiet(b)
			benchChdir(b, root)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				_, bundle, err := loadProject(".")
				if err != nil {
					b.Fatal(err)
				}
				benchIntSink = len(bundle.All())
			}
		})
	}
}

// BenchmarkEntryPointRender measures the entry-point render: build each
// target's pointer body, apply rule appendices and import modes, and
// dedupe byte-identical files across targets by path.
func BenchmarkEntryPointRender(b *testing.B) {
	for _, n := range benchSpecCounts {
		b.Run(fmt.Sprintf("specs=%d", n), func(b *testing.B) {
			root := benchProject(b, n)
			cfg, bundle := benchLoad(b, root)
			body := adapters.EntryPointBody()
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				out, err := renderEntryPointFiles(cfg, bundle, cfg.Targets, body)
				if err != nil {
					b.Fatal(err)
				}
				benchIntSink = len(out)
			}
		})
	}
}

// BenchmarkCompare measures `compare claude codex`: every agent, skill,
// and rule emitted alone to both targets. The fixture is synced first
// because each per-spec emit still reads the output tree on disk.
func BenchmarkCompare(b *testing.B) {
	for _, n := range benchSpecCounts {
		b.Run(fmt.Sprintf("specs=%d", n), func(b *testing.B) {
			cfg, bundle := benchSyncedProject(b, n)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				out, err := compareTargets(cfg, bundle, []string{"claude", "codex"})
				if err != nil {
					b.Fatal(err)
				}
				benchIntSink = len(out.Specs)
			}
		})
	}
}

// BenchmarkGraph measures the `graph` edge computation: every spec
// emitted alone to every target, against a synced output tree.
func BenchmarkGraph(b *testing.B) {
	for _, n := range benchSpecCounts {
		b.Run(fmt.Sprintf("specs=%d", n), func(b *testing.B) {
			cfg, bundle := benchSyncedProject(b, n)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				edges, err := computeGraphEdges(bundle, cfg, graphFilter{})
				if err != nil {
					b.Fatal(err)
				}
				benchIntSink = len(edges)
			}
		})
	}
}

// BenchmarkExplain measures `explain` for one rule: every adapter
// rendered with and without it, against a synced output tree.
func BenchmarkExplain(b *testing.B) {
	for _, n := range benchSpecCounts {
		b.Run(fmt.Sprintf("specs=%d", n), func(b *testing.B) {
			cfg, bundle := benchSyncedProject(b, n)
			rule := bundle.Rules[0]
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				configured, extra, err := computeContributions(rule, bundle, cfg)
				if err != nil {
					b.Fatal(err)
				}
				benchIntSink = len(configured) + len(extra)
			}
		})
	}
}

// benchExplainTargets are the targets whose `explain --context` and
// `explain --file` render every rule alone to find its output.
var benchExplainTargets = []string{"claude", "codex"}

// benchExplainFile is the project file the --file benchmarks ask about.
const benchExplainFile = "README.md"

// BenchmarkExplainContext measures `explain --context`, with and without
// --file. It follows the "three subjects" shape: status-quo is the
// command, baseline/full-render is one render of the whole bundle (the
// floor), and baseline/single-rule is one render of one rule, the unit
// the command repeats per rule. µs/rule divides the command's time by
// the rule count.
func BenchmarkExplainContext(b *testing.B) {
	for _, n := range benchSpecCounts {
		for _, target := range benchExplainTargets {
			b.Run(fmt.Sprintf("status-quo/context/%s/specs=%d", target, n), func(b *testing.B) {
				cfg, bundle := benchSyncedProject(b, n)
				benchExplainLoop(b, bundle, target, func() int {
					out, err := explainContext(cfg, bundle, target, "")
					if err != nil {
						b.Fatal(err)
					}
					return len(out.Contributions)
				})
			})
			b.Run(fmt.Sprintf("status-quo/context-file/%s/specs=%d", target, n), func(b *testing.B) {
				cfg, bundle := benchSyncedProject(b, n)
				writeBenchFile(b, benchExplainFile, "# Project\n")
				benchExplainLoop(b, bundle, target, func() int {
					out, err := explainContext(cfg, bundle, target, benchExplainFile)
					if err != nil {
						b.Fatal(err)
					}
					return len(out.Contributions)
				})
			})
			benchExplainBaselines(b, n, target)
		}
	}
}

// BenchmarkExplainFile measures `explain --file`, which renders every
// rule alone for Claude and Codex. The baselines are the same as
// BenchmarkExplainContext's.
func BenchmarkExplainFile(b *testing.B) {
	for _, n := range benchSpecCounts {
		for _, target := range benchExplainTargets {
			b.Run(fmt.Sprintf("status-quo/file/%s/specs=%d", target, n), func(b *testing.B) {
				cfg, bundle := benchSyncedProject(b, n)
				writeBenchFile(b, benchExplainFile, "# Project\n")
				benchExplainLoop(b, bundle, target, func() int {
					out, err := explainFile(benchExplainFile, target, cfg, bundle, ".")
					if err != nil {
						b.Fatal(err)
					}
					return len(out.Instructions)
				})
			})
			benchExplainBaselines(b, n, target)
		}
	}
}

// benchExplainBaselines adds the full-render and single-rule floors for
// target at n specs.
func benchExplainBaselines(b *testing.B, n int, target string) {
	b.Helper()
	b.Run(fmt.Sprintf("baseline/full-render/%s/specs=%d", target, n), func(b *testing.B) {
		cfg, bundle := benchSyncedProject(b, n)
		adapter := benchAdapter(b, target)
		benchExplainLoop(b, bundle, target, func() int {
			files, err := captureEmit(adapter, bundle, cfg)
			if err != nil {
				b.Fatal(err)
			}
			return len(files)
		})
	})
	b.Run(fmt.Sprintf("baseline/single-rule/%s/specs=%d", target, n), func(b *testing.B) {
		cfg, bundle := benchSyncedProject(b, n)
		adapter := benchAdapter(b, target)
		one := singleEntryBundle(bundle.For(target).Rules[0])
		defer holdStateFile(".")()
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			files, err := captureEmit(adapter, one, cfg)
			if err != nil {
				b.Fatal(err)
			}
			benchIntSink = len(files)
		}
	})
}

// benchExplainLoop times op b.N times and reports its cost per rule the
// target renders. It holds the state file as the explain commands do.
func benchExplainLoop(b *testing.B, bundle spec.Bundle, target string, op func() int) {
	b.Helper()
	rules := len(bundle.For(target).Rules)
	defer holdStateFile(".")()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		benchIntSink = op()
	}
	b.StopTimer()
	b.ReportMetric(float64(b.Elapsed().Microseconds())/float64(b.N)/float64(rules), "µs/rule")
}

// benchAdapter resolves target's adapter for a benchmark.
func benchAdapter(b *testing.B, target string) adapters.Adapter {
	b.Helper()
	adapter, err := adapters.Resolve(target)
	if err != nil {
		b.Fatal(err)
	}
	return adapter
}

// BenchmarkLint measures the full project `lint` report, the same pass
// `doctor` runs for spec health.
func BenchmarkLint(b *testing.B) {
	for _, n := range benchSpecCounts {
		b.Run(fmt.Sprintf("specs=%d", n), func(b *testing.B) {
			benchSyncedProject(b, n)
			scope, err := loadCheckScope(false)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				findings, _, err := lintScopeReport(scope)
				if err != nil {
					b.Fatal(err)
				}
				benchIntSink = len(findings)
			}
		})
	}
}

// BenchmarkWatchResync measures one `sync --watch` event for a rule saved
// without changes, so the pass writes nothing and the loads weigh the most.
// status-quo is what the loop runs today: resyncForChanges loads the
// project to pick the affected targets, the pass loads it again, and the
// loop parses the config once more to refresh the watch list. proposed
// loads the project once, hands it to the pass, and refreshes the watch
// list from that same config. baseline is
// the pass alone over the same targets, the lower bound any reuse of the
// loaded project can reach.
func BenchmarkWatchResync(b *testing.B) {
	opts := watchOptions{gitignoreFlag: "off", jobs: 1}
	for _, n := range benchSpecCounts {
		b.Run(fmt.Sprintf("specs=%d", n), func(b *testing.B) {
			benchSyncedProject(b, n)
			changed := []string{filepath.Join(config.SourceBaseDir, "rules", "rule-0000.md")}
			cfg, bundle, err := loadProject(".")
			if err != nil {
				b.Fatal(err)
			}
			plan := planWatchResync(".", cfg, bundle, changed, cfg.Targets)
			if plan.full || len(plan.targets) == 0 {
				b.Fatalf("a rule save should re-sync a target subset, got %+v", plan)
			}
			b.Run("status-quo", func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					if err := resyncForChanges(".", nil, changed, opts); err != nil {
						b.Fatal(err)
					}
					newCfg, err := config.Load(".")
					if err != nil {
						b.Fatal(err)
					}
					benchIntSink = len(watchDirs(".", newCfg))
				}
			})
			b.Run("proposed", func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					cfg, bundle, err := loadProject(".")
					if err != nil {
						b.Fatal(err)
					}
					targets := planWatchResync(".", cfg, bundle, changed, cfg.Targets).targets
					gitquery.Reset()
					pass := syncRunOptions{gitignoreFlag: opts.gitignoreFlag, jobs: opts.jobs}
					if err := runSyncPassWith(".", readLedger("."), &projectLoad{cfg: cfg, bundle: bundle}, targets, pass); err != nil {
						b.Fatal(err)
					}
					benchIntSink = len(watchDirs(".", cfg))
				}
			})
			b.Run("baseline", func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					if err := opts.sync(".", plan.targets); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}

// benchSyncedProject builds the fixture, makes it the working directory,
// and syncs it once, for commands whose cost depends on the output tree.
func benchSyncedProject(b *testing.B, n int) (*config.Config, spec.Bundle) {
	b.Helper()
	root := benchProject(b, n)
	benchQuiet(b)
	benchChdir(b, root)
	if err := runSyncOnce(".", nil, false, false, "off", 0); err != nil {
		b.Fatal(err)
	}
	resetBenchBuffers()
	return benchLoad(b, root)
}

// TestWriteBenchFixture writes the benchmark fixture to
// $AGNOSTIC_AI_BENCH_FIXTURE with $AGNOSTIC_AI_BENCH_SPECS specs (default
// 500) for scripts/bench-commands.sh. It skips when the variable is unset.
func TestWriteBenchFixture(t *testing.T) {
	root := os.Getenv("AGNOSTIC_AI_BENCH_FIXTURE")
	if root == "" {
		t.Skip("AGNOSTIC_AI_BENCH_FIXTURE not set")
	}
	if !filepath.IsAbs(root) {
		t.Fatalf("AGNOSTIC_AI_BENCH_FIXTURE=%q: want an absolute path outside the source tree", root)
	}
	n := 500
	if v := os.Getenv("AGNOSTIC_AI_BENCH_SPECS"); v != "" {
		var err error
		if n, err = strconv.Atoi(v); err != nil || n < 1 {
			t.Fatalf("AGNOSTIC_AI_BENCH_SPECS=%q: want a positive integer", v)
		}
	}
	writeBenchProject(t, root, n)
}

// BenchmarkFolderFingerprint measures the shared-skills folder
// fingerprint. It follows the "three subjects" shape from
// docs/internal/benchmarks.md:
//
//   - status-quo: folderFingerprint, which streams each entry into the
//     digest with no intermediate buffer.
//   - baseline:   naiveFolderFingerprint, the direct/uncached version
//     that concatenates every entry into one buffer before hashing.
//
// A future proposal adds a third "proposed" subject and compares all
// three side by side before changing the production impl.
func BenchmarkFolderFingerprint(b *testing.B) {
	for _, n := range benchFingerprintFileCounts {
		files := benchSkillFolderFiles(n)
		b.Run(fmt.Sprintf("status-quo/files=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				benchStringSink = folderFingerprint(files)
			}
		})
		b.Run(fmt.Sprintf("baseline/files=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				benchStringSink = naiveFolderFingerprint(files)
			}
		})
	}
}

// naiveFolderFingerprint is the direct/baseline subject for
// BenchmarkFolderFingerprint. It builds the full sorted concatenation in
// one buffer, then hashes it in a single pass. It computes the same digest
// as the streaming folderFingerprint but by way of a full intermediate
// buffer, so the two differ only in cost profile. Not used in production.
func naiveFolderFingerprint(files map[string]string) string {
	rels := make([]string, 0, len(files))
	for rel := range files {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	var sb strings.Builder
	for _, rel := range rels {
		sb.WriteString(rel)
		sb.WriteByte(0)
		sb.WriteString(files[rel])
		sb.WriteByte(0)
	}
	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}

// --- fixture helpers ---

// benchProject writes a deterministic project fixture under a fresh temp
// dir and returns its root. It holds n rules, n agents, and n skills plus
// benchFixedSpecs hooks, MCPs, and commands. Every spec's content is fixed
// by index so bench numbers stay comparable across runs.
func benchProject(b *testing.B, n int) string {
	b.Helper()
	root := b.TempDir()
	writeBenchProject(b, root, n)
	return root
}

// writeBenchProject writes the benchProject fixture under root.
// TestWriteBenchFixture reuses it so the end-to-end timing script
// measures the same project shape as the benchmarks.
func writeBenchProject(tb testing.TB, root string, n int) {
	tb.Helper()
	writeBenchFile(tb, filepath.Join(root, "agnostic-ai.yaml"), benchConfigYAML())
	base := filepath.Join(root, config.SourceBaseDir)
	for i := range n {
		writeBenchFile(tb, filepath.Join(base, "rules", fmt.Sprintf("rule-%04d.md", i)), benchRule(i))
		writeBenchFile(tb, filepath.Join(base, "agents", fixtureAgentName(i)+".md"), benchAgent(i))
		writeBenchFile(tb, filepath.Join(base, "skills", fmt.Sprintf("skill-%04d", i), "SKILL.md"), benchSkill(i))
	}
	for i := range benchFixedSpecs {
		writeBenchFile(tb, filepath.Join(base, "hooks", fmt.Sprintf("hook-%02d.yaml", i)), benchHook(i))
		writeBenchFile(tb, filepath.Join(base, "mcps", fmt.Sprintf("mcp-%02d.yaml", i)), benchMCP(i))
		writeBenchFile(tb, filepath.Join(base, "commands", fmt.Sprintf("command-%02d.md", i)), benchCommand(i))
	}
}

// benchLoad loads the fixture config and a project-only bundle. It skips
// resolveLayers so the project-user layer cannot leak host state into the
// numbers.
func benchLoad(b *testing.B, root string) (*config.Config, spec.Bundle) {
	b.Helper()
	cfg, err := config.Load(root)
	if err != nil {
		b.Fatalf("load config: %v", err)
	}
	bundle, err := spec.LoadLayered([]spec.Layer{{
		Name:    "project",
		Root:    root,
		Sources: cfg.Sources,
	}})
	if err != nil {
		b.Fatalf("load bundle: %v", err)
	}
	return cfg, bundle
}

// benchConfigYAML renders a config that enables every registered target
// and sets prefer-spec so the collision pre-flight is a no-op. The full
// target set makes the emission loop fan out the way a maximal project
// would.
func benchConfigYAML() string {
	names := adapters.Names()
	sort.Strings(names)
	var sb strings.Builder
	sb.WriteString("version: 1\n")
	sb.WriteString("sync:\n  collision-policy: prefer-spec\n")
	sb.WriteString("targets:\n")
	for _, n := range names {
		sb.WriteString("  - " + n + "\n")
	}
	return sb.String()
}

func benchRule(i int) string {
	return fmt.Sprintf(`---
name: rule-%04d
description: Bench rule %d for deterministic fixtures.
globs: "**/*.go"
alwaysApply: true
---

Rule %d body. Wrap errors with context so the failing path is obvious.
Prefer stdlib. Keep one idea per sentence. Fixtures stay byte-stable.
`, i, i, i)
}

func fixtureAgentName(i int) string {
	name := ""
	for {
		name = string(rune('a'+i%26)) + name
		i = i/26 - 1
		if i < 0 {
			return "agent-" + name
		}
	}
}

func benchAgent(i int) string {
	return fmt.Sprintf(`---
name: %s
description: Bench agent %d for deterministic fixtures.
tools: [Read, Grep, Bash]
model: sonnet
---

Agent %d instructions. Review the diff. Report path:line problem then fix.
`, fixtureAgentName(i), i, i)
}

func benchSkill(i int) string {
	return fmt.Sprintf(`---
name: skill-%04d
description: Bench skill %d for deterministic fixtures.
---

# skill-%04d

Steps: read the target, parse it, report violations. Deterministic body.
`, i, i, i)
}

func benchHook(i int) string {
	return fmt.Sprintf(`name: hook-%02d
description: Bench hook %d.
event: PostToolUse
matcher: "Edit|Write"
command: "echo hook-%02d"
`, i, i, i)
}

func benchMCP(i int) string {
	return fmt.Sprintf(`name: mcp-%02d
description: Bench MCP %d.
command: npx
args:
  - -y
  - "@modelcontextprotocol/server-filesystem"
  - "."
`, i, i)
}

func benchCommand(i int) string {
	return fmt.Sprintf(`---
name: command-%02d
description: Bench command %d.
---

Command %d body. Deterministic content for stable numbers.
`, i, i, i)
}

// benchSkillFolderFiles builds a deterministic rel-path -> content map
// standing in for one rendered skill folder: a SKILL.md plus n-1 sibling
// assets. Content is fixed by index so the fingerprint is stable.
func benchSkillFolderFiles(n int) map[string]string {
	files := make(map[string]string, n)
	files["SKILL.md"] = benchSkill(0)
	for i := 1; i < n; i++ {
		files[fmt.Sprintf("assets/file-%04d.md", i)] = fmt.Sprintf(
			"# asset %d\n\nDeterministic asset body for the fingerprint bench.\n", i)
	}
	return files
}

func writeBenchFile(tb testing.TB, path, content string) {
	tb.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		tb.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		tb.Fatalf("write %s: %v", path, err)
	}
}

// benchChdir switches into dir for the benchmark and restores the prior
// working directory on cleanup. The disk-backed sync paths resolve output
// paths relative to the working directory.
func benchChdir(b *testing.B, dir string) {
	b.Helper()
	b.Chdir(dir)
}

// benchQuiet silences the CLI log sink and capability warner so benchmark
// output stays clean, restoring both on cleanup.
func benchQuiet(b *testing.B) {
	b.Helper()
	prevVerbosity, prevOut := verbosity, logOut
	verbosity = levelQuiet
	logOut = io.Discard
	adapters.SetWarner(io.Discard)
	b.Cleanup(func() {
		verbosity = prevVerbosity
		logOut = prevOut
		adapters.SetWarner(os.Stderr)
	})
}

// resetBenchBuffers clears the process-global capability-warning and
// coverage-note buffers so repeated emit passes do not accumulate across
// iterations or benchmarks.
func resetBenchBuffers() {
	adapters.ResetCapabilityWarnings()
	adapters.ResetCoverageNotes()
}

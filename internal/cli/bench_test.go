package cli

// Permanent benchmark suite for the sync hot paths. See
// docs/internal/benchmarks.md for how to run and read these, and
// .agnostic-ai/rules/bench-before-perf-refactor.md for the policy that
// makes them a prerequisite for any hot-path perf change.
//
// Covered paths:
//   - BenchmarkSyncEmit:          the emission loop over N specs x every
//     target (capture mode, no disk), the core of `sync`.
//   - BenchmarkSyncFull:          the full end-to-end `sync` including
//     disk compare, ledger, and state write (steady-state re-sync).
//   - BenchmarkSyncCheck:         the `--check` capture-compare pass.
//   - BenchmarkEntryPointRender:  the entry-point body render plus
//     byte-dedupe across targets.
//   - BenchmarkFolderFingerprint: the shared-skills folder fingerprint,
//     shaped as the rule's "three subjects" (status-quo + naive
//     baseline) so a future proposal slots a third subject in.
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
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
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
// This is the CPU core of `sync` (sync_run.go emitTarget loop) and the
// subject a future parallel-emission change would optimize.
func BenchmarkSyncEmit(b *testing.B) {
	for _, n := range benchSpecCounts {
		b.Run(fmt.Sprintf("specs=%d", n), func(b *testing.B) {
			root := benchProject(b, n)
			cfg, bundle := benchLoad(b, root)
			resetBenchBuffers()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				out, err := captureRenders(cfg, bundle, cfg.Targets)
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
// Each spec count runs twice — serial (--jobs 1) and parallel (--jobs 0 =
// one worker per CPU) — so the two subjects sit side by side per
// bench-before-perf-refactor.md and the parallel-emission crossover
// (the spec count where fan-out beats serial) is read straight off the
// numbers.
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
				benchIsolateGlobalLayer(b)
				// Prime once so timed iterations measure the re-sync path.
				if err := runSyncOnce(".", nil, false, false, "off", jc.jobs); err != nil {
					b.Fatal(err)
				}
				resetBenchBuffers()
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if err := runSyncOnce(".", nil, false, false, "off", jc.jobs); err != nil {
						b.Fatal(err)
					}
				}
			})
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
			benchIsolateGlobalLayer(b)
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
			for i := 0; i < b.N; i++ {
				out, err := collectDrift(nil)
				if err != nil {
					b.Fatal(err)
				}
				benchIntSink = len(out)
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
			body := adapters.EntryPointBody(cfg)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				out, err := renderEntryPointFiles(cfg, bundle, cfg.Targets, body)
				if err != nil {
					b.Fatal(err)
				}
				benchIntSink = len(out)
			}
		})
	}
}

// BenchmarkFolderFingerprint measures the shared-skills folder
// fingerprint. It follows the "three subjects" shape from
// bench-before-perf-refactor.md:
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
			for i := 0; i < b.N; i++ {
				benchStringSink = folderFingerprint(files)
			}
		})
		b.Run(fmt.Sprintf("baseline/files=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
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
	writeBenchFile(b, filepath.Join(root, "agnostic-ai.yaml"), benchConfigYAML())
	base := filepath.Join(root, config.SourceBaseDir)
	for i := 0; i < n; i++ {
		writeBenchFile(b, filepath.Join(base, "rules", fmt.Sprintf("rule-%04d.md", i)), benchRule(i))
		writeBenchFile(b, filepath.Join(base, "agents", fmt.Sprintf("agent-%04d.md", i)), benchAgent(i))
		writeBenchFile(b, filepath.Join(base, "skills", fmt.Sprintf("skill-%04d", i), "SKILL.md"), benchSkill(i))
	}
	for i := 0; i < benchFixedSpecs; i++ {
		writeBenchFile(b, filepath.Join(base, "hooks", fmt.Sprintf("hook-%02d.yaml", i)), benchHook(i))
		writeBenchFile(b, filepath.Join(base, "mcps", fmt.Sprintf("mcp-%02d.yaml", i)), benchMCP(i))
		writeBenchFile(b, filepath.Join(base, "commands", fmt.Sprintf("command-%02d.md", i)), benchCommand(i))
	}
	return root
}

// benchLoad loads the fixture config and a project-only bundle. It skips
// resolveLayers on purpose so the user-global and project-user layers
// never leak host state into the numbers; benchIsolateGlobalLayer covers
// the paths that load through the CLI instead.
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

func benchAgent(i int) string {
	return fmt.Sprintf(`---
name: agent-%04d
description: Bench agent %d for deterministic fixtures.
tools: [Read, Grep, Bash]
model: sonnet
---

Agent %d instructions. Review the diff. Report path:line problem then fix.
`, i, i, i)
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

func writeBenchFile(b *testing.B, path, content string) {
	b.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		b.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		b.Fatalf("write %s: %v", path, err)
	}
}

// benchChdir switches into dir for the benchmark and restores the prior
// working directory on cleanup. The disk-backed sync paths resolve output
// paths relative to the working directory.
func benchChdir(b *testing.B, dir string) {
	b.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		b.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		b.Fatalf("chdir %s: %v", dir, err)
	}
	b.Cleanup(func() { _ = os.Chdir(cwd) })
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

// benchIsolateGlobalLayer points the user-global layer at an empty temp
// dir so host state under ~/.agnostic-ai cannot leak into a benchmark
// that loads through the CLI (resolveLayers). The dir exists but has no
// spec subdirs, so the layer contributes nothing.
func benchIsolateGlobalLayer(b *testing.B) {
	b.Helper()
	b.Setenv(envUserGlobalRoot, b.TempDir())
}

// resetBenchBuffers clears the process-global capability-warning and
// coverage-note buffers so repeated emit passes do not accumulate across
// iterations or benchmarks.
func resetBenchBuffers() {
	adapters.ResetCapabilityWarnings()
	adapters.ResetCoverageNotes()
}

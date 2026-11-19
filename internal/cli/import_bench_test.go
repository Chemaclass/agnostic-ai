package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// benchImportDirCounts parameterizes the import walk benchmark by the
// number of project directories each root walk visits.
var benchImportDirCounts = []int{1000, 8000}

// benchImportSources are the tools the import fixture holds native
// configuration for, imported in this order by `status-quo/all`.
var benchImportSources = []string{"codex", "cursor", "warp", "opencode", "factory"}

// BenchmarkImportWalks measures the project-wide directory walks import
// makes. Each skill, hierarchical, scoped-rules and Kimi lookup walks
// the whole root on its own, so the cost grows as lookups x directories.
// It follows the "three subjects" shape from docs/internal/benchmarks.md:
//
//   - status-quo/opencode: one OpenCode skill import over its three
//     native directories.
//   - status-quo/all:      `import` over the five fixture sources in one
//     run, the shape of `import all`.
//   - baseline/one-walk:   one bare filepath.WalkDir with no skip checks,
//     the floor for a single walk.
//
// walks/op counts the root walks one operation made, so status-quo reads
// against walks x baseline.
func BenchmarkImportWalks(b *testing.B) {
	for _, d := range benchImportDirCounts {
		b.Run(fmt.Sprintf("status-quo/opencode/dirs=%d", d), func(b *testing.B) {
			root := benchImportProject(b, d)
			dst := filepath.Join(b.TempDir(), "skills")
			nativeDirs := []string{opencodeSkillsDir, ".agents/skills", ".claude/skills"}
			benchImportWalks(b, func() {
				err := withImportTree(root, func() error {
					n, err := importScopedSkillFoldersWith(root, nativeDirs, dst, &opencodeSkillFields)
					benchIntSink = n
					return err
				})
				if err != nil {
					b.Fatal(err)
				}
			}, nil)
		})
		b.Run(fmt.Sprintf("status-quo/all/dirs=%d", d), func(b *testing.B) {
			root := benchImportProject(b, d)
			benchChdir(b, root)
			cfg, err := config.Load(".")
			if err != nil {
				b.Fatal(err)
			}
			specs := filepath.Join(root, config.SourceBaseDir)
			benchImportWalks(b, func() {
				if err := withImportTree(".", func() error {
					return runImportMany(".", benchImportSources, cfg)
				}); err != nil {
					b.Fatal(err)
				}
			}, func() {
				if err := os.RemoveAll(specs); err != nil {
					b.Fatal(err)
				}
			})
		})
		b.Run(fmt.Sprintf("baseline/one-walk/dirs=%d", d), func(b *testing.B) {
			root := benchImportProject(b, d)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				seen := 0
				err := filepath.WalkDir(root, func(_ string, entry fs.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if entry.IsDir() {
						seen++
					}
					return nil
				})
				if err != nil {
					b.Fatal(err)
				}
				benchIntSink = seen
			}
			b.ReportMetric(1, "walks/op")
		})
	}
}

// benchImportWalks times op b.N times, running reset before each one
// outside the timer, and reports the root walks per operation.
func benchImportWalks(b *testing.B, op, reset func()) {
	b.Helper()
	benchQuiet(b)
	benchDiscardStdout(b)
	b.ReportAllocs()
	walks := 0
	b.ResetTimer()
	for range b.N {
		if reset != nil {
			b.StopTimer()
			reset()
			b.StartTimer()
		}
		before := importTreeWalks
		op()
		walks += importTreeWalks - before
	}
	b.ReportMetric(float64(walks)/float64(b.N), "walks/op")
}

// benchImportProject writes a project with d empty directories, two
// levels deep, plus native configuration for every benchImportSources
// tool and an agnostic-ai.yaml that targets them. It is not a git
// repository, so the walks pay no ignore lookup.
func benchImportProject(b *testing.B, d int) string {
	b.Helper()
	root := b.TempDir()
	for i := range d {
		dir := filepath.Join(root, fmt.Sprintf("pkg-%03d", i/50), fmt.Sprintf("dir-%02d", i%50))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			b.Fatal(err)
		}
	}
	yaml := "version: 1\ntargets:\n"
	var yamlSb133 strings.Builder
	for _, s := range benchImportSources {
		yamlSb133.WriteString("  - " + s + "\n")
	}
	yaml += yamlSb133.String()
	writeBenchFile(b, filepath.Join(root, "agnostic-ai.yaml"), yaml)
	writeBenchFile(b, filepath.Join(root, "AGENTS.md"), "# Project\n\nKeep functions small.\n")
	writeBenchFile(b, filepath.Join(root, "pkg-000", "AGENTS.md"), "# Package\n\nPrefer table tests.\n")
	writeBenchFile(b, filepath.Join(root, "WARP.md"), "# Project\n\nKeep functions small.\n")
	writeBenchFile(b, filepath.Join(root, ".codex", "config.toml"), "model = \"gpt-5\"\n")
	writeBenchFile(b, filepath.Join(root, ".cursor", "rules", "style.mdc"), "---\ndescription: Style\nalwaysApply: true\n---\n\nUse tabs.\n")
	writeBenchFile(b, filepath.Join(root, "opencode.json"), "{}\n")
	for _, dir := range []string{".agents/skills", ".opencode/skills", ".cursor/skills", ".factory/skills", ".warp/skills"} {
		name := filepath.Base(filepath.Dir(dir))[1:] + "-skill"
		writeBenchFile(b, filepath.Join(root, filepath.FromSlash(dir), name, "SKILL.md"),
			"---\nname: "+name+"\ndescription: A "+name+" for the import benchmark.\n---\n\nDo the task.\n")
	}
	return root
}

// benchDiscardStdout points os.Stdout at the null device for the
// benchmark, since import prints its progress there.
func benchDiscardStdout(b *testing.B) {
	b.Helper()
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		b.Fatal(err)
	}
	prev := os.Stdout
	os.Stdout = null
	b.Cleanup(func() {
		os.Stdout = prev
		_ = null.Close()
	})
}

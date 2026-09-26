package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const (
	sharedOverRule  = "---\nname: over\ndescription: shared rule\n---\nShared body.\n"
	sharedRevAgent  = "---\nname: rev\ndescription: shared agent\n---\nShared agent.\n"
	localRuleSource = ".agnostic-ai/local/rules/personal.md"
)

// syncWithLocalSpecs writes a project whose local layer adds a rule, an
// agent, and a folder skill, and extends one shared rule and one shared
// agent, then runs a real sync for claude and codex.
func syncWithLocalSpecs(t *testing.T) {
	t.Helper()
	testutil.TempCwd(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\n")
	writeAgnosticFile(t, "# Shared\n")
	writeFile(t, filepath.Join(".agnostic-ai", "rules", "over.md"), sharedOverRule)
	writeFile(t, filepath.Join(".agnostic-ai", "agents", "rev.md"), sharedRevAgent)
	local := filepath.FromSlash(defaultProjectUser)
	writeFile(t, filepath.FromSlash(localRuleSource), "---\nname: personal\ndescription: personal rule\n---\nPersonal body.\n")
	writeFile(t, filepath.Join(local, "rules", "over.md"), "---\nname: over\n---\n::parent\nLocal addition.\n")
	writeFile(t, filepath.Join(local, "agents", "helper.md"), "---\nname: helper\ndescription: local agent\n---\nHelper agent.\n")
	writeFile(t, filepath.Join(local, "agents", "rev.md"), "---\nname: rev\ndescription: local description\n---\n")
	writeFile(t, filepath.Join(local, "skills", "mine", "SKILL.md"), "---\nname: mine\ndescription: local skill\n---\nSkill body.\n")
	writeFile(t, filepath.Join(local, "skills", "mine", "notes.txt"), "asset\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
}

// importCapturing runs `import args...` and returns what it printed.
func importCapturing(t *testing.T, args ...string) string {
	t.Helper()
	return captureStdout(t, func() {
		if out, err := runCLI(t, append([]string{"import"}, args...)...); err != nil {
			t.Fatalf("import %v: %v\n%s", args, err, out)
		}
	})
}

func assertNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("%s: want no shared copy of a local spec, stat err = %v", path, err)
	}
}

func assertFileEquals(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	if string(data) != want {
		t.Errorf("%s changed:\ngot:\n%s\nwant:\n%s", path, data, want)
	}
}

func TestImport_LeavesLocalOnlySpecsOutOfTheSharedSource(t *testing.T) {
	for _, sources := range [][]string{{"claude"}, {"codex"}, {"claude", "codex"}} {
		t.Run(strings.Join(sources, "+"), func(t *testing.T) {
			syncWithLocalSpecs(t)

			out := importCapturing(t, sources...)

			assertNotExist(t, filepath.Join(".agnostic-ai", "rules", "personal.md"))
			assertNotExist(t, filepath.Join(".agnostic-ai", "agents", "helper.md"))
			assertNotExist(t, filepath.Join(".agnostic-ai", "skills", "mine"))
			for _, name := range []string{"agent helper", "rule personal", "skill mine"} {
				if !strings.Contains(out, name) {
					t.Errorf("want the note to name %q:\n%s", name, out)
				}
			}
		})
	}
}

func TestImport_KeepsSharedSpecsTheLocalLayerExtends(t *testing.T) {
	for _, sources := range [][]string{{"claude"}, {"codex"}, {"claude", "codex"}} {
		t.Run(strings.Join(sources, "+"), func(t *testing.T) {
			syncWithLocalSpecs(t)

			out := importCapturing(t, sources...)

			assertFileEquals(t, filepath.Join(".agnostic-ai", "rules", "over.md"), sharedOverRule)
			assertFileEquals(t, filepath.Join(".agnostic-ai", "agents", "rev.md"), sharedRevAgent)
			if !strings.Contains(out, "rule over") {
				t.Errorf("want the note to name the extended rule:\n%s", out)
			}
			if strings.Contains(out, "merge by hand") {
				t.Errorf("want no hint to merge local content into the shared rule:\n%s", out)
			}
		})
	}
}

func TestImport_StillImportsNativeSpecsWithNoLocalCounterpart(t *testing.T) {
	syncWithLocalSpecs(t)
	writeFile(t, filepath.Join(".claude", "agents", "manual.md"), "---\nname: manual\ndescription: hand written\n---\nManual agent.\n")

	importCapturing(t, "claude")

	if _, err := os.Stat(filepath.Join(".agnostic-ai", "agents", "manual.md")); err != nil {
		t.Errorf("hand-written agent not imported: %v", err)
	}
}

func TestImport_DryRunDoesNotListLocalSpecs(t *testing.T) {
	syncWithLocalSpecs(t)

	out := importCapturing(t, "codex", "--dry-run")

	if strings.Contains(out, filepath.Join(".agnostic-ai", "rules", "personal.md")) {
		t.Errorf("dry-run lists the local rule as a shared write:\n%s", out)
	}
	if !strings.Contains(out, "rule personal") {
		t.Errorf("want the dry-run note to name the local rule:\n%s", out)
	}
}

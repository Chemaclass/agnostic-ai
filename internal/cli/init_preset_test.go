package cli

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// presetExpectedFiles enumerates the files each preset must emit. When
// a preset adds or removes a spec, update this list — the snapshot
// guards against silently shipping the wrong content to new adopters.
var presetExpectedFiles = map[string][]string{
	"go": {
		"rules/go-errors.md",
		"rules/go-style.md",
		"rules/go-testing.md",
	},
	"ts-react": {
		"rules/react.md",
		"rules/testing.md",
		"rules/typescript.md",
		"skills/component-scaffold.md",
	},
	"python": {
		"rules/pytest.md",
		"rules/python-style.md",
		"rules/typing.md",
	},
}

func TestInit_PresetGoSeeds(t *testing.T) {
	assertPresetSeeds(t, "go")
}

func TestInit_PresetTSReactSeeds(t *testing.T) {
	assertPresetSeeds(t, "ts-react")
}

func TestInit_PresetPythonSeeds(t *testing.T) {
	assertPresetSeeds(t, "python")
}

func assertPresetSeeds(t *testing.T, preset string) {
	t.Helper()
	dir := t.TempDir()
	if err := scaffold(dir, "", false, preset, allTargetNames()); err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	got := walkFiles(t, filepath.Join(dir, ".agnostic-ai"))
	want := presetExpectedFiles[preset]
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("preset %q file list mismatch\n want: %v\n  got: %v", preset, want, got)
	}
	for _, rel := range got {
		body, err := os.ReadFile(filepath.Join(dir, ".agnostic-ai", rel))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), "name:") {
			t.Errorf("%s: missing frontmatter `name:` field", rel)
		}
	}
}

func TestInit_PresetUnknownErrors(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"init", "--preset", "wat"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for unknown preset")
	}
	if !strings.Contains(err.Error(), "unknown preset") {
		t.Errorf("error should name the issue: got %v", err)
	}
	if !strings.Contains(err.Error(), "go") || !strings.Contains(err.Error(), "ts-react") || !strings.Contains(err.Error(), "python") {
		t.Errorf("error should list available presets: got %v", err)
	}
}

func TestInit_PresetComposesWithDemo(t *testing.T) {
	dir := t.TempDir()
	if err := scaffold(dir, "", true, "go", allTargetNames()); err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	got := walkFiles(t, filepath.Join(dir, ".agnostic-ai"))
	mustContain := []string{
		"rules/conventional-commits.md", // from --demo
		"rules/go-style.md",             // from --preset go
	}
	for _, p := range mustContain {
		found := false
		for _, g := range got {
			if g == p {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %s in seeded files; got %v", p, got)
		}
	}
}

func TestInit_PresetDoesNotOverwriteExistingFiles(t *testing.T) {
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, ".agnostic-ai", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(rulesDir, "go-style.md")
	preserved := []byte("---\nname: go-style\n---\nUSER WROTE THIS\n")
	if err := os.WriteFile(existing, preserved, 0o644); err != nil {
		t.Fatal(err)
	}
	// scaffold errors when agnostic-ai.yaml exists, so call
	// writePresetFiles directly to test the no-clobber guarantee.
	if err := writePresetFiles(filepath.Join(dir, ".agnostic-ai"), "go"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(existing)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(preserved) {
		t.Error("preset overwrote a pre-existing user file")
	}
}

func TestInit_PresetSuggestsSyncInNextSteps(t *testing.T) {
	dir := t.TempDir()
	buf := captureSummary(t)
	if err := scaffold(dir, "", false, "go", allTargetNames()); err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "agnostic-ai sync --check") {
		t.Errorf("preset scaffold should suggest sync --check:\n%s", out)
	}
	if strings.Contains(out, "agnostic-ai import <target>") {
		t.Errorf("preset scaffold should not show import <target>:\n%s", out)
	}
}

func TestAvailablePresets_ListsEmbedded(t *testing.T) {
	got := availablePresets()
	for _, want := range []string{"go", "ts-react", "python"} {
		found := false
		for _, g := range got {
			if g == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("availablePresets missing %q; got %v", want, got)
		}
	}
}

// walkFiles returns relative paths (forward slashes) of every file
// under root, sorted, suitable for snapshot comparison.
func walkFiles(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

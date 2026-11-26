package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// applyGitignoreEdits makes edits in the ignore files under dir, last
// line first so each edit's line number still holds.
func applyGitignoreEdits(t *testing.T, dir string, edits []gitignoreEdit) {
	t.Helper()
	bySource := map[string][]gitignoreEdit{}
	for _, e := range edits {
		bySource[e.Source] = append(bySource[e.Source], e)
	}
	for source, list := range bySource {
		path := filepath.Join(dir, filepath.FromSlash(source))
		data, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
		if len(data) == 0 {
			lines = nil
		}
		slices.SortFunc(list, func(a, b gitignoreEdit) int {
			if a.Line == 0 {
				return 1
			}
			if b.Line == 0 {
				return -1
			}
			return b.Line - a.Line
		})
		var tail []string
		for _, e := range list {
			switch {
			case e.Line == 0:
				tail = append(tail, e.Add...)
			case e.Replace:
				if lines[e.Line-1] != e.Pattern {
					t.Fatalf("%s line %d = %q, want %q", source, e.Line, lines[e.Line-1], e.Pattern)
				}
				lines = slices.Concat(lines[:e.Line-1], e.Add, lines[e.Line:])
			default:
				lines = slices.Concat(lines[:e.Line], e.Add, lines[e.Line:])
			}
		}
		lines = append(lines, tail...)
		writeFile(t, path, strings.Join(lines, "\n")+"\n")
	}
}

func TestPlanGitignoreEdits_MakesIgnoredFilesTrackable(t *testing.T) {
	dir, _ := gitRepo(t)
	for _, rel := range []string{
		".claude/settings.json", ".claude/hooks/install.sh", ".claude/hooks/other.sh", ".claude/launch.json",
		".agents/setup", ".agents/other", "AGENTS.md", "sub/AGENTS.md",
		".github/workflows/copilot-setup-steps.yml", ".ellipsis/environments/dev.yaml",
		"nested/.cursor/environment.json", ".amp/services.yaml",
	} {
		writeFile(t, filepath.Join(dir, filepath.FromSlash(rel)), "x\n")
	}
	writeFile(t, filepath.Join(dir, ".gitignore"), strings.Join([]string{
		"# generated",
		"/.claude/",
		".agents/",
		"**/AGENTS.md",
		"/.github/workflows/copilot-setup-steps.yml",
		".ellipsis",
	}, "\n")+"\n")
	writeFile(t, filepath.Join(dir, "nested", ".gitignore"), "/.cursor/\n")
	writeFile(t, filepath.Join(dir, ".git", "info", "exclude"), "/.amp/\n")
	files := []string{
		".claude/settings.json", ".claude/hooks/install.sh", ".agents/setup", "AGENTS.md",
		".github/workflows/copilot-setup-steps.yml", ".ellipsis/environments/dev.yaml",
		"nested/.cursor/environment.json", ".amp/services.yaml",
	}

	edits, err := planGitignoreEdits(dir, files)
	if err != nil {
		t.Fatal(err)
	}
	want := []gitignoreEdit{
		{Source: ".gitignore", Line: 2, Pattern: "/.claude/", Replace: true, Add: []string{"/.claude/*", "!/.claude/settings.json", "!/.claude/hooks/", "/.claude/hooks/*", "!/.claude/hooks/install.sh"}},
		{Source: ".gitignore", Line: 3, Pattern: ".agents/", Add: []string{"!/.agents/", "/.agents/*", "!/.agents/setup"}},
		{Source: ".gitignore", Line: 4, Pattern: "**/AGENTS.md", Add: []string{"!/AGENTS.md"}},
		{Source: ".gitignore", Line: 5, Pattern: "/.github/workflows/copilot-setup-steps.yml", Replace: true, Add: []string{}},
		{Source: ".gitignore", Line: 6, Pattern: ".ellipsis", Add: []string{"!/.ellipsis/", "/.ellipsis/*", "!/.ellipsis/environments/", "/.ellipsis/environments/*", "!/.ellipsis/environments/dev.yaml"}},
		{Source: ".gitignore", Add: []string{"!/.amp/", "/.amp/*", "!/.amp/services.yaml"}},
		{Source: "nested/.gitignore", Line: 1, Pattern: "/.cursor/", Replace: true, Add: []string{"/.cursor/*", "!/.cursor/environment.json"}},
	}
	if len(edits) != len(want) {
		t.Fatalf("edits = %+v\nwant %+v", edits, want)
	}
	for i := range want {
		got := edits[i]
		if got.Source != want[i].Source || got.Line != want[i].Line || got.Pattern != want[i].Pattern || got.Replace != want[i].Replace || !slices.Equal(got.Add, want[i].Add) {
			t.Errorf("edit %d = %+v, want %+v", i, got, want[i])
		}
	}

	applyGitignoreEdits(t, dir, edits)
	for _, rel := range files {
		if gitIgnored(t, dir, rel) {
			t.Errorf("%s is still ignored after the edits", rel)
		}
	}
	for _, rel := range []string{".claude/hooks/other.sh", ".claude/launch.json", ".agents/other", "sub/AGENTS.md"} {
		if !gitIgnored(t, dir, rel) {
			t.Errorf("%s is no longer ignored; the edits must re-include only the named files", rel)
		}
	}
	again, err := planGitignoreEdits(dir, files)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Errorf("edits after applying = %+v, want none", again)
	}
}

func TestNamesExactly_MatchesOnlyAnchoredLiteralPaths(t *testing.T) {
	tests := []struct {
		pattern, path string
		want          bool
	}{
		{"/.claude/", ".claude", true},
		{"/.claude", ".claude", true},
		{".github/workflows/x.yml", ".github/workflows/x.yml", true},
		{".claude/", ".claude", false},
		{"**/AGENTS.md", "AGENTS.md", false},
		{"/.c*/", ".claude", false},
		{"/.cursor/", ".claude", false},
	}
	for _, tt := range tests {
		if got := namesExactly(tt.pattern, tt.path); got != tt.want {
			t.Errorf("namesExactly(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
		}
	}
}

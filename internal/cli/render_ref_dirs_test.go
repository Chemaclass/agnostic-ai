package cli

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestRenderDirs(t *testing.T) {
	files := []string{
		"agnostic-ai.yaml",
		".agnostic-ai/rules/backend/auth.md",
		".agnostic-ai/rules/frontend/ui.md",
		".agnostic-ai/skills/review/SKILL.md",
		"backend/handler.go",
		"apps/web/page.go",
		"apps/api/route.go",
		"docs/guide.md",
	}
	specs := files[:4]
	for _, tc := range []struct {
		name       string
		prefix     string
		files      []string
		specs      []string
		linkedSpec bool
		want       []string
	}{
		{
			name:  "creates the scopes the specs name, not every tracked directory",
			files: files,
			specs: specs,
			want:  []string{".agnostic-ai", ".agnostic-ai/rules", ".agnostic-ai/rules/backend", ".agnostic-ai/rules/frontend", ".agnostic-ai/skills", ".agnostic-ai/skills/review", "backend"},
		},
		{
			name:       "a linked spec may reach any directory, so every tracked one exists",
			files:      files,
			specs:      specs,
			linkedSpec: true,
			want:       []string{".agnostic-ai", ".agnostic-ai/rules", ".agnostic-ai/rules/backend", ".agnostic-ai/rules/frontend", ".agnostic-ai/skills", ".agnostic-ai/skills/review", "apps", "apps/api", "apps/web", "backend", "docs"},
		},
		{
			name:   "keeps the project directory of a nested project",
			prefix: "svc/",
			files:  []string{"svc/agnostic-ai.yaml", "svc/backend/handler.go", "svc/apps/web/page.go", "svc/.agnostic-ai/rules/backend/auth.md"},
			specs:  []string{"svc/agnostic-ai.yaml", "svc/.agnostic-ai/rules/backend/auth.md"},
			want:   []string{"svc", "svc/.agnostic-ai", "svc/.agnostic-ai/rules", "svc/.agnostic-ai/rules/backend", "svc/backend"},
		},
		{
			name:   "keeps the project directory when no spec sits in it",
			prefix: "svc/",
			files:  []string{"svc/backend/handler.go"},
			want:   []string{"svc"},
		},
		{
			name:  "a project with only root files needs no directory",
			files: []string{"agnostic-ai.yaml", "README.md"},
			specs: []string{"agnostic-ai.yaml"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := renderDirs(tc.prefix, tc.files, tc.specs, tc.linkedSpec)
			sort.Strings(got)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("renderDirs:\n got %v\nwant %v", got, tc.want)
			}
		})
	}
}

// A render creates empty only the directories a spec can scope to, and
// the result must match a render where every directory exists.
func TestRenderRef_MatchesARenderWhereEveryDirectoryExists(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("scoped output paths are compared as slash paths")
	}
	for _, nested := range []bool{false, true} {
		t.Run(fmt.Sprintf("nested=%t", nested), func(t *testing.T) {
			dir, git := launchGitRepo(t)
			project, prefix := dir, ""
			if nested {
				project, prefix = filepath.Join(dir, "svc"), "svc/"
			}
			testutil.Chdir(t, dir)
			mustWriteFile(t, filepath.Join(project, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, codex]\ngitignore: {enabled: false}\n")
			mustWriteFile(t, filepath.Join(project, ".agnostic-ai/rules/backend/auth.md"), "---\nname: auth\n---\nAuth rule.\n")
			mustWriteFile(t, filepath.Join(project, ".agnostic-ai/rules/frontend/ui.md"), "---\nname: ui\n---\nUI rule.\n")
			mustWriteFile(t, filepath.Join(project, ".agnostic-ai/skills/backend/review/SKILL.md"), "---\nname: review\ndescription: Review.\n---\nReview.\n")
			mustWriteFile(t, filepath.Join(project, ".agnostic-ai/rules/global.md"), "---\nname: global\nalwaysApply: true\n---\nGlobal rule.\n")
			mustWriteFile(t, filepath.Join(project, "backend/handler.go"), "package backend\n")
			for i := 0; i < 12; i++ {
				mustWriteFile(t, filepath.Join(project, fmt.Sprintf("apps/unit-%02d/src/main.go", i)), "package main\n")
			}
			git("add", "-A")
			git("commit", "-q", "-m", "project")
			testutil.Chdir(t, project)

			want, err := plannedOutputs()
			if err != nil {
				t.Fatal(err)
			}
			got, err := renderRef(dir, prefix, "HEAD", filepath.Join(t.TempDir(), "tree"), nil)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("render with only the scope directories differs from one with every directory:\n got %v\nwant %v", slices.Sorted(maps.Keys(got)), slices.Sorted(maps.Keys(want)))
			}
			if !hasKeyUnder(got, "backend/") {
				t.Errorf("the existing backend/ directory should scope its rule, got %v", slices.Sorted(maps.Keys(got)))
			}
			if hasKeyUnder(got, "frontend/") {
				t.Errorf("frontend/ holds no files, so its rule stays global, got %v", slices.Sorted(maps.Keys(got)))
			}
			if wd, err := os.Getwd(); err != nil || !sameFile(wd, project) {
				t.Errorf("the working directory was not restored: %q, %v", wd, err)
			}
		})
	}
}

func hasKeyUnder(m map[string]string, dir string) bool {
	for k := range m {
		if strings.HasPrefix(k, dir) {
			return true
		}
	}
	return false
}

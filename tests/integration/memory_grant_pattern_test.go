package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type memoryGrantCase struct{ target, file, home string }

var memoryGrantRefused = []memoryGrantCase{
	{"opencode", "opencode.json", "home*1"},
	{"opencode", "opencode.json", "home?1"},
	{"windsurf", ".devin/config.json", "home[1]"},
	{"windsurf", ".devin/config.json", "home(1)"},
	{"cursor", ".cursor/cli.json", "home[1]"},
	{"cursor", ".cursor/cli.json", "home(1)"},
}

var memoryGrantConsequence = map[string]string{
	"opencode": "OpenCode will ask before it writes memory there",
	"windsurf": "Devin will ask before it writes memory there",
	"cursor":   "Cursor will ask before it writes memory there",
}

// grantsPath reports whether a line of content names home in a pattern
// that ends in /**. An OpenCode instructions entry names the folder too,
// as a literal file path, and is not a grant.
func grantsPath(content, home string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, home) && strings.Contains(line, "/**") {
			return true
		}
	}
	return false
}

func memoryGrantProject(t *testing.T, target string) string {
	t.Helper()
	project := t.TempDir()
	handoffInitGit(t, project)
	handoffWrite(t, filepath.Join(project, "agnostic-ai.yaml"), "version: 1\nbuiltins: [memory]\ntargets: ["+target+"]\ngitignore:\n  enabled: true\n")
	handoffWrite(t, filepath.Join(project, "agnostic-ai.local.yaml"), "memory:\n  personal: repo\n")
	return project
}

func memoryGrantHome(t *testing.T, name string) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), name)
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGNOSTIC_AI_HOME", home)
	return home
}

func memoryGrantSync(t *testing.T, binary, project string, args ...string) string {
	t.Helper()
	cmd := exec.Command(binary, append([]string{"sync"}, args...)...)
	cmd.Dir = project
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sync %v: %v\n%s", args, err, output)
	}
	return string(output)
}

func memoryGrantFile(t *testing.T, project, file string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(project, file))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(content)
}

// The personal memory folder lies under AGNOSTIC_AI_HOME. A pattern
// character in that path would widen or break the write grant, so sync
// writes no grant and says why.
func TestMemoryGrant_RefusesPatternCharactersInTheFolderPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows filenames cannot contain these characters")
	}
	binary := handoffBuildCLI(t)
	for i, tc := range memoryGrantRefused {
		t.Run(fmt.Sprintf("%s/%d", tc.target, i), func(t *testing.T) {
			home := memoryGrantHome(t, tc.home)
			project := memoryGrantProject(t, tc.target)
			output := memoryGrantSync(t, binary, project)
			for _, want := range []string{"pattern characters", memoryGrantConsequence[tc.target]} {
				if !strings.Contains(output, want) {
					t.Errorf("note lacks %q: %s", want, output)
				}
			}
			memoryGrantSync(t, binary, project, "--check")
			if content := memoryGrantFile(t, project, tc.file); grantsPath(content, tc.home) {
				t.Errorf("pattern path became a grant: %s", content)
			}
			if _, err := os.Stat(filepath.Join(home, "local", "memory")); !os.IsNotExist(err) {
				t.Errorf("unsafe path created the store: %v", err)
			}
		})
	}
}

// OpenCode escapes every pattern character but `*` and `?`, and takes
// the folder of an instructions entry as a literal directory, so `[` and
// `{` stay safe in both.
func TestMemoryGrant_OpenCodeKeepsBracketsAndBracesLiteral(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows filenames cannot contain these characters")
	}
	binary := handoffBuildCLI(t)
	for i, name := range []string{"home[1]", "home{a,b}"} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			memoryGrantHome(t, name)
			project := memoryGrantProject(t, "opencode")
			output := memoryGrantSync(t, binary, project)
			if strings.Contains(output, "pattern characters") {
				t.Errorf("literal path was refused: %s", output)
			}
			memoryGrantSync(t, binary, project, "--check")
			content := memoryGrantFile(t, project, "opencode.json")
			if !strings.Contains(content, name+"/local/memory/") || !strings.Contains(content, `/**": "allow"`) {
				t.Errorf("grant or instructions entry missing: %s", content)
			}
		})
	}
}

// A grant an earlier sync wrote leaves once the folder moves to a path
// sync refuses, and the next check finds nothing to change.
func TestMemoryGrant_RefusedPathRemovesTheEarlierGrant(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows filenames cannot contain these characters")
	}
	binary := handoffBuildCLI(t)
	for i, tc := range memoryGrantRefused {
		t.Run(fmt.Sprintf("%s/%d", tc.target, i), func(t *testing.T) {
			safe := memoryGrantHome(t, "home")
			project := memoryGrantProject(t, tc.target)
			memoryGrantSync(t, binary, project)
			if content := memoryGrantFile(t, project, tc.file); !strings.Contains(content, safe) {
				t.Fatalf("first sync wrote no grant for %s: %s", safe, content)
			}
			memoryGrantHome(t, tc.home)
			memoryGrantSync(t, binary, project)
			memoryGrantSync(t, binary, project, "--check")
			if content := memoryGrantFile(t, project, tc.file); strings.Contains(content, safe) || grantsPath(content, tc.home) {
				t.Errorf("earlier grant stayed or refused path was written: %s", content)
			}
		})
	}
}

package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The personal memory folder lies under AGNOSTIC_AI_HOME. A pattern
// character in that path would widen or break the write grant, and
// OpenCode reads each instructions entry as a glob, so sync names the
// folder in neither and says why.
func TestMemoryGrant_RefusesPatternCharactersInTheFolderPath(t *testing.T) {
	binary := handoffBuildCLI(t)
	for _, tc := range []struct{ target, file, home string }{
		{"opencode", "opencode.json", "home[1]"},
		{"opencode", "opencode.json", "home{a,b}"},
		{"windsurf", ".devin/config.json", "home[1]"},
		{"windsurf", ".devin/config.json", "home(1)"},
		{"cursor", ".cursor/cli.json", "home[1]"},
		{"cursor", ".cursor/cli.json", "home(1)"},
	} {
		t.Run(tc.target+"/"+tc.home, func(t *testing.T) {
			home := filepath.Join(t.TempDir(), tc.home)
			if err := os.Mkdir(home, 0o700); err != nil {
				t.Fatal(err)
			}
			project := t.TempDir()
			handoffInitGit(t, project)
			handoffWrite(t, filepath.Join(project, "agnostic-ai.yaml"), "version: 1\nbuiltins: [memory]\ntargets: ["+tc.target+"]\ngitignore:\n  enabled: true\n")
			handoffWrite(t, filepath.Join(project, "agnostic-ai.local.yaml"), "memory:\n  personal: repo\n")
			t.Setenv("AGNOSTIC_AI_HOME", home)
			cmd := exec.Command(binary, "sync")
			cmd.Dir = project
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("sync: %v\n%s", err, output)
			}
			if !strings.Contains(string(output), "glob characters") {
				t.Errorf("missing note: %s", output)
			}
			check := exec.Command(binary, "sync", "--check")
			check.Dir = project
			if output, err := check.CombinedOutput(); err != nil {
				t.Errorf("sync --check: %v\n%s", err, output)
			}
			content, err := os.ReadFile(filepath.Join(project, tc.file))
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			if strings.Contains(string(content), tc.home) {
				t.Errorf("pattern path became a grant: %s", content)
			}
			if _, err := os.Stat(filepath.Join(home, "local", "memory")); !os.IsNotExist(err) {
				t.Errorf("unsafe path created the store: %v", err)
			}
		})
	}
}

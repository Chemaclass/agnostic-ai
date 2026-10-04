package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// settings.json merges into what is on disk, so a portable rule must
// leave when its spec drops it, a hand-written rule must stay, and
// import must bring back only the hand-written one (#1530).
func TestClaudePermissions_RemovedRulesLeaveAndImportKeepsOnlyHandWrittenOnes(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	spec := filepath.Join(dir, ".agnostic-ai", "settings", "security.yaml")
	must(t, os.MkdirAll(filepath.Dir(spec), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte("version: 1\ntargets: [claude]\n"), 0o644))
	must(t, os.WriteFile(spec, []byte("permissions:\n  allow: [\"Bash(git push:*)\"]\n  deny: [\"Bash(rm:*)\"]\n"), 0o644))
	runCmd(t, "sync")

	settings := filepath.Join(dir, ".claude", "settings.json")
	body := readFile(t, settings)
	must(t, os.WriteFile(settings, []byte(strings.Replace(body, `"Bash(rm:*)"`, `"Bash(rm:*)", "Bash(mine:*)"`, 1)), 0o644))

	must(t, os.WriteFile(spec, []byte("permissions:\n  deny: [\"Bash(git push:*)\"]\n"), 0o644))
	runCmd(t, "sync")
	body = readFile(t, settings)
	for _, gone := range []string{"Bash(rm:*)", `"allow"`} {
		if strings.Contains(body, gone) {
			t.Errorf("settings.json still has %s:\n%s", gone, body)
		}
	}
	for _, kept := range []string{"Bash(git push:*)", "Bash(mine:*)"} {
		if !strings.Contains(body, kept) {
			t.Errorf("settings.json lost %s:\n%s", kept, body)
		}
	}
	runCmd(t, "sync", "--check")

	runCmd(t, "import", "claude")
	sources := sourceSnapshot(t, filepath.Join(dir, ".agnostic-ai"))
	all := ""
	for _, body := range sources {
		all += body
	}
	if strings.Contains(all, "Bash(rm:*)") || strings.Count(all, "Bash(git push:*)") != 1 {
		t.Errorf("import brought back a generated rule:\n%s", all)
	}
	if !strings.Contains(all, "shell(mine:*)") {
		t.Errorf("import dropped the hand-written rule:\n%s", all)
	}
}

package cli

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Every target that takes agents either writes an agent's `hooks` and
// `permissionMode` or says they have no effect, never neither (#3052).
// Only a target that copies frontmatter verbatim writes a field and notes
// it.
func TestSync_AgentHooksAndPermissionModeAreWrittenOrNoted(t *testing.T) {
	verbatim := map[string]bool{"junie/hooks": true}
	fields := []struct{ field, frontmatter, written string }{
		{"hooks", "hooks:\n  PreToolUse:\n    - matcher: Bash\n      hooks:\n        - type: command\n          command: echo guard-3052\n", "guard-3052"},
		{"permissionMode", "permissionMode: plan\n", "permissionMode"},
	}
	for _, target := range adapters.Names() {
		a, _ := adapters.Get(target)
		if !slices.Contains(a.Capabilities(), spec.KindAgent) {
			continue
		}
		for _, f := range fields {
			t.Run(target+"/"+f.field, func(t *testing.T) {
				dir := testutil.TempCwd(t)
				silence(t)
				notes := captureNotes(t)
				mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: ["+target+"]\n")
				mustWrite(t, filepath.Join(dir, ".agnostic-ai", "agents", "guard.md"), "---\nname: guard\ndescription: Guards the shell.\n"+f.frontmatter+"---\n\nGuard.\n")
				mustSync(t)

				written := outputContains(t, dir, f.written)
				noted := strings.Contains(notes.String(), "`"+f.field+"` on 1 agent has no effect on "+target+" (")
				if wantNote := !written || verbatim[target+"/"+f.field]; noted != wantNote {
					t.Errorf("%s: written=%v noted=%v; notes:\n%s", f.field, written, noted, notes)
				}
			})
		}
	}
}

func outputContains(t *testing.T, root, needle string) bool {
	t.Helper()
	found := false
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".agnostic-ai" || d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), needle) {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const finalReviewRule = "---\nname: example\n---\nKeep changes small.\n"

// Directory settings get no sibling .bak, so one named like another's
// backup is no collision.
func TestSync_DirectorySettingNamedLikeABackup_Passes(t *testing.T) {
	for _, args := range [][]string{{"sync", "--gitignore=off", "--jobs=1"}, {"validate"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			dir := testutil.TempCwd(t)
			silence(t)
			mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "example.md"), finalReviewRule)
			mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "example", "SKILL.md"), "---\nname: example\ndescription: An example skill.\n---\nCheck the change.\n")
			mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\noutputs:\n  claude:\n    rules-dir: rules\n    skills-dir: rules.bak\n")

			if out, err := runCLI(t, args...); err != nil {
				t.Fatalf("%v: %v\n%s", args, err, out)
			}
			if args[0] != "sync" {
				return
			}
			for _, p := range []string{"rules/example.md", "rules.bak/example/SKILL.md"} {
				if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
					t.Errorf("sync must write %s: %v", p, err)
				}
			}
		})
	}
}

// sharedAgentsEditProject syncs a project whose Codex rules-file is the
// AGENTS.md Amp reads, then saves a hand edit to it as AGENTS.md.bak.
func sharedAgentsEditProject(t *testing.T) (dir, edited string) {
	t.Helper()
	dir = testutil.TempCwd(t)
	silence(t)
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "example.md"), finalReviewRule)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [codex, amp]\noutputs:\n  codex:\n    rules-file: AGENTS.md\n")
	if out, err := runCLI(t, "sync", "--gitignore=off", "--jobs=1"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	edited = string(data) + "\nUNIQUE USER NOTE\n"
	mustWriteFile(t, filepath.Join(dir, "AGENTS.md"), edited)
	if out, err := runCLI(t, "sync", "--gitignore=off", "--jobs=1"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if bak, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md.bak")); string(bak) != edited {
		t.Fatalf("sync must save the edit as AGENTS.md.bak, got:\n%s", bak)
	}
	return dir, edited
}

func TestRevertCmd_ForceRestoresASharedRulesFileOnce(t *testing.T) {
	for _, args := range [][]string{{"revert", "--force"}, {"revert", "--force", "--json"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			dir, edited := sharedAgentsEditProject(t)

			out, err := runCLI(t, args...)

			if err != nil {
				t.Fatalf("%v: %v\n%s", args, err, out)
			}
			if got, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md")); string(got) != edited {
				t.Errorf("revert must restore the saved edit, got:\n%s", got)
			}
			if args[len(args)-1] == "--json" {
				if n := agentsActions(t, out); len(n) != 1 || n[0] != "restore" {
					t.Errorf("revert must report one restore of AGENTS.md, got %v", n)
				}
			}
		})
	}
}

func TestRevertCmd_DryRunListsASharedRulesFileOnce(t *testing.T) {
	_, _ = sharedAgentsEditProject(t)

	out, err := runCLI(t, "revert", "--force", "--dry-run", "--json")

	if err != nil {
		t.Fatalf("revert: %v\n%s", err, out)
	}
	if n := agentsActions(t, out); len(n) != 1 || n[0] != "restore" {
		t.Errorf("the preview must list one restore of AGENTS.md, got %v", n)
	}
}

func agentsActions(t *testing.T, out string) []string {
	t.Helper()
	var got jsonOutput
	if err := json.Unmarshal([]byte(out[strings.Index(out, "{"):]), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	var actions []string
	for _, r := range append(got.Writes, got.Skipped...) {
		if filepath.Clean(r.Path) == "AGENTS.md" {
			actions = append(actions, r.Action)
		}
	}
	return actions
}

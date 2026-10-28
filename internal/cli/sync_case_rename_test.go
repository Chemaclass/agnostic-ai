package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSync_RemovesDistinctHardLinkedOutput(t *testing.T) {
	testSyncRemovesHardLinkedOutput(t, "old-rule", "current-rule")
}

func TestSync_CaseOnlyRenameRemovesDistinctHardLinkedOutput(t *testing.T) {
	if caseInsensitiveDirectory(t, t.TempDir()) {
		t.Skip("the temp filesystem is case-insensitive")
	}
	testSyncRemovesHardLinkedOutput(t, "My Rule", "my rule")
}

func testSyncRemovesHardLinkedOutput(t *testing.T, oldName, currentName string) {
	t.Helper()
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	for _, name := range []string{oldName, currentName} {
		mustWriteFile(t, ".agnostic-ai/rules/"+name+".md", "---\nname: "+name+"\ndescription: Rule.\n---\nUse this rule.\n")
	}
	runSyncOK(t)
	old, current := ".claude/rules/"+oldName+".md", ".claude/rules/"+currentName+".md"
	want := readFile(t, current)
	if err := os.Remove(old); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(current, old); err != nil {
		t.Fatalf("create hard link: %v", err)
	}
	if err := os.Remove(".agnostic-ai/rules/" + oldName + ".md"); err != nil {
		t.Fatal(err)
	}
	result := runSyncJSONArgs(t, "--json")
	if record, ok := recordFor(result.Writes, old); !ok || record.Action != "delete" {
		t.Errorf("sync did not report stale hard link removed: %+v", result.Writes)
	}
	if _, err := os.Lstat(old); !os.IsNotExist(err) {
		t.Errorf("stale hard link remains: %v", err)
	}
	if got := readFile(t, current); got != want {
		t.Errorf("current output changed: %q", got)
	}
	state := readStateFile(".")
	if slices.Contains(state.Outputs, old) || slices.Contains(state.Orphans, old) || slices.Contains(state.Unledgered, old) {
		t.Errorf("removed hard link remains in ledger: %+v", state)
	}
	if !slices.Contains(state.Outputs, current) {
		t.Errorf("current output lost ownership: %v", state.Outputs)
	}
	runSyncOK(t, "--check")
}

func TestSync_CaseOnlyRenamePreservesOutputs(t *testing.T) {
	dir := t.TempDir()
	if !caseInsensitiveDirectory(t, dir) {
		t.Skip("the temp filesystem is case-sensitive")
	}
	testSyncCaseOnlyRename(t, true)
}

func TestSync_CaseOnlyRenamePreservesHandEdits(t *testing.T) {
	if !caseInsensitiveDirectory(t, t.TempDir()) {
		t.Skip("the temp filesystem is case-sensitive")
	}
	for _, mode := range []string{"backup", "backup conflict", "keep edits"} {
		t.Run(mode, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
			const source = ".agnostic-ai/rules/example.md"
			const old, current = ".claude/rules/My Rule.md", ".claude/rules/my rule.md"
			mustWriteFile(t, source, "---\nname: My Rule\ndescription: Rule.\n---\nOriginal.\n")
			runSyncOK(t)
			baseline := readStateFile(".").OutputSums[old]
			edited := readFile(t, old) + "\nUser edit.\n"
			mustWriteFile(t, old, edited)
			if mode == "backup conflict" {
				mustWriteFile(t, old+".bak", "Earlier backup.\n")
			}
			mustWriteFile(t, source, "---\nname: my rule\ndescription: Rule.\n---\nUpdated.\n")
			args := []string{"--json"}
			if mode == "keep edits" {
				args = append(args, "--keep-edits")
			}
			result := runSyncJSONArgs(t, args...)
			state := readStateFile(".")
			if slices.Contains(state.Outputs, old) || !slices.Contains(state.Outputs, current) {
				t.Errorf("rename ownership not transferred: %v", state.Outputs)
			}
			if mode == "backup" {
				if got := readFile(t, current+".bak"); got != edited {
					t.Errorf("backup=%q, want exact hand edit", got)
				}
				if got := readFile(t, current); !strings.Contains(got, "Updated.") || strings.Contains(got, "User edit.") {
					t.Errorf("output not updated: %q", got)
				}
				if state.OutputSums[current] == baseline || state.OutputSums[current] == "" {
					t.Errorf("new baseline not saved: %v", state.OutputSums)
				}
				runSyncOK(t, "--check")
				if out, err := runCLI(t, "revert"); err != nil {
					t.Fatalf("revert: %v\n%s", err, out)
				}
				if got := readFile(t, current); got != edited {
					t.Errorf("revert lost edit: %q", got)
				}
				return
			}
			if got := readFile(t, current); got != edited {
				t.Errorf("preserved output=%q, want hand edit", got)
			}
			if state.OutputSums[current] != baseline {
				t.Errorf("lost prior baseline: got %q, want %q", state.OutputSums[current], baseline)
			}
			if record, ok := recordFor(result.Skipped, current); !ok || record.Action != "edited" {
				t.Errorf("edit not reported as kept: %+v", result)
			}
			if got := checkJSONActions(t)[current]; got != "edited" {
				t.Errorf("check action=%q, want edited", got)
			}
			if mode == "backup conflict" {
				if got := readFile(t, current+".bak"); got != "Earlier backup.\n" {
					t.Errorf("earlier backup changed: %q", got)
				}
			} else if _, err := os.Lstat(current + ".bak"); !os.IsNotExist(err) {
				t.Errorf("keep-edits wrote a backup: %v", err)
			}
		})
	}
}

func TestSync_CaseOnlyRenameRemovesDistinctOldOutputs(t *testing.T) {
	dir := t.TempDir()
	if caseInsensitiveDirectory(t, dir) {
		t.Skip("the temp filesystem is case-insensitive")
	}
	testSyncCaseOnlyRename(t, false)
}

func caseInsensitiveDirectory(t *testing.T, dir string) bool {
	t.Helper()
	path := filepath.Join(dir, "CaseProbe")
	mustWriteFile(t, path, "probe\n")
	a, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.Stat(filepath.Join(dir, "caseprobe"))
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		t.Fatal(err)
	}
	return os.SameFile(a, b)
}

func testSyncCaseOnlyRename(t *testing.T, sameFile bool) {
	t.Helper()
	for _, tc := range []struct {
		kind    string
		spec    string
		name    string
		outputs []string
	}{
		{"rule", ".agnostic-ai/rules/example.md", "My Rule", []string{
			".claude/rules/My Rule.md", ".cursor/rules/My Rule.mdc",
		}},
		{"skill", ".agnostic-ai/skills/example/SKILL.md", "My Skill", []string{
			".claude/skills/My Skill/SKILL.md", ".cursor/skills/My Skill/SKILL.md",
			".claude/skills/My Skill/references/check.md", ".cursor/skills/My Skill/references/check.md",
		}},
		{"agent", ".agnostic-ai/agents/example.md", "My Agent", []string{
			".claude/agents/My Agent.md", ".cursor/agents/My Agent.md",
		}},
	} {
		for _, mode := range []string{"text", "json"} {
			t.Run(tc.kind+"/"+mode, func(t *testing.T) {
				testutil.Chdir(t, t.TempDir())
				mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, cursor]\n")
				spec := "---\nname: " + tc.name + "\ndescription: Rename fixture.\n"
				if tc.kind == "rule" {
					spec += "globs: \"**/*.go\"\n"
				}
				spec += "---\nBefore rename.\n"
				mustWriteFile(t, tc.spec, spec)
				if tc.kind == "skill" {
					mustWriteFile(t, ".agnostic-ai/skills/example/references/check.md", "Reference content.\n")
				}
				var args []string
				if mode == "json" {
					args = []string{"--json"}
				}
				runSyncOK(t, args...)
				for _, path := range tc.outputs {
					if _, err := os.Stat(path); err != nil {
						t.Fatalf("baseline %s: %v", path, err)
					}
				}

				spec = strings.Replace(spec, tc.name, strings.ToLower(tc.name), 1)
				mustWriteFile(t, tc.spec, strings.Replace(spec, "Before rename.", "After rename.", 1))
				runSyncOK(t, args...)
				state := readStateFile(".")
				for _, old := range tc.outputs {
					current := strings.Replace(old, tc.name, strings.ToLower(tc.name), 1)
					data, err := os.ReadFile(current)
					if err != nil {
						t.Errorf("renamed output %s: %v", current, err)
					} else if !strings.Contains(string(data), "After rename.") && !strings.Contains(string(data), "Reference content.") {
						t.Errorf("%s kept stale content: %s", current, data)
					}
					if !sameFile {
						if _, err := os.Lstat(old); !os.IsNotExist(err) {
							t.Errorf("old output %s remains: %v", old, err)
						}
					}
					if slices.Contains(state.Outputs, old) || !slices.Contains(state.Outputs, current) {
						t.Errorf("ledger did not replace %s with %s: %v", old, current, state.Outputs)
					}
					if slices.Contains(state.Orphans, old) || slices.Contains(state.Orphans, current) {
						t.Errorf("renamed output became an orphan: %v", state.Orphans)
					}
				}
				runSyncOK(t, "--check")
			})
		}
	}
}

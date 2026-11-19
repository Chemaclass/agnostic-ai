package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func entryPointVarsProject(t *testing.T, targets string) string {
	t.Helper()
	testutil.TempCwd(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+targets+"]\n")
	writeAgnosticFile(t, "# S\n")
	rule := filepath.Join(".agnostic-ai", "rules", "push.md")
	writeFile(t, rule, "---\nname: push\n---\n\nSkills live in {{$SKILLS_DIR}}.\n")
	return rule
}

func syncNotes(t *testing.T, args ...string) []string {
	t.Helper()
	var warned bytes.Buffer
	adapters.ResetCoverageNotes()
	adapters.SetWarner(&warned)
	t.Cleanup(func() { adapters.ResetCoverageNotes(); adapters.SetWarner(os.Stderr) })
	if out, err := runCLI(t, append([]string{"sync"}, args...)...); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	var notes []string
	for _, line := range strings.Split(warned.String(), "\n") {
		if strings.Contains(line, "note:") {
			notes = append(notes, line)
		}
	}
	return notes
}

func TestRender_EntryPointRuleExpandsItsOnlyReadersPaths(t *testing.T) {
	rule := entryPointVarsProject(t, "codex, gemini")
	for target, want := range map[string]string{
		"codex":  "Skills live in .agents/skills.",
		"gemini": "Skills live in .gemini/skills.",
	} {
		out, err := runCLI(t, "render", rule, "-t", target)
		if err != nil {
			t.Fatalf("render %s: %v\n%s", target, err, out)
		}
		if !strings.Contains(out, want) {
			t.Errorf("%s: want %q in:\n%s", target, want, out)
		}
	}
}

func TestSync_SharedAGENTSMdExpandsAVariableEveryReaderAgreesOn(t *testing.T) {
	entryPointVarsProject(t, "codex, amp")

	for _, note := range syncNotes(t) {
		if strings.Contains(note, "{{$SKILLS_DIR}}") {
			t.Errorf("no note expected when every reader agrees: %s", note)
		}
	}

	if got := readFile(t, "AGENTS.md"); !strings.Contains(got, "Skills live in .agents/skills.") {
		t.Errorf("want .agents/skills in AGENTS.md:\n%s", got)
	}
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check: %v\n%s", err, out)
	}
}

func TestSync_SharedAGENTSMdKeepsAVariableItsReadersResolveApart(t *testing.T) {
	entryPointVarsProject(t, "codex, opencode")

	notes := syncNotes(t)

	if got := readFile(t, "AGENTS.md"); !strings.Contains(got, "Skills live in {{$SKILLS_DIR}}.") {
		t.Errorf("want the token verbatim in AGENTS.md:\n%s", got)
	}
	var matched []string
	for _, note := range notes {
		if strings.Contains(note, "{{$SKILLS_DIR}}") {
			matched = append(matched, note)
		}
	}
	if len(matched) != 1 || !strings.Contains(matched[0], "AGENTS.md") {
		t.Errorf("want one note naming AGENTS.md, got:\n%s", strings.Join(notes, "\n"))
	}
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check: %v\n%s", err, out)
	}
}

// Kilo loads AGENTS.md in every session, so once the inlined copy
// carries Kilo's own path the rule loads from there alone.
func TestSync_KiloSkipsARuleWhoseVariableAGENTSMdNowExpands(t *testing.T) {
	entryPointVarsProject(t, "codex, kilo")
	syncNotes(t)

	if got := readFile(t, "AGENTS.md"); !strings.Contains(got, "Skills live in .agents/skills.") {
		t.Errorf("want .agents/skills in AGENTS.md:\n%s", got)
	}
	if exists(filepath.Join(".kilo", "rules", "push.md")) {
		t.Error(".kilo/rules/push.md loads the rule a second time next to AGENTS.md")
	}
}

func sharedDocVarsProject(t *testing.T, targets string) {
	t.Helper()
	testutil.TempCwd(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+targets+"]\n")
	writeAgnosticFile(t, "# S\n")
	writeFile(t, filepath.Join("services", "api", "main.go"), "package main\n")
	writeFile(t, filepath.Join(".agnostic-ai", "reviews", "paths.md"), "---\nname: paths\n---\n\nFlag skills added outside {{$SKILLS_DIR}}.\n")
	writeFile(t, filepath.Join(".agnostic-ai", "rules", "api.md"), "---\nname: api\nscope: services/api\n---\n\nAPI skills live in {{$SKILLS_DIR}}.\n")
}

func notesNaming(notes []string, file string) []string {
	var out []string
	for _, note := range notes {
		if strings.Contains(note, "{{$SKILLS_DIR}}") && strings.Contains(note, "note: "+file+":") {
			out = append(out, note)
		}
	}
	return out
}

func TestSync_SharedDocumentsExpandAVariableEveryReaderAgreesOn(t *testing.T) {
	sharedDocVarsProject(t, "codex, amp")

	notes := syncNotes(t)

	if got := readFile(t, "AGENTS.md"); !strings.Contains(got, "Flag skills added outside .agents/skills.") {
		t.Errorf("want .agents/skills in the root review section:\n%s", got)
	}
	if got := readFile(t, filepath.Join("services", "api", "AGENTS.md")); !strings.Contains(got, "API skills live in .agents/skills.") {
		t.Errorf("want .agents/skills in the nested AGENTS.md:\n%s", got)
	}
	for _, note := range notes {
		if strings.Contains(note, "{{$SKILLS_DIR}}") {
			t.Errorf("no note expected when every reader agrees: %s", note)
		}
	}
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check: %v\n%s", err, out)
	}
}

func TestSync_SharedDocumentsKeepAVariableTheirReadersResolveApart(t *testing.T) {
	sharedDocVarsProject(t, "codex, opencode")

	notes := syncNotes(t)

	if got := readFile(t, "AGENTS.md"); !strings.Contains(got, "Flag skills added outside {{$SKILLS_DIR}}.") {
		t.Errorf("want the token verbatim in the root review section:\n%s", got)
	}
	if got := readFile(t, filepath.Join("services", "api", "AGENTS.md")); !strings.Contains(got, "API skills live in {{$SKILLS_DIR}}.") {
		t.Errorf("want the token verbatim in the nested AGENTS.md:\n%s", got)
	}
	if n := notesNaming(notes, "AGENTS.md"); len(n) != 1 {
		t.Errorf("want one note naming AGENTS.md, got:\n%s", strings.Join(notes, "\n"))
	}
	if n := notesNaming(notes, "services/api/AGENTS.md"); len(n) != 1 {
		t.Errorf("want one note naming services/api/AGENTS.md, got:\n%s", strings.Join(notes, "\n"))
	}
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check: %v\n%s", err, out)
	}
}

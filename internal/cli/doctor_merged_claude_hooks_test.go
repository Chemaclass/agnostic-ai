package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const mergedHookHandWritten = `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[
{"type":"command","command":"a.sh","timeout":10},
{"type":"command","command":"b.sh"}
]}]}}`

const mergedHookSpec = "name: bash-a\ndescription: Run a.sh and b.sh before Bash.\nevent: PreToolUse\nmatcher: Bash\ncommand: [a.sh, b.sh]\ntimeout: 10\n"

func TestDoctor_NamesHookSpecMergedBeforeHandlerSettingsWereKept(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	writeFile(t, ".claude/settings.json", mergedHookHandWritten)
	writeFile(t, ".agnostic-ai/hooks/bash-a.yaml", mergedHookSpec)
	execCLI(t, "sync", "-t", "claude")

	out, _ := runCLI(t, "doctor")
	if !strings.Contains(out, ".agnostic-ai/hooks/bash-a.yaml") || !strings.Contains(out, "run: agnostic-ai sync, then: agnostic-ai import claude") {
		t.Errorf("doctor output does not name the merged spec and the fix:\n%s", out)
	}
}

func TestDoctor_StaysQuietOnHookSpecsImportedPerHandler(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	writeFile(t, ".claude/settings.json", mergedHookHandWritten)
	execCLI(t, "import", "claude")
	execCLI(t, "sync", "-t", "claude")

	out, _ := runCLI(t, "doctor")
	if strings.Contains(out, "agnostic-ai import claude") {
		t.Errorf("doctor flagged a clean import:\n%s", out)
	}
}

func TestDoctor_MergedHookSpecAdviceLeavesEachCommandOnce(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	writeFile(t, ".claude/settings.json", mergedHookHandWritten)
	writeFile(t, ".agnostic-ai/hooks/bash-a.yaml", mergedHookSpec)
	execCLI(t, "sync", "-t", "claude")

	if err := os.Remove(".agnostic-ai/hooks/bash-a.yaml"); err != nil {
		t.Fatal(err)
	}
	execCLI(t, "sync")
	execCLI(t, "import", "claude")
	execCLI(t, "sync")

	runs := map[string]int{}
	for _, g := range claudePreToolUseGroups(t, readFile(t, ".claude/settings.json")) {
		for _, h := range g.(map[string]any)["hooks"].([]any) {
			runs[h.(map[string]any)["command"].(string)]++
		}
	}
	if runs["a.sh"] != 1 || runs["b.sh"] != 1 {
		t.Errorf("runs per command = %v, want a.sh and b.sh once", runs)
	}
	if out, _ := runCLI(t, "doctor"); strings.Contains(out, "agnostic-ai import claude") {
		t.Errorf("doctor still flags a spec after following its advice:\n%s", out)
	}
}

func TestDoctor_StaysQuietWhenTwoHookSpecsShareACommand(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	writeFile(t, ".agnostic-ai/hooks/h1.yaml", "name: h1\ndescription: Run a.sh and b.sh before Bash.\nevent: PreToolUse\nmatcher: Bash\ncommand: [a.sh, b.sh]\ntimeout: 10\n")
	writeFile(t, ".agnostic-ai/hooks/h2.yaml", "name: h2\ndescription: Run a.sh before Bash.\nevent: PreToolUse\nmatcher: Bash\ncommand: a.sh\ntimeout: 5\n")
	execCLI(t, "sync", "-t", "claude")

	if out, _ := runCLI(t, "doctor"); strings.Contains(out, "agnostic-ai import claude") {
		t.Errorf("doctor flagged specs that share a command:\n%s", out)
	}
}

func TestDoctor_NamesMergedHookSpecByItsClaudeMatcher(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	writeFile(t, ".claude/settings.json", mergedHookHandWritten)
	writeFile(t, ".agnostic-ai/hooks/bash-a.yaml", "name: bash-a\ndescription: Run a.sh and b.sh before Bash.\nevent: PreToolUse\nmatcher: Write\nx-claude:\n  matcher: Bash\ncommand: [a.sh, b.sh]\ntimeout: 10\n")
	execCLI(t, "sync", "-t", "claude")

	if out, _ := runCLI(t, "doctor"); !strings.Contains(out, ".agnostic-ai/hooks/bash-a.yaml") {
		t.Errorf("doctor does not name the spec by its Claude matcher:\n%s", out)
	}
}

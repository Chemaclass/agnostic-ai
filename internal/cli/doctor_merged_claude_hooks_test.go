package cli

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const mergedHookHandWritten = `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[
{"type":"command","command":"a.sh","timeout":10},
{"type":"command","command":"b.sh"}
]}]}}`

func TestDoctor_NamesHookSpecMergedBeforeHandlerSettingsWereKept(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	writeFile(t, ".claude/settings.json", mergedHookHandWritten)
	writeFile(t, ".agnostic-ai/hooks/bash-a.yaml", "name: bash-a\ndescription: Run a.sh and b.sh before Bash.\nevent: PreToolUse\nmatcher: Bash\ncommand: [a.sh, b.sh]\ntimeout: 10\n")
	execCLI(t, "sync", "-t", "claude")

	out, _ := runCLI(t, "doctor")
	if !strings.Contains(out, ".agnostic-ai/hooks/bash-a.yaml") || !strings.Contains(out, "agnostic-ai import claude") {
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

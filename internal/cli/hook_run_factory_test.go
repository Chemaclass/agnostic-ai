package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// factoryGuardScript blocks a recursive delete: through Factory's
// documented PreToolUse deny reply on its Execute tool, and through exit
// 2 elsewhere.
const factoryGuardScript = `#!/bin/sh
payload=$(cat)
case "$payload" in
  *'rm -rf'*) ;;
  *) exit 0 ;;
esac
case "$payload" in
  *'"tool_name":"Execute"'*)
    echo '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"no recursive delete"}}'
    exit 0 ;;
esac
echo "no recursive delete" >&2
exit 2
`

// factoryProject syncs a claude+factory project with one hook spec whose
// command runs body.
func factoryProject(t *testing.T, hook, body string) {
	t.Helper()
	dir := testutil.TempCwd(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, factory]\n")
	script := filepath.Join(dir, ".agnostic-ai", "scripts", "protect-files.sh")
	mustWrite(t, script, body)
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "protect-files.yaml"), hook)
	mustSync(t)
}

const factoryHookSpec = "name: protect-files\nevent: PreToolUse\ncommand: .agnostic-ai/scripts/protect-files.sh\n"

func TestHookRun_FactoryRunsOnAnAssumedShellAndCountsOnlyWhenAsked(t *testing.T) {
	skipWithoutPOSIXShell(t)
	factoryProject(t, factoryHookSpec, factoryGuardScript)

	out, err := runHookRun(t, "protect-files", "--bash", "rm -rf /", "--expect", "block")
	for _, want := range []string{
		"claude: block (exit 2",
		"factory: block (exit 0", "(assumed: shell, working directory)", "event: PreToolUse (Execute)",
		"assumed shell: sh -c (Factory does not document the shell that runs a hook command)",
		"assumed working directory: project root (Factory runs hooks from \"Droid's current working directory, which can differ from your repository root\")",
		"docs: https://docs.factory.com/cli/configuration/hooks-guide",
		"1 checked, 1 assumed (not counted; --include-assumed to count)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output misses %q:\n%s", want, out)
		}
	}
	if err != nil {
		t.Errorf("the counted claude result passes --expect: %v", err)
	}
	if strings.Contains(out, "warning:") || strings.Contains(out, "assumed timeout") {
		t.Errorf("a fresh sync warns, or the documented 60s timeout is marked assumed:\n%s", out)
	}

	out, err = runHookRun(t, "protect-files", "--bash", "rm -rf /", "--expect", "block", "--include-assumed")
	if err != nil || !strings.Contains(out, "2 checked, 1 assumed (counted)") {
		t.Errorf("--include-assumed must count factory: %v\n%s", err, out)
	}

	out, _ = runHookRun(t, "protect-files", "--bash", "rm -rf /", "--expect", "allow")
	if !strings.Contains(out, "factory: warning: assumed result block differs from allow and is not counted") {
		t.Errorf("an uncounted disagreement must warn:\n%s", out)
	}
}

func TestHookRun_FactoryDisagreementWithTheCountedTargetsWarns(t *testing.T) {
	skipWithoutPOSIXShell(t)
	factoryProject(t, factoryHookSpec, "#!/bin/sh\ncat >/dev/null\n[ -n \"$FACTORY_PROJECT_DIR\" ] && exit 0\nexit 2\n")

	out, err := runHookRun(t, "protect-files", "--bash", "ls")
	if err != nil || !strings.Contains(out, "warning: assumed result allow differs from block and is not counted") {
		t.Errorf("factory allows where claude blocks; the run must warn and pass: %v\n%s", err, out)
	}
	if _, err := runHookRun(t, "protect-files", "--bash", "ls", "--include-assumed"); err == nil {
		t.Error("with --include-assumed the disagreement must fail the run")
	}
}

func TestHookRun_FactorySkipsAnInlineShellCommand(t *testing.T) {
	skipWithoutPOSIXShell(t)
	factoryProject(t, "name: protect-files\nevent: PreToolUse\ncommand: 'cat | grep -q rm && exit 2'\n", factoryGuardScript)

	out, _ := runHookRun(t, "protect-files", "--bash", "rm -rf /")
	if !strings.Contains(out, "factory: not run (Factory does not document its shell; use a script path)") {
		t.Errorf("output = %s", out)
	}
}

func TestHookRun_FactoryRefusesAnUndocumentedEditInput(t *testing.T) {
	skipWithoutPOSIXShell(t)
	factoryProject(t, "name: protect-files\nevent: PreToolUse\nmatcher: Edit\ncommand: .agnostic-ai/scripts/protect-files.sh\n", factoryGuardScript)

	out, _ := runHookRun(t, "protect-files", "--edit", "a.go")
	if !strings.Contains(out, "factory: not run (Factory documents no tool_input for its Edit and ApplyPatch tools") || !strings.Contains(out, "claude: ") {
		t.Errorf("--edit on Edit must list Factory as not run and still run the others:\n%s", out)
	}
}

func TestHookRun_FactoryRunsTheDocumentedProjectRootForm(t *testing.T) {
	skipWithoutPOSIXShell(t)
	dir := filepath.Join(t.TempDir(), "it's a dir")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, dir)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [factory]\n")
	script := filepath.Join(dir, ".factory", "hooks", "x.sh")
	mustWrite(t, script, factoryGuardScript)
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "x.yaml"), "name: x\nevent: PreToolUse\ncommand: '\"$FACTORY_PROJECT_DIR\"/.factory/hooks/x.sh'\n")
	mustSync(t)

	out, err := runHookRun(t, "x", "--bash", "rm -rf /", "--include-assumed", "--expect", "block")
	if err != nil || !strings.Contains(out, "factory: block (exit 0") || !strings.Contains(out, `command: "$FACTORY_PROJECT_DIR"/.factory/hooks/x.sh`) {
		t.Errorf("the guide's \"$FACTORY_PROJECT_DIR\" form must run from a root with a space and a quote: %v\n%s", err, out)
	}
}

func TestHookRun_FactoryJSONListsItsAssumptions(t *testing.T) {
	skipWithoutPOSIXShell(t)
	factoryProject(t, factoryHookSpec, factoryGuardScript)

	out, _ := runHookRun(t, "protect-files", "--edit", "a.go", "--format", "json")
	var report struct {
		Targets []struct {
			Target      string `json:"target"`
			Decision    string `json:"decision"`
			Trigger     string `json:"trigger"`
			Counted     bool   `json:"counted"`
			Assumptions []struct {
				Item, Value, Reason string
			} `json:"assumptions"`
		} `json:"targets"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	for _, r := range report.Targets {
		if r.Target != "factory" {
			continue
		}
		if r.Decision != "allow" || r.Trigger != "Create" || r.Counted || len(r.Assumptions) != 2 || r.Assumptions[0].Item != "shell" || r.Assumptions[0].Value != "sh -c" || r.Assumptions[1].Item != "working directory" || r.Assumptions[1].Value != "project root" {
			t.Errorf("factory = %+v; want allow on Create, not counted, shell and cwd assumed", r)
		}
		return
	}
	t.Fatalf("no factory result:\n%s", out)
}

package hookrun

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestDecideFactory_FollowsTheHooksGuide(t *testing.T) {
	for name, tc := range map[string]struct {
		event string
		r     Result
		want  Decision
	}{
		"exit 2 blocks a tool call":                {"PreToolUse", Result{Exit: 2}, Block},
		"exit 2 blocks a prompt":                   {"UserPromptSubmit", Result{Exit: 2}, Block},
		"exit 2 feeds Stop back":                   {"Stop", Result{Exit: 2}, Block},
		"exit 2 only surfaces on SubagentStop":     {"SubagentStop", Result{Exit: 2}, Error},
		"exit 2 only surfaces on SessionStart":     {"SessionStart", Result{Exit: 2}, Error},
		"another exit is a non-blocking error":     {"PreToolUse", Result{Exit: 1}, Error},
		"deny blocks a tool call":                  {"PreToolUse", Result{Stdout: `{"hookSpecificOutput":{"permissionDecision":"deny"}}`}, Block},
		"ask blocks until the user answers":        {"PreToolUse", Result{Stdout: `{"hookSpecificOutput":{"permissionDecision":"ask"}}`}, Block},
		"allow allows":                             {"PreToolUse", Result{Stdout: `{"hookSpecificOutput":{"permissionDecision":"allow"}}`}, Allow},
		"decision block blocks SubagentStop":       {"SubagentStop", Result{Stdout: `{"decision":"block"}`}, Block},
		"decision block is not a PreToolUse reply": {"PreToolUse", Result{Stdout: `{"decision":"block"}`}, Allow},
		"continue false stops":                     {"PostToolUse", Result{Stdout: `{"continue":false}`}, Block},
		"SessionEnd cannot block":                  {"SessionEnd", Result{Stdout: `{"continue":false}`}, Allow},
		"plain stdout allows":                      {"UserPromptSubmit", Result{Stdout: "context"}, Allow},
		"a timeout":                                {"PreToolUse", Result{TimedOut: true}, Timeout},
	} {
		t.Run(name, func(t *testing.T) {
			if got := decideFactory(tc.event, tc.r); got != tc.want {
				t.Errorf("decideFactory = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestBuildFactory_WritesTheDocumentedPayloads(t *testing.T) {
	p, err := buildFactory("PreToolUse", "Execute", "/project", Input{Bash: "ls"})
	if err != nil || !p.Fires || p.Trigger != "Execute" {
		t.Fatalf("--bash calls Execute: %+v %v", p, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(p.Body, &doc); err != nil || doc["cwd"] != "/project" || doc["permission_mode"] != "off" || doc["tool_input"].(map[string]any)["command"] != "ls" {
		t.Errorf("payload = %s", p.Body)
	}
	p, err = buildFactory("PostToolUse", "Create|Edit|ApplyPatch", "/project", Input{Edit: "a.go"})
	if err != nil || !p.Fires || p.Trigger != "Create" {
		t.Fatalf("--edit calls Create, whose input is documented: %+v %v", p, err)
	}
	if err := json.Unmarshal(p.Body, &doc); err != nil || doc["tool_input"].(map[string]any)["file_path"] != filepath.Join("/project", "a.go") || doc["tool_response"] == nil {
		t.Errorf("payload = %s", p.Body)
	}
	if _, err := buildFactory("PreToolUse", "ApplyPatch", "/project", Input{Edit: "a.go"}); err == nil {
		t.Error("--edit on ApplyPatch must be refused: its input is undocumented")
	}
	if p, _ := buildFactory("PreToolUse", "Exec", "/project", Input{Bash: "ls"}); p.Fires {
		t.Error("an exact matcher names one tool")
	}
	if p, _ := buildFactory("SessionStart", "resume", "/project", Input{}); !p.Fires || p.Trigger != "resume" {
		t.Errorf("a SessionStart matcher picks its source: %+v", p)
	}
}

func TestAssumptions_FactoryAssumesTheShellAndCwd(t *testing.T) {
	if _, reason := Assumptions("factory", "windows", Handler{Command: ".factory/hooks/a.sh"}); reason == "" {
		t.Error("Windows must not run: the shell is undocumented")
	}
	got, reason := Assumptions("factory", "linux", Handler{Command: ".factory/hooks/a.sh"})
	if reason != "" || len(got) != 2 || got[0].Item != "shell" || got[1].Item != "working directory" {
		t.Errorf("Assumptions = %+v %q; want the shell and cwd, not the documented 60s timeout", got, reason)
	}
	for _, command := range []string{`"$FACTORY_PROJECT_DIR"/.factory/hooks/a.sh`, `${FACTORY_PROJECT_DIR}/.factory/hooks/a.sh --strict`} {
		if _, reason := Assumptions("factory", "linux", Handler{Command: command}); reason != "" {
			t.Errorf("%s must run: %s", command, reason)
		}
	}
	if DefaultTimeout("factory", "PreToolUse") != 60*time.Second {
		t.Error("Factory's documented default is 60 seconds")
	}
}

func TestFactoryRoot_RunsAsTheShellExpandsIt(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	for _, root := range []string{"/tmp/a b", "/tmp/it's", "/tmp/$(printf INJECTED)", "/tmp/`printf INJECTED`", "/tmp/project with spaces"} {
		for command, want := range map[string]string{
			`printf %s "$FACTORY_PROJECT_DIR"`:     root,
			`printf %s "${FACTORY_PROJECT_DIR}/x"`: root + "/x",
			`printf %s '$FACTORY_PROJECT_DIR'`:     "$FACTORY_PROJECT_DIR",
			`printf %s \$FACTORY_PROJECT_DIR`:      "$FACTORY_PROJECT_DIR",
			`printf %s $FACTORY_PROJECT_DIRS`:      "",
		} {
			if got := ExpandCommand("factory", "linux", command, root); got != command {
				t.Errorf("the command must run as written: %s became %s", command, got)
			}
			cmd := exec.Command("sh", "-c", command)
			cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "FACTORY_PROJECT_DIR=" + root}
			out, err := cmd.Output()
			if err != nil || string(out) != want {
				t.Errorf("root %q: %s printed %q, want %q (%v)", root, command, out, want, err)
			}
		}
	}
	for command, runs := range map[string]bool{
		`"$FACTORY_PROJECT_DIR"/.factory/hooks/guard.sh`:  true,
		`${FACTORY_PROJECT_DIR}/.factory/hooks/guard.sh`:  true,
		`.factory/hooks/guard.sh '$FACTORY_PROJECT_DIR'`:  true,
		`"$FACTORY_PROJECT_DIR/.factory/hooks/guard.sh"`:  false,
		`$FACTORY_PROJECT_DIRS/.factory/hooks/guard.sh`:   false,
		`"$FACTORY_PROJECT_DIR"/guard.sh | tee /dev/null`: false,
	} {
		if _, reason := Assumptions("factory", "linux", Handler{Command: command}); (reason == "") != runs {
			t.Errorf("%s: runs = %t, reason %q", command, !runs, reason)
		}
	}
}

func TestFactoryRoot_UnquotedReferenceSplitsAsNativeSh(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	const command = `set -- $FACTORY_PROJECT_DIR; printf %s "$#"`
	argv := Argv("factory", "linux", Handler{Command: ExpandCommand("factory", "linux", command, "/tmp/project with spaces")})
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "FACTORY_PROJECT_DIR=/tmp/project with spaces"}
	if out, err := cmd.Output(); err != nil || string(out) != "3" {
		t.Errorf("an unquoted root splits into 3 words under sh, got %q (%v)", out, err)
	}
}

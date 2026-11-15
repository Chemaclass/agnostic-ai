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
	deny := `{"hookSpecificOutput":{"permissionDecision":"deny"}}`
	for name, tc := range map[string]struct {
		event string
		r     Result
		want  Decision
	}{
		"exit 2 blocks a tool call":                    {"PreToolUse", Result{Exit: 2}, Block},
		"exit 2 blocks a prompt":                       {"UserPromptSubmit", Result{Exit: 2}, Block},
		"exit 2 feeds Stop back":                       {"Stop", Result{Exit: 2}, Block},
		"exit 2 blocks a manual compaction":            {"PreCompact", Result{Exit: 2}, Block},
		"exit 2 cannot block SubagentStop":             {"SubagentStop", Result{Exit: 2}, Error},
		"exit 2 only surfaces on SessionStart":         {"SessionStart", Result{Exit: 2}, Error},
		"exit 2 ignores the JSON on Notification":      {"Notification", Result{Exit: 2, Stdout: `{"continue":false}`}, Error},
		"another exit with plain stdout is an error":   {"PreToolUse", Result{Exit: 1, Stdout: "oops"}, Error},
		"another exit with a JSON deny denies":         {"PreToolUse", Result{Exit: 1, Stdout: deny}, Block},
		"another exit with a JSON allow allows":        {"PreToolUse", Result{Exit: 3, Stdout: `{}`}, Allow},
		"deny blocks a tool call":                      {"PreToolUse", Result{Stdout: deny}, Block},
		"ask leaves normal permissions":                {"PreToolUse", Result{Stdout: `{"hookSpecificOutput":{"permissionDecision":"ask"}}`}, Allow},
		"allow allows":                                 {"PreToolUse", Result{Stdout: `{"hookSpecificOutput":{"permissionDecision":"allow"}}`}, Allow},
		"top-level decision block denies a tool call":  {"PreToolUse", Result{Stdout: `{"decision":"block"}`}, Block},
		"permissionDecision wins over decision":        {"PreToolUse", Result{Stdout: `{"decision":"block","hookSpecificOutput":{"permissionDecision":"allow"}}`}, Allow},
		"decision approve allows":                      {"PreToolUse", Result{Stdout: `{"decision":"approve"}`}, Allow},
		"decision block feeds PostToolUse back":        {"PostToolUse", Result{Stdout: `{"decision":"block"}`}, Block},
		"decision block keeps Stop running":            {"Stop", Result{Stdout: `{"decision":"block"}`}, Block},
		"decision block blocks a manual compaction":    {"PreCompact", Result{Stdout: `{"decision":"block"}`}, Block},
		"decision block does not stop a subagent":      {"SubagentStop", Result{Stdout: `{"decision":"block"}`}, Allow},
		"continue false ends the turn before the tool": {"PreToolUse", Result{Stdout: `{"continue":false}`}, Block},
		"continue false blocks a prompt":               {"UserPromptSubmit", Result{Stdout: `{"continue":false}`}, Block},
		"continue false lets Stop stop":                {"Stop", Result{Stdout: `{"continue":false}`}, Allow},
		"continue false does not stop a subagent":      {"SubagentStop", Result{Stdout: `{"continue":false}`}, Allow},
		"continue false stops nothing on Notification": {"Notification", Result{Stdout: `{"continue":false}`}, Allow},
		"continue false stops nothing on SessionStart": {"SessionStart", Result{Stdout: `{"continue":false}`}, Allow},
		"continue false stops nothing on SessionEnd":   {"SessionEnd", Result{Stdout: `{"continue":false}`}, Allow},
		"plain stdout allows":                          {"UserPromptSubmit", Result{Stdout: "context"}, Allow},
		"an object with no output field is plain text": {"PreToolUse", Result{Stdout: `{"message":"done"}`}, Allow},
		"JSON that does not parse is an error":         {"PreToolUse", Result{Stdout: `{"continue":}`}, Error},
		"an unknown permissionDecision is an error":    {"PreToolUse", Result{Stdout: `{"hookSpecificOutput":{"permissionDecision":"defer"}}`}, Error},
		"a field of the wrong type is an error":        {"PreToolUse", Result{Stdout: `{"continue":"no"}`}, Error},
		"another event's hookEventName is an error":    {"PreToolUse", Result{Stdout: `{"hookSpecificOutput":{"hookEventName":"PostToolUse","permissionDecision":"deny"}}`}, Error},
		"a null field counts as absent":                {"PreToolUse", Result{Stdout: `{"reason":null,"hookSpecificOutput":{"permissionDecision":"deny"}}`}, Block},
		"a timeout":                                    {"PreToolUse", Result{TimedOut: true}, Timeout},
	} {
		t.Run(name, func(t *testing.T) {
			if got := decideFactory(tc.event, Handler{}, tc.r); got != tc.want {
				t.Errorf("decideFactory = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestDecideFactory_OnFailureBlock(t *testing.T) {
	h := Handler{FailClosed: true}
	for name, tc := range map[string]struct {
		event string
		r     Result
		want  Decision
	}{
		"a timeout blocks":                         {"PreToolUse", Result{TimedOut: true}, Block},
		"a start failure blocks":                   {"PostToolUse", Result{StartErr: os.ErrNotExist}, Block},
		"another exit blocks":                      {"UserPromptSubmit", Result{Exit: 1}, Block},
		"another exit blocks even with JSON allow": {"PreToolUse", Result{Exit: 1, Stdout: `{}`}, Block},
		"invalid JSON blocks":                      {"PreCompact", Result{Stdout: `{"continue":}`}, Block},
		"exit 0 with an allow still allows":        {"PreToolUse", Result{Stdout: `{"hookSpecificOutput":{"permissionDecision":"allow"}}`}, Allow},
		"no effect on Stop":                        {"Stop", Result{Exit: 1}, Error},
		"no effect on SubagentStop":                {"SubagentStop", Result{TimedOut: true}, Timeout},
		"no effect on SessionStart":                {"SessionStart", Result{Exit: 1}, Error},
	} {
		t.Run(name, func(t *testing.T) {
			if got := decideFactory(tc.event, h, tc.r); got != tc.want {
				t.Errorf("decideFactory = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestFactoryAsks_OnlyAnAppliedPreToolUseAsk(t *testing.T) {
	ask := `{"hookSpecificOutput":{"permissionDecision":"ask"}}`
	if !FactoryAsks("PreToolUse", Result{Exit: 1, Stdout: ask}) {
		t.Error("a JSON ask applies on any exit but 2")
	}
	if FactoryAsks("PreToolUse", Result{Exit: 2, Stdout: ask}) || FactoryAsks("PostToolUse", Result{Stdout: ask}) {
		t.Error("exit 2 and other events carry no ask")
	}
}

func TestHandlersFromDoc_FactoryReadsOnFailure(t *testing.T) {
	handlers, err := HandlersFromDoc("factory", []byte(`{"PreToolUse":[{"matcher":"Execute","hooks":[{"type":"command","command":"./a.sh","onFailure":"block"},{"type":"command","command":"./b.sh"}]}]}`))
	if err != nil || len(handlers) != 2 || !handlers[0].FailClosed || handlers[1].FailClosed {
		t.Errorf("handlers = %+v %v; want only ./a.sh fail closed", handlers, err)
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

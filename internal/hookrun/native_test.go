package hookrun

import (
	"reflect"
	"testing"
)

func anyCover(string, string) bool { return true }

func driftHandlers(d []HandlerDrift) []Handler {
	var out []Handler
	for _, x := range d {
		out = append(out, x.Handler)
	}
	return out
}

func TestDrift_ReturnsTheHandlersTheNativeEventLacks(t *testing.T) {
	native := []byte(`{"hooks":{
		"PreToolUse":[{"matcher":"Edit","hooks":[{"type":"command","command":"old.sh"}]},
			{"matcher":"Bash","hooks":[{"type":"command","command":"kept.sh","args":["-v"]}]}],
		"PostToolUse":[{"hooks":[{"type":"command","command":"new.sh"}]}]}}`)
	want := []Handler{{Command: "kept.sh", Args: []string{"-v"}}, {Command: "new.sh"}, {Command: "kept.sh"}}

	got, err := Drift("claude", native, "PreToolUse", "", "linux", want, anyCover)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(driftHandlers(got), want[1:]) {
		t.Errorf("Drift = %+v, want new.sh (other event) and kept.sh without its args", got)
	}
}

func TestDrift_ComparesTheWindowsCommandOnlyOnWindows(t *testing.T) {
	native := []byte(`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"a.sh","commandWindows":"a.ps1"}]}]}}`)
	handlers := []Handler{{Command: "a.sh", CommandWindows: "b.ps1"}}

	if got, err := Drift("codex", native, "Stop", "", "windows", handlers, anyCover); err != nil || len(got) != 1 {
		t.Errorf("windows: Drift = %+v, %v, want the changed commandWindows reported", got, err)
	}
	if got, err := Drift("codex", native, "Stop", "", "darwin", handlers, anyCover); err != nil || len(got) != 0 {
		t.Errorf("darwin: Drift = %+v, %v, want nothing: Codex runs command there", got, err)
	}
}

func TestDrift_CountsOnlyCommandHandlers(t *testing.T) {
	for _, handler := range []string{`{"command":"a.sh"}`, `{"type":"prompt","command":"a.sh"}`} {
		native := []byte(`{"hooks":{"Stop":[{"hooks":[` + handler + `]}]}}`)

		if got, err := Drift("claude", native, "Stop", "", "linux", []Handler{{Command: "a.sh"}}, anyCover); err != nil || len(got) != 1 {
			t.Errorf("%s: Drift = %+v, %v, want a handler that is no command hook reported", handler, got, err)
		}
	}
}

func TestDrift_RejectsAFileThatIsNoJSON(t *testing.T) {
	if _, err := Drift("claude", []byte("{"), "Stop", "", "linux", []Handler{{Command: "a.sh"}}, anyCover); err == nil {
		t.Error("err = nil, want a parse error")
	}
}

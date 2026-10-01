package hookrun

import (
	"reflect"
	"testing"
)

func TestUnsynced_ReturnsTheHandlersTheNativeEventLacks(t *testing.T) {
	native := []byte(`{"hooks":{
		"PreToolUse":[{"matcher":"Edit","hooks":[{"type":"command","command":"old.sh"}]},
			{"matcher":"Bash","hooks":[{"type":"command","command":"kept.sh","args":["-v"]}]}],
		"PostToolUse":[{"hooks":[{"type":"command","command":"new.sh"}]}]}}`)
	want := []Handler{{Command: "kept.sh", Args: []string{"-v"}}, {Command: "new.sh"}, {Command: "kept.sh"}}

	got, err := Unsynced(native, "PreToolUse", "linux", want)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want[1:]) {
		t.Errorf("Unsynced = %+v, want new.sh (other event) and kept.sh without its args", got)
	}
}

func TestUnsynced_ComparesTheWindowsCommandOnlyOnWindows(t *testing.T) {
	native := []byte(`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"a.sh","commandWindows":"a.ps1"}]}]}}`)
	handlers := []Handler{{Command: "a.sh", CommandWindows: "b.ps1"}}

	if got, err := Unsynced(native, "Stop", "windows", handlers); err != nil || len(got) != 1 {
		t.Errorf("windows: Unsynced = %+v, %v, want the changed commandWindows reported", got, err)
	}
	if got, err := Unsynced(native, "Stop", "darwin", handlers); err != nil || len(got) != 0 {
		t.Errorf("darwin: Unsynced = %+v, %v, want nothing: Codex runs command there", got, err)
	}
}

func TestUnsynced_RejectsAFileThatIsNoJSON(t *testing.T) {
	if _, err := Unsynced([]byte("{"), "Stop", "linux", []Handler{{Command: "a.sh"}}); err == nil {
		t.Error("err = nil, want a parse error")
	}
}

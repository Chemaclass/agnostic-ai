package hookrun

import (
	"strings"
	"testing"
	"time"
)

func exactCover(native, spec string) bool { return native == spec }

func TestDrift_NamesTheMatcherTimeoutAndEnvThatDiffer(t *testing.T) {
	native := []byte(`{"hooks":{"BeforeTool":[{"matcher":"write_file","hooks":[
		{"type":"command","command":"a.sh","timeout":5000,"env":{"AGNOSTIC_AI_TARGET":"gemini"}}]}]}}`)
	base := Handler{Command: "a.sh", Timeout: 5 * time.Second, Env: map[string]string{"AGNOSTIC_AI_TARGET": "gemini"}}

	for _, tc := range []struct {
		name, matcher string
		h             Handler
		want          string
	}{
		{"in sync", "write_file", base, ""},
		{"matcher", "replace", base, `matcher "write_file"`},
		{"timeout", "write_file", Handler{Command: "a.sh", Timeout: 9 * time.Second, Env: base.Env}, "timeout 5s"},
		{"no timeout", "write_file", Handler{Command: "a.sh", Env: base.Env}, "timeout 5s"},
		{"env", "write_file", Handler{Command: "a.sh", Timeout: 5 * time.Second, Env: map[string]string{"AGNOSTIC_AI_TARGET": "gemini", "X": "1"}}, "env"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			drift, err := Drift("gemini", native, "BeforeTool", tc.matcher, "linux", []Handler{tc.h}, exactCover)
			if err != nil {
				t.Fatal(err)
			}
			switch {
			case tc.want == "" && len(drift) != 0:
				t.Errorf("Drift = %+v, want none", drift)
			case tc.want != "" && (len(drift) != 1 || !strings.Contains(drift[0].Reason, tc.want)):
				t.Errorf("Drift = %+v, want a reason naming %q", drift, tc.want)
			}
		})
	}
}

func TestDrift_ReadsClaudeAndCodexTimeoutsInSeconds(t *testing.T) {
	native := []byte(`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"a.sh","timeout":10}]}]}}`)
	for _, target := range []string{"claude", "codex"} {
		drift, err := Drift(target, native, "PreToolUse", "Bash", "linux", []Handler{{Command: "a.sh", Timeout: 10 * time.Second}}, exactCover)
		if err != nil || len(drift) != 0 {
			t.Errorf("%s: Drift = %+v, %v, want none", target, drift, err)
		}
	}
}

func TestDrift_UsesTheTargetsMatcherCoverage(t *testing.T) {
	native := []byte(`{"hooks":{"PreToolUse":[{"matcher":"Write|Edit","hooks":[{"type":"command","command":"a.sh"}]}]}}`)
	covers := func(native, spec string) bool { return strings.Contains("|"+native+"|", "|"+spec+"|") }
	drift, err := Drift("codex", native, "PreToolUse", "Edit", "linux", []Handler{{Command: "a.sh"}}, covers)
	if err != nil || len(drift) != 0 {
		t.Errorf("Drift = %+v, %v, want a merged matcher accepted", drift, err)
	}
}

func TestDrift_ReportsAMissingCommand(t *testing.T) {
	native := []byte(`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"old.sh"}]}]}}`)
	drift, err := Drift("claude", native, "Stop", "", "linux", []Handler{{Command: "new.sh"}}, exactCover)
	if err != nil || len(drift) != 1 || !strings.Contains(drift[0].Reason, `no Stop command "new.sh"`) {
		t.Errorf("Drift = %+v, %v", drift, err)
	}
}

func TestDecideHandler_ClaudeFailClosedBlocksAFailedRun(t *testing.T) {
	closed, open := Handler{Command: "guard.sh", FailClosed: true}, Handler{Command: "guard.sh"}
	cases := []struct {
		name  string
		event string
		h     Handler
		r     Result
		want  Decision
	}{
		{"exit 1 fails closed", "PreToolUse", closed, Result{Exit: 1}, Block},
		{"timeout fails closed", "PreToolUse", closed, Result{TimedOut: true}, Block},
		{"exit 1 fails open by default", "PreToolUse", open, Result{Exit: 1}, Error},
		{"exit 0 with no output still allows", "PreToolUse", closed, Result{}, Allow},
		{"session events cannot block", "SessionStart", closed, Result{Exit: 1}, Error},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := DecideHandler("claude", c.event, c.h, c.r); got != c.want {
				t.Errorf("decision = %s, want %s", got, c.want)
			}
		})
	}
}

func TestDrift_NamesAClaudeOnFailureThatDiffersFromFailClosed(t *testing.T) {
	body := []byte(`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"guard.sh"}]}]}}`)
	drift, err := Drift("claude", body, "PreToolUse", "Bash", "linux", []Handler{{Command: "guard.sh", FailClosed: true}}, func(n, s string) bool { return n == s })
	if err != nil {
		t.Fatal(err)
	}
	if len(drift) != 1 || !strings.Contains(drift[0].Reason, "onFailure") {
		t.Errorf("drift = %+v, want one naming onFailure", drift)
	}
}

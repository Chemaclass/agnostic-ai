package hookrun

import "testing"

func TestClineVSCodeTaskStart_ReadsOnlyValidatedContext(t *testing.T) {
	cases := []struct {
		name     string
		result   Result
		decision Decision
		context  bool
	}{
		{"context on nonzero", Result{Exit: 1, Stdout: `{"contextModification":"fact"}`}, Allow, true},
		{"last JSON after diagnostics", Result{Stdout: "diagnostic\n{\"contextModification\":\"fact\"}\n"}, Allow, true},
		{"cancel suppresses context", Result{Stdout: `{"cancel":true,"contextModification":"fact"}`}, Block, false},
		{"SDK context is not VS Code context", Result{Stdout: `{"context":"fact"}`}, Allow, false},
		{"invalid type", Result{Stdout: `{"cancel":"false","contextModification":"fact"}`}, Allow, false},
		{"removed field", Result{Stdout: `{"shouldContinue":true,"contextModification":"fact"}`}, Allow, false},
		{"failed script without JSON", Result{Exit: 1, Stdout: "diagnostic"}, Error, false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			decision, context, _ := ClineVSCodeTaskStartResult(test.result)
			if decision != test.decision || context != test.context {
				t.Errorf("got %s context=%t, want %s context=%t", decision, context, test.decision, test.context)
			}
		})
	}
}

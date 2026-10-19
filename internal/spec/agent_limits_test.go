package spec

import (
	"math"
	"testing"
)

func TestAgentMaxTurns_RejectsValuesThatOverflowInt(t *testing.T) {
	for _, v := range []any{uint64(math.MaxUint64), uint64(math.MaxInt) + 1} {
		if n, ok := AgentMaxTurns(v); ok {
			t.Errorf("%v accepted as %d", v, n)
		}
	}
	if n, ok := AgentMaxTurns(uint64(7)); !ok || n != 7 {
		t.Errorf("7 read as %d, %t", n, ok)
	}
}

func TestAgentLimitProblems_NameTheValueType(t *testing.T) {
	for meta, want := range map[string]struct {
		field string
		value any
		msg   string
	}{
		"text bool":   {"background", "true", `background is the text "true"; write true or false`},
		"text number": {"maxTurns", "10", `maxTurns is the text "10"; write a whole number of 1 or more`},
		"decimal":     {"maxTurns", 2.0, "maxTurns is the decimal number 2; write a whole number of 1 or more"},
		"zero":        {"maxTurns", 0, "maxTurns is 0; write a whole number of 1 or more"},
		"too large":   {"maxTurns", uint64(math.MaxUint64), "maxTurns is 18446744073709551615, which is too large; write a whole number of 1 or more"},
	} {
		got := AgentLimitProblems(map[string]any{want.field: want.value})[want.field]
		if got != want.msg {
			t.Errorf("%s: got %q, want %q", meta, got, want.msg)
		}
	}
}

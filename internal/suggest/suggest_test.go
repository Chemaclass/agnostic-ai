package suggest

import (
	"strings"
	"testing"
)

func TestName(t *testing.T) {
	names := []string{"claude", "cline", "codex", "cursor", "kiro"}
	cases := map[string]string{
		"claud":  "claude",
		"cursr":  "cursor",
		"Claude": "claude",
		"codx":   "codex",
		"kilo":   "kiro",
		"cdoex":  "codex",
		"clien":  "cline",
		"zzzzzz": "",
		"":       "",
		"claude": "",
	}
	for in, want := range cases {
		if got := Name(in, names); got != want {
			t.Errorf("Name(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestName_LongInputSuggestsNothing(t *testing.T) {
	if got := Name(strings.Repeat("a", maxInput+1), []string{"a"}); got != "" {
		t.Errorf("an input past maxInput should suggest nothing, got %q", got)
	}
}

func TestName_TieSuggestsNothing(t *testing.T) {
	if got := Name("kigo", []string{"kilo", "kiro"}); got != "" {
		t.Errorf("a tie should suggest nothing, got %q", got)
	}
}

func TestName_LongImpossibleCandidateAddsNoDistanceAllocations(t *testing.T) {
	input := "cluade"
	ordinary := []string{"claude"}
	withLong := []string{strings.Repeat("a", 4096), "claude"}
	baseline := testing.AllocsPerRun(5, func() {
		if got := Name(input, ordinary); got != "claude" {
			t.Fatalf("ordinary suggestion = %q, want claude", got)
		}
	})
	withCandidate := testing.AllocsPerRun(5, func() {
		if got := Name(input, withLong); got != "claude" {
			t.Fatalf("long candidate changed suggestion = %q, want claude", got)
		}
	})
	if withCandidate > baseline+1 {
		t.Errorf("candidate outside the accepted distance added %.0f allocations; reject it before allocating distance rows", withCandidate-baseline)
	}
}

func TestName_FoldsCandidateBeforeCheckingByteLength(t *testing.T) {
	if got := Name("kevin", []string{"Kevin"}); got != "Kevin" {
		t.Errorf("case folding changes candidate byte length: got %q, want Kelvin-sign spelling", got)
	}
}

func TestName_AcceptsCandidatesAtDistanceLengthBoundary(t *testing.T) {
	input := strings.Repeat("a", maxInput)
	candidate := input + "aa"
	if got := Name(input, []string{candidate + "a", candidate}); got != candidate {
		t.Errorf("candidate within accepted distance must remain suggestible: length %d, want %d", len(got), len(candidate))
	}
}

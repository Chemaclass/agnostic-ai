package suggest

import "testing"

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

func TestName_TieSuggestsNothing(t *testing.T) {
	if got := Name("kigo", []string{"kilo", "kiro"}); got != "" {
		t.Errorf("a tie should suggest nothing, got %q", got)
	}
}

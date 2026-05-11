package emit

import (
	"strings"
	"testing"
)

func TestWriteTOMLString_EscapesQuotesAndBackslashes(t *testing.T) {
	var sb strings.Builder
	WriteTOMLString(&sb, "description", `Says "hi" with a \ slash`)
	got := sb.String()
	want := `description = "Says \"hi\" with a \\ slash"` + "\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestWriteTOMLMultiline_PreservesNewlinesEscapesDelimiter(t *testing.T) {
	var sb strings.Builder
	WriteTOMLMultiline(&sb, "prompt", "line1\nline2 with \"quotes\"\n")
	got := sb.String()
	for _, want := range []string{
		"prompt = \"\"\"\n",
		`line1` + "\n",
		`line2 with \"quotes\"`,
		"\n\"\"\"\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
}

func TestWriteTOMLMultiline_AppendsTrailingNewlineWhenMissing(t *testing.T) {
	var sb strings.Builder
	WriteTOMLMultiline(&sb, "prompt", "no-trailing-newline")
	got := sb.String()
	if !strings.HasSuffix(got, "no-trailing-newline\n\"\"\"\n") {
		t.Errorf("expected trailing newline before closing delim: %q", got)
	}
}

func TestWriteTOMLStringArray(t *testing.T) {
	var sb strings.Builder
	WriteTOMLStringArray(&sb, "names", []string{"alpha", "beta"})
	got := sb.String()
	want := `names = ["alpha", "beta"]` + "\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestEscapeTOMLBasic(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{`plain`, `plain`},
		{`with "quote"`, `with \"quote\"`},
		{`back\slash`, `back\\slash`},
		{`both " and \`, `both \" and \\`},
	}
	for _, c := range cases {
		if got := EscapeTOMLBasic(c.in); got != c.want {
			t.Errorf("EscapeTOMLBasic(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

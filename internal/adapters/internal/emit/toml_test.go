package emit

import (
	"strings"
	"testing"
)

func TestWriteTOMLString_EscapesQuotesAndBackslashes(t *testing.T) {
	t.Parallel()
	var sb strings.Builder
	WriteTOMLString(&sb, "description", `Says "hi" with a \ slash`)
	got := sb.String()
	want := `description = "Says \"hi\" with a \\ slash"` + "\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestWriteTOMLMultiline_PreservesNewlinesEscapesDelimiter(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	var sb strings.Builder
	WriteTOMLMultiline(&sb, "prompt", "no-trailing-newline")
	got := sb.String()
	if !strings.HasSuffix(got, "no-trailing-newline\n\"\"\"\n") {
		t.Errorf("expected trailing newline before closing delim: %q", got)
	}
}

func TestWriteTOMLStringArray(t *testing.T) {
	t.Parallel()
	var sb strings.Builder
	WriteTOMLStringArray(&sb, "names", []string{"alpha", "beta"})
	got := sb.String()
	want := `names = ["alpha", "beta"]` + "\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestWriteTOMLValue_TypedScalarsAndArrays(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		v    any
		want string
	}{
		{"string", "manual", `k = "manual"` + "\n"},
		{"bool", true, "k = true\n"},
		{"int", 7, "k = 7\n"},
		{"int64", int64(9), "k = 9\n"},
		{"float", 1.5, "k = 1.5\n"},
		{"stringSlice", []string{"a", "b"}, `k = ["a", "b"]` + "\n"},
		{"anySlice", []any{"a", "b"}, `k = ["a", "b"]` + "\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var sb strings.Builder
			if !WriteTOMLValue(&sb, "k", c.v) {
				t.Fatalf("WriteTOMLValue returned false for %#v", c.v)
			}
			if got := sb.String(); got != c.want {
				t.Errorf("got %q want %q", got, c.want)
			}
		})
	}
}

func TestWriteTOMLValue_SkipsUnsupported(t *testing.T) {
	t.Parallel()
	for _, v := range []any{
		map[string]any{"nested": "table"},
		[]any{"a", 1}, // mixed-type array
		nil,
	} {
		var sb strings.Builder
		if WriteTOMLValue(&sb, "k", v) {
			t.Errorf("expected skip for %#v, wrote %q", v, sb.String())
		}
		if sb.Len() != 0 {
			t.Errorf("wrote output for skipped %#v: %q", v, sb.String())
		}
	}
}

func TestEscapeTOMLBasic(t *testing.T) {
	t.Parallel()
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

package emit

import (
	"math"
	"strings"
	"testing"
	"time"
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

func TestWriteTOMLInlineTable_KeepsScalarTypes(t *testing.T) {
	t.Parallel()
	var sb strings.Builder
	skipped := WriteTOMLInlineTable(&sb, "features", map[string]any{"shell_tool": false, "memories": true, "name": "x", "n": 2, "ratio": 2.0, "my key": 1})
	want := `features = { memories = true, "my key" = 1, n = 2, name = "x", ratio = 2.0, shell_tool = false }` + "\n"
	if len(skipped) != 0 || sb.String() != want {
		t.Errorf("got %v %q want %q", skipped, sb.String(), want)
	}
}

func TestWriteTOMLInlineTable_SkipsOnlyUnencodableKeys(t *testing.T) {
	t.Parallel()
	var sb strings.Builder
	skipped := WriteTOMLInlineTable(&sb, "k", map[string]any{"off": false, "bad": math.NaN(), "gone": nil, "ints": []any{1, 2}})
	if want := `k = { ints = [1, 2], off = false }` + "\n"; sb.String() != want {
		t.Errorf("got %q want %q", sb.String(), want)
	}
	if len(skipped) != 1 || skipped[0] != "bad" {
		t.Errorf("skipped = %v, want [bad]", skipped)
	}
}

func TestTOMLLiteral_Shapes(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		v    any
		want string
		ok   bool
	}{
		"whole float": {2.0, "2.0", true},
		"fraction":    {1.5, "1.5", true},
		"nan":         {math.NaN(), "", false},
		"inf":         {math.Inf(-1), "", false},
		"control":     {"a\tb\nc\x01", `"a\tb\nc\u0001"`, true},
		"nested":      {map[string]any{"a": map[string]any{"b": true}}, "{ a = { b = true } }", true},
		"date":        {time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), "", false},
		"channel":     {make(chan int), "", false},
	}
	for name, c := range cases {
		got, ok := TOMLLiteral(c.v)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: got %q %v, want %q %v", name, got, ok, c.want, c.ok)
		}
	}
}

func TestTOMLKey(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{"plain_key-1": "plain_key-1", "my key": `"my key"`, "a.b": `"a.b"`, "": `""`} {
		if got := TOMLKey(in); got != want {
			t.Errorf("TOMLKey(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestWriteTOMLValue_FloatsKeepLegacyBytes(t *testing.T) {
	t.Parallel()
	var sb strings.Builder
	if !WriteTOMLValue(&sb, "k", 2.0) || sb.String() != "k = 2\n" {
		t.Errorf("got %q", sb.String())
	}
	sb.Reset()
	if WriteTOMLValue(&sb, "k", math.Inf(1)) || sb.Len() != 0 {
		t.Errorf("infinity must be skipped, got %q", sb.String())
	}
}

func TestWriteTOMLString_EscapesControlCharacters(t *testing.T) {
	t.Parallel()
	var sb strings.Builder
	WriteTOMLString(&sb, "k", "a\nb\tc\x00")
	if want := `k = "a\nb\tc\u0000"` + "\n"; sb.String() != want {
		t.Errorf("got %q want %q", sb.String(), want)
	}
}

func TestTOMLLiteral_RejectsUint64AboveInt64(t *testing.T) {
	t.Parallel()
	if got, ok := TOMLLiteral(uint64(math.MaxInt64) + 1); ok {
		t.Errorf("wrote %q, TOML integers stop at int64", got)
	}
	if got, ok := TOMLLiteral(uint64(5)); !ok || got != "5" {
		t.Errorf("got %q %v", got, ok)
	}
}

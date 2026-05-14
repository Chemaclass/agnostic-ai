package header

import (
	"strings"
	"testing"
)

func TestLine_FormatsCarryMarker(t *testing.T) {
	for _, tc := range []struct {
		name   string
		format Format
		prefix string
	}{
		{"markdown", FormatMarkdown, "<!--"},
		{"toml", FormatTOML, "#"},
		{"yaml", FormatYAML, "#"},
		{"shell", FormatShell, "#"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Line(tc.format)
			if !strings.HasPrefix(got, tc.prefix) {
				t.Errorf("expected prefix %q, got %q", tc.prefix, got)
			}
			if !strings.Contains(got, Marker) {
				t.Errorf("header missing marker: %q", got)
			}
			if !strings.HasSuffix(got, "\n") {
				t.Errorf("header must end with newline: %q", got)
			}
		})
	}
}

func TestLine_JSONReturnsEmpty(t *testing.T) {
	if got := Line(FormatJSON); got != "" {
		t.Errorf("FormatJSON header should be empty, got %q", got)
	}
}

func TestWith_PlainMarkdownPrepends(t *testing.T) {
	got := With("# Title\n\nbody\n", FormatMarkdown)
	if !strings.HasPrefix(got, "<!-- "+Marker) {
		t.Errorf("expected header at top, got:\n%s", got)
	}
	if !strings.Contains(got, "# Title") {
		t.Errorf("body lost: %s", got)
	}
}

func TestWith_MarkdownFrontmatterInsertsBelow(t *testing.T) {
	in := "---\nname: foo\n---\n\nbody text\n"
	got := With(in, FormatMarkdown)
	if !strings.HasPrefix(got, "---\nname: foo\n---\n") {
		t.Errorf("frontmatter mangled: %q", got)
	}
	idxFM := strings.Index(got, "---\nname: foo\n---\n")
	idxHeader := strings.Index(got, "<!-- "+Marker)
	if idxHeader < idxFM {
		t.Errorf("header must appear after frontmatter:\n%s", got)
	}
	if !strings.Contains(got, "body text") {
		t.Errorf("body lost: %s", got)
	}
}

func TestWith_TOMLPrepends(t *testing.T) {
	got := With("key = \"value\"\n", FormatTOML)
	if !strings.HasPrefix(got, "# "+Marker) {
		t.Errorf("expected toml header, got:\n%s", got)
	}
}

func TestWith_EmptyContentUnchanged(t *testing.T) {
	if got := With("", FormatMarkdown); got != "" {
		t.Errorf("empty content must stay empty, got %q", got)
	}
}

func TestWith_JSONUnchanged(t *testing.T) {
	in := "{\"k\":1}\n"
	if got := With(in, FormatJSON); got != in {
		t.Errorf("FormatJSON must not modify content, got %q", got)
	}
}

func TestStrip_RemovesMarkdownHeader(t *testing.T) {
	in := With("# Title\n\nbody\n", FormatMarkdown)
	got := Strip(in)
	if got != "# Title\n\nbody\n" {
		t.Errorf("expected stripped body, got %q", got)
	}
}

func TestStrip_RemovesTOMLHeader(t *testing.T) {
	in := With("k = 1\n", FormatTOML)
	got := Strip(in)
	if got != "k = 1\n" {
		t.Errorf("expected stripped body, got %q", got)
	}
}

func TestStrip_PreservesFrontmatter(t *testing.T) {
	in := With("---\nname: foo\n---\n\nbody\n", FormatMarkdown)
	got := Strip(in)
	want := "---\nname: foo\n---\n\nbody\n"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestStrip_LeavesUserContentAlone(t *testing.T) {
	in := "# My doc\n\nUnrelated text.\n"
	if got := Strip(in); got != in {
		t.Errorf("user content modified: got %q want %q", got, in)
	}
}

func TestStrip_NoMarkerNoOp(t *testing.T) {
	in := "# heading\n\nbody.\n"
	if got := Strip(in); got != in {
		t.Errorf("expected no-op, got %q", got)
	}
}

func TestRoundTrip_FrontmatterStaysIntact(t *testing.T) {
	original := "---\nname: foo\ndescription: bar\n---\n\nbody\n"
	round := Strip(With(original, FormatMarkdown))
	if round != original {
		t.Errorf("roundtrip changed content:\noriginal=%q\nround=%q", original, round)
	}
}

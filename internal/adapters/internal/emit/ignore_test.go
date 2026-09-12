package emit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestIgnoreBody_ConcatenatesTrimmed(t *testing.T) {
	t.Parallel()
	got := IgnoreBody([]spec.Entry{
		{Body: "*.env\n"},
		{Body: "  \n"}, // blank-only: skipped
		{Body: "secrets/"},
	})
	want := "*.env\n\nsecrets/"
	if got != want {
		t.Errorf("IgnoreBody = %q, want %q", got, want)
	}
}

func TestIgnoreBody_PreservesPatternWhitespace(t *testing.T) {
	t.Parallel()
	got := IgnoreBody([]spec.Entry{
		{Body: "\n secret.key\n"},
		{Body: "\t\n"},
		{Body: "trailing.key\\ \n"},
	})
	const want = " secret.key\n\n\t\n\ntrailing.key\\ "
	if got != want {
		t.Errorf("IgnoreBody = %q, want %q", got, want)
	}
}

func TestWriteIgnoreFile_WritesHeaderAndPatterns(t *testing.T) {
	t.Parallel()
	sess := NewSession()
	dir := t.TempDir()
	path := filepath.Join(dir, ".cursorignore")
	if err := sess.WriteIgnoreFile([]spec.Entry{{Body: "*.env"}}, "cursor", path, false); err != nil {
		t.Fatalf("write: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, "*.env") {
		t.Errorf("missing pattern: %s", body)
	}
	if !strings.HasPrefix(body, "#") {
		t.Errorf("expected shell-style (#) provenance header: %s", body)
	}
}

func TestWriteIgnoreFile_NoOpWhenEmpty(t *testing.T) {
	t.Parallel()
	sess := NewSession()
	dir := t.TempDir()
	path := filepath.Join(dir, ".cursorignore")
	if err := sess.WriteIgnoreFile(nil, "cursor", path, false); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected no file written for empty ignore set, got err=%v", err)
	}
}

// TestWriteIgnoreFile_RefusesHandAuthoredFile pins #754: an ignore file
// the user wrote by hand is what keeps credentials out of agent
// context, so a sync that would drop its patterns aborts and leaves the
// file untouched.
func TestWriteIgnoreFile_RefusesHandAuthoredFile(t *testing.T) {
	t.Parallel()
	sess := NewSession()
	dir := t.TempDir()
	path := filepath.Join(dir, ".kiroignore")
	const existing = "# hand-authored by the team\nmy-secrets/\n*.key\n"
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	err := sess.WriteIgnoreFile([]spec.Entry{{Body: "dist/\nnode_modules/"}}, "kiro", path, false)
	if err == nil {
		t.Fatal("expected an error on a hand-authored ignore file")
	}
	if got := errs.CodeOf(err); got != errs.CodeIgnoreOverwrite {
		t.Errorf("CodeOf = %q, want %q", got, errs.CodeIgnoreOverwrite)
	}
	for _, want := range []string{".kiroignore", "my-secrets/", "*.key", "agnostic-ai import kiro"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q: %v", want, err)
		}
	}
	if got := readFileString(t, path); got != existing {
		t.Errorf("hand-authored file must stay untouched, got:\n%s", got)
	}
}

func TestWriteIgnoreFile_RefusesUnprovenPatternPreservation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		existing string
		body     string
	}{
		{
			name:     "reordered negation",
			existing: "!example.key\n*.key\n",
			body:     "*.key\n!example.key\n",
		},
		{
			name:     "appended negation",
			existing: "*.key\n",
			body:     "*.key\n!example.key\n",
		},
		{
			name:     "duplicated negation",
			existing: "!example.key\n*.key\n",
			body:     "!example.key\n*.key\n!example.key\n",
		},
		{
			name:     "removed repeated exclusion",
			existing: "*.key\n!example.key\n*.key\n",
			body:     "*.key\n!example.key\n",
		},
		{
			name:     "changed leading space",
			existing: " secret.key\n",
			body:     "secret.key\n",
		},
		{
			name:     "indented hash is a pattern",
			existing: " #secret.key\n",
			body:     "dist/\n",
		},
		{
			name:     "changed escaped trailing space",
			existing: "secret.key\\ \n",
			body:     "secret.key\\\n",
		},
		{
			name:     "tab is a pattern",
			existing: "\t\n",
			body:     "dist/\n",
		},
		{
			name:     "BOM moves behind generated header",
			existing: "\uFEFF*.key\n",
			body:     "\uFEFF*.key\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), ".cursorignore")
			if err := os.WriteFile(path, []byte(tt.existing), 0o644); err != nil {
				t.Fatal(err)
			}

			err := NewSession().WriteIgnoreFile([]spec.Entry{{Body: tt.body}}, "cursor", path, false)
			if got := errs.CodeOf(err); got != errs.CodeIgnoreOverwrite {
				t.Errorf("CodeOf(%v) = %q, want %q", err, got, errs.CodeIgnoreOverwrite)
			}
			if got := readFileString(t, path); got != tt.existing {
				t.Errorf("hand-authored file changed: got %q, want %q", got, tt.existing)
			}
		})
	}
}

// Extra exclusions before and after unchanged patterns are safe, even
// when the hand-authored file already contains a negation.
func TestWriteIgnoreFile_WritesWhenPatternsSurvive(t *testing.T) {
	t.Parallel()
	sess := NewSession()
	dir := t.TempDir()
	path := filepath.Join(dir, ".kiroignore")
	if err := os.WriteFile(path, []byte("# team notes\nmy-secrets/\n*.key\n!example.key\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []spec.Entry{{Body: "cache/"}, {Body: "my-secrets/\n*.key\n!example.key"}, {Body: "dist/"}}
	if err := sess.WriteIgnoreFile(entries, "kiro", path, false); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := readFileString(t, path)
	for _, want := range []string{header.Marker, "cache/", "my-secrets/\n*.key\n!example.key", "dist/"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in:\n%s", want, got)
		}
	}
}

// A previously generated file carries the provenance marker, so
// dropping a pattern the specs no longer name is a normal regeneration,
// not a loss. Without this the first spec edit that removes a pattern
// would break every later sync.
func TestWriteIgnoreFile_OverwritesItsOwnOutput(t *testing.T) {
	t.Parallel()
	sess := NewSession()
	dir := t.TempDir()
	path := filepath.Join(dir, ".kiroignore")
	if err := sess.WriteIgnoreFile([]spec.Entry{{Body: "my-secrets/\n*.key"}}, "kiro", path, false); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := sess.WriteIgnoreFile([]spec.Entry{{Body: "*.key"}}, "kiro", path, false); err != nil {
		t.Fatalf("second write: %v", err)
	}
	if got := readFileString(t, path); strings.Contains(got, "my-secrets/") {
		t.Errorf("expected the dropped pattern to be gone:\n%s", got)
	}
}

// Dry-run writes nothing, so nothing is at risk and the preview must
// not fail. Matches readExistingJSON, which skips its own read in
// dry-run for the same reason.
func TestWriteIgnoreFile_DryRunSkipsTheCheck(t *testing.T) {
	t.Parallel()
	sess := NewSession()
	dir := t.TempDir()
	path := filepath.Join(dir, ".kiroignore")
	const existing = "my-secrets/\n"
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := sess.WriteIgnoreFile([]spec.Entry{{Body: "dist/"}}, "kiro", path, true); err != nil {
		t.Fatalf("dry-run must not fail: %v", err)
	}
	if got := readFileString(t, path); got != existing {
		t.Errorf("dry-run must not write, got:\n%s", got)
	}
}

// `outputs.<target>.provenance-header: false` removes the marker the
// check reads, so the check cannot tell the tool's own output from the
// user's and stands down rather than failing every sync.
func TestWriteIgnoreFile_SkipsTheCheckWithoutProvenance(t *testing.T) {
	sess := NewSession()
	dir := t.TempDir()
	path := filepath.Join(dir, ".kiroignore")
	if err := os.WriteFile(path, []byte("my-secrets/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	prev := SetProvenanceEnabled(false)
	t.Cleanup(func() { SetProvenanceEnabled(prev) })

	if err := sess.WriteIgnoreFile([]spec.Entry{{Body: "dist/"}}, "kiro", path, false); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := readFileString(t, path); !strings.Contains(got, "dist/") {
		t.Errorf("expected the emitted pattern in:\n%s", got)
	}
}

// `sync --check` runs adapters in capture mode. It must report the
// refusal there too, so the problem surfaces before a real sync hits
// it.
func TestWriteIgnoreFile_RefusesWhileCapturing(t *testing.T) {
	t.Parallel()
	sess := NewSession()
	sess.StartCapture()
	defer sess.StopCapture()
	dir := t.TempDir()
	path := filepath.Join(dir, ".kiroignore")
	const existing = "!example.key\n*.key\n"
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := sess.WriteIgnoreFile([]spec.Entry{{Body: "*.key\n!example.key"}}, "kiro", path, false); err == nil {
		t.Fatal("expected capture mode to report the refusal")
	}
	if got := readFileString(t, path); got != existing {
		t.Errorf("capture changed hand-authored file: %q", got)
	}
}

// A comment-only or blank file excludes nothing, so replacing it loses
// no protection and must not block a sync.
func TestWriteIgnoreFile_IgnoresCommentOnlyFile(t *testing.T) {
	t.Parallel()
	sess := NewSession()
	dir := t.TempDir()
	path := filepath.Join(dir, ".kiroignore")
	if err := os.WriteFile(path, []byte("# nothing here yet\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := sess.WriteIgnoreFile([]spec.Entry{{Body: "dist/"}}, "kiro", path, false); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestNamedIgnorePatterns_CapsTheList(t *testing.T) {
	t.Parallel()
	patterns := []string{"a", "b", "c", "d", "e", "f", "g"}
	got := namedIgnorePatterns(patterns)
	if want := `"a", "b", "c", "d", "e" and 2 more`; got != want {
		t.Errorf("namedIgnorePatterns = %q, want %q", got, want)
	}
}

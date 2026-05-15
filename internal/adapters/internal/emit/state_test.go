package emit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCapture_SuppressesIOAndRecordsContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "captured.md")

	StartCapture()
	if err := WriteFile(path, "hello", false); err != nil {
		t.Fatal(err)
	}
	files := StopCapture()

	if len(files) != 1 {
		t.Fatalf("expected 1 captured file, got %d", len(files))
	}
	if files[0].Path != path || files[0].Content != "hello\n" {
		t.Errorf("captured = %+v", files[0])
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("capture mode wrote a file, err=%v", err)
	}
}

func TestStopCapture_AfterStartCaptureClearsState(t *testing.T) {
	StartCapture()
	if err := WriteFile("a.md", "x", false); err != nil {
		t.Fatal(err)
	}
	first := StopCapture()
	if len(first) != 1 {
		t.Fatalf("expected 1 file, got %d", len(first))
	}
	// Starting a new capture should not see the prior content.
	StartCapture()
	second := StopCapture()
	if len(second) != 0 {
		t.Errorf("expected fresh capture, got %d", len(second))
	}
}

func TestRecording_DoesNotSuppressIO(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "recorded.md")

	StartRecording()
	if err := WriteFile(path, "hello", false); err != nil {
		t.Fatal(err)
	}
	paths := StopRecording()

	if len(paths) != 1 || paths[0] != path {
		t.Errorf("recorded = %+v", paths)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("recording mode should still write the file, err=%v", err)
	}
}

func TestStopRecording_AfterStartClearsState(t *testing.T) {
	dir := t.TempDir()
	StartRecording()
	if err := WriteFile(filepath.Join(dir, "a.md"), "x", false); err != nil {
		t.Fatal(err)
	}
	first := StopRecording()
	if len(first) != 1 {
		t.Fatalf("expected 1 path, got %d", len(first))
	}
	StartRecording()
	second := StopRecording()
	if len(second) != 0 {
		t.Errorf("expected fresh recording, got %d", len(second))
	}
}

func TestSourceComment(t *testing.T) {
	if got := SourceComment(""); got != "" {
		t.Errorf("empty path should return empty, got %q", got)
	}
	if got := SourceComment("rules/foo.md"); got != "<!-- source: rules/foo.md -->\n" {
		t.Errorf("unexpected: %q", got)
	}
}

func TestWriteSection_RendersHeadingProvenanceDescBody(t *testing.T) {
	var sb stringBuilder
	WriteSection(sb.Builder(), "my-heading", entryFixture())
	got := sb.String()
	for _, want := range []string{
		"### my-heading\n",
		"<!-- source: rules/x.md -->\n",
		"_a description_\n",
		"body content\n",
	} {
		if !contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

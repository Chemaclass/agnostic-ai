package emit

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestRenderMemoryBlock_ImportsTheIndexFromTheProjectRoot(t *testing.T) {
	got := RenderMemoryBlock("CLAUDE.md")

	want := MemoryStartMarker + "\n\n## Shared memory\n\n@.agnostic-ai/memory/MEMORY.md\n\n" + MemoryEndMarker + "\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestRenderMemoryBlock_ImportsRelativeToANestedEntryPoint(t *testing.T) {
	got := RenderMemoryBlock(".claude/CLAUDE.md")

	if !strings.Contains(got, "\n@../.agnostic-ai/memory/MEMORY.md\n") {
		t.Errorf("import not relative to .claude/:\n%s", got)
	}
}

func TestAppendMemoryBlock_ReplacesAnEarlierBlock(t *testing.T) {
	once := AppendMemoryBlock("# Shared\n", RenderMemoryBlock("CLAUDE.md"))
	twice := AppendMemoryBlock(once, RenderMemoryBlock("CLAUDE.md"))

	if strings.Count(twice, MemoryStartMarker) != 1 {
		t.Errorf("blocks stacked:\n%s", twice)
	}
	if !strings.HasPrefix(twice, "# Shared\n\n"+MemoryStartMarker) {
		t.Errorf("block not after the body:\n%s", twice)
	}
}

func TestAppendMemoryBlock_NoOpWithoutBlock(t *testing.T) {
	if got := AppendMemoryBlock("# Shared\n", ""); got != "# Shared\n" {
		t.Errorf("got %q", got)
	}
}

func TestStripGeneratedAppendices_DropsTheMemoryBlock(t *testing.T) {
	body := AppendMemoryBlock("# Shared\n", RenderMemoryBlock("CLAUDE.md"))
	body = AppendLocalInstructions(body, "Private note.")

	if got := StripGeneratedAppendices(body); got != "# Shared\n" {
		t.Errorf("got %q, want the shared body alone", got)
	}
}

func TestWriteSection_NamesABuiltinSourceInsteadOfItsCachePath(t *testing.T) {
	var sb strings.Builder
	WriteSection(&sb, "shared-memory", spec.Entry{Kind: spec.KindRule, Name: "shared-memory", Layer: "builtin", Path: "/cache/builtins/abc/rules/shared-memory.md", Body: "Rule."})

	if got := sb.String(); !strings.Contains(got, "<!-- source: builtin:shared-memory -->") || strings.Contains(got, "/cache/") {
		t.Errorf("got:\n%s", got)
	}
}

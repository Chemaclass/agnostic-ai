package applypatch

import (
	"reflect"
	"strings"
	"testing"
)

func TestParse_ReadsEveryFileOperationInOnePatch(t *testing.T) {
	patch := strings.Join([]string{
		"*** Begin Patch",
		"*** Add File: docs/new.md",
		"+# New",
		"+*** Update File: not/a/header.go",
		"*** Update File: src/app.go",
		"@@ func main() {",
		"-\told()",
		"+\tnew()",
		"*** Delete File: src/old.go",
		"*** Update File: src/a.go",
		"*** Move to: src/b.go",
		"@@",
		" context",
		"*** End of File",
		"*** End Patch",
	}, "\n")

	got := Parse(patch)
	want := []Op{
		{Kind: Add, Path: "docs/new.md"},
		{Kind: Update, Path: "src/app.go"},
		{Kind: Delete, Path: "src/old.go"},
		{Kind: Update, Path: "src/a.go", MoveTo: "src/b.go"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse() = %#v, want %#v", got, want)
	}
}

func TestParse_ReadsAPatchWrappedInAHeredocWithCRLF(t *testing.T) {
	patch := "apply_patch <<'EOF'\r\n*** Begin Patch\r\n*** Update File: dir with space/x.go  \r\n@@\r\n-a\r\n+b\r\n*** End Patch\r\nEOF\r\n"

	got := Parse(patch)
	want := []Op{{Kind: Update, Path: "dir with space/x.go"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse() = %#v, want %#v", got, want)
	}
}

func TestParse_IgnoresAMoveWithNoUpdateAndAnEmptyPath(t *testing.T) {
	patch := "*** Move to: stray.go\n*** Add File: \n*** Delete File: gone.go\n*** Move to: nowhere.go\n"

	got := Parse(patch)
	want := []Op{{Kind: Delete, Path: "gone.go"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse() = %#v, want %#v", got, want)
	}
}

func TestParse_ReturnsNothingForTextThatIsNoPatch(t *testing.T) {
	if got := Parse("go test ./..."); len(got) != 0 {
		t.Errorf("Parse() = %#v, want none", got)
	}
}

package errs

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"testing"
)

func TestCoded_PrefixIncludesCode(t *testing.T) {
	t.Parallel()
	err := Coded(CodeSpecParse, "boom")
	want := "[AAI-001] boom"
	if err.Error() != want {
		t.Errorf("Error()=%q, want %q", err.Error(), want)
	}
}

func TestCoded_FormatArgsExpand(t *testing.T) {
	t.Parallel()
	err := Coded(CodeConfigDecode, "parse %s: %s", "agnostic-ai.yaml", "bad indent")
	if !strings.Contains(err.Error(), "parse agnostic-ai.yaml: bad indent") {
		t.Errorf("missing formatted args: %q", err.Error())
	}
	if !strings.HasPrefix(err.Error(), "[AAI-004] ") {
		t.Errorf("missing code prefix: %q", err.Error())
	}
}

func TestCoded_WrapPreservesIs(t *testing.T) {
	t.Parallel()
	err := Coded(CodeConfigMissing, "read %s: %w", "agnostic-ai.yaml", fs.ErrNotExist)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Error("errors.Is should still match wrapped sentinel")
	}
	if got := CodeOf(err); got != CodeConfigMissing {
		t.Errorf("CodeOf=%q, want %q", got, CodeConfigMissing)
	}
}

func TestCoded_WrapPreservesAs(t *testing.T) {
	t.Parallel()
	original := &fs.PathError{Op: "open", Path: "x", Err: fs.ErrNotExist}
	err := Coded(CodeUnsupportedKind, "missing dir: %w", original)
	var pe *fs.PathError
	if !errors.As(err, &pe) {
		t.Fatal("errors.As should unwrap to *fs.PathError")
	}
	if pe.Path != "x" {
		t.Errorf("PathError.Path=%q, want %q", pe.Path, "x")
	}
}

func TestCodeOf_NilAndPlainReturnEmpty(t *testing.T) {
	t.Parallel()
	if got := CodeOf(nil); got != "" {
		t.Errorf("CodeOf(nil)=%q, want empty", got)
	}
	if got := CodeOf(fmt.Errorf("plain")); got != "" {
		t.Errorf("CodeOf(plain)=%q, want empty", got)
	}
}

func TestIsCode_ShapeOnly(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"AAI-001":  true,
		"AAI-9999": true,
		"AAI-12":   false, // require >= 3 digits
		"aai-001":  false,
		"AAI001":   false,
		"":         false,
	}
	for in, want := range cases {
		if got := IsCode(in); got != want {
			t.Errorf("IsCode(%q)=%v, want %v", in, got, want)
		}
	}
}

func TestRegistry_AllCoveredCodesHaveEntries(t *testing.T) {
	t.Parallel()
	codes := []Code{
		CodeSpecParse, CodeUnsupportedKind, CodeConfigMissing, CodeConfigDecode,
		CodeOutputCollision,
		CodeImportFileUnknown,
		CodeSyncTargetUnknown, CodeFlagConflict,
	}
	for _, c := range codes {
		e, ok := Lookup(c)
		if !ok {
			t.Errorf("Lookup(%s): missing registry entry", c)
			continue
		}
		if e.Title == "" || e.Cause == "" || e.Fix == "" {
			t.Errorf("%s: empty Title/Cause/Fix", c)
		}
	}
}

func TestAll_SortedByCode(t *testing.T) {
	t.Parallel()
	all := All()
	if len(all) == 0 {
		t.Fatal("All(): want non-empty")
	}
	for i := 1; i < len(all); i++ {
		if all[i-1].Code >= all[i].Code {
			t.Errorf("All() not sorted: %s before %s", all[i-1].Code, all[i].Code)
		}
	}
}

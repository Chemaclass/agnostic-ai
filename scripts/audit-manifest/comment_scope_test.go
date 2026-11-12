package main

import (
	"bytes"
	"os"
	"testing"
)

func TestCommentOnlyKeepsRuntimeTokensAndLinePositions(t *testing.T) {
	before := []byte("package cli\n// Keep in sync with docs/check.md.\nfunc enabled() bool { return true }\n")
	after := []byte("package cli\n// Keep in sync with docs/lint-codes.md.\nfunc enabled() bool { return true }\n")
	if !commentOnly(before, after) {
		t.Error("same-line ordinary comment edit required runtime review")
	}
	for _, changed := range []string{
		"package cli\n// Keep in sync with docs/lint-codes.md.\nfunc enabled() bool { return false }\n",
		"package cli\n// New comment\n// Added line\nfunc enabled() bool { return true }\n",
		"package cli\n//go:noinline\nfunc enabled() bool { return true }\n",
		"package cli\n// New comment\nfunc enabled() bool { return tr\x00ue }\n",
	} {
		if commentOnly(before, []byte(changed)) {
			t.Errorf("accepted behavior/directive/position edit %q", changed)
		}
	}
}

func TestCommentOnlyKeepsCgoAndExistingDirectivesConservative(t *testing.T) {
	for _, header := range []string{"//go:build linux\n\npackage cli\n", "package cli\nimport \"C\"\n", "// +build linux\n\npackage cli\n", "//line generated.go:4\npackage cli\n"} {
		if commentOnly([]byte(header+"// before\nvar value = true\n"), []byte(header+"// after\nvar value = true\n")) {
			t.Errorf("accepted directive/cgo %q", header)
		}
	}
}

func TestHistoricalCommentEditKeepsRuntimeProofSeparateFromProse(t *testing.T) {
	before, err := os.ReadFile("testdata/comment-scope/lint_codes.go")
	if err != nil {
		t.Fatal(err)
	}
	after := bytes.Replace(before, []byte("docs/site/content/docs/cli-reference/check.md."), []byte("docs/site/content/docs/cli-reference/lint-codes.md."), 1)
	if digest(before) != "e39452d79c2b638d0257a9921310ed17a192e3e815906c21fb9a071e169aedbe" || digest(after) != "6af059d9ceb090497aeac9b1931f5f82418499904999ed9dbb7c69f96baa32fd" {
		t.Fatal("historical fixture differs from recorded source blobs")
	}
	if !commentOnly(before, after) {
		t.Error("886a0c9fb docs-link edit needs runtime review")
	}
	behavior := bytes.Replace(after, []byte("Severity lintSeverity"), []byte("Severity string"), 1)
	if commentOnly(before, behavior) {
		t.Error("nearby type change escaped runtime review")
	}
}

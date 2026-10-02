package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestExplain_LintCode_Human(t *testing.T) {
	silence(t)
	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"explain", "LINT011"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	got := out.String()
	for _, want := range []string{"LINT011:", "Cause:", "Fix:", "lint.instructions-words"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestExplain_LintCode_JSON(t *testing.T) {
	silence(t)
	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"explain", "LINT019", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	var got struct {
		Code, Title, Severity, Cause, Fix string
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if got.Code != "LINT019" || got.Severity != "warn" || got.Title == "" || got.Cause == "" || got.Fix == "" {
		t.Errorf("incomplete entry: %+v", got)
	}
}

func TestExplain_LintCode_Unknown(t *testing.T) {
	silence(t)
	root := NewRootCmd("test")
	root.SetArgs([]string{"explain", "LINT999"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown lint code") {
		t.Fatalf("expected unknown-lint-code error, got %v", err)
	}
}

// Every code lint can raise needs an explain entry and a row on the
// check reference page, and neither may list a code lint never raises.
func TestLintCodes_MatchSourceAndDocs(t *testing.T) {
	inSource := map[string]bool{}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	quoted := regexp.MustCompile(`"(LINT\d{3})"`)
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "lint_codes.go" {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range quoted.FindAllStringSubmatch(string(data), -1) {
			inSource[m[1]] = true
		}
	}
	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "site", "content", "docs", "cli-reference", "check.md"))
	if err != nil {
		t.Fatal(err)
	}
	inDocs := map[string]bool{}
	for _, c := range regexp.MustCompile(`(?m)^(?:\| )?(LINT\d{3})(?: \|| warns)`).FindAllStringSubmatch(string(doc), -1) {
		inDocs[c[1]] = true
	}
	inRegistry := map[string]bool{}
	for code := range lintCodes {
		inRegistry[code] = true
	}
	compare := func(name string, got map[string]bool) {
		t.Helper()
		var missing, extra []string
		for c := range inSource {
			if !got[c] {
				missing = append(missing, c)
			}
		}
		for c := range got {
			if !inSource[c] {
				extra = append(extra, c)
			}
		}
		sort.Strings(missing)
		sort.Strings(extra)
		if len(missing) > 0 || len(extra) > 0 {
			t.Errorf("%s: missing %v, never raised %v", name, missing, extra)
		}
	}
	compare("lintCodes", inRegistry)
	compare("check.md table", inDocs)
	for code, e := range lintCodes {
		if e.Title == "" || e.Cause == "" || e.Fix == "" || (e.Severity != lintWarn && e.Severity != lintError) {
			t.Errorf("%s: incomplete entry %+v", code, e)
		}
	}
}

func TestLint_PointsAtExplain(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "empty.md"), "---\nname: empty\n---\n")
	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"lint"})
	_ = root.Execute()
	if !strings.Contains(out.String(), "LINT001") || !strings.Contains(out.String(), "agnostic-ai explain LINT001") {
		t.Errorf("expected a pointer to explain the first code, got:\n%s", out.String())
	}
}

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func duplicateRuleProject(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "b.md"),
		"---\nname: b\ndescription: first\nalwaysApply: true\n---\nbody1\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "b2.md"),
		"---\nname: b\ndescription: second\nalwaysApply: true\n---\nbody2\n")
	testutil.Chdir(t, dir)
}

func TestValidate_FailsOnDuplicateSpecNameInOneLayer(t *testing.T) {
	duplicateRuleProject(t)

	out, err := runCLI(t, "validate")
	if err == nil {
		t.Fatalf("validate passed with two rules named b:\n%s", out)
	}
	for _, want := range []string{".agnostic-ai/rules/b.md", ".agnostic-ai/rules/b2.md", `duplicate rule name "b"`} {
		if !strings.Contains(filepath.ToSlash(out), want) {
			t.Errorf("validate output lacks %q:\n%s", want, out)
		}
	}
}

func TestSync_WarnsWhenADuplicateNameDropsASpec(t *testing.T) {
	duplicateRuleProject(t)

	out, err := runCLI(t, "sync")
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if !strings.Contains(filepath.ToSlash(out), "warning: .agnostic-ai/rules/b.md: duplicate rule name \"b\"; .agnostic-ai/rules/b2.md wins and this spec reaches no tool") {
		t.Errorf("sync did not warn about the dropped spec:\n%s", out)
	}
}

func TestValidate_NoAutofixHintWhenNothingIsAutofixable(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "ignore", "x.md"), "---\nname: x\ndescription: d\n---\nnode_modules/\n")
	testutil.Chdir(t, dir)

	out, err := runCLI(t, "validate")
	if err == nil {
		t.Fatalf("validate passed an ignore spec no target reads:\n%s", out)
	}
	if strings.Contains(out, "autofixable") {
		t.Errorf("validate offered --fix with no autofixable issue:\n%s", out)
	}
}

func TestNew_NotesWhenNoEnabledTargetReadsTheKind(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, codex]\n")
	testutil.Chdir(t, dir)

	var logged bytes.Buffer
	previous := logOut
	logOut = &logged
	t.Cleanup(func() { logOut = previous })
	if _, err := runCLI(t, "new", "ignore", "my-ignore"); err != nil {
		t.Fatalf("new ignore: %v", err)
	}
	out := logged.String()
	if _, err := os.Stat(filepath.Join(".agnostic-ai", "ignore", "my-ignore.md")); err != nil {
		t.Fatalf("new did not write the spec: %v", err)
	}
	if !strings.Contains(out, "no enabled target reads ignore specs") || !strings.Contains(out, "cursor") {
		t.Errorf("new did not say which targets read ignore specs:\n%s", out)
	}
}

func TestValidate_NamesTheSameLayerWinnerWhenALocalSpecExtendsIt(t *testing.T) {
	duplicateRuleProject(t)
	writeFile(t, filepath.Join(".agnostic-ai", "local", "rules", "b.md"), "---\nname: b\n---\nlocal\n")

	out, _ := runCLI(t, "validate")
	if !strings.Contains(filepath.ToSlash(out), `duplicate rule name "b"; .agnostic-ai/rules/b2.md wins`) {
		t.Errorf("validate did not name the same-layer winner:\n%s", out)
	}
}

func TestSyncGlobal_WarnsWhenADuplicateNameDropsASpec(t *testing.T) {
	_, source := globalAgentTestHome(t)
	writeFile(t, filepath.Join(source, "rules", "b.md"), "---\nname: b\n---\nbody1\n")
	writeFile(t, filepath.Join(source, "rules", "b2.md"), "---\nname: b\n---\nbody2\n")

	_, errOut, err := runGlobalCheck("sync", "--dry-run")
	if err != nil {
		t.Fatalf("sync --global --dry-run: %v\n%s", err, errOut)
	}
	if !strings.Contains(filepath.ToSlash(errOut), `duplicate rule name "b"`) {
		t.Errorf("sync --global did not warn about the dropped spec:\n%s", errOut)
	}
}

func TestReportIssues_OffersFixForAnAutofixableIssue(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCmd("test")
	cmd.SetOut(&out)
	reportIssues(cmd, []validationIssue{{Path: "rules/a.md", Message: "missing name", kind: issueMissingName}})
	if !strings.Contains(out.String(), "(* = autofixable; rerun with --fix)") {
		t.Errorf("reportIssues did not offer --fix:\n%s", out.String())
	}
}

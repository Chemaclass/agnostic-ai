package cli

import (
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/overview"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func userFindings(findings []overview.Finding) []overview.Finding {
	var out []overview.Finding
	for _, f := range findings {
		if f.Layer == overview.LayerUser {
			out = append(out, f)
		}
	}
	return out
}

func TestOverviewInputs_IncludesTheUserSetupsLintFindings(t *testing.T) {
	home := userOverviewHome(t)
	source := filepath.Join(home, "source")
	writeFile(t, filepath.Join(source, "agents", "helper.md"), "---\nname: helper\ndescription: Helps with chores\nmodl: opus\n---\nHelp with chores.\n")
	dir := testutil.TempCwd(t)
	writeBareReadProject(t, dir)

	in, err := overviewInputs(".")
	if err != nil {
		t.Fatal(err)
	}
	userLevel := userFindings(in.Findings)
	want, err := filepath.EvalSymlinks(filepath.Join(source, "agents", "helper.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(userLevel) != 1 || userLevel[0].Code != "LINT007" || userLevel[0].Path != "agents/helper.md" || userLevel[0].Fix == "" {
		t.Fatalf("user-level findings = %+v, all = %+v", userLevel, in.Findings)
	}
	if got, _ := filepath.EvalSymlinks(filepath.FromSlash(userLevel[0].OpenPath)); got != want {
		t.Errorf("open path = %q, want %q", userLevel[0].OpenPath, want)
	}
	project := 0
	for _, f := range in.Findings {
		if f.Code == "LINT038" && f.Layer == "" {
			project++
		}
	}
	if project != 1 {
		t.Errorf("project LINT038 findings = %d, want 1: %+v", project, in.Findings)
	}
}

func TestOverviewInputs_NoUserSetupAddsNoUserFindings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("AGNOSTIC_AI_HOME", filepath.Join(home, "missing"))
	dir := testutil.TempCwd(t)
	writeBareReadProject(t, dir)

	in, err := overviewInputs(".")
	if err != nil {
		t.Fatal(err)
	}
	if got := userFindings(in.Findings); len(got) != 0 {
		t.Errorf("user-level findings without a user setup = %+v", got)
	}
}

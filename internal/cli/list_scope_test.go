package cli

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// listScopeProject has one shared rule, a local change to it, and one
// rule that is only local.
func listScopeProject(t *testing.T) {
	t.Helper()
	testutil.Chdir(t, setupEmptyProject(t))
	silence(t)
	writeFile(t, ".agnostic-ai/rules/style.md", "---\nname: style\ndescription: Team style.\nalwaysApply: true\n---\n\nShared.\n")
	writeFile(t, ".agnostic-ai/rules/tests.md", "---\nname: tests\ndescription: Team tests.\nalwaysApply: true\n---\n\nShared.\n")
	writeFile(t, ".agnostic-ai/local/rules/style.md", "---\nname: style\ndescription: My style.\n---\n")
	writeFile(t, ".agnostic-ai/local/rules/scratch.md", "---\nname: scratch\ndescription: Mine.\nalwaysApply: true\n---\n\nMine.\n")
}

func TestList_NamesTheLocalLayerAndWhatItOverrides(t *testing.T) {
	listScopeProject(t)
	out, _, err := runSplit(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"rule\tstyle\tlocal\toverrides project\n",
		"rule\tscratch\tlocal\n",
		"rule\ttests\tproject\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("list lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "project-user") {
		t.Errorf("list still prints the old layer name:\n%s", out)
	}
}

func TestList_ScopeLocalShowsOnlyPersonalSpecs(t *testing.T) {
	listScopeProject(t)
	out, _, err := runSplit(t, "list", "--scope", "local")
	if err != nil {
		t.Fatal(err)
	}
	if want := "rule\tscratch\tlocal\nrule\tstyle\tlocal\toverrides project\n"; sortedLines(out) != sortedLines(want) {
		t.Errorf("list --scope local:\n%s\nwant:\n%s", out, want)
	}

	raw, _, err := runSplit(t, "list", "--scope", "local", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		Entries []struct {
			Name, Layer string
			Overrides   *struct{ Layer, Path string }
		}
	}
	if err := json.Unmarshal([]byte(raw), &report); err != nil {
		t.Fatalf("%v\n%s", err, raw)
	}
	if len(report.Entries) != 2 {
		t.Fatalf("want two local entries, got %s", raw)
	}
	for _, e := range report.Entries {
		if e.Layer != "local" {
			t.Errorf("%s: layer %q, want local", e.Name, e.Layer)
		}
		if overrides := e.Overrides != nil; overrides != (e.Name == "style") {
			t.Errorf("%s: overrides=%v", e.Name, e.Overrides)
		}
		if e.Overrides != nil && (e.Overrides.Layer != "project" || e.Overrides.Path != ".agnostic-ai/rules/style.md") {
			t.Errorf("style overrides %+v, want the shared rule", e.Overrides)
		}
	}
}

func TestList_ScopeLocalWithoutPersonalSpecsSaysHowToAddOne(t *testing.T) {
	testutil.Chdir(t, setupEmptyProject(t))
	silence(t)
	writeFile(t, ".agnostic-ai/rules/style.md", "---\nname: style\ndescription: Team style.\nalwaysApply: true\n---\n\nShared.\n")
	out, stderr, err := runSplit(t, "list", "--scope", "local")
	if err != nil {
		t.Fatal(err)
	}
	if out != "" || !strings.Contains(stderr, "new <kind> <name> --scope local") {
		t.Errorf("stdout=%q stderr=%q, want only a hint naming new --scope local", out, stderr)
	}
}

func TestList_UserScopeNamesTheUserLayers(t *testing.T) {
	_, source := globalConfigTestHome(t)
	writeFile(t, source+"/rules/mine.md", "---\nname: mine\ndescription: Mine.\n---\n\nMine.\n")
	writeFile(t, source+"/local/rules/mine.md", "---\nname: mine\ndescription: Really mine.\n---\n")
	writeFile(t, source+"/rules/other.md", "---\nname: other\ndescription: Other.\n---\n\nOther.\n")
	out, _, err := runSplit(t, "list", "--scope", "user")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"rule\tmine\tuser-local\toverrides user\n", "rule\tother\tuser\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("list --scope user lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "global") {
		t.Errorf("list --scope user still prints a global layer:\n%s", out)
	}
}

func TestScopeFlag_LocalIsOfferedOnlyWhereItRuns(t *testing.T) {
	for _, tc := range []struct {
		command string
		want    bool
	}{{"list", true}, {"validate", false}} {
		out, _, err := runSplit(t, "__completeNoDesc", tc.command, "--scope", "")
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(out, specScopeLocal+"\n"); got != tc.want {
			t.Errorf("%s completion offers local=%t, want %t:\n%s", tc.command, got, tc.want, out)
		}
	}
}

func sortedLines(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	slices.Sort(lines)
	return strings.Join(lines, "\n")
}

// Commands that read a project take --scope like every other command,
// and say what to run instead at a scope they do not have.
func TestScopeFlag_ProjectOnlyCommandsNameTheUserLevelAlternative(t *testing.T) {
	testutil.Chdir(t, setupEmptyProject(t))
	silence(t)
	for _, command := range []string{"why", "status", "render", "compare", "graph", "revert", "cleanup"} {
		for _, scope := range []string{specScopeUser, specScopeLocal} {
			_, _, err := runSplit(t, command, "--scope", scope)
			if err == nil || !strings.Contains(err.Error(), "does not support --scope "+scope) || !strings.Contains(err.Error(), "agnostic-ai") && command != "cleanup" {
				t.Errorf("%s --scope %s: err=%v, want the scope refused with an alternative", command, scope, err)
			}
		}
		if _, _, err := runSplit(t, command, "--scope", "bogus"); err == nil || !strings.Contains(err.Error(), "user, project, or local") {
			t.Errorf("%s --scope bogus: err=%v", command, err)
		}
	}
	if _, _, err := runSplit(t, "status", "--scope", "project"); err != nil {
		t.Errorf("status --scope project must run as status does: %v", err)
	}
}

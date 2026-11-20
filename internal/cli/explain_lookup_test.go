package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func explainLookupProject(t *testing.T) {
	t.Helper()
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "tone.md"), "---\nname: tone\ndescription: Tone.\nalwaysApply: true\n---\n\nBe kind.\n")
}

func TestExplain_FindsASpecByItsPathUnderTheSpecRoot(t *testing.T) {
	explainLookupProject(t)
	for _, args := range [][]string{{"explain", "rules/tone.md"}, {"explain", "rules/tone.md", "--layers"}} {
		out, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		if !strings.Contains(out, ".agnostic-ai/rules/tone.md") {
			t.Errorf("%v does not name the spec:\n%s", args, out)
		}
	}
}

func TestExplain_SuggestsASpecByName(t *testing.T) {
	explainLookupProject(t)
	out, err := runCLI(t, "explain", "tone")
	if err == nil || !strings.Contains(err.Error(), "Did you mean: .agnostic-ai/rules/tone.md") {
		t.Errorf("explain tone: %v\n%s", err, out)
	}
}

func TestExplain_AnUnknownErrorCodeNamesThePublicPage(t *testing.T) {
	explainLookupProject(t)
	_, err := runCLI(t, "explain", "AAI-999")
	if err == nil || !strings.Contains(err.Error(), "https://agnostic-ai.org/docs/errors/") {
		t.Errorf("explain AAI-999: %v", err)
	}
}

func TestExplain_LayersPicksTheProjectSpecAmongItsLayers(t *testing.T) {
	explainLookupProject(t)
	writeFile(t, filepath.Join(".agnostic-ai", "local", "rules", "tone.md"), "Also be brief.\n")
	out, err := runCLI(t, "explain", "rules/tone.md", "--layers")
	if err != nil || !strings.Contains(out, ".agnostic-ai/local/rules/tone.md") {
		t.Errorf("explain --layers: %v\n%s", err, out)
	}
}

func TestExplain_AFileAtTheGivenPathWinsOverTheSpecRoot(t *testing.T) {
	explainLookupProject(t)
	writeFile(t, filepath.Join("rules", "tone.md"), "Not a spec.\n")
	_, err := runCLI(t, "explain", "rules/tone.md")
	if err == nil || !strings.Contains(err.Error(), "exists but is not loaded as a spec") {
		t.Errorf("explain rules/tone.md: %v", err)
	}
}

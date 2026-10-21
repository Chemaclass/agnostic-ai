package integration

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/cli"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

type heroFanout struct {
	Source  string   `toml:"source"`
	Line    string   `toml:"line"`
	Targets []string `toml:"targets"`
	Outputs []struct {
		Path  string `toml:"path"`
		Tools string `toml:"tools"`
		Line  string `toml:"line"`
		Drift string `toml:"drift"`
	} `toml:"outputs"`
}

func loadHeroFanout(t *testing.T) heroFanout {
	t.Helper()
	var landing struct {
		Hero struct {
			Fanout heroFanout `toml:"fanout"`
		} `toml:"hero"`
	}
	if _, err := toml.DecodeFile("../../docs/site/data/landing.toml", &landing); err != nil {
		t.Fatalf("decode landing hero: %v", err)
	}
	return landing.Hero.Fanout
}

// TestSiteDocs_LandingHeroMatchesSync keeps the hero honest: one source line
// and one line per output card, each found verbatim in a real sync of
// fixtures/hero.
func TestSiteDocs_LandingHeroMatchesSync(t *testing.T) {
	fanout := loadHeroFanout(t)
	fixture := filepath.Join("fixtures", "hero")

	var config struct {
		Targets []string `yaml:"targets"`
	}
	raw, err := os.ReadFile(filepath.Join(fixture, "agnostic-ai.yaml"))
	if err != nil {
		t.Fatalf("read hero fixture config: %v", err)
	}
	if err := yaml.Unmarshal(raw, &config); err != nil {
		t.Fatalf("decode hero fixture config: %v", err)
	}
	if !slices.Equal(config.Targets, fanout.Targets) {
		t.Errorf("hero lists targets %v, fixtures/hero syncs %v", fanout.Targets, config.Targets)
	}
	if len(fanout.Outputs) == 0 {
		t.Fatal("hero shows no output cards")
	}

	dir := t.TempDir()
	copyLandingFixture(t, fixture, dir)
	testutil.Chdir(t, dir)
	t.Setenv("AGNOSTIC_AI_HOME", filepath.Join(dir, "global"))
	root := cli.NewRootCmd("test")
	root.SetArgs([]string{"sync", "--gitignore=off"})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	if err := root.Execute(); err != nil {
		t.Fatalf("sync the hero fixture: %v", err)
	}

	assertShownLine := func(path, line string) {
		t.Helper()
		if !isOneLine(line) {
			t.Errorf("hero shows %q for %s; each card shows exactly one line", line, path)
		}
		data, err := os.ReadFile(filepath.FromSlash(path))
		if err != nil {
			t.Errorf("hero shows %s, which a sync of fixtures/hero does not write", path)
			return
		}
		if !hasLine(string(data), line) {
			t.Errorf("hero shows %q for %s, but that line is not in the file:\n%s", line, path, data)
		}
	}
	assertShownLine(fanout.Source, fanout.Line)

	shown := map[string]bool{}
	drifted := false
	for _, output := range fanout.Outputs {
		if !isOneLine(output.Drift) {
			t.Errorf("hero card %s needs one drift line for the Without view, got %q", output.Path, output.Drift)
		}
		assertShownLine(output.Path, output.Line)
		shown[output.Line] = true
		drifted = drifted || output.Drift != output.Line
	}
	if len(shown) < 2 {
		t.Error("every hero card shows the same line, so the hero reads as a copy")
	}
	if !drifted {
		t.Error("the Without view shows no drift on any hero card")
	}
}

func isOneLine(text string) bool {
	return strings.TrimSpace(text) != "" && !strings.Contains(text, "\n")
}

func hasLine(body, want string) bool {
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimSpace(line) == want {
			return true
		}
	}
	return false
}

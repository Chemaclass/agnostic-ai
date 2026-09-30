package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestDoctor_WarnsAboutGeneratedPathsMissingFromPackagingIgnores(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	t.Setenv("CODEX_HOME", t.TempDir())
	mustWriteGlobalTest(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\n")
	mustWriteGlobalTest(t, ".agnostic-ai/skills/review/SKILL.md", "---\nname: review\ndescription: Review code.\n---\nReview.\n")
	ignore := ".claude/**\n.codex/**\n"
	mustWriteGlobalTest(t, ".vscodeignore", ignore)
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	root = NewRootCmd("test")
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs([]string{"doctor"})
	if err := root.Execute(); err != nil {
		t.Errorf("packaging warning should be advisory: %v", err)
	}
	if got := output.String(); !strings.Contains(got, "Packaging ignores:") || !strings.Contains(got, ".vscodeignore") || !strings.Contains(got, ".agents/skills/review/SKILL.md") || strings.Contains(got, "All checks passed") {
		t.Errorf("missing packaging warning: %s", got)
	}
	output.Reset()
	root = NewRootCmd("test")
	root.SetOut(&output)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"doctor", "--json"})
	if err := root.Execute(); err != nil {
		t.Errorf("JSON warning should be advisory: %v", err)
	}
	var report map[string]json.RawMessage
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if got := report["packaging_ignore"]; !bytes.Contains(got, []byte(".vscodeignore")) || !bytes.Contains(got, []byte(".agents/skills/review/SKILL.md")) {
		t.Errorf("missing JSON packaging warning: %s", got)
	}
	unchanged, err := os.ReadFile(".vscodeignore")
	if err != nil {
		t.Fatal(err)
	}
	if string(unchanged) != ignore {
		t.Errorf("doctor changed ignore file: %s", unchanged)
	}
}

func TestPackagingIgnore_FormatRules(t *testing.T) {
	cases := []struct {
		name, format, rules, file string
		ignored                   bool
	}{
		{"npm directory", ".npmignore", ".agents/", ".agents/skills/review/SKILL.md", true},
		{"npm bare basename", ".npmignore", "SKILL.md", "services/.agents/skills/review/SKILL.md", true},
		{"npm root anchor", ".npmignore", "/SKILL.md", "services/SKILL.md", false},
		{"npm case folding", ".npmignore", "skill.md", ".agents/skills/review/SKILL.md", true},
		{"npm later exclusion", ".npmignore", "!.agents/**\n.agents/**", ".agents/skills/review/SKILL.md", true},
		{"npm child reinclude traverses parent", ".npmignore", ".agents/\n!.agents/skills/review/SKILL.md", ".agents/skills/review/SKILL.md", false},
		{"npm excluded parent stops basename reinclude", ".npmignore", ".agents/\n!SKILL.md", ".agents/skills/review/SKILL.md", true},
		{"vsce directory slash does not cover same-name file", ".vscodeignore", "AGENTS.md/", "AGENTS.md", false},
		{"vsce directory slash covers descendants", ".vscodeignore", "AGENTS.md/", "AGENTS.md/child.txt", true},
		{"vsce bare basename covers file", ".vscodeignore", "AGENTS.md", "AGENTS.md", true},
		{"vsce bare basename covers directory descendants", ".vscodeignore", "AGENTS.md", "AGENTS.md/child.txt", true},
		{"vsce wildcard directory covers descendants", ".vscodeignore", "*.md/", "AGENTS.md/child.txt", true},
		{"vsce wildcard directory does not cover file", ".vscodeignore", "*.md/", "AGENTS.md", false},
		{"vsce directory negation does not reinclude same-name file", ".vscodeignore", "AGENTS.md\n!AGENTS.md/", "AGENTS.md", true},
		{"vsce directory negation reincludes descendants", ".vscodeignore", "AGENTS.md\n!AGENTS.md/", "AGENTS.md/child.txt", false},
		{"vsce explicit descendants do not cover file", ".vscodeignore", "AGENTS.md/**", "AGENTS.md", false},
		{"vsce globstar directory expansion covers root file", ".vscodeignore", "**/", "AGENTS.md", true},
		{"vsce folder expands", ".vscodeignore", ".agents", ".agents/skills/review/SKILL.md", true},
		{"vsce negation wins before exclusion", ".vscodeignore", "!.agents/**\n.agents/**", ".agents/skills/review/SKILL.md", false},
		{"vsce case sensitive", ".vscodeignore", "**/skill.md", ".agents/skills/review/SKILL.md", false},
		{"vsce leading slash stays absolute", ".vscodeignore", "/.agents/**", ".agents/skills/review/SKILL.md", false},
		{"vsce bare basename is root only", ".vscodeignore", "SKILL.md", ".agents/skills/review/SKILL.md", false},
		{"docker directory", ".dockerignore", "/.agents/", ".agents/skills/review/SKILL.md", true},
		{"docker bare basename is root only", ".dockerignore", "SKILL.md", ".agents/skills/review/SKILL.md", false},
		{"docker root only directory", ".dockerignore", ".agents", "services/.agents/skills/review/SKILL.md", false},
		{"docker last exclusion wins", ".dockerignore", "!.agents/**\n.agents/**", ".agents/skills/review/SKILL.md", true},
		{"docker child reinclude", ".dockerignore", ".agents\n!.agents/skills/review/SKILL.md", ".agents/skills/review/SKILL.md", false},
		{"globstar zero directories", ".dockerignore", "**/SKILL.md", "SKILL.md", true},
		{"globstar deep directories", ".vscodeignore", "**/SKILL.md", "services/.agents/skills/review/SKILL.md", true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			rules, err := parsePackagingIgnore(test.format, test.rules)
			if err != nil {
				t.Fatal(err)
			}
			if got := packagingIgnored(test.format, rules, test.file); got != test.ignored {
				t.Errorf("ignored=%v,want %v", got, test.ignored)
			}
		})
	}
}

func TestPackagingIgnore_ReportsUnsupportedSyntaxWithoutClaimingUncoveredPaths(t *testing.T) {
	testutil.TempCwd(t)
	mustWriteGlobalTest(t, ".vscodeignore", "{.agents,.codex}/**\n")
	reports := []driftReport{{Current: []adapters.CapturedFile{{Path: ".agents/skills/review/SKILL.md"}}}}
	findings := collectPackagingIgnoreFindings(reports)
	if len(findings) != 1 || findings[0].Path != ".vscodeignore" || findings[0].Problem == "" || len(findings[0].Uncovered) != 0 {
		t.Errorf("unsupported syntax treated as uncovered: %+v", findings)
	}
}

func TestPackagingIgnore_UsesCapturedPathsIncludingOverridesAndAssets(t *testing.T) {
	dir := testutil.TempCwd(t)
	mustWriteGlobalTest(t, ".dockerignore", "/.agents/\n")
	reports := []driftReport{{Current: []adapters.CapturedFile{{Path: "native/skills/review/helper.sh"}}, Missing: []adapters.CapturedFile{{Path: "native/skills/review/SKILL.md"}, {Path: "native/skills/review/helper.sh"}, {Path: dir + "/../outside.md"}}}}
	findings := collectPackagingIgnoreFindings(reports)
	if len(findings) != 1 || strings.Join(findings[0].Uncovered, ",") != "native/skills/review/SKILL.md,native/skills/review/helper.sh" {
		t.Errorf("captured output coverage: %+v", findings)
	}
}

func TestPackagingIgnore_AbsentFilesDoNotWarn(t *testing.T) {
	testutil.TempCwd(t)
	reports := []driftReport{{Current: []adapters.CapturedFile{{Path: "AGENTS.md"}}}}
	if got := collectPackagingIgnoreFindings(reports); len(got) != 0 {
		t.Errorf("missing ignore files warned: %+v", got)
	}
}

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const describedTiersConfig = `models:
  strong: {claude: opus, codex: gpt-5.5, description: for everyday coding}
  fast: {claude: haiku, description: for lookups and high-volume subagents.}
  balanced: {claude: sonnet}
`

const describedTiersLine = "Model tiers: `fast` for lookups and high-volume subagents; `strong` for everyday coding."

func syncProjectOK(t *testing.T) {
	t.Helper()
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
}

func TestSync_ListsDescribedTiersInEveryInstructionsFile(t *testing.T) {
	dir := budgetProject(t, "targets: [claude, codex, gemini, copilot, zed, junie]\n"+describedTiersConfig)
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), "# Project\n\nShared instructions.\n")

	syncProjectOK(t)

	for _, path := range []string{"AGENTS.md", "GEMINI.md", ".github/copilot-instructions.md", ".rules", ".junie/AGENTS.md"} {
		got := readFileString(t, filepath.Join(dir, path))
		if strings.Count(got, describedTiersLine) != 1 {
			t.Errorf("%s should list the described tiers once:\n%s", path, got)
		}
		if strings.Contains(got, "`balanced`") {
			t.Errorf("%s lists a tier without a description:\n%s", path, got)
		}
	}
	claude := readFileString(t, filepath.Join(dir, "CLAUDE.md"))
	if !strings.Contains(claude, "@AGENTS.md") || strings.Contains(claude, "Model tiers") {
		t.Errorf("CLAUDE.md should import AGENTS.md, which carries the tiers:\n%s", claude)
	}
}

func TestSync_ClaudeOnlyCompanionCarriesTheTiers(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n"+describedTiersConfig)
	writeFile(t, "AGENTS.md", "# Project\n\nShared instructions.\n")
	writeFile(t, "CLAUDE.md", "@AGENTS.md\n\nClaude only.\n")
	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}

	syncProjectOK(t)

	agents := readFile(t, "AGENTS.md")
	if strings.Count(agents, describedTiersLine) != 1 || !strings.Contains(agents, "Shared instructions.") {
		t.Errorf("AGENTS.md should hold the body and the tiers once:\n%s", agents)
	}
	if claude := readFile(t, "CLAUDE.md"); !strings.Contains(claude, "@AGENTS.md") || strings.Contains(claude, "Model tiers") {
		t.Errorf("CLAUDE.md should stay the companion:\n%s", claude)
	}
}

func TestSync_TiersWithoutDescriptionsChangeNoInstructionsFile(t *testing.T) {
	plain := syncedInstructions(t, "")
	tiered := syncedInstructions(t, "models:\n  strong: {claude: opus, codex: gpt-5.5}\n")

	for path, want := range plain {
		if got := tiered[path]; got != want {
			t.Errorf("%s differs once tiers exist:\n%s\nwant:\n%s", path, got, want)
		}
		if strings.Contains(want, "agnostic-ai:models") {
			t.Errorf("%s holds a model tiers block:\n%s", path, want)
		}
	}
}

func syncedInstructions(t *testing.T, models string) map[string]string {
	t.Helper()
	dir := budgetProject(t, "targets: [claude, codex, gemini]\n"+models)
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), "# Project\n\nShared instructions.\n")
	syncProjectOK(t)
	out := map[string]string{}
	for _, path := range []string{"AGENTS.md", "CLAUDE.md", "GEMINI.md"} {
		out[path] = readFileString(t, filepath.Join(dir, path))
	}
	return out
}

func TestSync_LocalTierReplacesItsDescription(t *testing.T) {
	cases := map[string]struct{ local, want, absent string }{
		"local description wins": {
			local:  "models:\n  strong: {claude: opus, description: for the hard parts}\n",
			want:   "Model tiers: `fast` for lookups and high-volume subagents; `strong` for the hard parts.",
			absent: "everyday coding",
		},
		"local tier without one drops it": {
			local:  "models:\n  strong: {claude: opus}\n",
			want:   "Model tiers: `fast` for lookups and high-volume subagents.",
			absent: "`strong`",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir := budgetProject(t, "targets: [codex]\n"+describedTiersConfig)
			mustWriteFile(t, filepath.Join(dir, "agnostic-ai.local.yaml"), tc.local)

			syncProjectOK(t)

			agents := readFileString(t, filepath.Join(dir, "AGENTS.md"))
			if !strings.Contains(agents, tc.want) || strings.Contains(agents, tc.absent) {
				t.Errorf("AGENTS.md should read %q:\n%s", tc.want, agents)
			}
		})
	}
}

func TestSync_TierDescriptionIsNotATarget(t *testing.T) {
	budgetProject(t, "targets: [claude]\nmodels:\n  fast: {claude: haiku, descr: for lookups}\n")

	_, err := runCLI(t, "sync")

	if err == nil || !strings.Contains(err.Error(), `unknown target "descr"`) {
		t.Errorf("a misspelled description should still fail as a target, got %v", err)
	}
}

func TestImport_DropsTheModelTiersBlock(t *testing.T) {
	for _, from := range []string{"claude", "codex"} {
		t.Run(from, func(t *testing.T) {
			dir := budgetProject(t, "targets: [claude, codex]\n"+describedTiersConfig)
			source := filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md")
			mustWriteFile(t, source, "# Project\n\nShared instructions.\n")
			syncProjectOK(t)

			if out, err := runCLI(t, "import", from); err != nil {
				t.Fatalf("import: %v\n%s", err, out)
			}

			if got := readFileString(t, source); strings.Contains(got, "Model tiers") || strings.Contains(got, "agnostic-ai:models") {
				t.Errorf("import copied the tiers into AGNOSTIC_AI.md:\n%s", got)
			}
			if out, err := runCLI(t, "sync", "--check"); err != nil {
				t.Errorf("sync --check after import: %v\n%s", err, out)
			}
		})
	}
}

func TestLintBudget_CountsTheModelTiersLayer(t *testing.T) {
	dir := budgetProject(t, "targets: [codex]\nlint:\n  instructions-words: 10\n"+describedTiersConfig)
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), words(5)+"\n")

	out, _ := runCLI(t, "lint")

	lines := findingLines(out, "LINT011")
	if len(lines) != 1 || !strings.Contains(lines[0], "model tiers ") {
		t.Errorf("expected a LINT011 line naming the model tiers layer, got:\n%s", out)
	}
}

func TestSyncGlobal_ListsDescribedTiersInTheManagedBlock(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, codex]\n"+describedTiersConfig)
	mustWriteGlobalTest(t, filepath.Join(source, "AGNOSTIC_AI.md"), "Home instructions.\n")
	mustWriteGlobalTest(t, filepath.Join(source, "rules", "safe.md"), "---\nname: safe\n---\nBe safe.\n")

	if _, errOut, err := runGlobalCheck("sync"); err != nil {
		t.Fatalf("sync --global: %v\n%s", err, errOut)
	}

	for _, path := range []string{filepath.Join(home, ".claude", "CLAUDE.md"), filepath.Join(home, ".codex", "AGENTS.md")} {
		got := readGlobalManaged(t, path)
		instructions, tiers, rules := strings.Index(got, "Home instructions."), strings.Index(got, describedTiersLine), strings.Index(got, "## safe")
		if instructions < 0 || tiers < instructions || rules < tiers {
			t.Errorf("%s should hold instructions, tiers, then rules:\n%s", path, got)
		}
	}
}

func TestSyncGlobal_LocalTierReplacesItsDescription(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n"+describedTiersConfig)
	mustWriteGlobalTest(t, filepath.Join(source, "local", "agnostic-ai.yaml"), "version: 1\nmodels:\n  strong: {claude: opus, description: for the hard parts}\n")

	if _, errOut, err := runGlobalCheck("sync"); err != nil {
		t.Fatalf("sync --global: %v\n%s", err, errOut)
	}

	got := readGlobalManaged(t, filepath.Join(home, ".claude", "CLAUDE.md"))
	if !strings.Contains(got, "`strong` for the hard parts") || strings.Contains(got, "everyday coding") {
		t.Errorf("the local tier should replace the description:\n%s", got)
	}
}

func TestSyncGlobal_TiersWithoutDescriptionsKeepTheManagedBlock(t *testing.T) {
	blocks := make([]string, 0, 2)
	for _, models := range []string{"", "models:\n  strong: {claude: opus}\n"} {
		home, source := globalAgentTestHome(t)
		mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n"+models)
		mustWriteGlobalTest(t, filepath.Join(source, "AGNOSTIC_AI.md"), "Home instructions.\n")
		if _, errOut, err := runGlobalCheck("sync"); err != nil {
			t.Fatalf("sync --global: %v\n%s", err, errOut)
		}
		blocks = append(blocks, readGlobalManaged(t, filepath.Join(home, ".claude", "CLAUDE.md")))
	}

	if blocks[0] != blocks[1] {
		t.Errorf("tiers without descriptions changed the managed block:\n%s\nwant:\n%s", blocks[1], blocks[0])
	}
}

func readGlobalManaged(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return globalBlock(data)
}

func TestSync_RulesFileThatOwnsTheEntryPointListsTheTiers(t *testing.T) {
	dir := budgetProject(t, "targets: [codex]\noutputs:\n  codex: {rules-file: AGENTS.md}\n"+describedTiersConfig)
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "style.md"), "---\nname: style\n---\nKeep it short.\n")

	syncProjectOK(t)

	agents := readFileString(t, filepath.Join(dir, "AGENTS.md"))
	if strings.Count(agents, describedTiersLine) != 1 || !strings.Contains(agents, "Keep it short.") {
		t.Errorf("AGENTS.md should hold the rules and the tiers once:\n%s", agents)
	}
	if strings.Index(agents, describedTiersLine) > strings.Index(agents, "## Rules") {
		t.Errorf("the tiers should come before the rules:\n%s", agents)
	}
}

func TestSync_RulesFileBesideTheEntryPointGetsNoTiers(t *testing.T) {
	dir := budgetProject(t, "targets: [codex]\noutputs:\n  codex: {rules-file: CODEX_RULES.md}\n"+describedTiersConfig)
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "style.md"), "---\nname: style\n---\nKeep it short.\n")

	syncProjectOK(t)

	if got := readFileString(t, filepath.Join(dir, "CODEX_RULES.md")); strings.Contains(got, "Model tiers") {
		t.Errorf("a rules file that is not the entry point should not list the tiers:\n%s", got)
	}
}

func TestSyncGlobal_RejectsADescribedTierNamedLikeAMarker(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\nmodels:\n  \"<!-- agnostic-ai:global:end -->\": {claude: opus, description: for all}\n")
	mustWriteGlobalTest(t, filepath.Join(source, "AGNOSTIC_AI.md"), "Home instructions.\n")

	_, _, err := runGlobalCheck("sync")

	if err == nil || !strings.Contains(err.Error(), "tier name") {
		t.Errorf("sync --global error = %v, want the tier name rejected", err)
	}
	if _, statErr := os.Stat(filepath.Join(home, ".claude", "CLAUDE.md")); statErr == nil {
		t.Error("sync --global wrote instructions despite the bad tier")
	}
}

func TestSync_AdapterWrittenRulesFileListsTheTiers(t *testing.T) {
	cases := map[string]string{"claude": "CLAUDE.md", "copilot": ".github/copilot-instructions.md"}
	for target, path := range cases {
		for _, withRule := range []bool{true, false} {
			name := target + "/tiers only"
			if withRule {
				name = target + "/with a rule"
			}
			t.Run(name, func(t *testing.T) {
				dir := budgetProject(t, "targets: ["+target+"]\noutputs:\n  "+target+": {rules-file: "+path+"}\n"+describedTiersConfig)
				if withRule {
					mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "style.md"), "---\nname: style\n---\nKeep it short.\n")
				}

				syncProjectOK(t)

				got := readFileString(t, filepath.Join(dir, path))
				if strings.Count(got, describedTiersLine) != 1 {
					t.Errorf("%s should list the tiers once:\n%s", path, got)
				}
				if withRule && !strings.Contains(got, "Keep it short.") {
					t.Errorf("%s lost the rule:\n%s", path, got)
				}
			})
		}
	}
}

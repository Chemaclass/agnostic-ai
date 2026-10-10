package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestExplainContext_RanksDescriptionsAndSeparatesBodies(t *testing.T) {
	dir := budgetProject(t, "targets: [claude, codex]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), "Project instructions.\n")
	for name, size := range map[string]int{"large": 40, "small": 20} {
		mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "skills", name, "SKILL.md"), "---\nname: "+name+"\ndescription: "+words(size)+"\n---\n"+words(300)+"\n")
	}
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "excluded", "SKILL.md"), "---\nname: excluded\ntarget-exclude: claude\ndescription: "+words(200)+"\n---\nHidden body.\n")
	out, err := runCLI(t, "explain", "--context", "--target", "claude", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var report explainContextOutput
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if report.Contributions[0].Source != ".agnostic-ai/skills/large/SKILL.md" || report.Contributions[0].Words != 40 {
		t.Fatalf("ranking: %+v", report.Contributions)
	}
	var sum int
	for _, c := range report.Contributions {
		if strings.Contains(c.Source, "excluded") {
			t.Errorf("excluded source: %+v", c)
		}
		if c.Load == "startup" {
			sum += c.Words
		}
		if c.Category == "skill body" && (c.Load != "on-demand" || c.Words != 300) {
			t.Errorf("body: %+v", c)
		}
	}
	if sum != report.Startup.Words {
		t.Errorf("startup total %d != sum %d", report.Startup.Words, sum)
	}
	cfg, bundle, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	loads, err := projectSessionLoads(cfg, projectKindSupport(cfg), bundle)
	if err != nil {
		t.Fatal(err)
	}
	if report.Startup.Words != loads[0].total() {
		t.Errorf("lint parity: %d != %d", report.Startup.Words, loads[0].total())
	}
	text, err := runCLI(t, "explain", "--context", "--target", "claude")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range report.Contributions {
		row := fmt.Sprintf("[%s] %d words, %d bytes: %s (%s)", c.Load, c.Words, c.Bytes, c.Source, c.Category)
		if !strings.Contains(text, row) {
			t.Errorf("text missing %s", row)
		}
	}
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "large", "SKILL.md"), "---\nname: large\ndescription: "+words(10)+"\n---\n"+words(300)+"\n")
	out, err = runCLI(t, "explain", "--context", "--target", "claude", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var shorter explainContextOutput
	if err := json.Unmarshal([]byte(out), &shorter); err != nil {
		t.Fatal(err)
	}
	if report.Startup.Words-shorter.Startup.Words != 30 {
		t.Errorf("shortening changed total by %d", report.Startup.Words-shorter.Startup.Words)
	}
	for _, c := range shorter.Contributions {
		if c.Source == ".agnostic-ai/skills/large/SKILL.md" && c.Category == "skill discovery" && c.Words != 10 {
			t.Errorf("shortened contribution: %+v", c)
		}
	}
}

func TestExplainContext_TracksScopedRulesWithoutIncreasingStartup(t *testing.T) {
	for _, target := range []string{"cursor", "claude"} {
		t.Run(target, func(t *testing.T) {
			dir := budgetProject(t, "targets: ["+target+"]\n")
			mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), "Root guidance.\n")
			mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "scoped.md"), "---\nname: scoped\nglobs: '**/*.go'\nalwaysApply: false\n---\n"+words(40)+"\n")
			cfg, bundle, err := loadProject(".")
			if err != nil {
				t.Fatal(err)
			}
			matching, err := explainContext(cfg, bundle, target, "src/main.go")
			if err != nil {
				t.Fatal(err)
			}
			missing, err := explainContext(cfg, bundle, target, "src/main.ts")
			if err != nil {
				t.Fatal(err)
			}
			if matching.Startup != missing.Startup || matching.FileScope.Words != 40 || missing.FileScope.Words != 0 {
				t.Errorf("scope totals: matched %+v / %+v, missed %+v / %+v", matching.Startup, matching.FileScope, missing.Startup, missing.FileScope)
			}
		})
	}
}

func TestExplainContext_AttributesInlineImportsAndSharedSectionsOnce(t *testing.T) {
	dir := budgetProject(t, "targets: [codex, amp]\nsync:\n  resolve-imports: inline\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), "Root guidance.\n@docs/extra.md\n")
	mustWriteFile(t, filepath.Join(dir, "docs", "extra.md"), words(30)+"\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "shared.md"), "---\nname: shared\n---\n"+words(20)+"\n")
	cfg, bundle, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	report, err := explainContext(cfg, bundle, "codex", "")
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, c := range report.Contributions {
		if c.Load == "startup" {
			counts[c.Source]++
			if c.Words < 0 || c.Bytes < 0 {
				t.Errorf("negative count: %+v", c)
			}
		}
	}
	if counts["docs/extra.md"] != 1 || counts[".agnostic-ai/rules/shared.md"] != 1 {
		t.Errorf("source counts: %+v", counts)
	}
	loads, err := projectSessionLoads(cfg, projectKindSupport(cfg), bundle)
	if err != nil {
		t.Fatal(err)
	}
	if report.Startup.Words != loads[0].total() {
		t.Errorf("lint parity: %d != %d", report.Startup.Words, loads[0].total())
	}
	out, err := runCLI(t, "explain", "--context", "--target", "codex", "--json")
	if err != nil {
		t.Fatal(err)
	}
	again, err := runCLI(t, "explain", "--context", "--target", "codex", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if out != again {
		t.Error("JSON ordering changed between runs")
	}
	if _, err := runCLI(t, "explain", "--context", "--target", "codex", "--file", "main.go"); err == nil {
		t.Error("unsupported file target accepted")
	}
	if _, err := runCLI(t, "explain", "--context", "--target", "cursor"); err == nil {
		t.Error("unconfigured target accepted")
	}
}

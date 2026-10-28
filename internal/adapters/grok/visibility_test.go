package grok

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/gitquery"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func visibilityRepo(t *testing.T) {
	t.Helper()
	testutil.TempCwd(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if out, err := exec.Command("git", "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %s: %v", out, err)
	}
	gitquery.Reset()
}

func manyRules(n int) []spec.Entry {
	rules := make([]spec.Entry, n)
	for i := range rules {
		rules[i] = spec.Entry{Kind: spec.KindRule, Name: fmt.Sprintf("rule-%03d", i), Scope: fmt.Sprintf("area%d", i%5), Body: "x"}
	}
	return rules
}

func gitStarts(t *testing.T, run func()) int {
	t.Helper()
	trace := filepath.Join(t.TempDir(), "trace.jsonl")
	t.Setenv("GIT_TRACE2_EVENT", trace)
	run()
	data, err := os.ReadFile(trace)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	starts := 0
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var event struct {
			Event string `json:"event"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("parse Git trace: %v", err)
		}
		if event.Event == "start" {
			starts++
		}
	}
	return starts
}

func TestVisibility_GitProcessesDoNotGrowWithRules(t *testing.T) {
	count := func(n int) int {
		visibilityRepo(t)
		return gitStarts(t, func() {
			if err := validateRulesVisible(manyRules(n), visibleConfig(), rulesOpts(visibleConfig())); err != nil {
				t.Fatal(err)
			}
		})
	}
	few, many := count(2), count(60)
	if few != many {
		t.Errorf("git processes: %d for 2 rules, %d for 60 rules", few, many)
	}
}

func TestVisibility_RepeatedPassStartsNoGit(t *testing.T) {
	visibilityRepo(t)
	rules, cfg := manyRules(10), visibleConfig()
	if err := validateRulesVisible(rules, cfg, rulesOpts(cfg)); err != nil {
		t.Fatal(err)
	}
	again := gitStarts(t, func() {
		if err := validateRulesVisible(rules, cfg, rulesOpts(cfg)); err != nil {
			t.Fatal(err)
		}
	})
	if again != 0 {
		t.Errorf("second pass started %d git processes", again)
	}
}

func TestVisibility_NamesTheIgnoredRuleAmongMany(t *testing.T) {
	visibilityRepo(t)
	if err := os.WriteFile(".gitignore", []byte("/area3/\n"), 0600); err != nil {
		t.Fatal(err)
	}
	err := validateRulesVisible(manyRules(10), visibleConfig(), rulesOpts(visibleConfig()))
	want := filepath.Join("area3", ".grok", "rules", "rule-003.md")
	if err == nil || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), "rejects ignored output") {
		t.Fatalf("got %v, want refusal naming %s", err, want)
	}
	if err := validateRulesVisible(manyRules(10), visibleConfig(), emit.RulesDirOpts{Dir: "elsewhere"}); err != nil {
		t.Fatalf("unignored layout refused: %v", err)
	}
}

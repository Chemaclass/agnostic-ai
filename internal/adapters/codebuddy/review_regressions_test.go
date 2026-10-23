package codebuddy

import (
	"os"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestAgentMemoryReportsToolWideningBeforeWrites(t *testing.T) {
	testutil.TempCwd(t)
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindRule, Name: "rule", Body: "Unrelated rule."}, {Kind: spec.KindAgent, Name: "reviewer", Path: "agents/reviewer.md", Meta: map[string]any{"tools": []any{"Read"}, "x-codebuddy": map[string]any{"memory": "project"}}}})
	err := New().Emit(emit.NewSession(), b, &config.Config{OnUnsupported: "error"}, false)
	if err == nil {
		t.Fatal("memory widened a restrictive tools list silently")
	}
	for _, want := range []string{"memory", "Write", "Edit"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("diagnostic lacks %s: %v", want, err)
		}
	}
	if _, err := os.Stat(".codebuddy"); !os.IsNotExist(err) {
		t.Fatal("error policy wrote files")
	}
}

func TestNativeInstructionsRejectUnsafePlacementBeforeWrites(t *testing.T) {
	for _, c := range []struct {
		name      string
		placement any
		scope     string
		metaScope any
		override  string
		duplicate bool
	}{
		{name: "unknown", placement: "elsewhere.md"},
		{name: "type", placement: false},
		{name: "absolute", placement: "/CODEBUDDY.md"},
		{name: "traversal", placement: "../CODEBUDDY.md"},
		{name: "scope", placement: "CODEBUDDY.md", scope: "../outside"},
		{name: "absolute-meta-scope", placement: "CODEBUDDY.md", metaScope: "/outside"},
		{name: "meta-scope-type", placement: "CODEBUDDY.md", metaScope: false},
		{name: "hidden-scope", placement: ".codebuddy/CODEBUDDY.md", scope: "backend"},
		{name: "hidden-override", placement: ".codebuddy/CODEBUDDY.md", override: "custom.md"},
		{name: "duplicate", placement: "CODEBUDDY.md", duplicate: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			testutil.TempCwd(t)
			rule := spec.Entry{Kind: spec.KindRule, Name: "native", Path: "rules/native.md", Scope: c.scope, Meta: map[string]any{"x-codebuddy": map[string]any{"instruction-file": c.placement}}}
			if c.metaScope != nil {
				rule.Meta["scope"] = c.metaScope
			}
			rules := []spec.Entry{rule}
			if c.duplicate {
				rules = append(rules, rule)
			}
			cfg := &config.Config{Outputs: map[string]config.Output{"codebuddy": {File: c.override}}}
			sess := emit.NewSession()
			sess.StartCapture()
			if err := New().Emit(sess, spec.Bundle{Rules: rules}, cfg, false); err == nil {
				t.Fatal("invalid placement accepted")
			}
			if files := sess.StopCapture(); len(files) != 0 {
				t.Fatalf("invalid placement planned writes: %v", files)
			}
		})
	}
}

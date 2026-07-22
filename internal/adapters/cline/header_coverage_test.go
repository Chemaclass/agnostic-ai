package cline

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestEmit_ProvenanceHeaderOnEveryEmittedFile is the cline adapter's
// header-coverage contract: every Markdown file the adapter writes
// must carry the agnostic-ai provenance marker. Cline emits no JSON
// so there are no header exemptions.
func TestEmit_ProvenanceHeaderOnEveryEmittedFile(t *testing.T) {
	dir := testutil.TempCwd(t)
	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"cline": {WorkflowsDir: ".clinerules/workflows"},
		},
	}
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), cfg, false); err != nil {
		t.Fatalf("emit: %v", err)
	}

	var checked int
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if strings.HasPrefix(rel, ".agnostic-ai/") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !header.Has(string(data)) {
			t.Errorf("missing provenance header in %s:\n%s", rel, headFor(t, data))
			return nil
		}
		checked++
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if checked == 0 {
		t.Fatalf("no header-bearing files inspected; kit-sink bundle likely emitted nothing")
	}
}

// kitSinkBundle returns a Bundle exercising every kind the cline
// adapter declares in caps.Supports (Agent, Skill, Rule) with three
// specimens per kind.
func kitSinkBundle() spec.Bundle {
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule 1 body"},
		{Kind: spec.KindRule, Name: "r2", Path: "rules/r2.md", Body: "rule 2 body"},
		{Kind: spec.KindRule, Name: "r3", Path: "rules/r3.md", Body: "rule 3 body"},
		{Kind: spec.KindAgent, Name: "alpha", Path: "agents/alpha.md", Body: "alpha body"},
		{Kind: spec.KindAgent, Name: "beta", Path: "agents/beta.md", Body: "beta body"},
		{Kind: spec.KindAgent, Name: "gamma", Path: "agents/gamma.md", Body: "gamma body"},
		{Kind: spec.KindSkill, Name: "uno", Path: "skills/uno/SKILL.md", Body: "uno skill body"},
		{Kind: spec.KindSkill, Name: "dos", Path: "skills/dos/SKILL.md", Body: "dos skill body"},
		{Kind: spec.KindSkill, Name: "tres", Path: "skills/tres/SKILL.md", Body: "tres skill body"},
	}
	return spec.NewBundle(entries)
}

func headFor(t *testing.T, data []byte) string {
	t.Helper()
	if i := strings.IndexByte(string(data), '\n'); i >= 0 && i < 120 {
		return string(data[:i])
	}
	if len(data) > 120 {
		return string(data[:120]) + "..."
	}
	return string(data)
}

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestPerTargetModel_EmitsResolvedModelPerTarget proves the per-target
// `model:` map resolves end to end through a real sync: each target
// reads the model meant for it, an unlisted target falls back to
// `default`, and a target with neither a match nor a default emits no
// model at all (inherits nothing). A bare string still applies to every
// target. Three targets are exercised because each consumes `model`
// from a different output path: codex agent TOML, cursor subagent
// frontmatter, copilot chatmode frontmatter.
func TestPerTargetModel_EmitsResolvedModelPerTarget(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte(`version: 1
sources:
  agents: .agnostic-ai/agents
targets:
  - codex
  - cursor
  - copilot
outputs:
  copilot:
    chatmodes-dir: .github/chatmodes
gitignore:
  enabled: false
`), 0o644))

	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/agents"), 0o755))

	// alpha: full map. codex picks its own, cursor picks its own,
	// copilot is unlisted so it falls back to default.
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/agents/alpha.md"),
		[]byte(`---
name: alpha
description: agent alpha
model:
  codex: gpt-5.5
  cursor: cursor-opus
  default: gpt-4o
---

alpha body
`), 0o644))

	// beta: map without codex and without default. codex must emit no
	// model; cursor still gets its match.
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/agents/beta.md"),
		[]byte(`---
name: beta
description: agent beta
model:
  cursor: cursor-only
---

beta body
`), 0o644))

	// gamma: bare string applies to every target unchanged.
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/agents/gamma.md"),
		[]byte(`---
name: gamma
description: agent gamma
model: shared-model
---

gamma body
`), 0o644))

	runCmd(t, "sync")

	read := func(rel string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		return string(data)
	}
	wantModel := func(rel, model string) {
		t.Helper()
		if got := read(rel); !strings.Contains(got, model) {
			t.Errorf("%s: expected model %q, not found in:\n%s", rel, model, got)
		}
	}
	noModel := func(rel string) {
		t.Helper()
		for _, line := range strings.Split(read(rel), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "model:") || strings.HasPrefix(trimmed, "model =") {
				t.Errorf("%s: expected no model line, got %q", rel, trimmed)
			}
		}
	}

	// alpha: per-target pick + default fallback.
	wantModel(".codex/agents/alpha.toml", `model = "gpt-5.5"`)
	wantModel(".cursor/agents/alpha.md", "model: cursor-opus")
	wantModel(".github/chatmodes/alpha.chatmode.md", "model: gpt-4o")

	// beta: cursor match, codex dropped (no match, no default).
	wantModel(".cursor/agents/beta.md", "model: cursor-only")
	noModel(".codex/agents/beta.toml")
	noModel(".github/chatmodes/beta.chatmode.md")

	// gamma: bare string everywhere.
	wantModel(".codex/agents/gamma.toml", `model = "shared-model"`)
	wantModel(".cursor/agents/gamma.md", "model: shared-model")
	wantModel(".github/chatmodes/gamma.chatmode.md", "model: shared-model")
}

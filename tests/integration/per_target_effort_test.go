package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestPerTargetEffort_EmitsResolvedEffortPerTarget proves the per-target
// `effort:` map resolves end to end through a real sync, the same way
// `model:` does. Four targets are exercised because each treats the
// resolved value differently: Claude writes it verbatim, Qoder writes it
// verbatim including an integer budget, Factory renames it to
// `reasoningEffort` and refuses a value outside its own enum, and Cursor
// has no effort field at all.
func TestPerTargetEffort_EmitsResolvedEffortPerTarget(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte(`version: 1
sources:
  agents: .agnostic-ai/agents
targets:
  - claude
  - qoder
  - factory
  - cursor
gitignore:
  enabled: false
`), 0o644))

	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/agents"), 0o755))

	// alpha: full map. Claude takes its name, Qoder takes an integer
	// budget, Factory takes a value its enum rejects, Cursor is
	// unlisted and falls back to default it then discards.
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/agents/alpha.md"),
		[]byte(`---
name: alpha
description: agent alpha
effort:
  claude: xhigh
  qoder: 8000
  factory: max
  default: high
---

alpha body
`), 0o644))

	// beta: map naming claude only, with no default. Every other target
	// must emit no effort at all.
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/agents/beta.md"),
		[]byte(`---
name: beta
description: agent beta
effort:
  claude: xhigh
---

beta body
`), 0o644))

	// gamma: bare scalar applies to every target unchanged.
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/agents/gamma.md"),
		[]byte(`---
name: gamma
description: agent gamma
effort: high
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
	wantLine := func(rel, line string) {
		t.Helper()
		if got := read(rel); !strings.Contains(got, line) {
			t.Errorf("%s: expected %q, not found in:\n%s", rel, line, got)
		}
	}
	noKey := func(rel, key string) {
		t.Helper()
		for _, l := range strings.Split(read(rel), "\n") {
			if strings.HasPrefix(strings.TrimSpace(l), key+":") {
				t.Errorf("%s: expected no %s line, got %q", rel, key, l)
			}
		}
	}

	// alpha: per-target pick on the two targets that write the key.
	wantLine(".claude/agents/alpha.md", "effort: xhigh")
	wantLine(".qoder/agents/alpha.md", "effort: 8000")
	// `max` is outside Factory's enum, so nothing is written even
	// though the map named factory.
	noKey(".factory/droids/alpha.md", "reasoningEffort")
	noKey(".factory/droids/alpha.md", "effort")

	// beta: claude match, everything else dropped.
	wantLine(".claude/agents/beta.md", "effort: xhigh")
	noKey(".qoder/agents/beta.md", "effort")
	noKey(".factory/droids/beta.md", "reasoningEffort")

	// gamma: the bare scalar still reaches every target that has a key
	// for it, including Factory under its own spelling.
	wantLine(".claude/agents/gamma.md", "effort: high")
	wantLine(".qoder/agents/gamma.md", "effort: high")
	wantLine(".factory/droids/gamma.md", "reasoningEffort: high")
	noKey(".factory/droids/gamma.md", "effort")

	// Cursor subagents document no effort field, so no spelling of it
	// reaches any of the three files.
	for _, name := range []string{"alpha", "beta", "gamma"} {
		noKey(".cursor/agents/"+name+".md", "effort")
	}
}

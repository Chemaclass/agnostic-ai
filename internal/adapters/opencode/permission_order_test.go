package opencode

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// OpenCode takes the last matching pattern, while Claude Code lets a deny
// or ask win over any allow. A narrower allow written after a broad deny
// would bypass it, so each tool's patterns go allow, ask, then deny.
func TestEmit_DenyAndAskComeAfterOverlappingAllows(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{settingsEntry("base", map[string]any{
		"allow": []any{"Bash(git push origin:*)", "Bash(npm run:*)", "Bash", "Edit(secrets/token.txt)"},
		"ask":   []any{"Bash(git:*)"},
		"deny":  []any{"Bash(git push:*)", "Bash(npm:*)", "Edit(secrets/**)"},
	})}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw := readFile(t, filepath.Join(dir, "opencode.json"))
	for tool, order := range map[string][]string{
		"edit": {`"secrets/token.txt": "allow"`, `"secrets/**": "deny"`},
		"bash": {`"*": "allow"`, `"git push origin *": "allow"`, `"npm run *": "allow"`, `"git *": "ask"`, `"git push *": "deny"`, `"npm *": "deny"`},
	} {
		start := strings.Index(raw, `"`+tool+`": {`)
		if start < 0 {
			t.Fatalf("no %s object in:\n%s", tool, raw)
		}
		data := raw[start:]
		data = data[:strings.Index(data, "}")]
		at := -1
		for _, want := range order {
			i := strings.Index(data, want)
			if i < 0 || i < at {
				t.Errorf("%s is missing or out of order in:\n%s", want, data)
			}
			at = i
		}
	}
}

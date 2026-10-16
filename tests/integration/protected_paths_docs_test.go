package integration

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestDocs_TargetPagesStateProtectedPathEnforcement(t *testing.T) {
	t.Parallel()
	mechanisms := map[string]struct{ enforcement, text string }{
		"claude": {"permission", "Edit(/<path>)"},
		"cursor": {"permission", "Write(<path>)"},
		"codex":  {"hook", "apply_patch"},
		"gemini": {"hook", "BeforeTool"},
	}
	targets := adapters.Names()
	for target := range mechanisms {
		if !slices.Contains(targets, target) {
			t.Errorf("native mechanism names an unregistered target %q", target)
		}
	}
	takesSettings := map[string]bool{}
	for _, row := range adapters.CapabilityMatrix() {
		takesSettings[row.Name] = slices.Contains(row.Supports, spec.KindSettings)
	}
	for _, target := range targets {
		want := "Not enforced."
		enforcement := adapters.ProtectedPathsEnforcement(target)
		if mechanism, known := mechanisms[target]; known {
			if enforcement != mechanism.enforcement {
				t.Errorf("%s native mechanism expects %q enforcement, adapter reports %q", target, mechanism.enforcement, enforcement)
			}
			want = mechanism.text
		} else if enforcement != "" {
			t.Errorf("%s has %q enforcement but no documented native mechanism assertion", target, enforcement)
			continue
		}
		data, err := os.ReadFile(filepath.Join("..", "..", "docs", "site", "content", "docs", "targets", target+".md"))
		if err != nil {
			t.Errorf("%s: %v", target, err)
			continue
		}
		_, section, found := strings.Cut(string(data), "\n## Protected paths\n\n")
		if !found {
			t.Errorf("%s page has no Protected paths section", target)
			continue
		}
		paragraph, _, _ := strings.Cut(section, "\n")
		if !strings.Contains(paragraph, want) {
			t.Errorf("%s Protected paths starts %q, want native mechanism %q", target, paragraph, want)
		}
		if says := strings.Contains(paragraph, "takes no settings specs"); says == takesSettings[target] {
			t.Errorf("%s page says takes-no-settings=%v, but the adapter takes settings=%v", target, says, takesSettings[target])
		}
	}
}

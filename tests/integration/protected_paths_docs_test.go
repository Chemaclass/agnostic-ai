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

// Every target page says whether protected paths are enforced there,
// and how, in words that match what the adapter does (#1497).
func TestDocs_TargetPagesStateProtectedPathEnforcement(t *testing.T) {
	t.Parallel()
	takesSettings := map[string]bool{}
	for _, row := range adapters.CapabilityMatrix() {
		takesSettings[row.Name] = slices.Contains(row.Supports, spec.KindSettings)
	}
	for _, target := range adapters.Names() {
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
		want := "Advisory."
		if enforcement := adapters.ProtectedPathsEnforcement(target); enforcement != "" {
			want = "Enforced (" + enforcement + ")."
		}
		if !strings.HasPrefix(section, want) {
			first, _, _ := strings.Cut(section, "\n")
			t.Errorf("%s Protected paths starts %q, want %q", target, first, want)
		}
		paragraph, _, _ := strings.Cut(section, "\n")
		if says := strings.Contains(paragraph, "takes no settings specs"); says == takesSettings[target] {
			t.Errorf("%s page says takes-no-settings=%v, but the adapter takes settings=%v", target, says, takesSettings[target])
		}
	}
}

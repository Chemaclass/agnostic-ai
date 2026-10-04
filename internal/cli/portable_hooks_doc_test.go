package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// portableHookDocNames are the hooks page's names for the targets the
// portable form translates for, in the table's order.
var portableHookDocNames = []struct{ target, name string }{
	{"claude", "Claude Code"}, {"codex", "Codex"}, {"gemini", "Gemini"}, {"factory", "Factory"},
	{"qoder", "Qoder"}, {"openhands", "OpenHands"}, {"goose", "Goose"},
	{"augment", "Augment"}, {"crush", "Crush"}, {"windsurf", "Windsurf"},
	{"copilot", "Copilot"}, {"cline", "Cline"},
}

// portableHookDocRow is the hooks page's table row for target.
func portableHookDocRow(target, name string) string {
	var events, kinds []string
	for _, on := range spec.PortableHookEvents {
		if _, ok := spec.PortableHookEvent(target, on); ok {
			events = append(events, "`"+on+"`")
		}
	}
	for _, kind := range []string{"shell", "edit", "read", "web", "mcp:<server>"} {
		lookup := kind
		if kind == "mcp:<server>" {
			lookup = "mcp:<server>"
		}
		if m, ok := spec.HookToolMatcher(target, lookup); ok {
			kinds = append(kinds, "`"+kind+"` `"+strings.ReplaceAll(m, "|", `\|`)+"`")
		}
	}
	if len(kinds) == 0 {
		kinds = []string{"`any` only"}
	}
	return "| " + name + " | " + strings.Join(events, ", ") + " | " + strings.Join(kinds, ", ") + " |"
}

func TestHooksDoc_PortableTableMatchesTheTranslation(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "site", "content", "docs", "spec-format", "hooks.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(portableHookDocNames) != len(spec.PortableHookTargets()) {
		t.Fatalf("name every target in %v", spec.PortableHookTargets())
	}
	for _, n := range portableHookDocNames {
		if row := portableHookDocRow(n.target, n.name); !strings.Contains(string(data), row+"\n") {
			t.Errorf("hooks.md misses the row:\n%s", row)
		}
	}
}

package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// nestedClaudeCopy is a hand-written nested CLAUDE.md whose whole text a
// rule scoped to its directory holds, as `import claude` leaves it.
// Claude Code loads it beside the rule sync writes, and next to a scoped
// AGENTS.md it stops sync. sum proves the bytes doctor compared.
type nestedClaudeCopy struct{ path, rule, sum string }

// nestedClaudeCopies lists the nested CLAUDE.md files a rule holds when
// claude is a target. Only an exact match counts: the trimmed file text
// must equal the body of a rule for Claude scoped to that directory.
func nestedClaudeCopies(cfg *config.Config, b spec.Bundle) []nestedClaudeCopy {
	if !slices.Contains(cfg.Targets, "claude") {
		return nil
	}
	var out []nestedClaudeCopy
	seen := map[string]bool{}
	for _, r := range b.Rules {
		scope, err := spec.RuleScope(r)
		if err != nil || scope == "" || !r.EmitsTo("claude") {
			continue
		}
		p := filepath.Join(filepath.FromSlash(scope), claudeMainFile)
		if seen[p] || cfg.IsUnmanaged(p) || underSymlinkedDir(p) {
			continue
		}
		if fi, err := os.Lstat(p); err != nil || !fi.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		text := string(data)
		if header.Has(text) || comparableText(text) != comparableText(r.Body) {
			continue
		}
		seen[p] = true
		out = append(out, nestedClaudeCopy{path: p, rule: r.Name, sum: adapters.ContentSum(text)})
	}
	slices.SortFunc(out, func(a, b nestedClaudeCopy) int { return strings.Compare(a.path, b.path) })
	return out
}

func comparableText(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
}

// reportNestedClaudeCopies lists the copies under the drift section.
func reportNestedClaudeCopies(cmd *cobra.Command, copies []nestedClaudeCopy) {
	if len(copies) == 0 {
		return
	}
	cmd.Printf("  ✗ %d nested CLAUDE.md file(s) a rule already holds, so Claude Code loads the text twice (run `agnostic-ai doctor --fix` to remove):\n", len(copies))
	for _, c := range copies {
		cmd.Printf("      - %s  (rule %s)\n", filepath.ToSlash(c.path), c.rule)
	}
}

// removeNestedClaudeCopies deletes each copy whose bytes have not changed
// since doctor compared them, and returns how many it removed.
func removeNestedClaudeCopies(copies []nestedClaudeCopy, backup bool) (int, error) {
	sess := adapters.NewSession()
	sess.SetBackup(backup)
	removed := 0
	for _, c := range copies {
		ok, err := sess.RemoveCopy(c.path, c.sum, false)
		if err != nil {
			return removed, err
		}
		if ok {
			removed++
		}
	}
	return removed, nil
}

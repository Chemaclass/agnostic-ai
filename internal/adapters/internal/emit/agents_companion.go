package emit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// AgentsCompanionFile is the file a project adds next to an AGENTS.md so
// Claude Code, which does not read AGENTS.md when a CLAUDE.md exists,
// loads it through an `@AGENTS.md` import.
const AgentsCompanionFile = "CLAUDE.md"

var agentsImportLines = map[string]bool{"@AGENTS.md": true, "@./AGENTS.md": true}

// SplitAgentsCompanion reports whether text, a CLAUDE.md, imports the
// AGENTS.md beside it, and returns what is left without the import line
// and a leading `# CLAUDE.md` title. Lines inside code fences never count.
func SplitAgentsCompanion(text string) (rest string, ok bool) {
	if header.Has(text) {
		return "", false
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	kept := make([]string, 0, len(lines))
	inFence, started := false, false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
		}
		if !started && trimmed != "" {
			started = true
			if !inFence && strings.EqualFold(trimmed, "# "+AgentsCompanionFile) {
				continue
			}
		}
		if !inFence && agentsImportLines[trimmed] {
			ok = true
			continue
		}
		kept = append(kept, line)
	}
	if !ok {
		return "", false
	}
	return strings.TrimSpace(strings.Join(kept, "\n")), true
}

// IsAgentsCompanion reports whether text, a CLAUDE.md, holds nothing but
// an import of the AGENTS.md beside it and a `# CLAUDE.md` title.
func IsAgentsCompanion(text string) bool {
	rest, ok := SplitAgentsCompanion(text)
	return ok && rest == ""
}

// AgentsCompanionDirs returns the scopes whose CLAUDE.md companion the
// Claude adapter replaces: it writes their rules to its rules directory
// and sync owns the companion path. rules are Claude's prepared rules.
// Sync may only allow and remove a companion in these directories.
func AgentsCompanionDirs(cfg *config.Config, rules []spec.Entry) []string {
	if OutputRulesFile(cfg, "claude", "") != "" {
		return nil
	}
	seen := map[string]bool{}
	var dirs []string
	for _, r := range rules {
		dir := r.EffectiveScope()
		if dir == "" || seen[dir] {
			continue
		}
		seen[dir] = true
		if !cfg.IsUnmanaged(filepath.Join(dir, AgentsCompanionFile)) {
			dirs = append(dirs, dir)
		}
	}
	return dirs
}

// RemoveAgentsCompanion deletes the CLAUDE.md in dir when it only imports
// the AGENTS.md beside it and covered, the rules Claude Code now loads
// for dir, holds that file's text. Left in place, the companion would
// load the same instructions a second time. Under backup mode the file
// is kept as `<path>.bak` so revert can restore it.
func (s *Session) RemoveAgentsCompanion(dir, covered string, dryRun bool) error {
	path := filepath.Join(dir, AgentsCompanionFile)
	data, err := os.ReadFile(path)
	if IsAbsent(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if !IsAgentsCompanion(string(data)) {
		return nil
	}
	sibling := filepath.Join(dir, "AGENTS.md")
	agents, err := os.ReadFile(sibling)
	if IsAbsent(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", sibling, err)
	}
	if !header.Has(string(agents)) {
		if _, missing := lineMissingFrom(string(agents), covered); missing {
			return nil
		}
	}
	if s.skipUnmanaged(path) {
		return nil
	}
	_, err = s.remove(path, "", data, true, dryRun)
	return err
}

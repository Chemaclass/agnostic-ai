package emit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
)

// AgentsCompanionFile is the file a project adds next to an AGENTS.md so
// Claude Code, which does not read AGENTS.md when a CLAUDE.md exists,
// loads it through an `@AGENTS.md` import.
const AgentsCompanionFile = "CLAUDE.md"

var agentsImportLines = map[string]bool{"@AGENTS.md": true, "@./AGENTS.md": true}

// SplitAgentsCompanion reports whether text, a CLAUDE.md, imports the
// AGENTS.md beside it, and returns what is left without the import line
// and the leading title. Lines inside code fences never count.
func SplitAgentsCompanion(text string) (rest string, ok bool) {
	if header.Has(text) {
		return "", false
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	kept := make([]string, 0, len(lines))
	inFence, titled := false, false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
		}
		if !titled && trimmed != "" {
			titled = true
			if !inFence && strings.HasPrefix(trimmed, "# ") {
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
// an import of the AGENTS.md beside it, headings aside.
func IsAgentsCompanion(text string) bool {
	rest, ok := SplitAgentsCompanion(text)
	if !ok {
		return false
	}
	for _, l := range strings.Split(rest, "\n") {
		if c := strings.TrimSpace(l); c != "" && !isHeading(c) {
			return false
		}
	}
	return true
}

// RemoveAgentsCompanion deletes the CLAUDE.md in dir when it only imports
// the AGENTS.md beside it and covered, the rules Claude Code now loads
// for dir, holds that file's text. Left in place, the companion would
// load the same instructions a second time.
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
	_, err = s.remove(path, "", data, dryRun)
	return err
}

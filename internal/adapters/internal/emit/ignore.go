package emit

import (
	"fmt"
	"os"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// IgnoreBody concatenates the patterns of ignore specs into one
// gitignore-syntax block in spec order, separated by blank lines.
// Only outer line breaks are trimmed: spaces and tabs can be patterns.
// Returns "" when no spec contributes content.
func IgnoreBody(ignores []spec.Entry) string {
	parts := make([]string, 0, len(ignores))
	for _, e := range ignores {
		for _, block := range ignoreSpecPatterns(strings.ReplaceAll(e.Body, "\r\n", "\n")) {
			block = strings.Trim(block, "\n")
			if strings.Trim(block, " \n") != "" {
				parts = append(parts, block)
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

// ignoreSpecPatterns returns the pattern blocks of one spec body. When the
// body has fenced code blocks, they hold the patterns and the text around
// them is prose: Markdown formatters such as Prettier rewrite `*` and `_`
// in plain text but leave code alone (#1275). A body without a fence is
// one block, read whole.
func ignoreSpecPatterns(body string) []string {
	var blocks []string
	lines := strings.Split(body, "\n")
	for i := 0; i < len(lines); i++ {
		indent, fence := openingCodeFence(lines[i])
		if fence == "" {
			continue
		}
		var content []string
		for i++; i < len(lines) && !closesCodeFence(lines[i], fence); i++ {
			content = append(content, trimFenceIndent(lines[i], indent))
		}
		blocks = append(blocks, strings.Join(content, "\n"))
	}
	if blocks == nil {
		return []string{body}
	}
	return blocks
}

// openingCodeFence reports the indent and marker of a CommonMark fence
// opener: up to three spaces, then three or more backticks or tildes. A
// backtick info string cannot hold a backtick, or the line is inline code.
func openingCodeFence(line string) (int, string) {
	rest := strings.TrimLeft(line, " ")
	indent := len(line) - len(rest)
	if indent > 3 || rest == "" || (rest[0] != '`' && rest[0] != '~') {
		return 0, ""
	}
	info := strings.TrimLeft(rest, rest[:1])
	fence := rest[:len(rest)-len(info)]
	if len(fence) < 3 || (fence[0] == '`' && strings.Contains(info, "`")) {
		return 0, ""
	}
	return indent, fence
}

// closesCodeFence reports whether line closes a block opened by fence: up
// to three spaces, a run of the same character at least as long, and
// nothing after it but spaces or tabs.
func closesCodeFence(line, fence string) bool {
	rest := strings.TrimLeft(line, " ")
	if len(line)-len(rest) > 3 {
		return false
	}
	after := strings.TrimLeft(rest, fence[:1])
	return len(rest)-len(after) >= len(fence) && strings.Trim(after, " \t") == ""
}

// trimFenceIndent drops up to indent leading spaces, as CommonMark does
// for the content of an indented fence.
func trimFenceIndent(line string, indent int) string {
	for i := 0; i < indent && strings.HasPrefix(line, " "); i++ {
		line = line[1:]
	}
	return line
}

// WriteIgnoreFile writes the combined ignore patterns to path with a
// shell-style (`#`) provenance header, matching the comment syntax
// every gitignore-style ignore file understands: cursor `.cursorignore`,
// gemini `.geminiignore`, aider `.aiderignore`, windsurf `.devinignore` and `.windsurfignore`,
// kiro `.kiroignore`, trae `.trae/.ignore`, junie `.aiignore`. No-op
// when the patterns are empty so a target never writes a surprise empty
// ignore file.
//
// A hand-authored file is replaced only when every existing pattern
// survives unchanged and in order, with no added negations (#761).
// Extra exclusion patterns are safe. This keeps import-then-sync usable
// without guessing which files a negation can make readable. If the
// check cannot establish preservation, the file stays untouched.
//
// The check is skipped in dry-run (nothing is written, so nothing is at
// risk) and when `outputs.<target>.provenance-header` is false, since
// that flag removes the very marker the check reads to recognize
// agnostic-ai's own output. Capture mode still checks, so `sync --check`
// reports the problem before a real sync runs into it.
func (s *Session) WriteIgnoreFile(ignores []spec.Entry, target, path string, dryRun bool) error {
	body := IgnoreBody(ignores)
	if body == "" {
		return nil
	}
	// Ownership first: a user-owned ignore file is skipped, not refused.
	if s.skipUnmanaged(path) {
		return nil
	}
	if !dryRun && ProvenanceEnabled() {
		if err := refuseIgnoreOverwrite(target, path, body); err != nil {
			return err
		}
	}
	return s.WriteFile(path, WithHeader(body+"\n", FormatShell), dryRun)
}

// maxNamedIgnorePatterns caps how many patterns the refusal error spells
// out before it switches to a count, so a large hand-authored file
// still produces a readable one-line message.
const maxNamedIgnorePatterns = 5

// refuseIgnoreOverwrite returns an error when path holds hand-authored
// patterns whose exclusions body cannot be shown to preserve. A missing,
// empty, or agnostic-ai-generated file returns nil.
func refuseIgnoreOverwrite(target, path, body string) error {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return nil
	}
	if header.Has(string(data)) {
		return nil
	}
	reason := ignoreOverwriteRisk(string(data), body)
	if reason == "" {
		return nil
	}
	return errs.Coded(errs.CodeIgnoreOverwrite,
		"%s: hand-authored ignore file cannot be safely overwritten: %s. Run `agnostic-ai import %s` to copy its patterns into an ignore spec, then keep their order and review any added negations before syncing again",
		path, reason, target)
}

// ignoreOverwriteRisk requires existing patterns to remain an ordered
// subsequence, allowing only extra exclusions. An unmatched negation
// can re-include excluded files even when no old pattern is missing.
func ignoreOverwriteRisk(existing, body string) string {
	// Git skips a BOM only at the start of the file. The generated body
	// follows a provenance header, so a BOM there is a pattern character.
	patterns := ignorePatterns(strings.TrimPrefix(existing, "\uFEFF"))
	if len(patterns) == 0 {
		return ""
	}
	next := 0
	for _, pattern := range ignorePatterns(body) {
		if next < len(patterns) && pattern == patterns[next] {
			next++
			continue
		}
		if strings.HasPrefix(pattern, "!") {
			return fmt.Sprintf("new or reordered negation %q could re-include excluded files", pattern)
		}
	}
	if next < len(patterns) {
		return "existing patterns are missing or reordered: " + namedIgnorePatterns(patterns[next:])
	}
	return ""
}

// ignorePatterns drops only comments and blank lines, normalizing CRLF.
// Spaces are compared verbatim to avoid changing escaped trailing spaces
// or treating an indented hash or exclamation mark as a prefix.
func ignorePatterns(text string) []string {
	var out []string
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if strings.Trim(line, " ") == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

// namedIgnorePatterns quotes whitespace and caps the list, with a
// trailing count for any remaining patterns.
func namedIgnorePatterns(patterns []string) string {
	named := make([]string, 0, min(len(patterns), maxNamedIgnorePatterns))
	for _, pattern := range patterns[:min(len(patterns), maxNamedIgnorePatterns)] {
		named = append(named, fmt.Sprintf("%q", pattern))
	}
	if len(patterns) <= maxNamedIgnorePatterns {
		return strings.Join(named, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(named, ", "), len(patterns)-maxNamedIgnorePatterns)
}

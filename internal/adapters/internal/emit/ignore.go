package emit

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// IgnoreBody concatenates the bodies of ignore specs into one
// gitignore-syntax block: each spec's trimmed body, joined by a blank
// line, in spec order. Returns "" when no spec contributes content.
func IgnoreBody(ignores []spec.Entry) string {
	parts := make([]string, 0, len(ignores))
	for _, e := range ignores {
		if body := strings.TrimSpace(e.Body); body != "" {
			parts = append(parts, body)
		}
	}
	return strings.Join(parts, "\n\n")
}

// WriteIgnoreFile writes the combined ignore patterns to path with a
// shell-style (`#`) provenance header, matching the comment syntax
// every gitignore-style ignore file understands: cursor `.cursorignore`,
// gemini `.geminiignore`, aider `.aiderignore`, windsurf `.devinignore`,
// kiro `.kiroignore`, trae `.trae/.ignore`, junie `.aiignore`. No-op
// when the patterns are empty so a target never writes a surprise empty
// ignore file.
//
// A hand-authored ignore file is never silently replaced. These files
// exist to keep credentials out of agent context, so dropping a pattern
// nobody asked to drop is the worst shape a lost-content bug takes
// (#754). When path holds content without the agnostic-ai provenance
// header and the new body would not reproduce every pattern already
// there, the write is refused and the error names both the patterns at
// risk and the `agnostic-ai import <target>` command that copies them
// into a spec. Same stance MergeJSONFile takes on a JSON config it
// cannot parse: when the content is not ours, write nothing.
//
// An overwrite that reproduces every on-disk pattern proceeds, so the
// import-then-sync migration needs no manual cleanup step and a repo
// whose hand-authored file the specs already cover never sees an error.
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
	if !dryRun && ProvenanceEnabled() {
		if err := refuseIgnoreOverwrite(target, path, body); err != nil {
			return err
		}
	}
	return s.WriteFile(path, WithHeader(body+"\n", FormatShell), dryRun)
}

// maxNamedLostPatterns caps how many patterns the refusal error spells
// out before it switches to a count, so a large hand-authored file
// still produces a readable one-line message.
const maxNamedLostPatterns = 5

// refuseIgnoreOverwrite returns an error when path holds hand-authored
// ignore patterns that body would drop. A missing, empty, or
// agnostic-ai-generated file returns nil, as does one whose every
// pattern body already carries.
func refuseIgnoreOverwrite(target, path, body string) error {
	data, err := os.ReadFile(path)
	if err != nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if header.Has(string(data)) {
		return nil
	}
	lost := lostIgnorePatterns(string(data), body)
	if len(lost) == 0 {
		return nil
	}
	return errs.Coded(errs.CodeIgnoreOverwrite,
		"%s: hand-authored, and overwriting it would drop patterns that keep files out of agent context. Would be dropped: %s. Run `agnostic-ai import %s` to copy them into an ignore spec, then sync again",
		path, namedLostPatterns(lost), target)
}

// lostIgnorePatterns returns the patterns in existing that body does
// not carry, sorted for a stable error message. Comment and blank lines
// are ignored on both sides: they exclude nothing, so losing one costs
// the user a note rather than a file left readable by the agent.
func lostIgnorePatterns(existing, body string) []string {
	kept := ignorePatternSet(body)
	var lost []string
	for pattern := range ignorePatternSet(existing) {
		if !kept[pattern] {
			lost = append(lost, pattern)
		}
	}
	sort.Strings(lost)
	return lost
}

// ignorePatternSet splits gitignore-syntax text into its set of
// meaningful patterns.
func ignorePatternSet(text string) map[string]bool {
	out := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out[line] = true
	}
	return out
}

// namedLostPatterns renders up to maxNamedLostPatterns of lost, with a
// trailing count for the rest.
func namedLostPatterns(lost []string) string {
	if len(lost) <= maxNamedLostPatterns {
		return strings.Join(lost, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(lost[:maxNamedLostPatterns], ", "), len(lost)-maxNamedLostPatterns)
}

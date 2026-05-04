package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

const (
	gitignoreBlockStart = "# >>> agnostic-ai (managed) >>>"
	gitignoreBlockEnd   = "# <<< agnostic-ai (managed) <<<"
	gitignoreBlockNote  = "# Generated paths. Edit specs, not this block. Run `agnostic-ai sync` to refresh."
)

// normalizeAndSort converts a slice of filesystem paths to gitignore
// entries: forward slashes, leading `./` trimmed, deduplicated, sorted.
func normalizeAndSort(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		seen[normalizeGitignorePath(p)] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// normalizeGitignorePath converts a filesystem path to a gitignore entry:
// always forward-slashed, leading "./" stripped.
func normalizeGitignorePath(p string) string {
	p = filepath.ToSlash(p)
	p = strings.TrimPrefix(p, "./")
	return p
}

// updateGitignore rewrites the managed block in `.gitignore` (or
// cfg.Gitignore.Path) with the provided entries. Lines outside the block
// are preserved as-is. The file is created if missing. An empty entries
// list removes the block.
func updateGitignore(cfg *config.Config, entries []string) error {
	path := ".gitignore"
	if cfg.Gitignore.Path != "" {
		path = cfg.Gitignore.Path
	}
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read %s: %w", path, err)
	}
	updated := replaceManagedBlock(string(existing), entries)
	if updated == string(existing) {
		return nil
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// replaceManagedBlock returns content with the agnostic-ai managed block
// rewritten to list entries. If entries is empty, the block is removed.
// If no prior block exists, a new one is appended (with a leading blank
// line when the file is non-empty).
func replaceManagedBlock(content string, entries []string) string {
	startIdx := strings.Index(content, gitignoreBlockStart)
	if startIdx >= 0 {
		endIdx := strings.Index(content[startIdx:], gitignoreBlockEnd)
		if endIdx < 0 {
			// Truncated block; treat the rest of the file as the block to
			// avoid duplicating markers on the next run.
			endIdx = len(content) - startIdx
		} else {
			endIdx += len(gitignoreBlockEnd)
		}
		before := content[:startIdx]
		after := strings.TrimLeft(content[startIdx+endIdx:], "\n")
		newBlock := renderBlock(entries)
		if newBlock == "" {
			before = strings.TrimRight(before, "\n")
			if before != "" && after != "" {
				return before + "\n\n" + after
			}
			if before != "" {
				return before + "\n"
			}
			return after
		}
		joined := strings.TrimRight(before, "\n") + "\n\n" + newBlock
		if after != "" {
			joined += "\n" + after
		} else {
			joined += "\n"
		}
		if before == "" {
			joined = strings.TrimLeft(joined, "\n")
		}
		return joined
	}

	newBlock := renderBlock(entries)
	if newBlock == "" {
		return content
	}
	if content == "" {
		return newBlock + "\n"
	}
	return strings.TrimRight(content, "\n") + "\n\n" + newBlock + "\n"
}

func renderBlock(entries []string) string {
	if len(entries) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(gitignoreBlockStart)
	sb.WriteString("\n")
	sb.WriteString(gitignoreBlockNote)
	sb.WriteString("\n")
	for _, e := range entries {
		sb.WriteString(e)
		sb.WriteString("\n")
	}
	sb.WriteString(gitignoreBlockEnd)
	return sb.String()
}

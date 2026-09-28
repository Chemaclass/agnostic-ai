package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// codexReviewHeadings are the H2 slugs Codex code review reads guidance
// from. learn.chatgpt.com/docs/third-party/github names `## Code Review
// Rules`; `## Review guidelines` is the older spelling projects still
// carry.
var codexReviewHeadings = map[string]bool{
	"code-review-rules": true,
	"review-guidelines": true,
}

// importCodexReviews reads the code review section of every AGENTS.md
// into a review spec with the file's scope (#1341). The section sync
// writes is found by its markers; a hand-written one by its heading, in
// a nested AGENTS.md only: the root file is mirrored whole to
// AGNOSTIC_AI.md, which would keep a second copy of the section.
func importCodexReviews(root string, src config.Sources) (int, error) {
	if src.Reviews == "" {
		return 0, nil
	}
	files, err := findHierarchicalMainFiles(root, "AGENTS.md", src)
	if err != nil {
		return 0, err
	}
	dstDir := filepath.Join(root, src.Reviews)
	used := map[string]int{}
	count := 0
	for _, f := range files {
		raw, err := readEntryFile(root, f.path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return count, fmt.Errorf("read %s: %w", f.path, err)
		}
		text := strings.ReplaceAll(string(raw), "\r\n", "\n")
		body := codexGeneratedReview(text)
		if body == "" && f.globs != "" {
			_, body = splitCodexReviewSections(text)
		}
		if body == "" {
			continue
		}
		scope := strings.TrimSuffix(f.globs, "/**")
		if err := writeReviewSpec(dstDir, cursorReviewSpecName(used, scope), scope, body); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// codexGeneratedReview returns the body of the marked review section,
// without its heading.
func codexGeneratedReview(text string) string {
	inner := strings.TrimSpace(extractMarkedBlock(text, adapters.ReviewsStartMarker, adapters.ReviewsEndMarker))
	if m := h2HeadingRE.FindString(firstLine(inner)); m != "" {
		inner = strings.TrimSpace(strings.TrimPrefix(inner, m))
	}
	return inner
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

// splitCodexReviewSections removes the code review H2 sections from text
// and returns the rest and their bodies joined, headings dropped.
// Headings inside fenced code blocks do not count.
func splitCodexReviewSections(text string) (rest, review string) {
	lines := strings.Split(text, "\n")
	var kept, reviews []string
	inFence, inReview := false, false
	for _, line := range lines {
		if fenceRE.MatchString(line) {
			inFence = !inFence
		}
		if !inFence {
			if m := h2HeadingRE.FindStringSubmatch(line); m != nil {
				inReview = codexReviewHeadings[slugify(m[1])]
				if inReview {
					continue
				}
			}
		}
		if inReview {
			reviews = append(reviews, line)
		} else {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n"), strings.TrimSpace(strings.Join(reviews, "\n"))
}

// writeReviewSpec writes one review spec, with scope in the frontmatter
// when it is not the root.
func writeReviewSpec(dstDir, name, scope, body string) error {
	var doc strings.Builder
	doc.WriteString("---\nname: " + name + "\n")
	if scope != "" {
		doc.WriteString(yamlFrontmatterLine("scope", scope))
	}
	doc.WriteString("---\n\n" + strings.Trim(body, "\n") + "\n")
	if err := importMkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dstDir, err)
	}
	out := filepath.Join(dstDir, name+".md")
	if err := importWriteFile(out, []byte(doc.String()), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", out, err)
	}
	return nil
}

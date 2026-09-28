package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
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
		if body == "" {
			body = handWrittenReview(f, text)
		}
		if body == "" {
			continue
		}
		scope := strings.TrimSuffix(f.globs, "/**")
		wrote, err := writeReviewSpec(dstDir, cursorReviewSpecName(used, scope), scope, body)
		if err != nil {
			return count, err
		}
		if wrote {
			count++
		}
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
// when it is not the root. It writes nothing when a spec under dstDir
// already carries the same scope and body: sync wrote the native file
// from that spec, and a second copy would render the text twice.
func writeReviewSpec(dstDir, name, scope, body string) (bool, error) {
	exists, err := reviewSpecExists(dstDir, scope, body)
	if err != nil || exists {
		return false, err
	}
	var doc strings.Builder
	doc.WriteString("---\nname: " + name + "\n")
	if scope != "" {
		doc.WriteString(yamlFrontmatterLine("scope", scope))
	}
	doc.WriteString("---\n\n" + strings.Trim(body, "\n") + "\n")
	if err := importMkdirAll(dstDir, 0o755); err != nil {
		return false, fmt.Errorf("mkdir %s: %w", dstDir, err)
	}
	out := filepath.Join(dstDir, name+".md")
	if err := importWriteFile(out, []byte(doc.String()), 0o644); err != nil {
		return false, fmt.Errorf("write %s: %w", out, err)
	}
	return true, nil
}

// reviewSpecExists reports whether a review spec under dir has scope and
// body. A spec's scope is its folder under dir, or else its frontmatter
// `scope`, the order the loader uses.
func reviewSpecExists(dir, scope, body string) (bool, error) {
	want := strings.TrimSpace(body)
	found := false
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, fs.ErrNotExist) {
				return filepath.SkipAll
			}
			return walkErr
		}
		if d.IsDir() || filepath.Ext(p) != ".md" {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return fmt.Errorf("read %s: %w", p, err)
		}
		entry, err := spec.ParseMarkdownBytes(spec.KindReview, data)
		if err != nil {
			return nil
		}
		if rel, err := filepath.Rel(dir, filepath.Dir(p)); err == nil && rel != "." {
			entry.Scope = filepath.ToSlash(rel)
		}
		got, err := spec.NormalizeScope(entry.EffectiveScope())
		if err == nil && got == scope && strings.TrimSpace(entry.Body) == want {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found, err
}

// handWrittenReview returns the code review sections a nested AGENTS.md
// carries outside its generated blocks. A heading inside the rules block
// belongs to a rule's body, so it does not count.
func handWrittenReview(f hierarchicalFile, text string) string {
	if f.globs == "" {
		return ""
	}
	_, review := splitCodexReviewSections(adapters.StripGeneratedAppendices(text))
	return review
}

// rulesTextIsWholeFile reports whether the rules import reads f whole: a
// nested AGENTS.md with no generated rules block. Otherwise it reads the
// block alone, and a review heading there is part of a rule.
func rulesTextIsWholeFile(f hierarchicalFile, text string) bool {
	return f.globs != "" && !strings.Contains(text, adapters.RulesStartMarker)
}

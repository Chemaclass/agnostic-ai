package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// cursorReviewFile is the Bugbot guidance file Cursor reads from the root
// `.cursor/` and from every `<dir>/.cursor/` on the way up from a changed
// file (cursor.com/docs/bugbot).
const cursorReviewFile = "BUGBOT.md"

// importCursorReviews reads every hand-authored `.cursor/BUGBOT.md`, at
// the root and below project subdirectories, into a review spec with the
// same scope, reversing the cursor emit (#1276). The root file becomes
// `review`, matching `import goose`, so the spec stays target-neutral;
// a nested one is named after its scope and sets `scope:`.
//
// A file carrying the provenance header imports nothing: specs sharing a
// scope concatenate into one BUGBOT.md, so reading it back would emit
// them twice on the next sync.
func importCursorReviews(root string, src config.Sources) (int, error) {
	if src.Reviews == "" {
		return 0, nil
	}
	dirs, err := findScopedSkillDirs(root, ".cursor")
	if err != nil {
		return 0, err
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].scope < dirs[j].scope })
	dstDir := filepath.Join(root, src.Reviews)
	used := map[string]int{}
	count := 0
	for _, dir := range dirs {
		path := filepath.Join(dir.path, cursorReviewFile)
		data, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return count, fmt.Errorf("read %s: %w", path, err)
		}
		if header.Has(string(data)) {
			continue
		}
		body := strings.TrimPrefix(string(data), "\uFEFF")
		body = strings.Trim(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
		if strings.TrimSpace(body) == "" {
			continue
		}
		wrote, err := writeReviewSpec(dstDir, cursorReviewSpecName(used, dir.scope), dir.scope, body)
		if err != nil {
			return count, err
		}
		if wrote {
			count++
		}
	}
	return count, nil
}

// cursorReviewSpecName names the review spec for one BUGBOT.md scope.
// Names must be unique across scopes, or the loader shadows all but one.
func cursorReviewSpecName(used map[string]int, scope string) string {
	slug := slugify(scope)
	if slug == "" {
		slug = rootReviewSpecName
	}
	return dedupSlug(used, slug)
}

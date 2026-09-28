package emit

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
)

// CheckScopePath checks the nearest existing ancestor without scanning a tree.
// Capture and WASM may plan files in directories that do not exist yet.
func CheckScopePath(path string) error {
	if runtime.GOOS == "js" {
		return nil
	}
	root, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("scope working directory: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("scope root: %w", err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	for p := abs; ; p = filepath.Dir(p) {
		resolved, err := filepath.EvalSymlinks(p)
		if err == nil {
			rel, err := filepath.Rel(root, resolved)
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return fmt.Errorf("%s: scoped output escapes the project", path)
			}
			return nil
		}
		if info, statErr := os.Lstat(p); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s: cannot resolve scoped symlink: %w", p, err)
		}
		if !os.IsNotExist(err) {
			return fmt.Errorf("%s: %w", path, err)
		}
		if filepath.Dir(p) == p {
			return fmt.Errorf("%s: no existing ancestor for scoped output", path)
		}
	}
}

// CheckScopedDestination protects newly introduced instruction files and aliases
// that the same host would load instead of (or alongside) the generated file.
// content is what sync writes at path. A hand-authored file there passes
// when every line of its text is in content, as after `import` captured
// it (#1269).
func CheckScopedDestination(path, content string) error {
	if runtime.GOOS == "js" {
		return nil
	}
	if err := CheckScopePath(path); err != nil {
		return err
	}
	candidates := []string{path}
	switch filepath.Base(path) {
	case "AGENTS.md":
		for _, name := range []string{"AGENTS.override.md", "WARP.md", "CLAUDE.md", ".goosehints"} {
			candidates = append(candidates, filepath.Join(filepath.Dir(path), name))
		}
	}
	for _, p := range candidates {
		data, err := os.ReadFile(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		if p != path {
			// The Claude adapter removes a companion that only imports
			// this AGENTS.md, since its scoped rules load the same text.
			if filepath.Base(p) == AgentsCompanionFile && IsAgentsCompanion(string(data)) {
				continue
			}
			return fmt.Errorf("%s: alternate instructions conflict with scoped %s; import or move the alternate file before syncing", p, filepath.Base(path))
		}
		if header.Has(string(data)) {
			continue
		}
		if line, missing := lineMissingFrom(string(data), content); missing {
			return fmt.Errorf("%s: %q is in no spec, so sync would drop it; the file changed after import or was never imported: move the text into .agnostic-ai/, or set the file aside with `mv %s %s.before-agnostic` and run `agnostic-ai sync`", p, line, filepath.ToSlash(p), filepath.ToSlash(p))
		}
	}
	return nil
}

// lineMissingFrom returns the first line of text in have that want lacks.
// Blank lines and headings are skipped: import turns headings into rule
// names. Lines compare without surrounding space or emphasis markers, so
// a `*description*` import rendered as `_description_` still matches.
func lineMissingFrom(have, want string) (string, bool) {
	lines := map[string]bool{}
	for _, l := range strings.Split(want, "\n") {
		lines[comparableLine(l)] = true
	}
	for _, l := range strings.Split(have, "\n") {
		c := comparableLine(l)
		if c == "" || isHeading(c) {
			continue
		}
		if !lines[c] {
			return shortLine(strings.TrimSpace(l)), true
		}
	}
	return "", false
}

func comparableLine(l string) string {
	return strings.Trim(strings.TrimSpace(l), "*_")
}

func isHeading(l string) bool {
	n := len(l) - len(strings.TrimLeft(l, "#"))
	return n >= 1 && n <= 6 && (n == len(l) || l[n] == ' ' || l[n] == '\t')
}

// shortLine keeps an error message readable when the line is long.
func shortLine(l string) string {
	const limit = 80
	if r := []rune(l); len(r) > limit {
		return string(r[:limit-3]) + "..."
	}
	return l
}

package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// importClaudeAgents copies .claude/agents/*.md byte-for-byte to dstDir.
func importClaudeAgents(root, dstDir string) (int, error) {
	src := filepath.Join(root, claudeDir, "agents")
	if !dirExists(src) {
		return 0, nil
	}
	return copyMarkdownDir(src, dstDir)
}

// importClaudeSkills copies each `.claude/skills/<name>/` directory tree
// byte-for-byte into `<dstDir>/<name>/`. SKILL.md must exist for the
// skill to be considered; once present, every sibling file (and nested
// subdirectories such as `scripts/`, `assets/`, helper Python/JS
// modules, fixtures, etc.) is mirrored verbatim so a roundtrip
// preserves the full skill payload.
func importClaudeSkills(root, dstDir string) (int, error) {
	src := filepath.Join(root, claudeDir, "skills")
	entries, err := os.ReadDir(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillSrc := filepath.Join(src, e.Name())
		if _, err := os.Stat(filepath.Join(skillSrc, "SKILL.md")); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return count, fmt.Errorf("stat skill %s: %w", e.Name(), err)
		}
		skillDst := filepath.Join(dstDir, e.Name())
		if err := copyDirTree(skillSrc, skillDst); err != nil {
			return count, fmt.Errorf("copy skill %s: %w", e.Name(), err)
		}
		count++
	}
	return count, nil
}

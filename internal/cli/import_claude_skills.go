package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// importClaudeAgents copies .claude/agents/*.md byte-for-byte to dstDir.
func importClaudeAgents(root, dstDir string) (int, error) {
	src := filepath.Join(root, ".claude", "agents")
	entries, err := os.ReadDir(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			return count, fmt.Errorf("read agent %s: %w", e.Name(), err)
		}
		dst := filepath.Join(dstDir, e.Name())
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return count, fmt.Errorf("write %s: %w", dst, err)
		}
		count++
	}
	return count, nil
}

// importClaudeSkills copies each .claude/skills/<name>/SKILL.md to
// <dstDir>/<name>/SKILL.md. Other files inside the skill dir are ignored.
func importClaudeSkills(root, dstDir string) (int, error) {
	src := filepath.Join(root, ".claude", "skills")
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
		skillPath := filepath.Join(src, e.Name(), "SKILL.md")
		data, err := os.ReadFile(skillPath)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return count, fmt.Errorf("read skill %s: %w", e.Name(), err)
		}
		out := filepath.Join(dstDir, e.Name())
		if err := os.MkdirAll(out, 0o755); err != nil {
			return count, fmt.Errorf("mkdir %s: %w", out, err)
		}
		dst := filepath.Join(out, "SKILL.md")
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return count, fmt.Errorf("write %s: %w", dst, err)
		}
		count++
	}
	return count, nil
}

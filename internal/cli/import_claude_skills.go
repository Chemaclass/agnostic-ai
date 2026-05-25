package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
)

// importClaudeAgents copies .claude/agents/*.md byte-for-byte to dstDir.
// When the project also carries a Codex installation but the matching
// codex agent is absent there, the captured spec gains `target: claude`
// frontmatter so the next sync does not cross-emit a claude-only agent
// into `.codex/`.
func importClaudeAgents(root, dstDir string) (int, error) {
	src := filepath.Join(root, claudeDir, "agents")
	if !dirExists(src) {
		return 0, nil
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}
	codexPresent := codexTreeExists(root)
	count := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		srcPath := filepath.Join(src, e.Name())
		data, err := os.ReadFile(srcPath)
		if err != nil {
			return count, fmt.Errorf("read %s: %w", srcPath, err)
		}
		out := header.Strip(string(data))
		name := strings.TrimSuffix(e.Name(), ".md")
		if codexPresent && !codexHasAgent(root, canonicalSpecSlug(name)) {
			out = addTargetFrontmatter(out, "claude")
		}
		dstPath := filepath.Join(dstDir, e.Name())
		if err := importMkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return count, fmt.Errorf("mkdir %s: %w", filepath.Dir(dstPath), err)
		}
		if err := importWriteFile(dstPath, []byte(out), 0o644); err != nil {
			return count, fmt.Errorf("write %s: %w", dstPath, err)
		}
		count++
	}
	return count, nil
}

// importClaudeSkills copies each `.claude/skills/<name>/` directory tree
// byte-for-byte into `<dstDir>/<name>/`. SKILL.md must exist for the
// skill to be considered; once present, every sibling file (and nested
// subdirectories such as `scripts/`, `assets/`, helper Python/JS
// modules, fixtures, etc.) is mirrored verbatim so a roundtrip
// preserves the full skill payload. SKILL.md gains `target: claude`
// frontmatter when the project has a codex tree but no matching codex
// skill (auto-scoping per #299).
func importClaudeSkills(root, dstDir string) (int, error) {
	src := filepath.Join(root, claudeDir, "skills")
	entries, err := os.ReadDir(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}
	codexPresent := codexTreeExists(root)
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
		if codexPresent && !codexHasSkill(root, e.Name()) {
			if err := injectTargetInSkillMD(filepath.Join(skillDst, "SKILL.md"), "claude"); err != nil {
				return count, fmt.Errorf("scope skill %s: %w", e.Name(), err)
			}
		}
		count++
	}
	return count, nil
}

package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// codexTreeExists reports whether root carries any sign of a Codex
// installation. A claude-only project must not gain `target: claude`
// tags during import, since cross-emit would never have triggered
// anyway.
func codexTreeExists(root string) bool {
	for _, d := range codexAgentDirs {
		if dirExists(filepath.Join(root, d)) {
			return true
		}
	}
	for _, d := range codexSkillsDirs {
		if dirExists(filepath.Join(root, d)) {
			return true
		}
	}
	return dirExists(filepath.Join(root, codexDir))
}

// claudeTreeExists is the symmetric check for `import codex`.
func claudeTreeExists(root string) bool {
	return dirExists(filepath.Join(root, claudeDir))
}

// codexHasAgent reports whether any codex agents directory contains a
// TOML whose basename canonicalises to the same slug. Canonicalisation
// folds dash/underscore variants (changelog-keeper vs changelog_keeper)
// so the auto-target check matches the round-trip merge already done
// for codex agent specs.
func codexHasAgent(root, canonical string) bool {
	for _, dir := range codexAgentDirs {
		entries, err := os.ReadDir(filepath.Join(root, dir))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
				continue
			}
			base := strings.TrimSuffix(e.Name(), ".toml")
			if canonicalSpecSlug(base) == canonical {
				return true
			}
		}
	}
	return false
}

// codexHasSkill reports whether `<root>/<codexSkillsDir>/<name>/SKILL.md`
// exists under any known codex skills root.
func codexHasSkill(root, name string) bool {
	for _, dir := range codexSkillsDirs {
		if _, err := os.Stat(filepath.Join(root, dir, name, "SKILL.md")); err == nil {
			return true
		}
	}
	return false
}

// claudeHasAgent reports whether `<root>/.claude/agents/<canonical>.md`
// exists (after canonicalising the on-disk filename).
func claudeHasAgent(root, canonical string) bool {
	dir := filepath.Join(root, claudeDir, "agents")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		base := strings.TrimSuffix(e.Name(), ".md")
		if canonicalSpecSlug(base) == canonical {
			return true
		}
	}
	return false
}

// claudeHasSkill reports whether `<root>/.claude/skills/<name>/SKILL.md`
// exists.
func claudeHasSkill(root, name string) bool {
	_, err := os.Stat(filepath.Join(root, claudeDir, "skills", name, "SKILL.md"))
	return err == nil
}

var targetKeyRE = regexp.MustCompile(`(?m)^target[\t ]*:`)

// addTargetFrontmatter sets `target: <name>` in the leading YAML
// frontmatter of body. If body already declares any `target:` key the
// original is returned unchanged (idempotent). Bodies without
// frontmatter get one synthesized.
func addTargetFrontmatter(body, target string) string {
	if !strings.HasPrefix(body, "---\n") {
		return "---\ntarget: " + target + "\n---\n\n" + body
	}
	rest := body[len("---\n"):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return body
	}
	front := rest[:end]
	afterMarker := rest[end+len("\n---"):]
	if targetKeyRE.MatchString(front) {
		return body
	}
	return "---\n" + front + "\ntarget: " + target + "\n---" + afterMarker
}

// injectTargetInSkillMD rewrites SKILL.md at path to declare
// `target: <name>`. No-op when the file is missing or already scoped.
func injectTargetInSkillMD(path, target string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	out := addTargetFrontmatter(string(data), target)
	if out == string(data) {
		return nil
	}
	return importWriteFile(path, []byte(out), 0o644)
}

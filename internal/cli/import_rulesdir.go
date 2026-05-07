package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

type rulesDirCounts struct {
	rules, agents, skills int
}

// importRulesDirectory walks srcDir for .md files and reclassifies each
// as a rule, agent, or skill based on filename prefix:
//
//	agent-<name>.md → <agentsDst>/<name>.md
//	skill-<name>.md → <skillsDst>/<name>.md
//	<name>.md       → <rulesDst>/<name>.md
//
// Subdirectories under srcDir are preserved as scope. The leading
// `# <heading>\n\n` block (re-emitted on sync) is stripped from the body.
func importRulesDirectory(root, srcDir string, src config.Sources) (rulesDirCounts, error) {
	var c rulesDirCounts
	full := filepath.Join(root, srcDir)
	if _, err := os.Stat(full); errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}

	err := filepath.WalkDir(full, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		rel, err := filepath.Rel(full, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}

		kind, baseName := classifyRulesDirFile(rel)
		dstDir := pickKindDir(kind, src)
		body := stripLeadingHeading(string(data))
		out := filepath.Join(root, dstDir, scopeDir(rel), baseName+".md")
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(out), err)
		}
		content := fmt.Sprintf("---\nname: %s\n---\n\n%s", baseName, body)
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		if err := os.WriteFile(out, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", out, err)
		}
		switch kind {
		case "agents":
			c.agents++
		case "skills":
			c.skills++
		default:
			c.rules++
		}
		return nil
	})
	if err != nil {
		return c, err
	}
	return c, nil
}

// scopeDir returns the directory portion of rel, normalized so root-level
// files produce "" instead of the "." that filepath.Dir yields.
func scopeDir(rel string) string {
	d := filepath.Dir(rel)
	if d == "." {
		return ""
	}
	return d
}

func classifyRulesDirFile(rel string) (kind, baseName string) {
	base := strings.TrimSuffix(filepath.Base(rel), ".md")
	switch {
	case strings.HasPrefix(base, "agent-"):
		return "agents", strings.TrimPrefix(base, "agent-")
	case strings.HasPrefix(base, "skill-"):
		return "skills", strings.TrimPrefix(base, "skill-")
	default:
		return "rules", base
	}
}

func pickKindDir(kind string, src config.Sources) string {
	switch kind {
	case "agents":
		return src.Agents
	case "skills":
		return src.Skills
	default:
		return src.Rules
	}
}

func stripLeadingHeading(content string) string {
	if !strings.HasPrefix(content, "# ") {
		return content
	}
	nl := strings.IndexByte(content, '\n')
	if nl < 0 {
		return ""
	}
	return strings.TrimLeft(content[nl+1:], "\n")
}

// importFromRulesDir reads an existing rules directory (cline/windsurf/
// continue) and writes specs into the configured source directories.
func importFromRulesDir(root, target, srcDir string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Skills); err != nil {
		return err
	}
	c, err := importRulesDirectory(root, srcDir, src)
	if err != nil {
		return err
	}
	summaryf("imported %d rules, %d agents, %d skills (from %s)\n",
		c.rules, c.agents, c.skills, target)
	return nil
}

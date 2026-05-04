package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type rulesDirCounts struct {
	rules, agents, skills int
}

// importRulesDirectory walks srcDir for .md files and reclassifies each
// as a rule, agent, or skill based on filename prefix:
//
//	agent-<name>.md → agents/<name>.md
//	skill-<name>.md → skills/<name>.md
//	<name>.md       → rules/<name>.md
//
// Subdirectories under srcDir are preserved as scope. The leading
// `# <heading>\n\n` block (re-emitted on sync) is stripped from the body.
func importRulesDirectory(root, srcDir string) (rulesDirCounts, error) {
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

		kindDir, baseName := classifyRulesDirFile(rel)
		body := stripLeadingHeading(string(data))
		out := filepath.Join(root, kindDir, scopeDir(rel), baseName+".md")
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
		switch kindDir {
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

func classifyRulesDirFile(rel string) (kindDir, baseName string) {
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

// importFromRulesDir scaffolds an agnostic-ai project from an existing
// rules directory layout (cline/windsurf/continue). Refuses if
// agnostic.config.yaml already exists.
func importFromRulesDir(root, target, srcDir string) error {
	cfgPath := filepath.Join(root, "agnostic.config.yaml")
	if _, err := os.Stat(cfgPath); err == nil {
		return fmt.Errorf("agnostic.config.yaml already exists")
	}
	if err := ensureSourceDirs(root); err != nil {
		return err
	}
	c, err := importRulesDirectory(root, srcDir)
	if err != nil {
		return err
	}
	if err := os.WriteFile(cfgPath, []byte(singleTargetConfig(target)), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", cfgPath, err)
	}
	fmt.Printf("imported %d rules, %d agents, %d skills\n", c.rules, c.agents, c.skills)
	return nil
}

// singleTargetConfig builds the minimal agnostic.config.yaml that all
// importers write: standard source dirs, one target enabled.
func singleTargetConfig(target string) string {
	return fmt.Sprintf(`version: 1

sources:
  agents: agents
  skills: skills
  rules: rules
  hooks: hooks

targets:
  - %s

on-unsupported: warn
`, target)
}

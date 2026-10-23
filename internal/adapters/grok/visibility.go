package grok

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/gitquery"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func validateRulesVisible(rules []spec.Entry, cfg *config.Config, opts emit.RulesDirOpts) error {
	if len(rules) == 0 {
		return nil
	}
	if cfg.Gitignore.Enabled && !slices.Contains(cfg.Gitignore.CommitKinds(target), config.GitignoreInstructions) {
		return fmt.Errorf("grok: project rules cannot be Git-ignored; set gitignore.commit: [grok:instructions] or explicitly disable gitignore management")
	}
	layout, err := gitquery.LayoutAt(".")
	if err != nil {
		return nil
	}
	scratch, err := os.MkdirTemp("", "agnostic-ai-grok-ignore-")
	if err != nil {
		return fmt.Errorf("grok: create ignore check: %w", err)
	}
	defer func() { _ = os.RemoveAll(scratch) }()
	managedIgnore := cfg.Gitignore.Path
	if managedIgnore == "" {
		managedIgnore = ".gitignore"
	}
	managedIgnore, err = filepath.Abs(managedIgnore)
	if err != nil {
		return fmt.Errorf("grok managed ignore path: %w", err)
	}

	rootIgnore := filepath.Join(layout.Toplevel, ".gitignore")
	data, err := os.ReadFile(rootIgnore)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%s: %w", rootIgnore, err)
	}
	if cfg.Gitignore.Enabled && filepath.Clean(rootIgnore) == filepath.Clean(managedIgnore) {
		data = withoutManagedIgnore(data)
		for _, pattern := range cfg.Gitignore.Allow {
			data = append(data, []byte("\n!"+pattern)...)
		}
	}
	globalPath := grokGlobalIgnorePath()
	if globalPath != "" {
		global, err := os.ReadFile(globalPath)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("%s: %w", globalPath, err)
		}
		data = append(data, append([]byte("\n"), global...)...)
	}
	if output, err := exec.Command("git", "-c", "init.templateDir=", "init", "--quiet", scratch).CombinedOutput(); err != nil {
		return fmt.Errorf("grok create visibility check: %w (%s)", err, output)
	}
	if err := os.WriteFile(filepath.Join(scratch, ".gitignore"), data, 0600); err != nil {
		return fmt.Errorf("grok visibility check: %w", err)
	}
	for _, rule := range rules {
		path := filepath.Join(emit.RulesDirectoryEntryDir(opts, rule), rule.Name+".md")
		abs, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		rel, err := filepath.Rel(layout.Toplevel, abs)
		if err != nil || emit.ScopeEscapesRoot(rel) {
			return fmt.Errorf("%s: grok rule must stay inside the Git worktree", path)
		}
		cmd := exec.Command("git", "-C", scratch, "-c", "core.excludesFile=", "check-ignore", "--no-index", "--", filepath.ToSlash(rel))
		output, err := cmd.CombinedOutput()
		if err == nil && len(output) > 0 {
			return fmt.Errorf("%s: grok instruction visibility check rejects ignored output; remove the matching root or global ignore pattern, or re-include its parent directories and file, then sync again", path)
		}
		var exit *exec.ExitError
		if err != nil && (!errors.As(err, &exit) || exit.ExitCode() != 1) {
			return fmt.Errorf("%s: grok check instruction visibility: %w", path, err)
		}
	}
	return nil
}
func withoutManagedIgnore(data []byte) []byte {
	var out []string
	managed := false
	for _, line := range strings.Split(string(data), "\n") {
		if line == "# >>> agnostic-ai (managed) >>>" {
			managed = true
			continue
		}
		if line == "# <<< agnostic-ai (managed) <<<" {
			managed = false
			continue
		}
		if !managed {
			out = append(out, line)
		}
	}
	return []byte(strings.Join(out, "\n"))
}

func grokGlobalIgnorePath() string {
	for _, scope := range []string{"--global", "--system"} {
		if output, err := exec.Command("git", "config", scope, "--path", "--get", "core.excludesFile").Output(); err == nil && strings.TrimSpace(string(output)) != "" {
			return strings.TrimSpace(string(output))
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".gitignore")
	}
	return ""
}

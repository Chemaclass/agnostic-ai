package config

import (
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/errs"
)

// EscapesProject reports whether a project-relative path from a committed
// file is absolute or climbs above the project root. Such a path would let
// a cloned repository make sync touch files outside it.
func EscapesProject(p string) bool {
	if filepath.IsAbs(p) || filepath.VolumeName(p) != "" || strings.HasPrefix(filepath.ToSlash(p), "/") {
		return true
	}
	clean := path.Clean(filepath.ToSlash(p))
	return clean == ".." || strings.HasPrefix(clean, "../")
}

// UnsafeProjectPath reports whether sync must refuse a path from a
// committed file: one outside the project, or one in the repository's
// Git folder, where a written file can be a hook Git runs.
func UnsafeProjectPath(p string) bool {
	return EscapesProject(p) || inGitDir(p)
}

// gitExcludeFile is the local-only ignore file gitignore.path may name.
const gitExcludeFile = ".git/info/exclude"

// inGitDir reports whether any segment of p names the Git folder. The
// match ignores case and trailing dots or spaces, which case-insensitive
// and Windows file systems drop.
func inGitDir(p string) bool {
	for _, seg := range strings.Split(path.Clean(filepath.ToSlash(p)), "/") {
		if strings.EqualFold(strings.TrimRight(seg, ". "), ".git") {
			return true
		}
	}
	return false
}

// outputPaths lists the path-valued keys of one outputs.<target> entry.
func (o Output) outputPaths() map[string]string {
	return map[string]string{
		"dir": o.Dir, "file": o.File, "rules-file": o.RulesFile, "rules-dir": o.RulesDir,
		"review-file": o.ReviewFile, "environment-file": o.EnvironmentFile, "setup-file": o.SetupFile,
		"ignore-file": o.IgnoreFile, "mcp-file": o.MCPFile, "cli-mcp-file": o.CLIMCPFile,
		"root-mcp-file": o.RootMCPFile, "agents-dir": o.AgentsDir, "skills-dir": o.SkillsDir,
		"instructions-dir": o.InstructionsDir, "commands-dir": o.CommandsDir, "chatmodes-dir": o.ChatmodesDir,
		"workflows-dir": o.WorkflowsDir, "assistants-dir": o.AssistantsDir, "tasks-file": o.TasksFile,
		"conf-file": o.ConfFile, "mcp-dir": o.MCPDir, "exec-policies-file": o.ExecPoliciesFile,
		"hooks-file": o.HooksFile, "hooks-dir": o.HooksDir,
	}
}

// validateProjectPaths refuses config paths that point outside the
// project or into .git, and gitignore lines that would split into several. Project
// sync writes these paths; `sync --global` does not read these keys.
func validateProjectPaths(cfg *Config, source string) error {
	targets := make([]string, 0, len(cfg.Outputs))
	for target := range cfg.Outputs {
		targets = append(targets, target)
	}
	slices.Sort(targets)
	for _, target := range targets {
		paths := cfg.Outputs[target].outputPaths()
		keys := make([]string, 0, len(paths))
		for key := range paths {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			if p := paths[key]; p != "" && UnsafeProjectPath(p) {
				return errs.Coded(errs.CodeConfigDecode, "%s: outputs.%s.%s: %q is outside the project or inside .git; use a path relative to the project root", source, target, key, p)
			}
		}
	}
	if p := cfg.Gitignore.Path; p != "" && UnsafeProjectPath(p) && path.Clean(filepath.ToSlash(p)) != gitExcludeFile {
		return errs.Coded(errs.CodeConfigDecode, "%s: gitignore.path: %q is outside the project or inside .git; use a path relative to the project root, or %s", source, p, gitExcludeFile)
	}
	for _, entry := range cfg.Gitignore.Allow {
		if strings.ContainsAny(entry, "\r\n") {
			return errs.Coded(errs.CodeConfigDecode, "%s: gitignore.allow: %q holds a line break; write one pattern per entry", source, entry)
		}
	}
	for _, entry := range cfg.Sync.Unmanaged {
		if strings.ContainsAny(entry, "\r\n") {
			return errs.Coded(errs.CodeConfigDecode, "%s: sync.unmanaged: %q holds a line break; write one pattern per entry", source, entry)
		}
		if n := normalizeUnmanaged(filepath.ToSlash(entry)); n == ".." || strings.HasPrefix(n, "../") {
			return errs.Coded(errs.CodeConfigDecode, "%s: sync.unmanaged: %q is outside the project", source, entry)
		}
	}
	return nil
}

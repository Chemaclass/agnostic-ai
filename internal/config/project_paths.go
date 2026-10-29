package config

import (
	"path"
	"path/filepath"
	"runtime"
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

// UnsafeProjectPathIn is UnsafeProjectPath for a path that may also be
// written absolute, as a ledger entry can be: an absolute path is safe
// only when it resolves inside root and outside its Git folder.
func UnsafeProjectPathIn(root, p string) bool {
	if !filepath.IsAbs(p) {
		return UnsafeProjectPath(p)
	}
	rel, inside := relativeToRoot(root, p)
	return !inside || UnsafeProjectPath(rel)
}

// relativeToRoot returns absolute p relative to root, and whether it
// lies inside root.
func relativeToRoot(root, p string) (string, bool) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(resolvedPath(absRoot), resolvedPath(filepath.Clean(p)))
	return rel, err == nil && !EscapesProject(rel)
}

// resolvedPath follows the links in p's longest existing prefix, so a
// path through /var and one through /private/var compare equal on macOS.
func resolvedPath(p string) string {
	if real, err := filepath.EvalSymlinks(p); err == nil {
		return real
	}
	parent := filepath.Dir(p)
	if parent == p {
		return p
	}
	return filepath.Join(resolvedPath(parent), filepath.Base(p))
}

// gitExcludeFile is the local-only ignore file gitignore.path may name.
const gitExcludeFile = ".git/info/exclude"

// inGitDir reports whether any segment of p names the Git folder.
func inGitDir(p string) bool {
	for _, seg := range strings.Split(path.Clean(filepath.ToSlash(p)), "/") {
		if GitDirName(seg) {
			return true
		}
	}
	return false
}

// GitDirName reports whether one path segment names the Git folder on
// this system, as Git itself refuses it in a tree.
func GitDirName(seg string) bool {
	return gitDirName(seg, runtime.GOOS)
}

// gitDirName matches DotGitName, since case-insensitive and Windows file
// systems drop the difference. On Windows it also matches the short name
// `git~1`, and reads a segment with a colon as an NTFS stream of the file
// before the colon, so `.git::$INDEX_ALLOCATION` names the Git folder and
// `git:commit` does not.
func gitDirName(seg, goos string) bool {
	if DotGitName(seg) {
		return true
	}
	if goos != "windows" {
		return false
	}
	base, _, _ := strings.Cut(seg, ":")
	return DotGitName(base) || strings.EqualFold(strings.TrimRight(base, ". "), "git~1")
}

// NamesDataStream reports whether p holds a colon after its volume on
// this system. Windows stores such a name as a hidden stream of the file
// named before the colon, not as a file of its own.
func NamesDataStream(p string) bool {
	return namesDataStream(p, runtime.GOOS)
}

func namesDataStream(p, goos string) bool {
	return goos == "windows" && strings.Contains(p[len(filepath.VolumeName(p)):], ":")
}

// DotGitName reports whether seg is `.git` once case and trailing dots
// or spaces are ignored, on any system.
func DotGitName(seg string) bool {
	return strings.EqualFold(strings.TrimRight(seg, ". "), ".git")
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
// project at root or into its .git folder, and gitignore lines that would
// split into several. Project sync writes these paths; `sync --global`
// does not read these keys.
func validateProjectPaths(cfg *Config, root, source string) error {
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
			if p := paths[key]; p != "" && unsafeOutputPath(root, p) {
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

// unsafeOutputPath reports whether an output override must fail to
// load: a relative path above root or in .git, or an absolute one inside
// root's .git. An absolute path elsewhere loads, because import previews
// read the config from a copy of the project; the session sync writes
// through refuses it instead (see emit.Session.SetProjectRoot).
func unsafeOutputPath(root, p string) bool {
	if !filepath.IsAbs(p) {
		return UnsafeProjectPath(p)
	}
	rel, inside := relativeToRoot(root, p)
	return inside && inGitDir(rel)
}

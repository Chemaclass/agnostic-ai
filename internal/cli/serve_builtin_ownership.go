package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/claude"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/gitquery"
	"github.com/chemaclass/agnostic-ai/internal/preview"
)

func validateBuiltinConfigPaths(root string, cfg *config.Config) error {
	for _, target := range cfg.Targets {
		out := cfg.Outputs[target]
		for _, path := range []string{out.Dir, out.File, out.RulesFile, out.RulesDir, out.ReviewFile, out.EnvironmentFile, out.SetupFile, out.IgnoreFile, out.MCPFile, out.CLIMCPFile, out.RootMCPFile, out.Agents, out.AgentsDir, out.SkillsDir, out.InstructionsDir, out.CommandsDir, out.ChatmodesDir, out.WorkflowsDir, out.AssistantsDir, out.TasksFile, out.ConfFile, out.MCPDir, out.ExecPoliciesFile, out.HooksFile, out.HooksDir} {
			if path != "" {
				if _, err := builtinRelativePath(root, path); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateBuiltinOutputs(root string, reports []driftReport, previous syncStateFile) error {
	for _, report := range reports {
		if len(report.Blocking) > 0 || len(report.Orphaned) > 0 || len(report.Unmanaged) > 0 || report.Unledgered {
			return fmt.Errorf("%s: output ownership needs manual review; run agnostic-ai doctor before changing built-ins", report.Target)
		}
		for _, files := range [][]adapters.CapturedFile{report.Current, report.Missing, report.Stale, report.Edited} {
			for _, file := range files {
				rel, err := builtinRelativePath(root, file.Path)
				if err != nil {
					return err
				}
				current, err := snapshotEditFile(filepath.Join(root, rel))
				if err != nil {
					return err
				}
				if current.info == nil {
					continue
				}
				if bytes.Equal(current.data, []byte(file.Content)) && (file.Mode == 0 || current.info.Mode().Perm() == file.Mode) {
					continue
				}
				if err := proveBuiltinOutput(rel, file.Path, current, previous); err != nil {
					return err
				}
			}
		}
		for _, path := range report.Leftover {
			rel, err := builtinRelativePath(root, path)
			if err != nil {
				return err
			}
			current, err := snapshotEditFile(filepath.Join(root, rel))
			if err != nil {
				return err
			}
			if current.info == nil {
				continue
			}
			if err := proveBuiltinOutput(rel, path, current, previous); err != nil {
				return err
			}
			if merged, ok := previous.Merged[path]; ok && merged.Unrecorded || !ok && unrecordedMergedCandidate(previous, path) {
				return fmt.Errorf("%s: merged ownership is unrecorded; use agnostic-ai doctor and CLI sync", rel)
			}
		}
	}
	return nil
}

func proveBuiltinOutput(rel, path string, current editFileSnapshot, previous syncStateFile) error {
	if sum := previous.OutputSums[path]; sum == "" || sum != adapters.ContentSum(string(current.data)) {
		return fmt.Errorf("%s: sync cannot prove ownership of the current bytes; review with agnostic-ai doctor and CLI sync", rel)
	}
	return nil
}

func captureBuiltinMergedRelease(path string, ownership mergedOutput) ([]byte, bool, error) {
	if ownership.Unrecorded {
		return nil, false, fmt.Errorf("%s: merged ownership is unrecorded; use CLI sync", path)
	}
	sess := adapters.NewSession()
	sess.StartCapture()
	result, _, err := sess.ReleaseMerged(path, ownership.Keys, ownership.Created, false, false)
	writes := sess.StopCapture()
	if err != nil {
		return nil, false, err
	}
	switch result {
	case adapters.MergedRemoved:
		if len(sess.CapturedRemovals()) != 1 || sess.CapturedRemovals()[0].Path != path {
			return nil, false, fmt.Errorf("%s: merged removal was not captured", path)
		}
		return nil, true, nil
	case adapters.MergedStripped:
		if len(writes) != 1 || writes[0].Path != path {
			return nil, false, fmt.Errorf("%s: merged release was not captured", path)
		}
		return []byte(writes[0].Content), true, nil
	case adapters.MergedUnchanged:
		return nil, false, nil
	default:
		return nil, false, fmt.Errorf("%s: merged configuration needs manual review; use agnostic-ai doctor and CLI sync", path)
	}
}

func validateBuiltinMemory(root string, cfg *config.Config, reports []driftReport) error {
	if !slices.Contains(cfg.Builtins, "memory") {
		return nil
	}
	store, err := adapters.PersonalMemoryDir(cfg, root)
	if err != nil {
		return err
	}
	if _, err := builtinRelativePath(root, store); err != nil {
		return fmt.Errorf("memory.personal: web changes require a project-local store; use agnostic-ai sync in the CLI")
	}
	if hasMarkdownFile(store) {
		return nil
	}
	type source struct{ path, label string }
	var sources []source
	redirects, err := builtinRedirectsMemory(root, cfg, reports)
	if err != nil {
		return err
	}
	if redirects {
		native, ok, err := adapters.NativeMemoryDir("claude", root)
		if err != nil {
			return fmt.Errorf("discover Claude memory: %w", err)
		}
		if ok {
			sources = append(sources, source{native, memorySourceClaude})
		}
	}
	if repo, err := adapters.PersonalMemoryDir(&config.Config{Memory: config.MemoryConfig{Personal: config.PersonalMemoryRepo}}, root); err == nil {
		sources = append(sources, source{repo, memorySourceRepo})
	}
	for _, candidate := range sources {
		if canonicalDir(candidate.path) == canonicalDir(store) {
			continue
		}
		if _, err := os.Stat(candidate.path); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return fmt.Errorf("%s: %w", candidate.path, err)
		}
		plan, err := planMemoryImport(candidate.path, store, candidate.label, nil)
		if err != nil {
			return err
		}
		if len(plan.copies) > 0 || len(plan.skipped) > 0 {
			return fmt.Errorf("%s: existing memory needs migration into %s; use agnostic-ai sync in the CLI", candidate.path, store)
		}
	}
	return nil
}

func builtinRedirectsMemory(root string, cfg *config.Config, reports []driftReport) (bool, error) {
	path := filepath.ToSlash(filepath.Join(claudeLayoutFor(cfg).dir, "settings.local.json"))
	for _, report := range reports {
		for _, files := range [][]adapters.CapturedFile{report.Current, report.Missing, report.Stale, report.Edited} {
			for _, file := range files {
				rel, err := builtinRelativePath(root, file.Path)
				if err != nil {
					return false, err
				}
				if rel != path {
					continue
				}
				var after map[string]any
				normalized, _ := preview.StripJSONC([]byte(file.Content))
				if err := json.Unmarshal(normalized, &after); err != nil {
					return false, fmt.Errorf("parse proposed %s: %w; use CLI sync", path, err)
				}
				if after == nil {
					return false, fmt.Errorf("%s: proposed settings must be a JSON object; use CLI sync", path)
				}
				var before map[string]any
				existing, err := os.ReadFile(filepath.Join(root, path))
				if err != nil && !os.IsNotExist(err) {
					return false, fmt.Errorf("%s: %w", path, err)
				}
				if err == nil {
					normalized, _ = preview.StripJSONC(existing)
					if err := json.Unmarshal(normalized, &before); err != nil {
						return false, fmt.Errorf("parse %s: %w; use CLI sync", path, err)
					}
					if before == nil {
						return false, fmt.Errorf("%s: settings must be a JSON object; use CLI sync", path)
					}
				}
				directory, _ := after["autoMemoryDirectory"].(string)
				if directory != "" && directory != before["autoMemoryDirectory"] {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

// planBuiltinMemoryExclusion returns the info/exclude update sync makes
// for private memory settings, or an empty path when none is needed.
func planBuiltinMemoryExclusion(root string, cfg *config.Config, desired map[string][]byte) (string, []byte, error) {
	if !slices.Contains(cfg.Builtins, "memory") || !slices.Contains(cfg.Targets, "claude") {
		return "", nil, nil
	}
	dir := claudeLayoutFor(cfg).dir
	path := filepath.ToSlash(filepath.Join(dir, "settings.local.json"))
	data, planned := desired[path]
	if !planned || !bytes.Contains(data, []byte(`"autoMemoryDirectory"`)) {
		return "", nil, nil
	}
	if tracked, err := gitquery.Tracked(root, path); err == nil && tracked {
		return "", nil, fmt.Errorf("%s: private memory settings are tracked by Git; review them and use CLI sync", path)
	}
	if !gitWouldTrack(root, path) {
		return "", nil, nil
	}
	if filepath.Clean(gitignoreRel(cfg)) == ".gitignore" {
		for _, entry := range managedBlockLines(string(desired[".gitignore"])) {
			entry = strings.TrimPrefix(entry, "/")
			if entry == path || strings.HasSuffix(entry, "/") && strings.HasPrefix(path, entry) {
				return "", nil, nil
			}
		}
	}
	unavailable := fmt.Errorf("%s: private memory settings need a Git exclusion outside this project; use agnostic-ai sync in the CLI", path)
	exclude, ok := adapters.GitExcludePath()
	if !ok {
		return "", nil, unavailable
	}
	// A linked worktree keeps info/exclude in its main checkout's Git folder.
	gitDir := filepath.Dir(filepath.Dir(exclude))
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", nil, fmt.Errorf("%s: %w", root, err)
	}
	rel, err := filepath.Rel(canonicalDir(absRoot), filepath.Join(canonicalDir(gitDir), "info", "exclude"))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", nil, unavailable
	}
	rel = filepath.ToSlash(rel)
	if err := builtinSafeParents(root, filepath.Join(root, filepath.FromSlash(rel))); err != nil {
		return "", nil, unavailable
	}
	current, err := snapshotEditFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return "", nil, err
	}
	updated, changed := adapters.WithGitExclusion(current.data, claude.AutoMemoryGitExclusion(dir))
	if !changed {
		return "", nil, nil
	}
	return rel, updated, nil
}

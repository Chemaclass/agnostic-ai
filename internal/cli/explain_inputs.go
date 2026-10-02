package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// explainInputsOutput is the JSON envelope for `explain --inputs --json`.
type explainInputsOutput struct {
	Version string   `json:"version"`
	Command string   `json:"command"`
	Inputs  []string `json:"inputs"`
}

// runExplainInputs prints every path whose change can change what sync
// writes: the config files, `.agnostic-ai/**` and any source directory
// outside it as `<dir>/**`, the files `@path` lines inline (reviews, and
// the entry point with `sync.resolve-imports: inline`), the pack lock, and
// `.gitignore`, which holds the managed block and decides what
// `sync --check --against` compares. Paths are relative to the repository
// root, as a hook manager's glob is, and to the project outside Git.
func runExplainInputs(cmd *cobra.Command, jsonOut bool) error {
	cfg, _, err := loadProject(".")
	if err != nil {
		return err
	}
	inputs, err := projectInputs(cfg)
	if err != nil {
		return err
	}
	if prefix, ok := runGit(".", "rev-parse", "--show-prefix"); ok {
		if prefix = strings.TrimSpace(prefix); prefix != "" {
			for i, p := range inputs {
				inputs[i] = prefix + p
			}
		}
	}
	if jsonOut {
		return writeIndentedJSON(cmd, explainInputsOutput{Version: "1", Command: "explain --inputs", Inputs: inputs})
	}
	for _, p := range inputs {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), p); err != nil {
			return err
		}
	}
	return nil
}

// projectInputs lists the inputs runExplainInputs prints, relative to the
// project root, sorted.
func projectInputs(cfg *config.Config) ([]string, error) {
	set := map[string]bool{".gitignore": true, config.ConfigFileName: true, config.LocalOverrideFileName: true}
	for _, f := range []string{config.LegacyConfigFileName, packsLockfile} {
		if _, err := os.Stat(f); err == nil {
			set[f] = true
		}
	}
	base := config.SourceBaseDir + "/"
	set[base+"**"] = true
	for _, src := range []string{
		cfg.Sources.Agents, cfg.Sources.Skills, cfg.Sources.Rules, cfg.Sources.Hooks, cfg.Sources.MCPs,
		cfg.Sources.Commands, cfg.Sources.Settings, cfg.Sources.Reviews, cfg.Sources.Environments, cfg.Sources.Ignore,
	} {
		if dir := filepath.ToSlash(filepath.Clean(src)); src != "" && !strings.HasPrefix(dir+"/", base) {
			set[dir+"/**"] = true
		}
	}
	add := func(refs []string) {
		for _, r := range refs {
			set[r] = true
		}
	}
	for _, dir := range []string{cfg.Sources.Reviews, filepath.Join(defaultProjectUser, "reviews")} {
		refs, err := reviewIncludes(dir)
		if err != nil {
			return nil, err
		}
		add(refs)
	}
	if cfg.Sync.ResolveImports == "inline" {
		for _, f := range []string{adapters.AgnosticEntryPointPath, filepath.Join(defaultProjectUser, "AGNOSTIC_AI.md")} {
			if data, err := os.ReadFile(f); err == nil {
				add(spec.IncludeRefs(string(data)))
			}
		}
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out, nil
}

// reviewIncludes returns the `@path` files the review specs under dir
// inline, as project-relative slash paths.
func reviewIncludes(dir string) ([]string, error) {
	if dir == "" {
		return nil, nil
	}
	var out []string
	err := spec.WalkSourceRoot(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return filepath.SkipDir
			}
			return err
		}
		if d.IsDir() || filepath.Ext(p) != ".md" {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out = append(out, spec.IncludeRefs(spec.StripFrontmatter(string(data)))...)
		return nil
	})
	return out, err
}

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// explainInputsOutput is the JSON envelope for `explain --inputs --json`.
type explainInputsOutput struct {
	Version string   `json:"version"`
	Command string   `json:"command"`
	Inputs  []string `json:"inputs"`
}

// runExplainInputs prints every project path whose change can change what
// sync writes: the config files, `.agnostic-ai/**` and any source
// directory outside it as `<dir>/**`, the files review specs inline with `@path`,
// and `.gitignore`, which holds the managed block and decides what
// `sync --check --against` compares. A hook or CI filter built from it
// runs the check whenever an output can move.
func runExplainInputs(cmd *cobra.Command, jsonOut bool) error {
	cfg, _, err := loadProject(".")
	if err != nil {
		return err
	}
	inputs, err := projectInputs(cfg)
	if err != nil {
		return err
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
	set := map[string]bool{".gitignore": true}
	base := config.SourceBaseDir + "/"
	for _, p := range watchDirs(".", cfg) {
		p = filepath.ToSlash(filepath.Clean(p))
		switch {
		case strings.HasPrefix(p, base):
			// One glob covers every source, the local layer, and overlays.
			p = base + "**"
		case p == config.LegacyConfigFileName:
			if _, err := os.Stat(p); err != nil {
				continue
			}
		case !strings.Contains(filepath.Base(p), "."):
			p += "/**"
		}
		set[p] = true
	}
	for _, dir := range []string{cfg.Sources.Reviews, filepath.Join(defaultProjectUser, "reviews")} {
		refs, err := reviewIncludes(dir)
		if err != nil {
			return nil, err
		}
		for _, r := range refs {
			set[r] = true
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
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
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
		out = append(out, spec.IncludeRefs(string(data))...)
		return nil
	})
	return out, err
}

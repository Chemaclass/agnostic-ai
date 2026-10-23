package codebuddy

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func instructionPlacement(e spec.Entry) (string, string, bool, error) {
	custom, _ := e.Meta["x-codebuddy"].(map[string]any)
	raw, set := custom["instruction-file"]
	if !set {
		return "", "", false, nil
	}
	placement, ok := raw.(string)
	if !ok || !slices.Contains([]string{"CODEBUDDY.md", ".codebuddy/CODEBUDDY.md"}, placement) {
		return "", "", true, fmt.Errorf("%s: codebuddy: instruction-file must be CODEBUDDY.md or .codebuddy/CODEBUDDY.md", e.Path)
	}
	resolved := e
	resolved.Meta = emit.ResolveMeta(e.Meta, target)
	scope, err := spec.RuleScope(resolved)
	if err != nil {
		return "", "", true, fmt.Errorf("%s: codebuddy: instruction scope: %w", e.Path, err)
	}
	if scope != "" {
		if err := emit.CheckScopePath(scope); err != nil {
			return "", "", true, fmt.Errorf("%s: %w", e.Path, err)
		}
		if placement != "CODEBUDDY.md" {
			return "", "", true, fmt.Errorf("%s: codebuddy: the hidden instruction placement is supported at project root only", e.Path)
		}
	}
	return placement, scope, true, nil
}
func RootInstructionSource(rules []spec.Entry) (string, error) {
	source := ""
	for _, r := range rules {
		_, scope, marked, err := instructionPlacement(r)
		if err != nil {
			return "", err
		}
		if marked && scope == "" {
			if source != "" {
				return "", fmt.Errorf("codebuddy: multiple native root instruction specs conflict: %s and %s", source, r.Path)
			}
			source = r.Path
			if source == "" {
				source = r.Name
			}
			if source == "" {
				source = "native root instructions"
			}
		}
	}
	return source, nil
}
func emitNativeInstructions(sess *emit.Session, rules []spec.Entry, cfg *config.Config, dryRun bool) ([]spec.Entry, error) {
	var regular []spec.Entry
	files := map[string]string{}
	for _, r := range rules {
		placement, scope, marked, err := instructionPlacement(r)
		if err != nil {
			return nil, err
		}
		if !marked {
			regular = append(regular, r)
			continue
		}
		path := emit.OutputFile(cfg, target, "CODEBUDDY.md")
		if scope != "" {
			path = filepath.Join(filepath.FromSlash(scope), "CODEBUDDY.md")
		} else if placement == ".codebuddy/CODEBUDDY.md" {
			if cfg.Outputs[target].File != "" {
				return nil, fmt.Errorf("%s: codebuddy: hidden instruction placement conflicts with outputs.codebuddy.file", r.Path)
			}
			path = filepath.Join(emit.OutputDir(cfg, target, ".codebuddy"), "CODEBUDDY.md")
		}
		if _, exists := files[path]; exists {
			return nil, fmt.Errorf("%s: codebuddy: multiple instruction specs write %s", r.Path, path)
		}
		files[path] = emit.WithHeader(markdown(ruleMeta(r), r.Body), emit.FormatMarkdown)
	}
	for _, path := range slices.Sorted(maps.Keys(files)) {
		if err := sess.WriteFile(path, files[path], dryRun); err != nil {
			return nil, err
		}
	}
	return regular, nil
}

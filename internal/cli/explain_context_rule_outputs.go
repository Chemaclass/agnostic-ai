package cli

import (
	"fmt"
	"path/filepath"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func contextRuleOutputs(cfg *config.Config, b spec.Bundle, target string, startup map[string]bool) (map[string][]string, error) {
	outputs := map[string][]string{}
	adapter, err := adapters.Resolve(target)
	if err != nil {
		return outputs, nil
	}
	full, err := captureEmit(adapter, b, cfg)
	if err != nil {
		return nil, fmt.Errorf("%s rule outputs: %w", target, err)
	}
	paths := map[string]bool{}
	for _, f := range full {
		if !cfg.IsUnmanaged(f.Path) {
			paths[filepath.ToSlash(f.Path)] = true
		}
	}
	empty, err := captureEmit(adapter, spec.Bundle{}, cfg)
	if err != nil {
		return nil, fmt.Errorf("%s rule outputs: %w", target, err)
	}
	baseline := map[string]string{}
	for _, f := range empty {
		baseline[filepath.ToSlash(f.Path)] = f.Content
	}
	for _, rule := range b.For(target).Rules {
		source := filepath.ToSlash(adapters.EntrySourcePath(rule))
		if startup[source] {
			continue
		}
		files, err := captureEmit(adapter, singleEntryBundle(rule), cfg)
		if err != nil {
			return nil, fmt.Errorf("%s rule %s: %w", target, source, err)
		}
		for _, f := range files {
			path := filepath.ToSlash(f.Path)
			previous, present := baseline[path]
			if paths[path] && (!present || previous != f.Content) {
				outputs[source] = append(outputs[source], path)
			}
		}
		slices.Sort(outputs[source])
		outputs[source] = slices.Compact(outputs[source])
	}
	return outputs, nil
}

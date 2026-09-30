package cli

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// lintProtectedPaths reports an invalid `protected` block (LINT023) and
// a protected path that covers a file sync writes (LINT022). Sync
// rewrites that file from its source spec on every run, so the guard
// only blocks the agent from the output while the source stays open.
// Rendering every target costs a full capture, so it runs only when a
// settings spec protects something.
func lintProtectedPaths(scope checkScope) ([]lintFinding, error) {
	var groups []spec.ProtectGroup
	var invalid []lintFinding
	for _, entry := range scope.bundle.Settings {
		group, err := spec.ProtectedPaths([]spec.Entry{entry})
		if err != nil {
			invalid = append(invalid, lintFinding{Code: "LINT023", Severity: lintError, Path: entry.Path, Message: err.Error()})
		}
		groups = append(groups, group...)
	}
	if len(invalid) > 0 {
		return invalid, nil
	}
	if len(groups) == 0 {
		return nil, nil
	}
	written, err := syncWrittenPaths(scope)
	if err != nil {
		return nil, err
	}
	var findings []lintFinding
	for _, group := range groups {
		for _, pattern := range group.Paths {
			one := spec.ProtectGroup{Paths: []string{pattern}}
			var covered []string
			for _, file := range written {
				if _, ok := one.Match(file); ok {
					covered = append(covered, file)
				}
			}
			if len(covered) == 0 {
				continue
			}
			example := strings.Join(covered[:min(3, len(covered))], ", ")
			findings = append(findings, lintFinding{Code: "LINT022", Severity: lintWarn, Path: group.Source,
				Message: fmt.Sprintf("protected path %s covers %d file(s) sync writes (%s); sync regenerates them from .agnostic-ai/, so protect the source spec instead", pattern, len(covered), example)})
		}
	}
	return findings, nil
}

// syncWrittenPaths is every project-relative path a sync of the
// scope's targets writes, entry points included, sorted.
func syncWrittenPaths(scope checkScope) ([]string, error) {
	sess := adapters.NewSession()
	seen := map[string]bool{}
	add := func(path string) {
		seen[filepath.ToSlash(filepath.Clean(path))] = true
	}
	for _, target := range scope.targets {
		adapter, err := adapters.Resolve(target)
		if err != nil {
			continue
		}
		files, err := captureAdapterFiles(sess, adapter, scope.bundle, scope.cfg)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", target, err)
		}
		for _, f := range files {
			add(f.Path)
		}
	}
	entryPoints, err := collectEntryPointDrift(scope.cfg, scope.bundle, scope.targets)
	if err != nil {
		return nil, err
	}
	for _, path := range driftGeneratedPaths([]driftReport{entryPoints}) {
		add(path)
	}
	out := make([]string, 0, len(seen))
	for path := range seen {
		out = append(out, path)
	}
	slices.Sort(out)
	return out, nil
}

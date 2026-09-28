package cli

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// nativeSpecDirLabels maps the native artifact labels adapters report to
// the spec kind a directory with that label holds.
var nativeSpecDirLabels = map[string]spec.Kind{
	"Skills":   spec.KindSkill,
	"Rules":    spec.KindRule,
	"Agents":   spec.KindAgent,
	"Commands": spec.KindCommand,
}

// nativeSpecDir is one directory a target emits specs of kind to.
type nativeSpecDir struct {
	kind spec.Kind
	dir  string
}

var nativeSpecDirs = sync.OnceValue(func() []nativeSpecDir {
	cfg := &config.Config{}
	seen := map[nativeSpecDir]bool{}
	var out []nativeSpecDir
	for _, name := range adapters.Names() {
		for _, a := range adapters.NativeArtifactsFor(name, cfg) {
			kind, ok := nativeSpecDirLabels[a.Label]
			if !ok || !strings.HasSuffix(a.Location, "/") || filepath.IsAbs(a.Location) || strings.HasPrefix(a.Location, "~") {
				continue
			}
			d := nativeSpecDir{kind, strings.TrimSuffix(filepath.ToSlash(a.Location), "/")}
			if d.dir == "" || seen[d] {
				continue
			}
			seen[d] = true
			out = append(out, d)
		}
	}
	// Longer directories first, so `.claude/skills` wins over a shorter
	// directory that prefixes it.
	sort.Slice(out, func(i, j int) bool {
		if len(out[i].dir) != len(out[j].dir) {
			return len(out[i].dir) > len(out[j].dir)
		}
		return out[i].dir < out[j].dir
	})
	return out
})

var pathRunRE = regexp.MustCompile(`[\w./-]+`)

// lintNativeSpecPaths flags a spec body that names another spec by the
// path one target emits it to, such as `.claude/skills/style/SKILL.md`
// (LINT015, warn). Every other target writes that spec elsewhere, so the
// path only resolves for one tool; the source path resolves for all.
func lintNativeSpecPaths(b spec.Bundle) []lintFinding {
	byKind := map[spec.Kind]map[string]spec.Entry{}
	for _, e := range b.All() {
		if _, ok := byKind[e.Kind]; !ok {
			byKind[e.Kind] = map[string]spec.Entry{}
		}
		byKind[e.Kind][e.Name] = e
	}
	var out []lintFinding
	for _, e := range b.All() {
		if e.Body == "" {
			continue
		}
		reported := map[string]bool{}
		for _, loc := range pathRunRE.FindAllStringIndex(e.Body, -1) {
			ref := strings.TrimRight(strings.TrimPrefix(e.Body[loc[0]:loc[1]], "./"), ".")
			if loc[0] > 0 && strings.ContainsAny(e.Body[loc[0]-1:loc[0]], "~$") {
				continue
			}
			suggest, target, ok := nativeSpecRefSource(ref, byKind)
			if !ok || reported[ref] {
				continue
			}
			reported[ref] = true
			out = append(out, lintFinding{
				Code:     "LINT015",
				Severity: lintWarn,
				Path:     e.Path,
				Message: fmt.Sprintf("names %s, a target-native path of %s %q; use the source path %s, which exists for every target",
					ref, target.Kind, target.Name, suggest),
			})
		}
	}
	return out
}

// nativeSpecRefSource resolves ref, a path under a target-native spec
// directory, to the source path of the spec it names.
func nativeSpecRefSource(ref string, byKind map[spec.Kind]map[string]spec.Entry) (string, spec.Entry, bool) {
	for _, d := range nativeSpecDirs() {
		rest, ok := strings.CutPrefix(ref, d.dir+"/")
		if !ok || rest == "" {
			continue
		}
		specs := byKind[d.kind]
		first, tail, _ := strings.Cut(rest, "/")
		if e, ok := specs[stem(first)]; ok && e.Path != "" {
			src := filepath.ToSlash(e.Path)
			if d.kind == spec.KindSkill {
				src = path.Dir(src)
				if tail != "" {
					src += "/" + tail
				}
			}
			return src, e, true
		}
		if d.kind == spec.KindRule {
			if e, ok := specs[stem(path.Base(rest))]; ok && e.Path != "" {
				return filepath.ToSlash(e.Path), e, true
			}
		}
		return "", spec.Entry{}, false
	}
	return "", spec.Entry{}, false
}

func stem(name string) string {
	return strings.TrimSuffix(name, path.Ext(name))
}

package authority

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// evidenceNote explains the three evidence classes every snapshot
// names.
const evidenceNote = "Declared effects come from the loaded specs. Installed compares the files sync would write with what is on disk. Runtime enforcement is never observed: no running tool reports what it allowed or blocked."

// paths turns spec paths into the form a view shows: relative to the
// project root with forward slashes, never a machine path outside it.
type paths struct {
	root    string
	absRoot string
}

func newPaths(root string) paths {
	if root == "" {
		root = "."
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	return paths{root: root, absRoot: abs}
}

// display returns e's path relative to the root, or a layer-qualified
// name such as "builtin:skill/review" for a spec outside it.
func (p paths) display(e spec.Entry) string {
	if e.Path == "" {
		return string(e.Kind) + "/" + e.Name
	}
	abs, err := filepath.Abs(e.Path)
	if err == nil {
		if rel, err := filepath.Rel(p.absRoot, abs); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return filepath.ToSlash(rel)
		}
	}
	layer := e.Layer
	if layer == "" {
		layer = "spec"
	}
	return layer + ":" + string(e.Kind) + "/" + e.Name
}

func (p paths) source(e spec.Entry) *Source {
	return &Source{Path: p.display(e), Layer: e.Layer}
}

// Revision is the short content hash of every input a view is built
// from: the config files, in order, then every loaded spec sorted by
// kind and path, each with its fields and body. Two views with the same
// revision were built from the same sources.
func Revision(b spec.Bundle, opts Options) string {
	p := newPaths(opts.Root)
	h := sha256.New()
	for _, f := range opts.ConfigFiles {
		data, err := os.ReadFile(filepath.Join(p.root, filepath.FromSlash(f)))
		if err != nil {
			continue
		}
		writeField(h, "config")
		writeField(h, filepath.ToSlash(f))
		writeField(h, string(data))
	}
	entries := b.All()
	type keyed struct {
		entry spec.Entry
		path  string
	}
	sorted := make([]keyed, 0, len(entries))
	for _, e := range entries {
		sorted = append(sorted, keyed{entry: e, path: p.display(e)})
	}
	slices.SortStableFunc(sorted, func(a, b keyed) int {
		return cmp.Or(cmp.Compare(a.entry.Kind, b.entry.Kind), cmp.Compare(a.path, b.path))
	})
	for _, k := range sorted {
		writeField(h, string(k.entry.Kind))
		writeField(h, k.path)
		writeField(h, canonicalMeta(k.entry.Meta))
		writeField(h, k.entry.Body)
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}

func writeField(h hash.Hash, s string) {
	_, _ = fmt.Fprintf(h, "%d:%s\n", len(s), s)
}

// canonicalMeta renders a spec's fields with sorted keys. JSON sorts map
// keys; YAML, which also sorts them, covers values JSON cannot hold.
func canonicalMeta(meta map[string]any) string {
	if data, err := json.Marshal(meta); err == nil {
		return string(data)
	}
	if data, err := yaml.Marshal(meta); err == nil {
		return string(data)
	}
	return fmt.Sprint(meta)
}

// NewSnapshot describes the configuration a view is built from.
func NewSnapshot(cfg *config.Config, b spec.Bundle, opts Options) Snapshot {
	p := newPaths(opts.Root)
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	name := filepath.Base(p.absRoot)
	if opts.Global {
		name = "global (" + p.root + ")"
	}
	configFiles := make([]string, 0, len(opts.ConfigFiles))
	for _, f := range opts.ConfigFiles {
		configFiles = append(configFiles, filepath.ToSlash(f))
	}
	specs := []SpecRef{}
	for _, e := range b.All() {
		specs = append(specs, SpecRef{Kind: string(e.Kind), Name: e.Name, Path: p.display(e), Layer: e.Layer})
	}
	targets := []string{}
	if cfg != nil {
		targets = append(targets, cfg.Targets...)
	}
	installed := InstalledNotChecked
	if opts.Installed != nil {
		installed = "checked"
	}
	return Snapshot{
		Revision:    Revision(b, opts),
		GeneratedAt: now.UTC().Format(time.RFC3339),
		Project:     Project{Root: p.root, Name: name},
		Targets:     targets,
		ConfigFiles: configFiles,
		Specs:       specs,
		Evidence:    Evidence{Declared: "loaded", Installed: installed, Runtime: "not-observed", Note: evidenceNote},
	}
}

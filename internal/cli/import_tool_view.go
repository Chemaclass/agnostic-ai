package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// importViewGuard remembers each file an import run writes as it was
// before the run, with the sources that wrote it.
type importViewGuard struct {
	source string
	files  map[string]*importViewFile
}

type importViewFile struct {
	before  []byte
	existed bool
	sources []string
	info    os.FileInfo
}

// importView is the active guard, or nil outside an import run.
// Sequential use only, like importSandbox.
var importView *importViewGuard

// importDrops maps each spec the last import run rewrote, by
// specPathKey, to what the new bytes lose that the importing tools never
// showed (see replacesSpec). Sequential use only.
var importDrops = map[string][]string{}

func (g *importViewGuard) beginSource(source string) {
	if g != nil {
		g.source = source
	}
}

// note records path before its first write in the run.
func (g *importViewGuard) note(path string) {
	if g == nil {
		return
	}
	path = filepath.Clean(path)
	f, ok := g.files[path]
	if !ok {
		info, _ := os.Stat(path)
		for _, prior := range g.files {
			if info != nil && prior.info != nil && os.SameFile(info, prior.info) {
				f = prior
				break
			}
		}
		if f == nil {
			f = &importViewFile{info: info}
			if data, err := os.ReadFile(path); err == nil {
				f.before, f.existed = data, true
			}
		}
		g.files[path] = f
	}
	if g.source != "" && !slices.Contains(f.sources, g.source) {
		f.sources = append(f.sources, g.source)
	}
}

// withImportViewGuard runs fn, then settles each existing spec the run
// rewrote. When the tool files the spec gives each importing tool still
// hold what the last sync wrote there, nobody edited them, so the spec
// goes back byte for byte: it keeps what only other tools read, such as
// ::target blocks, workspaces, comments, and a format an older release
// wrote. When one was edited, the edit stays, and keys the tool never
// shows and comments go back into it (see carryBack).
func withImportViewGuard(root string, fn func() error) error {
	prior := importView
	guard := &importViewGuard{files: map[string]*importViewFile{}}
	importView = guard
	importDrops = map[string][]string{}
	runErr := func() error {
		defer func() { importView = prior }()
		return fn()
	}()
	return errors.Join(runErr, guard.settle(root))
}

// settle decides on each rewritten spec. Until the ledger and the
// project show otherwise, a spec counts as losing every ::target block,
// key, and comment the run left out, so a project that does not load,
// or a tool file sync never recorded, stops the import instead.
func (g *importViewGuard) settle(root string) error {
	paths := g.rewrittenSpecs(root)
	if len(paths) == 0 {
		return nil
	}
	nows := map[string][]byte{}
	for _, path := range paths {
		now, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		nows[path] = now
		before := g.files[path].before
		setImportDrops(path, carriedLosses(path, before, now, missingKeys(path, before, now), nil))
	}
	cfg, after, err := loadImportProject(root)
	if err != nil {
		return nil
	}
	adapters.SetWarner(io.Discard)
	defer func() {
		adapters.ResetCapabilityWarnings()
		adapters.SetWarner(os.Stderr)
	}()
	state := readStateFile(root)
	defer holdPriorState(root, state)()
	v := &importToolView{cfg: cfg, after: after, sums: state.OutputSums, specSums: state.SpecFileSums, entries: entryIndex(after)}
	type candidate struct {
		path  string
		tools []adapters.Adapter
		old   spec.Entry
	}
	var specs []candidate
	var overlays []string
	for _, path := range paths {
		f := g.files[path]
		tools := importingTools(f.sources)
		if len(tools) == 0 {
			continue
		}
		entry, ok := v.entryAt(path)
		switch {
		case ok:
			old, err := entryWithBytes(entry, f.before, cfg)
			if err == nil {
				specs = append(specs, candidate{path: path, tools: tools, old: old})
			}
		case underDir(path, filepath.Join(root, agnosticOverlayDir)):
			overlays = append(overlays, path)
		}
	}
	olds := make([]spec.Entry, len(specs))
	for i, c := range specs {
		olds[i] = c.old
	}
	v.before = withEntries(after, olds)
	for _, path := range overlays {
		if v.toolsUnedited(importingTools(g.files[path].sources)) {
			if err := g.rewrite(path, g.files[path].before); err != nil {
				return err
			}
			setImportDrops(path, nil)
		}
	}
	// A spec is kept when every file its old bytes gave each tool that
	// wrote it holds what the last sync recorded. Specs render in groups,
	// so a run costs a few renders rather than one per spec.
	kept := make([]bool, len(specs))
	byTool := map[string][]int{}
	tools := map[string]adapters.Adapter{}
	for i, c := range specs {
		kept[i] = v.syncedAsIs(c.path, g.files[c.path])
		if !kept[i] {
			continue
		}
		for _, a := range c.tools {
			byTool[a.Name()] = append(byTool[a.Name()], i)
			tools[a.Name()] = a
		}
	}
	for name, idx := range byTool {
		olds := make([]spec.Entry, len(idx))
		for j, i := range idx {
			olds[j] = specs[i].old
		}
		for j, ok := range v.uneditedFor(tools[name], olds) {
			kept[idx[j]] = kept[idx[j]] && ok
		}
	}
	// Restoring one alias also restores every other name for that file.
	byFile := map[*importViewFile]bool{}
	for i, c := range specs {
		f := g.files[c.path]
		held, seen := byFile[f]
		byFile[f] = kept[i] && (!seen || held)
	}
	for i, c := range specs {
		kept[i] = byFile[g.files[c.path]]
	}
	for i, c := range specs {
		if kept[i] {
			if err := g.rewrite(c.path, g.files[c.path].before); err != nil {
				return err
			}
			setImportDrops(c.path, nil)
			continue
		}
		lost, err := g.carryBack(v, c.path, c.tools, c.old, nows[c.path])
		if err != nil {
			return err
		}
		setImportDrops(c.path, lost)
	}
	return nil
}

// rewrite writes data to path as a merge by each source that wrote it in
// the run, so the import records and the stop check name only those
// sources and a merge stays one.
func (g *importViewGuard) rewrite(path string, data []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return withImportMerge(func() error {
		for _, s := range g.files[path].sources {
			if err := writeImportAs(s, path, data, info.Mode().Perm()); err != nil {
				return err
			}
		}
		return nil
	})
}

// writeImportAs writes through importWriteFile with source recorded as
// the writer.
func writeImportAs(source, path string, data []byte, mode os.FileMode) error {
	if importRecording == nil {
		return importWriteFile(path, data, mode)
	}
	order := importRecording.order
	importRecording.order = append(slices.Clone(order), source)
	defer func() { importRecording.order = order }()
	return importWriteFile(path, data, mode)
}

// carryBack puts into the run's bytes at path what the old spec holds
// that the tools never show, so no edit in them removed it: top-level
// keys their files do not depend on, and the comments of keys still
// there. It returns what it cannot carry: ::target blocks, since a
// tool's file holds that tool's view of the body, keys whose effect it
// could not render, and comments it found no place for.
func (g *importViewGuard) carryBack(v *importToolView, path string, tools []adapters.Adapter, old spec.Entry, now []byte) ([]string, error) {
	before := g.files[path].before
	var hidden, unknown []string
	for _, key := range missingKeys(path, before, now) {
		switch hides, known := v.hidesKey(tools, old, key); {
		case !known:
			unknown = append(unknown, key)
		case hides:
			hidden = append(hidden, key)
		}
	}
	if merged, ok := mergeKeptYAML(path, before, now, hidden); ok {
		if !bytes.Equal(merged, now) {
			if err := g.rewrite(path, merged); err != nil {
				return nil, err
			}
		}
		now = merged
	} else {
		unknown = append(unknown, hidden...)
	}
	return carriedLosses(path, before, now, unknown, presentKeys(path, now)), nil
}

// carriedLosses names what now loses from before that no tool file
// carries: ::target blocks, the given keys, and comments. With present
// set, only a comment of a key still in now, or of no key, counts.
func carriedLosses(path string, before, now []byte, keys []string, present []string) []string {
	var lost []string
	if filepath.Ext(path) == ".md" {
		_, beforeBody, _ := splitFrontmatter(before)
		_, nowBody, _ := splitFrontmatter(now)
		if len(spec.FenceTargets(beforeBody)) > 0 && nowBody != beforeBody {
			lost = append(lost, "::target blocks")
		}
	}
	lost = append(lost, keys...)
	nowComments := map[string]bool{}
	for _, c := range yamlComments(yamlPart(path, now)) {
		nowComments[c.text] = true
	}
	for _, c := range yamlComments(yamlPart(path, before)) {
		if nowComments[c.text] || present != nil && c.key != "" && !slices.Contains(present, c.key) {
			continue
		}
		lost = append(lost, "comments")
		break
	}
	return lost
}

// missingKeys lists the top-level keys of before that now lacks.
func missingKeys(path string, before, now []byte) []string {
	nowKeys := presentKeys(path, now)
	missing := []string{}
	for _, key := range presentKeys(path, before) {
		if !slices.Contains(nowKeys, key) {
			missing = append(missing, key)
		}
	}
	return missing
}

func presentKeys(path string, data []byte) []string {
	keys := []string{}
	m, err := frontmatterMapping(yamlPart(path, data))
	if err != nil || m == nil {
		return keys
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		keys = append(keys, m.Content[i].Value)
	}
	return keys
}

// yamlComment is one comment line of a spec's YAML and the top-level key
// it belongs to, "" for none.
type yamlComment struct {
	text, key string
}

func yamlComments(yamlBytes []byte) []yamlComment {
	var doc yaml.Node
	if len(bytes.TrimSpace(yamlBytes)) == 0 || yaml.Unmarshal(yamlBytes, &doc) != nil {
		return nil
	}
	var out []yamlComment
	var walk func(n *yaml.Node, key string)
	walk = func(n *yaml.Node, key string) {
		for _, c := range []string{n.HeadComment, n.LineComment, n.FootComment} {
			for _, line := range strings.Split(c, "\n") {
				if line = strings.TrimSpace(line); line != "" {
					out = append(out, yamlComment{text: line, key: key})
				}
			}
		}
		for i, child := range n.Content {
			k := key
			if key == "" && n.Kind == yaml.MappingNode && n == topMapping(&doc) {
				k = n.Content[i-i%2].Value
			}
			walk(child, k)
		}
	}
	walk(&doc, "")
	return out
}

func topMapping(doc *yaml.Node) *yaml.Node {
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		return doc.Content[0]
	}
	return nil
}

// mergeKeptYAML returns now with the hidden keys of before put back in
// before's order, and each comment of before on a key now still holds
// put back on it. It reports false when the YAML does not parse.
func mergeKeptYAML(path string, before, now []byte, hidden []string) ([]byte, bool) {
	beforeYAML, nowYAML := yamlPart(path, before), yamlPart(path, now)
	if len(bytes.TrimSpace(beforeYAML)) == 0 {
		return now, len(hidden) == 0
	}
	var beforeDoc, nowDoc yaml.Node
	if yaml.Unmarshal(beforeYAML, &beforeDoc) != nil || yaml.Unmarshal(nowYAML, &nowDoc) != nil {
		return nil, false
	}
	from, into := topMapping(&beforeDoc), topMapping(&nowDoc)
	if from == nil || into == nil || from.Kind != yaml.MappingNode || into.Kind != yaml.MappingNode {
		return nil, false
	}
	changed := false
	for i := 0; i+1 < len(from.Content); i += 2 {
		key, value := from.Content[i], from.Content[i+1]
		j := mappingIndex(into, key.Value)
		if j < 0 {
			if !slices.Contains(hidden, key.Value) {
				continue
			}
			at := 0
			if i > 0 {
				if prev := mappingIndex(into, from.Content[i-2].Value); prev >= 0 {
					at = prev + 2
				} else {
					at = len(into.Content)
				}
			}
			into.Content = slices.Insert(into.Content, at, key, value)
			changed = true
			continue
		}
		changed = copyComments(into.Content[j], key) || changed
		changed = copyComments(into.Content[j+1], value) || changed
	}
	changed = copyComments(&nowDoc, &beforeDoc) || changed
	changed = copyComments(into, from) || changed
	if !changed {
		return now, true
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if enc.Encode(&nowDoc) != nil || enc.Close() != nil {
		return nil, false
	}
	if filepath.Ext(path) != ".md" {
		return buf.Bytes(), true
	}
	_, body, _ := splitFrontmatter(now)
	out := "---\n" + buf.String() + "---\n"
	if body != "" {
		out += "\n" + body
	}
	return []byte(out), true
}

func mappingIndex(m *yaml.Node, key string) int {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return i
		}
	}
	return -1
}

// copyComments gives to each comment slot of from that to leaves empty,
// and reports whether it set one.
func copyComments(to, from *yaml.Node) bool {
	changed := false
	for _, slot := range []struct{ to, from *string }{
		{&to.HeadComment, &from.HeadComment}, {&to.LineComment, &from.LineComment}, {&to.FootComment, &from.FootComment},
	} {
		if *slot.to == "" && *slot.from != "" {
			*slot.to = *slot.from
			changed = true
		}
	}
	return changed
}

// rewrittenSpecs lists existing spec files changed by the run, except AGNOSTIC_AI.md.
func (g *importViewGuard) rewrittenSpecs(root string) []string {
	specDirs := importViewSpecDirs(root)
	var paths []string
	for path, f := range g.files {
		if !f.existed || specPathKey(path) == agnosticMainFile || !inSpecDir(path, specDirs) {
			continue
		}
		if now, err := os.ReadFile(path); err == nil && !bytes.Equal(now, f.before) {
			paths = append(paths, path)
		}
	}
	slices.Sort(paths)
	return paths
}

// setImportDrops records what the run's bytes at path lose, keyed as
// replacesSpec looks it up, also in a preview of an outside source.
func setImportDrops(path string, lost []string) {
	key := specPathKey(importOriginalSourcePath(path))
	if len(lost) == 0 {
		delete(importDrops, key)
		return
	}
	importDrops[key] = lost
}

// loadImportProject loads the project as the running import sees it: a
// preview reads source directories outside the project from their copies.
func loadImportProject(root string) (*config.Config, spec.Bundle, error) {
	cfg, b, err := loadProject(root)
	if err != nil || len(importSourceCopies) == 0 {
		return cfg, b, err
	}
	mapped := *cfg
	mapped.Sources = importMappedSources(root, cfg.Sources)
	layers, err := resolveLayers(root, &mapped)
	if err != nil {
		return nil, spec.Bundle{}, err
	}
	if b, err = spec.LoadLayered(layers); err != nil {
		return nil, spec.Bundle{}, err
	}
	b.ApplyModelTiers(cfg.Models)
	return cfg, b, nil
}

// importViewSpecDirs lists the spec directories the running import
// writes, a preview's copies of outside source directories included.
func importViewSpecDirs(root string) []string {
	dirs := importSpecDirs(root)
	cfg, err := config.Load(root)
	if err != nil {
		return dirs
	}
	for _, d := range sourceDirsByKind(importMappedSources(root, cfg.Sources)) {
		if d == "" {
			continue
		}
		for _, form := range specPathForms(d) {
			if !slices.Contains(dirs, form) {
				dirs = append(dirs, form)
			}
		}
	}
	return dirs
}

// importToolView answers, from the ledger the last sync wrote, whether a
// tool's files still hold what sync put there.
type importToolView struct {
	cfg      *config.Config
	after    spec.Bundle
	before   spec.Bundle
	sums     map[string]string
	specSums map[string]specFileSum
	entries  map[string]spec.Entry
	real     map[string]spec.Entry
	empty    map[string]map[string]string
	holds    map[string]bool
}

func entryIndex(b spec.Bundle) map[string]spec.Entry {
	index := map[string]spec.Entry{}
	for _, e := range b.All() {
		if e.Path != "" {
			index[specPathKey(e.Path)] = e
		}
	}
	return index
}

func (v *importToolView) entryAt(path string) (spec.Entry, bool) {
	if e, ok := v.entries[specPathKey(path)]; ok {
		return e, true
	}
	// The same file under another spelling, such as /var and /private/var.
	if v.real == nil {
		v.real = map[string]spec.Entry{}
		for _, e := range v.entries {
			if real := realPath(e.Path); real != "" {
				v.real[real] = e
			}
		}
	}
	e, ok := v.real[realPath(path)]
	return e, ok
}

// syncedAsIs reports whether the last sync rendered the spec's old bytes
// for every tool that wrote it, so the tools' files came from them.
func (v *importToolView) syncedAsIs(path string, f *importViewFile) bool {
	rec, ok := v.specSums[specPathKey(importOriginalSourcePath(path))]
	if !ok || rec.By != specSumBySync || rec.Sum != sha256Hex(f.before) {
		return false
	}
	return !slices.ContainsFunc(f.sources, func(s string) bool { return !slices.Contains(rec.Targets, s) })
}

// uneditedFor reports, for each of olds, whether every file it gives
// the tool still holds what the last sync recorded. Skills, agents, and
// commands render in groups that split only where an edited file shows.
// A spec that does not reach the tool, or gives it no file of its own,
// counts as edited, except a rule the tool inlines into its
// instructions file, which that file decides.
func (v *importToolView) uneditedFor(a adapters.Adapter, olds []spec.Entry) []bool {
	out := make([]bool, len(olds))
	var group []int
	for i, e := range olds {
		switch {
		case !e.EmitsTo(a.Name()):
		case e.Kind == spec.KindSkill || e.Kind == spec.KindAgent || e.Kind == spec.KindCommand:
			group = append(group, i)
		default:
			out[i] = v.ownFilesHold(a, []spec.Entry{e})
		}
	}
	var settle func(idx []int)
	settle = func(idx []int) {
		entries := make([]spec.Entry, len(idx))
		for j, i := range idx {
			entries[j] = olds[i]
		}
		if v.ownFilesHold(a, entries) {
			for _, i := range idx {
				out[i] = true
			}
			return
		}
		if len(idx) > 1 {
			settle(idx[:len(idx)/2])
			settle(idx[len(idx)/2:])
		}
	}
	if len(group) > 0 {
		settle(group)
	}
	return out
}

// ownFilesHold renders entries for the tool and reports whether the
// files they add or change, at least one, hold what the ledger records.
// Rules also answer for the tool's instructions file, which sync writes
// on its own and which may inline them.
func (v *importToolView) ownFilesHold(a adapters.Adapter, entries []spec.Entry) bool {
	files, err := v.render(a, spec.NewBundle(entries))
	if err != nil {
		return false
	}
	base, err := v.renderNothing(a)
	if err != nil {
		return false
	}
	var paths []string
	for p, content := range files {
		if got, ok := base[p]; !ok || got != content {
			paths = append(paths, p)
		}
	}
	if slices.ContainsFunc(entries, func(e spec.Entry) bool { return e.Kind == spec.KindRule }) {
		paths = append(paths, v.entryPoint(a)...)
	}
	return v.recorded(paths, true)
}

// entryPoint is the tool's instructions file when the last sync
// recorded it.
func (v *importToolView) entryPoint(a adapters.Adapter) []string {
	p := adapters.EntryPointPath(v.cfg, a.Name())
	if _, ok := v.sums[p]; p == "" || !ok {
		return nil
	}
	return []string{p}
}

// toolsUnedited reports whether every file each tool got from the whole
// project before the run, its instructions file included, still holds
// what the last sync recorded, so no file of theirs was edited since. It
// renders each tool once per run.
func (v *importToolView) toolsUnedited(tools []adapters.Adapter) bool {
	for _, a := range tools {
		if held, ok := v.holds[a.Name()]; ok {
			if !held {
				return false
			}
			continue
		}
		files, err := v.render(a, v.before)
		held := err == nil && v.recorded(append(slices.Collect(maps.Keys(files)), v.entryPoint(a)...), true)
		if v.holds == nil {
			v.holds = map[string]bool{}
		}
		v.holds[a.Name()] = held
		if !held {
			return false
		}
	}
	return true
}

// recorded reports whether each of paths holds the bytes the ledger
// records for it, with at least one recorded. With all set, a path the
// ledger lacks fails.
func (v *importToolView) recorded(paths []string, all bool) bool {
	matched := 0
	for _, p := range paths {
		sum, ok := v.sums[p]
		if !ok {
			if all {
				return false
			}
			continue
		}
		data, err := os.ReadFile(filepath.FromSlash(p))
		if err != nil || adapters.ContentSum(string(data)) != sum {
			return false
		}
		matched++
	}
	return matched > 0
}

// hidesKey reports whether each tool renders e the same without key, and
// whether every render needed to tell succeeded.
func (v *importToolView) hidesKey(tools []adapters.Adapter, e spec.Entry, key string) (hides, known bool) {
	without := e
	without.Meta = maps.Clone(e.Meta)
	delete(without.Meta, key)
	without.MetaKeys = slices.DeleteFunc(slices.Clone(e.MetaKeys), func(k string) bool { return k == key })
	for _, a := range tools {
		with, err := v.render(a, spec.NewBundle([]spec.Entry{e}))
		if err != nil {
			return false, false
		}
		got, err := v.render(a, spec.NewBundle([]spec.Entry{without}))
		if err != nil {
			return false, false
		}
		if !maps.Equal(with, got) {
			return false, true
		}
	}
	return true, true
}

// renderNothing is what the tool gets from no spec at all, once per tool.
func (v *importToolView) renderNothing(a adapters.Adapter) (map[string]string, error) {
	if files, ok := v.empty[a.Name()]; ok {
		return files, nil
	}
	files, err := v.render(a, spec.Bundle{})
	if err == nil {
		if v.empty == nil {
			v.empty = map[string]map[string]string{}
		}
		v.empty[a.Name()] = files
	}
	return files, err
}

func (v *importToolView) render(a adapters.Adapter, b spec.Bundle) (map[string]string, error) {
	captured, err := captureAdapterFiles(adapters.NewSession(), a, b, v.cfg)
	if err != nil {
		return nil, err
	}
	files := make(map[string]string, len(captured))
	for _, f := range captured {
		files[f.Path] = f.Content
	}
	return files, nil
}

// withEntries is b with each of entries in place of the entry at its path.
func withEntries(b spec.Bundle, entries []spec.Entry) spec.Bundle {
	byPath := make(map[string]spec.Entry, len(entries))
	for _, e := range entries {
		byPath[e.Path] = e
	}
	all := b.All()
	for i, e := range all {
		if replaced, ok := byPath[e.Path]; ok {
			all[i] = replaced
		}
	}
	out := spec.NewBundle(all)
	out.Shadowed = b.Shadowed
	return out
}

// entryWithBytes is e as data would load, without writing data to disk.
func entryWithBytes(e spec.Entry, data []byte, cfg *config.Config) (spec.Entry, error) {
	var parsed spec.Entry
	var err error
	switch filepath.Ext(e.Path) {
	case ".md":
		parsed, err = spec.ParseMarkdownBytes(e.Kind, data)
	case ".yaml", ".yml":
		parsed, err = spec.ParseYAMLBytes(e.Kind, data)
	default:
		return spec.Entry{}, fmt.Errorf("%s: not a spec file", e.Path)
	}
	if err != nil {
		return spec.Entry{}, err
	}
	old := e
	old.Meta, old.MetaKeys, old.MetaStyles = parsed.Meta, parsed.MetaKeys, parsed.MetaStyles
	old.NestedKeys, old.Literals, old.Body, old.BodyLine = parsed.NestedKeys, parsed.Literals, parsed.Body, 0
	old.ModelTier = ""
	if parsed.Name != "" {
		old.Name = parsed.Name
	}
	b := spec.NewBundle([]spec.Entry{old})
	b.ApplyModelTiers(cfg.Models)
	return b.All()[0], nil
}

// importingTools resolves each source to its target, or returns nil when
// one has none, so no render speaks for it.
func importingTools(sources []string) []adapters.Adapter {
	var tools []adapters.Adapter
	for _, s := range sources {
		a, err := adapters.Resolve(s)
		if err != nil {
			return nil
		}
		tools = append(tools, a)
	}
	return tools
}

// yamlPart is the frontmatter of a markdown spec, the whole of a YAML
// one, and nothing for any other file.
func yamlPart(path string, data []byte) []byte {
	switch filepath.Ext(path) {
	case ".md":
		yamlBytes, _, _ := splitFrontmatter(data)
		return yamlBytes
	case ".yaml", ".yml":
		return data
	}
	return nil
}

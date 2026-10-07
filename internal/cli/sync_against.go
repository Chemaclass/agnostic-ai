package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
)

// The two states `sync --check --against` compares: what is staged for the
// next commit, and the last commit.
const (
	againstIndex = "index"
	againstHEAD  = "HEAD"
)

// againstGitTimeout bounds each git call that exports a tree: a large
// repository takes longer than the 2 second budget of a status query.
const againstGitTimeout = 60 * time.Second

func validateAgainst(ref string, check, plan, watch, global bool) error {
	if ref == "" {
		return nil
	}
	if ref != againstIndex && ref != againstHEAD {
		return fmt.Errorf("--against: expected %q or %q, got %q", againstIndex, againstHEAD, ref)
	}
	if !check {
		return errs.Coded(errs.CodeFlagConflict, "--against requires --check")
	}
	if plan || watch || global {
		return errs.Coded(errs.CodeFlagConflict, "--against cannot be combined with --plan, --watch, or --global")
	}
	return nil
}

// enterAgainstTree exports the Git index or HEAD to a temporary directory
// and makes the matching project directory the working directory, so a
// check reads the specs and outputs as Git holds them. Only the files a
// check reads are checked out, since a large repository spends seconds
// writing and removing the rest. The directory is a repository of its
// own with the full index, which lets ignored apply the exported
// .gitignore. The caller ends with leave.
func enterAgainstTree(ref string) (_ *againstTree, err error) {
	origin, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	toplevel, err := gitOutput(origin, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("--against %s needs a Git work tree: %w", ref, err)
	}
	prefix, err := gitOutput(origin, nil, "rev-parse", "--show-prefix")
	if err != nil {
		return nil, fmt.Errorf("--against %s: %w", ref, err)
	}
	scratch, err := os.MkdirTemp("", "agnostic-ai-against-")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(scratch)
		}
	}()
	root := filepath.Join(scratch, "tree")
	if err := os.Mkdir(root, 0o755); err != nil {
		return nil, err
	}
	toplevel = strings.TrimSpace(toplevel)
	var env []string
	if ref == againstHEAD {
		env = append(os.Environ(), "GIT_INDEX_FILE="+filepath.Join(scratch, "index"))
		if _, err := gitOutput(toplevel, env, "read-tree", "HEAD"); err != nil {
			return nil, fmt.Errorf("--against %s: %w", ref, err)
		}
	}
	if env == nil {
		env = os.Environ()
	}
	entries, err := gitOutput(toplevel, env, "ls-files", "--stage", "-z")
	if err != nil {
		return nil, fmt.Errorf("--against %s: %w", ref, err)
	}
	if _, err := gitOutput(root, isolatedGitEnv(), "init", "--quiet"); err != nil {
		return nil, fmt.Errorf("--against %s: %w", ref, err)
	}
	if err := gitInput(root, isolatedGitEnv(), entries, "update-index", "-z", "--index-info"); err != nil {
		return nil, fmt.Errorf("--against %s: index: %w", ref, err)
	}
	t := &againstTree{
		origin: origin, scratch: scratch, root: root,
		toplevel: toplevel, prefix: strings.TrimSpace(prefix), ref: ref,
		exportEnv: env, exported: map[string]bool{}, blobs: map[string]string{}, modes: map[string]string{},
	}
	t.project = filepath.Join(root, filepath.FromSlash(t.prefix))
	if err := t.exportInputs(entries); err != nil {
		return nil, fmt.Errorf("--against %s: export: %w", ref, err)
	}
	if err := os.Chdir(t.project); err != nil {
		return nil, fmt.Errorf("--against %s: %w", ref, err)
	}
	if err := t.exportOutputs(); err != nil {
		_ = os.Chdir(origin)
		return nil, fmt.Errorf("--against %s: export: %w", ref, err)
	}
	t.gitEnv = isolateGitEnv()
	return t, nil
}

// exportInputs checks out the paths a check reads whatever the specs
// say: the config files, which also mark a nested project, the packs
// lockfile, every symlink, which may hold a source root or a scope, and
// every path with a dot segment, which holds the specs, the .gitignore
// files, the tool folders, and the root dotfiles.
func (t *againstTree) exportInputs(entries string) error {
	var paths []string
	for _, rec := range strings.Split(entries, "\x00") {
		meta, p, ok := strings.Cut(rec, "\t")
		if !ok {
			continue
		}
		t.tracked = append(t.tracked, p)
		if f := strings.Fields(meta); len(f) > 1 {
			t.modes[p], t.blobs[p] = f[0], f[1]
		}
		switch base := path.Base(p); {
		case base == config.ConfigFileName, base == config.LegacyConfigFileName, base == config.LocalOverrideFileName, base == packsLockfile,
			strings.HasPrefix(p, "."), strings.Contains(p, "/."), t.modes[p] == "120000":
			paths = append(paths, p)
		}
	}
	return t.export(paths)
}

// exportOutputs checks out the rest of what a check of the project in the
// working directory reads: the inputs `explain --inputs` lists, the files
// at the project root, the entry points, the files a scoped AGENTS.md
// checks beside it, the outputs a tracked ledger lists, every tracked file
// where a target could write one, and what exported links point to. An
// output elsewhere comes through exportMissing. A project whose config
// does not load gets every tracked file, so the check reports the failure
// the way a full export would.
func (t *againstTree) exportOutputs() error {
	// A linked config layer loads only once its target is exported.
	if err := t.exportLinkTargets(); err != nil {
		return err
	}
	// The config alone: loading the specs resolves includes not exported yet.
	cfg, _, err := config.LoadWithSources(".")
	if err != nil {
		return t.export(t.tracked)
	}
	// Twice: a linked source directory lists its review includes only once
	// its target is exported.
	for range 2 {
		if err := t.exportProjectInputs(cfg); err != nil {
			return err
		}
	}
	if err := t.makeScopeDirs(); err != nil {
		return err
	}
	loc := newOutputLocations(cfg, nil)
	entry := map[string]bool{}
	for _, p := range entryPointPaths(cfg, cfg.Targets) {
		entry[filepath.ToSlash(p)] = true
	}
	var outputs []string
	for _, p := range t.tracked {
		rel, ok := strings.CutPrefix(p, t.prefix)
		if ok && (entry[rel] || !strings.Contains(rel, "/") || scopeSiblings[path.Base(rel)] || loc.holds(rel) != noLocation) {
			outputs = append(outputs, p)
		}
	}
	for _, p := range readStateFile(".").Outputs {
		outputs = append(outputs, t.prefix+filepath.ToSlash(p))
	}
	if err := t.export(t.trackedOf(outputs)); err != nil {
		return err
	}
	return t.exportLinkTargets()
}

// exportProjectInputs checks out the tracked inputs `explain --inputs`
// lists for cfg, and what their links point to, and records them for
// sameInputs.
func (t *againstTree) exportProjectInputs(cfg *config.Config) error {
	inputs, err := projectInputs(cfg)
	if err != nil {
		return err
	}
	t.inputs = t.inputs[:0]
	for _, in := range inputs {
		dir, isDir := strings.CutSuffix(in, "/**")
		// From the top level: a source may sit outside the project.
		in = path.Clean(t.prefix + dir)
		switch {
		case in == ".." || strings.HasPrefix(in, "../"):
			continue
		case in == "." && isDir:
			// The repository root: every tracked file is an input.
			in = ""
		case isDir:
			in += "/"
		}
		t.inputs = append(t.inputs, in)
	}
	var paths []string
	for _, p := range t.tracked {
		if underRoots(p, t.inputs) {
			paths = append(paths, p)
		}
	}
	if err := t.export(paths); err != nil {
		return err
	}
	return t.exportLinkTargets()
}

// scopeCandidates lists the directories a spec folder can scope to: a
// rule or skill under `<kind>/backend/` applies to `backend/` only when
// that directory exists (spec.assignScopes). Every directory path that
// an exported file's folders spell is a candidate, a superset of the
// scopes that also covers specs reached through a link.
func (t *againstTree) scopeCandidates() []string {
	seen := map[string]bool{}
	var out []string
	for p := range t.exported {
		segs := strings.Split(path.Dir(p), "/")
		for i := range segs {
			for j := i + 1; j <= len(segs); j++ {
				if d := t.prefix + strings.Join(segs[i:j], "/"); !seen[d] {
					seen[d] = true
					out = append(out, d)
				}
			}
		}
	}
	return out
}

// trackedDirs returns every directory that holds one of paths.
func trackedDirs(paths []string) map[string]bool {
	dirs := map[string]bool{}
	for _, p := range paths {
		for d := path.Dir(p); d != "." && !dirs[d]; d = path.Dir(d) {
			dirs[d] = true
		}
	}
	return dirs
}

// makeScopeDirs creates the scope candidates the state holds.
func (t *againstTree) makeScopeDirs() error {
	dirs := trackedDirs(t.tracked)
	for _, d := range t.scopeCandidates() {
		if !dirs[d] {
			continue
		}
		dir := filepath.Join(t.root, filepath.FromSlash(d))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("%s: %w", dir, err)
		}
	}
	return nil
}

// scopeSiblings are the files a scoped AGENTS.md refuses to sit beside,
// as emit.CheckScopedDestination lists them.
var scopeSiblings = map[string]bool{"AGENTS.override.md": true, "WARP.md": true, "CLAUDE.md": true}

// exportLinkTargets checks out the tracked files an exported symlink
// points to, or holds when it points to a directory, and their links in
// turn, so a linked output or source reads as
// Git holds it.
func (t *againstTree) exportLinkTargets() error {
	done := map[string]bool{}
	for {
		var targets []string
		for p := range t.exported {
			if done[p] || t.modes[p] != "120000" {
				continue
			}
			done[p] = true
			dest, err := os.Readlink(filepath.Join(t.root, filepath.FromSlash(p)))
			if err != nil || filepath.IsAbs(dest) {
				continue
			}
			targets = append(targets, path.Clean(path.Join(path.Dir(p), filepath.ToSlash(dest))))
		}
		var files []string
		for _, dest := range targets {
			for _, p := range t.tracked {
				if p == dest || strings.HasPrefix(p, dest+"/") {
					files = append(files, p)
				}
			}
		}
		targets = files
		if len(targets) == 0 {
			return nil
		}
		if err := t.export(targets); err != nil {
			return err
		}
	}
}

// exportMissing checks out each output reports find missing that Git
// tracks in the state, and reports whether it found one: exportOutputs
// left it out, so the drift has to be collected again.
func (t *againstTree) exportMissing(reports []driftReport) (bool, error) {
	var late []string
	for _, r := range reports {
		for _, f := range r.Missing {
			if p := t.prefix + filepath.ToSlash(f.Path); !t.exported[p] {
				late = append(late, p)
			}
		}
	}
	late = t.trackedOf(late)
	return len(late) > 0, t.export(late)
}

// trackedOf keeps the paths Git tracks in the state.
func (t *againstTree) trackedOf(paths []string) []string {
	var out []string
	for _, p := range paths {
		if t.isTrackedPath(p) {
			out = append(out, p)
		}
	}
	return out
}

// isTrackedPath reports whether Git tracks p, a path from the top level,
// in the state.
func (t *againstTree) isTrackedPath(p string) bool {
	if t.isTracked == nil {
		t.isTracked = make(map[string]bool, len(t.tracked))
		for _, p := range t.tracked {
			t.isTracked[p] = true
		}
	}
	return t.isTracked[p]
}

// export checks out the listed tracked paths not exported yet.
func (t *againstTree) export(paths []string) error {
	var todo []string
	for _, p := range paths {
		if !t.exported[p] {
			t.exported[p] = true
			todo = append(todo, p)
		}
	}
	return checkoutPaths(t.toplevel, t.exportEnv, t.root, todo)
}

// isolateGitEnv unsets the variables that point git at another
// repository, so every git call the check makes reads the export, and
// returns them for leave to restore. A pre-commit hook sets GIT_DIR and
// GIT_INDEX_FILE to the real repository.
func isolateGitEnv() map[string]string {
	saved := map[string]string{}
	for _, kv := range os.Environ() {
		k, v, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, "GIT_") && !strings.HasPrefix(k, "GIT_CONFIG") && !strings.HasPrefix(k, "GIT_TERMINAL") {
			saved[k] = v
			_ = os.Unsetenv(k)
		}
	}
	return saved
}

// againstTree is the export enterAgainstTree made.
type againstTree struct {
	origin, scratch, root, project string
	toplevel, prefix, ref          string
	gitEnv                         map[string]string
	// exportEnv points git at the index the export reads.
	exportEnv []string
	// tracked lists every path Git tracks in the state, from the top level.
	tracked   []string
	isTracked map[string]bool
	exported  map[string]bool
	// blobs and modes map each tracked path to its object id and mode.
	blobs, modes map[string]string
	// inputs lists what `explain --inputs` reports, from the top level;
	// a directory ends in a slash.
	inputs []string
}

// previousRef is the state before the one the check reads: the last
// commit for the index, and the first parent for HEAD, which in a pull
// request's merge commit is the base branch.
func (t *againstTree) previousRef() string {
	if t.ref == againstIndex {
		return "HEAD"
	}
	return "HEAD^1"
}

// droppedOutputs lists the files the previous state's specs render that
// the checked state's specs no longer do, while Git still tracks them in
// the checked state with the bytes the previous state rendered: the
// outputs of a deleted spec or a dropped target, with a provenance header
// or without one, such as a JSON file. A file edited since, such as a
// settings file that also holds hand-written keys, is left alone. reports
// are the checked state's drift reports before trackedDrift narrows them.
// A run narrowed to some targets skips the comparison, since the other
// targets' outputs are not in reports. The note says what the check could
// not compare, for the caller to print.
func (t *againstTree) droppedOutputs(filtered bool, reports []driftReport) ([]string, string, error) {
	if filtered {
		return nil, "", nil
	}
	prev := t.previousRef()
	if _, err := gitOutput(t.toplevel, nil, "rev-parse", "--verify", "--quiet", prev+"^{tree}"); err != nil {
		if t.ref == againstHEAD {
			if shallow, _ := gitOutput(t.toplevel, nil, "rev-parse", "--is-shallow-repository"); strings.TrimSpace(shallow) == "true" {
				return nil, "the parent commit is not in this shallow clone (set fetch-depth: 2), so outputs of a deleted spec were not checked", nil
			}
		}
		return nil, "", nil
	}
	skipped := func(err error) ([]string, string, error) {
		return nil, fmt.Sprintf("the %s state could not be rendered (%v), so outputs of a deleted spec were not checked", prev, err), nil
	}
	cfg, _, err := loadProject(".")
	if err != nil {
		return nil, "", err
	}
	if same, err := t.sameInputs(prev); err != nil {
		return skipped(err)
	} else if same {
		return nil, "", nil
	}
	before, err := renderRef(t.toplevel, t.prefix, prev, filepath.Join(t.scratch, "previous"), configuredSources(cfg))
	if err != nil {
		return skipped(err)
	}
	now := map[string]bool{}
	for _, r := range reports {
		for _, list := range [][]adapters.CapturedFile{r.Current, r.Missing, r.Stale, r.Edited} {
			for _, f := range list {
				now[filepath.ToSlash(f.Path)] = true
			}
		}
	}
	var previous []string
	for p := range before {
		previous = append(previous, t.prefix+p)
	}
	if err := t.export(t.trackedOf(previous)); err != nil {
		return nil, "", err
	}
	tracked, ok := trackedFiles(".")
	if !ok {
		return nil, "", nil
	}
	var dropped []string
	for _, p := range tracked {
		slash := filepath.ToSlash(p)
		content, rendered := before[slash]
		if !rendered || now[slash] || cfg.IsUnmanaged(p) {
			continue
		}
		if data, err := os.ReadFile(p); err == nil && string(data) == content {
			dropped = append(dropped, p)
		}
	}
	return dropped, "", nil
}

// sameInputs reports whether ref holds the checked state's inputs, the
// ones `explain --inputs` lists, and the same scope directories, so it
// renders the same files and no output can be dropped.
func (t *againstTree) sameInputs(ref string) (bool, error) {
	listed, err := gitOutput(t.toplevel, nil, "ls-tree", "-r", "-z", "--full-tree", ref)
	if err != nil {
		return false, err
	}
	isInput := func(p string) bool { return underRoots(p, t.inputs) }
	var paths []string
	inputs := 0
	for _, rec := range strings.Split(listed, "\x00") {
		meta, p, ok := strings.Cut(rec, "\t")
		if !ok {
			continue
		}
		paths = append(paths, p)
		if isInput(p) {
			if f := strings.Fields(meta); len(f) < 3 || f[2] != t.blobs[p] {
				return false, nil
			}
			inputs++
		}
	}
	for _, p := range t.tracked {
		if isInput(p) {
			inputs--
		}
	}
	if inputs != 0 {
		return false, nil
	}
	before, now := trackedDirs(paths), trackedDirs(t.tracked)
	for _, d := range t.scopeCandidates() {
		if before[d] != now[d] {
			return false, nil
		}
	}
	return true, nil
}

// renderRef exports the spec inputs of ref into dir, renders the project
// at prefix there, and returns each path sync would write with its
// content. Only the config, `.agnostic-ai/`, the configured source
// directories, and the files reviews inline with `@path` are exported;
// every other tracked directory is created empty, since scoped outputs
// depend on which directories exist. A file sync merges into, such as a
// settings file, is not exported, so its render can differ from the
// committed bytes and never counts as proof. The working directory is
// restored. A project directory ref does not hold renders nothing.
func renderRef(toplevel, prefix, ref, dir string, sources []string) (map[string]string, error) {
	origin, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.Chdir(origin) }()
	if err := os.Mkdir(dir, 0o755); err != nil {
		return nil, err
	}
	env := append(os.Environ(), "GIT_INDEX_FILE="+dir+".index")
	if _, err := gitOutput(toplevel, env, "read-tree", ref); err != nil {
		return nil, err
	}
	listed, err := gitOutput(toplevel, env, "ls-files", "-z")
	if err != nil {
		return nil, err
	}
	roots := []string{config.SourceBaseDir + "/", config.ConfigFileName, config.LegacyConfigFileName}
	for _, s := range sources {
		if s != "" {
			roots = append(roots, strings.TrimSuffix(filepath.ToSlash(filepath.Clean(s)), "/")+"/")
		}
	}
	var specs []string
	dirs := map[string]bool{}
	for _, f := range strings.Split(listed, "\x00") {
		rel, ok := strings.CutPrefix(f, prefix)
		if f == "" || !ok {
			continue
		}
		dirs[path.Dir(f)] = true
		for _, r := range roots {
			if rel == r || strings.HasSuffix(r, "/") && strings.HasPrefix(rel, r) {
				specs = append(specs, f)
				break
			}
		}
	}
	for d := range dirs {
		if err := os.MkdirAll(filepath.Join(dir, filepath.FromSlash(d)), 0o755); err != nil {
			return nil, err
		}
	}
	if err := checkoutPaths(toplevel, env, dir, specs); err != nil {
		return nil, err
	}
	project := filepath.Join(dir, filepath.FromSlash(prefix))
	if err := os.Chdir(project); err != nil {
		return map[string]string{}, nil
	}
	var includes []string
	for _, r := range roots {
		if strings.HasSuffix(r, "/") {
			refs, err := reviewIncludes(filepath.FromSlash(strings.TrimSuffix(r, "/")))
			if err != nil {
				return nil, err
			}
			for _, inc := range refs {
				includes = append(includes, prefix+inc)
			}
		}
	}
	if err := checkoutPaths(toplevel, env, dir, includes); err != nil {
		return nil, err
	}
	return plannedOutputs()
}

// underRoots reports whether rel is one of roots, a root directory
// itself, such as a link to the real one, or sits in one of them. An
// empty root is the repository root and holds every path.
func underRoots(rel string, roots []string) bool {
	for _, r := range roots {
		if r == "" || rel == r || rel+"/" == r || strings.HasSuffix(r, "/") && strings.HasPrefix(rel, r) {
			return true
		}
	}
	return false
}

// checkoutPaths writes the listed index entries under dir.
func checkoutPaths(toplevel string, env []string, dir string, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	return gitInput(toplevel, env, strings.Join(paths, "\x00")+"\x00", "checkout-index", "--force", "-z", "--stdin", "--prefix="+dir+string(filepath.Separator))
}

// renderedAtHEAD renders the last commit's specs for the project in the
// working directory, or returns nil outside a repository with a commit.
// A tracked file still holding what HEAD rendered is one sync wrote,
// with or without a provenance header.
func renderedAtHEAD(sources []string) map[string]string {
	origin, err := os.Getwd()
	if err != nil {
		return nil
	}
	toplevel, err := gitOutput(origin, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil
	}
	prefix, err := gitOutput(origin, nil, "rev-parse", "--show-prefix")
	if err != nil {
		return nil
	}
	toplevel = strings.TrimSpace(toplevel)
	if _, err := gitOutput(toplevel, nil, "rev-parse", "--verify", "--quiet", "HEAD^{tree}"); err != nil {
		return nil
	}
	scratch, err := os.MkdirTemp("", "agnostic-ai-head-")
	if err != nil {
		return nil
	}
	defer func() { _ = os.RemoveAll(scratch) }()
	out, err := renderRef(toplevel, strings.TrimSpace(prefix), "HEAD", filepath.Join(scratch, "tree"), sources)
	if err != nil {
		return nil
	}
	return out
}

// plannedOutputs renders the specs in the working directory for every
// configured target and returns each path sync would write with its
// content. Its warnings, notes, and verbose lines stay quiet: the checked
// state's render already reported what applies.
func plannedOutputs() (map[string]string, error) {
	defer quiet()()
	cfg, b, err := loadProject(".")
	if err != nil {
		return nil, err
	}
	sess := adapters.NewSession()
	out := map[string]string{}
	add := func(files []adapters.CapturedFile) {
		for _, f := range files {
			out[filepath.ToSlash(f.Path)] = f.Content
		}
	}
	for _, t := range cfg.Targets {
		adapter, err := adapters.Resolve(t)
		if err != nil {
			continue
		}
		files, err := captureAdapterFiles(sess, adapter, b, cfg)
		if err != nil {
			return nil, err
		}
		add(files)
	}
	ep, err := collectEntryPointDrift(cfg, b, cfg.Targets)
	if err != nil {
		return nil, err
	}
	add(ep.Current)
	add(ep.Missing)
	add(ep.Stale)
	add(ep.Edited)
	return out, nil
}

// leave restores the working directory and git environment, and removes
// the export.
func (t *againstTree) leave() {
	_ = os.Chdir(t.origin)
	for k, v := range t.gitEnv {
		_ = os.Setenv(k, v)
	}
	_ = os.RemoveAll(t.scratch)
}

// ignored returns the paths the exported .gitignore ignores, relative to
// the project directory the working directory now is.
func (t *againstTree) ignored(paths []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(paths) == 0 {
		return out, nil
	}
	var stdin bytes.Buffer
	for _, p := range paths {
		stdin.WriteString(filepath.ToSlash(p))
		stdin.WriteByte(0)
	}
	// --no-index skips the index lookup that costs most of the run; a
	// tracked path is never ignored, which the filter below keeps.
	cmd := exec.Command("git", "check-ignore", "--no-index", "-z", "--stdin")
	cmd.Env = isolatedGitEnv()
	cmd.Stdin = &stdin
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// check-ignore exits 1 when it matches nothing.
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
			return nil, fmt.Errorf("git check-ignore: %w: %s", err, strings.TrimSpace(stderr.String()))
		}
	}
	for _, p := range strings.Split(stdout.String(), "\x00") {
		if p != "" && !t.isTrackedPath(t.prefix+p) {
			out[p] = true
		}
	}
	return out, nil
}

// trackedDrift narrows reports from a check of an exported tree to what
// that state gets wrong about the outputs Git tracks there: an output
// that differs from what the specs render, or one the state lacks that
// its .gitignore does not ignore, and a tracked file that carries the
// provenance header where a target writes but that no spec produces, and
// each dropped output droppedOutputs found. The ledger is never committed,
// so ledger findings say nothing about the state, and a tracked leftover
// is one to delete by hand.
func (t *againstTree) trackedDrift(reports []driftReport, dropped []string) ([]driftReport, error) {
	var missing []string
	for _, r := range reports {
		for _, f := range r.Missing {
			missing = append(missing, f.Path)
		}
	}
	ignored, err := t.ignored(missing)
	if err != nil {
		return nil, err
	}
	var out []driftReport
	for _, r := range reports {
		kept := driftReport{Target: r.Target, Stale: r.Stale, Edited: r.Edited}
		if r.Target == unledgeredReportTarget {
			kept.Unledgered = true
			kept.Orphaned = mergePaths(r.Orphaned, r.Leftover, dropped)
			dropped = nil
		}
		for _, f := range r.Missing {
			if !ignored[filepath.ToSlash(f.Path)] {
				kept.Missing = append(kept.Missing, f)
			}
		}
		out = append(out, kept)
	}
	if len(dropped) > 0 {
		out = append(out, driftReport{Target: unledgeredReportTarget, Unledgered: true, Orphaned: mergePaths(dropped)})
	}
	return out, nil
}

// mergePaths joins path lists, keeping each path once in first-seen order.
func mergePaths(lists ...[]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, l := range lists {
		for _, p := range l {
			if k := filepath.ToSlash(p); !seen[k] {
				seen[k] = true
				out = append(out, p)
			}
		}
	}
	return out
}

// trackedUnmanaged returns the hand-written config Git tracks in the
// exported state inside a folder its managed block ignores: a skill or
// agent a branch added in a tool's generated folder before the project
// moved its specs to `.agnostic-ai/`. Only the tool that reads that folder
// sees it, and git add refuses a new one there, so it is always a file
// left over from before the move. Paths in sync.unmanaged are left alone.
func (t *againstTree) trackedUnmanaged(cfg *config.Config) ([]unmanagedFinding, error) {
	found, err := findUnmanagedConfig(".", cfg)
	if err != nil || len(found) == 0 {
		return nil, err
	}
	tracked, ok := trackedFiles(".")
	if !ok {
		return nil, nil
	}
	isTracked := map[string]bool{}
	for _, p := range tracked {
		isTracked[filepath.ToSlash(p)] = true
	}
	var candidates []unmanagedFinding
	var paths []string
	for _, f := range found {
		if isTracked[f.Path] && !cfg.IsUnmanaged(f.Path) {
			candidates = append(candidates, f)
			paths = append(paths, f.Path)
		}
	}
	if len(paths) == 0 {
		return nil, nil
	}
	// --no-index: every candidate is tracked, which check-ignore skips.
	var stdin bytes.Buffer
	for _, p := range paths {
		stdin.WriteString(p)
		stdin.WriteByte(0)
	}
	cmd := exec.Command("git", "check-ignore", "--no-index", "-z", "--stdin")
	cmd.Env = isolatedGitEnv()
	cmd.Stdin = &stdin
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
			return nil, fmt.Errorf("git check-ignore: %w: %s", err, strings.TrimSpace(stderr.String()))
		}
	}
	ignored := map[string]bool{}
	for _, p := range strings.Split(stdout.String(), "\x00") {
		ignored[p] = true
	}
	var out []unmanagedFinding
	for _, f := range candidates {
		if ignored[f.Path] {
			out = append(out, f)
		}
	}
	return out, nil
}

// regeneratedDrift reports whether any drift is settled by staging or
// committing what sync writes. A leftover or a hand-written file has its
// own fix, which the reconcile hint names.
func regeneratedDrift(reports []driftReport) bool {
	for _, r := range reports {
		if len(r.Missing)+len(r.Stale)+len(r.Edited) > 0 {
			return true
		}
	}
	return false
}

// markAgainst records that reports compared ref. The ledger describes
// the working tree, not ref, so a file that differs there is out of date,
// not edited by hand.
func markAgainst(reports []driftReport, ref string) {
	for i := range reports {
		reports[i].against = ref
		reports[i].Stale = append(reports[i].Stale, reports[i].Edited...)
		reports[i].Edited = nil
	}
}

// againstPlace names where ref keeps the outputs and the step that puts
// the regenerated ones there.
func againstPlace(ref string) (where, step string) {
	if ref == againstHEAD {
		return "in the last commit", "run agnostic-ai sync, then commit the regenerated files"
	}
	return "in the Git index", "run agnostic-ai sync, then stage the regenerated files with git add"
}

// againstHint says which step settles drift found against ref.
func againstHint(ref string) string {
	_, step := againstPlace(ref)
	if ref == againstHEAD {
		return "the check compared the last commit; " + step
	}
	return "the check compared the Git index; " + step
}

// gitInput runs git in dir with the environment env, feeding it stdin.
func gitInput(dir string, env []string, stdin string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), againstGitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdin = strings.NewReader(stdin)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// gitOutput runs git in dir with the environment env (the process
// environment when nil) and returns its standard output.
func gitOutput(dir string, env []string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), againstGitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// isolatedGitEnv is the process environment without the variables that
// point git at another repository. A git hook runs with GIT_DIR and
// GIT_INDEX_FILE set, which would send a command meant for the export to
// the real repository.
func isolatedGitEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") || strings.HasPrefix(kv, "GIT_CONFIG") || strings.HasPrefix(kv, "GIT_TERMINAL") {
			env = append(env, kv)
		}
	}
	return env
}

// configuredSources lists cfg's spec source directories.
func configuredSources(cfg *config.Config) []string {
	return []string{
		cfg.Sources.Agents, cfg.Sources.Skills, cfg.Sources.Rules, cfg.Sources.Hooks, cfg.Sources.MCPs,
		cfg.Sources.Commands, cfg.Sources.Settings, cfg.Sources.Reviews, cfg.Sources.Environments, cfg.Sources.Ignore,
	}
}

// recollect collects the drift again when exportMissing checks out an
// output the first pass found missing. The first pass already printed its
// warnings, so the second runs quietly and replaces the pending ones.
func (t *againstTree) recollect(reports []driftReport, targets []string, gitignoreFlag string) ([]driftReport, error) {
	late, err := t.exportMissing(reports)
	if err != nil || !late {
		return reports, err
	}
	defer quiet()()
	resetDrops()
	return collectDriftWithGitignore(targets, nil, gitignoreFlag)
}

// quiet silences warnings, notes, and verbose lines until the returned
// function restores them.
func quiet() func() {
	adapters.SetWarner(io.Discard)
	prevVerbosity, prevWarn, prevSkipped := verbosity, requiresWarnOut, requiresSkipped
	verbosity, requiresWarnOut, requiresSkipped = levelQuiet, io.Discard, true
	prevDrift := driftQuiet
	driftQuiet = true
	return func() {
		adapters.SetWarner(os.Stderr)
		verbosity, requiresWarnOut, requiresSkipped = prevVerbosity, prevWarn, prevSkipped
		driftQuiet = prevDrift
	}
}

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
// check reads the specs and outputs as Git holds them. The directory is a
// repository of its own, which lets againstIgnored apply the exported
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
	if _, err := gitOutput(toplevel, env, "checkout-index", "--all", "--force", "--prefix="+root+string(filepath.Separator)); err != nil {
		return nil, fmt.Errorf("--against %s: export: %w", ref, err)
	}
	if _, err := gitOutput(root, isolatedGitEnv(), "init", "--quiet"); err != nil {
		return nil, fmt.Errorf("--against %s: %w", ref, err)
	}
	entries, err := gitOutput(toplevel, env, "ls-files", "--stage", "-z")
	if err != nil {
		return nil, fmt.Errorf("--against %s: %w", ref, err)
	}
	if err := gitInput(root, isolatedGitEnv(), entries, "update-index", "-z", "--index-info"); err != nil {
		return nil, fmt.Errorf("--against %s: index: %w", ref, err)
	}
	project := filepath.Join(root, filepath.FromSlash(strings.TrimSpace(prefix)))
	if err := os.Chdir(project); err != nil {
		return nil, fmt.Errorf("--against %s: %w", ref, err)
	}
	return &againstTree{
		origin: origin, scratch: scratch, root: root, project: project,
		toplevel: toplevel, prefix: strings.TrimSpace(prefix), ref: ref, gitEnv: isolateGitEnv(),
	}, nil
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
	adapters.SetWarner(io.Discard)
	defer adapters.SetWarner(os.Stderr)
	prevVerbosity, prevWarn, prevSkipped := verbosity, requiresWarnOut, requiresSkipped
	verbosity, requiresWarnOut, requiresSkipped = levelQuiet, io.Discard, true
	defer func() { verbosity, requiresWarnOut, requiresSkipped = prevVerbosity, prevWarn, prevSkipped }()
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
	cmd := exec.Command("git", "check-ignore", "-z", "--stdin")
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
		if p != "" {
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

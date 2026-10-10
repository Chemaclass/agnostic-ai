package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/term"
)

// configSpecKey fingerprints the merged config beside the spec entries, so
// an edit to agnostic-ai.yaml shows up as a source change too.
const configSpecKey = "config"

// reportPathsShown caps the paths listed per action at the default
// verbosity; -v lists every path.
const reportPathsShown = 3

// syncReport collects what one sync run changed, for the closing summary.
type syncReport struct {
	created, updated, removed []string
	// targets holds every target that created or updated a file.
	targets map[string]bool
	specs   []specChange
	// pending lists changed paths git still has to record: tracked files
	// sync modified or deleted, and new files no .gitignore rule covers.
	pending []string
	// trackedIgnored lists generated paths git both tracks and ignores: a
	// file the repo committed before it moved into the managed .gitignore
	// block. Never in pending, since re-adding it is the opposite of what
	// is needed (#1330). Untouched unless --untrack ran.
	trackedIgnored []string
	// untracked lists the paths --untrack removed from git's index this
	// run. Mutually exclusive with trackedIgnored: a path moves from one
	// to the other once removed.
	untracked []string
}

// specChange is one source spec that differs from the previous sync.
type specChange struct {
	mark  string // "+" added, "~" changed, "-" removed
	label string
}

func (r *syncReport) addWrites(target string, writes []adapters.WrittenFile) {
	for _, w := range writes {
		p := filepath.ToSlash(w.Path)
		switch w.Action {
		case "create":
			r.created = append(r.created, p)
		case "update":
			r.updated = append(r.updated, p)
		default:
			continue
		}
		if target != "" {
			if r.targets == nil {
				r.targets = map[string]bool{}
			}
			r.targets[target] = true
		}
	}
}

func (r *syncReport) changedPaths() []string {
	out := []string{}
	out = append(out, r.created...)
	out = append(out, r.updated...)
	out = append(out, r.removed...)
	return out
}

func (r *syncReport) filesChanged() int {
	return len(r.created) + len(r.updated) + len(r.removed)
}

const untrackPullAdvice = "other clones lose these files on their next pull after this deletion is committed; run `agnostic-ai install-hook --post-checkout` there before pulling, or `agnostic-ai sync` after pulling"

// render prints the change list and the closing summary line.
func (r *syncReport) render(w io.Writer, targets int, elapsed time.Duration, verbose bool) {
	if len(r.specs) > 0 {
		labels := make([]string, len(r.specs))
		for i, s := range r.specs {
			labels[i] = s.mark + " " + s.label
		}
		_, _ = fmt.Fprintf(w, "  %s → %s\n", capPaths(labels, verbose), r.fanOut())
	}
	r.renderPaths(w, term.Colorize(w, "+", term.Green), r.created, verbose)
	r.renderPaths(w, term.Colorize(w, "~", term.Yellow), r.updated, verbose)
	r.renderPaths(w, term.Colorize(w, "-", term.Red), r.removed, verbose)
	if n := len(r.pending); n > 0 {
		_, _ = fmt.Fprintf(w, "  %s %d file%s to commit: %s\n", term.Bang(w), n, plural(n), capPaths(r.pending, verbose))
	}
	if n := len(r.untracked); n > 0 {
		_, _ = fmt.Fprintf(w, "  %s untracked %d file%s no longer covered by git: %s\n", term.Bang(w), n, plural(n), strings.Join(r.untracked, " "))
		_, _ = fmt.Fprintf(w, "  %s %s\n", term.Bang(w), untrackPullAdvice)
	}
	if n := len(r.trackedIgnored); n > 0 {
		_, _ = fmt.Fprintf(w, "  %s %d file%s tracked despite being ignored: git rm --cached %s\n", term.Bang(w), n, plural(n), strings.Join(r.trackedIgnored, " "))
	}

	if r.filesChanged() == 0 {
		_, _ = fmt.Fprintf(w, "%s %d target%s up to date · %s\n", term.Tick(w), targets, plural(targets), shortDuration(elapsed))
		return
	}
	parts := []string{fmt.Sprintf("synced %d target%s", targets, plural(targets))}
	for _, c := range []struct {
		n    int
		verb string
	}{{len(r.created), "created"}, {len(r.updated), "updated"}, {len(r.removed), "removed"}} {
		if c.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", c.n, c.verb))
		}
	}
	parts = append(parts, shortDuration(elapsed))
	_, _ = fmt.Fprintf(w, "%s %s\n", term.Tick(w), strings.Join(parts, " · "))
}

func (r *syncReport) fanOut() string {
	n := r.filesChanged()
	if n == 0 {
		return "no output changed"
	}
	// Removed files come from the ledger, which records no target, so a
	// target count would leave out the targets that only lost files.
	if len(r.targets) == 0 || len(r.removed) > 0 {
		return fmt.Sprintf("%d file%s", n, plural(n))
	}
	return fmt.Sprintf("%d file%s in %d target%s", n, plural(n), len(r.targets), plural(len(r.targets)))
}

func (r *syncReport) renderPaths(w io.Writer, mark string, paths []string, verbose bool) {
	if len(paths) == 0 {
		return
	}
	if verbose {
		for _, p := range paths {
			_, _ = fmt.Fprintf(w, "  %s %s\n", mark, p)
		}
		return
	}
	_, _ = fmt.Fprintf(w, "  %s %s\n", mark, capPaths(paths, false))
}

// capPaths joins paths, keeping the first few at the default verbosity.
func capPaths(paths []string, verbose bool) string {
	if verbose || len(paths) <= reportPathsShown {
		return strings.Join(paths, "  ")
	}
	return fmt.Sprintf("%s  (+%d more)", strings.Join(paths[:reportPathsShown], "  "), len(paths)-reportPathsShown)
}

// specSums fingerprints every source spec and the merged config, keyed by
// a stable identity, so the next sync can name what changed since this one.
func specSums(cfg *config.Config, b spec.Bundle) map[string]string {
	sums := map[string]string{}
	if data, err := json.Marshal(cfg); err == nil {
		sums[configSpecKey] = sha256Hex(data)
	}
	for _, e := range b.All() {
		sums[specKey(e)] = entrySum(e)
	}
	return sums
}

func specKey(e spec.Entry) string {
	name := e.Name
	if e.Scope != "" {
		name = e.Scope + "/" + e.Name
	}
	return string(e.Kind) + " " + name
}

func entrySum(e spec.Entry) string {
	h := sha256.New()
	meta, err := json.Marshal(e.Meta)
	if err != nil {
		meta = []byte(fmt.Sprint(e.Meta))
	}
	// Key order and quoting style reach the output, so they count too.
	styles, err := json.Marshal(e.MetaStyles)
	if err != nil {
		styles = []byte(fmt.Sprint(e.MetaStyles))
	}
	_, _ = fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\x00", meta, strings.Join(e.MetaKeys, "\x01"), styles, e.Body)
	if dir := e.SkillAssetDir(); dir != "" {
		_ = spec.WalkSourceRoot(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			rel, _ := filepath.Rel(dir, p)
			_, _ = fmt.Fprintf(h, "%s\x00%s\x00", filepath.ToSlash(rel), data)
			return nil
		})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// diffSpecSums names the specs added, changed, or removed between two
// syncs. A missing baseline (first sync, older state file) yields nothing:
// every spec would read as added, which says nothing the file list does not.
func diffSpecSums(prev, cur map[string]string) []specChange {
	if len(prev) == 0 {
		return nil
	}
	var out []specChange
	for k, sum := range cur {
		old, ok := prev[k]
		switch {
		case !ok:
			out = append(out, specChange{mark: "+", label: k})
		case old != sum:
			out = append(out, specChange{mark: "~", label: k})
		}
	}
	for k := range prev {
		if _, ok := cur[k]; !ok {
			out = append(out, specChange{mark: "-", label: k})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].label < out[j].label })
	return out
}

// gitPending returns the paths among changed that git reports as modified,
// deleted, or untracked. Ignored outputs never appear, so what is left is
// what the user has to commit. Outside a git work tree, or when git is
// missing or slow, it returns nothing: the hint is a convenience.
func gitPending(root string, changed []string) []string {
	if len(changed) == 0 {
		return nil
	}
	// Porcelain paths are relative to the repository root; strip the
	// project's own prefix so they match the lists above.
	prefix, ok := runGit(root, "rev-parse", "--show-prefix")
	if !ok {
		return nil
	}
	prefix = strings.TrimSpace(prefix)
	var pending []string
	for start := 0; start < len(changed); start += gitPathsPerCall {
		end := min(start+gitPathsPerCall, len(changed))
		got, ok := gitStatusPaths(root, prefix, changed[start:end])
		if !ok {
			return nil
		}
		for _, p := range got {
			pending = append(pending, strings.TrimPrefix(p, prefix))
		}
	}
	sort.Strings(pending)
	return pending
}

// removeMatching returns paths without any entry also in exclude. Used to
// keep a tracked-and-ignored file out of the "files to commit" hint:
// re-adding it is the opposite of what sync --untrack is for (#1330).
func removeMatching(paths, exclude []string) []string {
	if len(exclude) == 0 {
		return paths
	}
	skip := make(map[string]struct{}, len(exclude))
	for _, p := range exclude {
		skip[p] = struct{}{}
	}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if _, ok := skip[p]; !ok {
			out = append(out, p)
		}
	}
	return out
}

// gitPathsPerCall keeps each git invocation under the OS argument limit.
const gitPathsPerCall = 500

// gitTrackedAndIgnored returns the paths among candidates that git both
// tracks and ignores, plus such files under a candidate directory: a
// file the repo committed before it moved into a gitignore rule,
// typically the managed block, so the ignore has no effect until the
// index entry is removed (#1330). Outside a git work tree, or when git is
// missing or slow, it returns nothing, the same convenience contract as
// gitPending.
//
// One git call per batch of output paths made git rescan the index for
// each batch. Asking once for the top-level entries that hold outputs
// keeps one scan and keeps git out of unrelated trees such as vendor
// folders, where matching every file against every ignore rule is slow.
func gitTrackedAndIgnored(root string, candidates []string) []string {
	want := make(map[string]struct{}, len(candidates))
	tops := map[string]struct{}{}
	for _, c := range candidates {
		// git composes Unicode in the names it prints on macOS.
		c = norm.NFC.String(path.Clean(filepath.ToSlash(c)))
		if c == "." || c == ".." || strings.HasPrefix(c, "../") || path.IsAbs(c) {
			continue
		}
		want[c] = struct{}{}
		top, _, _ := strings.Cut(c, "/")
		tops[top] = struct{}{}
	}
	if len(want) == 0 {
		return nil
	}
	var out []string
	pathspecs := slices.Sorted(maps.Keys(tops))
	for start := 0; start < len(pathspecs); start += gitPathsPerCall {
		end := min(start+gitPathsPerCall, len(pathspecs))
		got, ok := runGit(root, append([]string{"ls-files", "-ci", "--exclude-standard", "-z", "--"}, pathspecs[start:end]...)...)
		if !ok {
			return nil
		}
		for _, p := range strings.Split(strings.TrimRight(got, "\x00"), "\x00") {
			if p != "" && underCandidate(norm.NFC.String(p), want) {
				out = append(out, p)
			}
		}
	}
	sort.Strings(out)
	return out
}

func underCandidate(p string, want map[string]struct{}) bool {
	for d := p; d != "." && d != "/"; d = path.Dir(d) {
		if _, ok := want[d]; ok {
			return true
		}
	}
	return false
}

// gitRmCached removes paths from git's index without touching the
// working tree, batched under the OS argument limit, and returns the
// paths actually removed. `sync --untrack` uses it to stop tracking a
// file its gitignore rule already covers.
//
// `git rm --cached` validates every pathspec in one invocation before
// touching the index: a single path it cannot remove (already
// untracked, a race, a permission error) fails the whole batch with
// nothing removed. A failed batch is retried one path at a time so a
// real partial failure still removes what it can; the returned error
// names exactly the paths that could not be removed, and every path
// not in that error is in removed.
func gitRmCached(root string, paths []string) (removed []string, err error) {
	var failed []string
	for start := 0; start < len(paths); start += gitPathsPerCall {
		end := min(start+gitPathsPerCall, len(paths))
		batch := paths[start:end]
		if rmCached(root, batch) == nil {
			removed = append(removed, batch...)
			continue
		}
		for _, p := range batch {
			if rmCached(root, []string{p}) == nil {
				removed = append(removed, p)
			} else {
				failed = append(failed, p)
			}
		}
	}
	if len(failed) > 0 {
		return removed, fmt.Errorf("git rm --cached in %s: %d of %d path(s) could not be untracked: %s", root, len(failed), len(paths), strings.Join(failed, " "))
	}
	return removed, nil
}

// rmCached runs one `git rm --cached` invocation over paths.
// --literal-pathspecs keeps a path holding `*`, `?`, `[`, or a leading
// `:` from being read as a glob or a magic pathspec.
func rmCached(root string, paths []string) error {
	args := append([]string{"--no-optional-locks", "--literal-pathspecs", "rm", "--cached", "--quiet", "--"}, paths...)
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// trackedFiles lists every git-tracked file under root, relative to it:
// `git ls-files` defaults to the working directory's own subtree, so no
// prefix-stripping is needed the way gitPending needs it for `status`.
// Used as the leftover-output scan's candidate set when no ledger names
// one (#1334). Outside a git work tree, or when git is missing or slow,
// it reports not ok, the same convenience contract as gitPending.
func trackedFiles(root string) ([]string, bool) {
	out, ok := runGit(root, "ls-files", "-z")
	if !ok {
		return nil, false
	}
	var files []string
	for _, p := range strings.Split(out, "\x00") {
		if p != "" {
			files = append(files, filepath.FromSlash(p))
		}
	}
	return files, true
}

func gitStatusPaths(root, prefix string, paths []string) ([]string, bool) {
	out, ok := runGit(root, append([]string{"status", "--porcelain=v1", "-z", "--untracked-files=all", "--"}, paths...)...)
	if !ok {
		return nil, false
	}
	var pending []string
	recs := strings.Split(out, "\x00")
	for i := 0; i < len(recs); i++ {
		rec := recs[i]
		if len(rec) <= 3 {
			continue
		}
		// A staged deletion of a file still on disk is `git rm --cached`
		// untracking an output the gitignore block now covers. Committing
		// that deletion is the user's own step, not sync's output to add.
		if rel, ok := strings.CutPrefix(rec[3:], prefix); ok && rec[0] == 'D' && rec[1] == ' ' && fileExists(filepath.Join(root, filepath.FromSlash(rel))) {
			continue
		}
		pending = append(pending, rec[3:])
		// A staged rename or copy carries its source path as the next record.
		if rec[0] == 'R' || rec[0] == 'C' {
			i++
		}
	}
	return pending, true
}

// runGit runs a read-only git command in root. --no-optional-locks keeps
// status from rewriting the index, so a sync (or --watch loop) never makes
// a concurrent git add or commit fail on index.lock.
func runGit(root string, args ...string) (string, bool) {
	return runGitWithin(root, 2*time.Second, args...)
}

// runGitWithin is runGit with its own time limit.
func runGitWithin(root string, timeout time.Duration, args ...string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"--no-optional-locks", "--literal-pathspecs"}, args...)...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return string(out), true
}

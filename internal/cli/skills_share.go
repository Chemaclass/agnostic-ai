package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// skillLink describes one planned symlink: path becomes a relative
// symlink pointing at canonical, the folder that keeps the real files.
type skillLink struct {
	path      string
	canonical string
}

// targetCapture pairs a target name with the files its adapter would
// write, collected via a capture-mode render (no IO).
type targetCapture struct {
	target string
	files  []adapters.CapturedFile
}

// sharedSkillsState carries the per-sync shared-skills plan between the
// pre-emit reconcile and the post-emit link swap.
type sharedSkillsState struct {
	enabled   bool
	coversAll bool
	links     []skillLink
	// captured maps every rendered output path to the set of contents
	// the enabled targets would write there. Used by partial-run
	// reconciliation to detect writes that would diverge through an
	// existing link.
	captured map[string]map[string]bool
	// unmanaged holds the sync.unmanaged patterns. A skill folder that
	// could hold a user-owned file is never linked, and an existing link
	// there becomes a real copy before emission.
	unmanaged []string
}

// planSharedSkills renders every effective target in capture mode and
// computes which emitted skill folders can collapse into symlinks.
// Returns an inert state when sync.shared-skills is off. Links are only
// planned on full runs (every configured target emitted): a partial run
// cannot see all participants, so it must not pick a canonical copy.
func planSharedSkills(cfg *config.Config, b spec.Bundle, targets []string) (*sharedSkillsState, error) {
	st := &sharedSkillsState{
		enabled:   cfg.Sync.SharedSkills,
		coversAll: coversAllConfiguredTargets(targets, cfg.Targets),
		unmanaged: cfg.Sync.Unmanaged,
	}
	if !st.enabled {
		return st, nil
	}
	captures, err := captureRenders(cfg, b, targets)
	if err != nil {
		return nil, err
	}
	if st.coversAll {
		st.links = planSkillLinks(captures, cfg.Sync.Unmanaged)
	}
	st.captured = map[string]map[string]bool{}
	for _, tc := range captures {
		for _, f := range tc.files {
			if st.captured[f.Path] == nil {
				st.captured[f.Path] = map[string]bool{}
			}
			st.captured[f.Path][f.Content] = true
		}
	}
	return st, nil
}

// captureRenders emits every resolvable target in capture mode and
// returns the per-target file sets. No files touch disk.
func captureRenders(cfg *config.Config, b spec.Bundle, targets []string) ([]targetCapture, error) {
	var out []targetCapture
	sess := adapters.NewSession()
	for _, t := range targets {
		adapter, err := adapters.Resolve(t)
		if err != nil {
			continue
		}
		sess.StartCapture()
		if err := adapters.EmitWithProvenance(sess, adapter, b, cfg, false); err != nil {
			sess.StopCapture()
			return nil, fmt.Errorf("%s: %w", t, err)
		}
		out = append(out, targetCapture{target: t, files: sess.StopCapture()})
	}
	return out, nil
}

// planSkillLinks groups the captured skill folders (`<root>/<name>/SKILL.md`
// plus every sibling file) by skill name, fingerprints each folder's
// rendered bytes, and plans one canonical + N links for every group of
// byte-identical folders. Divergent folders keep real copies. The
// `.agents/skills` tree is preferred as canonical because several tools
// scan it natively; otherwise the first-emitted folder wins. A folder
// that could hold a path matching unmanaged is left out entirely: as a
// link it would send the user's writes into another target's canonical
// copy, and as a canonical it would expose the user's file through links.
func planSkillLinks(captures []targetCapture, unmanaged []string) []skillLink {
	type folder struct {
		path  string
		order int
		files map[string]string
	}
	folders := map[string]*folder{}
	order := 0
	for _, tc := range captures {
		roots := skillFolderRoots(tc.files)
		for _, f := range tc.files {
			for _, r := range roots {
				if !underDir(f.Path, r) || config.MatchUnmanagedDir(unmanaged, r) {
					continue
				}
				fo := folders[r]
				if fo == nil {
					fo = &folder{path: r, order: order, files: map[string]string{}}
					order++
					folders[r] = fo
				}
				if rel, err := filepath.Rel(r, f.Path); err == nil {
					fo.files[rel] = f.Content
				}
				break // roots are non-nested, so at most one matches
			}
		}
	}

	byName := map[string][]*folder{}
	for _, fo := range folders {
		byName[filepath.Base(fo.path)] = append(byName[filepath.Base(fo.path)], fo)
	}

	agentsSkillsRoot := filepath.Join(".agents", "skills")
	var links []skillLink
	for _, group := range byName {
		if len(group) < 2 {
			continue
		}
		byPrint := map[string][]*folder{}
		for _, fo := range group {
			fp := folderFingerprint(fo.files)
			byPrint[fp] = append(byPrint[fp], fo)
		}
		for _, identical := range byPrint {
			if len(identical) < 2 {
				continue
			}
			canonical := identical[0]
			for _, fo := range identical[1:] {
				if filepath.Dir(fo.path) == agentsSkillsRoot ||
					(filepath.Dir(canonical.path) != agentsSkillsRoot && fo.order < canonical.order) {
					canonical = fo
				}
			}
			for _, fo := range identical {
				if fo != canonical {
					links = append(links, skillLink{path: fo.path, canonical: canonical.path})
				}
			}
		}
	}
	sort.Slice(links, func(i, j int) bool { return links[i].path < links[j].path })
	return links
}

// skillFolderRoots returns every captured folder that holds a SKILL.md,
// excluding folders nested inside another skill folder (a bundled asset
// that happens to be named SKILL.md must not be linked independently).
func skillFolderRoots(files []adapters.CapturedFile) []string {
	set := map[string]bool{}
	for _, f := range files {
		if filepath.Base(f.Path) == "SKILL.md" {
			set[filepath.Dir(f.Path)] = true
		}
	}
	var roots []string
	for r := range set {
		nested := false
		for other := range set {
			if other != r && underDir(r, other) {
				nested = true
				break
			}
		}
		if !nested {
			roots = append(roots, r)
		}
	}
	sort.Strings(roots)
	return roots
}

// underDir reports whether path lies strictly inside dir.
func underDir(path, dir string) bool {
	return strings.HasPrefix(path, dir+string(filepath.Separator))
}

// folderFingerprint hashes the rel-path -> content map so byte-identical
// folders compare equal regardless of capture order.
func folderFingerprint(files map[string]string) string {
	rels := make([]string, 0, len(files))
	for rel := range files {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	h := sha256.New()
	for _, rel := range rels {
		h.Write([]byte(rel))
		h.Write([]byte{0})
		h.Write([]byte(files[rel]))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// reconcile removes managed symlinks recorded in the prior ledger that
// no longer match the current plan, so adapters write real trees instead
// of pushing divergent bytes through a stale link into the canonical
// copy. Runs before emission on every real sync, feature on or off.
//
// On partial runs (some configured targets not emitted) an unplanned
// link survives unless this run would write different bytes through it:
// tearing it down without re-emitting the owner would leave that target
// with no skills at all.
//
// A link at a folder that could hold a user-owned file (sync.unmanaged)
// is replaced by a real copy of the bytes it resolves to, on every run.
// Removing it instead would let the canonical target overwrite a file
// the user edited through the link. The copy keeps that file in place;
// emission then regenerates the siblings. A failed copy aborts the sync
// rather than risk the user's file.
func (st *sharedSkillsState) reconcile(prior []string, dryRun bool) error {
	if dryRun {
		return nil
	}
	keep := map[string]string{}
	for _, l := range st.links {
		keep[l.path] = l.canonical
	}
	pruned := map[string]bool{}
	for _, p := range prior {
		fi, err := os.Lstat(p)
		if err != nil || fi.Mode()&os.ModeSymlink == 0 {
			continue
		}
		if config.MatchUnmanagedDir(st.unmanaged, p) {
			if err := materializeLink(p); err != nil {
				return fmt.Errorf("shared-skills: %w", err)
			}
			continue
		}
		if canonical, ok := keep[p]; ok {
			rel, relErr := filepath.Rel(filepath.Dir(p), canonical)
			if cur, readErr := os.Readlink(p); relErr == nil && readErr == nil && cur == rel {
				continue
			}
		} else if !st.coversAll && !st.capturedDiffersUnder(p) {
			continue
		}
		if os.Remove(p) == nil {
			pruneAncestorDirs(p, pruned)
		}
	}
	return nil
}

// materializeLink replaces the symlink at p with a real directory holding
// a copy of every regular file it resolves to. The copy is built in a
// fresh temporary sibling and swapped in only when complete, so a failed
// copy leaves the link untouched.
func materializeLink(p string) error {
	src, err := filepath.EvalSymlinks(p)
	if err != nil {
		return fmt.Errorf("%s: %w", p, err)
	}
	tmp, err := os.MkdirTemp(filepath.Dir(p), filepath.Base(p)+".agnostic-copy-")
	if err != nil {
		return fmt.Errorf("%s: %w", p, err)
	}
	if err := copyRegularTree(src, tmp); err != nil {
		_ = os.RemoveAll(tmp) // fresh MkdirTemp dir, holds only the partial copy
		return err
	}
	if err := os.Remove(p); err != nil {
		_ = os.RemoveAll(tmp)
		return fmt.Errorf("%s: %w", p, err)
	}
	if err := os.Rename(tmp, p); err != nil {
		return fmt.Errorf("%s: link removed, copy left at %s: %w", p, tmp, err)
	}
	return nil
}

// copyRegularTree copies every regular file under src into dst, keeping
// relative paths and permission bits. dst must exist.
func copyRegularTree(src, dst string) error {
	if err := os.Chmod(dst, 0o755); err != nil {
		return fmt.Errorf("%s: %w", dst, err)
	}
	return filepath.WalkDir(src, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		out := filepath.Join(dst, rel)
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(out), err)
		}
		if err := os.WriteFile(out, data, info.Mode().Perm()); err != nil {
			return fmt.Errorf("write %s: %w", out, err)
		}
		return nil
	})
}

// capturedDiffersUnder reports whether this run's rendered bytes for any
// file under the linked folder p differ from what is on disk (read
// through the link). A difference means an upcoming write would clobber
// the canonical copy, so the caller must unlink first.
func (st *sharedSkillsState) capturedDiffersUnder(p string) bool {
	for path, contents := range st.captured {
		if !underDir(path, p) {
			continue
		}
		disk, err := os.ReadFile(path)
		if err != nil {
			return true
		}
		for c := range contents {
			if string(disk) != c {
				return true
			}
		}
	}
	return false
}

// apply swaps each planned folder for its symlink after emission wrote
// the real trees. The link is created at a temporary name first so a
// filesystem without symlink support (e.g. Windows without the
// privilege) degrades to real copies without ever losing the tree.
// Folders that still contain user-authored files after the managed
// sweep keep their real copy. Returns the links now in place.
func (st *sharedSkillsState) apply(sess *adapters.Session, dryRun bool) []skillLink {
	var applied []skillLink
	warned := false
	warnf := func(format string, a ...any) {
		if !warned {
			fmt.Fprintf(os.Stderr, "! shared-skills: "+format+"; keeping per-target copies\n", a...)
			warned = true
		}
	}
	created := 0
	for _, l := range st.links {
		rel, err := filepath.Rel(filepath.Dir(l.path), l.canonical)
		if err != nil {
			warnf("%v", err)
			continue
		}
		if cur, err := os.Readlink(l.path); err == nil && cur == rel {
			applied = append(applied, l)
			continue
		}
		if dryRun {
			summaryf("  would link %s -> %s\n", l.path, rel)
			continue
		}
		tmp := l.path + ".agnostic-link"
		if fi, err := os.Lstat(tmp); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			// Leftover from an interrupted swap; anything else at this
			// name is user-owned and makes os.Symlink fail below.
			_ = os.Remove(tmp)
		}
		if err := os.Symlink(rel, tmp); err != nil {
			warnf("%v", err)
			continue
		}
		if err := sess.RemoveGeneratedTree(l.path, false); err != nil {
			_ = os.Remove(tmp)
			warnf("%v", err)
			continue
		}
		if _, err := os.Lstat(l.path); err == nil {
			// User-authored files kept the folder alive; it cannot
			// become a link.
			_ = os.Remove(tmp)
			continue
		}
		if err := os.Rename(tmp, l.path); err != nil {
			_ = os.Remove(tmp)
			warnf("%v", err)
			continue
		}
		applied = append(applied, l)
		created++
	}
	if !dryRun && created > 0 {
		summaryf("  linked %d shared skill folder%s\n", created, plural(created))
	}
	return applied
}

// adjustLedgerForLinks rewrites the sync ledger after the link swap:
// per-file entries under a linked folder disappear (the files now live
// only in the canonical tree, which keeps its own entries) and the link
// path itself is recorded so a later sync can sweep it when the skill
// or the opt-in goes away.
func adjustLedgerForLinks(session []string, applied []skillLink) []string {
	if len(applied) == 0 {
		return session
	}
	out := make([]string, 0, len(session)+len(applied))
	for _, p := range session {
		under := false
		for _, l := range applied {
			if underDir(p, l.path) {
				under = true
				break
			}
		}
		if !under {
			out = append(out, p)
		}
	}
	for _, l := range applied {
		out = append(out, l.path)
	}
	return out
}
